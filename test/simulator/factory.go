// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"fmt"
	"sync"

	"github.com/cometbft/cometbft/libs/log"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/bsn"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/blockscheduler"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	execute "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator/consensus"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator/services"
	"golang.org/x/exp/slog"
)

type simFactory struct {
	// Options
	network    *accumulated.NetworkInit
	storeOpt   OpenDatabaseFunc
	snapshot   SnapshotFunc
	recordings RecordingFunc
	abci       abciFunc

	dropDispatchedMessages      bool
	skipProposalCheck           bool
	ignoreDeliverResults        bool
	ignoreCommitResults         bool
	deterministic               bool
	interceptDispatchedMessages dispatchInterceptor

	// State
	logger           log.Logger
	taskQueue        *taskQueue
	router           *Router
	services         *services.Network
	dispatcherFunc   func() execute.Dispatcher
	networkFactories []*networkFactory
}

type networkFactory struct {
	*simFactory

	// Options
	id    string
	typ   protocol.PartitionType
	app   appFunc
	nodes []*accumulated.NodeInit

	// State
	logger log.Logger
	gossip *consensus.Gossip
}

type nodeFactory struct {
	*networkFactory

	// Options
	id      int
	network *accumulated.NodeInit

	// State
	logger     log.Logger
	_nodeKey   []byte
	peerID     peer.ID
	store      keyvalue.Beginner
	database   *database.Database
	eventBus   *events.Bus
	svcHandler *message.Handler
}

func (f *simFactory) Build() *Simulator {
	// Initialize
	s := new(Simulator)
	s.deterministic = f.deterministic
	s.logger = f.getLogger()
	s.router = f.getRouter()
	s.services = f.getServices()
	s.tasks = f.getTaskQueue()

	// Setup the faucet
	handler, _ := message.NewHandler(message.Faucet{Faucet: (*simFaucet)(s)})
	s.services.RegisterService("", api.ServiceTypeFaucet.AddressForUrl(protocol.AcmeUrl()), handler.Handle)

	// Build the networks
	s.partitions = map[string]*Partition{}
	for _, net := range f.getNetworkFactories() {
		s.partitions[net.id] = net.Build(s)
		s.partIDs = append(s.partIDs, net.id)
	}
	return s
}

func (f *networkFactory) Build(s *Simulator) *Partition {
	p := new(Partition)
	p.ID = f.id
	p.Type = f.typ
	p.sim = s
	p.logger = f.getLogger()
	p.gossip = f.getGossip()
	p.mu = new(sync.Mutex)

	for id, init := range f.nodes {
		node := &nodeFactory{
			networkFactory: f,
			id:             id,
			network:        init,
		}

		// This is hacky, but 🤷 I don't see another choice that wouldn't be
		// significantly less readable
		if f.typ == protocol.PartitionTypeDirectory && id == 0 {
			events.SubscribeSync(node.getEventBus(), s.router.willChangeGlobals)
		}

		p.nodes = append(p.nodes, node.Build(p))
	}
	return p
}

func (f *nodeFactory) Build(p *Partition) *Node {
	n := new(Node)
	n.id = f.id
	n.network = f.network
	n.partition = p
	n.logger = f.getLogger()
	n.eventBus = f.getEventBus()
	n.nodeKey = f.getNodeKey()
	n.peerID = f.getPeerID()
	n.consensus = f.app(f)
	n.services = f.getSvcHandler()

	// This is a hack
	if f.typ != protocol.PartitionTypeBlockSummary {
		n.database = f.getDatabase()
	}

	// Register services
	f.registerSvc(api.ServiceTypeConsensus, message.ConsensusService{ConsensusService: n})
	f.registerSvc(api.ServiceTypeSubmit, message.Submitter{Submitter: n})
	f.registerSvc(api.ServiceTypeValidate, message.Validator{Validator: n})

	// Collect and submit block summaries
	if f.simFactory.network.Bsn != nil && n.partition.Type != protocol.PartitionTypeBlockSummary {
		f.initCollector(p.sim)
	}

	if f.recordings != nil {
		// Set up the recorder
		file, err := f.recordings(p.ID, f.id)
		if err != nil {
			panic(fmt.Errorf("open record file: %w", err))
		}
		r := newRecorder(file)

		// Record the header
		err = r.WriteHeader(&recordHeader{
			Partition: &p.PartitionInfo,
			Config:    f.network,
			NodeID:    fmt.Sprint(f.id),
		})
		if err != nil {
			panic(err)
		}

		n.consensus.SetRecorder(r)
	}

	return n
}

func (f *nodeFactory) initCollector(s *Simulator) {
	// Collect block summaries
	_, err := bsn.StartCollector(bsn.CollectorOptions{
		Partition: f.networkFactory.id,
		Database:  f.getDatabase(),
		Events:    f.getEventBus(),
	})
	if err != nil {
		panic(fmt.Errorf("start collector: %w", err))
	}

	signer := nodeSigner(f.network.PrivValKey)
	events.SubscribeAsync(f.getEventBus(), func(e bsn.DidCollectBlock) {
		env, err := build.SignatureForMessage(e.Summary).
			Url(protocol.PartitionUrl(f.networkFactory.id)).
			Signer(signer).
			Done()
		if err != nil {
			f.getLogger().Error("Failed to sign block summary", "error", err)
			return
		}

		msg := new(messaging.BlockAnchor)
		msg.Anchor = e.Summary
		msg.Signature = env.Signatures[0].(protocol.KeySignature)

		st, err := s.SubmitTo(f.simFactory.network.Bsn.Id, &messaging.Envelope{Messages: []messaging.Message{msg}})
		if err != nil {
			f.getLogger().Error("Failed to submit block summary envelope", "error", err)
			return
		}
		for _, st := range st {
			if st.Error != nil {
				f.getLogger().Error("Block summary envelope failed", "error", st.AsError())
			}
		}
	})
}

func (f *simFactory) getLogger() log.Logger {
	if f.logger != nil {
		return f.logger
	}

	f.logger = (*logging.Slogger)(slog.Default()).With("module", "sim")
	return f.logger
}

func (f *networkFactory) getLogger() log.Logger {
	if f.logger != nil {
		return f.logger
	}

	f.logger = f.simFactory.getLogger().With("partition", f.id)
	return f.logger
}

func (f *nodeFactory) getLogger() log.Logger {
	if f.logger != nil {
		return f.logger
	}

	f.logger = f.networkFactory.getLogger().With("node", f.id)
	return f.logger
}

func (f *simFactory) getTaskQueue() *taskQueue {
	if f.taskQueue != nil {
		return f.taskQueue
	}

	f.taskQueue = newTaskQueue()
	return f.taskQueue
}

func (f *simFactory) getRouter() *Router {
	if f.router != nil {
		return f.router
	}

	f.router = newRouter(f.getLogger())
	return f.router
}

func (f *simFactory) getServices() *services.Network {
	if f.services != nil {
		return f.services
	}

	f.services = services.NewNetwork(f.getRouter())
	return f.services
}

func (f *simFactory) getDispatcherFunc() func() execute.Dispatcher {
	if f.dispatcherFunc != nil {
		return f.dispatcherFunc
	}

	if f.dropDispatchedMessages {
		f.dispatcherFunc = func() execute.Dispatcher {
			return new(fakeDispatcher)
		}
		return f.dispatcherFunc
	}

	// Avoid capture
	services := f.getServices()
	router := f.getRouter()
	interceptor := f.interceptDispatchedMessages

	f.dispatcherFunc = func() execute.Dispatcher {
		d := new(dispatcher)
		d.client = services
		d.router = router
		d.envelopes = map[string][]*messaging.Envelope{}
		d.interceptor = interceptor
		return d
	}
	return f.dispatcherFunc
}

func (f *simFactory) getNetworkFactories() []*networkFactory {
	if f.networkFactories != nil {
		return f.networkFactories
	}

	dirNodes := []*accumulated.NodeInit{}
	for _, init := range f.network.Bvns {
		dirNodes = append(dirNodes, init.Nodes...)
	}

	f.networkFactories = append(f.networkFactories, &networkFactory{
		simFactory: f,
		id:         protocol.Directory,
		typ:        protocol.PartitionTypeDirectory,
		app:        (*nodeFactory).makeCoreApp,
		nodes:      dirNodes,
	})

	for _, bvn := range f.network.Bvns {
		f.networkFactories = append(f.networkFactories, &networkFactory{
			simFactory: f,
			id:         bvn.Id,
			typ:        protocol.PartitionTypeBlockValidator,
			app:        (*nodeFactory).makeCoreApp,
			nodes:      bvn.Nodes,
		})
	}

	if f.network.Bsn != nil {
		f.networkFactories = append(f.networkFactories, &networkFactory{
			simFactory: f,
			id:         f.network.Bsn.Id,
			typ:        protocol.PartitionTypeBlockSummary,
			app:        (*nodeFactory).makeSummaryApp,
			nodes:      f.network.Bsn.Nodes,
		})
	}

	return f.networkFactories
}

func (f *networkFactory) getGossip() *consensus.Gossip {
	if f.gossip != nil {
		return f.gossip
	}

	f.gossip = new(consensus.Gossip)
	return f.gossip
}

func (f *nodeFactory) getNodeKey() []byte {
	if f._nodeKey != nil {
		return f._nodeKey
	}

	switch f.networkFactory.typ {
	case protocol.PartitionTypeDirectory:
		f._nodeKey = f.network.DnNodeKey
	case protocol.PartitionTypeBlockValidator:
		f._nodeKey = f.network.BvnNodeKey
	case protocol.PartitionTypeBlockSummary:
		f._nodeKey = f.network.BsnNodeKey
	default:
		panic(fmt.Errorf("unknown partition type %v", f.networkFactory.typ))
	}
	return f._nodeKey
}

func (f *nodeFactory) getPeerID() peer.ID {
	if f.peerID != "" {
		return f.peerID
	}

	sk, err := crypto.UnmarshalEd25519PrivateKey(f.getNodeKey())
	if err != nil {
		panic(err)
	}
	f.peerID, err = peer.IDFromPrivateKey(sk)
	if err != nil {
		panic(err)
	}
	return f.peerID
}

func (f *nodeFactory) getStore() keyvalue.Beginner {
	if f.store != nil {
		return f.store
	}

	f.store = f.storeOpt(&protocol.PartitionInfo{
		ID:   f.networkFactory.id,
		Type: f.networkFactory.typ,
	}, f.id, f.getLogger())
	return f.store
}

func (f *nodeFactory) getDatabase() *database.Database {
	if f.database != nil {
		return f.database
	}

	f.database = database.New(f.getStore(), f.getLogger())
	return f.database
}

func (f *nodeFactory) getEventBus() *events.Bus {
	if f.eventBus != nil {
		return f.eventBus
	}

	f.eventBus = events.NewBus(f.getLogger())
	return f.eventBus
}

func (f *nodeFactory) getSvcHandler() *message.Handler {
	if f.svcHandler != nil {
		return f.svcHandler
	}

	f.svcHandler, _ = message.NewHandler()
	return f.svcHandler
}

func (f *nodeFactory) registerSvc(typ api.ServiceType, svc message.Service) {
	h := f.getSvcHandler()
	_ = h.Register(svc)
	f.getServices().RegisterService(f.getPeerID(), typ.AddressFor(f.networkFactory.id), h.Handle)
}

type abciFunc = func(*nodeFactory, execute.Executor, consensus.RestoreFunc) consensus.App

func noABCI(node *nodeFactory, exec execute.Executor, restore consensus.RestoreFunc) consensus.App {
	return &consensus.ExecutorApp{
		Executor: exec,
		EventBus: node.getEventBus(),
		Restore:  restore,
	}
}

// func withABCI(node *nodeFactory, exec execute.Executor, restore consensus.RestoreFunc) consensus.App {
// 	a := abci.NewAccumulator(abci.AccumulatorOptions{
// 		Config: &config.Config{
// 			Accumulate: config.Accumulate{
// 				Describe: config.Describe{
// 					PartitionId: node.networkFactory.id,
// 				},
// 			},
// 		},
// 		Executor: exec,
// 		EventBus: node.eventBus,
// 		Logger:   node.logger,
// 		Database: node.getDatabase(),
// 		Address:  node.network.PrivValKey,
// 	})
// 	return (*consensus.AbciApp)(a)
// }

type appFunc = func(*nodeFactory) *consensus.Node

func (f *nodeFactory) makeSummaryApp() *consensus.Node {
	exec, err := bsn.NewExecutor(bsn.ExecutorOptions{
		PartitionID: f.networkFactory.id,
		Logger:      f.getLogger(),
		Store:       f.getStore(),
		EventBus:    f.getEventBus(),
	})
	if err != nil {
		panic(err)
	}

	// Create the app interface
	abci := f.abci(f, exec, func(file ioutil.SectionReader) error {
		return bsn.LoadSnapshot(file, f.getStore(), f.getLogger())
	})

	// Create the consensus node
	return f.makeConsensusNode(abci)
}

func (f *nodeFactory) makeCoreApp() *consensus.Node {
	// Register a querier service
	f.registerSvc(api.ServiceTypeQuery, message.Querier{
		Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{
			Logger:    f.getLogger().With("module", "acc-rpc"),
			Database:  f.getDatabase(),
			Partition: f.networkFactory.id,
		}),
	})

	// Register an event service
	f.registerSvc(api.ServiceTypeEvent, message.EventService{
		EventService: apiimpl.NewEventService(apiimpl.EventServiceParams{
			Logger:    f.getLogger().With("module", "acc-rpc"),
			Database:  f.getDatabase(),
			Partition: f.networkFactory.id,
			EventBus:  f.getEventBus(),
		}),
	})

	// Register a network service
	f.registerSvc(api.ServiceTypeNetwork, message.NetworkService{
		NetworkService: apiimpl.NewNetworkService(apiimpl.NetworkServiceParams{
			Logger:    f.getLogger().With("module", "acc-rpc"),
			Database:  f.getDatabase(),
			Partition: f.networkFactory.id,
			EventBus:  f.getEventBus(),
		}),
	})

	// Register a sequencer service
	f.registerSvc(private.ServiceTypeSequencer, &message.Sequencer{
		Sequencer: apiimpl.NewSequencer(apiimpl.SequencerParams{
			Logger:       f.getLogger().With("module", "acc-rpc"),
			Database:     f.getDatabase(),
			EventBus:     f.getEventBus(),
			Partition:    f.networkFactory.id,
			ValidatorKey: f.network.PrivValKey,
		}),
	})

	// Set up the executor options
	execOpts := block.ExecutorOptions{
		Logger:        f.getLogger(),
		Database:      f.getDatabase(),
		Key:           f.network.PrivValKey,
		Router:        f.getRouter(),
		EventBus:      f.getEventBus(),
		NewDispatcher: f.getDispatcherFunc(),
		Sequencer:     f.getServices().Private(),
		Querier:       f.getServices(),
		EnableHealing: true,
		Describe:      execute.DescribeShim{NetworkType: f.networkFactory.typ, PartitionId: f.networkFactory.id},
	}

	// Add background tasks to the block's error group. The simulator must call
	// Group.Wait before changing the group, to ensure no race conditions.
	tasks := f.getTaskQueue()
	execOpts.BackgroundTaskLauncher = func(f func()) {
		tasks.Go(func() error {
			f()
			return nil
		})
	}

	// Initialize the major block scheduler
	if f.networkFactory.typ == protocol.PartitionTypeDirectory {
		execOpts.MajorBlockScheduler = blockscheduler.Init(f.getEventBus())
	}

	// Create an executor
	exec, err := execute.NewExecutor(execOpts)
	if err != nil {
		panic(err)
	}

	// Create the app interface
	abci := f.abci(f, exec, func(file ioutil.SectionReader) error {
		return snapshot.FullRestore(execOpts.Database, file, f.getLogger(), execOpts.Describe.PartitionUrl())
	})

	// Create the consensus node
	return f.makeConsensusNode(abci)
}

func (node *nodeFactory) makeConsensusNode(app consensus.App) *consensus.Node {
	cn := consensus.NewNode(node.network.PrivValKey, app, node.getGossip(), node.getLogger())
	cn.SkipProposalCheck = node.skipProposalCheck
	cn.IgnoreDeliverResults = node.ignoreDeliverResults
	cn.IgnoreCommitResults = node.ignoreCommitResults
	return cn
}
