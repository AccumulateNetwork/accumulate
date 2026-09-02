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
	"strconv"
	"time"

	"github.com/fatih/color"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/crosschain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	multiexec "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/dagbft"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	dagconfig "gitlab.com/accumulatenetwork/accumulate/pkg/consensus/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
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
	// ACC_EXECUTION_SHARDS overrides the configured count, so a shard sweep
	// does not need a regenerated config (and therefore a new genesis) per
	// data point. Same idiom as ACC_LEVELDB_CACHE_MB. An invalid value is
	// REFUSED, not ignored: silently falling back to the configured count
	// would make a sweep report the serial number under a parallel label,
	// which is the one way this measurement can lie.
	// An EMPTY value means "not set", not "invalid". Compose renders an unset
	// variable as the empty string ("${ACC_EXECUTION_SHARDS-}"), so refusing
	// it would refuse to start every node on every network that never set it
	// — the default case. A non-empty value that is not a number is still a
	// hard error.
	if v, ok := os.LookupEnv("ACC_EXECUTION_SHARDS"); ok && v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return errors.BadRequest.WithFormat("ACC_EXECUTION_SHARDS %q is not a number", v)
		}
		s.ExecutionShards = Ptr(int64(n))
		slog.Info("Execution shards overridden", "shards", n, "module", "dagbft")
	}
	if *s.ExecutionShards < 0 || *s.ExecutionShards > 1024 {
		return errors.BadRequest.WithFormat("execution-shards %d is out of range [0, 1024]", *s.ExecutionShards)
	}
	setDefaultPtr(&s.DAGGCDepth, dagconfig.DefaultDAGGCDepth)
	setDefaultPtr(&s.CommitBufferSize, dagconfig.DefaultCommitBufferSize)
	setDefaultPtr(&s.BlockInterval, encoding.Duration(dagconfig.DefaultBlockInterval))

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

	// The executor's staging store, created here because the conductor needs
	// the SAME instance: staging is what the node holds, and healing asking a
	// different view of that is the disagreement #4189 removes.
	staging := multiexec.NewStaging()

	// Create executor options
	execOpts := multiexec.Options{
		Staging:   staging,
		Logger:    logger.With("module", "executor"),
		Database:  db,
		Key:       validatorKey,
		Router:    router,
		EventBus:  s.eventBus,
		Sequencer: client.Private(),
		Querier:   client,
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
		Staging:      staging,
		Heals:        healCounters,
		RunTask:      execOpts.BackgroundTaskLauncher,
		// Healing is the ONLY retry mechanism for anchors — the conductor's
		// per-block dispatch is one-shot, and a single lost anchor freezes
		// the destination's delivered-sequence forever (observed as BVN
		// ledgers stuck at height 2, #4054). The conductor paces healing
		// scans internally (HealInterval), so this is safe even at DAG-BFT
		// block rates.
		EnableAnchorHealing: Ptr(true),
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

		// The same limit the executor's package budget derives from
		// (#4151) — never let the two diverge.
		WorkerConfig: worker.Config{
			MaxBatchBytes: dagCfg.Batching.MaxBatchBytes,
		},

		// Rounds pace at half the block interval: Bullshark commits a leader
		// every other round, so blocks arrive at roughly 2x the round
		// interval. Before this was wired, primary fell back to its 100ms
		// default and the Directory ran at ~21 blocks/sec under load — and
		// since every block emits an anchor, anchor traffic ran at block
		// rate and drowned one-shot dispatch (#4098).
		MinRoundInterval: time.Duration(*s.BlockInterval) / 2,
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

	// Apply a fast-sync rejoin seed if one was written by `accumulated
	// fastsync` (#4058) — consumed once, on the first start after the sync
	rejoin, err := dagbft.LoadRejoinSeed(inst.path(), s.Partition.ID)
	if err != nil {
		slog.Error("Failed to load fast-sync rejoin seed — starting without it", "partition", s.Partition.ID, "error", err)
	}

	// Create the service
	svcConfig := dagbft.ServiceConfig{
		Partition:         s.Partition,
		NodeConfig:        nodeConfig,
		Adapter:           executorBridge,
		EventBus:          s.eventBus,
		Logger:            logger.With("module", "dagbft"),
		Genesis:           inst.path(s.Genesis),
		InitialValidators: initialValidators,
		Rejoin:            rejoin,
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

	// Start the service
	err = s.service.Start(inst.context)
	if err != nil {
		return errors.UnknownError.WithFormat("start DAG-BFT service: %w", err)
	}

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
	err = s.registerAPIServices(inst, store, validatorKey, globals, healCounters)
	if err != nil {
		return err
	}

	inst.logger.Info(color.HiBlueString("Running DAG-BFT"), "partition", s.Partition.ID, "module", "run", "service", "dagbft")
	return nil
}

// registerAPIServices registers the API services for DAG-BFT.
func (s *DAGBFTService) registerAPIServices(inst *Instance, store keyvalue.Beginner, validatorKey []byte, globals *network.GlobalValues, healCounters *crosschain.HealCounters) error {
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
