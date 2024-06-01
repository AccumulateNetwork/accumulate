// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"context"
	"encoding/json"
	"io"

	"github.com/cometbft/cometbft/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/abci"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type AbciApp abci.Accumulator

func (a *AbciApp) Info(*InfoRequest) (*InfoResponse, error) {
	last, hash, err := (*abci.Accumulator)(a).LastBlock()
	if err != nil {
		return nil, err
	}
	return &InfoResponse{
		LastBlock: last,
		LastHash:  hash,
	}, nil
}

func (a *AbciApp) Check(req *CheckRequest) (*CheckResponse, error) {
	tx, err := req.Envelope.MarshalBinary()
	if err != nil {
		return nil, err
	}

	typ := types.CheckTxType_Recheck
	if req.New {
		typ = types.CheckTxType_New
	}

	res, err := (*abci.Accumulator)(a).CheckTx(context.TODO(), &types.RequestCheckTx{Tx: tx, Type: typ})
	if err != nil {
		return nil, err
	}
	if res.Code != 0 {
		return nil, errors.UnknownError.With(res.Log)
	}

	rs := new(protocol.TransactionResultSet)
	err = rs.UnmarshalBinary(res.Data)
	if err != nil {
		return nil, err
	}

	return &CheckResponse{
		Results: rs.Results,
	}, nil
}

func (a *AbciApp) Init(req *InitRequest) (*InitResponse, error) {
	snap, err := io.ReadAll(req.Snapshot)
	if err != nil {
		return nil, err
	}

	snap, err = json.Marshal(snap)
	if err != nil {
		return nil, err
	}

	res, err := (*abci.Accumulator)(a).InitChain(context.TODO(), &types.RequestInitChain{
		AppStateBytes: snap,
	})
	if err != nil {
		return nil, err
	}

	return &InitResponse{
		Hash: res.AppHash,
	}, nil
}

func (a *AbciApp) Execute(req *ExecuteRequest) (*ExecuteResponse, error) {
	// Make sure that late commit is disabled
	a.DisableLateCommit = true

	var req2 types.RequestFinalizeBlock
	req2.Height = int64(req.Params.Index)
	req2.Time = req.Params.Time
	req2.Misbehavior = req.Params.Evidence

	if req.Params.IsLeader {
		req2.ProposerAddress = a.Address
	}
	if req.Params.CommitInfo != nil {
		req2.DecidedLastCommit = *req.Params.CommitInfo
	}

	var err error
	req2.Txs = make([][]byte, len(req.Envelopes))
	for i, env := range req.Envelopes {
		req2.Txs[i], err = env.MarshalBinary()
		if err != nil {
			return nil, err
		}
	}

	res3, err := (*abci.Accumulator)(a).FinalizeBlock(context.Background(), &req2)
	if err != nil {
		return nil, err
	}

	var results []*protocol.TransactionStatus
	for _, r := range res3.TxResults {
		rs := new(protocol.TransactionResultSet)
		err = rs.UnmarshalBinary(r.Data)
		if err != nil {
			return nil, err
		}
		results = append(results, rs.Results...)
	}

	return &ExecuteResponse{
		Results: results,
		Block: &abciBlockState{
			hash: [32]byte(res3.AppHash),
		},
	}, nil
}

func (a *AbciApp) Commit(req *CommitRequest) (*CommitResponse, error) {
	_, err := (*abci.Accumulator)(a).Commit(context.Background(), &types.RequestCommit{})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	b := req.Block.(*abciBlockState)
	return &CommitResponse{
		Hash: b.hash,
	}, nil
}

type abciBlockState struct {
	hash [32]byte
}
