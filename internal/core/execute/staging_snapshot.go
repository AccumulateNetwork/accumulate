// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// MaxSnapshotSpan bounds how many sequence numbers one page of a staging
// snapshot covers. A stage may hold thousands of entries — run
// 20260918T023054Z held 290 and 551 on the Directory's streams — and a
// synthetic message is not small, so the whole of it is not one message on
// the wire (healing spec, "Staging snapshot").
const MaxSnapshotSpan = 256

// Snapshot is staging as of the last committed block: one page of it,
// starting at the stream named by ledger and source and, within that stream,
// at number. A nil ledger starts at the first stream. Limit bounds how many
// sequence numbers the page covers; zero and anything above MaxSnapshotSpan
// mean MaxSnapshotSpan.
//
// The page and the block index are read under one lock, so what the page
// carries is what this node held when it committed that block and not a
// mixture of two blocks — a reader that pairs a page with a different block
// executes a different block (executor spec, "Sync" step 2). The block moves
// on between pages, though: each page says which block it is as of, and a
// reader whose pages disagree starts over.
//
// Block is zero when the node has committed no block. Staging is then not a
// snapshot of anything and the caller refuses to serve it.
func (s *Staging) Snapshot(ledger, source *url.URL, number, limit uint64) *private.StagingSnapshot {
	if limit == 0 || limit > MaxSnapshotSpan {
		limit = MaxSnapshotSpan
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	snap := &private.StagingSnapshot{Block: s.block}

	// Every stream staging knows, in key order, and after them a carrier for
	// each source that holds proofs and no stream: proofs are kept per
	// source, and a source whose entries have all executed may still have
	// one waiting for its anchor.
	type position struct {
		key string
		id  StreamID
	}
	var order []position
	for k, id := range s.ids {
		order = append(order, position{k, id})
	}
	seen := map[string]bool{}
	for _, p := range order {
		seen[sourceKey(p.id.Source)] = true
	}
	for k, m := range s.proofs {
		if seen[k] || len(m) == 0 {
			continue
		}
		order = append(order, position{"\xff" + k, StreamID{Source: s.sources[k]}})
	}
	sort.Slice(order, func(i, j int) bool { return order[i].key < order[j].key })

	// The first stream of a source carries that source's proofs, so a source
	// shared by a synthetic stream and an anchor stream does not send them
	// twice.
	owner := map[string]string{}
	for _, p := range order {
		k := sourceKey(p.id.Source)
		if _, ok := owner[k]; !ok {
			owner[k] = p.key
		}
	}

	start := 0
	if ledger != nil || source != nil {
		want := StreamID{Ledger: ledger, Source: source}.key()
		for start < len(order) && order[start].key < want {
			start++
		}
	} else {
		number = 0
	}

	budget := limit
	for i := start; i < len(order); i++ {
		p := order[i]
		st := s.streams[p.key]

		// Only the page's first stream starts part way through; every stream
		// after it starts at its own Delivered + 1.
		var from uint64
		if i == start {
			from = number
		}

		out := &private.StagedStream{Ledger: p.id.Ledger, Source: p.id.Source}
		var span uint64
		if st != nil {
			out.Delivered, out.Sighted = st.delivered, st.sighted
			span = uint64(len(st.entries))
			if n := uint64(len(st.validated)); n > span {
				span = n
			}
			if from <= st.delivered {
				from = st.delivered + 1
			}
		}

		// The source's proofs travel with the first page of its first
		// stream, where the reader can act on them before the entries they
		// prove arrive.
		if owner[sourceKey(p.id.Source)] == p.key && (st == nil || from == st.delivered+1) {
			for _, b := range sortedProofBlocks(s.proofs[sourceKey(p.id.Source)]) {
				for _, pr := range s.proofs[sourceKey(p.id.Source)][b] {
					out.Proofs = append(out.Proofs, &private.StagedProof{AnchorBlock: b, Proof: pr})
				}
			}
		}

		if st != nil && span > 0 {
			last := st.delivered + span
			if from+budget-1 < last {
				last = from + budget - 1
			}
			for n := from; n <= last; n++ {
				if h := st.entry(n); h != nil {
					out.Entries = append(out.Entries, &private.StagedEntry{
						Number:    n,
						Message:   h.Message,
						Companion: h.Companion,
						Collected: h.Collected,
						Hash:      h.Hash,
					})
				}
				if v, ok := st.hash(n); ok {
					out.Validated = append(out.Validated, &private.StagedHash{Number: n, Hash: v})
				}
			}
			budget -= last - from + 1
			if last < st.delivered+span {
				// The stream is not finished, so neither is the snapshot
				snap.Streams = append(snap.Streams, out)
				snap.NextLedger, snap.NextSource, snap.NextNumber = p.id.Ledger, p.id.Source, last+1
				return snap
			}
		}

		snap.Streams = append(snap.Streams, out)
		if budget == 0 && i+1 < len(order) {
			snap.NextLedger, snap.NextSource, snap.NextNumber = order[i+1].id.Ledger, order[i+1].id.Source, 0
			return snap
		}
	}
	return snap
}

func sortedProofBlocks(m map[uint64][]*protocol.AnnotatedReceipt) []uint64 {
	out := make([]uint64, 0, len(m))
	for b, ps := range m {
		if len(ps) > 0 {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Load puts a snapshot taken from a running validator into staging. It
// refuses staging that already holds something: a join starts from what its
// peer held, not from a mixture of that and whatever this node collected
// before it asked (executor spec, "Sync" step 2). A snapshot served in
// several pages is assembled by the reader — every page as of the same block
// — and loaded once.
//
// A held entry's ID is the message's own: every place that holds one holds
// it under Message.ID(), so the ID is derived rather than carried.
func (s *Staging) Load(snap *private.StagingSnapshot) error {
	if snap == nil {
		return errors.BadRequest.With("missing snapshot")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.streams) > 0 || len(s.proofs) > 0 {
		return errors.Conflict.With("staging is not empty")
	}

	for _, in := range snap.Streams {
		if in.Source == nil {
			return errors.BadRequest.With("staged stream has no source")
		}

		// Proofs are held per source, and the snapshot carries them on the
		// first stream of their source.
		sk := sourceKey(in.Source)
		for _, p := range in.Proofs {
			if p.Proof == nil {
				continue
			}
			if hasSameProof(s.proofs[sk][p.AnchorBlock], p.Proof) {
				continue
			}
			if s.proofs[sk] == nil {
				s.proofs[sk] = map[uint64][]*protocol.AnnotatedReceipt{}
			}
			s.proofs[sk][p.AnchorBlock] = append(s.proofs[sk][p.AnchorBlock], p.Proof)
			if _, ok := s.sources[sk]; !ok {
				s.sources[sk] = in.Source
			}
		}

		if in.Ledger == nil {
			continue // a carrier for a source that holds nothing but proofs
		}

		id := StreamID{Ledger: in.Ledger, Source: in.Source}
		k := id.key()
		st := s.streams[k]
		if st == nil {
			st = new(streamState)
			s.streams[k] = st
			s.ids[k] = id
		}
		if in.Delivered > st.delivered {
			st.delivered = in.Delivered
		}

		// Hashes first, as a block's commit does: an entry a validated hash
		// contradicts is not the stream's entry and is not held.
		for _, v := range in.Validated {
			st.validate(v.Number, v.Hash)
		}
		for _, e := range in.Entries {
			if e.Message == nil {
				return errors.BadRequest.WithFormat("staged entry %d of %v has no message", e.Number, in.Source)
			}
			if _, ok := st.at(e.Number); !ok {
				continue // at or below Delivered, or beyond the span
			}
			if v, ok := st.hash(e.Number); ok && e.Collected && v != e.Hash {
				continue
			}
			h := &Held{
				ID:        e.Message.ID(),
				Message:   e.Message,
				Companion: e.Companion,
				Collected: e.Collected,
				Hash:      e.Hash,
			}
			h.size = heldSize(h)
			st.hold(e.Number, h)
			st.keep(h)
			s.index(h)
		}
		if in.Sighted > st.sighted {
			st.sighted = in.Sighted
		}
		st.observe(k)
	}

	s.block = snap.Block
	return nil
}
