// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package tendermint

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cometbft/cometbft/types"
	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// NodeStatusClient is an interface for getting node status from CometBFT.
type NodeStatusClient interface {
	Status(context.Context) (*coretypes.ResultStatus, error)
	NetInfo(context.Context) (*coretypes.ResultNetInfo, error)
}

// ConsensusService implements api.ConsensusService for CometBFT.
type ConsensusService struct {
	logger        logging.OptionalLogger
	local         NodeStatusClient
	db            database.Viewer
	partitionID   string
	partitionType protocol.PartitionType
	partition     config.NetworkUrl
	nodeKeyHash   [32]byte
	valKeyHash    [32]byte
}

var _ api.ConsensusService = (*ConsensusService)(nil)

// ConsensusServiceParams holds the parameters for creating a ConsensusService.
type ConsensusServiceParams struct {
	Logger           logging.Logger
	Local            NodeStatusClient
	Database         database.Viewer
	PartitionID      string
	PartitionType    protocol.PartitionType
	EventBus         *events.Bus
	NodeKeyHash      [32]byte
	ValidatorKeyHash [32]byte
}

// NewConsensusService creates a new CometBFT ConsensusService.
func NewConsensusService(params ConsensusServiceParams) *ConsensusService {
	s := new(ConsensusService)
	s.logger.L = params.Logger
	s.local = params.Local
	s.db = params.Database
	s.partitionID = params.PartitionID
	s.partitionType = params.PartitionType
	s.partition.URL = protocol.PartitionUrl(params.PartitionID)
	s.nodeKeyHash = params.NodeKeyHash
	s.valKeyHash = params.ValidatorKeyHash
	return s
}

// Type returns the service type.
func (s *ConsensusService) Type() api.ServiceType { return api.ServiceTypeConsensus }

// ConsensusStatus returns the current consensus status.
func (s *ConsensusService) ConsensusStatus(ctx context.Context, opts api.ConsensusStatusOptions) (*api.ConsensusStatus, error) {
	// Basic data
	res := new(api.ConsensusStatus)
	res.Ok = true
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

	// Get the latest block info from Tendermint
	status, err := s.local.Status(ctx)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get status: %w", err)
	}
	res.LastBlock.Height = status.SyncInfo.LatestBlockHeight
	res.LastBlock.Time = status.SyncInfo.LatestBlockTime
	switch len(status.SyncInfo.LatestBlockHash) {
	case 0: // No block yet
	case 32:
		res.LastBlock.StateRoot = *(*[32]byte)(status.SyncInfo.LatestBlockHash)
	default:
		return nil, errors.InternalError.WithFormat("invalid block hash returned from Tendermint")
	}

	if !boolOpt(opts.IncludePeers, true) {
		return res, nil
	}

	// Get peers from Tendermint
	netInfo, err := s.local.NetInfo(ctx)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get net info: %w", err)
	}
	res.Peers = make([]*api.ConsensusPeerInfo, len(netInfo.Peers))
	for i, src := range netInfo.Peers {
		peer := new(api.ConsensusPeerInfo)
		peer.NodeID = string(src.NodeInfo.ID())
		res.Peers[i] = peer

		addr := src.NodeInfo.ListenAddr
		if !strings.Contains(addr, "://") {
			addr = "tcp://" + addr
		}
		u, err := url.Parse(addr)
		if err == nil && u.Scheme == "" {
			u, err = url.Parse("tcp://" + u.Scheme)
		}
		if err != nil {
			continue
		}
		peer.Host = u.Hostname()

		port, err := strconv.ParseUint(u.Port(), 10, 64)
		if err == nil {
			peer.Port = port
		}
	}

	return res, nil
}

func boolOpt(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

// SubmitClient is an interface for submitting transactions to CometBFT.
type SubmitClient interface {
	BroadcastTxAsync(context.Context, types.Tx) (*coretypes.ResultBroadcastTx, error)
	BroadcastTxSync(context.Context, types.Tx) (*coretypes.ResultBroadcastTx, error)
}

// Submitter implements api.Submitter for CometBFT.
type Submitter struct {
	logger logging.OptionalLogger
	local  SubmitClient
}

var _ api.Submitter = (*Submitter)(nil)

// SubmitterParams holds the parameters for creating a Submitter.
type SubmitterParams struct {
	Logger logging.Logger
	Local  SubmitClient
}

// NewSubmitter creates a new CometBFT Submitter.
func NewSubmitter(params SubmitterParams) *Submitter {
	s := new(Submitter)
	s.logger.L = params.Logger
	s.local = params.Local
	return s
}

// Type returns the service type.
func (s *Submitter) Type() api.ServiceType { return api.ServiceTypeSubmit }

// Submit submits an envelope to CometBFT.
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

// ValidateClient is an interface for validating transactions with CometBFT.
type ValidateClient interface {
	CheckTx(context.Context, types.Tx) (*coretypes.ResultCheckTx, error)
}

// Validator implements api.Validator for CometBFT.
type Validator struct {
	logger logging.OptionalLogger
	local  ValidateClient
}

var _ api.Validator = (*Validator)(nil)

// ValidatorParams holds the parameters for creating a Validator.
type ValidatorParams struct {
	Logger logging.Logger
	Local  ValidateClient
}

// NewValidator creates a new CometBFT Validator.
func NewValidator(params ValidatorParams) *Validator {
	s := new(Validator)
	s.logger.L = params.Logger
	s.local = params.Local
	return s
}

// Type returns the service type.
func (s *Validator) Type() api.ServiceType { return api.ServiceTypeValidate }

// Validate validates an envelope without submitting it.
func (s *Validator) Validate(ctx context.Context, envelope *messaging.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
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
	case len(res.Log) > 0:
		message = res.Log
	case len(res.Info) > 0:
		message = res.Info
	case res.Code != 0:
		message = "An unknown error occurred"
	}

	return unpackConsensusResult(res.Code, message, res.Data)
}
