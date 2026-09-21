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
	"io"
	"sort"
	"sync"
	"time"

	execute "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
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

// RestartNode stands node i where a restarted validator stands: its staging is
// empty -- staging is memory, and a restart loses it -- and from here it
// collects the blocks it is handed into its own stage instead of executing
// them (executor spec, "Sync", §5). It executes nothing until the join
// completes; a restart IS a join (#4205, #4294).
//
// The simulator has no process to restart and no consensus buffer: every node
// is handed every block, so "buffered" and "collected" are the same thing
// here. What the join must get right is the same, and it is the production
// join that gets it: see StartJoin.
func (p *Partition) RestartNode(i int) {
	p.nodes[i].staging.Reset()
	p.nodes[i].join.leave()
}

// Joining reports whether node i is collecting rather than executing.
func (p *Partition) Joining(i int) bool { return p.nodes[i].join.Joining() }

// A Join is node i's join, running.
type Join struct {
	mu   sync.Mutex
	err  error
	done bool
}

// Done reports whether the join has finished, and Err says how.
func (j *Join) Done() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.done
}

// Err is the error the join ended with, or nil.
func (j *Join) Err() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.err
}

// StartJoin runs node i's join -- THE PRODUCTION JOIN, join.Run, with the
// production pull -- against its peers, and returns while it runs. The caller
// steps the network until Joining(i) is false.
//
// It used to copy node j's STORE into node i's and call that the state pull.
// That is not a join and it could not fail: it moved a verified state from one
// process to another with no peer, no receipt, no anchored root and no
// convergence, so every defect the pull has ever had was invisible here --
// including the one that stopped every restarted node on the twelve-node
// network for a month (#4362). What runs now is what a node runs: the pull is
// addressed at NAMED PEERS with this node's own ID dropped (#4303), every
// account is verified against the root a quorum signed for the block it was
// asked at, and the node executes the block after that one only when no stream
// has a gap.
//
// What it still cannot do, and what Docker is for: there is no process to
// restart, so nothing here exercises the daemon's start-up -- the node-state
// seam, the services registered while BOOTING, or reading the executed block
// from a record no pull writes. Those are #4295's, and the soak's.
func (p *Partition) StartJoin(i int) *Join {
	out := new(Join)
	n := p.nodes[i]

	state, err := join.NewState(join.StateOptions{
		Partition: protocol.PartitionUrl(p.ID),
		Database:  n.database,
		Sources: &join.QueryPeers{
			Client:  p.sim.Services(),
			Network: p.sim.networkId,
			Router:  p.sim.router,
			Self:    n.peerID,
		},
		EventBus: n.eventBus,
	})
	if err != nil {
		out.err, out.done = err, true
		return out
	}

	stage, ok := n.executor.(join.Stage)
	if !ok {
		out.err, out.done = errors.NotAllowed.With("this executor cannot settle its staging at a block"), true
		return out
	}

	go func() {
		err := join.Run(context.Background(), join.Options{
			Partition: p.ID,
			Buffer:    n.join,
			Stage:     stage,
			State:     state,
			Retry:     time.Millisecond,
		})
		out.mu.Lock()
		out.err, out.done = err, true
		out.mu.Unlock()
	}()
	return out
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

// NodeHeals is node i's healing counters. Per node, because whether a
// particular node asks for its own gaps is the question, and the Prometheus
// counters are the whole process's (#4367).
func (p *Partition) NodeHeals(i int) *crosschain.HealCounters { return p.nodes[i].heals }

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

// isValidatorOn reports whether this node runs as a validator on the
// partition, from the node's OWN configuration — the same field
// BuildGenesisDocs turns into the definition's active flag
// (internal/node/daemon/init.go:213-214). A node configured as a follower is
// in the definition and active on nothing, and the two readings must agree:
// a simulator node that is a follower in genesis and a validator in consensus
// is a node no operator can deploy.
//
// It settles who LEADS (state_block.go, the leader is drawn over
// n.validators) and not who votes. A follower here still votes and still
// counts its own vote: newVotes seeds the tracker with n.self
// unconditionally and reachedThreshold counts it (consensus/voting.go:24-29,
// 46-50), so a follower logs `Vote by non-validator` on its peers -- 1,021
// of them in one run of TestAFollowerDispatchesNoAnchor -- while its own
// node reaches the threshold one vote early. Harmless in lockstep, where
// every node executes the same block anyway, but it means **this simulator
// cannot show that a follower does not vote**: that is consensus, it is the
// same shape as #4371, and a test asserting it here would be asserting it of
// a node that does.
func (n *Node) isValidatorOn(typ protocol.PartitionType) bool {
	switch typ {
	case protocol.PartitionTypeDirectory:
		return n.network.DnnType == config.Validator
	case protocol.PartitionTypeBlockSummary:
		return n.network.BsnnType == config.Validator
	default:
		return n.network.BvnnType == config.Validator
	}
}

func (p *Partition) initChain(snapshot ioutil2.SectionReader) error {
	// Only the partition's validators. Every node used to be handed to
	// InitChain as a validator whatever the definition said, so a follower
	// could not be built here at all: the executor refuses a validator
	// genesis does not have (v1/block/executor.go:299, "InitChain request
	// includes N validator(s) not present in genesis"), and before that
	// check a follower would have run as a full voting member of a committee
	// it is not in — the simulator quietly answering the question gate 0
	// asks (#4367).
	var val []*execute.ValidatorUpdate
	for _, n := range p.nodes {
		if !n.isValidatorOn(p.Type) {
			continue
		}
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
