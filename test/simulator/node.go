// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"sort"
	"sync/atomic"
	"time"

	abci "github.com/tendermint/tendermint/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	apiv2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/blockscheduler"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/testing"
)

type Node struct {
	id         int
	init       *accumulated.NodeInit
	partition  *Partition
	logger     logging.OptionalLogger
	eventBus   *events.Bus
	database   database.Beginner
	nodeKey    []byte
	privValKey []byte

	globals  atomic.Value
	executor *block.Executor
	apiV2    *apiv2.JrpcMethods
	clientV2 *client.Client
	querySvc api.Querier
	seqSvc   private.Sequencer

	validatorUpdates []*validatorUpdate
}

func newNode(s *Simulator, p *Partition, node int, init *accumulated.NodeInit) (*Node, error) {
	n := new(Node)
	n.id = node
	n.init = init
	n.partition = p
	n.logger.Set(p.logger, "node", node)
	n.eventBus = events.NewBus(n.logger)
	n.database = s.database(p.ID, node, n.logger)
	n.privValKey = init.PrivValKey
	n.nodeKey = init.NodeKey

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
		Logger:   n.logger,
		Key:      init.PrivValKey,
		Describe: network,
		Router:   s.router,
		EventBus: n.eventBus,
	}

	// Initialize the major block scheduler
	if p.Type == config.Directory {
		execOpts.MajorBlockScheduler = blockscheduler.Init(n.eventBus)
	}

	// Create an executor
	var err error
	n.executor, err = block.NewNodeExecutor(execOpts, n)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Add background tasks to the block's error group. The simulator must call
	// Group.Wait before changing the group, to ensure no race conditions.
	n.executor.Background = func(f func()) {
		s.blockErrGroup.Go(func() error {
			f()
			return nil
		})
	}

	// Set up the API
	n.apiV2, err = apiv2.NewJrpc(apiv2.Options{
		Logger:        n.logger,
		Describe:      &network,
		Router:        s.router,
		TxMaxWaitTime: time.Hour,
		Database:      n,
		Key:           init.PrivValKey,
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
	batch := n.Begin(true)
	defer batch.Discard()
	err = n.executor.RestoreSnapshot(batch, snapshot)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("restore snapshot: %w", err)
	}
	err = batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
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

func (n *Node) checkTx(delivery *chain.Delivery, typ abci.CheckTxType) (*protocol.TransactionStatus, error) {
	// TODO: Maintain a shared batch if typ is not recheck. I tried to do this
	// but it lead to "attempted to use a commited or discarded batch" panics.

	batch := n.database.Begin(false)
	defer batch.Discard()

	r, err := n.executor.ValidateEnvelope(batch, delivery)
	s := new(protocol.TransactionStatus)
	s.TxID = delivery.Transaction.ID()
	s.Result = r
	if err != nil {
		s.Set(err)
	}
	return s, nil
}

func (n *Node) beginBlock(block *block.Block) error {
	block.Batch = n.Begin(true)
	err := n.executor.BeginBlock(block)
	if err != nil {
		return errors.UnknownError.WithFormat("begin block: %w", err)
	}
	return nil
}

func (n *Node) deliverTx(block *block.Block, delivery *chain.Delivery) (*protocol.TransactionStatus, error) {
	s, err := n.executor.ExecuteEnvelope(block, delivery)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("deliver envelope: %w", err)
	}
	return s, nil
}

func (n *Node) endBlock(block *block.Block) ([]*validatorUpdate, error) {
	err := n.executor.EndBlock(block)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("end block: %w", err)
	}

	if block.State.Empty() {
		return nil, nil
	}

	u := n.validatorUpdates
	n.validatorUpdates = nil
	return u, nil
}

func (n *Node) commit(block *block.Block) ([]byte, error) {
	if block.State.Empty() {
		// Discard changes
		block.Batch.Discard()

		// Get the old root
		batch := n.database.Begin(false)
		defer batch.Discard()
		return batch.BptRoot(), nil
	}

	// Commit
	err := block.Batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("commit: %w", err)
	}

	// Notify
	err = n.eventBus.Publish(events.DidCommitBlock{
		Index: block.Index,
		Time:  block.Time,
		Major: block.State.MakeMajorBlock,
	})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("notify of commit: %w", err)
	}

	// Get the  root
	batch := n.database.Begin(false)
	defer batch.Discard()
	return batch.BptRoot(), nil
}

// nodeService implements API v3.
type nodeService Node

// Private returns the private sequencer service.
func (s *nodeService) Private() private.Sequencer { return s.seqSvc }

// NodeStatus implements pkg/api/v3.NodeService.
func (s *nodeService) NodeStatus(ctx context.Context, opts api.NodeStatusOptions) (*api.NodeStatus, error) {
	return &api.NodeStatus{
		Ok: true,
		LastBlock: &api.LastBlock{
			Height: int64(s.partition.blockIndex),
			Time:   s.partition.blockTime,
			// TODO: chain root, state root
		},
		NodeKeyHash:      sha256.Sum256(s.nodeKey[32:]),
		ValidatorKeyHash: sha256.Sum256(s.privValKey[32:]),
		PartitionID:      s.partition.ID,
		PartitionType:    s.partition.Type,
	}, nil
}

// NetworkStatus implements pkg/api/v3.NetworkService.
func (s *nodeService) NetworkStatus(ctx context.Context, opts api.NetworkStatusOptions) (*api.NetworkStatus, error) {
	v, ok := s.globals.Load().(*core.GlobalValues)
	if !ok {
		return nil, errors.NotReady
	}
	return &api.NetworkStatus{
		Oracle:  v.Oracle,
		Network: v.Network,
		Globals: v.Globals,
		Routing: v.Routing,
	}, nil
}

// Metrics implements pkg/api/v3.MetricsService.
func (s *nodeService) Metrics(ctx context.Context, opts api.MetricsOptions) (*api.Metrics, error) {
	return nil, errors.NotAllowed
}

// Query implements pkg/api/v3.Querier.
func (s *nodeService) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	r, err := s.querySvc.Query(ctx, scope, query)
	if err != nil {
		return nil, err
	}
	// Force despecialization of generic types
	b, err := r.MarshalBinary()
	if err != nil {
		return nil, errors.InternalError.Wrap(err)
	}
	r, err = api.UnmarshalRecord(b)
	if err != nil {
		return nil, errors.InternalError.Wrap(err)
	}
	return r, nil
}

// Submit implements pkg/api/v3.Submitter.
func (s *nodeService) Submit(ctx context.Context, envelope *protocol.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	return s.submit(envelope, false)
}

// Validate implements pkg/api/v3.Validator.
func (s *nodeService) Validate(ctx context.Context, envelope *protocol.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
	return s.submit(envelope, true)
}

func (s *nodeService) submit(envelope *protocol.Envelope, pretend bool) ([]*api.Submission, error) {
	// Convert the envelope to deliveries
	deliveries, err := chain.NormalizeEnvelope(envelope)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Submit each delivery
	var r []*api.Submission
	for i, delivery := range deliveries {
		sub := new(api.Submission)
		sub.Status, err = s.partition.Submit(delivery, pretend)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("delivery %d: %w", i, err)
		}

		// Create an api.Submission
		sub.Success = sub.Status.Code.Success()
		if sub.Status.Error != nil {
			sub.Message = sub.Status.Error.Message
		}
		r = append(r, sub)
	}
	return r, nil
}
