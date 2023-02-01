// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/tendermint/tendermint/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	execute "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Partition struct {
	protocol.PartitionInfo
	logger     logging.OptionalLogger
	nodes      []*Node
	validators [][32]byte

	mu         *sync.Mutex
	mempool    []messaging.Message
	deliver    []messaging.Message
	blockIndex uint64
	blockTime  time.Time

	submitHook       SubmitHookFunc
	routerSubmitHook RouterSubmitHookFunc
}

type SubmitHookFunc func(messaging.Message) (dropTx, keepHook bool)
type RouterSubmitHookFunc func([]messaging.Message) (_ []messaging.Message, keepHook bool)

type validatorUpdate struct {
	key [32]byte
	typ core.ValidatorUpdate
}

func newPartition(s *Simulator, partition protocol.PartitionInfo) *Partition {
	p := new(Partition)
	p.PartitionInfo = partition
	p.logger.Set(s.logger, "partition", partition.ID)
	p.mu = new(sync.Mutex)
	return p
}

func newBvn(s *Simulator, init *accumulated.BvnInit) (*Partition, error) {
	p := newPartition(s, protocol.PartitionInfo{
		ID:   init.Id,
		Type: protocol.PartitionTypeBlockValidator,
	})

	for _, node := range init.Nodes {
		n, err := newNode(s, p, len(p.nodes), node)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		p.nodes = append(p.nodes, n)
	}
	return p, nil
}

func newDn(s *Simulator, init *accumulated.NetworkInit) (*Partition, error) {
	p := newPartition(s, protocol.PartitionInfo{
		ID:   protocol.Directory,
		Type: protocol.PartitionTypeDirectory,
	})

	for _, init := range init.Bvns {
		for _, init := range init.Nodes {
			n, err := newNode(s, p, len(p.nodes), init)
			if err != nil {
				return nil, errors.UnknownError.Wrap(err)
			}
			p.nodes = append(p.nodes, n)
		}
	}
	return p, nil
}

func (p *Partition) View(fn func(*database.Batch) error) error { return p.nodes[0].View(fn) }

func (p *Partition) Update(fn func(*database.Batch) error) error {
	for i, n := range p.nodes {
		err := n.Update(fn)
		if err != nil {
			if i > 0 {
				panic("update succeeded on one node and failed on another")
			}
			return err
		}
	}
	return nil
}

// Begin will panic if called to create a writable batch if the partition has
// more than one node.
func (p *Partition) Begin(writable bool) *database.Batch {
	if !writable {
		return p.nodes[0].Begin(false)
	}
	if len(p.nodes) > 1 {
		panic("cannot create a writeable batch when running with multiple nodes")
	}
	return p.nodes[0].Begin(true)
}

func (p *Partition) SetSubmitHook(fn SubmitHookFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.submitHook = fn
}

func (p *Partition) SetRouterSubmitHook(fn RouterSubmitHookFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.routerSubmitHook = fn
}

func (p *Partition) initChain(snapshot ioutil2.SectionReader) error {
	results := make([][]byte, len(p.nodes))
	for i, n := range p.nodes {
		_, err := snapshot.Seek(0, io.SeekStart)
		if err != nil {
			return errors.UnknownError.WithFormat("reset snapshot file: %w", err)
		}
		results[i], err = n.initChain(snapshot)
		if err != nil {
			return errors.UnknownError.WithFormat("init chain: %w", err)
		}
	}
	for _, v := range results[1:] {
		if !bytes.Equal(results[0], v) {
			return errors.FatalError.WithFormat("consensus failure: init chain: expected %x, got %x", results[0], v)
		}
	}
	return nil
}

func (p *Partition) Submit(message messaging.Message, pretend bool) (*protocol.TransactionStatus, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.submitHook != nil {
		drop, keep := p.submitHook(message)
		if !keep {
			p.submitHook = nil
		}
		if drop {
			s := new(protocol.TransactionStatus)
			s.TxID = message.(*messaging.LegacyMessage).Transaction.ID()
			return s, nil
		}
	}

	var err error
	result := make([]*protocol.TransactionStatus, len(p.nodes))
	for i, node := range p.nodes {
		// Make a copy to prevent changes
		result[i], err = node.checkTx(messaging.CopyMessage(message), types.CheckTxType_New)
		if err != nil {
			return nil, errors.FatalError.Wrap(err)
		}
	}
	for _, r := range result[1:] {
		if !result[0].Equal(r) {
			message := message.(*messaging.LegacyMessage)
			return nil, errors.FatalError.WithFormat("consensus failure: check tx: transaction %x (%v)", message.Transaction.GetHash()[:4], message.Transaction.Body.Type())
		}
	}

	if !pretend && result[0].Code.Success() {
		p.mempool = append(p.mempool, message)
	}
	return result[0], nil
}

func (p *Partition) execute() error {
	// TODO: Limit how many transactions are added to the block? Call recheck?
	p.mu.Lock()
	deliveries := p.deliver
	p.deliver = p.mempool
	p.mempool = nil
	p.mu.Unlock()

	// Initialize block index
	if p.blockIndex > 0 {
		p.blockIndex++
		p.blockTime = p.blockTime.Add(time.Second)
	} else {
		err := p.View(func(batch *database.Batch) error {
			record := batch.Account(protocol.PartitionUrl(p.ID).JoinPath(protocol.Ledger))
			c, err := record.RootChain().Index().Get()
			if err != nil {
				return errors.FatalError.WithFormat("load root index chain: %w", err)
			}
			entry := new(protocol.IndexEntry)
			err = c.EntryAs(c.Height()-1, entry)
			if err != nil {
				return errors.FatalError.WithFormat("load root index chain entry 0: %w", err)
			}
			p.blockIndex = entry.BlockIndex + 1
			p.blockTime = entry.BlockTime.Add(time.Second)
			return nil
		})
		if err != nil {
			return err
		}
	}
	p.logger.Debug("Stepping", "block", p.blockIndex)

	// Begin block
	leader := int(p.blockIndex) % len(p.nodes)
	blocks := make([]execute.Block, len(p.nodes))
	for i, n := range p.nodes {
		var err error
		blocks[i], err = n.beginBlock(execute.BlockParams{
			Context:  context.Background(),
			Index:    p.blockIndex,
			Time:     p.blockTime,
			IsLeader: i == leader,
		})
		if err != nil {
			return errors.FatalError.WithFormat("execute: %w", err)
		}
	}

	// Deliver Tx
	var err error
	results := make([]*protocol.TransactionStatus, len(p.nodes))
	for _, message := range deliveries {
		for i, node := range p.nodes {
			// Make a copy to prevent changes
			results[i], err = node.deliverTx(blocks[i], messaging.CopyMessage(message))
			if err != nil {
				return errors.FatalError.WithFormat("execute: %w", err)
			}
		}
		for _, r := range results[1:] {
			if !results[0].Equal(r) {
				message := message.(*messaging.LegacyMessage)
				return errors.FatalError.WithFormat("consensus failure: deliver tx: transaction %x (%v)", message.Transaction.GetHash()[:4], message.Transaction.Body.Type())
			}
		}
	}

	// End block
	blockState := make([]execute.BlockState, len(p.nodes))
	endBlock := make([][]*validatorUpdate, len(p.nodes))
	for i, n := range p.nodes {
		blockState[i], endBlock[i], err = n.endBlock(blocks[i])
		if err != nil {
			return errors.FatalError.WithFormat("execute: %w", err)
		}
	}
	for _, v := range endBlock[1:] {
		if len(v) != len(endBlock[0]) {
			return errors.FatalError.WithFormat("consensus failure: end block")
		}
		for i, v := range v {
			if *v != *endBlock[0][i] {
				return errors.FatalError.WithFormat("consensus failure: end block")
			}
		}
	}

	// Update validators
	for _, update := range endBlock[0] {
		switch update.typ {
		case core.ValidatorUpdateAdd:
			ptr, new := sortutil.BinaryInsert(&p.validators, func(k [32]byte) int { return bytes.Compare(k[:], update.key[:]) })
			if new {
				*ptr = update.key
			}

		case core.ValidatorUpdateRemove:
			i, found := sortutil.Search(p.validators, func(k [32]byte) int { return bytes.Compare(k[:], update.key[:]) })
			if found {
				sortutil.RemoveAt(&p.validators, i)
			}

		default:
			panic(fmt.Errorf("unknown validator update type %v", update.typ))
		}
	}

	// Commit
	commit := make([][]byte, len(p.nodes))
	for i, n := range p.nodes {
		commit[i], err = n.commit(blockState[i])
		if err != nil {
			return errors.FatalError.WithFormat("execute: %w", err)
		}
	}
	for i, v := range commit[1:] {
		if !bytes.Equal(commit[0], v) {
			return errors.FatalError.WithFormat("consensus failure: commit %s.%d: expected %x, got %x", p.ID, i+1, commit[0], v)
		}
	}

	return nil
}
