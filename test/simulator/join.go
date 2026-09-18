// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	coreexec "gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	execute "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

// joinState is one node's join, as the DAG service keeps it (#4292, #4294):
// while it is joining, the node executes nothing. The blocks it is handed are
// buffered until it has taken a peer's staging, then applied to that staging
// in order, and each new one is applied as it arrives.
//
// The order is the spec's (executor.md, "Sync", step 2) and it is not
// interchangeable: staging comes from a validator as of its block P, and the
// blocks after P are applied to THAT staging. A node that collected into its
// own staging first would have nothing to load into.
type joinState struct {
	mu      sync.Mutex
	joining bool
	staged  bool
	buffer  []joinedBlock
	exec    execute.Executor
}

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

// leave starts the join: the node stops executing and starts keeping what it
// is handed.
func (j *joinState) leave() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.joining, j.staged, j.buffer = true, false, nil
}

// done ends the join: the node executes again, from the next block.
func (j *joinState) done() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.joining, j.staged, j.buffer = false, false, nil
}

// collect keeps a block, and applies it to staging once staging is the peers'.
func (j *joinState) Collect(params coreexec.BlockParams, envelopes []*messaging.Envelope) error {
	j.mu.Lock()
	staged := j.staged
	if !staged {
		j.buffer = append(j.buffer, joinedBlock{params, envelopes})
	}
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

// takeStaging loads a peer's staging and applies every block buffered since
// the node left, in order.
func (j *joinState) takeStaging(snap *private.StagingSnapshot) error {
	loader, ok := j.exec.(interface {
		LoadStaging(*private.StagingSnapshot) error
	})
	if !ok {
		return errors.NotAllowed.With("this executor cannot take staging from a peer")
	}

	j.mu.Lock()
	if !j.joining {
		j.mu.Unlock()
		return errors.NotAllowed.With("this node is not joining")
	}
	if j.staged {
		j.mu.Unlock()
		return errors.NotAllowed.With("staging has already been taken")
	}
	buffered := j.buffer
	j.mu.Unlock()

	err := loader.LoadStaging(snap)
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
