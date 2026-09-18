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

// MaxSnapshotBytes bounds what one page costs to build, hold and send.
//
// A span bound alone does not bound a page: entries vary in size, and one
// source may hold up to MaxStagedProofBytes of waiting proofs, which are
// charged against no span at all because they stand at no sequence number. A
// page that carried a source's whole proof budget would be hundreds of
// megabytes — read into memory twice, once by the server and once by the
// reader — for a call any peer may make (#4291 review).
//
// The budget is advisory in one direction only: a page always carries at
// least one entry or one proof, however large, so paging always advances.
//
// It is a var only so a test can lower it; nothing changes it at run time.
var MaxSnapshotBytes = 4 << 20

// Snapshot is staging as of the last committed block: one page of it,
// starting where the request says.
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
//
// The second return is what the page costs in bytes, measured as it is built
// — the metric the server counts, so that counting it does not mean encoding
// the largest message the node sends a second time (#4291 review).
//
// A request that names a position that does not exist — a Ledger or a Number
// without a Source — is served from the beginning rather than being guessed
// at. Servers refuse it first ([private.StagingSnapshotRequest.Validate]);
// this method only declines to panic on it.
func (s *Staging) Snapshot(req *private.StagingSnapshotRequest) (*private.StagingSnapshot, int) {
	if req == nil {
		req = new(private.StagingSnapshotRequest)
	}
	limit := req.Limit
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
		order = append(order, position{carrierKey(k), StreamID{Source: s.sources[k]}})
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
	var number, offset uint64
	if req.Source != nil {
		want := snapshotKey(req.Ledger, req.Source)
		for start < len(order) && order[start].key < want {
			start++
		}
		number, offset = req.Number, req.ProofOffset
	}

	budget := limit // sequence numbers left in this page
	bytes := 0      // what the page has cost so far
	empty := true   // nothing has been put in it yet

	// more ends the page at a position and says where the next one starts.
	more := func(out *private.StagedStream, id StreamID, number, offset uint64) (*private.StagingSnapshot, int) {
		if out != nil {
			snap.Streams = append(snap.Streams, out)
		}
		snap.More = true
		snap.NextLedger, snap.NextSource = id.Ledger, id.Source
		snap.NextNumber, snap.NextProofOffset = number, offset
		return snap, bytes
	}

	for i := start; i < len(order); i++ {
		p := order[i]

		// A page that is full stops at the start of the next stream, which is
		// a position the cursor can name whether or not that stream has a
		// ledger: More says there is a next page, so a nil NextLedger is a
		// carrier and not an ending (#4291 review).
		if i > start && (budget == 0 || bytes >= MaxSnapshotBytes) {
			return more(nil, p.id, 0, 0)
		}

		st := s.streams[p.key]

		// Only the page's first position starts part way through; every
		// position after it starts at its own beginning.
		var from uint64
		var skip uint64
		if i == start {
			from, skip = number, offset
		}

		out := &private.StagedStream{Ledger: p.id.Ledger, Source: p.id.Source}
		var end uint64 // the highest number the stream stages
		var stages bool
		if st != nil {
			out.Delivered, out.Sighted = st.delivered, st.sighted
			span := uint64(len(st.entries))
			if n := uint64(len(st.validated)); n > span {
				span = n
			}
			if from <= st.delivered {
				from = st.delivered + 1
			}
			if span > 0 {
				end, stages = st.delivered+span, true
			}
		}

		// The source's proofs travel with the first page of its first
		// stream, where the reader can act on them before the entries they
		// prove arrive. They are charged bytes, not numbers: a proof stands
		// at no sequence number, and one source may hold up to
		// MaxStagedProofBytes of them.
		sk := sourceKey(p.id.Source)
		if owner[sk] == p.key && (st == nil || from == st.delivered+1) {
			proofs := s.stagedProofs(sk)
			n := int(min(skip, uint64(len(proofs))))
			for ; n < len(proofs); n++ {
				size := proofSize(proofs[n].Proof)
				if !empty && bytes+size > MaxSnapshotBytes {
					break
				}
				out.Proofs = append(out.Proofs, proofs[n])
				bytes += size
				empty = false
			}
			if n < len(proofs) {
				// The source's proofs are not finished, so neither is the
				// snapshot: the next page resumes at the same position with
				// the proofs the reader already has counted off.
				return more(out, p.id, from, uint64(n))
			}
			skip = uint64(len(proofs))
		}

		if stages && from <= end {
			next := from
			var span uint64
			for next <= end && span < budget {
				if !empty && bytes >= MaxSnapshotBytes {
					break
				}
				if h := st.entry(next); h != nil {
					out.Entries = append(out.Entries, &private.StagedEntry{
						Number:    next,
						Message:   h.Message,
						Companion: h.Companion,
						Collected: h.Collected,
						Hash:      h.Hash,
					})
					bytes += h.size
					empty = false
				}
				if v, ok := st.hash(next); ok {
					out.Validated = append(out.Validated, &private.StagedHash{Number: next, Hash: v})
					bytes += stagedHashSize
					empty = false
				}
				span++
				next++
			}
			budget -= span
			if next <= end {
				// The stream is not finished, so neither is the snapshot
				return more(out, p.id, next, skip)
			}
		}
		// A start number past the end of the stream carries nothing and
		// costs nothing. Charging it would underflow the budget and serve
		// the whole stage in one page (#4291 review).

		snap.Streams = append(snap.Streams, out)
	}
	return snap, bytes
}

// stagedHashSize is what a validated hash costs on the wire: the hash and its
// number.
const stagedHashSize = 32 + 8

// carrierKey orders the carrier of a source that holds proofs and no stream
// after every real stream. No ledger URL sorts there, so the carrier's key
// cannot collide with a stream's.
func carrierKey(source string) string { return "\xff" + source }

// snapshotKey is the order key of the position a request names. A request
// may name a carrier, whose ledger is nil, so this is not StreamID.key —
// which dereferences the ledger and panics (#4291 review).
func snapshotKey(ledger, source *url.URL) string {
	if ledger == nil {
		return carrierKey(sourceKey(source))
	}
	return StreamID{Ledger: ledger, Source: source}.key()
}

// stagedProofs is a source's waiting proofs in the order a page sends them:
// by the anchor block they wait on, and within a block in the order they were
// staged. The caller holds s.mu.
func (s *Staging) stagedProofs(source string) []*private.StagedProof {
	var out []*private.StagedProof
	for _, b := range sortedProofBlocks(s.proofs[source]) {
		for _, pr := range s.proofs[source][b] {
			out = append(out, &private.StagedProof{AnchorBlock: b, Proof: pr})
		}
	}
	return out
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
//
// The whole snapshot is built aside and published only once all of it has
// loaded. A snapshot with one bad entry in it therefore leaves staging empty
// and the load can be tried again, against the same peer or another one; a
// load that wrote as it went would wedge a joining node on the first
// malformed entry, because a half-loaded staging is not empty and Load
// refuses one that is not (#4291 review).
func (s *Staging) Load(snap *private.StagingSnapshot) error {
	if snap == nil {
		return errors.BadRequest.With("missing snapshot")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.streams) > 0 || len(s.proofs) > 0 {
		return errors.Conflict.With("staging is not empty")
	}

	// Built aside, published at the end.
	load := &Staging{
		block:   snap.Block,
		streams: map[string]*streamState{},
		ids:     map[string]StreamID{},
		proofs:  map[string]map[uint64][]*protocol.AnnotatedReceipt{},
		sources: map[string]*url.URL{},
		byID:    map[[32]byte]*Held{},
		byTxn:   map[[32]byte]*Held{},
	}
	proofBytes := map[string]int{}

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
			if hasSameProof(load.proofs[sk][p.AnchorBlock], p.Proof) {
				continue
			}

			// A peer's proofs are bounded as this node's own are: the same
			// budget, in the same currency, for the same reason — a source
			// cannot grow this without bound (#4282, #4291 review).
			size := proofSize(p.Proof)
			if proofBytes[sk]+size > MaxStagedProofBytes {
				return errors.BadRequest.WithFormat("staged proofs for %v exceed %d bytes", in.Source, MaxStagedProofBytes)
			}
			proofBytes[sk] += size

			if load.proofs[sk] == nil {
				load.proofs[sk] = map[uint64][]*protocol.AnnotatedReceipt{}
			}
			load.proofs[sk][p.AnchorBlock] = append(load.proofs[sk][p.AnchorBlock], p.Proof)
			if _, ok := load.sources[sk]; !ok {
				load.sources[sk] = in.Source
			}
		}

		if in.Ledger == nil {
			continue // a carrier for a source that holds nothing but proofs
		}

		id := StreamID{Ledger: in.Ledger, Source: in.Source}
		k := id.key()
		st := load.streams[k]
		if st == nil {
			st = new(streamState)
			load.streams[k] = st
			load.ids[k] = id
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
			load.index(h)
		}
		if in.Sighted > st.sighted {
			st.sighted = in.Sighted
		}
	}

	// Published: from here nothing can fail.
	s.block = load.block
	s.streams = load.streams
	s.ids = load.ids
	s.proofs = load.proofs
	s.sources = load.sources
	s.byID = load.byID
	s.byTxn = load.byTxn
	for k, st := range s.streams {
		st.observe(k)
	}
	return nil
}
