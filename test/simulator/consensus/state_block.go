// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"golang.org/x/exp/slog"
)

type blockState state[blockState]

func (n *Node) proposeLeader() (blockState, []Message, error) {
	m := new(didProposeLeader)
	m.Node = n
	m.votes = n.newVotes()
	m.p.baseNodeMessage = n.newMsg()
	m.p.Leader = n.validators[n.lastBlockIndex%uint64(len(n.validators))].PubKeyHash

	s, out, err := executeState[blockState](n.context, m, nil)
	out = append(out, &m.p)
	return s, out, err
}

type proposeLeader struct {
	baseNodeMessage
	LeaderProposal
}

// didProposeLeader is the state of the node after a leader has been proposed.
type didProposeLeader struct {
	*Node
	p             proposeLeader
	votes         votes
	blockProposal *proposeBlock
}

// execute records a vote upon receipt of [proposeLeader] and waits for the
// threshold to be reached. Then, if the receiver is the leader, execute sends
// [proposeBlock] and transitions to [didProposeBlock] upon receipt of
// [proposeLeader]. If the receiver is not the leader, execute sends
// [acceptBlockProposal] and transitions to [didProposeBlock] upon receipt of
// [proposeBlock].
func (n *didProposeLeader) execute(msg Message) (blockState, []Message, error) {
	switch msg := msg.(type) {
	case *proposeLeader:
		// Verify the proposal matches
		if msg.LeaderProposal != n.p.LeaderProposal {
			slog.ErrorContext(n.context, "Conflicting leader proposal",
				"mine", logging.AsHex(n.p.Leader).Slice(0, 4),
				"theirs", logging.AsHex(msg.Leader).Slice(0, 4))
			return n, nil, &ConsensusError[LeaderProposal]{
				Message: "conflicting leader proposal",
				Mine:    n.p.LeaderProposal,
				Theirs:  msg.LeaderProposal,
			}
		}

		// Add the vote
		n.votes.add(msg.senderID())

	case *proposeBlock:
		// Verify the threshold has been reached
		if !n.votes.reachedThreshold() {
			return n, nil, errors.BadRequest.With("received block proposal before selecting a leader")
		}

		// And the block proposal came from the leader
		if h := msg.senderID(); h != n.p.Leader {
			return n, nil, errors.Conflict.WithFormat("got block proposal from wrong leader: want %x, got %x", n.p.Leader[:4], h[:4])
		}

		// Check the proposed block
		//
		// FIXME Fix this and reenable it. It is unreliable and causes
		// intermittent failures.
		if false && !n.SkipProposalCheck {
			for _, env := range msg.Envelopes {
				err := n.mempool.CheckProposed(msg.Index, env)
				if err != nil {
					slog.ErrorContext(n.context, "Consensus failure", "step", "propose", "error", err, "envelope", env, "block", n.lastBlockIndex+1)
					return n, nil, errors.Conflict.WithFormat("proposed envelope: %w", err)
				}
			}
		}

		// Record the proposal
		n.blockProposal = msg
	}

	switch {
	case !n.votes.reachedThreshold():
		// Awaiting more votes
		return n, nil, nil

	case n.p.Leader == n.self.PubKeyHash:
		// We're the leader - propose a block
		return n.proposeBlock(n.p.LeaderProposal)

	case n.blockProposal != nil:
		// We have received a valid proposal - accept it
		return n.acceptBlockProposal(n.blockProposal)

	default:
		// Awaiting a block proposal
		return n, nil, nil
	}
}

func (n *Node) proposeBlock(p LeaderProposal) (blockState, []Message, error) {
	slog.DebugContext(n.context, "Proposing a block", "block", n.lastBlockIndex+1)

	m := new(didProposeBlock)
	m.Node = n
	m.p.baseNodeMessage = n.newMsg()
	m.p.LeaderProposal = p
	m.p.Index = n.lastBlockIndex + 1
	m.p.Time = n.lastBlockTime.Add(time.Second)
	m.p.Envelopes = n.mempool.Propose(m.p.Index)
	m.votes = n.newVotes()
	return m, []Message{&m.p}, nil
}

func (n *Node) acceptBlockProposal(p *proposeBlock) (blockState, []Message, error) {
	m := new(didProposeBlock)
	m.Node = n
	m.p = *p
	m.votes = n.newVotes()
	m.votes.add(p.senderID())
	return m, []Message{&acceptBlockProposal{n.newMsg(), *p}}, nil
}

type proposeBlock struct {
	baseNodeMessage
	BlockProposal
}

type acceptBlockProposal struct {
	baseNodeMessage
	p proposeBlock
}

// didProposeBlock is the state of the node after a block has been proposed.
type didProposeBlock struct {
	*Node
	p     proposeBlock
	votes votes
}

func (m *proposeBlock) equal(n *proposeBlock) bool {
	// The proposals are equal and were proposed by the same node
	return m.senderID() == n.senderID() &&
		m.BlockProposal.Equal(&n.BlockProposal)
}

// execute records a vote upon receipt of [acceptBlockProposal] and waits for
// the threshold to be reached. Then, execute finalizes the block, sends
// [finalizedBlock], and transitions to [didFinalizeBlock].
func (n *didProposeBlock) execute(msg Message) (blockState, []Message, error) {
	switch msg := msg.(type) {
	case *acceptBlockProposal:
		if !msg.p.equal(&n.p) {
			return n, nil, &ConsensusError[BlockProposal]{
				Message: "conflicting block proposal",
				Mine:    n.p.BlockProposal,
				Theirs:  msg.p.BlockProposal,
			}
		}

		n.votes.add(msg.senderID())
	}

	if !n.votes.reachedThreshold() {
		return n, nil, nil
	}

	return n.finalizeBlock(&n.p.BlockProposal)
}

// finalizeBlock executes or 'finalizes' the block.
func (n *Node) finalizeBlock(p *BlockProposal) (blockState, []Message, error) {
	// Prevent races with methods like SetRecorder
	n.mu.Lock()
	defer n.mu.Unlock()

	// Update the last block info and mempool
	n.lastBlockIndex = p.Index
	n.lastBlockTime = p.Time
	n.mempool.AcceptProposed(p.Index, p.Envelopes)

	params := execute.BlockParams{
		Context:  n.context,
		Index:    n.lastBlockIndex,
		Time:     n.lastBlockTime,
		IsLeader: p.Leader == n.self.PubKeyHash,
	}

	// Apply the block hook
	envelopes := copyEnv(p.Envelopes)
	if n.executeHook != nil {
		var keep bool
		envelopes, keep = n.executeHook(n, params, envelopes)
		if !keep {
			n.executeHook = nil
		}
	}

	// Begin the block
	block, err := n.begin(params)
	if err != nil {
		return nil, nil, errors.FatalError.WithFormat("begin block: %w", err)
	}

	slog.DebugContext(n.context, "Block begin", "block", params.Index, "time", params.Time)

	// Deliver the envelopes
	results, err := n.deliver(block, envelopes)
	if err != nil {
		return nil, nil, errors.FatalError.WithFormat("deliver: %w", err)
	}

	for _, r := range results {
		slog.DebugContext(n.context, "Delivered", "block", params.Index, "result", r)
	}

	// End the block
	state, err := n.endBlock(block)
	if err != nil {
		return nil, nil, errors.FatalError.WithFormat("end block: %w", err)
	}

	valUp, _ := state.DidUpdateValidators()
	slog.DebugContext(n.context, "End block", "block", params.Index, "validator-updates", valUp)

	// Send [finalizedBlock] and transition to [didFinalizeBlock].
	m := new(didFinalizeBlock)
	m.Node = n
	m.b.baseNodeMessage = n.newMsg()
	if !n.IgnoreDeliverResults {
		m.b.results.MessageResults = results
	}
	m.b.results.ValidatorUpdates = valUp
	m.votes = n.newVotes()
	m.blockState = state
	return m, []Message{&m.b}, nil
}

type finalizedBlock struct {
	baseNodeMessage
	results BlockResults
}

// didFinalizeBlock is the state of a node after the block has been 'finalized'
// (executed).
type didFinalizeBlock struct {
	*Node
	b          finalizedBlock
	votes      votes
	blockState execute.BlockState
}

// execute records a vote upon receipt of [finalizedBlock] and waits for the
// threshold to be reached. Then, execute commits the block, sends
// [committedBlock], and transitions to [didCommitBlock].
func (n *didFinalizeBlock) execute(msg Message) (blockState, []Message, error) {
	switch msg := msg.(type) {
	case *finalizedBlock:
		if !n.b.results.Equal(&msg.results) {
			return n, nil, &ConsensusError[BlockResults]{
				Message: "conflicting block results",
				Mine:    n.b.results,
				Theirs:  msg.results,
			}
		}

		n.votes.add(msg.senderID())
	}

	if !n.votes.reachedThreshold() {
		return n, nil, nil
	}

	state := n.blockState
	n.blockState = nil
	return n.commitBlock(&n.b.results, state)
}

func (n *Node) commitBlock(results *BlockResults, state execute.BlockState) (blockState, []Message, error) {
	// Apply validator updates
	n.applyValUp(results.ValidatorUpdates)

	// Commit the block
	hash, err := n.commit(state)
	if err != nil {
		return nil, nil, errors.FatalError.WithFormat("commit: %w", err)
	}

	slog.DebugContext(n.context, "Commit", "block", state.Params().Index, "hash", logging.AsHex(hash))

	m := new(didCommitBlock)
	m.Node = n
	m.b.baseNodeMessage = n.newMsg()
	m.b.results.Hash = *(*[32]byte)(hash)
	m.votes = n.newVotes()
	return m, []Message{&m.b}, nil
}

type committedBlock struct {
	baseNodeMessage
	results CommitResult
}

// didCommitBlock is the state of the node after a block has been committed.
type didCommitBlock struct {
	*Node
	b     committedBlock
	votes votes
}

// execute records a vote upon receipt of [committedBlock] and waits for the
// threshold to be reached. Then, execute transitions to [nodeIsQuiescent].
func (n *didCommitBlock) execute(msg Message) (blockState, []Message, error) {
	switch msg := msg.(type) {
	case *committedBlock:
		if !n.IgnoreCommitResults && n.b.results != msg.results {
			return n, nil, &ConsensusError[CommitResult]{
				Message: "conflicting commit results",
				Mine:    n.b.results,
				Theirs:  msg.results,
			}
		}

		n.votes.add(msg.senderID())
	}

	if !n.votes.reachedThreshold() {
		return n, nil, nil
	}

	return n.completeBlock()
}

func (n *Node) completeBlock() (blockState, []Message, error) {
	return nil, []Message{&ExecutedBlock{
		Node:    n.self.PubKeyHash,
		Network: n.network,
	}}, nil
}
