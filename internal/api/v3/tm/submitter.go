// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package tm

import (
	"context"
	"fmt"

	"github.com/tendermint/tendermint/libs/log"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"
	"github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SubmitClient interface {
	BroadcastTxAsync(context.Context, types.Tx) (*coretypes.ResultBroadcastTx, error)
	BroadcastTxSync(context.Context, types.Tx) (*coretypes.ResultBroadcastTx, error)
}

type Submitter struct {
	logger logging.OptionalLogger
	local  SubmitClient
}

var _ api.Submitter = (*Submitter)(nil)

type SubmitterParams struct {
	Logger log.Logger
	Local  SubmitClient
}

func NewSubmitter(params SubmitterParams) *Submitter {
	s := new(Submitter)
	s.logger.L = params.Logger
	s.local = params.Local
	return s
}

func (s *Submitter) Type() api.ServiceType { return api.ServiceTypeSubmit }

func (s *Submitter) Submit(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	// Verify the envelope is well-formed
	if opts.Verify == nil || *opts.Verify {
		_, err := envelope.Normalize()
		if err != nil {
			return nil, errors.BadRequest.WithFormat("verify: %w", err)
		}
	}

	b, err := envelope.MarshalBinary()
	if err != nil {
		return nil, errors.EncodingError.WithFormat("marshal: %w", err)
	}

	var res *coretypes.ResultBroadcastTx
	if opts.Wait == nil || *opts.Wait {
		res, err = s.local.BroadcastTxSync(ctx, b)
	} else {
		res, err = s.local.BroadcastTxAsync(ctx, b)
	}
	if err != nil {
		return nil, errors.InternalError.WithFormat("broadcast: %w", err)
	}

	var message string
	switch {
	case len(res.Log) > 0:
		message = res.Log
	case res.Code != 0:
		message = "An unknown error occurred"
	}

	return unpackConsensusResult(res.Code, message, res.Data)
}

func unpackConsensusResult(code uint32, message string, data []byte) ([]*api.Submission, error) {
	rs := new(protocol.TransactionResultSet)
	err := rs.UnmarshalBinary(data)
	if err != nil {
		// If the code is zero there's no error but we should still tell the
		// user why statuses are missing
		if code == 0 {
			message = fmt.Sprintf("cannot unmarshal response from consensus engine: %v", err)
		}
		return []*api.Submission{{Success: code == 0, Message: message}}, nil
	}

	// This should not happen but if it does tell the user
	if len(rs.Results) == 0 {
		if code == 0 {
			message = "empty result set"
		}
		return []*api.Submission{{Success: code == 0, Message: message}}, nil
	}

	r := make([]*api.Submission, len(rs.Results))
	for i, status := range rs.Results {
		s := new(api.Submission)
		s.Success = code == 0
		s.Message = message
		s.Status = status
		r[i] = s
	}
	return r, nil
}
