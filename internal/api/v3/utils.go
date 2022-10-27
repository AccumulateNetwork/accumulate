package api

import (
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func fixRange(opts *api.RangeOptions) {
	if opts == nil {
		return
	}
	if opts.Count == nil {
		opts.Count = new(uint64)
		*opts.Count = defaultPageSize
	} else if *opts.Count > maxPageSize {
		*opts.Count = maxPageSize
	}
}

type indexType int

const (
	zeroBased indexType = iota
	oneBased
)

func (typ indexType) first() uint64 {
	if typ == zeroBased {
		return 0
	}
	return 1
}

// oobUpper returns true if index exceeds the upper bound for the given indexing type.
func (typ indexType) oobUpper(index, max uint64) bool {
	if typ == zeroBased {
		return index >= max
	}
	return index > max
}

// after returns the number of entries after the index for the given indexing type.
func (typ indexType) after(index, max uint64) uint64 {
	if typ == zeroBased {
		return max - index
	}
	return max - index + 1
}

func allocRange[T api.Record](r *api.RecordRange[T], opts *api.RangeOptions, idx indexType) {
	if opts.FromEnd {
		s := opts.Start + *opts.Count
		if s > r.Total {
			opts.Start = idx.first()
		} else {
			opts.Start = idx.after(s, r.Total)
		}
	} else if opts.Start < idx.first() {
		opts.Start = idx.first()
	}

	if idx.oobUpper(opts.Start, r.Total) {
		return
	}

	r.Start = opts.Start

	if idx.oobUpper(r.Start+*opts.Count, r.Total) {
		*opts.Count = idx.after(r.Start, r.Total)
	}

	r.Records = make([]T, *opts.Count)
}

func getMajorBlockBounds(partUrl config.NetworkUrl, batch *database.Batch, entry *protocol.IndexEntry, entryIndex uint64) (rootEntry, rootPrev *protocol.IndexEntry, err error) {
	rootChain, err := batch.Account(partUrl.Ledger()).RootChain().Index().Get()
	if err != nil {
		return nil, nil, errors.Format(errors.UnknownError, "load root index chain: %w", err)
	}

	rootEntry = new(protocol.IndexEntry)
	err = rootChain.EntryAs(int64(entry.RootIndexIndex), rootEntry)
	if err != nil {
		return nil, nil, errors.Format(errors.UnknownError, "load root index chain entry %d: %w", entry.RootIndexIndex, err)
	}

	rootPrev = new(protocol.IndexEntry)
	if entryIndex > 0 {
		chain, err := batch.Account(partUrl.AnchorPool()).MajorBlockChain().Get()
		if err != nil {
			return nil, nil, errors.Format(errors.UnknownError, "load major block chain: %w", err)
		}
		prev := new(protocol.IndexEntry)
		err = chain.EntryAs(int64(entryIndex)-1, prev)
		if err != nil {
			return nil, nil, errors.Format(errors.UnknownError, "load major block chain entry %d: %w", entryIndex-1, err)
		}
		err = rootChain.EntryAs(int64(prev.RootIndexIndex), rootPrev)
		if err != nil {
			return nil, nil, errors.Format(errors.UnknownError, "load root index chain entry %d: %w", prev.RootIndexIndex, err)
		}
	}

	return rootEntry, rootPrev, nil
}
