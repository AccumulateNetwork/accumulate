// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"sync"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	coreexec "gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	execute "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

// joinState is one node's join, as the DAG service keeps it (#4292, #4294):
// it is the simulator's [join.Buffer]. While the node is collecting it
// executes nothing, and every block it is handed is buffered. StageThrough
// takes the buffered blocks through the one after the state the join proved
// into the node's own staging, and none after it; Handoff leaves collecting
// mode at the block the pulled state is and produces the buffered blocks
// after it, as the DAG service does (dagbft/collect.go). The blocks after
// Q + 1 reach staging only by being produced, as they reach a peer's (#4398).
//
// No peer's staging is loaded (#4362): staging is what this node collected
// from consensus, minus what the pulled state says executed.
type joinState struct {
	mu      sync.Mutex
	joining bool
	buffer  []joinedBlock
	exec    execute.Executor

	// produce executes and commits one block as the consensus app does for
	// a block it is handed: what Handoff does with what it buffered.
	produce func(coreexec.BlockParams, []*messaging.Envelope) error
}

var _ join.Buffer = (*joinState)(nil)

type joinedBlock struct {
	params    coreexec.BlockParams
	envelopes []*messaging.Envelope
	staged    bool // taken into staging by StageThrough
}

// A blockCollector is an executor that can take a committed block into staging
// without executing it.
type blockCollector interface {
	CollectCommittedBlock(coreexec.BlockParams, []*messaging.Envelope) (*coreexec.CollectedBlock, error)
}

func (j *joinState) Joining() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.joining
}

// Collecting implements [join.Buffer].
func (j *joinState) Collecting() bool { return j.Joining() }

// BufferOverrun implements [join.Buffer]. The simulator's buffer is not
// bounded: every node is handed every block.
func (j *joinState) BufferOverrun() bool { return false }

// leave starts the join: the node stops executing and starts keeping what it
// is handed.
func (j *joinState) leave() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.joining, j.buffer = true, nil
}

// StartCollecting implements [join.Buffer]. A node that is already
// collecting keeps its buffer: starting again would lose the blocks it has
// kept since it left (#4351).
func (j *joinState) StartCollecting() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.joining {
		return
	}
	j.joining, j.buffer = true, nil
}

// Collect keeps a block. It does not take it into staging: StageThrough does
// that, through the block after the state the join proves, and no further.
func (j *joinState) Collect(params coreexec.BlockParams, envelopes []*messaging.Envelope) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.buffer = append(j.buffer, joinedBlock{params: params, envelopes: envelopes})
	return nil
}

func (j *joinState) apply(params coreexec.BlockParams, envelopes []*messaging.Envelope) error {
	col, ok := j.exec.(blockCollector)
	if !ok {
		return errors.NotAllowed.With("this executor cannot collect a block without executing it")
	}
	_, err := col.CollectCommittedBlock(params, envelopes)
	return errors.UnknownError.Wrap(err)
}

// StageThrough implements [join.Buffer]: it takes into staging, in order,
// every buffered block whose index is at most block and that is not in
// staging yet, and none after it. The simulator's blocks carry their index,
// so block is found by it rather than by the leader round the DAG service
// reads (DIFFERENCES E11). NotReady: block has not been collected yet;
// Conflict: block is at or before the block this node stood at when it
// started collecting.
func (j *joinState) StageThrough(block uint64) error {
	j.mu.Lock()
	if !j.joining {
		j.mu.Unlock()
		return errors.NotAllowed.With("this node is not joining")
	}
	from, err := j.from()
	if err != nil {
		j.mu.Unlock()
		return err
	}
	if block <= from {
		j.mu.Unlock()
		return errors.Conflict.WithFormat("cannot stage through block %d, at or behind the block %d this node stood at", block, from)
	}
	through := int(block - from)
	if through > len(j.buffer) {
		j.mu.Unlock()
		return errors.NotReady.WithFormat("block %d has not been collected: only %d blocks have been since %d", block, len(j.buffer), from)
	}
	// By position: the buffer only grows while the node is joining, so a
	// position names the same block after Collect has appended.
	var take []int
	for i := range j.buffer[:through] {
		if !j.buffer[i].staged {
			take = append(take, i)
		}
	}
	blocks := append([]joinedBlock(nil), j.buffer[:through]...)
	j.mu.Unlock()

	for _, i := range take {
		err := j.apply(blocks[i].params, blocks[i].envelopes)
		if err != nil {
			return errors.UnknownError.WithFormat("stage buffered block %d: %w", blocks[i].params.Index, err)
		}
		j.mu.Lock()
		j.buffer[i].staged = true
		j.mu.Unlock()
	}
	return nil
}

// from is the block this node stood at when it started collecting: the one
// before the first buffered block, or its last block when nothing is
// buffered. It is called with the lock held.
func (j *joinState) from() (uint64, error) {
	if len(j.buffer) > 0 {
		return j.buffer[0].params.Index - 1, nil
	}
	last, _, err := j.exec.LastBlock()
	if err != nil {
		return 0, errors.UnknownError.WithFormat("read the last block: %w", err)
	}
	return last.Index, nil
}

// Handoff implements [join.Buffer]: it leaves collecting mode at block q and
// produces every buffered block after q, in order. The blocks at or below q
// are in the state the pull put there and are not produced again; a q past
// the last block collected is not ready, because the blocks up to it have not
// been handed to this node yet. A block that fails to produce puts the node
// back into collecting mode holding it and the blocks after it, as the DAG
// service does (#4401).
func (j *joinState) Handoff(q uint64) error {
	j.mu.Lock()
	if !j.joining {
		j.mu.Unlock()
		return errors.NotAllowed.With("this node is not joining")
	}
	from, err := j.from()
	if err != nil {
		j.mu.Unlock()
		return err
	}
	if q < from {
		j.mu.Unlock()
		return errors.Conflict.WithFormat("cannot hand off at block %d, behind the block %d this node stood at", q, from)
	}
	skip := q - from
	if skip > uint64(len(j.buffer)) {
		j.mu.Unlock()
		return errors.NotReady.WithFormat("the state is block %d and only %d blocks have been collected since %d", q, len(j.buffer), from)
	}
	blocks := j.buffer[skip:]
	if j.produce == nil {
		j.mu.Unlock()
		return errors.NotAllowed.With("this node cannot produce a block")
	}
	j.joining, j.buffer = false, nil
	j.mu.Unlock()

	for i, b := range blocks {
		err := j.produce(b.params, b.envelopes)
		if err != nil {
			j.mu.Lock()
			j.joining, j.buffer = true, append([]joinedBlock(nil), blocks[i:]...)
			j.mu.Unlock()
			return errors.UnknownError.WithFormat("produce buffered block %d: %w", b.params.Index, err)
		}
	}
	return nil
}

// Resume implements [join.Buffer]: it leaves collecting mode where this node
// stood when it started collecting and produces every block it buffered, as
// the DAG service does from the position its checkpoint restored (#4447).
func (j *joinState) Resume() error {
	j.mu.Lock()
	if !j.joining {
		j.mu.Unlock()
		return errors.NotAllowed.With("this node is not joining")
	}
	from, err := j.from()
	j.mu.Unlock()
	if err != nil {
		return err
	}
	return j.Handoff(from)
}

// nodeState is the join state a node's API services refuse by, as
// cmd/accumulated/run/dagbft.go hands them the join's machine (#4295, #4363).
//
// The daemon decides at startup whether it joins and builds its services with
// that answer: nodestate.Always for a node that did not, the join's own
// nodestate.Machine for one that did. A simulator node is built once and
// "restarts" later (RestartNode), so the answer is held here and set when the
// join starts; the querier asks it, and it asks the join's machine. What a
// request is refused by is therefore the production machine, promoted by the
// production tracker, read by the production querier's servingFor — the
// simulator only supplies the moment the daemon would have wired it.
type nodeState struct {
	machine atomic.Pointer[nodestate.Machine]
}

var _ nodestate.Serving = (*nodeState)(nil)

// CanServeCurrent implements [nodestate.Serving]. A node with no join serves,
// as nodestate.Always does.
func (s *nodeState) CanServeCurrent() bool {
	m := s.machine.Load()
	return m == nil || m.CanServeCurrent()
}
