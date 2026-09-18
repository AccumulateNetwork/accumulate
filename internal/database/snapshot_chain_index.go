// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// A snapshot carries a chain's entries but not its merkle element index: the
// index is an index record, so Collect walks with IgnoreIndices and the BPT
// does not cover it. Nothing then rebuilds it, so a restored node has no
// element index for anything predating its snapshot - and the element index is
// what the executor reads to answer two different questions:
//
//   - Is this entry already on the chain? AddEntry reads it to skip a
//     duplicate, and production offers a duplicate nearly every block
//     (block_begin.go captures CommitInfo into <partition>/votes, and that
//     transaction's hash carries no height, time, block hash or signature). A
//     node with no index appends what its peers skip: divergence by append
//     (#4328).
//   - Do we hold this anchor? holdsAnchorRoot (msg_synthetic.go), the proof
//     checks in CreateTokenAccount and SetLiteAccountDelegate, and
//     indexing/receipts.go all ask IndexOf or HeightOf about existence. A node
//     with no index says "I never received that anchor" about an anchor it is
//     holding: divergence by refusal (#4330).
//
// rebuildChainIndexes writes the index back from the entries, once, at restore.
//
// FIRST OCCURRENCE, deliberately. AddEntry writes the index only when no record
// exists (chain.go: the Put is inside the ErrNotFound branch), so on a node that
// built its chain a repeated hash is indexed at the height it FIRST appeared
// at. Writing the last occurrence instead would still repair the dedup and the
// existence checks - they read presence, not the value - but it would break the
// receipt plane: indexing.getIndexedChainReceipt does Receipt(HeightOf(entry),
// anchorIndex), and if HeightOf names an occurrence after that anchor the
// receipt cannot be built at all. A missing index is a clean NotFound a caller
// can handle; a plausible wrong index is not.
//
// limit bounds how many entries are held in one batch before it is committed. A
// mainnet snapshot holds millions of accounts and tens of millions of entries;
// buffering every write into the batch that restored the snapshot, as a single
// commit, would not fit in memory.
func rebuildChainIndexes(db Beginner, limit int) error {
	if limit <= 0 {
		limit = defaultBatchRecordLimit
	}

	// The account walk gets its own read-only batch. IterateAccounts creates
	// account records without adding them to the batch's map, so walking does
	// not retain them.
	accounts := db.Begin(false)
	defer accounts.Discard()

	w := &chainIndexWriter{db: db, limit: limit}
	w.batch = db.Begin(true)
	defer func() { w.batch.Discard() }()

	it := accounts.IterateAccounts()
	for it.Next() {
		err := w.rebuildAccount(it.Value().Url())
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}
	if it.Err() != nil {
		return errors.UnknownError.Wrap(it.Err())
	}

	return w.flush()
}

// chainIndexWriter commits and replaces its batch every limit entries. Entries
// are read through the same batch, so rotating it also drops the mark point
// states the reads cached - which is the larger half of the memory.
type chainIndexWriter struct {
	db    Beginner
	limit int
	batch *Batch
	n     int
}

// rotate commits the current batch and starts a new one.
func (w *chainIndexWriter) rotate() error {
	err := w.batch.Commit()
	if err != nil {
		return errors.UnknownError.WithFormat("commit chain index: %w", err)
	}
	w.batch = w.db.Begin(true)
	w.n = 0
	return nil
}

func (w *chainIndexWriter) flush() error {
	err := w.batch.Commit()
	if err != nil {
		return errors.UnknownError.WithFormat("commit chain index: %w", err)
	}
	return nil
}

func (w *chainIndexWriter) rebuildAccount(u *url.URL) error {
	chains, err := w.batch.Account(u).Chains().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load chains of %v: %w", u, err)
	}

	for _, meta := range chains {
		err = w.rebuildChain(u, meta.Name)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}
	return nil
}

func (w *chainIndexWriter) rebuildChain(u *url.URL, name string) error {
	// The batch is replaced part way through a long chain, so the account and
	// the chain are resolved again after every rotation.
	for i := int64(0); ; {
		c, err := w.batch.Account(u).ChainByName(name)
		if err != nil {
			return errors.UnknownError.WithFormat("resolve chain %s of %v: %w", name, u, err)
		}
		inner := c.Inner()
		head, err := inner.Head().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load head of %s of %v: %w", name, u, err)
		}

		for ; i < head.Count && w.n < w.limit; i++ {
			w.n++

			hash, err := inner.Entry(i)
			if err != nil {
				return errors.UnknownError.WithFormat("load entry %d of %s of %v: %w", i, name, u, err)
			}

			// Leave an existing record alone: the first occurrence of a
			// repeated hash keeps the position AddEntry would have given it.
			_, err = inner.ElementIndex(hash).Get()
			switch {
			case err == nil:
				continue
			case !errors.Is(err, errors.NotFound):
				return errors.UnknownError.WithFormat("load index of entry %d of %s of %v: %w", i, name, u, err)
			}

			err = inner.ElementIndex(hash).Put(uint64(i))
			if err != nil {
				return errors.UnknownError.WithFormat("index entry %d of %s of %v: %w", i, name, u, err)
			}
		}

		if i >= head.Count {
			return nil
		}
		err = w.rotate()
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}
}
