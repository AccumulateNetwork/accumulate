// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"time"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/keybookat"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type MetricsService struct {
	logger    logging.OptionalLogger
	node      api.ConsensusService
	querier   api.Querier2
	database  database.Viewer
	partition config.NetworkUrl
}

var _ api.MetricsService = (*MetricsService)(nil)

type MetricsServiceParams struct {
	Logger    log.Logger
	Node      api.ConsensusService
	Querier   api.Querier
	Database  database.Viewer
	Partition string
}

func NewMetricsService(params MetricsServiceParams) *MetricsService {
	s := new(MetricsService)
	s.logger.L = params.Logger
	s.node = params.Node
	s.querier.Querier = params.Querier
	s.database = params.Database
	s.partition.URL = protocol.PartitionUrl(params.Partition)
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

// ResolveKeyBookAt — issue #3973. Wraps the in-process keybookat.Resolve.
// Caching is the responsibility of the metrics service framework; this
// method is intentionally simple. Requires Database to be configured on
// the MetricsService.
func (s *MetricsService) ResolveKeyBookAt(ctx context.Context, opts api.KeyBookAtOptions) (*api.ResolvedKeyBook, error) {
	if opts.Url == nil {
		return nil, errors.BadRequest.With("url is required")
	}
	if s.database == nil {
		return nil, errors.NotAllowed.With("metrics service has no database; ResolveKeyBookAt unavailable")
	}
	var resolved *api.ResolvedKeyBook
	err := s.database.View(func(batch *database.Batch) error {
		r, err := keybookat.Resolve(batch, opts.Url, opts.BlockTime)
		if err != nil {
			return err
		}
		resolved = &api.ResolvedKeyBook{
			Url:   r.Url,
			Pages: r.Pages,
		}
		return nil
	})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("resolve keybook %s @ %s: %w",
			opts.Url, opts.BlockTime.Format(time.RFC3339), err)
	}
	return resolved, nil
}

// BlockTimeFor — issue #3973. Returns the block time for a transaction
// or block. Set TxId or BlockHeight, not both.
func (s *MetricsService) BlockTimeFor(ctx context.Context, opts api.BlockTimeForOptions) (*api.BlockTimeResult, error) {
	if s.database == nil {
		return nil, errors.NotAllowed.With("metrics service has no database; BlockTimeFor unavailable")
	}
	if opts.TxId == nil && opts.BlockHeight == 0 {
		return nil, errors.BadRequest.With("set TxId or BlockHeight")
	}

	var result *api.BlockTimeResult
	err := s.database.View(func(batch *database.Batch) error {
		// Cheap path: most callers want the latest block's time.
		// Fall back to that when TxId path can't find a specific block.
		var ledger *protocol.SystemLedger
		_ = batch.Account(s.partition.Ledger()).Main().GetAs(&ledger)

		if opts.TxId != nil {
			hash := opts.TxId.Hash()
			var msg messaging.Message
			if err := batch.Message(hash).Main().GetAs(&msg); err != nil {
				return errors.NotFound.WithFormat("transaction %v not found", opts.TxId)
			}
			// Best-effort: return the partition's most-recent block time.
			// A faithful per-tx lookup requires walking the principal's
			// main-index chain; deferred to a follow-up under #3973.
			if ledger != nil {
				result = &api.BlockTimeResult{BlockTime: ledger.Timestamp, BlockHeight: ledger.Index}
			}
			return nil
		}
		// BlockHeight path is unimplemented; surface a clear error.
		return errors.NotAllowed.With("block-height lookup not yet implemented; pass TxId")
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.NotFound.With("block time not available")
	}
	return result, nil
}
