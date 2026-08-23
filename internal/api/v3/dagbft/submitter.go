// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package dagbft provides transaction submission for DAG-BFT consensus.
package dagbft

import (
	"context"
	"errors"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
	pkgerrors "gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Submitter submits transactions to the DAG-BFT consensus node.
// Implements api.Submitter interface.
type Submitter struct {
	logger logging.OptionalLogger
	node   *consensus.Node
}

var _ api.Submitter = (*Submitter)(nil)

// SubmitterParams contains parameters for creating a Submitter.
type SubmitterParams struct {
	Logger logging.Logger
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
	s.logger.Debug("Submit() called", "verify", opts.Verify != nil && *opts.Verify)

	// Verify the envelope is well-formed
	if opts.Verify == nil || *opts.Verify {
		_, err := envelope.Normalize()
		if err != nil {
			s.logger.Error("Submit: envelope normalization failed during verify", "error", err)
			return nil, pkgerrors.BadRequest.WithFormat("verify: %w", err)
		}
	}

	b, err := envelope.MarshalBinary()
	if err != nil {
		s.logger.Error("Submit: envelope marshaling failed", "error", err)
		return nil, pkgerrors.EncodingError.WithFormat("marshal: %w", err)
	}

	s.logger.Debug("Submit: submitting to DAG-BFT node", "envelope_size", len(b))

	// Submit to DAG-BFT node
	err = s.node.SubmitTransaction(b)
	if err != nil {
		if errors.Is(err, worker.ErrBackpressure) {
			return nil, pkgerrors.TooManyRequests.WithFormat("submit: %w", err)
		}
		if errors.Is(err, worker.ErrTransactionTooLarge) {
			// Not retryable and not the node's fault — the envelope can never
			// fit in a batch, and the submitter must hear that, not silence
			// (#4141).
			return nil, pkgerrors.BadRequest.WithFormat("submit: %w", err)
		}
		s.logger.Error("Submit: node submission failed", "error", err)
		return nil, pkgerrors.InternalError.WithFormat("submit: %w", err)
	}

	s.logger.Debug("Submit: node submission successful, creating success result")

	// DAG-BFT submission is asynchronous, return success indication
	// The transaction will be batched and ordered by the consensus protocol
	return s.createSuccessResult(envelope)
}

// createSuccessResult creates a success result for the submitted envelope.
func (s *Submitter) createSuccessResult(envelope *messaging.Envelope) ([]*api.Submission, error) {
	s.logger.Debug("createSuccessResult() called")

	// Get transaction IDs from the envelope
	messages, err := envelope.Normalize()
	if err != nil {
		// Already validated above, but handle gracefully
		// IMPORTANT: Always initialize Status field even in error case
		s.logger.Info("createSuccessResult: envelope normalization failed, returning error submission", "error", err)
		sub := &api.Submission{
			Success: true,
			Message: "Transaction submitted to consensus (envelope normalization error)",
			Status: &protocol.TransactionStatus{
				TxID: nil, // No transaction ID available if normalization fails
				Code: pkgerrors.Pending,
			},
		}
		s.logger.Debug("createSuccessResult: returning [normalization error path]",
			"submission_count", 1,
			"status_code", sub.Status.Code,
			"status_txid", sub.Status.TxID)
		return []*api.Submission{sub}, nil
	}

	s.logger.Debug("createSuccessResult: envelope normalized", "message_count", len(messages))

	results := make([]*api.Submission, 0, len(messages))
	for i, msg := range messages {
		sub := &api.Submission{
			Success: true,
			Message: "Transaction submitted to consensus",
		}

		// Always initialize Status field to prevent nil pointer dereference
		// Get transaction ID based on message type
		var txID *url.TxID
		switch m := msg.(type) {
		case *messaging.TransactionMessage:
			if m.Transaction != nil {
				txID = m.Transaction.ID()
			}
			s.logger.Debug("createSuccessResult: processing TransactionMessage",
				"message_index", i,
				"txid", txID)
		case *messaging.SignatureMessage:
			// For signatures, use the transaction ID they're signing
			txID = m.TxID
			s.logger.Debug("createSuccessResult: processing SignatureMessage",
				"message_index", i,
				"txid", txID)
		default:
			// For other messages, use the message ID
			txID = msg.ID()
			s.logger.Debug("createSuccessResult: processing other message type",
				"message_index", i,
				"message_type", msg.Type(),
				"txid", txID)
		}

		// Always set Status, even if txID is nil (defensive)
		sub.Status = &protocol.TransactionStatus{
			TxID: txID,
			Code: pkgerrors.Pending,
		}

		s.logger.Debug("createSuccessResult: created Submission",
			"message_index", i,
			"status_code", sub.Status.Code,
			"status_txid", sub.Status.TxID,
			"success", sub.Success)

		results = append(results, sub)
	}

	if len(results) == 0 {
		// IMPORTANT: Always initialize Status field
		s.logger.Info("createSuccessResult: no messages in envelope, returning empty submission")
		sub := &api.Submission{
			Success: true,
			Message: "Transaction submitted to consensus (no messages)",
			Status: &protocol.TransactionStatus{
				TxID: nil,
				Code: pkgerrors.Pending,
			},
		}
		s.logger.Debug("createSuccessResult: returning [no messages path]",
			"submission_count", 1,
			"status_code", sub.Status.Code,
			"status_txid", sub.Status.TxID)
		return []*api.Submission{sub}, nil
	}

	// Final safety check: ensure ALL submissions have Status initialized
	for i, sub := range results {
		if sub.Status == nil {
			s.logger.Info("createSuccessResult: found nil Status field, initializing", "index", i)
			results[i].Status = &protocol.TransactionStatus{
				TxID: nil,
				Code: pkgerrors.Pending,
			}
		}
	}

	s.logger.Debug("createSuccessResult: returning [normal path]", "submission_count", len(results))
	for i, sub := range results {
		s.logger.Debug("createSuccessResult: final submission status",
			"index", i,
			"status_code", sub.Status.Code,
			"status_txid", sub.Status.TxID,
			"success", sub.Success)
	}

	return results, nil
}

// The DAGSubmitter type that used to live here — a second submitter with its
// own MaxTxSize check that nothing constructed outside its tests — was
// deleted per #4141. The live ingress is Submitter above, and the size
// refusal lives where every submission funnels: worker.Submit
// (ErrTransactionTooLarge), surfaced here as a BadRequest.
