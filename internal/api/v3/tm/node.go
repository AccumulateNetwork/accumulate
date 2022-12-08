package tm

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/tendermint/tendermint/libs/log"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"
	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type NodeStatusClient interface {
	Status(context.Context) (*coretypes.ResultStatus, error)
	NetInfo(context.Context) (*coretypes.ResultNetInfo, error)
}

type NodeService struct {
	logger        logging.OptionalLogger
	local         NodeStatusClient
	db            database.Viewer
	partitionID   string
	partitionType protocol.PartitionType
	partition     config.NetworkUrl
	nodeKeyHash   [32]byte
	valKeyHash    [32]byte
}

var _ api.NodeService = (*NodeService)(nil)

type NodeServiceParams struct {
	Logger           log.Logger
	Local            NodeStatusClient
	Database         database.Viewer
	PartitionID      string
	PartitionType    protocol.PartitionType
	EventBus         *events.Bus
	NodeKeyHash      [32]byte
	ValidatorKeyHash [32]byte
}

func NewNodeService(params NodeServiceParams) *NodeService {
	s := new(NodeService)
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

func (s *NodeService) Type() api.ServiceType { return api.ServiceTypeNode }

func (s *NodeService) NodeStatus(ctx context.Context, _ api.NodeStatusOptions) (*api.NodeStatus, error) {
	// Basic data
	res := new(api.NodeStatus)
	res.Ok = true
	res.Version = accumulate.Version
	res.Commit = accumulate.Commit
	res.NodeKeyHash = s.nodeKeyHash
	res.ValidatorKeyHash = s.valKeyHash
	res.PartitionID = s.partitionID
	res.PartitionType = s.partitionType

	// Load values from the database
	res.LastBlock = new(api.LastBlock)
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

	// Get peers from Tendermint
	netInfo, err := s.local.NetInfo(ctx)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get net info: %w", err)
	}
	res.Peers = make([]*api.PeerInfo, len(netInfo.Peers))
	for i, src := range netInfo.Peers {
		peer := new(api.PeerInfo)
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
