// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"bytes"
	"context"
	"io"
	"sort"
	"sync"
	"time"

	execute "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator/consensus"
)

type Partition struct {
	protocol.PartitionInfo
	sim    *Simulator
	logger logging.OptionalLogger
	nodes  []*Node

	mu         *sync.Mutex
	mempool    []*messaging.Envelope
	deliver    []*messaging.Envelope
	blockIndex uint64
	blockTime  time.Time

	submitHook    SubmitHookFunc
	blockHook     BlockHookFunc
	nodeBlockHook NodeBlockHookFunc
	commitHook    CommitHookFunc
}

type SubmitHookFunc = func([]messaging.Message) (drop, keepHook bool)
type BlockHookFunc = func(execute.BlockParams, []*messaging.Envelope) (_ []*messaging.Envelope, keepHook bool)
type NodeBlockHookFunc = func(int, execute.BlockParams, []*messaging.Envelope) (_ []*messaging.Envelope, keepHook bool)
type CommitHookFunc = func(*protocol.PartitionInfo, execute.BlockState)

func newPartition(s *Simulator, partition protocol.PartitionInfo) *Partition {
	p := new(Partition)
	p.PartitionInfo = partition
	p.sim = s
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

func newBsn(s *Simulator, init *accumulated.BvnInit) (*Partition, error) {
	p := newPartition(s, protocol.PartitionInfo{
		ID:   init.Id,
		Type: protocol.PartitionTypeBlockSummary,
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

func (p *Partition) SetObserver(observer database.Observer) {
	for _, n := range p.nodes {
		n.SetObserver(observer)
	}
}

func (p *Partition) SetSubmitHook(fn SubmitHookFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.submitHook = fn
}

func (p *Partition) SetBlockHook(fn BlockHookFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.blockHook = fn
}

func (p *Partition) SetNodeBlockHook(fn NodeBlockHookFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.nodeBlockHook = fn
}

func (p *Partition) SetCommitHook(fn CommitHookFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commitHook = fn
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

func (p *Partition) Submit(envelope *messaging.Envelope, pretend bool) ([]*protocol.TransactionStatus, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.submitHook != nil {
		messages, err := envelope.Normalize()
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		drop, keep := p.submitHook(messages)
		if !keep {
			p.submitHook = nil
		}
		if drop {
			st := make([]*protocol.TransactionStatus, len(messages))
			for i, msg := range messages {
				st[i] = new(protocol.TransactionStatus)
				st[i].TxID = msg.ID()
				st[i].Code = errors.NotAllowed
				st[i].Error = errors.NotAllowed.With("dropped")
			}
			return st, nil
		}
	}

	results := make([][]*protocol.TransactionStatus, len(p.nodes))
	for i, node := range p.nodes {
		var err error
		// Set type = recheck to make the executor create a new batch to avoid timing issues
		results[i], err = node.checkTx(envelope, false)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	for _, r := range results[1:] {
		if len(r) != len(results[0]) {
			return nil, errors.FatalError.WithFormat("consensus failure: different number of results")
		}
		for i, st := range r {
			if !results[0][i].Equal(st) {
				p.logger.Error("Consensus failure", "expected", results[0][i], "actual", st)
				return nil, errors.FatalError.WithFormat("consensus failure: deliver message %v", st.TxID)
			}
		}
	}

	ok := true
	for _, st := range results[0] {
		if st.Failed() {
			ok = false
		}
	}

	if !pretend && ok {
		p.mempool = append(p.mempool, envelope)
	}
	return results[0], nil
}

func (p *Partition) applyBlockHook(node int, messages []*messaging.Envelope) []*messaging.Envelope {
	p.mu.Lock()
	defer p.mu.Unlock()

	block := execute.BlockParams{
		Context: context.Background(),
		Index:   p.blockIndex,
		Time:    p.blockTime,
	}

	var keep bool
	if node < 0 {
		if p.blockHook == nil {
			return messages
		}
		messages, keep = p.blockHook(block, messages)
		if !keep {
			p.blockHook = nil
		}
	} else {
		if p.nodeBlockHook == nil {
			return messages
		}
		messages, keep = p.nodeBlockHook(node, block, messages)
		if !keep {
			p.nodeBlockHook = nil
		}
	}

	return messages
}

func (p *Partition) getCommitHook() CommitHookFunc {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.commitHook
}

func (p *Partition) loadBlockIndex() {
	if p.blockIndex > 0 {
		return
	}

	res, err := p.nodes[0].consensus.Info(&consensus.InfoRequest{})
	if err != nil {
		panic(err)
	}
	p.blockIndex = res.LastBlock.Index
	p.blockTime = res.LastBlock.Time
}

func (p *Partition) execute() error {
	// TODO: Limit how many transactions are added to the block? Call recheck?
	p.mu.Lock()
	envelopes := p.deliver
	p.deliver = p.mempool
	p.mempool = nil
	p.mu.Unlock()

	// if p.sim.Deterministic {
	// 	// Order transactions to ensure the simulator is deterministic
	// 	orderMessagesDeterministically(messages)
	// }

	// Initialize block index
	p.loadBlockIndex()
	p.blockIndex++
	p.blockTime = p.blockTime.Add(time.Second)
	p.logger.Debug("Stepping", "block", p.blockIndex)

	envelopes = p.applyBlockHook(-1, envelopes)

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
	results := make([][]*protocol.TransactionStatus, len(p.nodes))
	for i, node := range p.nodes {
		results[i], err = node.deliverTx(blocks[i], p.applyBlockHook(i, envelopes))
		if err != nil {
			return errors.FatalError.WithFormat("execute: %w", err)
		}
	}
	if !p.sim.IgnoreDeliverResults {
		for _, r := range results[1:] {
			if len(r) != len(results[0]) {
				p.logger.Error("Consensus failure", "step", "deliver", "expected", results[0], "actual", r)
				return errors.FatalError.WithFormat("consensus failure: different number of results")
			}
			for i, st := range r {
				if !results[0][i].Equal(st) {
					p.logger.Error("Consensus failure", "step", "deliver", "id", st.TxID, "expected", results[0][i], "actual", st)
					return errors.FatalError.WithFormat("consensus failure: deliver message %v", st.TxID)
				}
			}
		}
	}

	status := map[[32]byte]error{}
	for _, r := range results[0] {
		if r.Error != nil {
			status[r.TxID.Hash()] = r.AsError()
		}
	}
	for _, envelope := range envelopes {
		messages, err := envelope.Normalize()
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
		for _, msg := range messages {
			if msg.Type() == messaging.MessageTypeTransaction ||
				msg.Type() == messaging.MessageTypeSignature {
				continue
			}
			for {
				if err, ok := status[msg.ID().Hash()]; ok {
					p.logger.Error("System message failed", "err", err, "type", msg.Type(), "id", msg.ID())
				}
				if u, ok := msg.(interface{ Unwrap() messaging.Message }); ok {
					msg = u.Unwrap()
				} else {
					break
				}
			}
		}
	}

	// End block
	blockState := make([]execute.BlockState, len(p.nodes))
	for i, n := range p.nodes {
		blockState[i], err = n.endBlock(blocks[i])
		if err != nil {
			return errors.FatalError.WithFormat("execute: %w", err)
		}
	}

	if !blockState[0].IsEmpty() {
		if hook := p.getCommitHook(); hook != nil {
			hook(&p.PartitionInfo, blockState[0])
		}
	}

	// Commit
	commit := make(CommitConsensusError, len(p.nodes))
	for i, n := range p.nodes {
		commit[i], err = n.commit(blockState[i])
		if err != nil {
			return errors.FatalError.WithFormat("execute: %w", err)
		}
	}
	if commit.isErr() {
		return commit
	}

	return nil
}

type CommitConsensusError [][]byte

func (c CommitConsensusError) isErr() bool {
	for _, v := range c[1:] {
		if !bytes.Equal(c[0], v) {
			return true
		}
	}
	return false
}

func (CommitConsensusError) Error() string { return "consensus failure during commit" }

// orderMessagesDeterministically reorders messages deterministically,
// preserving certain invariants. Transactions must sort first and user
// transactions must stay in their original order.
func orderMessagesDeterministically(messages []messaging.Message) {
	// Record order of user transactions and sequence of system transactions
	userTxnOrder := map[[32]byte]int{}
	sysTxnOrder := map[[32]byte]int{}
	for i, msg := range messages {
		switch msg := msg.(type) {
		case *messaging.TransactionMessage:
			if msg.Transaction.Body.Type().IsUser() {
				userTxnOrder[msg.ID().Hash()] = i
			}

		case *messaging.SignatureMessage:
			if sig, ok := msg.Signature.(*protocol.PartitionSignature); ok {
				sysTxnOrder[sig.TransactionHash] = int(sig.SequenceNumber)
			}
		}
	}

	sort.SliceStable(messages, func(i, j int) bool {
		// Sort by type - user transactions are sorted first because that is the
		// first message type
		a, b := messages[i], messages[j]
		if a.Type() != b.Type() {
			return a.Type() < b.Type()
		}

		switch a := a.(type) {
		case *messaging.TransactionMessage:
			// Sort user transactions first, then anchors, then synthetic
			b := b.(*messaging.TransactionMessage)
			if x := txnOrder(a) - txnOrder(b); x != 0 {
				return x < 0
			}

			// Sort user transactions by their original order
			if a.Transaction.Body.Type().IsUser() {
				return userTxnOrder[a.ID().Hash()] < userTxnOrder[b.ID().Hash()]
			}

			// Sort system transactions by their sequence number
			if x := sysTxnOrder[a.ID().Hash()] - sysTxnOrder[b.ID().Hash()]; x != 0 {
				return x < 0
			}
			return a.ID().Compare(b.ID()) < 0

		case *messaging.SignatureMessage:
			// Sort partition signatures first
			b := b.(*messaging.SignatureMessage)
			c, d := a.Signature.Type() == protocol.SignatureTypePartition, b.Signature.Type() == protocol.SignatureTypePartition
			switch {
			case c && !d:
				return true
			case !c && d:
				return false
			}

			// Otherwise sort by hash
			return bytes.Compare(a.Signature.Hash(), b.Signature.Hash()) < 0

		default:
			// Sort other messages by ID
			return a.ID().Compare(b.ID()) < 0
		}
	})
}

// txnOrder returns an order parameter for the given user transaction. Sorting
// with this will sort user transactions first, then anchors, then synthetic
// transactions.
func txnOrder(msg *messaging.TransactionMessage) int {
	switch {
	case msg.Transaction.Body.Type().IsUser():
		return 0
	case msg.Transaction.Body.Type().IsAnchor():
		return 1
	default:
		return 2
	}
}
