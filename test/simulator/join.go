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
// executes nothing. Every block it is handed is buffered, and once
// ApplyStaging has run each one is also taken into the node's own staging —
// the buffered ones in order, then each new one as it arrives. Handoff leaves
// collecting mode at the block the pulled state is and produces the buffered
// blocks after it, as the DAG service's handoff does (dagbft/collect.go).
//
// No peer's staging is loaded (#4362): staging is what this node collected
// from consensus, minus what the pulled state says executed.
type joinState struct {
	mu      sync.Mutex
	joining bool
	staged  bool
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
	j.joining, j.staged, j.buffer = true, false, nil
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
	j.joining, j.staged, j.buffer = true, false, nil
}

// Collect keeps a block, and takes it into staging once ApplyStaging has run.
func (j *joinState) Collect(params coreexec.BlockParams, envelopes []*messaging.Envelope) error {
	j.mu.Lock()
	j.buffer = append(j.buffer, joinedBlock{params, envelopes})
	staged := j.staged
	j.mu.Unlock()
	if !staged {
		return nil
	}
	return j.apply(params, envelopes)
}

func (j *joinState) apply(params coreexec.BlockParams, envelopes []*messaging.Envelope) error {
	col, ok := j.exec.(blockCollector)
	if !ok {
		return errors.NotAllowed.With("this executor cannot collect a block without executing it")
	}
	_, err := col.CollectCommittedBlock(params, envelopes)
	return errors.UnknownError.Wrap(err)
}

// ApplyStaging implements [join.Buffer]: it runs load, then takes every block
// buffered since the node left into staging, in order, and every one that
// arrives after.
func (j *joinState) ApplyStaging(load func() error) error {
	j.mu.Lock()
	if !j.joining {
		j.mu.Unlock()
		return errors.NotAllowed.With("this node is not joining")
	}
	if j.staged {
		j.mu.Unlock()
		return errors.NotAllowed.With("staging has already been applied")
	}
	buffered := j.buffer
	j.mu.Unlock()

	err := load()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	for _, b := range buffered {
		err := j.apply(b.params, b.envelopes)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	j.mu.Lock()
	defer j.mu.Unlock()
	j.staged = true
	return nil
}

// Handoff implements [join.Buffer]: it leaves collecting mode at block q and
// produces every buffered block after q, in order. The blocks at or below q
// are in the state the pull put there and are not produced again; a q past
// the last block collected is not ready, because the blocks up to it have not
// been handed to this node yet.
func (j *joinState) Handoff(q uint64) error {
	j.mu.Lock()
	if !j.joining {
		j.mu.Unlock()
		return errors.NotAllowed.With("this node is not joining")
	}
	var from uint64
	if len(j.buffer) > 0 {
		from = j.buffer[0].params.Index - 1
	} else {
		last, _, err := j.exec.LastBlock()
		if err != nil {
			j.mu.Unlock()
			return errors.UnknownError.WithFormat("read the last block: %w", err)
		}
		from = last.Index
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
	j.joining, j.staged, j.buffer = false, false, nil
	j.mu.Unlock()

	if j.produce == nil {
		return errors.NotAllowed.With("this node cannot produce a block")
	}
	for _, b := range blocks {
		err := j.produce(b.params, b.envelopes)
		if err != nil {
			return errors.UnknownError.WithFormat("produce buffered block %d: %w", b.params.Index, err)
		}
	}
	return nil
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
