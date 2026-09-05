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
// and the index ranges those proofs have proven. It is memory. Nothing in it
// is written to the database; what is written is what executes, at the
// block's commit. It is fed only by consensus, so it is a deterministic
// function of the same input on every node, and a node that joins rebuilds
// it by collecting from consensus while it pulls the chains' state down.
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
// a validated proof — a collected entry never runs until the proven set
// covers its hash (executor spec, "Collection").
type Held struct {
	ID        *url.TxID
	Message   messaging.Message
	Companion messaging.Message
	Collected bool
	Hash      [32]byte // the sequenced message's hash, what a proof proves
}

type streamState struct {
	held    map[uint64]*Held
	sighted uint64
	// The proven set: the source chain's hashes by index, from validated
	// proofs. Both directions, so a later proof can extend it backwards.
	proven      map[int64][32]byte
	provenIndex map[[32]byte]int64
}

func newStreamState() *streamState {
	return &streamState{held: map[uint64]*Held{}, proven: map[int64][32]byte{}, provenIndex: map[[32]byte]int64{}}
}

// NewStaging returns empty staging.
func NewStaging() *Staging {
	return &Staging{streams: map[string]*streamState{}, proofs: map[string]map[uint64][]*protocol.AnnotatedReceipt{}, sources: map[string]*url.URL{}, byID: map[[32]byte]*Held{}}
}

func (s *Staging) stream(id StreamID) *streamState {
	st := s.streams[id.key()]
	if st == nil {
		st = newStreamState()
		s.streams[id.key()] = st
	}
	return st
}

// A StagingTxn is one block's view of staging: everything committed, plus
// what this block has added, minus what it has released. Commit publishes
// it; Discard drops it.
type StagingTxn struct {
	s  *Staging
	mu sync.Mutex

	held     map[string]map[uint64]*Held
	sighted  map[string]uint64
	proven   map[string]map[int64][32]byte
	proofs   map[string]map[uint64][]*protocol.AnnotatedReceipt
	sources  map[string]*url.URL
	dropped  map[string]map[uint64]bool
	released map[string]uint64
}

// Begin starts a block's transaction.
func (s *Staging) Begin() *StagingTxn {
	return &StagingTxn{
		s:        s,
		held:     map[string]map[uint64]*Held{},
		sighted:  map[string]uint64{},
		proven:   map[string]map[int64][32]byte{},
		proofs:   map[string]map[uint64][]*protocol.AnnotatedReceipt{},
		sources:  map[string]*url.URL{},
		dropped:  map[string]map[uint64]bool{},
		released: map[string]uint64{},
	}
}

func sourceKey(source *url.URL) string { return strings.ToLower(source.String()) }

// Hold keeps a message at a number of a stream. The first sighting of a
// number wins: a number offered twice carries the same message.
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
	inBase := base != nil && base.held[n] != nil
	t.s.mu.Unlock()
	if inBase {
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
		if h, ok := base.held[n]; ok {
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

// Sighted is the highest number seen on a stream, executed or not.
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

// Prove records a validated collection proof's elements in the stream's
// proven set. Where the proof overlaps what is already proven the hashes
// must agree: two proofs claiming the same index with different hashes are
// an attack on the stream, the first stands and the second proves nothing
// (errors.Conflict). A proof below what is proven extends the set backwards.
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
		idx := start + int64(i)
		var h [32]byte
		copy(h[:], el)
		if have, ok := t.proven[k][idx]; ok && have != h {
			return errors.Conflict.WithFormat("conflicting proof for %v: index %d is proven as %x, proof says %x", id.Source, idx, have[:4], h[:4])
		}
		if base != nil {
			if have, ok := base.proven[idx]; ok && have != h {
				return errors.Conflict.WithFormat("conflicting proof for %v: index %d is proven as %x, proof says %x", id.Source, idx, have[:4], h[:4])
			}
		}
	}
	if t.proven[k] == nil {
		t.proven[k] = map[int64][32]byte{}
	}
	for i, el := range list.Elements {
		var h [32]byte
		copy(h[:], el)
		t.proven[k][start+int64(i)] = h
	}
	return nil
}

// IsProven reports whether a hash is in the stream's proven set.
func (t *StagingTxn) IsProven(id StreamID, hash [32]byte) bool {
	_, ok := t.ProvenIndex(id, hash)
	return ok
}

// ProvenIndex is the source chain index a proven hash sits at.
func (t *StagingTxn) ProvenIndex(id StreamID, hash [32]byte) (int64, bool) {
	if t == nil {
		return 0, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	k := id.key()
	for idx, h := range t.proven[k] {
		if h == hash {
			return idx, true
		}
	}
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	if base := t.s.streams[k]; base != nil {
		if idx, ok := base.provenIndex[hash]; ok {
			return idx, true
		}
	}
	return 0, false
}

// StageProof holds a collection proof from a source under the Directory
// anchor block it terminates in, until that anchor executes here.
func (t *StagingTxn) StageProof(source *url.URL, block uint64, proof *protocol.AnnotatedReceipt) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	k := sourceKey(source)
	if t.proofs[k] == nil {
		t.proofs[k] = map[uint64][]*protocol.AnnotatedReceipt{}
	}
	t.proofs[k][block] = append(t.proofs[k][block], proof)
	if _, ok := t.sources[k]; !ok {
		t.sources[k] = source
	}
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
// every entry held at or below n is dropped, and so is everything proven at
// or below the chain index of the entry delivered last.
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
	for k, m := range t.held {
		st := s.streams[k]
		if st == nil {
			st = newStreamState()
			s.streams[k] = st
		}
		for n, h := range m {
			if _, ok := st.held[n]; ok {
				continue // first sighting wins
			}
			st.held[n] = h
			if h.ID != nil {
				s.byID[h.ID.Hash()] = h
			}
		}
	}
	for k, n := range t.sighted {
		st := s.streams[k]
		if st == nil {
			st = newStreamState()
			s.streams[k] = st
		}
		if n > st.sighted {
			st.sighted = n
		}
	}
	for k, m := range t.proven {
		st := s.streams[k]
		if st == nil {
			st = newStreamState()
			s.streams[k] = st
		}
		for idx, h := range m {
			st.proven[idx] = h
			st.provenIndex[h] = idx
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
		st := s.streams[k]
		if st == nil {
			continue
		}
		var lastIdx int64 = -1
		for num, h := range st.held {
			if num > n {
				continue
			}
			if idx, ok := st.provenIndex[h.Hash]; ok && idx > lastIdx {
				lastIdx = idx
			}
			if h.ID != nil {
				delete(s.byID, h.ID.Hash())
			}
			delete(st.held, num)
		}
		if lastIdx >= 0 {
			for idx, h := range st.proven {
				if idx <= lastIdx {
					delete(st.proven, idx)
					delete(st.provenIndex, h)
				}
			}
		}
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
	t.sighted = map[string]uint64{}
	t.proven = map[string]map[int64][32]byte{}
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
