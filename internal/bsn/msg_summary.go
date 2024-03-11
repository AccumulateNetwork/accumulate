// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

import (
	"strings"

	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[BlockSummary](&executors, messaging.MessageTypeBlockSummary)
}

type BlockSummary struct{}

func (x BlockSummary) Validate(batch *ChangeSet, ctx *MessageContext) error {
	_, err := x.check(batch, ctx)
	return err
}

func (BlockSummary) check(batch *ChangeSet, ctx *MessageContext) (*messaging.BlockSummary, error) {
	msg, ok := ctx.message.(*messaging.BlockSummary)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeBlockAnchor, ctx.message.Type())
	}

	if msg.Partition == "" {
		return nil, errors.BadRequest.With("missing partition")
	}
	if msg.Index == 0 {
		return nil, errors.BadRequest.With("missing index")
	}
	if !msg.IsSorted() {
		return nil, errors.BadRequest.With("not sorted")
	}

	return msg, nil
}

func (x BlockSummary) Process(batch *ChangeSet, ctx *MessageContext) (err error) {
	batch = batch.Begin()
	defer func() { commitOrDiscard(batch, &err) }()

	msg, err := x.check(batch, ctx)
	switch {
	case err == nil:
		// Ok
	case errors.Code(err).IsClientError():
		ctx.recordErrorStatus(err)
		return nil
	default:
		return errors.UnknownError.Wrap(err)
	}

	// Load the signatures
	sigs, err := batch.Summary(msg.Hash()).Signatures().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load signatures: %w", err)
	}

	partdb := batch.Partition(msg.Partition)

	// Load the partition's globals
	g, err := ctx.executor.loadGlobals(msg.Partition, batch, nil, false)
	if err != nil {
		return errors.UnknownError.WithFormat("load %s globals: %w", msg.Partition, err)
	}

	// Check the threshold
	if uint64(len(sigs)) < g.ValidatorThreshold(msg.Partition) {
		ctx.statuses = append(ctx.statuses, &protocol.TransactionStatus{
			TxID: ctx.message.ID(),
			Code: errors.Pending,
		})
		return nil
	}

	// Check the block
	var ledger *protocol.SystemLedger
	u := protocol.PartitionUrl(msg.Partition).JoinPath(protocol.Ledger)
	err = partdb.Account(u).Main().GetAs(&ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("load %s system ledger: %w", msg.Partition, err)
	}

	switch {
	case msg.PreviousBlock > ledger.Index:
		// Record as pending
		err = batch.Pending(msg.Partition).OnBlock(msg.PreviousBlock).Put(msg.Hash())
		return errors.UnknownError.Wrap(err)

	case msg.PreviousBlock < ledger.Index:
		// Block is out of date
		ctx.recordErrorStatus(errors.BadTimestamp.WithFormat("block is old: current height is %d, summary applies to %d", ledger.Index, msg.PreviousBlock))
		return nil
	}

	// Process the message
	ctx2 := &SummaryContext{MessageContext: ctx, summary: msg}
	err = x.process(batch, ctx2)
	switch {
	case err == nil:
		// Ok
		ctx.executor.logger.Info("Processed block summary",
			"source", msg.Partition,
			"block", msg.Index,
			"previous-block", msg.PreviousBlock,
			"hash", logging.AsHex(msg.StateTreeHash).Slice(0, 4),
			"updates", len(msg.RecordUpdates))

	case errors.Code(err).IsClientError():
		ctx.executor.logger.Info("Processing block summary failed",
			"error", err,
			"source", msg.Partition,
			"block", msg.Index,
			"previous-block", msg.PreviousBlock,
			"hash", logging.AsHex(msg.StateTreeHash).Slice(0, 4),
			"updates", len(msg.RecordUpdates))

		ctx.recordErrorStatus(err)
		return nil
	default:
		return errors.UnknownError.Wrap(err)
	}

	// Check if the globals have updated
	if strings.EqualFold(msg.Partition, protocol.Directory) {
		_, err = ctx.executor.loadGlobals(msg.Partition, batch, g, true)
		if err != nil {
			return errors.UnknownError.WithFormat("load globals: %w", err)
		}
	}

	// Apply indexing
	err = x.applyIndexing(batch, ctx2)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Is there a pending update for the next block?
	hash, err := batch.Pending(msg.Partition).OnBlock(msg.Index).Get()
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		return nil
	default:
		return errors.UnknownError.WithFormat("load pending hash: %w", err)
	}

	pending, err := batch.Summary(hash).Main().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load %s block %d pending summary: %w", msg.Partition, msg.Index, err)
	}
	ctx.additional = append(ctx.additional, pending)
	return nil
}

func (x BlockSummary) process(batch *ChangeSet, ctx *SummaryContext) (err error) {
	batch = batch.Begin()
	defer func() { commitOrDiscard(batch, &err) }()
	msg := ctx.summary
	part := batch.Partition(msg.Partition)

	// Execute all the record updates
	err = x.executeUpdates(part, ctx)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Verify the root hash is the same
	hash, err := part.GetBptRootHash()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if msg.StateTreeHash != hash {
		return errors.BadRequest.With("state hash does not match")
	}

	// Load the ledger
	var ledger *protocol.SystemLedger
	err = part.Account(protocol.PartitionUrl(msg.Partition).JoinPath(protocol.Ledger)).Main().GetAs(&ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("load %s system ledger: %w", msg.Partition, err)
	}
	if ledger.Index != msg.Index {
		return errors.UnknownError.WithFormat("invalid summary: index does not match: summary says %d but ledger says %d", msg.Index, ledger.Index)
	}

	return nil
}

func (x BlockSummary) executeUpdates(batch *coredb.Batch, ctx *SummaryContext) (err error) {
	// Use a child batch and commit to force the BPT to update
	batch = batch.Begin(true)
	defer func() { commitOrDiscard(batch, &err) }()

	for _, v := range ctx.summary.RecordUpdates {
		w, err := values.Resolve[database.Value](batch, v.Key)
		if err != nil {
			return errors.UnknownError.WithFormat("store record update: %w", err)
		}

		err = x.willUpdate(batch, ctx, v, w)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		err = w.LoadBytes(v.Value, true)
		if err != nil {
			return errors.UnknownError.WithFormat("store record update: %w", err)
		}
	}
	return nil
}

func (BlockSummary) willUpdate(batch *coredb.Batch, ctx *SummaryContext, update *messaging.RecordUpdate, value database.Value) error {
	// Resolve the value
	var r database.Record = batch
	var key = update.Key
	var err error
	for key.Len() > 0 {
		r, key, err = r.Resolve(key)
		if err != nil {
			return errors.UnknownError.WithFormat("resolve: %w", err)
		}

		for _, f := range indexerFactories {
			err = f(ctx, update, r, key)
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
		}
	}
	return nil
}

func (BlockSummary) applyIndexing(batch *ChangeSet, ctx *SummaryContext) error {
	part := batch.Partition(ctx.summary.Partition)

	for _, idx := range ctx.indexers {
		// Resolve the value
		var r record.Record = part
		var key = idx.Key()
		var err error
		for key.Len() > 0 {
			r, key, err = r.Resolve(key)
			if err != nil {
				return errors.UnknownError.WithFormat("resolve: %w", err)
			}
		}

		// Some values require an additional resolve
		if s, _, err := r.Resolve(nil); err == nil {
			r = s
		}

		// Apply the indexer
		err = idx.Apply(batch, ctx, r)
		if err != nil {
			return errors.UnknownError.WithFormat("apply %v indexer: %w", idx.Key(), err)
		}
	}
	return nil
}
