// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"bytes"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	apiv2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/bsn"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/blockscheduler"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	execute "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/testing"
)

type Node struct {
	id         int
	mu         *sync.Mutex
	init       *accumulated.NodeInit
	simulator  *Simulator
	partition  *Partition
	logger     logging.OptionalLogger
	eventBus   *events.Bus
	database   *database.Database
	nodeKey    []byte
	privValKey []byte

	record      *recorder
	submissions []*messaging.Envelope

	globals  atomic.Value
	executor execute.Executor
	apiV2    *apiv2.JrpcMethods
	clientV2 *client.Client
	querySvc api.Querier
	eventSvc api.EventService
	netSvc   api.NetworkService
	seqSvc   private.Sequencer

	validatorUpdates []*validatorUpdate
}

func newNode(s *Simulator, p *Partition, node int, init *accumulated.NodeInit) (*Node, error) {
	n := new(Node)
	n.id = node
	n.mu = new(sync.Mutex)
	n.init = init
	n.simulator = s
	n.partition = p
	n.logger.Set(p.logger, "node", node)
	n.eventBus = events.NewBus(n.logger)
	n.privValKey = init.PrivValKey
	switch p.Type {
	case protocol.PartitionTypeDirectory:
		n.nodeKey = init.DnNodeKey
	case protocol.PartitionTypeBlockValidator:
		n.nodeKey = init.BvnNodeKey
	case protocol.PartitionTypeBlockSummary:
		n.nodeKey = init.BsnNodeKey
	default:
		return nil, errors.InternalError.WithFormat("unknown partition type %v", p.Type)
	}

	if s.Recordings != nil {
		f, err := s.Recordings(p.ID, node)
		if err != nil {
			return nil, errors.InternalError.WithFormat("open record file: %w", err)
		}
		n.record = newRecorder(f)
	}

	// This is hacky, but 🤷 I don't see another choice that wouldn't be
	// significantly less readable
	if p.Type == protocol.PartitionTypeDirectory && node == 0 {
		events.SubscribeSync(n.eventBus, s.router.willChangeGlobals)
	}

	var err error
	switch n.partition.Type {
	case protocol.PartitionTypeDirectory,
		protocol.PartitionTypeBlockValidator:
		err = n.initValidator()
	case protocol.PartitionTypeBlockSummary:
		err = n.initSummary()
	default:
		err = errors.BadRequest.WithFormat("unknown partition type %v", n.partition.Type)
	}
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Collect and submit block summaries
	if s.init.Bsn != nil && n.partition.Type != protocol.PartitionTypeBlockSummary {
		err := n.initCollector()
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	// Record the header
	if n.record != nil {
		err = n.record.WriteHeader(&recordHeader{
			Partition: &p.PartitionInfo,
			Config:    init,
			NodeNum:   int64(node),
		})
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	return n, nil
}

func (n *Node) initValidator() error {
	store := n.simulator.database(n.partition.ID, n.id, n.logger)
	n.database = database.New(store, n.logger)

	// Create a Querier service
	n.querySvc = apiimpl.NewQuerier(apiimpl.QuerierParams{
		Logger:    n.logger.With("module", "acc-rpc"),
		Database:  n,
		Partition: n.partition.ID,
	})

	// Create an Event service
	n.eventSvc = apiimpl.NewEventService(apiimpl.EventServiceParams{
		Logger:    n.logger.With("module", "acc-rpc"),
		Database:  n.database,
		Partition: n.partition.ID,
		EventBus:  n.eventBus,
	})

	// Create a Network service
	n.netSvc = apiimpl.NewNetworkService(apiimpl.NetworkServiceParams{
		Logger:    n.logger.With("module", "acc-rpc"),
		Database:  n.database,
		Partition: n.partition.ID,
		EventBus:  n.eventBus,
	})

	// Create a Sequencer service
	n.seqSvc = apiimpl.NewSequencer(apiimpl.SequencerParams{
		Logger:       n.logger.With("module", "acc-rpc"),
		Database:     n,
		EventBus:     n.eventBus,
		Partition:    n.partition.ID,
		ValidatorKey: n.privValKey,
	})

	// Describe the network, from the node's perspective
	network := config.Describe{
		NetworkType:  n.partition.Type,
		PartitionId:  n.partition.ID,
		LocalAddress: n.init.AdvertizeAddress,
		Network:      *n.simulator.netcfg,
	}

	// Set up the executor options
	execOpts := block.ExecutorOptions{
		Logger:        n.logger,
		Database:      n,
		Key:           n.init.PrivValKey,
		Describe:      network,
		Router:        n.simulator.router,
		EventBus:      n.eventBus,
		NewDispatcher: n.simulator.newDispatcher,
		Sequencer:     n.simulator.Services(),
		Querier:       n.simulator.Services(),
	}

	// Add background tasks to the block's error group. The simulator must call
	// Group.Wait before changing the group, to ensure no race conditions.
	execOpts.BackgroundTaskLauncher = func(f func()) {
		n.simulator.blockErrGroup.Go(func() error {
			f()
			return nil
		})
	}

	// Initialize the major block scheduler
	if n.partition.Type == protocol.PartitionTypeDirectory {
		execOpts.MajorBlockScheduler = blockscheduler.Init(n.eventBus)
	}

	// Create an executor
	var err error
	n.executor, err = execute.NewExecutor(execOpts)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Set up the API
	n.apiV2, err = apiv2.NewJrpc(apiv2.Options{
		Logger:        n.logger,
		TxMaxWaitTime: time.Hour,
		Describe:      &network,
		LocalV3:       (*nodeService)(n),
		Querier:       (*simService)(n.simulator),
		Submitter:     (*simService)(n.simulator),
		Network:       (*simService)(n.simulator),
		Faucet:        (*simService)(n.simulator),
		Validator:     (*simService)(n.simulator),
		Sequencer:     (*simService)(n.simulator),
	})
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Create an API client
	n.clientV2 = testing.DirectJrpcClient(n.apiV2)

	// Subscribe to global variable changes
	events.SubscribeSync(n.eventBus, n.willChangeGlobals)

	return nil
}

func (n *Node) initCollector() error {
	// Collect block summaries
	_, err := bsn.StartCollector(bsn.CollectorOptions{
		Partition: n.partition.ID,
		Database:  n,
		Events:    n.eventBus,
	})
	if err != nil {
		return errors.UnknownError.WithFormat("start collector: %w", err)
	}

	signer := nodeSigner{n}
	events.SubscribeAsync(n.eventBus, func(e bsn.DidCollectBlock) {
		env, err := build.SignatureForMessage(e.Summary).
			Url(protocol.PartitionUrl(n.partition.ID)).
			Signer(signer).
			Done()
		if err != nil {
			n.logger.Error("Failed to sign block summary", "error", err)
			return
		}

		msg := new(messaging.BlockAnchor)
		msg.Anchor = e.Summary
		msg.Signature = env.Signatures[0].(protocol.KeySignature)

		st, err := n.simulator.SubmitTo(n.simulator.init.Bsn.Id, &messaging.Envelope{Messages: []messaging.Message{msg}})
		if err != nil {
			n.logger.Error("Failed to submit block summary envelope", "error", err)
			return
		}
		for _, st := range st {
			if st.Error != nil {
				n.logger.Error("Block summary envelope failed", "error", st.AsError())
			}
		}
	})
	return nil
}

func (n *Node) initSummary() error {
	var err error
	n.executor, err = bsn.NewExecutor(bsn.ExecutorOptions{
		PartitionID: n.partition.ID,
		Logger:      n.logger,
		Store:       n.simulator.database(n.partition.ID, n.id, n.logger),
		EventBus:    n.eventBus,
	})

	return errors.UnknownError.Wrap(err)
}

func (n *Node) Begin(writable bool) *database.Batch         { return n.database.Begin(writable) }
func (n *Node) Update(fn func(*database.Batch) error) error { return n.database.Update(fn) }
func (n *Node) View(fn func(*database.Batch) error) error   { return n.database.View(fn) }
func (n *Node) SetObserver(observer database.Observer)      { n.database.SetObserver(observer) }

func (n *Node) willChangeGlobals(e events.WillChangeGlobals) error {
	n.globals.Store(e.New)

	// Compare the old and new partition definitions
	updates, err := core.DiffValidators(e.Old, e.New, n.partition.ID)
	if err != nil {
		return err
	}

	// Convert the update list into Tendermint validator updates
	for key, typ := range updates {
		key := key // See docs/developer/rangevarref.md
		n.validatorUpdates = append(n.validatorUpdates, &validatorUpdate{key, typ})
	}

	// Sort the list so we're deterministic
	sort.Slice(n.validatorUpdates, func(i, j int) bool {
		a := n.validatorUpdates[i].key[:]
		b := n.validatorUpdates[j].key[:]
		return bytes.Compare(a, b) < 0
	})

	return nil
}

func (n *Node) initChain(snapshot ioutil2.SectionReader) ([]byte, error) {
	// Check if initialization is required
	_, root, err := n.executor.LastBlock()
	switch {
	case err == nil:
		return root[:], nil
	case errors.Is(err, errors.NotFound):
		// Ok
	default:
		return nil, errors.UnknownError.WithFormat("load state root: %w", err)
	}

	// Restore the snapshot
	_, err = n.executor.Restore(snapshot, nil)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("restore snapshot: %w", err)
	}

	// Record the snapshot
	if n.record != nil {
		err = n.record.WriteSnapshot(snapshot)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("record snapshot: %w", err)
		}
	}

	_, root, err = n.executor.LastBlock()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load state root: %w", err)
	}
	return root[:], nil
}

func (n *Node) checkTx(envelope *messaging.Envelope, new bool) ([]*protocol.TransactionStatus, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	s, err := n.executor.Validate(envelope, !new)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("check messages: %w", err)
	}
	return s, nil
}

func (n *Node) beginBlock(params execute.BlockParams) (execute.Block, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	block, err := n.executor.Begin(params)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("begin block: %w", err)
	}
	return block, nil
}

func (n *Node) deliverTx(block execute.Block, envelopes []*messaging.Envelope) ([]*protocol.TransactionStatus, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.submissions = append(n.submissions, envelopes...)

	var results []*protocol.TransactionStatus
	for _, envelope := range envelopes {
		s, err := block.Process(envelope)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("deliver envelope: %w", err)
		}

		results = append(results, s...)
	}
	return results, nil
}

func (n *Node) endBlock(block execute.Block) (execute.BlockState, []*validatorUpdate, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	state, err := block.Close()
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("end block: %w", err)
	}

	if n.record != nil {
		err = n.record.WriteBlock(state, n.submissions)
		if err != nil {
			return nil, nil, errors.UnknownError.WithFormat("record block: %w", err)
		}
	}

	if state.IsEmpty() {
		return state, nil, nil
	}

	n.submissions = n.submissions[:0]
	u := n.validatorUpdates
	n.validatorUpdates = nil
	return state, u, nil
}

func (n *Node) commit(state execute.BlockState) ([]byte, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if state.IsEmpty() {
		// Discard changes
		state.Discard()

		// Get the old root
		_, root, err := n.executor.LastBlock()
		return root[:], errors.UnknownError.Wrap(err)
	}

	// Commit
	err := state.Commit()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("commit: %w", err)
	}

	// Notify
	major, _, _ := state.DidCompleteMajorBlock()
	err = n.eventBus.Publish(events.DidCommitBlock{
		Index: state.Params().Index,
		Time:  state.Params().Time,
		Major: major,
	})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("notify of commit: %w", err)
	}

	// Get the root
	_, hash, err := n.executor.LastBlock()
	return hash[:], err
}
