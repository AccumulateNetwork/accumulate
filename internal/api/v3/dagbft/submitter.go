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
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/txtracker"
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

// BroadcastMode specifies how to handle transaction submission.
type BroadcastMode int

const (
	// BroadcastModeAsync returns immediately after submission to the worker.
	BroadcastModeAsync BroadcastMode = iota

	// BroadcastModeSync waits for batch inclusion before returning.
	BroadcastModeSync

	// BroadcastModeCommit waits for block commit before returning.
	BroadcastModeCommit
)

// ErrSubmitterClosed is returned when operations are attempted on a closed submitter.
var ErrSubmitterClosed = errors.New("submitter is closed")

// ErrSubmitterNotStarted is returned when operations require the submitter to be started.
var ErrSubmitterNotStarted = errors.New("submitter not started")

// ErrTransactionTooLarge is returned when a transaction exceeds size limits.
var ErrTransactionTooLarge = errors.New("transaction too large")

// ErrValidationFailed is returned when pre-validation fails.
var ErrValidationFailed = errors.New("transaction validation failed")

// Default configuration values.
const (
	DefaultMaxTxSize     = 1024 * 1024 // 1MB max transaction size
	DefaultSyncTimeout   = 10 * time.Second
	DefaultCommitTimeout = 30 * time.Second
)

// TransactionSubmitter submits transactions to the DAG-BFT consensus.
type TransactionSubmitter interface {
	// Submit submits a transaction to consensus.
	Submit(ctx context.Context, tx []byte) error
}

// DAGSubmitterConfig holds configuration for DAGSubmitter.
type DAGSubmitterConfig struct {
	// MaxTxSize is the maximum transaction size in bytes.
	// Defaults to DefaultMaxTxSize.
	MaxTxSize int

	// SyncTimeout is the timeout for sync mode submissions.
	// Defaults to DefaultSyncTimeout.
	SyncTimeout time.Duration

	// CommitTimeout is the timeout for commit mode submissions.
	// Defaults to DefaultCommitTimeout.
	CommitTimeout time.Duration
}

// applyDefaults fills in default values for unset configuration fields.
func (c *DAGSubmitterConfig) applyDefaults() {
	if c.MaxTxSize <= 0 {
		c.MaxTxSize = DefaultMaxTxSize
	}
	if c.SyncTimeout <= 0 {
		c.SyncTimeout = DefaultSyncTimeout
	}
	if c.CommitTimeout <= 0 {
		c.CommitTimeout = DefaultCommitTimeout
	}
}

// DAGSubmitter implements transaction submission to DAG-BFT consensus.
type DAGSubmitter struct {
	config    DAGSubmitterConfig
	node      TransactionSubmitter
	validator adapter.TransactionValidator
	tracker   *txtracker.Tracker

	mu     sync.RWMutex
	closed bool
}

// NewDAGSubmitter creates a new DAGSubmitter.
func NewDAGSubmitter(
	config DAGSubmitterConfig,
	node TransactionSubmitter,
	validator adapter.TransactionValidator,
	tracker *txtracker.Tracker,
) *DAGSubmitter {
	config.applyDefaults()

	return &DAGSubmitter{
		config:    config,
		node:      node,
		validator: validator,
		tracker:   tracker,
	}
}

// SubmitEnvelope submits an envelope to the consensus layer using async mode.
func (s *DAGSubmitter) SubmitEnvelope(ctx context.Context, env *messaging.Envelope) error {
	return s.SubmitEnvelopeWithMode(ctx, env, BroadcastModeAsync)
}

// SubmitEnvelopeWithMode submits an envelope with a specific broadcast mode.
func (s *DAGSubmitter) SubmitEnvelopeWithMode(ctx context.Context, env *messaging.Envelope, mode BroadcastMode) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrSubmitterClosed
	}
	s.mu.RUnlock()

	if env == nil {
		return errors.New("envelope is nil")
	}

	// Marshal envelope to bytes
	tx, err := env.MarshalBinary()
	if err != nil {
		return err
	}

	return s.SubmitRawWithMode(ctx, tx, mode)
}

// SubmitRaw submits raw transaction bytes using async mode.
func (s *DAGSubmitter) SubmitRaw(ctx context.Context, tx []byte) error {
	return s.SubmitRawWithMode(ctx, tx, BroadcastModeAsync)
}

// SubmitRawWithMode submits raw transaction bytes with a specific broadcast mode.
func (s *DAGSubmitter) SubmitRawWithMode(ctx context.Context, tx []byte, mode BroadcastMode) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrSubmitterClosed
	}
	s.mu.RUnlock()

	if len(tx) == 0 {
		return errors.New("transaction is empty")
	}

	// Check transaction size
	if len(tx) > s.config.MaxTxSize {
		return ErrTransactionTooLarge
	}

	// Optional pre-validation
	if s.validator != nil {
		if err := s.validator.ValidateTransaction(tx); err != nil {
			return errors.Join(ErrValidationFailed, err)
		}
	}

	// Track the transaction if tracker is available
	var txHash [32]byte
	if s.tracker != nil {
		txHash = s.tracker.Track(tx)
	}

	// Submit to consensus
	if err := s.node.Submit(ctx, tx); err != nil {
		// Mark as failed if we tracked it
		if s.tracker != nil {
			s.tracker.SetStatus(txHash, txtracker.StatusFailed)
		}
		return err
	}

	// Handle broadcast modes
	switch mode {
	case BroadcastModeAsync:
		// Return immediately
		return nil

	case BroadcastModeSync:
		// Wait for batch inclusion
		if s.tracker == nil {
			return nil // No tracker, can't wait
		}
		ctx, cancel := context.WithTimeout(ctx, s.config.SyncTimeout)
		defer cancel()
		return s.tracker.WaitForStatus(ctx, txHash, txtracker.StatusBatched)

	case BroadcastModeCommit:
		// Wait for block commit
		if s.tracker == nil {
			return nil // No tracker, can't wait
		}
		ctx, cancel := context.WithTimeout(ctx, s.config.CommitTimeout)
		defer cancel()
		return s.tracker.WaitForStatus(ctx, txHash, txtracker.StatusCommitted)

	default:
		return nil
	}
}

// Close closes the submitter.
func (s *DAGSubmitter) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	return nil
}
