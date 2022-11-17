// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"bytes"
	"sort"
	"time"

	abci "github.com/tendermint/tendermint/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/blockscheduler"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/testing"
)

type Node struct {
	id        int
	init      *accumulated.NodeInit
	partition *protocol.PartitionInfo
	logger    logging.OptionalLogger
	eventBus  *events.Bus
	database  database.Beginner
	executor  *block.Executor
	api       *api.JrpcMethods
	client    *client.Client

	validatorUpdates []*validatorUpdate
}

func newNode(s *Simulator, p *Partition, node int, init *accumulated.NodeInit) (*Node, error) {
	n := new(Node)
	n.id = node
	n.init = init
	n.partition = &p.PartitionInfo
	n.logger.Set(p.logger, "node", node)
	n.eventBus = events.NewBus(n.logger)
	n.database = s.database(p.ID, node, n.logger)

	network := config.Describe{
		NetworkType:  p.Type,
		PartitionId:  p.ID,
		LocalAddress: init.AdvertizeAddress,
		Network:      *s.netcfg,
	}

	execOpts := block.ExecutorOptions{
		Logger:   n.logger,
		Key:      init.PrivValKey,
		Describe: network,
		Router:   s.router,
		EventBus: n.eventBus,
	}
	if p.Type == config.Directory {
		execOpts.MajorBlockScheduler = blockscheduler.Init(n.eventBus)
	}

	var err error
	n.executor, err = block.NewNodeExecutor(execOpts, n)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	n.api, err = api.NewJrpc(api.Options{
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
	n.client = testing.DirectJrpcClient(n.api)

	events.SubscribeSync(n.eventBus, n.willChangeGlobals)

	return n, nil
}

func (n *Node) Begin(writable bool) *database.Batch         { return n.database.Begin(writable) }
func (n *Node) Update(fn func(*database.Batch) error) error { return n.database.Update(fn) }
func (n *Node) View(fn func(*database.Batch) error) error   { return n.database.View(fn) }

func (n *Node) willChangeGlobals(e events.WillChangeGlobals) error {
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
