// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type App interface {
	Info(*InfoRequest) (*InfoResponse, error)
	Check(*CheckRequest) (*CheckResponse, error)
	Init(*InitRequest) (*InitResponse, error)
	Execute(*ExecuteRequest) (*ExecuteResponse, error)
	Commit(*CommitRequest) (*CommitResponse, error)
}

type InfoRequest struct{}

type InfoResponse struct {
	LastBlock *execute.BlockParams
	LastHash  [32]byte
}

type CheckRequest struct {
	Context  context.Context
	Envelope *messaging.Envelope
	New      bool
}

type CheckResponse struct {
	Results []*protocol.TransactionStatus
}

type InitRequest struct {
	Snapshot   ioutil.SectionReader
	Validators []*execute.ValidatorUpdate
}

type InitResponse struct {
	Hash       []byte
	Validators []*execute.ValidatorUpdate
}

type ExecuteRequest struct {
	Params    execute.BlockParams
	Envelopes []*messaging.Envelope
}

type ExecuteResponse struct {
	Block   any
	Results []*protocol.TransactionStatus
	Updates []*execute.ValidatorUpdate
}

type CommitRequest struct {
	Block any
}

type CommitResponse struct {
	Hash [32]byte
}

type ExecutorApp struct {
	Executor execute.Executor
	Restore  RestoreFunc
	EventBus *events.Bus
	Record   Recorder

	// Join, when it is set and says the node is joining, takes the blocks
	// this node is handed instead of executing them: a node that has left and
	// is coming back (executor spec, "Sync"; #4294). Collect keeps the block
	// in the join's buffer and nothing more, exactly as the DAG service does:
	// the join takes the buffer into staging only through the block after the
	// state it proves (#4398).
	Join Joining
}

// Joining is a node's join, as the DAG service keeps it: whether the node is
// joining, and what it does with a block while it is.
type Joining interface {
	Joining() bool
	Collect(execute.BlockParams, []*messaging.Envelope) error
}

type RestoreFunc func(ioutil.SectionReader) error

func (a *ExecutorApp) SetRecorder(rec Recorder) {
	a.Record = rec
}

func (a *ExecutorApp) Info(*InfoRequest) (*InfoResponse, error) {
	last, hash, err := a.Executor.LastBlock()
	if err != nil {
		return nil, err
	}
	return &InfoResponse{
		LastBlock: last,
		LastHash:  hash,
	}, nil
}

func (a *ExecutorApp) Check(req *CheckRequest) (*CheckResponse, error) {
	// Copy to avoid interference between nodes
	res, err := a.Executor.Validate(req.Envelope.Copy(), !req.New)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("check messages: %w", err)
	}

	return &CheckResponse{Results: res}, nil
}

func (a *ExecutorApp) Init(req *InitRequest) (*InitResponse, error) {
	// Check if initialization is required
	_, root, err := a.Executor.LastBlock()
	switch {
	case err == nil:
		return &InitResponse{Hash: root[:]}, nil
	case errors.Is(err, errors.NotFound):
		// Ok
	default:
		return nil, errors.UnknownError.WithFormat("load state root: %w", err)
	}

	// Restore the snapshot
	err = a.Restore(req.Snapshot)
	// err = snapshot.FullRestore(a.Database, req.Snapshot, nil, a.Describe.PartitionUrl())
	if err != nil {
		return nil, errors.UnknownError.WithFormat("restore snapshot: %w", err)
	}

	// Initialize the executor
	val, err := a.Executor.Init(req.Validators)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("restore snapshot: %w", err)
	}

	_, root, err = a.Executor.LastBlock()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load state root: %w", err)
	}
	return &InitResponse{Hash: root[:], Validators: val}, nil
}

// collected marks a block this node took into staging without executing it.
// Commit does nothing with one: nothing was executed, so there is nothing to
// commit and no hash to report but the one the node already stands at.
type collected struct{}

func (a *ExecutorApp) Execute(req *ExecuteRequest) (*ExecuteResponse, error) {
	// A joining node keeps the block and executes nothing (executor spec,
	// "Sync", step 1; #4292).
	if a.Join != nil && a.Join.Joining() {
		err := a.Join.Collect(req.Params, copyEnv(req.Envelopes))
		if err != nil {
			return nil, errors.UnknownError.WithFormat("collect block: %w", err)
		}
		return &ExecuteResponse{Block: collected{}}, nil
	}
	return a.execute(req.Params, req.Envelopes)
}

// Produce executes and commits one block exactly as Execute and Commit do for
// a block consensus hands the node: what a node that has joined does with the
// blocks it buffered past the state it pulled (join.Buffer's Handoff).
func (a *ExecutorApp) Produce(params execute.BlockParams, envelopes []*messaging.Envelope) error {
	res, err := a.execute(params, envelopes)
	if err != nil {
		return err
	}
	_, err = a.Commit(&CommitRequest{Block: res.Block})
	return err
}

func (a *ExecutorApp) execute(params execute.BlockParams, envs []*messaging.Envelope) (*ExecuteResponse, error) {
	block, err := a.Executor.Begin(params)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("begin block: %w", err)
	}

	// Copy to avoid interference between nodes
	envelopes := make([]*messaging.Envelope, len(envs))
	for i, envelope := range envs {
		envelopes[i] = envelope.Copy()
	}

	// Process the block's envelopes as a set, so execution sharding (#4145)
	// is exercised under the simulator exactly as it is under DAG-BFT. With
	// shard count <= 1 this is identical to a loop over Process.
	var results []*protocol.TransactionStatus
	if pb, ok := block.(execute.ParallelBlock); ok {
		for _, r := range pb.ProcessAll(envelopes) {
			if r.Error != nil {
				return nil, errors.UnknownError.WithFormat("deliver envelope: %w", r.Error)
			}
			results = append(results, r.Statuses...)
		}
	} else {
		for _, envelope := range envelopes {
			s, err := block.Process(envelope)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("deliver envelope: %w", err)
			}
			results = append(results, s...)
		}
	}

	state, err := block.Close()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("end block: %w", err)
	}

	valUp, _ := state.DidUpdateValidators()
	return &ExecuteResponse{
		Results: results,
		Block:   state,
		Updates: valUp,
	}, nil
}

func (a *ExecutorApp) Commit(req *CommitRequest) (*CommitResponse, error) {
	if _, ok := req.Block.(collected); ok {
		_, hash, err := a.Executor.LastBlock()
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		return &CommitResponse{Hash: hash}, nil
	}

	s := req.Block.(execute.BlockState)

	// An empty block still commits: the block's Commit discards the batch
	// itself, and commits the cache and staging transactions the block
	// began, as the node does. Discarding here left the cache's newest block
	// and staging's releases behind on quiet partitions.
	err := s.Commit()
	if s.IsEmpty() {
		return &CommitResponse{}, errors.UnknownError.Wrap(err)
	}
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	major, _, _ := s.DidCompleteMajorBlock()
	err = a.EventBus.Publish(events.DidCommitBlock{
		Index: s.Params().Index,
		Time:  s.Params().Time,
		Major: major,
	})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("notify of commit: %w", err)
	}

	hash, err := s.Hash()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	if a.Record != nil {
		err = a.Record.DidCommitBlock(s)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	return &CommitResponse{
		Hash: hash,
	}, nil
}
