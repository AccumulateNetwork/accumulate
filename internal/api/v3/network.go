// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// NodeStatusResult contains the subset of node status fields used by
// NetworkService for staleness detection. This replaces the CometBFT
// coretypes.ResultStatus dependency.
type NodeStatusResult struct {
	LatestBlockTime time.Time
	CatchingUp      bool
}

// NodeStatusClient is the interface for querying node status.
type NodeStatusClient interface {
	Status(context.Context) (*NodeStatusResult, error)
}

type NetworkService struct {
	logger     logging.OptionalLogger
	values     atomic.Pointer[core.GlobalValues]
	database   database.Viewer
	partition  string
	nodeStatus NodeStatusClient
	nodeState  nodestate.Serving
}

var _ api.NetworkService = (*NetworkService)(nil)

type NetworkServiceParams struct {
	Logger    logging.Logger
	EventBus  *events.Bus
	Partition string
	Database  database.Viewer
	// NodeStatus is optional; if provided, NetworkStatus will include
	// staleness detection fields (LastBlockTime and CatchingUp).
	NodeStatus NodeStatusClient

	// NodeState is this node's join state. A network status is a READ -- the
	// globals, the oracle and the routing table, out of the store -- so a
	// joining node refuses it like any other (executor.md, "Sync", step 6;
	// #4295 F1). Nil means the node never joined.
	NodeState nodestate.Serving
}

func NewNetworkService(params NetworkServiceParams) *NetworkService {
	s := new(NetworkService)
	s.logger.L = params.Logger
	s.database = params.Database
	s.partition = params.Partition
	s.nodeStatus = params.NodeStatus
	s.nodeState = params.NodeState
	events.SubscribeAsync(params.EventBus, func(e events.WillChangeGlobals) {
		s.values.Store(e.New)
	})
	return s
}

func (s *NetworkService) Type() api.ServiceType { return api.ServiceTypeNetwork }

func (s *NetworkService) NetworkStatus(ctx context.Context, _ api.NetworkStatusOptions) (*api.NetworkStatus, error) {
	// A joining node refuses every read, and this is one: the routing table
	// it would answer with is the half-filled store's, and an external client
	// builds its router out of it (pkg/api/v3/p2p/client.go) and keeps it.
	// How a peer learns this node is not ready is ConsensusStatus.CatchingUp,
	// which is a different service and is not gated (#4295 F1).
	if s.nodeState != nil && !s.nodeState.CanServeCurrent() {
		return nil, errors.NotReady.WithFormat(
			"%s is joining and cannot answer for state it has not executed", s.partition)
	}

	values := s.values.Load()
	if values == nil {
		values = new(core.GlobalValues)
		err := s.database.View(func(batch *database.Batch) error {
			return values.Load(protocol.PartitionUrl(s.partition), func(accountUrl *url.URL, target interface{}) error {
				return batch.Account(accountUrl).Main().GetAs(target)
			})
		})
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load globals: %w", err)
		}
	}

	// Basic data
	res := new(api.NetworkStatus)
	res.Globals = values.Globals
	res.Network = values.Network
	res.Oracle = values.Oracle
	res.Routing = values.Routing
	res.ExecutorVersion = values.ExecutorVersion
	res.BvnExecutorVersions = values.BvnExecutorVersions

	// Data from the database
	err := s.database.View(func(batch *database.Batch) error {
		var err error
		res.DirectoryHeight, err = s.getDnHeight(batch)
		if err != nil {
			return errors.UnknownError.WithFormat("load directory height: %w", err)
		}

		res.MajorBlockHeight, err = s.getMajorHeight(batch)
		if err != nil {
			return errors.UnknownError.WithFormat("load major block height: %w", err)
		}

		return err
	})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// If a node status client is available, populate staleness detection fields
	if s.nodeStatus != nil {
		status, err := s.nodeStatus.Status(ctx)
		if err != nil {
			s.logger.Error("Failed to get node status for staleness detection", "error", err)
		} else {
			t := status.LatestBlockTime
			res.LastBlockTime = &t
			catchingUp := status.CatchingUp
			res.CatchingUp = &catchingUp
		}
	}

	return res, nil
}

func (s *NetworkService) getDnHeight(batch *database.Batch) (uint64, error) {
	// On the Directory itself the height is the system ledger's index: one
	// read of mutable state. The anchor walk below loads anchor bodies by
	// hash, which are write-once records, and a status poll per submission
	// made it the largest reader of the Directory store's history (soak
	// 20260905T032333Z: 2.4M reads per node in 45 minutes, all from here).
	if strings.EqualFold(s.partition, protocol.Directory) {
		var ledger *protocol.SystemLedger
		err := batch.Account(protocol.DnUrl().JoinPath(protocol.Ledger)).Main().GetAs(&ledger)
		if err != nil {
			return 0, errors.UnknownError.WithFormat("load system ledger: %w", err)
		}
		return ledger.Index, nil
	}

	c := batch.Account(protocol.PartitionUrl(s.partition).JoinPath(protocol.AnchorPool)).MainChain()
	head, err := c.Head().Get()
	if err != nil {
		return 0, errors.UnknownError.WithFormat("load anchor ledger main chain head: %w", err)
	}

	for i := head.Count - 1; i >= 0; i-- {
		entry, err := c.Entry(i)
		if err != nil {
			return 0, errors.UnknownError.WithFormat("load anchor ledger main chain entry %d (1): %w", i, err)
		}

		var msg *messaging.TransactionMessage
		err = batch.Message2(entry).Main().GetAs(&msg)
		if err != nil {
			return 0, errors.UnknownError.WithFormat("load anchor ledger main chain entry %d (2): %w", i, err)
		}

		body, ok := msg.Transaction.Body.(*protocol.DirectoryAnchor)
		if ok {
			return body.MinorBlockIndex, nil
		}
	}

	return 0, nil
}

func (s *NetworkService) getMajorHeight(batch *database.Batch) (uint64, error) {
	c := batch.Account(protocol.PartitionUrl(s.partition).JoinPath(protocol.AnchorPool)).MajorBlockChain()
	head, err := c.Head().Get()
	if err != nil {
		return 0, errors.UnknownError.WithFormat("load major block chain head: %w", err)
	}
	if head.Count == 0 {
		return 0, nil
	}

	hash, err := c.Entry(head.Count - 1)
	if err != nil {
		return 0, errors.UnknownError.WithFormat("load major block chain latest entry: %w", err)
	}

	entry := new(protocol.IndexEntry)
	err = entry.UnmarshalBinary(hash)
	if err != nil {
		return 0, errors.EncodingError.WithFormat("decode major block chain entry: %w", err)
	}

	return entry.BlockIndex, nil
}
