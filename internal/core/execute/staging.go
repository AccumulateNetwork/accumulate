// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// StreamID names one ordered cross-partition stream: the account whose ledger
// tracks it, and the partition its messages come from.
//
// The ledger is part of the name, not just the source, because anchors and
// synthetics between the same pair of partitions are SEPARATE streams — anchors
// tracked by the anchor pool, synthetics by the synthetic account. Conflating
// them would let an anchor's position gate a synthetic's.
type StreamID struct {
	Ledger *url.URL
	Source *url.URL
}

// Staging is what a node has received and cannot execute yet: the numbers above
// a stream's Delivered watermark whose predecessors have not arrived. Nothing
// reaches it that consensus did not accept.
//
// It is not a type. It is two records on the stream's ledger account —
// Sequenced(source, number) and Sighted(source) — and this file is the
// vocabulary for them. Everything that needs to know what the node holds reads
// the same records: the executor deciding what to run, and healing deciding
// what to fetch. Two views of that is the disagreement this replaces.
//
// # Durable, and not hashed
//
// Both properties are load-bearing, and they are not the same property.
//
// DURABLE, because staging decides what executes. A block delivers the
// contiguous run from Delivered+1 taken from this block's arrivals AND from
// what is already held, so a node holding less than its peers executes a
// shorter run: different Delivered, different account state, different BPT
// root. That is a divergent block hash, not a node briefly behind. Keeping this
// in memory would make every restart a consensus fault.
//
// NOT HASHED, because it does not need to be. It is a deterministic function of
// the consensus stream, so every node derives the same set from the same input.
// Hashing it is what forced it into an account's main state; main state is
// rewritten whole every block, which is what forced it to be BOUNDED —
// MaxPendingSequenced, 4,096. Past that bound the executor stored the message
// and refused to record that it had it, so the node held a message and reported
// not holding it, and healing fetched back across the partition what was
// already in the local database: 8,556 sequence numbers fetched 53,011 times in
// one twenty-minute soak, every partition live throughout.
//
// One record per number, outside the hash, has no such limit. The records are
// `state` rather than `index` so that snapshots collect them — a snapshot is
// what a new node starts from, and one without staging diverges on that node's
// first block.
//
// # Nothing is deleted
//
// Delivered is the cutoff, so an executed number needs no cleanup: it is below
// the watermark, and these records are only ever consulted above it. Releasing
// on delivery would be work that buys nothing, and getting its timing wrong —
// dropping an entry for a block that is then discarded — would make the node
// fetch back something it still holds, which is the failure this exists to
// remove, reintroduced from the other end.
//
// So Sequenced is simply the record that a number arrived, and what it was.

// Hold records that a message for this number has been received.
//
// Idempotent, and the FIRST sighting wins. A number can be offered twice — a
// block re-executed, a healed message racing the original — and both carry the
// same message, because the number identifies it. Keeping the first means the
// same input always produces the same state.
func Hold(batch *database.Batch, id StreamID, n uint64, txid *url.TxID) error {
	rec := batch.Account(id.Ledger).Sequenced(id.Source, n)
	switch v, err := rec.Get(); {
	case err != nil && !errors.Is(err, errors.NotFound):
		return errors.UnknownError.WithFormat("load sequenced %v/%d: %w", id.Source, n, err)

	case v == nil:
		err = rec.Put(txid)
		if err != nil {
			return errors.UnknownError.WithFormat("store sequenced %v/%d: %w", id.Source, n, err)
		}
	}

	high := batch.Account(id.Ledger).Sighted(id.Source)
	switch h, err := high.Get(); {
	case err != nil && !errors.Is(err, errors.NotFound):
		return errors.UnknownError.WithFormat("load sighted %v: %w", id.Source, err)

	case h < n:
		err = high.Put(n)
		if err != nil {
			return errors.UnknownError.WithFormat("store sighted %v: %w", id.Source, err)
		}
	}
	return nil
}

// IDOf returns the message recorded for a number, if there is one.
//
// It answers about the record alone. Whether a number is HELD — received and
// not yet executed — is that plus being above Delivered, and the watermark is
// the caller's.
func IDOf(batch *database.Batch, id StreamID, n uint64) (*url.TxID, bool, error) {
	txid, err := batch.Account(id.Ledger).Sequenced(id.Source, n).Get()
	switch {
	case errors.Is(err, errors.NotFound):
		return nil, false, nil
	case err != nil:
		return nil, false, errors.UnknownError.WithFormat("load sequenced %v/%d: %w", id.Source, n, err)
	}
	return txid, txid != nil, nil
}

// Sighted is the highest number ever received from a source.
//
// It says a stream is behind; it does not say what is missing, which is
// [Missing]. It is a high-water mark and does not go backwards as messages
// execute: "this stream was behind" is what makes a hole below it a hole, and
// forgetting it would say the stream had never been behind at all.
func Sighted(batch *database.Batch, id StreamID) (uint64, error) {
	n, err := batch.Account(id.Ledger).Sighted(id.Source).Get()
	switch {
	case errors.Is(err, errors.NotFound):
		return 0, nil
	case err != nil:
		return 0, errors.UnknownError.WithFormat("load sighted %v: %w", id.Source, err)
	}
	return n, nil
}

// Missing returns every contiguous run of numbers in (delivered, through] that
// nothing was received for, oldest first, at most maxRuns of them.
//
// A run rather than a number because a run is what one range request covers: a
// collection proof is a merkle range, so it proves a run of adjacent entries and
// not an arbitrary selection. Oldest first because delivery is in order — the
// stream advances the moment the oldest run fills, and keeps advancing as each
// next one lands.
//
// maxRuns bounds the answer and the scan. A stream far enough behind has one
// enormous run, which costs nothing to find; a stream behind and dense with
// holes is the case that would otherwise walk the whole distance to `through`.
func Missing(batch *database.Batch, id StreamID, delivered, through uint64, maxRuns int) ([][2]uint64, error) {
	if through <= delivered || maxRuns <= 0 {
		return nil, nil
	}

	var runs [][2]uint64
	open := false
	for n := delivered + 1; n <= through; n++ {
		_, held, err := IDOf(batch, id, n)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		if held {
			open = false
			continue
		}
		if open {
			runs[len(runs)-1][1] = n
			continue
		}
		if len(runs) == maxRuns {
			break
		}
		runs = append(runs, [2]uint64{n, n})
		open = true
	}
	return runs, nil
}
