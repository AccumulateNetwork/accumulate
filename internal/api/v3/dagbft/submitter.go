// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Submitter submits transactions to the DAG-BFT consensus node.
type Submitter struct {
	logger logging.OptionalLogger
	node   *consensus.Node
}

var _ api.Submitter = (*Submitter)(nil)

// SubmitterParams contains parameters for creating a Submitter.
type SubmitterParams struct {
	Logger log.Logger
	Node   *consensus.Node
}

// NewSubmitter creates a new DAG-BFT submitter.
func NewSubmitter(params SubmitterParams) *Submitter {
	s := new(Submitter)
	s.logger.L = params.Logger
	s.node = params.Node
	return s
}

// Type returns the service type.
func (s *Submitter) Type() api.ServiceType { return api.ServiceTypeSubmit }

// Submit submits an envelope to the DAG-BFT consensus node.
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

	// Submit to DAG-BFT node
	err = s.node.SubmitTransaction(b)
	if err != nil {
		return nil, errors.InternalError.WithFormat("submit: %w", err)
	}

	// DAG-BFT submission is asynchronous, return success indication
	// The transaction will be batched and ordered by the consensus protocol
	return createSuccessResult(envelope)
}

// createSuccessResult creates a success result for the submitted envelope.
func createSuccessResult(envelope *messaging.Envelope) ([]*api.Submission, error) {
	// Get transaction IDs from the envelope
	messages, err := envelope.Normalize()
	if err != nil {
		// Already validated above, but handle gracefully
		return []*api.Submission{{
			Success: true,
			Message: "Transaction submitted to consensus",
		}}, nil
	}

	results := make([]*api.Submission, 0, len(messages))
	for _, msg := range messages {
		sub := &api.Submission{
			Success: true,
			Message: "Transaction submitted to consensus",
		}

		// Try to get transaction status info
		if txMsg, ok := msg.(*messaging.TransactionMessage); ok && txMsg.Transaction != nil {
			sub.Status = &protocol.TransactionStatus{
				TxID: txMsg.Transaction.ID(),
				Code: errors.Pending,
			}
		}

		results = append(results, sub)
	}

	if len(results) == 0 {
		return []*api.Submission{{
			Success: true,
			Message: "Transaction submitted to consensus",
		}}, nil
	}

	return results, nil
}
