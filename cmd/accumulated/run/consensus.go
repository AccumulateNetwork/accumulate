// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	types "github.com/cometbft/cometbft/abci/types"
	tmcfg "github.com/cometbft/cometbft/config"
	tmcrypto "github.com/cometbft/cometbft/crypto"
	tmed25519 "github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cometbft/cometbft/crypto/tmhash"
	cmtjson "github.com/cometbft/cometbft/libs/json"
	"github.com/cometbft/cometbft/libs/log"
	tmnode "github.com/cometbft/cometbft/node"
	tmp2p "github.com/cometbft/cometbft/p2p"
	tmpv "github.com/cometbft/cometbft/privval"
	"github.com/cometbft/cometbft/proxy"
	"github.com/cometbft/cometbft/rpc/client"
	tmrpc "github.com/cometbft/cometbft/rpc/client"
	"github.com/cometbft/cometbft/rpc/client/local"
	tmtypes "github.com/cometbft/cometbft/types"
	"github.com/fatih/color"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multihash"
	"github.com/spf13/viper"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	tmlib "gitlab.com/accumulatenetwork/accumulate/exp/tendermint"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	tmapi "gitlab.com/accumulatenetwork/accumulate/internal/api/v3/tm"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/crosschain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	execute "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/abci"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var (
	consensusProvidesEventBus  = ioc.Provides[*events.Bus](func(c *ConsensusService) string { return c.App.partition().ID })
	consensusProvidesService   = ioc.Provides[v3.ConsensusService](func(c ConsensusApp) string { return c.partition().ID })
	consensusProvidesSubmitter = ioc.Provides[v3.Submitter](func(c ConsensusApp) string { return c.partition().ID })
	consensusProvidesValidator = ioc.Provides[v3.Validator](func(c ConsensusApp) string { return c.partition().ID })

	coreConsensusNeedsStorage      = ioc.Needs[keyvalue.Beginner](func(c *CoreConsensusApp) string { return c.Partition.ID })
	coreConsensusProvidesSequencer = ioc.Provides[private.Sequencer](func(c *CoreConsensusApp) string { return c.Partition.ID })
	coreConsensusProvidesRouter    = ioc.Provides[routing.Router](func(c *CoreConsensusApp) string { return c.Partition.ID })
	coreConsensusProvidesClient    = ioc.Provides[client.Client](func(c *CoreConsensusApp) string { return c.Partition.ID })
)

type tendermint struct {
	config   *tmcfg.Config
	privVal  *tmpv.FilePV
	nodeKey  *tmp2p.NodeKey
	logger   log.Logger
	eventBus *events.Bus
	globals  chan *network.GlobalValues
}

var _ prestarter = (*ConsensusService)(nil)

func (c *ConsensusService) Requires() []ioc.Requirement {
	return c.App.Requires()
}

func (c *ConsensusService) Provides() []ioc.Provided {
	return append(c.App.Provides(),
		consensusProvidesEventBus.Provided(c),
	)
}

func (c *ConsensusService) Verify() error {
	// Verify bootstrap peers can be converted to CometBFT format
	for i, peer := range c.BootstrapPeers {
		_, err := cmtPeerAddress(peer)
		if err != nil {
			return errors.UnknownError.WithFormat("bootstrap peer %d: %w", i, err)
		}
	}
	return nil
}

func (c *ConsensusService) prestart(inst *Instance) error {
	return c.App.prestart(inst)
}

func (c *ConsensusService) start(inst *Instance) error {
	// Defaults
	setDefaultVal(&c.MetricsNamespace, fmt.Sprintf("consensus_%s", c.App.partition().ID))

	d := new(tendermint)
	d.logger = (*logging.Slogger)(inst.logger)
	d.eventBus = events.NewBus(d.logger.With("module", "events"))

	events.SubscribeAsync(d.eventBus, func(e events.FatalError) {
		slog.ErrorContext(inst.context, "Shutting down due to a fatal error", "error", e.Err)
		inst.shutdown()
	})

	// Make the node directories
	err := os.MkdirAll(inst.path(c.NodeDir, "config"), 0700)
	if err != nil {
		return err
	}
	err = os.MkdirAll(inst.path(c.NodeDir, "data"), 0700)
	if err != nil {
		return err
	}

	// Load CometBFT config
	d.config = tmcfg.DefaultConfig()
	d.config.SetRoot(inst.path(c.NodeDir))
	_, err = os.Stat(inst.path(c.NodeDir, "config", "tendermint.toml"))
	switch {
	case err == nil:
		// Load the existing file with Viper because that's what Tendermint does
		nodeDir := inst.path(c.NodeDir)
		v := viper.New()
		v.SetConfigFile(filepath.Join(nodeDir, "config", "tendermint.toml"))
		v.AddConfigPath(filepath.Join(nodeDir, "config"))
		err = v.ReadInConfig()
		if err != nil {
			return err
		}

		err = v.Unmarshal(d.config)
		if err != nil {
			return err
		}

		if d.config.Instrumentation.Prometheus {
			d.config.Instrumentation.Namespace = c.MetricsNamespace
		}

	case errors.Is(err, fs.ErrNotExist):
		d.config.NodeKey = ""
		d.config.PrivValidatorKey = ""
		d.config.Genesis = filepath.Join("..", c.Genesis)
		d.config.Mempool.MaxTxBytes = 4194304

		d.config.Instrumentation.Prometheus = true
		d.config.Instrumentation.PrometheusListenAddr = listenHostPort(c.Listen, defaultHost, portMetrics)
		d.config.Instrumentation.Namespace = c.MetricsNamespace

		d.config.P2P.ListenAddress = listenUrl(c.Listen, defaultHost, useTCP{}, portCmtP2P)
		d.config.RPC.ListenAddress = listenUrl(c.Listen, defaultHost, useTCP{}, portCmtRPC)

		// No duplicate IPs
		d.config.P2P.AllowDuplicateIP = false

		// Initial peers (should be bootstrap peers but that setting isn't
		// present in 0.37)
		for i, peer := range c.BootstrapPeers {
			id, err := cmtPeerAddress(peer)
			if err != nil {
				return errors.UnknownError.WithFormat("bootstrap peer %d: %w", i, err)
			}
			if i > 0 {
				d.config.P2P.PersistentPeers += ","
			}
			d.config.P2P.PersistentPeers += id
		}

		// Set whether unroutable addresses are allowed
		d.config.P2P.AddrBookStrict = !isPrivate(c.Listen)

		tmcfg.WriteConfigFile(inst.path(c.NodeDir, "config", "tendermint.toml"), d.config)

	default:
		return err
	}

	err = d.config.ValidateBasic()
	if err != nil {
		return err
	}

	// Load keys
	d.privVal, err = c.loadPrivVal(inst, d.config, c.ValidatorKey)
	if err != nil {
		return errors.UnknownError.WithFormat("load private validator key: %w", err)
	}

	d.nodeKey, err = convertNodeKey(inst)
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
		c.genesisDocProvider(inst),
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

	inst.cleanup("consensus node", func(context.Context) error {
		err := node.Stop()
		node.Wait()
		return err
	})

	err = consensusProvidesEventBus.Register(inst.services, c, d.eventBus)
	if err != nil {
		return err
	}

	return c.App.register(inst, d, node)
}

func convertNodeKey(inst *Instance) (*tmp2p.NodeKey, error) {
	var key PrivateKey
	if inst.config.P2P != nil {
		key = inst.config.P2P.Key
	}
	key2, err := convertKeyToComet(inst, key)
	if err != nil {
		return nil, err
	}
	return &tmp2p.NodeKey{PrivKey: key2}, nil
}

func (c *ConsensusService) loadPrivVal(inst *Instance, config *tmcfg.Config, key PrivateKey) (*tmpv.FilePV, error) {
	key2, err := convertKeyToComet(inst, key)
	if err != nil {
		return nil, err
	}

	// This is a hack to work around CometBFT
	pv := tmpv.NewFilePV(key2, "", config.PrivValidatorStateFile())

	b, err := os.ReadFile(config.PrivValidatorStateFile())
	switch {
	case err == nil:
		err = cmtjson.Unmarshal(b, &pv.LastSignState)
		return pv, err
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}

	b, err = cmtjson.MarshalIndent(pv.LastSignState, "", "  ")
	if err != nil {
		return nil, err
	}
	err = os.WriteFile(config.PrivValidatorStateFile(), b, 0600)
	return pv, err
}

func convertKeyToComet(inst *Instance, key PrivateKey) (tmcrypto.PrivKey, error) {
	switch key.(type) {
	case nil:
		return nil, errors.BadRequest.With("key is nil")
	case *TransientPrivateKey:
		return nil, errors.BadRequest.With("key is transient")
	}

	addr, err := key.get(inst)
	if err != nil {
		return nil, err
	}

	sk, ok := addr.GetPrivateKey()
	if !ok {
		return nil, errors.BadRequest.With("not a private key")
	}

	switch addr.GetType() {
	case protocol.SignatureTypeED25519:
		return tmed25519.PrivKey(sk), nil
	default:
		return nil, errors.BadRequest.With("unsupported key type %v", addr.GetType())
	}
}

func (c *ConsensusService) genesisDocProvider(inst *Instance) tmnode.GenesisDocProvider {
	path := inst.path(c.Genesis)

	if filepath.Ext(c.Genesis) == ".json" {
		return func() (*tmtypes.GenesisDoc, error) {
			return tmtypes.GenesisDocFromFile(path)
		}
	}

	return func() (*tmtypes.GenesisDoc, error) {
		// Open the snapshot
		all, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		return genesis.ConvertSnapshotToJson(all)
	}
}

func cmtPeerAddress(addr multiaddr.Multiaddr) (string, error) {
	var pub *multihash.DecodedMultihash
	var host, port string
	var err error
	multiaddr.ForEach(addr, func(c multiaddr.Component) bool {
		switch c.Protocol().Code {
		case multiaddr.P_P2P:
			pub, err = multihash.Decode(c.RawValue())
		case multiaddr.P_IP4,
			multiaddr.P_IP6,
			multiaddr.P_DNS,
			multiaddr.P_DNS4,
			multiaddr.P_DNS6:
			host = c.Value()
		case multiaddr.P_TCP,
			multiaddr.P_UDP:
			port = c.Value()
		}
		if err != nil {
			return false
		}
		return pub == nil || host == "" || port == ""
	})
	if err != nil {
		return "", err
	}
	if pub == nil {
		return "", errors.BadRequest.With("missing peer ID")
	}
	if host == "" {
		return "", errors.BadRequest.With("missing host")
	}
	if port == "" {
		return "", errors.BadRequest.With("missing port")
	}

	var hash []byte
	switch pub.Code {
	case multihash.IDENTITY:
		p, err := crypto.UnmarshalPublicKey(pub.Digest)
		if err != nil {
			return "", errors.BadRequest.WithFormat("decode public key: %w", err)
		}
		b, err := p.Raw()
		if err != nil {
			return "", errors.BadRequest.WithFormat("unwrap public key: %w", err)
		}
		hash = tmhash.SumTruncated(b)
	case multihash.SHA2_256:
		hash = pub.Digest[:tmhash.TruncatedSize]
	default:
		return "", errors.BadRequest.With("unsupported multihash type %v", pub.Name)
	}
	return tmp2p.IDAddressString(tmp2p.ID(hex.EncodeToString(hash)), fmt.Sprintf("%s:%s", host, port)), nil
}

func (c *CoreConsensusApp) partition() *protocol.PartitionInfo { return c.Partition }

func (c *CoreConsensusApp) Requires() []ioc.Requirement {
	return []ioc.Requirement{
		coreConsensusNeedsStorage.Requirement(c),
	}
}

func (c *CoreConsensusApp) Provides() []ioc.Provided {
	return []ioc.Provided{
		consensusProvidesService.Provided(c),
		consensusProvidesSubmitter.Provided(c),
		consensusProvidesValidator.Provided(c),
		coreConsensusProvidesSequencer.Provided(c),
		coreConsensusProvidesRouter.Provided(c),
		coreConsensusProvidesClient.Provided(c),
	}
}

func (c *CoreConsensusApp) prestart(inst *Instance) error {
	return coreConsensusProvidesClient.Register(inst.services, c, tmlib.NewDeferredClient())
}

func (c *CoreConsensusApp) start(inst *Instance, d *tendermint) (types.Application, error) {
	setDefaultPtr(&c.EnableHealing, false)
	setDefaultPtr(&c.EnableDirectDispatch, true)
	setDefaultPtr(&c.MaxEnvelopesPerBlock, 100)

	store, err := coreConsensusNeedsStorage.Get(inst.services, c)
	if err != nil {
		return nil, err
	}

	router := routing.NewRouter(routing.RouterOptions{
		Events: d.eventBus,
		Logger: d.logger,
	})
	err = coreConsensusProvidesRouter.Register(inst.services, c, router)
	if err != nil {
		return nil, err
	}

	db := database.New(store, d.logger)
	bli := indexing.NewBlockLedgerIndexer(inst.context, db, c.Partition.ID)

	dialer := inst.p2p.DialNetwork()
	client := &message.Client{Transport: &message.RoutedTransport{
		Network: inst.config.Network,
		Dialer:  dialer,
		Router:  routing.MessageRouter{Router: router},
	}}
	execOpts := execute.Options{
		Logger:        d.logger.With("module", "executor"),
		Database:      db,
		Key:           d.privVal.Key.PrivKey.Bytes(),
		Router:        router,
		EventBus:      d.eventBus,
		Sequencer:     client.Private(),
		Querier:       client,
		EnableHealing: *c.EnableHealing,
		Describe: execute.DescribeShim{
			NetworkType: c.Partition.Type,
			PartitionId: c.Partition.ID,
		},
		Indexers: []func(*database.Batch){
			bli.Write,
		},
	}

	// Why does this exist? Why not just use tmlib.DispatcherClient?
	type Client interface {
		tmrpc.ABCIClient
		tmrpc.NetworkClient
		tmrpc.MempoolClient
		tmrpc.StatusClient
	}

	clients := map[string]tmlib.DispatcherClient{}
	ioc.ForEach(inst.services, func(desc ioc.Descriptor, svc Client) {
		clients[strings.ToLower(desc.Namespace())] = svc
	})

	if _, ok := clients["directory"]; !ok ||
		!*c.EnableDirectDispatch {
		// If we are not attached to a DN node, or direct dispatch is disabled,
		// use the API dispatcher
		execOpts.NewDispatcher = func() execute.Dispatcher {
			return accumulated.NewDispatcher(inst.config.Network, router, dialer)
		}

	} else {
		// Otherwise, use the Tendermint dispatcher
		execOpts.NewDispatcher = func() execute.Dispatcher {
			return tmlib.NewDispatcher(router, clients)
		}
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

	// This must happen before creating the executor since it needs to receive
	// the initial WillChangeGlobals event
	conductor := &crosschain.Conductor{
		Partition:    c.Partition,
		ValidatorKey: execOpts.Key,
		Database:     execOpts.Database,
		Querier:      v3.Querier2{Querier: client},
		Dispatcher:   execOpts.NewDispatcher(),
		RunTask:      execOpts.BackgroundTaskLauncher,

		// TODO Fix the flooding issues and enable this by default
		EnableAnchorHealing: Ptr(false),
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
		ID:        inst.id,
		Address:   d.privVal.Key.PubKey.Address(),
		Executor:  exec,
		Logger:    d.logger.With("module", "abci"),
		EventBus:  d.eventBus,
		Database:  db,
		Genesis:   genesis.DocProvider(d.config),
		Partition: c.Partition.ID,
		RootDir:   d.config.RootDir,

		MaxEnvelopesPerBlock: int(*c.MaxEnvelopesPerBlock),
	})
	return app, nil
}

func (c *CoreConsensusApp) register(inst *Instance, d *tendermint, node *tmnode.Node) error {
	store, err := coreConsensusNeedsStorage.Get(inst.services, c)
	if err != nil {
		return err
	}

	// Register the tendermint node
	local := local.New(node)
	err = coreConsensusProvidesClient.Register(inst.services, c, local)
	if err != nil {
		return err
	}

	// Register the consensus service
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
	err = consensusProvidesService.Register(inst.services, c, svcImpl)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Register the submitter
	subImpl := tmapi.NewSubmitter(tmapi.SubmitterParams{
		Logger: d.logger.With("module", "api"),
		Local:  local,
	})
	registerRpcService(inst, subImpl.Type().AddressFor(c.Partition.ID), message.Submitter{Submitter: subImpl})
	err = consensusProvidesSubmitter.Register(inst.services, c, subImpl)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Register the validator
	valImpl := tmapi.NewValidator(tmapi.ValidatorParams{
		Logger: d.logger.With("module", "api"),
		Local:  local,
	})
	registerRpcService(inst, valImpl.Type().AddressFor(c.Partition.ID), message.Validator{Validator: valImpl})
	err = consensusProvidesValidator.Register(inst.services, c, valImpl)
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
	err = coreConsensusProvidesSequencer.Register(inst.services, c, seqImpl)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	inst.logger.Info(color.HiBlueString("Running"), "partition", c.Partition.ID, "module", "run", "service", "consensus")
	return nil
}
