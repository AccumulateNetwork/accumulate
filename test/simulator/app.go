// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Application interface {
	Info(*InfoRequest) (*InfoResponse, error)
	Check(*CheckRequest) (*CheckResponse, error)
	Init(*InitRequest) (*InitResponse, error)
	Begin(*BeginRequest) (*BeginResponse, error)
}

type InfoRequest struct{}

type InfoResponse struct {
	LastBlock *execute.BlockParams
	LastHash  [32]byte
}

type CheckRequest struct {
	Envelope *messaging.Envelope
	New      bool
}

type CheckResponse struct {
	Results []*protocol.TransactionStatus
}

type InitRequest struct {
	Snapshot ioutil2.SectionReader
}

type InitResponse struct {
	Hash []byte
}

type BeginRequest struct {
	Params execute.BlockParams
}

type BeginResponse struct {
	Block execute.Block
}

type ExecutorApp struct {
	Executor execute.Executor
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
	_, err = a.Executor.Restore(req.Snapshot, nil)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("restore snapshot: %w", err)
	}

	_, root, err = a.Executor.LastBlock()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load state root: %w", err)
	}
	return &InitResponse{Hash: root[:]}, nil
}

func (a *ExecutorApp) Begin(req *BeginRequest) (*BeginResponse, error) {
	res, err := a.Executor.Begin(req.Params)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("begin block: %w", err)
	}
	return &BeginResponse{Block: ExecutorBlock{res}}, nil
}

type ExecutorBlock struct {
	execute.Block
}

func (b ExecutorBlock) Process(envelope *messaging.Envelope) ([]*protocol.TransactionStatus, error) {
	// Copy to avoid interference between nodes
	return b.Block.Process(envelope.Copy())
}
