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
	"sync"
	"time"

	"github.com/fatih/color"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/crosschain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	execute "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/dagbft"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	dagconfig "gitlab.com/accumulatenetwork/accumulate/pkg/consensus/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
)

// DAGBFTService is defined in types_gen.go via schema.yml.
// This file contains the runtime implementation methods.

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

// dagbftRuntime holds transient runtime state for DAGBFTService.
type dagbftRuntime struct {
	service  *dagbft.Service
	eventBus *events.Bus
	globals  chan *network.GlobalValues
}

// dagbftRuntimes stores runtime state for each DAGBFTService instance.
var (
	dagbftRuntimes   = make(map[*DAGBFTService]*dagbftRuntime)
	dagbftRuntimesMu sync.RWMutex
)

// getRuntime returns the runtime state for a DAGBFTService, creating it if needed.
func (s *DAGBFTService) getRuntime() *dagbftRuntime {
	dagbftRuntimesMu.Lock()
	defer dagbftRuntimesMu.Unlock()
	rt := dagbftRuntimes[s]
	if rt == nil {
		rt = &dagbftRuntime{}
		dagbftRuntimes[s] = rt
	}
	return rt
}

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
	// Apply defaults - use int64 versions for generated type
	setDefaultPtr(&s.EnableHealing, false)
	setDefaultPtr(&s.EnableDirectDispatch, true)
	setDefaultPtr(&s.MaxEnvelopesPerBlock, uint64(100))
	setDefaultPtr(&s.NumWorkers, int64(dagconfig.DefaultNumWorkers))
	setDefaultPtr(&s.DAGGCDepth, int64(dagconfig.DefaultDAGGCDepth))
	setDefaultPtr(&s.CommitBufferSize, int64(dagconfig.DefaultCommitBufferSize))

	// Get runtime state
	rt := s.getRuntime()

	// Get the logger
	logger := (*logging.Slogger)(inst.logger)

	// Create event bus
	rt.eventBus = events.NewBus(logging.FromCometBFT(logger.With("module", "events")))

	// Subscribe to fatal errors
	events.SubscribeAsync(rt.eventBus, func(e events.FatalError) {
		slog.ErrorContext(inst.context, "Shutting down due to a fatal error", "error", e.Err)
		inst.shutdown()
	})

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
		Events: rt.eventBus,
		Logger: logging.FromCometBFT(logger),
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
	db := database.New(store, logging.FromCometBFT(logger))

	// Create executor options
	execOpts := execute.Options{
		Logger:        logging.FromCometBFT(logger.With("module", "executor")),
		Database:      db,
		Key:           validatorKey,
		Router:        router,
		EventBus:      rt.eventBus,
		Sequencer:     client.Private(),
		Querier:       client,
		EnableHealing: *s.EnableHealing,
		Describe: execute.DescribeShim{
			NetworkType: s.Partition.Type,
			PartitionId: s.Partition.ID,
		},
	}

	// Configure dispatcher
	if *s.EnableDirectDispatch {
		execOpts.NewDispatcher = func() execute.Dispatcher {
			return accumulated.NewDispatcher(inst.config.Network, router, dialer)
		}
	} else {
		execOpts.NewDispatcher = func() execute.Dispatcher {
			return accumulated.NewDispatcher(inst.config.Network, router, dialer)
		}
	}

	// Setup globals channel
	rt.globals = make(chan *network.GlobalValues, 1)
	events.SubscribeSync(rt.eventBus, func(e events.WillChangeGlobals) error {
		select {
		case rt.globals <- e.New:
		default:
		}
		return nil
	})

	// Start conductor for cross-chain communication
	conductor := &crosschain.Conductor{
		Partition:           s.Partition,
		ValidatorKey:        execOpts.Key,
		Database:            execOpts.Database,
		Querier:             v3.Querier2{Querier: client},
		Dispatcher:          execOpts.NewDispatcher(),
		RunTask:             execOpts.BackgroundTaskLauncher,
		EnableAnchorHealing: Ptr(false),
	}
	err = conductor.Start(rt.eventBus)
	if err != nil {
		return errors.UnknownError.WithFormat("start conductor: %w", err)
	}

	// Create executor
	exec, err := execute.NewExecutor(execOpts)
	if err != nil {
		return errors.UnknownError.WithFormat("create executor: %w", err)
	}

	// Create executor adapter
	executorBridge, err := adapter.NewExecutorBridge(adapter.ExecutorBridgeConfig{
		Executor:    exec,
		PartitionID: s.Partition.ID,
		EventBus:    rt.eventBus,
	})
	if err != nil {
		return errors.UnknownError.WithFormat("create executor bridge: %w", err)
	}

	// Build DAG-BFT configuration
	dagCfg := dagconfig.DefaultConfig()
	dagCfg.Consensus.NumWorkers = int(*s.NumWorkers)
	dagCfg.Consensus.DAGGCDepth = int(*s.DAGGCDepth)
	dagCfg.Consensus.CommitBufferSize = int(*s.CommitBufferSize)

	// Create the DAG-BFT node configuration
	nodeConfig := consensus.NodeConfig{
		Partition:        s.Partition.ID,
		KeyPair:          validatorKey,
		NumWorkers:       int(*s.NumWorkers),
		DAGGCDepth:       types.Round(*s.DAGGCDepth),
		CommitBufferSize: int(*s.CommitBufferSize),
	}

	// Create GossipSub for DAG-BFT certificate/batch dissemination
	// This enables multi-node consensus networking via libp2p
	var ps *pubsub.PubSub
	if inst.p2p != nil {
		h := inst.p2p.Host()
		if h != nil {
			ps, err = pubsub.NewGossipSub(inst.context, h,
				pubsub.WithPeerExchange(true),
				pubsub.WithFloodPublish(true),
			)
			if err != nil {
				return errors.UnknownError.WithFormat("create gossipsub: %w", err)
			}
			slog.Info("Created GossipSub for DAG-BFT networking", "partition", s.Partition.ID)
		}
	}

	// Create the service
	svcConfig := dagbft.ServiceConfig{
		Partition:  s.Partition,
		NodeConfig: nodeConfig,
		Adapter:    executorBridge,
		EventBus:   rt.eventBus,
		Logger:     logging.FromCometBFT(logger.With("module", "dagbft")),
		Genesis:    inst.path(s.Genesis),
	}

	// Wire in libp2p networking if available
	if inst.p2p != nil && ps != nil {
		svcConfig.Host = inst.p2p.Host()
		svcConfig.PubSub = ps
	}

	rt.service, err = dagbft.NewService(svcConfig)
	if err != nil {
		return errors.UnknownError.WithFormat("create DAG-BFT service: %w", err)
	}

	// Start the service
	err = rt.service.Start(inst.context)
	if err != nil {
		return errors.UnknownError.WithFormat("start DAG-BFT service: %w", err)
	}

	// Register cleanup
	inst.cleanup("dagbft service", func(ctx context.Context) error {
		return rt.service.Stop()
	})

	// Register event bus
	err = dagbftProvidesEventBus.Register(inst.services, s, rt.eventBus)
	if err != nil {
		return errors.UnknownError.WithFormat("register event bus: %w", err)
	}

	// Register consensus API services
	err = s.registerAPIServices(inst, rt, store, validatorKey)
	if err != nil {
		return err
	}

	inst.logger.Info(color.HiBlueString("Running DAG-BFT"), "partition", s.Partition.ID, "module", "run", "service", "dagbft")
	return nil
}

// registerAPIServices registers the API services for DAG-BFT.
func (s *DAGBFTService) registerAPIServices(inst *Instance, rt *dagbftRuntime, store keyvalue.Beginner, validatorKey []byte) error {
	logger := (*logging.Slogger)(inst.logger)
	db := database.New(store, logging.FromCometBFT(logger))

	// Create consensus service
	consensusSvc := dagbft.NewConsensusAPIService(dagbft.ConsensusAPIServiceParams{
		Logger:           logging.FromCometBFT(logger.With("module", "api")),
		Service:          rt.service,
		Database:         db,
		PartitionID:      s.Partition.ID,
		PartitionType:    s.Partition.Type,
		EventBus:         rt.eventBus,
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
		Logger:  logging.FromCometBFT(logger.With("module", "api")),
		Service: rt.service,
	})
	registerRpcService(inst, submitterSvc.Type().AddressFor(s.Partition.ID), message.Submitter{Submitter: submitterSvc})
	err = dagbftProvidesSubmitter.Register(inst.services, s, submitterSvc)
	if err != nil {
		return errors.UnknownError.WithFormat("register submitter service: %w", err)
	}

	// Create validator service
	validatorSvc := dagbft.NewValidatorService(dagbft.ValidatorServiceParams{
		Logger:  logging.FromCometBFT(logger.With("module", "api")),
		Service: rt.service,
	})
	registerRpcService(inst, validatorSvc.Type().AddressFor(s.Partition.ID), message.Validator{Validator: validatorSvc})
	err = dagbftProvidesValidator.Register(inst.services, s, validatorSvc)
	if err != nil {
		return errors.UnknownError.WithFormat("register validator service: %w", err)
	}

	// Wait for globals to be available
	var globals *network.GlobalValues
	select {
	case globals = <-rt.globals:
	case <-time.After(5 * time.Second):
		// Use a default if globals aren't available yet
		globals = new(network.GlobalValues)
	}

	// Create sequencer service
	sequencerSvc := api.NewSequencer(api.SequencerParams{
		Logger:       logging.FromCometBFT(logger.With("module", "api")),
		Database:     db,
		EventBus:     rt.eventBus,
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

// Ensure DAGBFTService implements the required interfaces
var (
	_ Service    = (*DAGBFTService)(nil)
	_ prestarter = (*DAGBFTService)(nil)
)
