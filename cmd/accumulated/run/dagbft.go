// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/crosschain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	execute "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	v3api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// dagbftState holds the DAG-BFT node state.
type dagbftState struct {
	node      *consensus.Node
	committee *types.Committee
	privKey   ed25519.PrivateKey
	eventBus  *events.Bus
	globals   chan *network.GlobalValues
	logger    *logging.Slogger
	host      host.Host
	pubsub    *pubsub.PubSub
}

// DAGBFTConsensusService represents a consensus service using DAG-BFT.
type DAGBFTConsensusService struct {
	// NodeDir is the directory for node data.
	NodeDir string

	// ValidatorKey is the validator's private key.
	ValidatorKey PrivateKey

	// Genesis is the path to the genesis file.
	Genesis string

	// Listen is the address to listen on.
	Listen Multiaddr

	// BootstrapPeers are the initial peers to connect to.
	BootstrapPeers []Multiaddr

	// MetricsNamespace is the namespace for metrics.
	MetricsNamespace string

	// NumWorkers is the number of DAG-BFT workers.
	NumWorkers int

	// DAGGCDepth is the garbage collection depth for the DAG.
	DAGGCDepth int

	// App is the consensus application.
	App ConsensusApp
}

var (
	dagbftProvidesEventBus = ioc.Provides[*events.Bus](func(c *DAGBFTConsensusService) string { return c.App.partition().ID })
)

func (c *DAGBFTConsensusService) Type() ServiceType { return ServiceTypeConsensus }

func (c *DAGBFTConsensusService) Requires() []ioc.Requirement {
	return c.App.Requires()
}

func (c *DAGBFTConsensusService) Provides() []ioc.Provided {
	return append(c.App.Provides(),
		dagbftProvidesEventBus.Provided(c),
	)
}

func (c *DAGBFTConsensusService) prestart(inst *Instance) error {
	return c.App.prestart(inst)
}

func (c *DAGBFTConsensusService) start(inst *Instance) error {
	// Set defaults
	setDefaultVal(&c.MetricsNamespace, fmt.Sprintf("dagbft_%s", c.App.partition().ID))
	if c.NumWorkers <= 0 {
		c.NumWorkers = consensus.DefaultNumWorkers
	}
	if c.DAGGCDepth <= 0 {
		c.DAGGCDepth = consensus.DefaultDAGGCDepth
	}

	d := new(dagbftState)
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

	// Load validator key
	if c.ValidatorKey == nil {
		return errors.BadRequest.With("validator key is required")
	}
	keyAddr, err := c.ValidatorKey.get(inst)
	if err != nil {
		return errors.UnknownError.WithFormat("load validator key: %w", err)
	}
	sk, ok := keyAddr.GetPrivateKey()
	if !ok {
		return errors.BadRequest.With("validator key is not a private key")
	}
	if len(sk) != ed25519.PrivateKeySize {
		return errors.BadRequest.With("validator key must be ed25519")
	}
	d.privKey = ed25519.PrivateKey(sk)

	// Load committee from genesis
	committee, err := c.loadCommitteeFromGenesis(inst)
	if err != nil {
		return errors.UnknownError.WithFormat("load committee from genesis: %w", err)
	}
	d.committee = committee

	// Create libp2p host for DAG-BFT consensus
	d.host, d.pubsub, err = c.createLibp2pHost(inst.context, d.privKey)
	if err != nil {
		return errors.UnknownError.WithFormat("create libp2p host: %w", err)
	}

	// Connect to bootstrap peers
	for _, peerAddr := range c.BootstrapPeers {
		peerInfo, err := peer.AddrInfoFromP2pAddr(peerAddr)
		if err != nil {
			slog.Warn("Invalid bootstrap peer address", "addr", peerAddr, "error", err)
			continue
		}
		if err := d.host.Connect(inst.context, *peerInfo); err != nil {
			slog.Warn("Failed to connect to bootstrap peer", "peer", peerInfo.ID, "error", err)
		} else {
			slog.Info("Connected to bootstrap peer", "peer", peerInfo.ID)
		}
	}

	// Create DAG-BFT node configuration
	nodeConfig := consensus.NodeConfig{
		Partition:        c.App.partition().ID,
		KeyPair:          d.privKey,
		NumWorkers:       c.NumWorkers,
		DAGGCDepth:       types.Round(c.DAGGCDepth),
		CommitBufferSize: consensus.DefaultCommitBufferSize,
	}

	// Create consensus node
	d.node, err = consensus.NewNode(nodeConfig, committee, d.host, d.pubsub)
	if err != nil {
		return errors.UnknownError.WithFormat("create DAG-BFT node: %w", err)
	}

	// Register event bus
	err = dagbftProvidesEventBus.Register(inst.services, c, d.eventBus)
	if err != nil {
		return err
	}

	// Start application and executor
	err = c.startDAGBFTApp(inst, d)
	if err != nil {
		return err
	}

	// Start consensus node
	if err := d.node.Start(inst.context); err != nil {
		return errors.UnknownError.WithFormat("start DAG-BFT node: %w", err)
	}

	inst.cleanup("dagbft node", func(context.Context) error {
		d.node.Stop()
		return d.host.Close()
	})

	slog.Info("DAG-BFT consensus started",
		"partition", c.App.partition().ID,
		"validators", len(committee.Validators),
		"numWorkers", c.NumWorkers)

	return nil
}

// createLibp2pHost creates a libp2p host and pubsub for DAG-BFT.
func (c *DAGBFTConsensusService) createLibp2pHost(ctx context.Context, privKey ed25519.PrivateKey) (host.Host, *pubsub.PubSub, error) {
	// Convert ed25519 key to libp2p format
	libp2pKey, _, err := crypto.KeyPairFromStdKey(&privKey)
	if err != nil {
		return nil, nil, fmt.Errorf("convert key: %w", err)
	}

	// Determine listen address
	var listenAddrs []multiaddr.Multiaddr
	if c.Listen != nil {
		listenAddrs = append(listenAddrs, c.Listen)
	} else {
		defaultAddr, _ := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/9000")
		listenAddrs = append(listenAddrs, defaultAddr)
	}

	// Create host
	h, err := libp2p.New(
		libp2p.Identity(libp2pKey),
		libp2p.ListenAddrs(listenAddrs...),
		libp2p.EnableRelay(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create host: %w", err)
	}

	// Create pubsub
	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		h.Close()
		return nil, nil, fmt.Errorf("create pubsub: %w", err)
	}

	slog.Info("DAG-BFT libp2p host created",
		"id", h.ID(),
		"addrs", h.Addrs())

	return h, ps, nil
}

// loadCommitteeFromGenesis loads the validator committee from the genesis file.
func (c *DAGBFTConsensusService) loadCommitteeFromGenesis(inst *Instance) (*types.Committee, error) {
	path := inst.path(c.Genesis)

	// Read genesis snapshot or JSON
	all, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read genesis: %w", err)
	}

	// Convert to CometBFT genesis doc to extract validators
	genDoc, err := genesis.ConvertSnapshotToJson(all)
	if err != nil {
		return nil, fmt.Errorf("parse genesis: %w", err)
	}

	// Extract validators
	var validators []types.ValidatorInfo
	for _, val := range genDoc.Validators {
		pubKeyBytes := val.PubKey.Bytes()
		if len(pubKeyBytes) != ed25519.PublicKeySize {
			continue
		}
		validators = append(validators, types.ValidatorInfo{
			PublicKey: pubKeyBytes,
			Stake:     uint64(val.Power),
		})
	}

	if len(validators) == 0 {
		return nil, fmt.Errorf("no validators found in genesis")
	}

	return types.NewCommittee(validators, 1), nil
}

// startDAGBFTApp initializes and starts the application layer for DAG-BFT.
func (c *DAGBFTConsensusService) startDAGBFTApp(inst *Instance, d *dagbftState) error {
	app, ok := c.App.(*CoreConsensusApp)
	if !ok {
		return errors.BadRequest.WithFormat("unsupported app type for DAG-BFT: %T", c.App)
	}

	setDefaultPtr(&app.EnableHealing, false)
	setDefaultPtr(&app.EnableDirectDispatch, true)
	setDefaultPtr(&app.MaxEnvelopesPerBlock, 100)

	store, err := coreConsensusNeedsStorage.Get(inst.services, app)
	if err != nil {
		return err
	}

	router := routing.NewRouter(routing.RouterOptions{
		Events: d.eventBus,
		Logger: d.logger,
	})
	err = coreConsensusProvidesRouter.Register(inst.services, app, router)
	if err != nil {
		return err
	}

	dialer := inst.p2p.DialNetwork()
	client := &message.Client{Transport: &message.RoutedTransport{
		Network: inst.config.Network,
		Dialer:  dialer,
		Router:  routing.MessageRouter{Router: router},
	}}

	db := database.New(store, d.logger)
	execOpts := execute.Options{
		Logger:        d.logger.With("module", "executor"),
		Database:      db,
		Key:           d.privKey,
		Router:        router,
		EventBus:      d.eventBus,
		Sequencer:     client.Private(),
		Querier:       client,
		EnableHealing: *app.EnableHealing,
		Describe: execute.DescribeShim{
			NetworkType: app.Partition.Type,
			PartitionId: app.Partition.ID,
		},
	}

	// Set up dispatcher
	execOpts.NewDispatcher = func() execute.Dispatcher {
		return accumulated.NewDispatcher(inst.config.Network, router, dialer)
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

	// Create conductor
	conductor := &crosschain.Conductor{
		Partition:           app.Partition,
		ValidatorKey:        execOpts.Key,
		Database:            execOpts.Database,
		Querier:             v3api.Querier2{Querier: client},
		Dispatcher:          execOpts.NewDispatcher(),
		RunTask:             execOpts.BackgroundTaskLauncher,
		EnableAnchorHealing: Ptr(false),
	}
	err = conductor.Start(d.eventBus)
	if err != nil {
		return errors.UnknownError.WithFormat("start conductor: %v", err)
	}

	exec, err := execute.NewExecutor(execOpts)
	if err != nil {
		return errors.UnknownError.WithFormat("initialize chain executor: %w", err)
	}

	// Create executor bridge for DAG-BFT
	bridge, err := adapter.NewExecutorBridge(exec)
	if err != nil {
		return errors.UnknownError.WithFormat("create executor bridge: %w", err)
	}

	// Start block production loop
	go c.runBlockProducer(inst.context, d, bridge, app)

	// Register API services
	err = c.registerDAGBFTServices(inst, d, app, store, db)
	if err != nil {
		return err
	}

	return nil
}

// runBlockProducer processes committed certificates and produces blocks.
func (c *DAGBFTConsensusService) runBlockProducer(ctx context.Context, d *dagbftState, bridge *adapter.ExecutorBridge, app *CoreConsensusApp) {
	committed := d.node.Committed()
	workers := d.node.Workers()
	pubKey := d.node.PublicKey()

	var blockIndex uint64

	for {
		select {
		case <-ctx.Done():
			return

		case cert, ok := <-committed:
			if !ok {
				return
			}
			if cert == nil {
				continue
			}

			// Collect batches for this certificate
			batches := make(map[types.BatchDigest]*types.Batch)
			digests := make([]types.BatchDigest, 0, len(cert.Header.Payload))
			for digest := range cert.Header.Payload {
				digests = append(digests, digest)
				for _, w := range workers {
					if batch, err := w.GetBatch(digest); err == nil && batch != nil {
						batches[digest] = batch
						break
					}
				}
			}

			// Produce block
			blockIndex++
			isLeader := cert.Header.Author.Equal(pubKey)

			params := adapter.BlockParams{
				Index:       blockIndex,
				Time:        time.Now(),
				IsLeader:    isLeader,
				LeaderRound: cert.Header.Round,
				Certificate: cert,
				Batches:     batches,
			}

			hash, err := bridge.ProduceBlock(ctx, params)
			if err != nil {
				slog.Error("Failed to produce block",
					"error", err,
					"block", blockIndex,
					"round", cert.Header.Round)
				continue
			}

			slog.Debug("Block produced",
				"block", blockIndex,
				"round", cert.Header.Round,
				"hash", fmt.Sprintf("%x", hash[:8]))

			// Prune committed batches
			for _, w := range workers {
				w.PruneBatches(digests)
			}
		}
	}
}

// registerDAGBFTServices registers API services for the DAG-BFT node.
func (c *DAGBFTConsensusService) registerDAGBFTServices(inst *Instance, d *dagbftState, app *CoreConsensusApp, store keyvalue.Beginner, db *database.Database) error {
	// Create a DAG-BFT specific consensus service
	svcImpl := &dagbftConsensusServiceImpl{
		partition:     app.Partition.ID,
		partitionType: app.Partition.Type,
		eventBus:      d.eventBus,
		node:          d.node,
		nodeKeyHash:   sha256.Sum256(d.privKey.Public().(ed25519.PublicKey)),
		validatorKey:  sha256.Sum256(d.privKey.Public().(ed25519.PublicKey)),
	}

	err := consensusProvidesService.Register(inst.services, app, svcImpl)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Register sequencer
	seqImpl := api.NewSequencer(api.SequencerParams{
		Logger:       d.logger.With("module", "api"),
		Database:     database.New(store, d.logger),
		EventBus:     d.eventBus,
		Globals:      <-d.globals,
		Partition:    app.Partition.ID,
		ValidatorKey: d.privKey,
	})
	registerRpcService(inst, seqImpl.Type().AddressFor(app.Partition.ID), message.Sequencer{Sequencer: seqImpl})
	err = coreConsensusProvidesSequencer.Register(inst.services, app, seqImpl)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	slog.Info("DAG-BFT services registered",
		"partition", app.Partition.ID,
		"module", "run",
		"service", "dagbft-consensus")

	return nil
}

// dagbftConsensusServiceImpl implements v3api.ConsensusService for DAG-BFT.
type dagbftConsensusServiceImpl struct {
	partition     string
	partitionType protocol.PartitionType
	eventBus      *events.Bus
	node          *consensus.Node
	nodeKeyHash   [32]byte
	validatorKey  [32]byte
}

// Ensure dagbftConsensusServiceImpl implements the required interface.
var _ v3api.ConsensusService = (*dagbftConsensusServiceImpl)(nil)

func (s *dagbftConsensusServiceImpl) Type() v3api.ServiceType {
	return v3api.ServiceTypeConsensus
}

func (s *dagbftConsensusServiceImpl) ConsensusStatus(ctx context.Context, opts v3api.ConsensusStatusOptions) (*v3api.ConsensusStatus, error) {
	lastBlock := &v3api.LastBlock{
		Height: int64(s.node.LastCommitRound()),
		Time:   time.Now(),
	}

	return &v3api.ConsensusStatus{
		Ok:               true,
		LastBlock:        lastBlock,
		NodeKeyHash:      s.nodeKeyHash,
		ValidatorKeyHash: s.validatorKey,
		PartitionID:      s.partition,
		PartitionType:    s.partitionType,
	}, nil
}
