// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// recordTaken writes what a chain's take leaves missing into the chain's
// SyncedEntries, the full description of what the node is missing of it
// (executor spec, "Sync", "Past the match"; #4493). It is called where the
// pull writes a chain's head, before the head is written:
//
//   - prevCount is the count the node held the chain at before: every entry
//     below it is held, but for the gaps the record already names;
//   - head is the head being written, and held is the first entry the take
//     brought with it: every entry from held up to the head's count is held;
//   - msgsFrom is the first of those that came with its message. A head
//     taken alone brings its open mark set without messages, so on a chain
//     that carries messages msgsFrom is the head's count; a chain taken
//     whole or by its window brings them, and msgsFrom is held.
//
// The head is recorded with the gaps: its pending roots prove the entries the
// backfill brings in below it.
func recordTaken(account *database.Account, name string, typ merkle.ChainType, prevCount uint64, head *merkle.State, held, msgsFrom uint64) error {
	rec, err := account.SyncedEntries(name).Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load the synced entries of %s: %w", name, err)
	}
	count := uint64(head.Count)
	bodies := carriesMessages(&api.ChainRecord{Name: name, Type: typ})

	var gaps []*database.SyncedGap
	add := func(low, high uint64, messagesOnly bool) {
		if low < high {
			gaps = append(gaps, &database.SyncedGap{Low: low, High: high, MessagesOnly: messagesOnly})
		}
	}

	// What the node was missing below the take stays missing.
	below := min(prevCount, held)
	for _, g := range rec.Gaps {
		add(g.Low, min(g.High, below), g.MessagesOnly)
	}
	// What lies between what the node held and what the take brought.
	if prevCount < held {
		add(prevCount, held, false)
	}
	if bodies {
		// What the node was missing inside the take: the take wrote the
		// entries, and the messages are missing still.
		for _, g := range rec.Gaps {
			add(max(g.Low, held), min(g.High, prevCount, msgsFrom), true)
		}
		// The messages the take did not bring, above what the node held.
		add(max(held, prevCount), min(msgsFrom, count), true)
	}

	rec = &database.SyncedEntries{Gaps: mergeGaps(gaps), HeadCount: count}
	for _, p := range head.Pending {
		rec.HeadPending = append(rec.HeadPending, append([]byte{}, p...))
	}
	return account.SyncedEntries(name).Put(rec)
}

// mergeGaps orders gaps newest first and joins those that touch and miss the
// same.
func mergeGaps(gaps []*database.SyncedGap) []*database.SyncedGap {
	sort.Slice(gaps, func(i, j int) bool { return gaps[i].High > gaps[j].High })
	var out []*database.SyncedGap
	for _, g := range gaps {
		if n := len(out); n > 0 && out[n-1].Low <= g.High && out[n-1].MessagesOnly == g.MessagesOnly {
			out[n-1].Low = min(out[n-1].Low, g.Low)
			continue
		}
		out = append(out, g)
	}
	return out
}

// headOf is the head a record was taken at.
func headOf(rec *database.SyncedEntries) *merkle.State {
	st := &merkle.State{Count: int64(rec.HeadCount)}
	for _, p := range rec.HeadPending {
		if len(p) == 0 {
			st.Pending = append(st.Pending, nil)
		} else {
			st.Pending = append(st.Pending, p)
		}
	}
	return st
}

// closeGap takes [low, high) out of the record's gaps of the given kind. An
// entry gap closed is closed for its messages too.
func closeGap(rec *database.SyncedEntries, low, high uint64) {
	var out []*database.SyncedGap
	for _, g := range rec.Gaps {
		if g.High <= low || g.Low >= high {
			out = append(out, g)
			continue
		}
		if g.Low < low {
			out = append(out, &database.SyncedGap{Low: g.Low, High: low, MessagesOnly: g.MessagesOnly})
		}
		if g.High > high {
			out = append(out, &database.SyncedGap{Low: high, High: g.High, MessagesOnly: g.MessagesOnly})
		}
	}
	rec.Gaps = mergeGaps(out)
}
