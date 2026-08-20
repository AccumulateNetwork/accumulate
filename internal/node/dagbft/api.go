// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	stderrors "errors"
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/crosschain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ConsensusAPIService implements api.ConsensusService for DAG-BFT.
type ConsensusAPIService struct {
	logger        logging.OptionalLogger
	service       *Service
	db            database.Viewer
	partitionID   string
	partitionType protocol.PartitionType
	partition     config.NetworkUrl
	nodeKeyHash   [32]byte
	valKeyHash    [32]byte
	heals         *crosschain.HealCounters
}

var _ api.ConsensusService = (*ConsensusAPIService)(nil)

// ConsensusAPIServiceParams holds the parameters for creating a ConsensusAPIService.
type ConsensusAPIServiceParams struct {
	Logger           logging.Logger
	Service          *Service
	Database         database.Viewer
	PartitionID      string
	PartitionType    protocol.PartitionType
	EventBus         *events.Bus
	NodeKeyHash      [32]byte
	ValidatorKeyHash [32]byte

	// Heals is shared with the conductor so recoveries are reportable, not
	// only loggable (#4075, #4105) — the soak monitor reads these fields.
	Heals *crosschain.HealCounters
}

// NewConsensusAPIService creates a new ConsensusAPIService.
func NewConsensusAPIService(params ConsensusAPIServiceParams) *ConsensusAPIService {
	s := new(ConsensusAPIService)
	s.logger.L = params.Logger
	s.service = params.Service
	s.db = params.Database
	s.partitionID = params.PartitionID
	s.partitionType = params.PartitionType
	s.partition.URL = protocol.PartitionUrl(params.PartitionID)
	s.nodeKeyHash = params.NodeKeyHash
	s.valKeyHash = params.ValidatorKeyHash
	s.heals = params.Heals
	return s
}

// Type returns the service type.
func (s *ConsensusAPIService) Type() api.ServiceType { return api.ServiceTypeConsensus }

// ConsensusStatus returns the current consensus status.
func (s *ConsensusAPIService) ConsensusStatus(ctx context.Context, opts api.ConsensusStatusOptions) (*api.ConsensusStatus, error) {
	// Basic data
	res := new(api.ConsensusStatus)
	res.Ok = true
	if s.heals != nil {
		res.SyntheticHeals = s.heals.Synthetic.Load()
		res.AnchorHeals = s.heals.Anchor.Load()
	}
	res.Version = accumulate.Version
	res.Commit = accumulate.Commit
	res.NodeKeyHash = s.nodeKeyHash
	res.ValidatorKeyHash = s.valKeyHash
	res.PartitionID = s.partitionID
	res.PartitionType = s.partitionType

	// Load values from the database
	res.LastBlock = new(api.LastBlock)
	if s.db != nil && boolOpt(opts.IncludeAccumulate, true) {
		err := s.db.View(func(batch *database.Batch) error {
			c, err := batch.Account(s.partition.Ledger()).RootChain().Get()
			if err != nil {
				return errors.UnknownError.WithFormat("load root chain: %w", err)
			}
			res.LastBlock.ChainRoot = *(*[32]byte)(c.Anchor())

			c, err = batch.Account(s.partition.AnchorPool()).AnchorChain(protocol.Directory).Root().Get()
			if err != nil {
				return errors.UnknownError.WithFormat("load root anchor chain for the DN: %w", err)
			}
			res.LastBlock.DirectoryAnchorHeight = uint64(c.Height())
			return nil
		})
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	// Get status from DAG-BFT service
	if s.service != nil {
		status := s.service.Status()
		res.LastBlock.Height = int64(status.LastBlockIndex)
		res.LastBlock.Time = status.LastBlockTime
		res.LastBlock.StateRoot = s.service.adapter.StateHash()
	}

	if !boolOpt(opts.IncludePeers, true) {
		return res, nil
	}

	// DAG-BFT doesn't have the same peer concept as CometBFT
	// Return empty peer list for now
	res.Peers = []*api.ConsensusPeerInfo{}

	return res, nil
}

func boolOpt(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

// SubmitterService implements api.Submitter for DAG-BFT.
type SubmitterService struct {
	logger  logging.OptionalLogger
	service *Service
}

var _ api.Submitter = (*SubmitterService)(nil)

// SubmitterServiceParams holds the parameters for creating a SubmitterService.
type SubmitterServiceParams struct {
	Logger  logging.Logger
	Service *Service
}

// NewSubmitterService creates a new SubmitterService.
func NewSubmitterService(params SubmitterServiceParams) *SubmitterService {
	s := new(SubmitterService)
	s.logger.L = params.Logger
	s.service = params.Service
	return s
}

// Type returns the service type.
func (s *SubmitterService) Type() api.ServiceType { return api.ServiceTypeSubmit }

// Submit submits an envelope to the DAG-BFT consensus.
func (s *SubmitterService) Submit(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	// Identify what is being submitted: the contentless version of this trace
	// made it impossible to follow a specific lost message (#4111) through
	// accept → batch → commit → execute.
	var msgIDs []string
	for _, m := range envelope.Messages {
		msgIDs = append(msgIDs, fmt.Sprintf("%v:%v", m.Type(), m.ID()))
	}
	s.logger.Info("TRACE-SUBMIT: SubmitterService.Submit() called (DAG-BFT)", "messages", strings.Join(msgIDs, ","))

	// Verify the envelope is well-formed
	if opts.Verify == nil || *opts.Verify {
		_, err := envelope.Normalize()
		if err != nil {
			s.logger.Error("TRACE-SUBMIT: envelope normalization failed", "error", err)
			return nil, errors.BadRequest.WithFormat("verify: %w", err)
		}
	}

	// Marshal envelope
	b, err := envelope.MarshalBinary()
	if err != nil {
		s.logger.Error("TRACE-SUBMIT: envelope marshaling failed", "error", err)
		return nil, errors.EncodingError.WithFormat("marshal: %w", err)
	}

	s.logger.Info("TRACE-SUBMIT: submitting to DAG-BFT service.SubmitTransaction")

	// Submit to consensus (includes pre-batch validation)
	if err := s.service.SubmitTransaction(b); err != nil {
		// Check if this is a validation error
		if stderrors.Is(err, worker.ErrValidationFailed) {
			s.logger.Error("TRACE-SUBMIT: validation failed, returning error submission", "error", err)
			return []*api.Submission{{
				Success: false,
				Message: fmt.Sprintf("Transaction validation failed: %v", err),
				Status: &protocol.TransactionStatus{
					TxID: nil,
					Code: errors.Pending,
				},
			}}, nil
		}
		if stderrors.Is(err, worker.ErrBackpressure) {
			s.logger.Error("TRACE-SUBMIT: backpressure error", "error", err)
			return nil, errors.TooManyRequests.WithFormat("submit: %w", err)
		}
		s.logger.Error("TRACE-SUBMIT: internal error", "error", err)
		return nil, errors.InternalError.WithFormat("submit: %w", err)
	}

	s.logger.Info("TRACE-SUBMIT: submission successful, creating result WITHOUT Status field (BUG!)")

	// Return success - DAG-BFT doesn't have synchronous result like CometBFT
	result := []*api.Submission{{
		Success: true,
		Message: "Transaction submitted to DAG-BFT consensus",
		Status: &protocol.TransactionStatus{
			TxID: nil,
			Code: errors.Pending,
		},
	}}

	s.logger.Info("TRACE-SUBMIT: returning result", "submission_count", len(result), "status_is_nil", result[0].Status == nil)

	return result, nil
}

// ValidatorService implements api.Validator for DAG-BFT.
type ValidatorService struct {
	logger  logging.OptionalLogger
	service *Service
}

var _ api.Validator = (*ValidatorService)(nil)

// ValidatorServiceParams holds the parameters for creating a ValidatorService.
type ValidatorServiceParams struct {
	Logger  logging.Logger
	Service *Service
}

// NewValidatorService creates a new ValidatorService.
func NewValidatorService(params ValidatorServiceParams) *ValidatorService {
	s := new(ValidatorService)
	s.logger.L = params.Logger
	s.service = params.Service
	return s
}

// Type returns the service type.
func (s *ValidatorService) Type() api.ServiceType { return api.ServiceTypeValidate }

// Validate validates an envelope without submitting it.
func (s *ValidatorService) Validate(ctx context.Context, envelope *messaging.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
	// Marshal envelope
	b, err := envelope.MarshalBinary()
	if err != nil {
		return nil, errors.EncodingError.WithFormat("marshal: %w", err)
	}

	// Validate using adapter
	if err := s.service.adapter.ValidateTransaction(b); err != nil {
		return []*api.Submission{{
			Success: false,
			Message: fmt.Sprintf("Validation failed: %v", err),
		}}, nil
	}

	return []*api.Submission{{
		Success: true,
		Message: "Transaction is valid",
	}}, nil
}
