// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"sync/atomic"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type NetworkService struct {
	logger    logging.OptionalLogger
	values    atomic.Pointer[core.GlobalValues]
	database  database.Viewer
	partition string
}

var _ api.NetworkService = (*NetworkService)(nil)

type NetworkServiceParams struct {
	Logger    log.Logger
	EventBus  *events.Bus
	Partition string
	Database  database.Viewer
}

func NewNetworkService(params NetworkServiceParams) *NetworkService {
	s := new(NetworkService)
	s.logger.L = params.Logger
	s.database = params.Database
	s.partition = params.Partition
	events.SubscribeAsync(params.EventBus, func(e events.WillChangeGlobals) {
		s.values.Store(e.New)
	})
	return s
}

func (s *NetworkService) Type() api.ServiceType { return api.ServiceTypeNetwork }

func (s *NetworkService) NetworkStatus(ctx context.Context, _ api.NetworkStatusOptions) (*api.NetworkStatus, error) {
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

	return res, nil
}

func (s *NetworkService) getDnHeight(batch *database.Batch) (uint64, error) {
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
