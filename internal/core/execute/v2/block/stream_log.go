// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"path"
	"sort"
	"strings"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// What a stream logs (executor spec, "What a stream logs").
//
// A stall on a synthetic or anchor stream could not be debugged from the
// node log: nothing recorded where a stream stood block by block, so
// "delivered stopped at 32,469 on the Directory" was a fact read off a
// dashboard after the event, with no record of when it stopped, what it
// was waiting on, or what the source had produced by then (#4279, run
// 20260915T042428Z). These two lines are that record: one per stream per
// block that moved or is behind, one per destination per block that
// produced, at Info, so every node's log carries its own streams' history
// and test/docker/soak/streamlog.py can read a stall out of it.

// StreamLogEvery is how many blocks may pass without a line for a stream.
// A healthy stream logs at this cadence and no more; a stream in trouble
// logs whenever anything about it changes. Without the cadence a caught-up
// network writes a line per stream per block forever (~24/s on the soak's
// eight nodes at a one-second block), and without the change rule a frozen
// stall writes the same line every block for as long as it lasts -- the
// #4182 log-volume mistake in both directions.
const StreamLogEvery = 60

// streamLogState is the last line written for a stream, so the next block
// can tell whether anything changed. Under the executor because it outlives
// the block; written only from the block's serial close.
type streamLogState struct {
	mu   sync.Mutex
	last map[string]streamLogEntry
}

type streamLogEntry struct {
	block                                       uint64
	delivered, sighted, reach, waiting, advance uint64
	received                                    uint64
	held                                        int
	behind                                      bool
}

// logStreams writes one `Stream position` line per stream, when something
// about the stream changed or the cadence has elapsed, after flushStreams
// has written Delivered.
func (b *Block) logStreams() {
	from := map[string]uint64{}
	b.positions.mu.Lock()
	for k, p := range b.positions.m {
		from[k] = p.delivered
	}
	b.positions.mu.Unlock()

	s := &b.Executor.streamLog
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last == nil {
		s.last = map[string]streamLogEntry{}
	}

	// The ledger's Received, as flushStreams left it (#4412). It is logged
	// beside staging's sighted because the two can disagree, and on a node
	// that rejoined behind a hole its peers hold entries behind, that
	// disagreement is the diagnosis: the state says N arrived, this node's
	// staging has sighted less.
	ledgers := map[string]protocol.SequenceLedger{}
	//
	// Delivered is the larger of staging's and the ledger's: a stage that
	// holds nothing has no Delivered of its own (it is zero after a
	// restart), and the ledger's is the one that counts.
	ledgerOf := func(id execute.StreamID) (delivered, received uint64) {
		if b.Batch == nil {
			return 0, 0
		}
		lk := strings.ToLower(id.Ledger.String())
		l, ok := ledgers[lk]
		if !ok {
			if err := b.Batch.Account(id.Ledger).Main().GetAs(&l); err != nil {
				l = nil
			}
			ledgers[lk] = l
		}
		if l == nil {
			return 0, 0
		}
		// FindPartition, never Partition: the ledger is the batch's
		// memoized record, and Partition inserts an entry for a stream only
		// this node's staging knows into hashed state (#4412 review F7).
		part, ok := l.FindPartition(id.Source)
		if !ok {
			return 0, 0
		}
		return part.Delivered, part.Received
	}

	for _, st := range b.staging.Streams() {
		k := stream{ledger: st.ID.Ledger, source: st.ID.Source}.key()
		ledgerDelivered, received := ledgerOf(st.ID)
		st.Delivered = max(st.Delivered, ledgerDelivered)
		before, seen := from[k]
		if !seen {
			before = st.Delivered
		}
		var advanced uint64
		if st.Delivered > before {
			advanced = st.Delivered - before
		}

		cur := streamLogEntry{block: b.Index, delivered: st.Delivered, sighted: st.Sighted, received: received,
			reach: st.Reach, waiting: st.Waiting, advance: advanced, held: st.Held,
			behind: st.Behind() || received > st.Delivered}
		prev, known := s.last[k]
		// A stream nobody has ever had anything to say about stays silent:
		// caught up, not advancing, never logged.
		if !known && advanced == 0 && !cur.behind {
			continue
		}
		changed := !known || prev.delivered != cur.delivered || prev.sighted != cur.sighted || prev.received != cur.received ||
			prev.reach != cur.reach || prev.waiting != cur.waiting || prev.held != cur.held ||
			prev.behind != cur.behind
		due := b.Index >= prev.block+StreamLogEvery
		if known && !changed && !due {
			continue
		}
		// Caught up and merely ticking along: the cadence is the whole
		// record. Behind, or changing, every block is worth a line.
		if known && !cur.behind && !prev.behind && !due {
			continue
		}
		s.last[k] = cur
		b.Executor.logger.Info("Stream position", "module", "stream",
			"block", b.Index, "ledger", ledgerKind(st.ID.Ledger), "source", partitionLabel(st.ID.Source),
			"delivered", st.Delivered, "advanced", advanced, "received", received, "sighted", st.Sighted,
			"reach", st.Reach, "held", st.Held, "waiting", st.Waiting)
	}
}

// logProduced writes one `Stream produced` line per destination this block
// sequenced synthetics for: the numbers it assigned, so a gap between one
// block's `to` and the next block's `from` shows in the producer's own log.
func (b *Block) logProduced() {
	if b.cacheBlock == nil || len(b.cacheBlock.Entries) == 0 {
		return
	}
	type span struct{ lo, hi, n uint64 }
	spans := map[string]*span{}
	dests := map[string]*url.URL{}
	for _, e := range b.cacheBlock.Entries {
		k := e.Stream.String()
		sp := spans[k]
		if sp == nil {
			sp = &span{lo: e.Number, hi: e.Number}
			spans[k] = sp
			dests[k] = e.Stream
		}
		if e.Number < sp.lo {
			sp.lo = e.Number
		}
		if e.Number > sp.hi {
			sp.hi = e.Number
		}
		sp.n++
	}
	keys := make([]string, 0, len(spans))
	for k := range spans {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sp := spans[k]
		b.Executor.logger.Info("Stream produced", "module", "stream",
			"block", b.Index, "destination", partitionLabel(dests[k]),
			"from", sp.lo, "to", sp.hi, "count", sp.n)
	}
}

// ledgerKind names a stream's ledger by its last path element: synthetic or
// anchors.
func ledgerKind(u *url.URL) string {
	if u == nil {
		return "?"
	}
	return path.Base(u.Path)
}

// partitionLabel names a partition the way the dashboard does -- Directory,
// BVN1. Anything that is not a partition URL is printed whole.
func partitionLabel(u *url.URL) string {
	if u == nil {
		return "?"
	}
	if id, ok := protocol.ParsePartitionUrl(u); ok {
		return id
	}
	return u.String()
}
