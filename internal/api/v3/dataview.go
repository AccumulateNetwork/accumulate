// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// dataView is an account's data index as it would be had the reorg not
// restarted it (#4273). The genesis restore started every data index it
// rebuilt at zero, so the live index counts only the entries written since,
// and its entry i is the i'th since the reorg while the sidecar's entry i is
// the i'th before it. The view maps the counts back: the pre-reorg entries
// first, then the live ones, numbered from where the pre-reorg index ended.
//
// A live index that was not restarted -- one the restore carried whole -- is
// served as it is. The two are told apart by one read: a restarted index has
// an entry in the era at the live count, a whole one cannot.
type dataView struct {
	live *indexing.DataIndexer
	era  *indexing.DataIndexer // nil without an era
	pre  uint64                // entries before the reorg that the live index does not count
}

// dataViews remembers each account's pre-reorg count. It never changes, so
// the reads that find it are paid once per account per process.
type dataViews struct {
	sync.Map // url string -> uint64
}

// dataView returns the view of the account's data index.
func (s *Querier) dataView(batch *database.Batch, era *indexing.Era, u *url.URL) (*dataView, error) {
	v := &dataView{live: indexing.Data(batch, u)}
	if era == nil {
		return v, nil
	}
	v.era = indexing.Data(era.Batch, u)
	if pre, ok := s.dataViews.Load(u.String()); ok {
		v.pre = pre.(uint64)
		return v, nil
	}
	live, err := v.live.Count()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get count: %w", err)
	}
	pre, err := preReorgCount(v.era, live)
	if err != nil {
		return nil, err
	}
	s.dataViews.Store(u.String(), pre)
	v.pre = pre
	return v, nil
}

// preReorgCount returns the number of entries the index had before the
// reorg, if the live index was restarted, else zero.
//
// The era's own count is the pre-reorg count only where the overwritten
// store holds it (the live database holds the key, restarted, so the sidecar
// does not). Where it does not, the entries themselves say: they are
// contiguous, so the count is the end of the run, found by doubling and
// bisection from the live count.
func preReorgCount(era *indexing.DataIndexer, live uint64) (uint64, error) {
	has := func(i uint64) (bool, error) {
		_, err := era.Entry(i)
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, errors.NotFound):
			return false, nil
		default:
			return false, errors.UnknownError.WithFormat("read the era's data index: %w", err)
		}
	}
	ok, err := has(live)
	if err != nil || !ok {
		return 0, err
	}
	count, err := era.Count()
	if err != nil {
		return 0, errors.UnknownError.WithFormat("get the era's count: %w", err)
	}
	if count > live {
		return count, nil
	}

	// Find the end of the run: the smallest i > live with no entry
	lo, hi := live, live+1 // has(lo) is true; find hi with !has(hi)
	for {
		ok, err := has(hi)
		if err != nil {
			return 0, err
		}
		if !ok {
			break
		}
		lo = hi
		hi *= 2
	}
	for hi-lo > 1 {
		mid := lo + (hi-lo)/2
		ok, err := has(mid)
		if err != nil {
			return 0, err
		}
		if ok {
			lo = mid
		} else {
			hi = mid
		}
	}
	return hi, nil
}

// Count is the number of entries, before and since the reorg.
func (v *dataView) Count() (uint64, error) {
	n, err := v.live.Count()
	if err != nil {
		return 0, err
	}
	return v.pre + n, nil
}

// Entry returns the hash of entry i: the era's for i below the pre-reorg
// count, else the live index's, renumbered.
func (v *dataView) Entry(i uint64) ([]byte, error) {
	if i < v.pre {
		return v.era.Entry(i)
	}
	return v.live.Entry(i - v.pre)
}

// Transaction returns the hash of the transaction that wrote the entry. The
// record is keyed by the entry's hash, so it is the same in both eras.
func (v *dataView) Transaction(entryHash []byte) ([]byte, error) {
	return v.live.Transaction(entryHash)
}
