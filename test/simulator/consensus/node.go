// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Node struct {
	mu          *sync.Mutex
	app         App
	submissions []*messaging.Envelope
	record      Recorder
}

type Recorder interface {
	DidInit(snapshot ioutil.SectionReader) error
	DidExecuteBlock(state execute.BlockState, submissions []*messaging.Envelope) error
}

func NewNode(app App, recorder Recorder) *Node {
	n := new(Node)
	n.mu = new(sync.Mutex)
	n.app = app
	n.record = recorder
	return n
}

func (n *Node) Info(req *InfoRequest) (*InfoResponse, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	return n.app.Info(req)
}

func (n *Node) Check(req *CheckRequest) (*CheckResponse, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	return n.app.Check(req)
}

func (n *Node) Init(req *InitRequest) (*InitResponse, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Initialize the app
	res, err := n.app.Init(req)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Record the snapshot
	if n.record != nil {
		err = n.record.DidInit(req.Snapshot)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("record snapshot: %w", err)
		}
	}

	return res, nil
}

func (n *Node) Begin(req *BeginRequest) (*BeginResponse, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	return n.app.Begin(req)
}

func (n *Node) Deliver(block execute.Block, envelopes []*messaging.Envelope) ([]*protocol.TransactionStatus, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.submissions = append(n.submissions, envelopes...)

	var results []*protocol.TransactionStatus
	for _, envelope := range envelopes {
		s, err := block.Process(envelope)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("deliver envelope: %w", err)
		}

		results = append(results, s...)
	}
	return results, nil
}

func (n *Node) EndBlock(block execute.Block) (execute.BlockState, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	state, err := block.Close()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("end block: %w", err)
	}

	if n.record != nil {
		err = n.record.DidExecuteBlock(state, n.submissions)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("record block: %w", err)
		}
	}

	n.submissions = n.submissions[:0]
	return state, nil
}

func (n *Node) Commit(state execute.BlockState) ([]byte, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Commit (let the app handle empty blocks and notifications)
	err := state.Commit()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("commit: %w", err)
	}

	// Get the old root
	res, err := n.app.Info(&InfoRequest{})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return res.LastHash[:], errors.UnknownError.Wrap(err)
}
