// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"sort"
	"strings"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Staging is the state a node builds up after it syncs with the protocol
// (executor spec, "Sync"): what it has received on each inbound stream and
// not yet executed, the collection proofs waiting for their Directory anchor,
// and the hashes those proofs have validated. It is memory. Nothing in it is
// written to the database; what is written is what executes, at the block's
// commit. It is fed only by consensus, so it is a deterministic function of
// the same input on every node, and a node that joins rebuilds it by
// collecting from consensus while it pulls the chains' state down.
//
// One stage per stream, and a stage is two lists indexed from Delivered + 1
// (executor spec, "One chain per pair, one stage per chain"): the entries
// held, and beside them the hashes collection proofs have validated at the
// same numbers. A proof covers one chain, so its index is the sequence
// number less one, and aligning the two lists is a walk, not a lookup.
// Anything at or below Delivered is dropped. The stage does not know which
// chain it serves.
//
// A block writes it through a Txn that commits with the block: the block's
// own arrivals are visible to the block's own run building, and a discarded
// block leaves nothing behind.
type Staging struct {
	mu      sync.Mutex
	streams map[string]*streamState
	proofs  map[string]map[uint64][]*protocol.AnnotatedReceipt // source -> anchor block
	sources map[string]*url.URL                                // the source URL as received, by key
	byID    map[[32]byte]*Held
	byTxn   map[[32]byte]*Held // held entries whose message carries a transaction, by its hash
}

// A StreamID names an inbound stream: the ledger that tracks it and the
// partition the messages come from.
type StreamID struct {
	Ledger *url.URL
	Source *url.URL
}

func (id StreamID) key() string {
	return strings.ToLower(id.Ledger.String()) + "|" + strings.ToLower(id.Source.String())
}

// A Held entry is a message received on a stream and not yet executed: the
// message as it arrived (what runs when its number is next), the transaction
// that travels with it when it has one, and whether it was collected without
// a validated proof — a collected entry never runs until a validated hash
// at its number matches it (executor spec, "Collection").
type Held struct {
	ID        *url.TxID
	Message   messaging.Message
	Companion messaging.Message
	Collected bool
	Hash      [32]byte // the sequenced message's hash, what a proof proves

	size int // the message's encoded size, measured once when it is held
}

// MaxStageSpan bounds how far above Delivered a stage grows: a number beyond
// it is not held and a hash there is not recorded. The sanity horizon is an
// hour of the source's production; this is well past it, and it is what
// keeps a forged sequence number from sizing the lists.
const MaxStageSpan = 4 << 20

// streamState is one stream's stage. entries[i] and validated[i] both stand
// for number delivered+1+i; a nil entry is a hole, a zero hash is unvalidated.
type streamState struct {
	delivered uint64
	entries   []*Held
	validated [][32]byte
	sighted   uint64 // the highest number ever held, executed or not

	// held and bytes are what the stream holds, for the gauges (#4233);
	// alarmed is whether the backlog has been reported and not yet cleared.
	held    int
	bytes   int
	alarmed bool
}

var zeroHash [32]byte

// at returns the offset of n in the lists, or false when n is at or below
// Delivered or beyond the span.
func (st *streamState) at(n uint64) (int, bool) {
	if n <= st.delivered || n-st.delivered > MaxStageSpan {
		return 0, false
	}
	return int(n - st.delivered - 1), true
}

func (st *streamState) entry(n uint64) *Held {
	i, ok := st.at(n)
	if !ok || i >= len(st.entries) {
		return nil
	}
	return st.entries[i]
}

func (st *streamState) hash(n uint64) ([32]byte, bool) {
	i, ok := st.at(n)
	if !ok || i >= len(st.validated) || st.validated[i] == zeroHash {
		return zeroHash, false
	}
	return st.validated[i], true
}

// reach is the highest number a validated hash stands at, or zero.
func (st *streamState) reach() uint64 {
	for i := len(st.validated) - 1; i >= 0; i-- {
		if st.validated[i] != zeroHash {
			return st.delivered + 1 + uint64(i)
		}
	}
	return 0
}

func (st *streamState) hold(n uint64, h *Held) {
	i, ok := st.at(n)
	if !ok {
		return
	}
	for len(st.entries) <= i {
		st.entries = append(st.entries, nil)
	}
	st.entries[i] = h
	if n > st.sighted {
		st.sighted = n
	}
}

func (st *streamState) validate(n uint64, h [32]byte) {
	i, ok := st.at(n)
	if !ok {
		return
	}
	for len(st.validated) <= i {
		st.validated = append(st.validated, zeroHash)
	}
	st.validated[i] = h
}

// release drops everything at or below n from both lists. The slices are
// re-sliced, and copied down once the dropped prefix outweighs what is kept,
// so the lists neither allocate per release nor pin what they have dropped.
func (st *streamState) release(n uint64, s *Staging) {
	if n <= st.delivered {
		return
	}
	drop := n - st.delivered
	if drop > uint64(len(st.entries)) {
		for _, h := range st.entries {
			st.forget(h, s)
		}
		// Released pointers are cleared, not merely cut off: a truncated
		// slice keeps its backing array, and every *Held in it would stay
		// reachable until overwritten — a drained backlog pinned for good
		// (review 2026-09-06, finding 3)
		clear(st.entries)
		st.entries = compactHeld(st.entries[:0])
	} else {
		for _, h := range st.entries[:drop] {
			st.forget(h, s)
		}
		clear(st.entries[:drop])
		st.entries = compactHeld(st.entries[drop:])
	}
	if drop > uint64(len(st.validated)) {
		st.validated = compactHashes(st.validated[:0])
	} else {
		st.validated = compactHashes(st.validated[drop:])
	}
	st.delivered = n
}

// keep counts a newly held entry; forget uncounts and unindexes a dropped
// one. The caller holds s.mu.
func (st *streamState) keep(h *Held) {
	st.held++
	st.bytes += h.size
}

func (st *streamState) forget(h *Held, s *Staging) {
	if h == nil {
		return
	}
	st.held--
	st.bytes -= h.size
	s.unindex(h)
}

// index and unindex keep the by-ID and by-transaction lookups in step with
// what the streams hold; the caller holds s.mu.
func (s *Staging) index(h *Held) {
	if h == nil {
		return
	}
	if h.ID != nil {
		s.byID[h.ID.Hash()] = h
	}
	if txn, ok := heldTransaction(h); ok {
		s.byTxn[*(*[32]byte)(txn.GetHash())] = h
	}
}

func (s *Staging) unindex(h *Held) {
	if h == nil {
		return
	}
	if h.ID != nil {
		delete(s.byID, h.ID.Hash())
	}
	if txn, ok := heldTransaction(h); ok {
		delete(s.byTxn, *(*[32]byte)(txn.GetHash()))
	}
}

// heldTransaction is the transaction a held message carries, if it carries
// one in full: a sequenced anchor, for instance.
func heldTransaction(h *Held) (*protocol.Transaction, bool) {
	txn, ok := messaging.UnwrapAs[messaging.MessageWithTransaction](h.Message)
	if !ok || txn.GetTransaction() == nil || txn.GetTransaction().Body == nil || txn.GetTransaction().Body.Type() == protocol.TransactionTypeRemote {
		return nil, false
	}
	return txn.GetTransaction(), true
}

func compactHeld(s []*Held) []*Held {
	if cap(s) > 2*len(s)+1024 {
		return append(make([]*Held, 0, len(s)), s...)
	}
	return s
}

func compactHashes(s [][32]byte) [][32]byte {
	if cap(s) > 2*len(s)+1024 {
		return append(make([][32]byte, 0, len(s)), s...)
	}
	return s
}

// NewStaging returns empty staging.
func NewStaging() *Staging {
	return &Staging{streams: map[string]*streamState{}, proofs: map[string]map[uint64][]*protocol.AnnotatedReceipt{}, sources: map[string]*url.URL{}, byID: map[[32]byte]*Held{}, byTxn: map[[32]byte]*Held{}}
}

// A StagingTxn is one block's view of staging: everything committed, plus
// what this block has added, minus what it has released. Commit publishes
// it; Discard drops it. The block's own additions are few and keyed by
// number; the lists live in the committed state.
type StagingTxn struct {
	s  *Staging
	mu sync.Mutex

	held      map[string]map[uint64]*Held
	validated map[string]map[uint64][32]byte
	sighted   map[string]uint64
	proofs    map[string]map[uint64][]*protocol.AnnotatedReceipt
	sources   map[string]*url.URL
	dropped   map[string]map[uint64]bool
	released  map[string]uint64
}

// Begin starts a block's transaction.
func (s *Staging) Begin() *StagingTxn {
	t := &StagingTxn{s: s}
	t.reset()
	return t
}

func sourceKey(source *url.URL) string { return strings.ToLower(source.String()) }

// Hold keeps a message at a number of a stream. The first sighting of a
// number wins: a number offered twice carries the same message. A collected
// entry that contradicts a hash already validated at its number is not held:
// it is not the stream's entry, and the hole it leaves is asked for again.
func (t *StagingTxn) Hold(id StreamID, n uint64, h *Held) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	k := id.key()
	if _, ok := t.held[k][n]; ok {
		return
	}
	t.s.mu.Lock()
	base := t.s.streams[k]
	var inBase, disproved bool
	if base != nil {
		if _, ok := base.at(n); !ok {
			t.s.mu.Unlock()
			return // at or below Delivered, or beyond the span
		}
		inBase = base.entry(n) != nil
		if v, ok := base.hash(n); ok && h.Collected && v != h.Hash {
			disproved = true
		}
	}
	t.s.mu.Unlock()
	if inBase || disproved {
		return
	}
	if v, ok := t.validated[k][n]; ok && h.Collected && v != h.Hash {
		return
	}
	if t.held[k] == nil {
		t.held[k] = map[uint64]*Held{}
	}
	t.held[k][n] = h
	if n > t.sighted[k] {
		t.sighted[k] = n
	}
}

// IDOf answers what is held at a number of a stream.
func (t *StagingTxn) IDOf(id StreamID, n uint64) (*Held, bool) {
	if t == nil {
		return nil, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	k := id.key()
	if h, ok := t.held[k][n]; ok {
		return h, true
	}
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	if base := t.s.streams[k]; base != nil {
		if h := base.entry(n); h != nil {
			return h, true
		}
	}
	return nil, false
}

// HeldByID answers a held entry by the ID it was held under.
func (t *StagingTxn) HeldByID(txid *url.TxID) (*Held, bool) {
	if t == nil || txid == nil {
		return nil, false
	}
	h := txid.Hash()
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, m := range t.held {
		for _, e := range m {
			if e.ID != nil && e.ID.Hash() == h {
				return e, true
			}
		}
	}
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	e, ok := t.s.byID[h]
	return e, ok
}

// HeldTransaction answers a transaction a held message carries in full, by
// its hash: what a placeholder in a later copy resolves against while the
// entry waits in its stage.
func (t *StagingTxn) HeldTransaction(hash [32]byte) (*protocol.Transaction, bool) {
	if t == nil {
		return nil, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, m := range t.held {
		for _, e := range m {
			if txn, ok := heldTransaction(e); ok && *(*[32]byte)(txn.GetHash()) == hash {
				return txn, true
			}
		}
	}
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	if h, ok := t.s.byTxn[hash]; ok {
		if txn, ok := heldTransaction(h); ok {
			return txn, true
		}
	}
	return nil, false
}

// Sighted is the highest number ever held on a stream, executed or not.
func (t *StagingTxn) Sighted(id StreamID) uint64 {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	k := id.key()
	n := t.sighted[k]
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	if base := t.s.streams[k]; base != nil && base.sighted > n {
		n = base.sighted
	}
	return n
}

// Reach is the highest number a validated hash stands at on a stream, above
// Delivered, or zero. Entries missing below it are a gap of entries.
func (t *StagingTxn) Reach(id StreamID) uint64 {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	k := id.key()
	var n uint64
	for m := range t.validated[k] {
		if m > n {
			n = m
		}
	}
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	if base := t.s.streams[k]; base != nil {
		if r := base.reach(); r > n {
			n = r
		}
	}
	return n
}

// Missing lists the runs of numbers above delivered and through the given
// number that nothing is held for, oldest first, up to maxRuns.
func (t *StagingTxn) Missing(id StreamID, delivered, through uint64, maxRuns int) [][2]uint64 {
	if through <= delivered || maxRuns <= 0 {
		return nil
	}
	var runs [][2]uint64
	open := false
	for n := delivered + 1; n <= through; n++ {
		if _, held := t.IDOf(id, n); held {
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
	return runs
}

// Prove records a validated collection proof's hashes at their numbers: the
// proof covers one chain, so element i of a proof starting at chain index s
// is sequence number s+i+1. Where a hash is already validated at a number the
// two must agree: two proofs claiming different hashes for one number are an
// attack on the stream, the first stands and the second proves nothing
// (errors.Conflict). A collected entry the proof contradicts is dropped, so
// the number is a hole and is asked for again. Numbers at or below Delivered
// are tossed.
func (t *StagingTxn) Prove(id StreamID, list *merkle.ReceiptList) error {
	if t == nil {
		return nil
	}
	start := countFromPending(list.MerkleState)
	if start < 0 || start != list.MerkleState.Count {
		return errors.BadRequest.WithFormat("collection proof state is inconsistent: count is %d but the structure holds %d", list.MerkleState.Count, start)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	k := id.key()
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	base := t.s.streams[k]
	for i, el := range list.Elements {
		n := uint64(start) + uint64(i) + 1
		var h [32]byte
		copy(h[:], el)
		if have, ok := t.validated[k][n]; ok && have != h {
			return errors.Conflict.WithFormat("conflicting proof for %v: %d is validated as %x, proof says %x", id.Source, n, have[:4], h[:4])
		}
		if base != nil {
			if have, ok := base.hash(n); ok && have != h {
				return errors.Conflict.WithFormat("conflicting proof for %v: %d is validated as %x, proof says %x", id.Source, n, have[:4], h[:4])
			}
		}
	}
	if t.validated[k] == nil {
		t.validated[k] = map[uint64][32]byte{}
	}
	for i, el := range list.Elements {
		n := uint64(start) + uint64(i) + 1
		if base != nil {
			if _, ok := base.at(n); !ok {
				continue
			}
		}
		var h [32]byte
		copy(h[:], el)
		t.validated[k][n] = h
		if e, ok := t.held[k][n]; ok && e.Collected && e.Hash != h {
			delete(t.held[k], n)
		}
	}
	return nil
}

// IsValidated reports whether the hash validated at a number of a stream is
// this one: what makes a collected entry runnable.
func (t *StagingTxn) IsValidated(id StreamID, n uint64, hash [32]byte) bool {
	v, ok := t.Validated(id, n)
	return ok && v == hash
}

// Validated is the hash validated at a number of a stream, if any.
func (t *StagingTxn) Validated(id StreamID, n uint64) ([32]byte, bool) {
	if t == nil {
		return zeroHash, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	k := id.key()
	if h, ok := t.validated[k][n]; ok {
		return h, true
	}
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	if base := t.s.streams[k]; base != nil {
		return base.hash(n)
	}
	return zeroHash, false
}

// StageProof holds a collection proof from a source under the Directory
// anchor block it terminates in, until that anchor executes here. A proof
// identical to one already waiting under that block — the same first index
// and the same element count — is not held twice: under Directory-anchor lag
// the same package proof arrives with every copy of its members, and stacking
// them is memory for nothing (review 2026-09-06, finding 28). Answers whether
// the proof was held.
func (t *StagingTxn) StageProof(source *url.URL, block uint64, proof *protocol.AnnotatedReceipt) bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	k := sourceKey(source)
	if t.dropped[k][block] {
		return false
	}
	t.s.mu.Lock()
	dup := hasSameProof(t.s.proofs[k][block], proof)
	t.s.mu.Unlock()
	if dup || hasSameProof(t.proofs[k][block], proof) {
		return false
	}
	if t.proofs[k] == nil {
		t.proofs[k] = map[uint64][]*protocol.AnnotatedReceipt{}
	}
	t.proofs[k][block] = append(t.proofs[k][block], proof)
	if _, ok := t.sources[k]; !ok {
		t.sources[k] = source
	}
	return true
}

// hasSameProof reports whether the list holds a proof over the same span as
// this one: the same starting count and the same number of elements.
func hasSameProof(list []*protocol.AnnotatedReceipt, proof *protocol.AnnotatedReceipt) bool {
	if proof == nil || proof.ReceiptList == nil || proof.ReceiptList.MerkleState == nil {
		return false
	}
	for _, p := range list {
		if p == nil || p.ReceiptList == nil || p.ReceiptList.MerkleState == nil {
			continue
		}
		if p.ReceiptList.MerkleState.Count == proof.ReceiptList.MerkleState.Count &&
			len(p.ReceiptList.Elements) == len(proof.ReceiptList.Elements) {
			return true
		}
	}
	return false
}

// StagedProofSpans lists the number spans [first, last] the proofs waiting
// for their Directory anchor from a source cover, in no particular order. A
// proof starting at chain count s with k elements covers numbers s+1..s+k.
// An entry under such a span is not a gap: its proof has arrived and its
// anchor is on its way (healing spec, "Deciding, in staging").
func (t *StagingTxn) StagedProofSpans(source *url.URL) [][2]uint64 {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	k := sourceKey(source)
	var spans [][2]uint64
	add := func(m map[uint64][]*protocol.AnnotatedReceipt) {
		for b, ps := range m {
			if t.dropped[k][b] {
				continue
			}
			for _, p := range ps {
				if p == nil || p.ReceiptList == nil || p.ReceiptList.MerkleState == nil || len(p.ReceiptList.Elements) == 0 {
					continue
				}
				start := uint64(p.ReceiptList.MerkleState.Count)
				spans = append(spans, [2]uint64{start + 1, start + uint64(len(p.ReceiptList.Elements))})
			}
		}
	}
	t.s.mu.Lock()
	add(t.s.proofs[k])
	t.s.mu.Unlock()
	add(t.proofs[k])
	return spans
}

// ProofBlocks lists the Directory anchor blocks a source has proofs waiting
// on, ascending.
func (t *StagingTxn) ProofBlocks(source *url.URL) []uint64 {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	k := sourceKey(source)
	set := map[uint64]bool{}
	t.s.mu.Lock()
	for b, ps := range t.s.proofs[k] {
		if len(ps) > 0 {
			set[b] = true
		}
	}
	t.s.mu.Unlock()
	for b, ps := range t.proofs[k] {
		if len(ps) > 0 {
			set[b] = true
		}
	}
	for b := range t.dropped[k] {
		delete(set, b)
	}
	out := make([]uint64, 0, len(set))
	for b := range set {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Proofs lists the proofs a source has waiting on a Directory anchor block.
func (t *StagingTxn) Proofs(source *url.URL, block uint64) []*protocol.AnnotatedReceipt {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	k := sourceKey(source)
	if t.dropped[k][block] {
		return nil
	}
	var out []*protocol.AnnotatedReceipt
	t.s.mu.Lock()
	out = append(out, t.s.proofs[k][block]...)
	t.s.mu.Unlock()
	out = append(out, t.proofs[k][block]...)
	return out
}

// DropProofs forgets a source's proofs for an anchor block, once the anchor
// has decided them.
func (t *StagingTxn) DropProofs(source *url.URL, block uint64) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	k := sourceKey(source)
	if t.dropped[k] == nil {
		t.dropped[k] = map[uint64]bool{}
	}
	t.dropped[k][block] = true
	delete(t.proofs[k], block)
}

// ProofSources lists the sources with proofs waiting, as their URLs were
// received — a stream is keyed case-insensitively, but the URL handed back
// must be the one the executor uses, or two positions are kept for one
// stream.
func (t *StagingTxn) ProofSources() []*url.URL {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	seen := map[string]*url.URL{}
	add := func(k string, m map[uint64][]*protocol.AnnotatedReceipt, u *url.URL) {
		for b, ps := range m {
			if len(ps) == 0 || t.dropped[k][b] {
				continue
			}
			if _, ok := seen[k]; !ok && u != nil {
				seen[k] = u
			}
		}
	}
	t.s.mu.Lock()
	for k, m := range t.s.proofs {
		add(k, m, t.s.sources[k])
	}
	t.s.mu.Unlock()
	for k, m := range t.proofs {
		u := t.sources[k]
		if u == nil {
			t.s.mu.Lock()
			u = t.s.sources[k]
			t.s.mu.Unlock()
		}
		add(k, m, u)
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]*url.URL, 0, len(keys))
	for _, k := range keys {
		out = append(out, seen[k])
	}
	return out
}

// Release records that the block delivered a stream through n: at commit,
// everything at or below n is dropped from both lists.
func (t *StagingTxn) Release(id StreamID, n uint64) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if n > t.released[id.key()] {
		t.released[id.key()] = n
	}
}

// Commit publishes the block's additions and applies its releases.
func (t *StagingTxn) Commit() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.s
	s.mu.Lock()
	defer s.mu.Unlock()
	stream := func(k string) *streamState {
		st := s.streams[k]
		if st == nil {
			st = new(streamState)
			s.streams[k] = st
		}
		return st
	}
	touched := map[string]bool{}
	for k, m := range t.validated {
		st := stream(k)
		for n, h := range m {
			st.validate(n, h)
			if e := st.entry(n); e != nil && e.Collected && e.Hash != h {
				// Contradicted by the proof: not the stream's entry
				st.forget(e, s)
				st.entries[n-st.delivered-1] = nil
				touched[k] = true
			}
		}
	}
	for k, m := range t.held {
		st := stream(k)
		for n, h := range m {
			if st.entry(n) != nil {
				continue // first sighting wins
			}
			if v, ok := st.hash(n); ok && h.Collected && v != h.Hash {
				continue
			}
			h.size = heldSize(h)
			st.hold(n, h)
			st.keep(h)
			s.index(h)
			touched[k] = true
		}
	}
	for k, n := range t.sighted {
		st := stream(k)
		if n > st.sighted {
			st.sighted = n
		}
	}
	for k, dropped := range t.dropped {
		for b := range dropped {
			delete(s.proofs[k], b)
		}
	}
	for k, m := range t.proofs {
		if s.proofs[k] == nil {
			s.proofs[k] = map[uint64][]*protocol.AnnotatedReceipt{}
		}
		for b, ps := range m {
			s.proofs[k][b] = append(s.proofs[k][b], ps...)
		}
		if _, ok := s.sources[k]; !ok {
			s.sources[k] = t.sources[k]
		}
	}
	for k, n := range t.released {
		if st := s.streams[k]; st != nil {
			st.release(n, s)
			touched[k] = true
		}
	}
	for k := range touched {
		s.streams[k].observe(k)
	}
	t.reset()
}

// Discard drops the block's additions.
func (t *StagingTxn) Discard() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.reset()
}

// reset empties the transaction; the caller holds t.mu.
func (t *StagingTxn) reset() {
	t.held = map[string]map[uint64]*Held{}
	t.validated = map[string]map[uint64][32]byte{}
	t.sighted = map[string]uint64{}
	t.proofs = map[string]map[uint64][]*protocol.AnnotatedReceipt{}
	t.sources = map[string]*url.URL{}
	t.dropped = map[string]map[uint64]bool{}
	t.released = map[string]uint64{}
}

var (
	registryMu sync.Mutex
	registry   = map[string]*Staging{}
)

// RegisterStaging names a partition's staging for readers that have only the
// partition's name — the node's API, reporting how far a stream has been
// sighted. A process running several networks (the simulator) does not use
// it; it hands each service its staging directly.
func RegisterStaging(partitionID string, s *Staging) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[strings.ToLower(partitionID)] = s
}

// StagingFor answers RegisterStaging.
func StagingFor(partitionID string) *Staging {
	registryMu.Lock()
	defer registryMu.Unlock()
	return registry[strings.ToLower(partitionID)]
}

// SightedOn answers, outside any block, how far a stream has been sighted:
// what the API reports as Received.
func (s *Staging) SightedOn(id StreamID) uint64 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.streams[id.key()]; st != nil {
		return st.sighted
	}
	return 0
}

// countFromPending derives a merkle state's count from its pending list,
// the structural check a proof's state must pass (#4106, #4152).
func countFromPending(s *merkle.State) int64 {
	if len(s.Pending) > 62 {
		return -1
	}
	var count int64
	for i, v := range s.Pending {
		if len(v) > 0 {
			count |= 1 << i
		}
	}
	return count
}
