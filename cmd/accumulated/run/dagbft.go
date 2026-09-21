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
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/libp2p/go-libp2p/core/peer"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/crosschain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	multiexec "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/dagbft"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	dagconfig "gitlab.com/accumulatenetwork/accumulate/pkg/consensus/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/primary"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// DAG-BFT service IOC providers
var (
	dagbftProvidesEventBus  = ioc.Provides[*events.Bus](func(s *DAGBFTService) string { return s.Partition.ID })
	dagbftProvidesService   = ioc.Provides[v3.ConsensusService](func(s *DAGBFTService) string { return s.Partition.ID })
	dagbftProvidesSubmitter = ioc.Provides[v3.Submitter](func(s *DAGBFTService) string { return s.Partition.ID })
	dagbftProvidesValidator = ioc.Provides[v3.Validator](func(s *DAGBFTService) string { return s.Partition.ID })
	dagbftProvidesSequencer = ioc.Provides[private.Sequencer](func(s *DAGBFTService) string { return s.Partition.ID })
	dagbftProvidesRouter    = ioc.Provides[routing.Router](func(s *DAGBFTService) string { return s.Partition.ID })

	// This node's join state, for the services that are configured apart from
	// consensus and must still refuse while it is joining -- the querier
	// (#4297). It is handed over rather than looked up in a registry keyed by
	// partition: a process can run several nodes of one partition, and each
	// has its own.
	dagbftProvidesNodeState = ioc.Provides[nodestate.Serving](func(s *DAGBFTService) string { return s.Partition.ID })

	dagbftNeedsStorage = ioc.Needs[keyvalue.Beginner](func(s *DAGBFTService) string { return s.Partition.ID })

	// The directory's storage, by name rather than by service, so a partition
	// can reach it. Every Accumulate node runs the directory alongside its own
	// BVN, and the proof service needs both halves (#4274).
	dagbftNeedsDnStorage = ioc.Needs[keyvalue.Beginner, string](func(string) string { return protocol.Directory })
)

// Requires returns the IOC requirements for DAG-BFT.
func (s *DAGBFTService) Requires() []ioc.Requirement {
	return []ioc.Requirement{
		dagbftNeedsStorage.Requirement(s),
	}
}

// Provides returns the IOC provisions for DAG-BFT.
func (s *DAGBFTService) Provides() []ioc.Provided {
	return []ioc.Provided{
		dagbftProvidesEventBus.Provided(s),
		dagbftProvidesService.Provided(s),
		dagbftProvidesSubmitter.Provided(s),
		dagbftProvidesValidator.Provided(s),
		dagbftProvidesSequencer.Provided(s),
		dagbftProvidesRouter.Provided(s),
		dagbftProvidesNodeState.Provided(s),
	}
}

// Verify validates the DAG-BFT configuration.
func (s *DAGBFTService) Verify() error {
	if s.Partition == nil {
		return errors.BadRequest.With("partition is required")
	}
	if s.ValidatorKey == nil {
		return errors.BadRequest.With("validator key is required")
	}
	return nil
}

// prestart performs pre-start initialization.
func (s *DAGBFTService) prestart(inst *Instance) error {
	// Nothing to do in prestart for DAG-BFT
	return nil
}

// start initializes and starts the DAG-BFT service.
func (s *DAGBFTService) start(inst *Instance) error {
	// Apply defaults
	setDefaultPtr(&s.EnableDirectDispatch, true)
	setDefaultPtr(&s.MaxEnvelopesPerBlock, uint64(100))
	setDefaultPtr(&s.NumWorkers, dagconfig.DefaultNumWorkers)
	// Execution sharding (#4145) defaults to serial; shard count is a local
	// parallelism choice that cannot change the result, so operators can
	// raise it per node. Bounded: the executor allocates per shard, so a
	// fat-fingered count must be refused at startup, not at block time
	// (#4151).
	setDefaultPtr(&s.ExecutionShards, 1)
	// The count is configuration -- written by `init network` from the
	// network definition's executionShards -- and nothing else. An
	// environment override used to sit here for shard sweeps; it went with
	// #4149's proof work, because a knob the environment can change is a
	// run that can silently differ from its frozen config (spec 1.10).
	if *s.ExecutionShards < 0 || *s.ExecutionShards > 1024 {
		return errors.BadRequest.WithFormat("execution-shards %d is out of range [0, 1024]", *s.ExecutionShards)
	}
	// How much superseded BPT state this node keeps, so it can serve an
	// account and a BPT page AS OF an anchored block (#4361). A peer that
	// keeps none can only serve its own current block, and an account that
	// changes every block is then unverifiable by anybody: no anchor ever
	// covers the root it was served at, which is why a restart converges on
	// nothing.
	//
	// 1024 minor blocks is the default, and the number comes from what the
	// join has to do inside the window rather than from a storage budget. A
	// joining node fixes on the newest block it holds a verified anchor for
	// and must still be able to ask for THAT block when the round finishes,
	// so the window has to cover the anchor lag plus one pull round. The
	// anchor lag is bounded on this line at 64 root-chain positions
	// (anchorSearchWindow, internal/api/v3/proof.go); a round is the block
	// ledger walk over (R, Q] and the accounts it names; blocks are around a
	// second under load, so 1024 blocks is roughly seventeen minutes — an
	// order of magnitude over the lag and comfortably over a round.
	//
	// What it costs: one BPT block-write per state-changing block, plus
	// (dirty accounts per block x depth x 146) bytes of retained state
	// receipts. Measuring that on a real store is #4165's soak, and the
	// number is here so the run has a prediction to check rather than a
	// figure to discover.
	setDefaultPtr(&s.BPTHistoryDepth, uint64(1024))
	slog.Info("BPT history", "depth", *s.BPTHistoryDepth, "serves-anchored-blocks", *s.BPTHistoryDepth > 0, "partition", s.Partition.ID, "module", "dagbft")

	// Said at startup, every time: a run that meant to shard and did not
	// must be visible in the log, not inferred from a metric that stays at
	// zero (REPORTING-SPEC 1).
	slog.Info("Execution shards", "shards", *s.ExecutionShards, "serial", *s.ExecutionShards <= 1, "partition", s.Partition.ID, "module", "dagbft")

	// THIS PARTITION'S STATE IS ON THE WIRE FROM HERE ON, whether or not this
	// node ever joins.
	//
	// The gauge used to be created by the join and only by the join, so a
	// healthy node exported no such series and "absent" could not be told
	// from "booting": a monitor reading absent as 0 painted a healthy fleet
	// as booting, and one reading it as fine could never assert that a node
	// was alive. A twelve-node network ran for twenty minutes with one member
	// executing nothing and every reading said it was healthy (#4345a).
	//
	// BOOTING is the honest value here: nothing has been decided yet. The
	// join overwrites it on every transition, and the branch below sets
	// ACTIVE for a node that executes without asking anyone.
	nodestate.Report(s.Partition.ID, nodestate.StateBooting)
	setDefaultPtr(&s.DAGGCDepth, dagconfig.DefaultDAGGCDepth)
	setDefaultPtr(&s.CommitBufferSize, dagconfig.DefaultCommitBufferSize)
	setDefaultPtr(&s.MaxExecutionLag, int64(primary.DefaultMaxExecutionLag))
	// BlockInterval is deliberately NOT defaulted here. The network declares
	// the cadence and this node paces from it; leaving the pointer nil is how
	// "the operator stated nothing" stays distinguishable from "the operator
	// stated the default", which is what lets a real divergence be refused
	// without refusing every node that simply did not set it (#4267).

	// Get the logger
	logger := logging.NewSlogLogger(inst.logger)

	// Create event bus
	s.eventBus = events.NewBus(logger.With("module", "events"))

	// Subscribe to fatal errors.
	// NOTE: EventBus subscribers are not unsubscribed on shutdown. This is a
	// known limitation of the events.Bus implementation which does not return
	// unsubscribe handles. Since the event bus is created per-service and the
	// service runs for the lifetime of the process, this does not cause a
	// practical memory leak. See issue #3830.
	events.SubscribeAsync(s.eventBus, func(e events.FatalError) {
		slog.ErrorContext(inst.context, "Shutting down due to a fatal error", "error", e.Err)
		inst.shutdown()
	})

	// Create and register the halt controller for this partition.
	//
	// This registration lived in the CometBFT consensus path and was not
	// carried over here, so `POST /admin/halt` was accepted and did nothing:
	// RequestHaltAll iterates registered controllers, and there were none.
	// The unit tests in halt_test.go construct a controller directly and so
	// kept passing while the feature was unwired — which is how it stayed
	// unnoticed (#4097).
	haltController := NewHaltController(
		s.Partition.ID,
		inst.shutdown,
		inst.logger.With("module", "halt", "partition", s.Partition.ID),
	)
	events.SubscribeSync(s.eventBus, haltController.OnDidCommitBlock)
	inst.RegisterHaltController(haltController)

	// Get the storage
	store, err := dagbftNeedsStorage.Get(inst.services, s)
	if err != nil {
		return errors.UnknownError.WithFormat("get storage: %w", err)
	}

	// Get the validator key
	validatorKeyAddr, err := s.ValidatorKey.get(inst)
	if err != nil {
		return errors.UnknownError.WithFormat("get validator key: %w", err)
	}
	validatorKey, ok := validatorKeyAddr.GetPrivateKey()
	if !ok {
		return errors.BadRequest.With("validator key is not a private key")
	}
	if len(validatorKey) != ed25519.PrivateKeySize {
		return errors.BadRequest.WithFormat("validator key has wrong size: %d", len(validatorKey))
	}

	// Create router
	router := routing.NewRouter(routing.RouterOptions{
		Events: s.eventBus,
		Logger: logger,
	})
	err = dagbftProvidesRouter.Register(inst.services, s, router)
	if err != nil {
		return errors.UnknownError.WithFormat("register router: %w", err)
	}

	// Create client for cross-chain
	dialer := inst.p2p.DialNetwork()
	client := &message.Client{Transport: &message.RoutedTransport{
		Network: inst.config.Network,
		Dialer:  dialer,
		Router:  routing.MessageRouter{Router: router},
	}}

	// Create database
	db := database.New(store, logger)

	// Load genesis snapshot if needed (before creating executor)
	genesisPath := inst.path(s.Genesis)
	genesisLoaded, err := s.loadGenesisIfNeeded(db, genesisPath, logger)
	if err != nil {
		return errors.UnknownError.WithFormat("load genesis: %w", err)
	}
	_ = genesisLoaded // May be used in the future for initialization logic

	// Build DAG-BFT configuration. Built before the executor options because
	// the executor's package budget derives from the batching limit (#4141).
	// This one value must ALSO drive the worker's refusal — nodeConfig below
	// sets WorkerConfig.MaxBatchBytes from it — or budget and enforcement
	// are two coincident constants that drift apart silently (#4151).
	dagCfg := dagconfig.DefaultConfig()

	// The producer's synthetic/anchor cache, shared by the executor that
	// fills it and the sequencer that answers healing from it (healing spec,
	// "The cache")
	synthCache := synthcache.New(0)

	// The partition's staging: memory, built up from consensus (executor
	// spec, "Sync"); registered so the API can report how far each stream
	// has been sighted
	staging := execute.NewStaging()
	execute.RegisterStaging(s.Partition.ID, staging)

	// Create executor options
	execOpts := multiexec.Options{
		Logger:     logger.With("module", "executor"),
		Database:   db,
		SynthCache: synthCache,
		Staging:    staging,
		Key:        validatorKey,
		Router:     router,
		EventBus:   s.eventBus,
		Sequencer:  client.Private(),
		Querier:    client,
		// Shard user-transaction execution by identity (#4145).
		ExecutionShards: int(*s.ExecutionShards),
		// A synthetic package must fit in one worker batch (#4141).
		MaxEnvelopeSize: dagCfg.Batching.MaxBatchBytes,
		// Retain superseded BPT state so this node can be served from as of
		// an anchored block (#4361).
		BPTHistoryDepth: *s.BPTHistoryDepth,
		Describe: multiexec.DescribeShim{
			NetworkType: s.Partition.Type,
			PartitionId: s.Partition.ID,
		},
	}

	// Configure dispatcher
	if *s.EnableDirectDispatch {
		execOpts.NewDispatcher = func() multiexec.Dispatcher {
			return accumulated.NewDispatcher(inst.config.Network, router, dialer)
		}
	} else {
		execOpts.NewDispatcher = func() multiexec.Dispatcher {
			return accumulated.NewDispatcher(inst.config.Network, router, dialer)
		}
	}

	// Setup globals channel for passing global values to API services.
	// NOTE: This subscriber is not unsubscribed on shutdown. See note above
	// about EventBus subscriber lifecycle (issue #3830).
	globalsChan := make(chan *network.GlobalValues, 1)
	events.SubscribeSync(s.eventBus, func(e events.WillChangeGlobals) error {
		select {
		case globalsChan <- e.New:
		default:
		}
		return nil
	})

	// One HealCounters instance, shared by the conductor (which increments it)
	// and the consensus API service (which reports it) — recoveries become
	// visible to the soak monitor instead of only to grep (#4075, #4105).
	healCounters := new(crosschain.HealCounters)

	// Start conductor for cross-chain communication
	conductor := &crosschain.Conductor{
		Partition:    s.Partition,
		ValidatorKey: execOpts.Key,
		Database:     execOpts.Database,
		Querier:      v3.Querier2{Querier: client},
		Dispatcher:   execOpts.NewDispatcher(),
		Sequencer:    client.Private(),
		Peers:        client,
		Staging:      staging,
		Heals:        healCounters,
		RunTask:      execOpts.BackgroundTaskLauncher,
		// Healing is the ONLY retry mechanism for anchors — the conductor's
		// per-block dispatch is one-shot, and a single lost anchor freezes
		// the destination's delivered-sequence forever (observed as BVN
		// ledgers stuck at height 2, #4054). The conductor paces healing
		// scans internally (HealInterval), so this is safe even at DAG-BFT
		// block rates.
	}
	err = conductor.Start(s.eventBus)
	if err != nil {
		return errors.UnknownError.WithFormat("start conductor: %w", err)
	}

	// Create executor
	exec, err := multiexec.NewExecutor(execOpts)
	if err != nil {
		return errors.UnknownError.WithFormat("create executor: %w", err)
	}

	// Create executor adapter
	executorBridge, err := adapter.NewExecutorBridge(adapter.ExecutorBridgeConfig{
		Executor:    exec,
		PartitionID: s.Partition.ID,
		EventBus:    s.eventBus,
	})
	if err != nil {
		return errors.UnknownError.WithFormat("create executor bridge: %w", err)
	}

	// Create the DAG-BFT node configuration
	nodeConfig := consensus.NodeConfig{
		Partition:        s.Partition.ID,
		KeyPair:          validatorKey,
		NumWorkers:       int(*s.NumWorkers),
		DAGGCDepth:       types.Round(*s.DAGGCDepth),
		CommitBufferSize: int(*s.CommitBufferSize),
		MaxExecutionLag:  int(*s.MaxExecutionLag),

		// The same limit the executor's package budget derives from
		// (#4151) — never let the two diverge.
		WorkerConfig: worker.Config{
			MaxBatchBytes: dagCfg.Batching.MaxBatchBytes,
		},

		// MinRoundInterval is set below, once the network's block interval is
		// known. Rounds pace at half of it: Bullshark commits a leader every
		// other round, so blocks arrive at roughly 2x the round interval.
		// Before this was wired, primary fell back to its 100ms default and
		// the Directory ran at ~21 blocks/sec under load — and since every
		// block emits an anchor, anchor traffic ran at block rate and drowned
		// one-shot dispatch (#4098).
	}

	// Use the shared GossipSub for DAG-BFT certificate/batch dissemination.
	// The GossipSub is created once per host in Instance.StartFiltered() and
	// shared across all partitions. Topics separate messages by partition.
	ps := inst.pubsub
	if ps != nil {
		slog.Info("Using shared GossipSub for DAG-BFT networking", "partition", s.Partition.ID)
	}

	// Wait for globals to be available before creating the service.
	// The WillChangeGlobals event fires during executor creation (in loadGlobals),
	// which populates globalsChan. We need these to get the initial validators.
	var globals *network.GlobalValues
	select {
	case globals = <-globalsChan:
		slog.Info("Received initial globals for DAG-BFT", "partition", s.Partition.ID)
	case <-time.After(5 * time.Second):
		slog.Warn("Timeout waiting for initial globals, DAG-BFT may not reach quorum", "partition", s.Partition.ID)
		globals = new(network.GlobalValues)
	}

	// The network declares the cadence; this node either paces from it or does
	// not run (#4267).
	blockInterval, err := resolveBlockInterval(s.BlockInterval, globals, s.Partition.ID)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	nodeConfig.MinRoundInterval = blockInterval / 2
	slog.Info("Block interval", "partition", s.Partition.ID, "interval", blockInterval,
		"minRoundInterval", nodeConfig.MinRoundInterval)

	// Seed the conductor's globals directly. The conductor subscribes to
	// WillChangeGlobals, but whether it observes the INITIAL event is a
	// startup ordering race — a conductor that misses it returns early from
	// every willBeginBlock and never sends or heals a single anchor, which
	// is exactly the same race the InitialValidators handling above works
	// around for the adapter (#4056).
	conductor.Globals.Store(globals)

	// Extract initial validators from globals
	var initialValidators []adapter.ValidatorInfo
	if globals != nil && globals.Network != nil {
		for _, v := range globals.Network.Validators {
			if !v.IsActiveOn(s.Partition.ID) {
				continue
			}
			if len(v.PublicKey) != 32 {
				continue
			}
			var pubKey [32]byte
			copy(pubKey[:], v.PublicKey)
			initialValidators = append(initialValidators, adapter.ValidatorInfo{
				PublicKey: pubKey,
				Stake:     1,
				Active:    true,
			})
		}
		slog.Info("Extracted initial validators for DAG-BFT",
			"partition", s.Partition.ID,
			"validators", len(initialValidators))
	}

	// Create the service
	svcConfig := dagbft.ServiceConfig{
		Partition:         s.Partition,
		NodeConfig:        nodeConfig,
		Adapter:           executorBridge,
		EventBus:          s.eventBus,
		Logger:            logger.With("module", "dagbft"),
		Genesis:           inst.path(s.Genesis),
		DataDir:           inst.path("consensus", strings.ToLower(s.Partition.ID)),
		InitialValidators: initialValidators,
	}
	if globals != nil && globals.Network != nil {
		// The committee epoch is the network definition version (state-derived)
		svcConfig.InitialNetworkVersion = globals.Network.Version
	}

	// Wire in libp2p networking if available
	if inst.p2p != nil && ps != nil {
		svcConfig.Host = inst.p2p.Host()
		svcConfig.PubSub = ps
	}

	s.service, err = dagbft.NewService(svcConfig)
	if err != nil {
		return errors.UnknownError.WithFormat("create DAG-BFT service: %w", err)
	}

	// A node that has executed a block before does not execute the blocks it
	// missed: it joins, from the block its state matches (executor spec,
	// "Sync"; #4294). Collecting starts BEFORE consensus does, so every block
	// committed from here is one this node has kept — which is what makes the
	// staging it takes from a peer exact.
	//
	// A node with nothing — genesis, or a fresh database — has nothing to join
	// from and executes from its first block as it always has.
	//
	// GENESIS IS NOT AN EXECUTION. Loading the genesis snapshot writes the
	// system ledger at protocol.GenesisBlock — `ledger.Index =
	// protocol.GenesisBlock`, internal/node/genesis/bootstrap.go — and every
	// node loads it before this point (loadGenesisIfNeeded, above). So
	// `lastBlock > 0` is true of every node that has ever started, including
	// the first node of a new network, and it sent all twelve nodes of a
	// fresh network into the join to ask each other for a staging none of
	// them had (#4304). What says a node must join is whether it has
	// executed a block OF ITS OWN, and that is `lastBlock > GenesisBlock`.
	err = s.noteExecutedBlock(db)
	if err != nil {
		return errors.UnknownError.WithFormat("read this node's last block: %w", err)
	}
	lastBlock := s.lastExecuted
	// This node's own height, on the wire, before a block of this run is
	// executed: a node that is joining and executes nothing must still say
	// where it stands, and a monitor must not have to infer it from an
	// absent series (#4345b). The executor moves it from here on.
	nodestate.ReportExecuted(s.Partition.ID, lastBlock)

	joining := nodeMustJoin(lastBlock)
	if !joining {
		// Said out loud, because it is the one condition under which a node
		// executes without asking anyone (executor spec, "Sync", step 2), and
		// because it is wrong for a node being added to a network that is
		// already running — which this node cannot tell apart from being the
		// first node of a new one (#4340).
		slog.Info("This node has executed no block beyond genesis: it is the first node of a network, "+
			"so it executes from genesis without asking for staging",
			"module", "join", "partition", s.Partition.ID, "block", lastBlock)

		// It executes from genesis without asking anyone, so it is ACTIVE:
		// it answers for the state it holds, and nothing it holds came from
		// a peer. Reported rather than left at BOOTING, because a node that
		// never joins never enters the state machine and BOOTING for the
		// life of the process is the negative-only reading of #4345a.
		nodestate.Report(s.Partition.ID, nodestate.StateActive)
	}

	// The join's state is built first, because its node state is what the
	// API services refuse by (#4295) and they are registered further down.
	// It is this node's, handed over rather than looked up: a process can run
	// several nodes of one partition — devnet does — and a registry keyed by
	// partition would give them all one node's state.
	var joinState *join.PulledState
	if joining {
		// The pull is addressed at NAMED PEERS, never at this node. The
		// client above is routed, and a routed client answers locally for any
		// service this node provides -- p2p.DialNetwork installs a
		// self-discoverer unconditionally -- so a join given it reads the
		// un-executed store it exists to fill, and every account is refused
		// (#4303). QueryPeers looks the partition's query service up, drops
		// this node's own peer ID, and addresses one peer at a time.
		var self peer.ID
		if inst.p2p != nil {
			self = inst.p2p.ID()
		}
		joinState, err = join.NewState(join.StateOptions{
			Partition: protocol.PartitionUrl(s.Partition.ID),
			Database:  db,
			Sources: &join.QueryPeers{
				Client:  client,
				Network: inst.config.Network,
				Router:  router,
				Self:    self,
			},
			// The block this node's EXECUTOR last executed, handed over
			// rather than read again later. The pull overwrites
			// `<partition>/ledger` with the peer's, so the store stops being
			// able to answer this question the moment the join starts
			// (#4295).
			ExecutedBlock: lastBlock,
			// The definition this join pulls is published here at the
			// handoff, before the first block executes (#4366's M3).
			EventBus: s.eventBus,
			Logger:   slog.Default(),
		})
		if err != nil {
			return errors.UnknownError.WithFormat("prepare the join: %w", err)
		}
		// The block this node's own executor last executed, handed in once:
		// the first group collected is the block after it, and every group
		// after that is the next block. StartCollecting runs before Start(),
		// and Start() is what used to set the number this derived itself
		// from, so it derived zero (#4351).
		s.service.StartCollecting(lastBlock)
	}

	// Start the service
	err = s.service.Start(inst.context)
	if err != nil {
		return errors.UnknownError.WithFormat("start DAG-BFT service: %w", err)
	}

	if joining {
		stage, ok := exec.(join.Stage)
		if !ok {
			return errors.InternalError.With("this executor cannot join: it cannot settle its staging at a block")
		}
		opts := join.Options{
			Partition: s.Partition.ID,
			Buffer:    s.service,
			Stage:     stage,
			State:     joinState,
			Logger:    slog.Default(),
		}
		go func() {
			err := join.Run(inst.context, opts)
			if err != nil {
				// A join that cannot finish leaves the node collecting: it
				// keeps up with consensus and executes nothing, which is the
				// spec's answer and is safe. It is also an operator's
				// problem, so it is an error and not a debug line.
				slog.Error("The join did not complete; this node is not executing",
					"module", "join", "partition", s.Partition.ID, "error", err)
			}
		}()
	}

	// The healing in-flight window is measured against the send, and the
	// send is the leader's. A node can only see its own executor's lag, so
	// that is what the window widens by (#4248). The consensus node is
	// built inside Start, so this has to come after it; an unwired source
	// would leave the window at InFlightBlocks and heal every late
	// dispatch, silently, so a missing node is a failure and not a default.
	node := s.service.Node()
	if node == nil {
		return errors.InternalError.WithFormat("DAG-BFT service started without a consensus node")
	}
	synthCache.SetExecutionLagSource(node.ExecutionLag)
	conductor.SetExecutionLagSource(node.ExecutionLag, node.MaxExecutionLag())

	// Register cleanup
	inst.cleanup("dagbft service", func(ctx context.Context) error {
		return s.service.Stop()
	})

	// Register event bus
	err = dagbftProvidesEventBus.Register(inst.services, s, s.eventBus)
	if err != nil {
		return errors.UnknownError.WithFormat("register event bus: %w", err)
	}

	// Register consensus API services
	var nodeState *nodestate.Machine
	if joinState != nil {
		nodeState = joinState.Machine()
	}

	// Registered whether or not this node joined: a node that never joined
	// answers for itself, and a service that wants the state must get an
	// answer rather than an absence.
	var serving nodestate.Serving = nodestate.Always{}
	if nodeState != nil {
		serving = nodeState
	}
	err = dagbftProvidesNodeState.Register(inst.services, s, serving)
	if err != nil {
		return errors.UnknownError.WithFormat("register node state: %w", err)
	}
	err = s.registerAPIServices(inst, store, validatorKey, globals, healCounters, synthCache, staging, nodeState, serving)
	if err != nil {
		return err
	}

	inst.logger.Info(color.HiBlueString("Running DAG-BFT"), "partition", s.Partition.ID, "module", "run", "service", "dagbft")
	return nil
}

// registerAPIServices registers the API services for DAG-BFT.
func (s *DAGBFTService) registerAPIServices(inst *Instance, store keyvalue.Beginner, validatorKey []byte, globals *network.GlobalValues, healCounters *crosschain.HealCounters, synthCache *synthcache.Cache, staging *execute.Staging, nodeState *nodestate.Machine, serving nodestate.Serving) error {
	logger := logging.NewSlogLogger(inst.logger)
	// These are the SERVING side of the node: consensus queries, the
	// sequencer answering a peer's healing request, the API.  They are
	// asked for whatever a caller names, which is routinely older than
	// the window the store answers protocol reads from (BlockchainDB
	// spec 1.3), so they read deep.  The executor keeps the windowed
	// database it is given above, where the cost of a miss is bounded.
	db := database.New(store, logger).Deep()

	// Create consensus service
	consensusSvc, err := newConsensusAPIService(dagbft.ConsensusAPIServiceParams{
		Heals:            healCounters,
		Logger:           logger.With("module", "api"),
		Service:          s.service,
		Database:         db,
		PartitionID:      s.Partition.ID,
		PartitionType:    s.Partition.Type,
		EventBus:         s.eventBus,
		NodeKeyHash:      sha256.Sum256(validatorKey[32:]), // Public key portion
		ValidatorKeyHash: sha256.Sum256(validatorKey[32:]),
		// Reported as CatchingUp, which is what tells another node's relay
		// that this one cannot propose yet (#4366).
		NodeState: serving,
		// Answers a relay's challenge, so that naming a validator's key
		// hash is not enough to be handed its traffic (#4366 F1).
		ValidatorKey: ed25519.PrivateKey(validatorKey),
	})
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	registerRpcService(inst, consensusSvc.Type().AddressFor(s.Partition.ID), message.ConsensusService{ConsensusService: consensusSvc})
	err = dagbftProvidesService.Register(inst.services, s, consensusSvc)
	if err != nil {
		return errors.UnknownError.WithFormat("register consensus service: %w", err)
	}

	// Create submitter service, with the committee predicate and the relay
	// it hands on what it cannot propose with (#4366).
	submitterSvc, err := newSubmitterService(submitterParams{
		Logger:       logger.With("module", "api"),
		Partition:    s.Partition.ID,
		AuthorKey:    validatorKey[32:],
		ValidatorKey: ed25519.PrivateKey(validatorKey),
		Globals:      globals,
		EventBus:     s.eventBus,
		Service:      s.service,
		NodeState:    serving,
		Node:         inst.p2p,
		Network:      inst.config.Network,
	})
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	registerRpcService(inst, submitterSvc.Type().AddressFor(s.Partition.ID), message.Submitter{Submitter: submitterSvc})
	err = dagbftProvidesSubmitter.Register(inst.services, s, submitterSvc)
	if err != nil {
		return errors.UnknownError.WithFormat("register submitter service: %w", err)
	}

	// Create validator service
	validatorSvc := dagbft.NewValidatorService(dagbft.ValidatorServiceParams{
		Logger:    logger.With("module", "api"),
		Service:   s.service,
		NodeState: nodeState,
	})
	registerRpcService(inst, validatorSvc.Type().AddressFor(s.Partition.ID), message.Validator{Validator: validatorSvc})
	err = dagbftProvidesValidator.Register(inst.services, s, validatorSvc)
	if err != nil {
		return errors.UnknownError.WithFormat("register validator service: %w", err)
	}

	// Create sequencer service
	sequencerSvc := api.NewSequencer(api.SequencerParams{
		Logger:       logger.With("module", "api"),
		Database:     db,
		Cache:        synthCache,
		Staging:      staging,
		NodeState:    nodeState,
		EventBus:     s.eventBus,
		Globals:      globals,
		Partition:    s.Partition.ID,
		ValidatorKey: validatorKey,
	})
	registerRpcService(inst, sequencerSvc.Type().AddressFor(s.Partition.ID), message.Sequencer{Sequencer: sequencerSvc})
	err = dagbftProvidesSequencer.Register(inst.services, s, sequencerSvc)
	if err != nil {
		return errors.UnknownError.WithFormat("register sequencer service: %w", err)
	}

	// Create the proof service. It is the public face of what the sequencer
	// already serves node-to-node, plus the second of the two calls an account
	// proof takes (#4272). Only the directory can answer that one -- a
	// partition's BPT root is bound to a directory root, and the binding lives
	// in the directory's anchor(P)-bpt chain -- but registering it everywhere
	// keeps the address uniform and lets the service itself say so.
	proofSvc := &api.ProofService{
		Ranger:    sequencerSvc,
		Database:  db,
		Partition: config.NetworkUrl{URL: protocol.PartitionUrl(s.Partition.ID)},
		Directory: newDirectoryResolver(s.Partition.ID, db, func() (database.Viewer, error) {
			store, err := dagbftNeedsDnStorage.Get(inst.services, "")
			if err != nil {
				return nil, err
			}
			return database.New(store, logger).Deep(), nil
		}),
	}
	registerRpcService(inst, proofSvc.Type().AddressFor(s.Partition.ID), message.ProofService{ProofService: proofSvc})

	return nil
}

// loadGenesisIfNeeded loads the genesis snapshot into the database if needed.
// It returns true if genesis was loaded, false if the database already has data.
func (s *DAGBFTService) loadGenesisIfNeeded(db *database.Database, genesisPath string, logger logging.Logger) (bool, error) {
	// Do NOT override the database observer. The default (production)
	// observer computes real account hashes; execute.NewDatabaseObserver is a
	// stub whose hasher is nil, so with it every account hash is empty — the
	// BPT stops committing to state and genesis restore fails its hash check
	// against the snapshot (#4053).

	// Check if database already has state
	batch := db.Begin(false)
	ledger := batch.Account(protocol.PartitionUrl(s.Partition.ID).JoinPath(protocol.Ledger))
	_, err := ledger.Main().Get()
	batch.Discard()

	if err == nil {
		// Database already initialized
		return false, nil
	}

	// Check if it's a "not found" error (expected for empty database)
	if !errors.Is(err, errors.NotFound) {
		return false, errors.UnknownError.WithFormat("check ledger: %w", err)
	}

	// Database is empty, need to load genesis
	if genesisPath == "" {
		return false, errors.BadRequest.With("genesis path is required for empty database")
	}

	// Check if genesis file exists
	if _, err := os.Stat(genesisPath); err != nil {
		if os.IsNotExist(err) {
			return false, errors.BadRequest.WithFormat("genesis file not found: %s", genesisPath)
		}
		return false, errors.UnknownError.WithFormat("check genesis file: %w", err)
	}

	// Read the genesis snapshot
	data, err := os.ReadFile(genesisPath)
	if err != nil {
		return false, errors.UnknownError.WithFormat("read genesis file: %w", err)
	}

	slog.Info("Loading genesis snapshot", "path", genesisPath, "partition", s.Partition.ID)

	// Restore the snapshot into the database
	network := config.NetworkUrl{URL: protocol.PartitionUrl(s.Partition.ID)}
	err = snapshot.FullRestore(db, ioutil.NewBuffer(data), logger, network)
	if err != nil {
		return false, errors.UnknownError.WithFormat("restore genesis snapshot: %w", err)
	}

	slog.Info("Genesis snapshot loaded successfully", "partition", s.Partition.ID)
	return true, nil
}

// Ensure DAGBFTService implements the required interfaces
var (
	_ Service    = (*DAGBFTService)(nil)
	_ prestarter = (*DAGBFTService)(nil)
)

// nodeMustJoin reports whether this node must take a running validator's
// staging before it executes anything (executor spec, "Sync").
//
// The block it is given is the system ledger's index, and the genesis
// snapshot writes that ledger at protocol.GenesisBlock before any block is
// executed. So "this node has executed nothing" is `lastBlock <=
// GenesisBlock`, not `lastBlock == 0`, and the difference is the whole of
// #4304: a fresh network's nodes all read 1, all joined, and all refused each
// other the staging none of them had.
//
// The predicate is a function so that the daemon and the test that gates it
// compute the same thing. A test that names the number itself proves nothing
// about what the daemon does.
func nodeMustJoin(lastBlock uint64) bool {
	return lastBlock > protocol.GenesisBlock
}

// noteExecutedBlock reads the block this node's own executor last executed
// and remembers it, ONCE, before anything is pulled.
//
// Once, because the store's copy stops being this node's the moment the join
// starts: a joining node executes nothing, so the number cannot change under
// it, while the pull writes a peer's state into this store from the first
// round. Everything that decides what this node does with its own height
// reads the remembered number — `nodeMustJoin`, the metric, and the block the
// buffer's first collected group is numbered from (#4344, #4351).
func (s *DAGBFTService) noteExecutedBlock(db database.Beginner) error {
	n, err := lastExecutedBlock(db, s.Partition.ID)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	s.lastExecuted = n
	return nil
}

// lastExecutedBlock is the block this node's state is, or zero when it has
// executed none: what says whether a node is starting from genesis or coming
// back to a network that has moved on (executor spec, "Sync").
//
// IT IS READ FROM A RECORD NO PULL WRITES.
//
// It used to read `<partition>/ledger`, and that is an ACCOUNT — one of the
// accounts the join's pull fetches from a peer and settles into this store.
// The live log of acc-bvn1-val1 names them in the held set:
//
//	Pulled accounts were given up on unanchored: ... first=acc://bvn-BVN1.acme/ledger
//	Pulled accounts were given up on unanchored: ... first=acc://dn.acme/ledger
//
// So what this read at start-up was whatever the previous process's pull left
// behind. Today it decides `nodeMustJoin`, where a wrong value is harmless,
// but it is also what the NoPeerHasStaging branch starts executing at — so a
// half-finished pull could start this node executing at a peer's block over a
// store that was only partly filled (#4344).
//
// The executor writes SystemData(partition).ExecutedBlock with every block it
// commits (block_end.go). SystemData is not an account, so no pull reaches it.
//
// THE FALLBACK. A store written before that record existed does not have it,
// and reading zero there would tell a node that has been running for a week
// that it is the first node of a new network — which executes from genesis
// without asking anyone. So an absent record falls back to the ledger, ONCE,
// and the number is written into the record before anything else runs. From
// that moment the pull cannot move it. The one start that reads the ledger is
// the first start after the upgrade, and it is no worse off than every start
// before it was.
func lastExecutedBlock(db database.Beginner, partition string) (uint64, error) {
	batch := db.Begin(false)
	n, err := batch.SystemData(partition).ExecutedBlock().Get()
	switch {
	case err == nil && n > 0:
		batch.Discard()
		return n, nil
	case err != nil && !errors.Is(err, errors.NotFound):
		batch.Discard()
		return 0, errors.UnknownError.WithFormat("read this node's executed block: %w", err)
	}

	// No record: read the ledger once, and seed the record from it.
	var ledger *protocol.SystemLedger
	err = batch.Account(protocol.PartitionUrl(partition).JoinPath(protocol.Ledger)).Main().GetAs(&ledger)
	batch.Discard()
	switch {
	case errors.Is(err, errors.NotFound):
		// No ledger at all: this node has executed nothing, so it is starting
		// from genesis rather than coming back to a network. Nothing to seed.
		return 0, nil
	case err != nil:
		// Anything else is a store this node cannot read. Treating it as
		// "no ledger" would start a node executing from a checkpoint against
		// state it could not read — silently.
		return 0, errors.UnknownError.Wrap(err)
	}

	slog.Info("This node has no record of its own executed block; seeding it from the ledger this once",
		"module", "join", "partition", partition, "block", ledger.Index)
	write := db.Begin(true)
	defer write.Discard()
	err = write.SystemData(partition).ExecutedBlock().Put(ledger.Index)
	if err != nil {
		return 0, errors.UnknownError.WithFormat("seed this node's executed block: %w", err)
	}
	err = write.Commit()
	if err != nil {
		return 0, errors.UnknownError.WithFormat("seed this node's executed block: %w", err)
	}
	return ledger.Index, nil
}
