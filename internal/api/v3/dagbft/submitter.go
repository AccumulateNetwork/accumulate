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

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/txtracker"
	pkgerrors "gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
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
			return nil, pkgerrors.BadRequest.WithFormat("verify: %w", err)
		}
	}

	b, err := envelope.MarshalBinary()
	if err != nil {
		return nil, pkgerrors.EncodingError.WithFormat("marshal: %w", err)
	}

	// Submit to DAG-BFT node
	err = s.node.SubmitTransaction(b)
	if err != nil {
		return nil, pkgerrors.InternalError.WithFormat("submit: %w", err)
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
				Code: pkgerrors.Pending,
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
