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
	gossip *consensus.Gossip
	nodes  []*Node

	mu         *sync.Mutex
	submitHook SubmitHookFunc
}

type SubmitHookFunc = func([]messaging.Message) (drop, keepHook bool)
type BlockHookFunc = func(execute.BlockParams, []*messaging.Envelope) (_ []*messaging.Envelope, keepHook bool)
type NodeBlockHookFunc = func(int, execute.BlockParams, []*messaging.Envelope) (_ []*messaging.Envelope, keepHook bool)

func newPartition(s *Simulator, partition protocol.PartitionInfo) *Partition {
	p := new(Partition)
	p.PartitionInfo = partition
	p.sim = s
	p.gossip = new(consensus.Gossip)
	p.logger.Set(s.logger, "partition", partition.ID)
	p.mu = new(sync.Mutex)
	return p
}

func (o *Options) newBvn(s *Simulator, init *accumulated.BvnInit) (*Partition, error) {
	p := newPartition(s, protocol.PartitionInfo{
		ID:   init.Id,
		Type: protocol.PartitionTypeBlockValidator,
	})

	for _, node := range init.Nodes {
		n, err := o.newNode(s, p, len(p.nodes), node)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		p.nodes = append(p.nodes, n)
	}
	return p, nil
}

func (o *Options) newDn(s *Simulator) (*Partition, error) {
	p := newPartition(s, protocol.PartitionInfo{
		ID:   protocol.Directory,
		Type: protocol.PartitionTypeDirectory,
	})

	for _, init := range o.network.Bvns {
		for _, init := range init.Nodes {
			n, err := o.newNode(s, p, len(p.nodes), init)
			if err != nil {
				return nil, errors.UnknownError.Wrap(err)
			}
			p.nodes = append(p.nodes, n)
		}
	}
	return p, nil
}

func (o *Options) newBsn(s *Simulator, init *accumulated.BvnInit) (*Partition, error) {
	p := newPartition(s, protocol.PartitionInfo{
		ID:   init.Id,
		Type: protocol.PartitionTypeBlockSummary,
	})

	for _, node := range init.Nodes {
		n, err := o.newNode(s, p, len(p.nodes), node)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		p.nodes = append(p.nodes, n)
	}
	return p, nil
}

func (p *Partition) View(fn func(*database.Batch) error) error { return p.nodes[0].database.View(fn) }

func (p *Partition) Update(fn func(*database.Batch) error) error {
	for i, n := range p.nodes {
		err := n.database.Update(fn)
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
		return p.nodes[0].database.Begin(false)
	}
	if len(p.nodes) > 1 {
		panic("cannot create a writeable batch when running with multiple nodes")
	}
	return p.nodes[0].database.Begin(true)
}

func (p *Partition) SetObserver(observer database.Observer) {
	for _, n := range p.nodes {
		n.database.SetObserver(observer)
	}
}

func (p *Partition) SetSubmitHook(fn SubmitHookFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.submitHook = fn
}

// SetBlockHook sets a general block hook. SetBlockHook is mutually exclusive
// with SetNodeBlockHook.
func (p *Partition) SetBlockHook(fn BlockHookFunc) {
	for _, n := range p.nodes {
		n.consensus.SetExecuteHook(func(_ *consensus.Node, block execute.BlockParams, envelopes []*messaging.Envelope) (_ []*messaging.Envelope, keepHook bool) {
			return fn(block, envelopes)
		})
	}
}

// SetNodeBlockHook sets a node-specific block hook. SetNodeBlockHook is
// mutually exclusive with SetBlockHook.
func (p *Partition) SetNodeBlockHook(fn NodeBlockHookFunc) {
	lup := map[*consensus.Node]int{}
	for _, n := range p.nodes {
		lup[n.consensus] = n.id
		n.consensus.SetExecuteHook(func(n *consensus.Node, block execute.BlockParams, envelopes []*messaging.Envelope) (_ []*messaging.Envelope, keepHook bool) {
			return fn(lup[n], block, envelopes)
		})
	}
}

func (p *Partition) initChain(snapshot ioutil2.SectionReader) error {
	var val []*execute.ValidatorUpdate
	for _, n := range p.nodes {
		val = append(val, &execute.ValidatorUpdate{
			Type:      protocol.SignatureTypeED25519,
			PublicKey: n.privValKey[32:],
			Power:     1,
		})
	}

	results := make([][]byte, len(p.nodes))
	for i, n := range p.nodes {
		_, err := snapshot.Seek(0, io.SeekStart)
		if err != nil {
			return errors.UnknownError.WithFormat("reset snapshot file: %w", err)
		}
		res, err := n.consensus.Init(&consensus.InitRequest{Snapshot: snapshot, Validators: val})
		if err != nil {
			return errors.UnknownError.WithFormat("init chain: %w", err)
		}
		results[i] = res.Hash
	}
	for _, v := range results[1:] {
		if !bytes.Equal(results[0], v) {
			return errors.FatalError.WithFormat("consensus failure: init chain: expected %x, got %x", results[0], v)
		}
	}
	return nil
}

func (p *Partition) applySubmitHook(messages []messaging.Message) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.submitHook == nil {
		return false
	}

	drop, keep := p.submitHook(messages)
	if !keep {
		p.submitHook = nil
	}

	return drop
}

func (p *Partition) Submit(envelope *messaging.Envelope, pretend bool) ([]*protocol.TransactionStatus, error) {
	// Apply the hook
	messages, err := envelope.Normalize()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if p.applySubmitHook(messages) {
		st := make([]*protocol.TransactionStatus, len(messages))
		for i, msg := range messages {
			st[i] = new(protocol.TransactionStatus)
			st[i].TxID = msg.ID()
			st[i].Code = errors.NotAllowed
			st[i].Error = errors.NotAllowed.With("dropped")
		}
		return st, nil
	}

	res, err := p.nodes[0].consensus.Check(&consensus.CheckRequest{
		Envelope: envelope.Copy(), // Copy to avoid weird bugs
		New:      false,           // Set type = recheck to make the executor create a new batch to avoid timing issues
		Pretend:  pretend,
	})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return res.Results, nil
}

func (p *Partition) execute() error {
	_, err := p.nodes[0].consensus.Execute(&consensus.ExecuteRequest{Context: context.Background()})
	return err
}

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
