// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

func init() {
	// Add a chain element indexer for each chain update
	registerIndexerFactory(func(ctx *SummaryContext, update *messaging.RecordUpdate, record record.Record, remainingKey *record.Key) error {
		chain, ok := record.(*database.Chain2)
		if !ok {
			return nil
		}

		if remainingKey.Len() != 1 || remainingKey.Get(0) != "Head" {
			return nil
		}

		// Get the previous head
		head, err := chain.Head().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load chain head: %w", err)
		}

		key := update.Key.SliceJ(update.Key.Len() - remainingKey.Len())
		ctx.index(&chainElemIndexer{key: key, oldHeight: uint64(head.Count)})
		return nil
	})
}

type chainElemIndexer struct {
	key       *record.Key
	oldHeight uint64
}

func (c *chainElemIndexer) Key() *record.Key { return c.key }

func (c *chainElemIndexer) Apply(batch *ChangeSet, ctx *SummaryContext, r record.Record) error {
	chain, ok := r.(*database.Chain2)
	if !ok {
		return errors.InternalError.WithFormat("expected %T, got %T", new(database.Chain2), r)
	}

	// Get the new entries
	head, err := chain.Head().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load new chain head: %w", err)
	}

	hashes, err := chain.Inner().Entries(int64(c.oldHeight), head.Count)
	if err != nil {
		return errors.UnknownError.WithFormat("load new chain entries: %w", err)
	}

	// Index the new entries. A hash already indexed keeps its first
	// position, as the node's own AddEntry does (pkg/database/merkle/
	// chain.go): chains that admit duplicates would otherwise have the
	// index rewritten with a later position -- a different value under
	// the same key, which a write-once storage layer refuses (#4174).
	for i, hash := range hashes {
		index := chain.Inner().ElementIndex(hash)
		_, err = index.Get()
		switch {
		case err == nil:
			continue // Already indexed
		case errors.Is(err, errors.NotFound):
			// Not yet
		default:
			return errors.UnknownError.WithFormat("load index for chain entry: %w", err)
		}
		err = index.Put(c.oldHeight + uint64(i))
		if err != nil {
			return errors.UnknownError.WithFormat("store index for new chain entry: %w", err)
		}
	}

	return nil
}
