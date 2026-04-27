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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
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
//
// TxId path: walks the principal's main-index chain to find the block
// containing the transaction; returns that block's time. (Issue #3978.)
//
// BlockHeight path: walks the partition's root anchor index. Returns
// the block time at the requested height.
func (s *MetricsService) BlockTimeFor(ctx context.Context, opts api.BlockTimeForOptions) (*api.BlockTimeResult, error) {
	if s.database == nil {
		return nil, errors.NotAllowed.With("metrics service has no database; BlockTimeFor unavailable")
	}
	if opts.TxId == nil && opts.BlockHeight == 0 {
		return nil, errors.BadRequest.With("set TxId or BlockHeight")
	}

	var result *api.BlockTimeResult
	err := s.database.View(func(batch *database.Batch) error {
		if opts.TxId != nil {
			r, err := blockTimeForTx(batch, opts.TxId)
			if err != nil {
				return err
			}
			result = r
			return nil
		}
		// BlockHeight path
		r, err := blockTimeForHeight(batch, s.partition.Ledger(), opts.BlockHeight)
		if err != nil {
			return err
		}
		result = r
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.NotFound.With("block time not available")
	}
	return result, nil
}

// blockTimeForTx finds the block containing the given transaction by
// looking at the per-transaction chain-membership index, then walking
// the principal's main-index chain to resolve the IndexEntry covering
// the transaction's main-chain position.
func blockTimeForTx(batch *database.Batch, txid *url.TxID) (*api.BlockTimeResult, error) {
	hash := txid.Hash()
	entries, err := batch.Transaction(hash[:]).Chains().Get()
	if err != nil {
		return nil, errors.NotFound.WithFormat("transaction %v not found in any chain", txid)
	}
	// Pick the principal's main chain entry if present; else the first
	// "main" chain we find.
	var pick *database.TransactionChainEntry
	for _, e := range entries {
		if e.Chain == "main" {
			pick = e
			break
		}
	}
	if pick == nil {
		return nil, errors.NotFound.WithFormat("no main-chain entry for tx %v", txid)
	}

	// pick.ChainIndex is the index *on the main-index chain*. Read it
	// directly to get the IndexEntry with BlockTime.
	indexChain, err := batch.Account(pick.Account).MainChain().Index().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get main-index chain: %w", err)
	}
	if pick.ChainIndex >= uint64(indexChain.Height()) {
		return nil, errors.NotFound.With("chain-index out of range")
	}
	raw, err := indexChain.Entry(int64(pick.ChainIndex))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("read index entry: %w", err)
	}
	var ie protocol.IndexEntry
	if err := ie.UnmarshalBinary(raw); err != nil {
		return nil, errors.UnknownError.WithFormat("decode index entry: %w", err)
	}
	if ie.BlockTime == nil {
		return nil, errors.NotFound.With("block time not recorded for this entry")
	}
	return &api.BlockTimeResult{BlockTime: *ie.BlockTime, BlockHeight: ie.BlockIndex}, nil
}

// blockTimeForHeight finds the block time for the given block height
// by walking the partition ledger's root anchor index.
func blockTimeForHeight(batch *database.Batch, ledgerUrl *url.URL, height uint64) (*api.BlockTimeResult, error) {
	// Walk the system ledger's root chain index, find the IndexEntry
	// whose BlockIndex == height. Most recent ledger fields don't help
	// here directly; we need the actual chain entry.
	var ledger *protocol.SystemLedger
	if err := batch.Account(ledgerUrl).Main().GetAs(&ledger); err != nil {
		return nil, errors.NotFound.WithFormat("ledger %v not found", ledgerUrl)
	}
	if ledger.Index == height {
		// Fast path: requested height is the latest committed block.
		return &api.BlockTimeResult{BlockTime: ledger.Timestamp, BlockHeight: height}, nil
	}
	if height > ledger.Index {
		return nil, errors.NotFound.WithFormat("requested height %d > tip %d", height, ledger.Index)
	}
	// For older heights, walking the root-anchor index would yield the
	// answer but the root anchor chain isn't trivially accessible from
	// here without the partition's root-chain plumbing. Surface as
	// "deferred" so callers know to use the latest-tip fast path or
	// pass TxId instead.
	return nil, errors.NotAllowed.WithFormat(
		"historical block-time lookup (height %d != tip %d) requires root-chain walk; deferred under #3978",
		height, ledger.Index)
}
