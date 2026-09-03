// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type MetricsService struct {
	logger  logging.OptionalLogger
	node    api.ConsensusService
	querier api.Querier2
}

var _ api.MetricsService = (*MetricsService)(nil)

type MetricsServiceParams struct {
	Logger  logging.Logger
	Node    api.ConsensusService
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
	status, err := s.node.ConsensusStatus(ctx, api.ConsensusStatusOptions{})
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
		// The block query reads the block ledger in whichever form recorded
		// it (executor spec, "The block ledger"). Only the count and the time
		// are wanted, so no entries are loaded.
		index := last - i
		block, err := s.querier.QueryMinorBlock(ctx, partition.URL, &api.BlockQuery{
			Minor:      &index,
			EntryRange: &api.RangeOptions{Count: new(uint64)},
		})
		switch {
		case err == nil:
		case errors.Is(err, errors.NotFound):
			continue // Empty
		default:
			return nil, errors.UnknownError.WithFormat("load block %d ledger: %w", index, err)
		}

		// This is technically chain entries per second, but that's a lot easier
		// to calculate than actual transactions per second
		if block.Time != nil {
			start = *block.Time
		}
		if block.Entries != nil {
			count += int(block.Entries.Total)
		}
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
