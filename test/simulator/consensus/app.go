// Copyright 2024 The Accumulate Authors
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

func (a *ExecutorApp) Execute(req *ExecuteRequest) (*ExecuteResponse, error) {
	block, err := a.Executor.Begin(req.Params)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("begin block: %w", err)
	}

	var results []*protocol.TransactionStatus
	for _, envelope := range req.Envelopes {
		// Copy to avoid interference between nodes
		s, err := block.Process(envelope.Copy())
		if err != nil {
			return nil, errors.UnknownError.WithFormat("deliver envelope: %w", err)
		}

		results = append(results, s...)
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
	s := req.Block.(execute.BlockState)

	// Discard changes if the block is empty
	if s.IsEmpty() {
		s.Discard()
		return &CommitResponse{}, nil
	}

	err := s.Commit()
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
