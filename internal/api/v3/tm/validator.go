// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package tm

import (
	"context"

	"github.com/tendermint/tendermint/libs/log"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"
	"github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

type ValidateClient interface {
	CheckTx(context.Context, types.Tx) (*coretypes.ResultCheckTx, error)
}

type Validator struct {
	logger logging.OptionalLogger
	local  ValidateClient
}

var _ api.Validator = (*Validator)(nil)

type ValidatorParams struct {
	Logger log.Logger
	Local  ValidateClient
}

func NewValidator(params ValidatorParams) *Validator {
	s := new(Validator)
	s.logger.L = params.Logger
	s.local = params.Local
	return s
}

func (s *Validator) Type() api.ServiceType { return api.ServiceTypeValidate }

func (s *Validator) Validate(ctx context.Context, envelope *messaging.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
	// if opts.Full == nil || *opts.Full {
	// 	return nil, errors.NotAllowed.WithFormat("full validation has not been implemented")
	// }

	b, err := envelope.MarshalBinary()
	if err != nil {
		return nil, errors.EncodingError.WithFormat("marshal: %w", err)
	}

	res, err := s.local.CheckTx(ctx, b)
	if err != nil {
		return nil, errors.InternalError.WithFormat("broadcast: %w", err)
	}

	var message string
	switch {
	case len(res.MempoolError) > 0:
		message = res.MempoolError
	case len(res.Log) > 0:
		message = res.Log
	case len(res.Info) > 0:
		message = res.Info
	case res.Code != 0:
		message = "An unknown error occurred"
	}

	return unpackConsensusResult(res.Code, message, res.Data)
}
