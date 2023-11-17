// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"crypto/sha256"
	"path/filepath"

	"github.com/cometbft/cometbft/abci/types"
	tm "github.com/cometbft/cometbft/config"
	tmcfg "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/libs/log"
	tmnode "github.com/cometbft/cometbft/node"
	tmp2p "github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/privval"
	"github.com/cometbft/cometbft/proxy"
	"github.com/cometbft/cometbft/rpc/client/local"
	"github.com/spf13/viper"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	tmapi "gitlab.com/accumulatenetwork/accumulate/internal/api/v3/tm"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/blockscheduler"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/crosschain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	execute "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/abci"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/exp/slog"
)

var (
	consensusProvidesEventBus  = provides[*events.Bus](func(c *ConsensusService) string { return c.App.partition().ID })
	consensusProvidesService   = provides[v3.ConsensusService](func(c ConsensusApp) string { return c.partition().ID })
	consensusProvidesSubmitter = provides[v3.Submitter](func(c ConsensusApp) string { return c.partition().ID })
	consensusProvidesValidator = provides[v3.Validator](func(c ConsensusApp) string { return c.partition().ID })

	coreConsensusNeedsStorage      = needs[keyvalue.Beginner](func(c *CoreConsensusApp) string { return c.Partition.ID })
	coreConsensusProvidesSequencer = provides[private.Sequencer](func(c *CoreConsensusApp) string { return c.Partition.ID })
)

type ConsensusApp interface {
	Type() ConsensusAppType
	CopyAsInterface() any

	partition() *protocol.PartitionInfo
	needs() []ServiceDescriptor
	provides() []ServiceDescriptor
	start(*Instance, *tendermint) (types.Application, error)
	register(*Instance, *tendermint, *tmnode.Node) error
}

type tendermint struct {
	config   *tm.Config
	privVal  *privval.FilePV
	nodeKey  *tmp2p.NodeKey
	logger   log.Logger
	eventBus *events.Bus
	globals  chan *network.GlobalValues
}

func (c *ConsensusService) needs() []ServiceDescriptor {
	return c.App.needs()
}

func (c *ConsensusService) provides() []ServiceDescriptor {
	return append(c.App.provides(),
		consensusProvidesEventBus.describe(c),
	)
}

func (c *ConsensusService) start(inst *Instance) error {
	d := new(tendermint)
	d.logger = (*logging.Slogger)(inst.logger)
	d.eventBus = events.NewBus(d.logger.With("module", "events"))

	events.SubscribeAsync(d.eventBus, func(e events.FatalError) {
		slog.ErrorCtx(inst.context, "Shutting down due to a fatal error", "error", e.Err)
		inst.cancel()
	})

	// Load config. Use Viper because that's what Tendermint does.{
	nodeDir := inst.path(c.NodeDir)
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
		return errors.UnknownError.WithFormat("load private validator key: %w", err)
	}

	d.nodeKey, err = tmp2p.LoadNodeKey(d.config.NodeKeyFile())
	if err != nil {
		return errors.UnknownError.WithFormat("load node key: %w", err)
	}

	// Start the application
	app, err := c.App.start(inst, d)
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
		return errors.UnknownError.WithFormat("initialize consensus: %w", err)
	}

	err = node.Start()
	if err != nil {
		return errors.UnknownError.WithFormat("start consensus: %w", err)
	}

	inst.cleanup(func() {
		err := node.Stop()
		if err != nil {
			slog.ErrorCtx(inst.context, "Error while stopping node", "error", err)
		}
		node.Wait()
	})

	err = consensusProvidesEventBus.register(inst, c, d.eventBus)

	return c.App.register(inst, d, node)
}

func (c *CoreConsensusApp) partition() *protocol.PartitionInfo { return c.Partition }

func (c *CoreConsensusApp) needs() []ServiceDescriptor {
	return []ServiceDescriptor{
		coreConsensusNeedsStorage.describe(c),
	}
}

func (c *CoreConsensusApp) provides() []ServiceDescriptor {
	return []ServiceDescriptor{
		consensusProvidesService.describe(c),
		consensusProvidesSubmitter.describe(c),
		consensusProvidesValidator.describe(c),
		coreConsensusProvidesSequencer.describe(c),
	}
}

func (c *CoreConsensusApp) start(inst *Instance, d *tendermint) (types.Application, error) {
	store, err := coreConsensusNeedsStorage.get(inst, c)
	if err != nil {
		return nil, err
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

	// Setup globals
	d.globals = make(chan *network.GlobalValues, 1)
	events.SubscribeSync(d.eventBus, func(e events.WillChangeGlobals) error {
		select {
		case d.globals <- e.New:
		default:
		}
		return nil
	})

	// On DNs initialize the major block scheduler
	if execOpts.Describe.NetworkType == protocol.PartitionTypeDirectory {
		execOpts.MajorBlockScheduler = blockscheduler.Init(execOpts.EventBus)
	}

	// This must happen before creating the executor since it needs to receive
	// the initial WillChangeGlobals event
	conductor := &crosschain.Conductor{
		Partition:    c.Partition,
		ValidatorKey: execOpts.Key,
		Database:     execOpts.Database,
		Querier:      v3.Querier2{Querier: client},
		Dispatcher:   execOpts.NewDispatcher(),
		RunTask:      execOpts.BackgroundTaskLauncher,
	}
	err = conductor.Start(d.eventBus)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("start conductor: %v", err)
	}

	exec, err := execute.NewExecutor(execOpts)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("initialize chain executor: %w", err)
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
	return app, nil
}

func (c *CoreConsensusApp) register(inst *Instance, d *tendermint, node *tmnode.Node) error {
	store, err := coreConsensusNeedsStorage.get(inst, c)
	if err != nil {
		return err
	}

	// Register the consensus service
	local := local.New(node)
	svcImpl := tmapi.NewConsensusService(tmapi.ConsensusServiceParams{
		Logger:           d.logger.With("module", "api"),
		Local:            local,
		Database:         database.New(store, d.logger),
		PartitionID:      c.Partition.ID,
		PartitionType:    c.Partition.Type,
		EventBus:         d.eventBus,
		NodeKeyHash:      sha256.Sum256(d.nodeKey.PubKey().Bytes()),
		ValidatorKeyHash: sha256.Sum256(d.privVal.Key.PubKey.Bytes()),
	})
	registerRpcService(inst, svcImpl.Type().AddressFor(c.Partition.ID), message.ConsensusService{ConsensusService: svcImpl})
	err = consensusProvidesService.register(inst, c, svcImpl)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Register the submitter
	subImpl := tmapi.NewSubmitter(tmapi.SubmitterParams{
		Logger: d.logger.With("module", "api"),
		Local:  local,
	})
	registerRpcService(inst, subImpl.Type().AddressFor(c.Partition.ID), message.Submitter{Submitter: subImpl})
	err = consensusProvidesSubmitter.register(inst, c, subImpl)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Register the validator
	valImpl := tmapi.NewValidator(tmapi.ValidatorParams{
		Logger: d.logger.With("module", "api"),
		Local:  local,
	})
	registerRpcService(inst, valImpl.Type().AddressFor(c.Partition.ID), message.Validator{Validator: valImpl})
	err = consensusProvidesValidator.register(inst, c, valImpl)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Register the sequencer
	seqImpl := api.NewSequencer(api.SequencerParams{
		Logger:       d.logger.With("module", "api"),
		Database:     database.New(store, d.logger),
		EventBus:     d.eventBus,
		Globals:      <-d.globals,
		Partition:    c.Partition.ID,
		ValidatorKey: d.privVal.Key.PrivKey.Bytes(),
	})
	registerRpcService(inst, seqImpl.Type().AddressFor(c.Partition.ID), message.Sequencer{Sequencer: seqImpl})
	err = coreConsensusProvidesSequencer.register(inst, c, seqImpl)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	return nil
}
