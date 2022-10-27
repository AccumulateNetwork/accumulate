package api

import (
	"context"
	"sync/atomic"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type NetworkService struct {
	logger logging.OptionalLogger
	values atomic.Value
}

var _ api.NetworkService = (*NetworkService)(nil)

type NetworkServiceParams struct {
	Logger   log.Logger
	EventBus *events.Bus
	Globals  *core.GlobalValues
}

func NewNetworkService(params NetworkServiceParams) *NetworkService {
	s := new(NetworkService)
	s.logger.L = params.Logger
	s.values.Store(params.Globals)
	events.SubscribeAsync(params.EventBus, func(e events.WillChangeGlobals) {
		s.values.Store(e.New)
	})
	return s
}

func (s *NetworkService) NetworkStatus(ctx context.Context, _ api.NetworkStatusOptions) (*api.NetworkStatus, error) {
	values := s.values.Load().(*core.GlobalValues)
	if values == nil {
		return nil, errors.NotReady
	}

	// Basic data
	res := new(api.NetworkStatus)
	res.Globals = values.Globals
	res.Network = values.Network
	res.Oracle = values.Oracle
	res.Routing = values.Routing
	return res, nil
}
