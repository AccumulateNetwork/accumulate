// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

// BridgeApp executes a block the way a DAG-BFT node does (#4142, part b): the
// block's envelopes become the batches of a certificate and
// [adapter.ExecutorBridge.ProduceBlock] hands them to the executor, which
// begins, processes, closes and commits the block in that one call. The
// lockstep node above it still orders the blocks and compares every node's
// results and hash.
//
// What ProduceBlock does not return is not available here: the per-message
// statuses (so no node reports any, and they agree on that), the block's
// validator updates, and the closed block state a recorder takes.
type BridgeApp struct {
	*ExecutorApp
	Bridge *adapter.ExecutorBridge

	// Observe, if set, sees the params of every block handed to ProduceBlock.
	Observe func(adapter.BlockParams)
}

// bridgeBlock is what Execute leaves for Commit: the block is committed
// already, so Commit only reports it.
type bridgeBlock struct {
	params execute.BlockParams
	hash   [32]byte
}

func (a *BridgeApp) Execute(req *ExecuteRequest) (*ExecuteResponse, error) {
	// A joining node collects the block into staging and executes nothing,
	// as under DAG-BFT (CollectBlock).
	if a.Join != nil && a.Join.Joining() {
		return a.ExecutorApp.Execute(req)
	}
	return a.execute(req.Params, req.Envelopes)
}

// Produce executes and commits one block, as Execute and Commit do for a
// block consensus hands the node (join.Buffer's Handoff).
func (a *BridgeApp) Produce(params execute.BlockParams, envelopes []*messaging.Envelope) error {
	res, err := a.execute(params, envelopes)
	if err != nil {
		return err
	}
	_, err = a.Commit(&CommitRequest{Block: res.Block})
	return err
}

func (a *BridgeApp) execute(params execute.BlockParams, envs []*messaging.Envelope) (*ExecuteResponse, error) {
	// One batch per envelope, in the block's order: the certificate's
	// canonical payload order is the order the executor sees (#4054). Copy to
	// avoid interference between nodes.
	var batches []*types.Batch
	for _, env := range envs {
		b, err := env.Copy().MarshalBinary()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("marshal envelope: %w", err)
		}
		batches = append(batches, types.NewBatch([][]byte{b}))
	}

	bp := adapter.BlockParams{
		Index:       params.Index,
		Time:        params.Time,
		IsLeader:    params.IsLeader,
		LeaderRound: types.Round(params.Index),
		Batches:     batches,
	}
	if a.Observe != nil {
		a.Observe(bp)
	}

	hash, err := a.Bridge.ProduceBlock(params.Context, bp)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("produce block: %w", err)
	}
	return &ExecuteResponse{Block: bridgeBlock{params, hash}}, nil
}

func (a *BridgeApp) Commit(req *CommitRequest) (*CommitResponse, error) {
	b, ok := req.Block.(bridgeBlock)
	if !ok {
		return a.ExecutorApp.Commit(req)
	}

	// Every block is announced, an empty one too, as the DAG service does.
	e := events.DidCommitBlock{Index: b.params.Index, Time: b.params.Time}
	if major, _, ok := a.Bridge.LastMajorBlock(); ok {
		e.Major = major
	}
	err := a.EventBus.Publish(e)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("notify of commit: %w", err)
	}
	return &CommitResponse{Hash: b.hash}, nil
}
