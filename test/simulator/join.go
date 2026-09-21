// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"sync"

	coreexec "gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	execute "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

// joinState is one node's join, as the DAG service keeps it: while it is
// joining the node executes nothing, and every block it is handed goes into
// ITS OWN stage and into a buffer, in the order it arrived (executor spec,
// "Sync", §5).
//
// It is the join's Buffer (internal/node/join). No peer is ever asked what it
// holds: there is no stage to load from a validator and nothing to wait for
// before holding what arrives. What the node may execute is decided by the gap
// check against the state it PULLED, and that is the only decision there is.
type joinState struct {
	mu      sync.Mutex
	joining bool
	overrun bool

	// next is the block number the next collected block is stamped with. It
	// is seeded once, when the first block arrives, and counts up from there;
	// nothing resets it, and a hole in the numbers is a thing the handoff can
	// see (#4351).
	//
	// The DAG service seeds it from the block the node stands at, because
	// consensus hands it groups and the node derives the block number itself.
	// The simulator hands the node the block number, so that is what the
	// first block is seeded from -- the node's own last executed block is
	// NOT it, because an empty block writes nothing, not even its index, and
	// the ledger therefore lags the blocks the node actually saw (measured
	// here at eighteen).
	next uint64

	// buffer is what has been collected and not executed, oldest first.
	buffer []joinedBlock

	// replay is what the handoff handed back to be executed: the blocks above
	// the one the state is. The block path takes them, as the DAG service's
	// block production loop does, because that is the only place blocks are
	// executed (#4294).
	replay []joinedBlock

	exec execute.Executor
}

type joinedBlock struct {
	block     uint64
	params    coreexec.BlockParams
	envelopes []*messaging.Envelope

	// reach is, per stream, the highest sequence number this block carried:
	// what the gap check is made against.
	reach []coreexec.StreamReach
}

// A blockCollector is an executor that can take a committed block into staging
// without executing it.
type blockCollector interface {
	CollectCommittedBlock(coreexec.BlockParams, []*messaging.Envelope) (*coreexec.CollectedBlock, error)
}

// A stagingGapper is an executor that can compare what a block carried against
// what the pulled state says was delivered.
type stagingGapper interface {
	StagingGaps([]coreexec.StreamReach) ([]coreexec.StreamGap, error)
}

func (j *joinState) Joining() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.joining
}

// Collecting is the join's view of the same thing.
func (j *joinState) Collecting() bool { return j.Joining() }

// BufferOverrun reports that a block could not be taken into staging, so the
// blocks since the node started listening are no longer all in hand.
func (j *joinState) BufferOverrun() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.overrun
}

// leave starts the join: the node stops executing and starts keeping what it
// is handed.
func (j *joinState) leave() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.joining, j.overrun = true, false
	j.buffer, j.replay = nil, nil
	j.next = 0
}

// Collect keeps a block and takes it into this node's own stage.
func (j *joinState) Collect(params coreexec.BlockParams, envelopes []*messaging.Envelope) error {
	col, ok := j.exec.(blockCollector)
	if !ok {
		return errors.NotAllowed.With("this executor cannot collect a block without executing it")
	}

	j.mu.Lock()
	if j.next == 0 {
		j.next = params.Index
	}
	b := joinedBlock{block: j.next, params: params, envelopes: envelopes}
	j.next++
	j.mu.Unlock()

	out, err := col.CollectCommittedBlock(params, envelopes)
	if err != nil {
		j.mu.Lock()
		j.overrun = true
		j.mu.Unlock()
		return errors.UnknownError.Wrap(err)
	}
	b.reach = out.Reach

	j.mu.Lock()
	defer j.mu.Unlock()
	j.buffer = append(j.buffer, b)
	return nil
}

// GapsAt reports whether the block collected as q+1 can be executed.
func (j *joinState) GapsAt(q uint64) ([]coreexec.StreamGap, error) {
	j.mu.Lock()
	var found *joinedBlock
	for i := range j.buffer {
		if j.buffer[i].block == q+1 {
			found = &j.buffer[i]
			break
		}
	}
	n := len(j.buffer)
	j.mu.Unlock()

	if found == nil {
		return nil, errors.NotReady.WithFormat(
			"the state is block %d and the block after it is not among the %d collected", q, n)
	}
	gapper, ok := j.exec.(stagingGapper)
	if !ok {
		return nil, errors.NotAllowed.With("this executor cannot say where its streams stand")
	}
	return gapper.StagingGaps(found.reach)
}

// Handoff leaves collecting mode at block q and hands the blocks above it back
// to be executed, in order.
func (j *joinState) Handoff(q uint64) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.joining {
		return errors.NotAllowed.With("this node is not joining")
	}
	if j.overrun {
		return errors.NotReady.With("the blocks this node collected are no longer all in hand")
	}

	i := 0
	for i < len(j.buffer) && j.buffer[i].block <= q {
		i++
	}
	out := j.buffer[i:]
	if len(out) == 0 {
		return errors.NotReady.WithFormat(
			"the state is block %d and no block after it has been collected yet", q)
	}
	if out[0].block != q+1 {
		return errors.Conflict.WithFormat(
			"the state is block %d and the first block collected after it is %d", q, out[0].block)
	}
	for k := 1; k < len(out); k++ {
		if out[k].block != out[k-1].block+1 {
			return errors.Conflict.WithFormat("the collected blocks jump from %d to %d", out[k-1].block, out[k].block)
		}
	}

	j.replay = out
	j.buffer = nil
	j.joining = false
	return nil
}

// TakeReplay hands back the blocks the handoff left to be executed, once.
func (j *joinState) TakeReplay() []joinedBlock {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := j.replay
	j.replay = nil
	return out
}

// Replay is the blocks the handoff left to be executed, as the consensus app
// sees them: the parameters and the envelopes, nothing else.
func (j *joinState) Replay() []struct {
	Params    coreexec.BlockParams
	Envelopes []*messaging.Envelope
} {
	var out []struct {
		Params    coreexec.BlockParams
		Envelopes []*messaging.Envelope
	}
	for _, b := range j.TakeReplay() {
		out = append(out, struct {
			Params    coreexec.BlockParams
			Envelopes []*messaging.Envelope
		}{b.params, b.envelopes})
	}
	return out
}
