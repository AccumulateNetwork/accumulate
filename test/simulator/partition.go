package simulator

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/tendermint/tendermint/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	"gitlab.com/accumulatenetwork/accumulate/internal/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/sortutil"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/sync/errgroup"
)

type Partition struct {
	protocol.PartitionInfo
	logger     logging.OptionalLogger
	nodes      []*Node
	validators [][32]byte

	mu         *sync.Mutex
	mempool    []*chain.Delivery
	deliver    []*chain.Delivery
	blockIndex uint64
	blockTime  time.Time

	submitHook SubmitHookFunc
}

type SubmitHookFunc func(*chain.Delivery) (dropTx, keepHook bool)

type validatorUpdate struct {
	key [32]byte
	typ core.ValidatorUpdate
}

func newPartition(s *Simulator, partition protocol.PartitionInfo) *Partition {
	p := new(Partition)
	p.blockTime = GenesisTime
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

	for _, init := range init.Nodes {
		n, err := newNode(s, p, len(p.nodes), init)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknownError, err)
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
				return nil, errors.Wrap(errors.StatusUnknownError, err)
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

func (p *Partition) SetSubmitHook(fn SubmitHookFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.submitHook = fn
}

func (p *Partition) initChain(snapshot ioutil2.SectionReader) error {
	results := make([][]byte, len(p.nodes))
	for i, n := range p.nodes {
		_, err := snapshot.Seek(0, io.SeekStart)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "reset snapshot file: %w", err)
		}
		results[i], err = n.initChain(snapshot)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "init chain: %w", err)
		}
	}
	for _, v := range results[1:] {
		if !bytes.Equal(results[0], v) {
			return errors.Format(errors.StatusFatalError, "consensus failure: init chain: expected %x, got %x", results[0], v)
		}
	}
	return nil
}

func (p *Partition) Submit(deliveries []*chain.Delivery, pretend bool) ([]*protocol.TransactionStatus, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Call the hook
	drop := make([]bool, len(deliveries))
	if p.submitHook != nil {
		for i, delivery := range deliveries {
			dropTx, keepHook := p.submitHook(delivery)
			if !dropTx {
				drop[i] = true
			}
			if !keepHook {
				p.submitHook = nil
				break
			}
		}
	}

	var err error
	results := make([]*protocol.TransactionStatus, len(deliveries))
	consensus := make([]*protocol.TransactionStatus, len(p.nodes))
	for i, delivery := range deliveries {
		if drop[i] {
			s := new(protocol.TransactionStatus)
			s.TxID = delivery.Transaction.ID()
			results[i] = s
			continue
		}

		for i, node := range p.nodes {
			consensus[i], err = node.checkTx(delivery, types.CheckTxType_New)
			if err != nil {
				return nil, errors.Wrap(errors.StatusFatalError, err)
			}
		}
		for _, r := range consensus[1:] {
			if !consensus[0].Equal(r) {
				return nil, errors.Format(errors.StatusFatalError, "consensus failure: check tx: transaction %x (%v)", delivery.Transaction.GetHash()[:4], delivery.Transaction.Body.Type())
			}
		}
		results[i] = consensus[0]
		if !pretend && consensus[0].Code.Success() {
			p.mempool = append(p.mempool, delivery)
		}
	}

	return results, nil
}

func (p *Partition) execute(background *errgroup.Group) error {
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
			var ledger *protocol.SystemLedger
			err := batch.Account(protocol.PartitionUrl(p.ID).JoinPath(protocol.Ledger)).GetStateAs(&ledger)
			switch {
			case err == nil:
				p.blockIndex = ledger.Index + 1
			case errors.Is(err, errors.StatusNotFound):
				p.blockIndex = protocol.GenesisBlock + 1
			default:
				return errors.Format(errors.StatusFatalError, "load system ledger: %w", err)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	// Begin block
	leader := int(p.blockIndex) % len(p.nodes)
	blocks := make([]*block.Block, len(p.nodes))
	for i, n := range p.nodes {
		b := new(block.Block)
		b.Index = p.blockIndex
		b.Time = p.blockTime
		b.IsLeader = i == leader
		blocks[i] = b

		n.executor.Background = func(f func()) { background.Go(func() error { f(); return nil }) }

		err := n.beginBlock(b)
		if err != nil {
			return errors.Format(errors.StatusFatalError, "execute: %w", err)
		}
	}

	// Deliver Tx
	var err error
	results := make([]*protocol.TransactionStatus, len(p.nodes))
	for i, delivery := range deliveries {
		for j, node := range p.nodes {
			results[j], err = node.deliverTx(blocks[i], delivery)
			if err != nil {
				return errors.Format(errors.StatusFatalError, "execute: %w", err)
			}
		}
		for _, r := range results[1:] {
			if !results[0].Equal(r) {
				return errors.Format(errors.StatusFatalError, "consensus failure: deliver tx: transaction %x (%v)", delivery.Transaction.GetHash()[:4], delivery.Transaction.Body.Type())
			}
		}
	}

	// End block
	endBlock := make([][]*validatorUpdate, len(p.nodes))
	for i, n := range p.nodes {
		endBlock[i], err = n.endBlock(blocks[i])
		if err != nil {
			return errors.Format(errors.StatusFatalError, "execute: %w", err)
		}
	}
	for _, v := range endBlock[1:] {
		if len(v) != len(endBlock[0]) {
			return errors.Format(errors.StatusFatalError, "consensus failure: end block")
		}
		for i, v := range v {
			if v != endBlock[0][i] {
				return errors.Format(errors.StatusFatalError, "consensus failure: end block")
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
		commit[i], err = n.commit(blocks[i])
		if err != nil {
			return errors.Format(errors.StatusFatalError, "execute: %w", err)
		}
	}
	for _, v := range commit[1:] {
		if !bytes.Equal(commit[0], v) {
			return errors.Format(errors.StatusFatalError, "consensus failure: commit: expected %x, got %x", commit[0], v)
		}
	}

	return nil
}
