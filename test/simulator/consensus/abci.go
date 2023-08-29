// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/tendermint/tendermint/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/abci"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
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

	res := (*abci.Accumulator)(a).CheckTx(types.RequestCheckTx{Tx: tx, Type: typ})
	if res.Code != 0 {
		return nil, fmt.Errorf("code %d, log %s, info %s", res.Code, res.Log, res.Info)
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

	res := (*abci.Accumulator)(a).InitChain(types.RequestInitChain{
		AppStateBytes: snap,
	})

	return &InitResponse{
		Hash: res.AppHash,
	}, nil
}

func (a *AbciApp) Begin(req *BeginRequest) (*BeginResponse, error) {
	var req2 types.RequestBeginBlock
	req2.Header.Height = int64(req.Params.Index)
	req2.Header.Time = req.Params.Time
	req2.ByzantineValidators = req.Params.Evidence

	if req.Params.IsLeader {
		req2.Header.ProposerAddress = a.Address
	}
	if req.Params.CommitInfo != nil {
		req2.LastCommitInfo = *req.Params.CommitInfo
	}

	(*abci.Accumulator)(a).BeginBlock(req2)
	return &BeginResponse{Block: (*AbciBlock)(a)}, nil
}

type AbciBlock abci.Accumulator

func (a *AbciBlock) Params() execute.BlockParams {
	return (*abci.Accumulator)(a).CurrentBlock().Params()
}

func (a *AbciBlock) Process(env *messaging.Envelope) ([]*protocol.TransactionStatus, error) {
	tx, err := env.MarshalBinary()
	if err != nil {
		return nil, err
	}

	res := (*abci.Accumulator)(a).DeliverTx(types.RequestDeliverTx{Tx: tx})
	if res.Code != 0 {
		return nil, fmt.Errorf("code %d, log %s, info %s", res.Code, res.Log, res.Info)
	}

	rs := new(protocol.TransactionResultSet)
	err = rs.UnmarshalBinary(res.Data)
	if err != nil {
		return nil, err
	}

	return rs.Results, nil
}

func (a *AbciBlock) Close() (execute.BlockState, error) {
	b := (*abci.Accumulator)(a)
	b.EndBlock(types.RequestEndBlock{})
	return &AbciBlockState{b, b.CurrentBlockState()}, nil
}

type AbciBlockState struct {
	abci  *abci.Accumulator
	block execute.BlockState
}

func (a *AbciBlockState) Params() execute.BlockParams {
	return a.block.Params()
}

func (a *AbciBlockState) IsEmpty() bool {
	return a.block.IsEmpty()
}

func (a *AbciBlockState) DidCompleteMajorBlock() (uint64, time.Time, bool) {
	return a.block.DidCompleteMajorBlock()
}

func (a *AbciBlockState) DidUpdateValidators() ([]*execute.ValidatorUpdate, bool) {
	return a.block.DidUpdateValidators()
}

func (a *AbciBlockState) ChangeSet() record.Record {
	return a.block.ChangeSet()
}

func (a *AbciBlockState) Commit() error {
	a.abci.Commit()
	return nil
}

func (a *AbciBlockState) Discard() {
	panic("not supported")
}
