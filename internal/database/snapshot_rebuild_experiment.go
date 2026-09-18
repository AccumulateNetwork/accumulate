// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// EXPERIMENT, NOT A FIX.
//
// Two candidate ports of dagbft-integration's rebuildChainIndexes (c2b0e9d2f,
// internal/database/snapshot.go:1009) onto main, so the difference between them
// can be settled by execution instead of by reading. Nothing selects a mode but
// a test; the default is the behaviour main ships today, which is to rebuild
// nothing.

type RebuildChainIndexModeT int

const (
	// RebuildNone is what main does today: a restore rebuilds no merkle element
	// index at all.
	RebuildNone RebuildChainIndexModeT = iota

	// RebuildVerbatim is dagbft-integration's function, copied without change:
	// an unconditional Put over 0..Count-1, so the LAST occurrence of a
	// repeated hash wins.
	RebuildVerbatim

	// RebuildFirstOccurrence skips the Put when a record already exists, so the
	// FIRST occurrence wins, which is what main's AddEntry writes.
	RebuildFirstOccurrence
)

// RebuildChainIndexMode selects the variant Restore runs. Test-only.
var RebuildChainIndexMode = RebuildNone

func rebuildChainIndexesExperiment(batch *Batch) error {
	switch RebuildChainIndexMode {
	case RebuildVerbatim:
		return rebuildChainIndexesVerbatim(batch)
	case RebuildFirstOccurrence:
		return rebuildChainIndexesFirstOccurrence(batch)
	default:
		return nil
	}
}

// rebuildChainIndexesVerbatim is dagbft-integration's rebuildChainIndexes,
// copied verbatim from c2b0e9d2f.
func rebuildChainIndexesVerbatim(batch *Batch) error {
	return batch.ForEachAccount(func(account *Account, _ [32]byte) error {
		chains, err := account.Chains().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load chains of %v: %w", account.Url(), err)
		}
		for _, meta := range chains {
			c2, err := account.ChainByName(meta.Name)
			if err != nil {
				return errors.UnknownError.WithFormat("resolve chain %s of %v: %w", meta.Name, account.Url(), err)
			}
			inner := c2.Inner()
			head, err := inner.Head().Get()
			if err != nil {
				return errors.UnknownError.WithFormat("load head of %s of %v: %w", meta.Name, account.Url(), err)
			}
			for i := int64(0); i < head.Count; i++ {
				hash, err := inner.Entry(i)
				if err != nil {
					return errors.UnknownError.WithFormat("load entry %d of %s of %v: %w", i, meta.Name, account.Url(), err)
				}
				err = inner.ElementIndex(hash).Put(uint64(i))
				if err != nil {
					return errors.UnknownError.WithFormat("index entry %d of %s of %v: %w", i, meta.Name, account.Url(), err)
				}
			}
		}
		return nil
	})
}

// rebuildChainIndexesFirstOccurrence is the same walk, except that it leaves an
// existing record alone, so a repeated hash keeps the position it was first
// written at - which is the position main's AddEntry records.
func rebuildChainIndexesFirstOccurrence(batch *Batch) error {
	return batch.ForEachAccount(func(account *Account, _ [32]byte) error {
		chains, err := account.Chains().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load chains of %v: %w", account.Url(), err)
		}
		for _, meta := range chains {
			c2, err := account.ChainByName(meta.Name)
			if err != nil {
				return errors.UnknownError.WithFormat("resolve chain %s of %v: %w", meta.Name, account.Url(), err)
			}
			inner := c2.Inner()
			head, err := inner.Head().Get()
			if err != nil {
				return errors.UnknownError.WithFormat("load head of %s of %v: %w", meta.Name, account.Url(), err)
			}
			for i := int64(0); i < head.Count; i++ {
				hash, err := inner.Entry(i)
				if err != nil {
					return errors.UnknownError.WithFormat("load entry %d of %s of %v: %w", i, meta.Name, account.Url(), err)
				}
				_, err = inner.ElementIndex(hash).Get()
				switch {
				case err == nil:
					continue // already indexed, at an earlier position
				case !errors.Is(err, errors.NotFound):
					return errors.UnknownError.WithFormat("index entry %d of %s of %v: %w", i, meta.Name, account.Url(), err)
				}
				err = inner.ElementIndex(hash).Put(uint64(i))
				if err != nil {
					return errors.UnknownError.WithFormat("index entry %d of %s of %v: %w", i, meta.Name, account.Url(), err)
				}
			}
		}
		return nil
	})
}
