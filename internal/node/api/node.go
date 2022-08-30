package api

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/rpc/client"
	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type NodeService struct {
	logger     logging.OptionalLogger
	local      client.StatusClient
	db         database.Beginner
	partition  config.NetworkUrl
	values     atomic.Value
	valKeyHash [32]byte
}

type NodeServiceParams struct {
	Logger           log.Logger
	Local            client.StatusClient
	Database         database.Beginner
	Partition        string
	EventBus         *events.Bus
	ValidatorKeyHash [32]byte
}

func NewNodeService(params NodeServiceParams) *NodeService {
	s := new(NodeService)
	s.logger.L = params.Logger
	s.local = params.Local
	s.db = params.Database
	s.partition.URL = protocol.PartitionUrl(params.Partition)
	s.valKeyHash = params.ValidatorKeyHash
	s.values.Store((*core.GlobalValues)(nil))
	events.SubscribeAsync(params.EventBus, func(e events.WillChangeGlobals) {
		s.values.Store(e.New)
	})
	return s
}

func (*NodeService) Version(context.Context) (*api.NodeVersion, error) {
	res := new(api.NodeVersion)
	res.Version = accumulate.Version
	res.Commit = accumulate.Commit
	res.VersionIsKnown = accumulate.IsVersionKnown()
	return res, nil
}

func (s *NodeService) Status(ctx context.Context) (*api.NodeStatus, error) {
	res := new(api.NodeStatus)
	res.Ok = true
	res.LastBlock = new(api.LastBlock)

	status, err := s.local.Status(ctx)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "get status: %w", err)
	}

	res.LastBlock.Height = status.SyncInfo.LatestBlockHeight
	res.LastBlock.Time = status.SyncInfo.LatestBlockTime
	switch len(status.SyncInfo.LatestBlockHash) {
	case 0: // No block yet
	case 32:
		res.LastBlock.StateRoot = *(*[32]byte)(status.SyncInfo.LatestBlockHash)
	default:
		return nil, errors.Format(errors.StatusInternalError, "invalid block hash returned from Tendermint")
	}

	err = s.db.View(func(batch *database.Batch) error {
		c, err := batch.Account(s.partition.Ledger()).RootChain().Get()
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "load root chain: %w", err)
		}
		res.LastBlock.ChainRoot = *(*[32]byte)(c.Anchor())

		c, err = batch.Account(s.partition.AnchorPool()).AnchorChain(protocol.Directory).Root().Get()
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "load root anchor chain for the DN: %w", err)
		}
		res.LastBlock.DirectoryAnchorHeight = uint64(c.Height())
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}
	return res, nil
}

func (s *NodeService) Describe(context.Context) (*api.NodeDescription, error) {
	values := s.values.Load().(*core.GlobalValues)
	if values == nil {
		return nil, errors.Format(errors.StatusConflict, "global values have not been loaded yet")
	}

	res := new(api.NodeDescription)
	res.ValidatorKeyHash = s.valKeyHash
	res.Network = values.Network
	return res, nil
}

func (s *NodeService) Metrics(ctx context.Context) (*api.NodeMetrics, error) {
	status, err := s.local.Status(ctx)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "get status: %w", err)
	}

	batch := s.db.Begin(false)
	defer batch.Discard()
	last := uint64(status.SyncInfo.LatestBlockHeight)
	var count int
	var start time.Time
	for i := uint64(0); i < 1 && i <= last; i++ {
		var block *protocol.BlockLedger
		err = batch.Account(s.partition.BlockLedger(last - i)).Main().GetAs(&block)
		switch {
		case err == nil:
		case errors.Is(err, errors.StatusNotFound):
			continue // Empty
		default:
			return nil, errors.Format(errors.StatusUnknownError, "load block %d ledger: %w", last-i, err)
		}

		// This is technically chain entries per second, but that's a lot easier
		// to calculate than actual transactions per second
		start = block.Time
		count += len(block.Entries)
	}

	res := new(api.NodeMetrics)
	if count == 0 {
		res.TPS = 0
	} else {
		duration := status.SyncInfo.LatestBlockTime.Round(time.Second).Sub(start) + time.Second
		res.TPS = float64(count) / duration.Seconds()
	}
	return res, nil
}
