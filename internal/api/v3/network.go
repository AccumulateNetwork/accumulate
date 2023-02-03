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
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
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
			return values.Load(config.NetworkUrl{URL: protocol.PartitionUrl(s.partition)}, func(accountUrl *url.URL, target interface{}) error {
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
	return res, nil
}
