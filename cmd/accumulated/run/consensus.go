// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"path/filepath"

	"github.com/cometbft/cometbft/abci/types"
	tm "github.com/cometbft/cometbft/config"
	tmcfg "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/libs/log"
	tmnode "github.com/cometbft/cometbft/node"
	tmp2p "github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/privval"
	"github.com/cometbft/cometbft/proxy"
	"github.com/cometbft/cometbft/rpc/client"
	"github.com/cometbft/cometbft/rpc/client/local"
	"github.com/spf13/viper"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/blockscheduler"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	execute "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/abci"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/exp/slog"
)

type ConsensusApp interface {
	Type() ConsensusAppType
	CopyAsInterface() any

	start(*Instance, *tendermint) (string, types.Application, error)
}

type tendermint struct {
	config   *tm.Config
	privVal  *privval.FilePV
	nodeKey  *tmp2p.NodeKey
	logger   log.Logger
	eventBus *events.Bus
}

func (s *ConsensusService) start(inst *Instance) error {
	d := new(tendermint)
	d.logger = (*logging.Slogger)(inst.logger)
	d.eventBus = events.NewBus(d.logger.With("module", "events"))

	events.SubscribeAsync(d.eventBus, func(e events.FatalError) {
		slog.ErrorCtx(inst.context, "Shutting down due to a fatal error", "error", e.Err)
		inst.cancel()
	})

	// Load config. Use Viper because that's what Tendermint does.{
	nodeDir := inst.path(s.NodeDir)
	v := viper.New()
	v.SetConfigFile(filepath.Join(nodeDir, "config", "tendermint.toml"))
	v.AddConfigPath(filepath.Join(nodeDir, "config"))
	err := v.ReadInConfig()
	if err != nil {
		return err
	}

	d.config = tm.DefaultConfig()
	err = v.Unmarshal(d.config)
	if err != nil {
		return err
	}

	d.config.SetRoot(nodeDir)
	err = d.config.ValidateBasic()
	if err != nil {
		return err
	}

	// Load keys
	d.privVal, err = config.LoadFilePV(
		d.config.PrivValidatorKeyFile(),
		d.config.PrivValidatorStateFile(),
	)
	if err != nil {
		return errors.UnknownError.WithFormat("load private validator key: %v", err)
	}

	d.nodeKey, err = tmp2p.LoadNodeKey(d.config.NodeKeyFile())
	if err != nil {
		return errors.UnknownError.WithFormat("load node key: %v", err)
	}

	// Start the application
	name, app, err := s.App.start(inst, d)
	if err != nil {
		return err
	}

	// Start consensus
	node, err := tmnode.NewNode(
		d.config,
		d.privVal,
		d.nodeKey,
		proxy.NewLocalClientCreator(app),
		genesis.DocProvider(d.config),
		tmcfg.DefaultDBProvider,
		tmnode.DefaultMetricsProvider(d.config.Instrumentation),
		d.logger,
	)
	if err != nil {
		return errors.UnknownError.WithFormat("initialize consensus: %v", err)
	}

	err = node.Start()
	if err != nil {
		return errors.UnknownError.WithFormat("start consensus: %v", err)
	}

	inst.cleanup(func() {
		err := node.Stop()
		if err != nil {
			slog.ErrorCtx(inst.context, "Error while stopping node", "error", err)
		}
		node.Wait()
	})

	// Register a local client
	return registerService[client.Client](inst, name, local.New(node))
}

func (c *CoreConsensusApp) start(inst *Instance, d *tendermint) (string, types.Application, error) {
	store, err := getService[keyvalue.Beginner](inst, c.Storage)
	if err != nil {
		return "", nil, err
	}

	router := routing.NewRouter(d.eventBus, d.logger)

	dialer := inst.p2p.DialNetwork()
	client := &message.Client{Transport: &message.RoutedTransport{
		Network: inst.network,
		Dialer:  dialer,
		Router:  routing.MessageRouter{Router: router},
	}}
	execOpts := execute.Options{
		Logger:        d.logger.With("module", "executor"),
		Database:      database.New(store, d.logger),
		Key:           d.privVal.Key.PrivKey.Bytes(),
		Router:        router,
		EventBus:      d.eventBus,
		Sequencer:     client.Private(),
		Querier:       client,
		EnableHealing: c.EnableHealing,
		Describe: execute.DescribeShim{
			NetworkType: c.Partition.Type,
			PartitionId: c.Partition.ID,
		},
		NewDispatcher: func() execute.Dispatcher {
			return accumulated.NewDispatcher(inst.network, router, dialer)
		},
	}

	// On DNs initialize the major block scheduler
	if execOpts.Describe.NetworkType == protocol.PartitionTypeDirectory {
		execOpts.MajorBlockScheduler = blockscheduler.Init(execOpts.EventBus)
	}

	exec, err := execute.NewExecutor(execOpts)
	if err != nil {
		return "", nil, errors.UnknownError.WithFormat("initialize chain executor: %v", err)
	}

	app := abci.NewAccumulator(abci.AccumulatorOptions{
		Address:   d.privVal.Key.PubKey.Address(),
		Executor:  exec,
		Logger:    d.logger.With("module", "abci"),
		EventBus:  d.eventBus,
		Database:  database.New(store, d.logger),
		Genesis:   genesis.DocProvider(d.config),
		Partition: c.Partition.ID,
		RootDir:   d.config.RootDir,
	})
	return c.Partition.ID, app, nil
}
