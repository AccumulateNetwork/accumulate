// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"bytes"
	"context"

	"github.com/libp2p/go-libp2p/core/peer"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/crosschain"
	coreexec "gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"io"
	"sort"
	"sync"

	execute "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator/consensus"
)

type Partition struct {
	protocol.PartitionInfo
	sim        *Simulator
	logger     logging.Logger
	mu         *sync.Mutex
	nodes      []*Node
	submitHook SubmitHookFunc
}

type SubmitHookFunc = func([]messaging.Message) (drop, keepHook bool)
type BlockHookFunc = func(execute.BlockParams, []*messaging.Envelope) (_ []*messaging.Envelope, keepHook bool)
type NodeBlockHookFunc = func(int, execute.BlockParams, []*messaging.Envelope) (_ []*messaging.Envelope, keepHook bool)

func (p *Partition) View(fn func(*database.Batch) error) error { return p.nodes[0].database.View(fn) }

// Staging is the partition's staging as its first node holds it: what has been
// received on each stream and not yet executed (executor spec, "Sync").
func (p *Partition) Staging() *coreexec.Staging { return p.nodes[0].staging }

// NodeCount is how many nodes the partition runs.
func (p *Partition) NodeCount() int { return len(p.nodes) }

// NodeDatabase is node i's database, for a test that compares nodes.
func (p *Partition) NodeDatabase(i int) *database.Database { return p.nodes[i].database }

// RestartNode stands node i where a restarted validator stands, and starts its
// join: its staging is empty — staging is memory, and a restart loses it — and
// from here it collects the blocks it is handed instead of executing them
// (executor spec, "Sync", step 1). It executes nothing until the join
// completes; a restart IS a join (#4205, #4294).
//
// The simulator has no process to restart and no consensus buffer: every node
// is handed every block, so "buffered" and "collected" are the same thing
// here. What the join must still get right is the same: the node takes a
// peer's staging, takes the state, and executes from the block after.
func (p *Partition) RestartNode(i int) {
	p.nodes[i].staging.Reset()
	p.nodes[i].join.leave()
}

// Joining reports whether node i is collecting rather than executing.
func (p *Partition) Joining(i int) bool { return p.nodes[i].join.Joining() }

// TakeStaging is step 2 of node i's join: it takes node j's staging as of j's
// last committed block, through the same private API a real node uses
// (#4291), and loads it. The blocks that follow are applied to it as they
// arrive, which is what collecting has been doing since RestartNode.
func (p *Partition) TakeStaging(i, j int) error {
	from, ok := p.NodePrivate(j).(private.StagingSnapshotter)
	if !ok {
		return errors.NotAllowed.With("this node does not serve staging")
	}
	snap, err := private.FetchStagingSnapshot(context.Background(), from, p.ID)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	return p.nodes[i].join.takeStaging(snap)
}

// CompleteJoin is steps 3 and 4 of node i's join: the state comes from node j
// — in the simulator by copying its store, where a real node pulls it account
// by account and verifies each against the anchored root (#4293) — staging is
// settled at the block that state is, and node i executes from the next block
// as any node does.
func (p *Partition) CompleteJoin(i, j int) error {
	src, ok := p.nodes[j].store.(*memory.Database)
	dst, ok2 := p.nodes[i].store.(*memory.Database)
	if !ok || !ok2 {
		return errors.NotAllowed.With("the simulator's join needs in-memory stores")
	}
	entries, err := src.Export()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	err = dst.Import(entries)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// The block the state is: what the peer's ledger says, which is what the
	// anchored-root match answers on a real network.
	var q uint64
	err = p.nodes[i].database.View(func(batch *database.Batch) error {
		var ledger *protocol.SystemLedger
		err := batch.Account(protocol.PartitionUrl(p.ID).JoinPath(protocol.Ledger)).Main().GetAs(&ledger)
		if err != nil {
			return err
		}
		q = ledger.Index
		return nil
	})
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	settler, ok := p.nodes[i].executor.(interface{ SettleStagingAt(uint64) error })
	if !ok {
		return errors.NotAllowed.With("this executor cannot settle staging")
	}
	err = settler.SettleStagingAt(q)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	p.nodes[i].join.done()
	return nil
}

// NodeStaging is node i's staging, for a test that compares nodes.
func (p *Partition) NodeStaging(i int) *coreexec.Staging { return p.nodes[i].staging }

// NodePeerID is node i's peer ID. A joining node must never pull from itself
// (#4303), and join.QueryPeers drops its own ID from every list; a test that
// stands a node where a restarted one stands needs that ID to do the same.
func (p *Partition) NodePeerID(i int) peer.ID { return p.nodes[i].peerID }

// NodePrivate is the private API as node i serves it, addressed to that node:
// what a joining node asks a running validator for (#4291).
func (p *Partition) NodePrivate(i int) private.Sequencer {
	addr := private.ServiceTypeSequencer.AddressFor(p.ID).Multiaddr()
	return p.sim.services.ForPeer(p.nodes[i].peerID).ForAddress(addr).Private()
}

// Heals is the partition's healing counters as its first node's conductor
// keeps them: entries pulled by the requester, requests, misses.
func (p *Partition) Heals() *crosschain.HealCounters { return p.nodes[0].heals }

// SynthCache is the partition's producer cache (node 0's).
func (p *Partition) SynthCache() *synthcache.Cache { return p.nodes[0].synthCache }

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
		//nolint:staticcheck // SA1019: SetObserver is deprecated but still needed for simulator
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
			PublicKey: n.network.PrivValKey[32:],
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
				userTxnOrder[msg.Hash()] = i
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
				return userTxnOrder[a.Hash()] < userTxnOrder[b.Hash()]
			}

			// Sort system transactions by their sequence number
			if x := sysTxnOrder[a.Hash()] - sysTxnOrder[b.Hash()]; x != 0 {
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
