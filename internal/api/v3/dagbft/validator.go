// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

// Validator validates transactions using the executor directly.
type Validator struct {
	logger   logging.OptionalLogger
	executor execute.Executor
}

var _ api.Validator = (*Validator)(nil)

// ValidatorParams contains parameters for creating a Validator.
type ValidatorParams struct {
	Logger   logging.Logger
	Executor execute.Executor
}

// NewValidator creates a new DAG-BFT validator.
func NewValidator(params ValidatorParams) *Validator {
	s := new(Validator)
	s.logger.L = params.Logger
	s.executor = params.Executor
	return s
}

// Type returns the service type.
func (s *Validator) Type() api.ServiceType { return api.ServiceTypeValidate }

// Validate validates an envelope using the executor.
func (s *Validator) Validate(ctx context.Context, envelope *messaging.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
	// Validate using executor directly
	statuses, err := s.executor.Validate(envelope, false)
	if err != nil {
		return nil, errors.InternalError.WithFormat("validate: %w", err)
	}

	// Convert statuses to submission results
	if len(statuses) == 0 {
		return []*api.Submission{{
			Success: true,
			Message: "Validation successful",
		}}, nil
	}

	results := make([]*api.Submission, len(statuses))
	for i, status := range statuses {
		sub := &api.Submission{
			Success: status.Error == nil,
			Status:  status,
		}

		if status.Error != nil {
			sub.Message = status.Error.Error()
		} else {
			sub.Message = "Validation successful"
		}

		results[i] = sub
	}

	return results, nil
}
