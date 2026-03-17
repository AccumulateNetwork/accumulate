// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ConsensusService provides consensus status information for DAG-BFT nodes.
type ConsensusService struct {
	logger        logging.OptionalLogger
	node          *consensus.Node
	db            database.Viewer
	partitionID   string
	partitionType protocol.PartitionType
	partition     config.NetworkUrl
	nodeKeyHash   [32]byte
	valKeyHash    [32]byte
}

var _ api.ConsensusService = (*ConsensusService)(nil)

// ConsensusServiceParams contains parameters for creating a ConsensusService.
type ConsensusServiceParams struct {
	Logger           log.Logger
	Node             *consensus.Node
	Database         database.Viewer
	PartitionID      string
	PartitionType    protocol.PartitionType
	EventBus         *events.Bus
	NodeKeyHash      [32]byte
	ValidatorKeyHash [32]byte
}

// NewConsensusService creates a new DAG-BFT consensus service.
func NewConsensusService(params ConsensusServiceParams) *ConsensusService {
	s := new(ConsensusService)
	s.logger.L = params.Logger
	s.node = params.Node
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

	// Get consensus info from the DAG-BFT node
	if s.node != nil {
		res.LastBlock.Height = int64(s.node.CurrentRound())
		// DAG-BFT doesn't have time per round, use zero value
		// The actual block time would come from the executor

		// Compute state root from last commit round
		lastCommit := s.node.LastCommitRound()
		stateBytes := []byte{byte(lastCommit >> 56), byte(lastCommit >> 48), byte(lastCommit >> 40),
			byte(lastCommit >> 32), byte(lastCommit >> 24), byte(lastCommit >> 16),
			byte(lastCommit >> 8), byte(lastCommit)}
		res.LastBlock.StateRoot = sha256.Sum256(stateBytes)
	}

	if !boolOpt(opts.IncludePeers, true) {
		return res, nil
	}

	// Get peers from DAG-BFT committee
	if s.node != nil && s.node.Committee() != nil {
		committee := s.node.Committee()
		res.Peers = make([]*api.ConsensusPeerInfo, 0, committee.Len())

		for i := 0; i < committee.Len(); i++ {
			validator := committee.GetValidator(uint16(i))
			if validator == nil {
				continue
			}

			peer := new(api.ConsensusPeerInfo)
			// Use public key hash as node ID
			pubKeyHash := sha256.Sum256(validator.PublicKey)
			peer.NodeID = string(ed25519.PublicKey(pubKeyHash[:16]))
			res.Peers = append(res.Peers, peer)
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
