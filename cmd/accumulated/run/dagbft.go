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
	// Said at startup, every time: a run that meant to shard and did not
	// must be visible in the log, not inferred from a metric that stays at
	// zero (REPORTING-SPEC 1).
	slog.Info("Execution shards", "shards", *s.ExecutionShards, "serial", *s.ExecutionShards <= 1, "partition", s.Partition.ID, "module", "dagbft")
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
	lastBlock, err := lastExecutedBlock(db, s.Partition.ID)
	if err != nil {
		return errors.UnknownError.WithFormat("read this node's last block: %w", err)
	}
	joining := lastBlock > 0

	// The join's state is built first, because its node state is what the
	// API services refuse by (#4295) and they are registered further down.
	// It is this node's, handed over rather than looked up: a process can run
	// several nodes of one partition — devnet does — and a registry keyed by
	// partition would give them all one node's state.
	var joinState *join.PulledState
	if joining {
		joinState, err = join.NewState(join.StateOptions{
			Partition: protocol.PartitionUrl(s.Partition.ID),
			Database:  db,
			Query:     client,
			Logger:    slog.Default(),
		})
		if err != nil {
			return errors.UnknownError.WithFormat("prepare the join: %w", err)
		}
		s.service.StartCollecting()
	}

	// Start the service
	err = s.service.Start(inst.context)
	if err != nil {
		return errors.UnknownError.WithFormat("start DAG-BFT service: %w", err)
	}

	if joining {
		stage, ok := exec.(join.Stage)
		if !ok {
			return errors.InternalError.With("this executor cannot join: it takes no staging from a peer")
		}
		state := joinState
		opts := join.Options{
			Partition: s.Partition.ID,
			Buffer:    s.service,
			Stage:     stage,
			State:     state,
			Peers:     &join.APIPeers{Partition: s.Partition.ID, Client: client, Network: inst.config.Network},
			Fresh:     lastBlock == 0,
			Logger:    slog.Default(),
		}
		go func() {
			outcome, err := join.Run(inst.context, opts)
			switch {
			case err != nil:
				// A join that cannot finish leaves the node collecting: it
				// keeps up with consensus and executes nothing, which is the
				// spec's answer and is safe. It is also an operator's
				// problem, so it is an error and not a debug line.
				slog.Error("The join did not complete; this node is not executing",
					"module", "join", "partition", s.Partition.ID, "error", err)

			case outcome == join.NoPeerHasStaging:
				// No validator of this partition has staging to give: they
				// all restarted too, and an empty stage is what every one of
				// them holds. There is nothing to take and nothing to be
				// exact about, so this node executes from where it stands —
				// the blocks it buffered while it was asking, in order, from
				// its own last block on.
				slog.Info("No peer had staging to give; executing from this node's own state",
					"module", "join", "partition", s.Partition.ID, "block", lastBlock)

				// Its state is recorded as executing BEFORE it is, because
				// the root recorded must be the root of the block named and
				// one produced block changes it. A node that could not
				// record it would refuse every request for the rest of its
				// life (#4295), so that is a failure and not a log line.
				err := state.Executing(lastBlock)
				if err != nil {
					slog.Error("This node cannot record that it is executing; it will refuse requests",
						"module", "join", "partition", s.Partition.ID, "block", lastBlock, "error", err)
					return
				}
				err = s.service.Handoff(lastBlock)
				if err != nil {
					slog.Error("This node could not start executing", "module", "join",
						"partition", s.Partition.ID, "block", lastBlock, "error", err)
				}

			default:
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
	err = s.registerAPIServices(inst, store, validatorKey, globals, healCounters, synthCache, staging, nodeState)
	if err != nil {
		return err
	}

	inst.logger.Info(color.HiBlueString("Running DAG-BFT"), "partition", s.Partition.ID, "module", "run", "service", "dagbft")
	return nil
}

// registerAPIServices registers the API services for DAG-BFT.
func (s *DAGBFTService) registerAPIServices(inst *Instance, store keyvalue.Beginner, validatorKey []byte, globals *network.GlobalValues, healCounters *crosschain.HealCounters, synthCache *synthcache.Cache, staging *execute.Staging, nodeState *nodestate.Machine) error {
	logger := logging.NewSlogLogger(inst.logger)
	// These are the SERVING side of the node: consensus queries, the
	// sequencer answering a peer's healing request, the API.  They are
	// asked for whatever a caller names, which is routinely older than
	// the window the store answers protocol reads from (BlockchainDB
	// spec 1.3), so they read deep.  The executor keeps the windowed
	// database it is given above, where the cost of a miss is bounded.
	db := database.New(store, logger).Deep()

	// Create consensus service
	consensusSvc := dagbft.NewConsensusAPIService(dagbft.ConsensusAPIServiceParams{
		Heals:            healCounters,
		Logger:           logger.With("module", "api"),
		Service:          s.service,
		Database:         db,
		PartitionID:      s.Partition.ID,
		PartitionType:    s.Partition.Type,
		EventBus:         s.eventBus,
		NodeKeyHash:      sha256.Sum256(validatorKey[32:]), // Public key portion
		ValidatorKeyHash: sha256.Sum256(validatorKey[32:]),
	})
	registerRpcService(inst, consensusSvc.Type().AddressFor(s.Partition.ID), message.ConsensusService{ConsensusService: consensusSvc})
	err := dagbftProvidesService.Register(inst.services, s, consensusSvc)
	if err != nil {
		return errors.UnknownError.WithFormat("register consensus service: %w", err)
	}

	// Create submitter service
	submitterSvc := dagbft.NewSubmitterService(dagbft.SubmitterServiceParams{
		Logger:  logger.With("module", "api"),
		Service: s.service,
	})
	registerRpcService(inst, submitterSvc.Type().AddressFor(s.Partition.ID), message.Submitter{Submitter: submitterSvc})
	err = dagbftProvidesSubmitter.Register(inst.services, s, submitterSvc)
	if err != nil {
		return errors.UnknownError.WithFormat("register submitter service: %w", err)
	}

	// Create validator service
	validatorSvc := dagbft.NewValidatorService(dagbft.ValidatorServiceParams{
		Logger:  logger.With("module", "api"),
		Service: s.service,
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

// lastExecutedBlock is the block this node's state is, or zero when it has
// executed none: what says whether a node is starting from genesis or coming
// back to a network that has moved on (executor spec, "Sync").
func lastExecutedBlock(db *database.Database, partition string) (uint64, error) {
	batch := db.Begin(false)
	defer batch.Discard()
	var ledger *protocol.SystemLedger
	switch err := batch.Account(protocol.PartitionUrl(partition).JoinPath(protocol.Ledger)).Main().GetAs(&ledger); {
	case err == nil:
		return ledger.Index, nil
	case errors.Is(err, errors.NotFound):
		// No ledger at all: this node has executed nothing, so it is starting
		// from genesis rather than coming back to a network.
		return 0, nil
	default:
		// Anything else is a store this node cannot read. Treating it as
		// "no ledger" would start a node executing from a checkpoint against
		// state it could not read — silently.
		return 0, errors.UnknownError.Wrap(err)
	}
}
