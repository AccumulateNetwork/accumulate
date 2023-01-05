// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"time"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type MetricsService struct {
	logger  logging.OptionalLogger
	node    api.NodeService
	querier api.Querier2
}

var _ api.MetricsService = (*MetricsService)(nil)

type MetricsServiceParams struct {
	Logger  log.Logger
	Node    api.NodeService
	Querier api.Querier
}

func NewMetricsService(params MetricsServiceParams) *MetricsService {
	s := new(MetricsService)
	s.logger.L = params.Logger
	s.node = params.Node
	s.querier.Querier = params.Querier
	return s
}

func (s *MetricsService) Type() api.ServiceType { return api.ServiceTypeMetrics }

func (s *MetricsService) Metrics(ctx context.Context, opts api.MetricsOptions) (*api.Metrics, error) {
	status, err := s.node.NodeStatus(ctx, api.NodeStatusOptions{})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get status: %w", err)
	}

	const maxSpan = time.Hour / time.Second
	if opts.Span == 0 || opts.Span > uint64(maxSpan) {
		opts.Span = uint64(maxSpan)
	}

	var partition config.NetworkUrl
	partition.URL = protocol.PartitionUrl(status.PartitionID)

	last := uint64(status.LastBlock.Height)
	var count int
	var start time.Time
	for i := uint64(0); i < opts.Span && i <= last; i++ {
		var block *protocol.BlockLedger
		_, err = s.querier.QueryAccountAs(ctx, partition.BlockLedger(last-i), nil, &block)
		switch {
		case err == nil:
		case errors.Is(err, errors.NotFound):
			continue // Empty
		default:
			return nil, errors.UnknownError.WithFormat("load block %d ledger: %w", last-i, err)
		}

		// This is technically chain entries per second, but that's a lot easier
		// to calculate than actual transactions per second
		start = block.Time
		count += len(block.Entries)
	}

	res := new(api.Metrics)
	if count == 0 {
		res.TPS = 0
	} else {
		duration := status.LastBlock.Time.Round(time.Second).Sub(start) + time.Second
		res.TPS = float64(count) / duration.Seconds()
	}
	return res, nil
}
