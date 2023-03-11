// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"bytes"
	"sort"
	"sync/atomic"
	"time"

	abcitypes "github.com/tendermint/tendermint/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	apiv2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
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
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/testing"
)

type Node struct {
	id         int
	init       *accumulated.NodeInit
	simulator  *Simulator
	partition  *Partition
	logger     logging.OptionalLogger
	eventBus   *events.Bus
	database   database.Beginner
	nodeKey    []byte
	privValKey []byte

	globals  atomic.Value
	executor execute.Executor
	apiV2    *apiv2.JrpcMethods
	clientV2 *client.Client
	querySvc api.Querier
	eventSvc api.EventService
	seqSvc   private.Sequencer

	validatorUpdates []*validatorUpdate
}

func newNode(s *Simulator, p *Partition, node int, init *accumulated.NodeInit) (*Node, error) {
	n := new(Node)
	n.id = node
	n.init = init
	n.simulator = s
	n.partition = p
	n.logger.Set(p.logger, "node", node)
	n.eventBus = events.NewBus(n.logger)
	n.database = s.database(p.ID, node, n.logger)
	n.privValKey = init.PrivValKey
	if p.Type == config.Directory {
		n.nodeKey = init.DnNodeKey
	} else {
		n.nodeKey = init.BvnNodeKey
	}

	// This is hacky, but 🤷 I don't see another choice that wouldn't be
	// significantly less readable
	if p.Type == config.Directory && node == 0 {
		events.SubscribeSync(n.eventBus, s.router.willChangeGlobals)
	}

	// Create a Querier service
	n.querySvc = apiimpl.NewQuerier(apiimpl.QuerierParams{
		Logger:    n.logger.With("module", "acc-rpc"),
		Database:  n,
		Partition: p.ID,
	})

	// Create an EventService
	n.eventSvc = apiimpl.NewEventService(apiimpl.EventServiceParams{
		Logger:    n.logger.With("module", "acc-rpc"),
		Database:  n.database,
		Partition: p.ID,
		EventBus:  n.eventBus,
	})

	// Create a Sequencer service
	n.seqSvc = apiimpl.NewSequencer(apiimpl.SequencerParams{
		Logger:       n.logger.With("module", "acc-rpc"),
		Database:     n,
		EventBus:     n.eventBus,
		Partition:    p.ID,
		ValidatorKey: n.privValKey,
	})

	// Describe the network, from the node's perspective
	network := config.Describe{
		NetworkType:  p.Type,
		PartitionId:  p.ID,
		LocalAddress: init.AdvertizeAddress,
		Network:      *s.netcfg,
	}

	// Set up the executor options
	execOpts := block.ExecutorOptions{
		Logger:        n.logger,
		Database:      n,
		Key:           init.PrivValKey,
		Describe:      network,
		Router:        s.router,
		EventBus:      n.eventBus,
		NewDispatcher: func() execute.Dispatcher { return &dispatcher{sim: s, envelopes: map[string][][]messaging.Message{}} },
		Sequencer:     s.Services(),
		Querier:       s.Services(),
	}

	// Add background tasks to the block's error group. The simulator must call
	// Group.Wait before changing the group, to ensure no race conditions.
	execOpts.BackgroundTaskLauncher = func(f func()) {
		s.blockErrGroup.Go(func() error {
			f()
			return nil
		})
	}

	// Initialize the major block scheduler
	if p.Type == config.Directory {
		execOpts.MajorBlockScheduler = blockscheduler.Init(n.eventBus)
	}

	// Create an executor
	var err error
	n.executor, err = execute.NewExecutor(execOpts)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Set up the API
	n.apiV2, err = apiv2.NewJrpc(apiv2.Options{
		Logger:        n.logger,
		TxMaxWaitTime: time.Hour,
		Describe:      &network,
		LocalV3:       (*nodeService)(n),
		Querier:       (*simService)(s),
		Submitter:     (*simService)(s),
		Network:       (*simService)(s),
		Faucet:        (*simService)(s),
		Validator:     (*simService)(s),
		Sequencer:     (*simService)(s),
	})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Create an API client
	n.clientV2 = testing.DirectJrpcClient(n.apiV2)

	// Subscribe to global variable changes
	events.SubscribeSync(n.eventBus, n.willChangeGlobals)

	return n, nil
}

func (n *Node) Begin(writable bool) *database.Batch         { return n.database.Begin(writable) }
func (n *Node) Update(fn func(*database.Batch) error) error { return n.database.Update(fn) }
func (n *Node) View(fn func(*database.Batch) error) error   { return n.database.View(fn) }
func (n *Node) SetObserver(observer database.Observer)      { n.database.SetObserver(observer) }

func (n *Node) willChangeGlobals(e events.WillChangeGlobals) error {
	n.globals.Store(e.New)

	// Compare the old and new partition definitions
	updates, err := e.Old.DiffValidators(e.New, n.partition.ID)
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
	var root []byte
	err := n.View(func(batch *database.Batch) (err error) {
		root, err = n.executor.LoadStateRoot(batch)
		return err
	})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load state root: %w", err)
	}
	if root != nil {
		return root, nil
	}

	// Restore the snapshot
	err = n.executor.RestoreSnapshot(n, snapshot)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("restore snapshot: %w", err)
	}

	err = n.View(func(batch *database.Batch) (err error) {
		root, err = n.executor.LoadStateRoot(batch)
		return err
	})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load state root: %w", err)
	}
	return root, nil
}

func (n *Node) checkTx(messages []messaging.Message, typ abcitypes.CheckTxType) ([]*protocol.TransactionStatus, error) {
	// TODO: Maintain a shared batch if typ is not recheck. I tried to do this
	// but it lead to "attempted to use a committed or discarded batch" panics.

	batch := n.database.Begin(false)
	defer batch.Discard()
	s, err := n.executor.Validate(batch, messages)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("check messages: %w", err)
	}
	return s, nil
}

func (n *Node) beginBlock(params execute.BlockParams) (execute.Block, error) {
	block, err := n.executor.Begin(params)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("begin block: %w", err)
	}
	return block, nil
}

func (n *Node) deliverTx(block execute.Block, messages []messaging.Message) ([]*protocol.TransactionStatus, error) {
	s, err := block.Process(messages)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("deliver messages: %w", err)
	}
	return s, nil
}

func (n *Node) endBlock(block execute.Block) (execute.BlockState, []*validatorUpdate, error) {
	state, err := block.Close()
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("end block: %w", err)
	}

	if state.IsEmpty() {
		return state, nil, nil
	}

	u := n.validatorUpdates
	n.validatorUpdates = nil
	return state, u, nil
}

func (n *Node) commit(state execute.BlockState) ([]byte, error) {
	if state.IsEmpty() {
		// Discard changes
		state.Discard()

		// Get the old root
		batch := n.database.Begin(false)
		defer batch.Discard()
		return batch.BptRoot(), nil
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

	// Get the  root
	batch := n.database.Begin(false)
	defer batch.Discard()
	return batch.BptRoot(), nil
}
