// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// StreamID names one ordered cross-partition stream: the account whose sequence
// ledger tracks it, and the partition its messages come from.
//
// The ledger is part of the name, not just the source, because anchors and
// synthetics between the same pair of partitions are SEPARATE streams — anchors
// tracked by the anchor pool, synthetics by the synthetic account. Conflating
// them would let an anchor's position gate a synthetic's.
type StreamID struct {
	Ledger *url.URL
	Source *url.URL
}

func (s StreamID) key() string { return s.Ledger.String() + "|" + s.Source.String() }

// Staging holds what the executor has received and cannot execute yet: the
// numbers above a stream's Delivered watermark whose predecessors have not
// arrived. Nothing reaches staging that consensus did not accept, so what fills
// it is the rate at which the network delivers messages a stream is not ready
// for.
//
// # It is executor state, not account state
//
// This was PartitionSyntheticLedger.Pending: main state of an account, hashed
// into the BPT and rewritten whole every block. That is what forced a bound — an
// array in a record rewritten every block cannot grow without limit — and the
// bound is what livelocked the network. Past MaxPendingSequenced the executor
// refused to RECORD the receipt while still storing the message, so the node
// held a message and reported not holding it, and healing re-fetched across the
// partition what was already in the local database: 8,556 sequence numbers
// fetched 53,011 times in one twenty-minute soak, with every partition live.
//
// Nothing here is hashed, so nothing here needs bounding. What staging holds is
// bounded by how far ahead of Delivered the network can run, which is the
// backlog itself — and a backlog the node is holding is not a reason to claim it
// is not.
//
// # It is not persisted
//
// Delivered is block output and survives because the block does. Everything
// above it is staging, and staging is empty when the process starts. A restarted
// node stages nothing, so every number above Delivered is a gap — which is what
// healing already exists for. Persisting staging would mean writing every block
// a state reconstructible from what is already durable.
//
// # Healing asks it
//
// A gap is a number above Delivered, up to what the source produced, that
// staging does not hold. [Staging.Missing] is that question, and it is the only
// way healing may answer it: reading what the block wrote is precisely what let
// the executor and the healer disagree.
//
// Safe for concurrent use. The executor writes it from the block's serial phase
// and healing reads it from its own goroutine.
type Staging struct {
	mu sync.RWMutex
	m  map[string]*stagedStream
}

// stagedStream is one stream's held set.
//
// A map rather than the positional array it replaces. The array indexed by
// offset from Delivered had to be shifted on every delivery and re-grown on
// every receipt, and the offset arithmetic — index = n - delivered - 1 — is the
// kind that reads a neighbouring stream's entry when it is wrong. A number is
// the key here because a number is what it is.
type stagedStream struct {
	held    map[uint64]*url.TxID
	highest uint64
}

// NewStaging returns an empty staging store.
func NewStaging() *Staging { return &Staging{m: map[string]*stagedStream{}} }

func (s *Staging) streamLocked(id StreamID) *stagedStream {
	k := id.key()
	e, ok := s.m[k]
	if !ok {
		e = &stagedStream{held: map[uint64]*url.TxID{}}
		if s.m == nil {
			s.m = map[string]*stagedStream{}
		}
		s.m[k] = e
	}
	return e
}

// Hold records that the executor has a message for this number and cannot
// execute it yet.
//
// Idempotent, and the FIRST sighting wins. A number can be offered twice — a
// block that is discarded and re-executed, a healed message racing the original
// — and both carry the same message, because the number identifies it. Keeping
// the first means the same block always produces the same staging, whatever
// order the duplicates arrived in.
func (s *Staging) Hold(id StreamID, n uint64, txid *url.TxID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e := s.streamLocked(id)
	if n > e.highest {
		e.highest = n
	}
	if _, ok := e.held[n]; !ok {
		e.held[n] = txid
	}
}

// IDOf returns the staged message ID for a number, if staging holds one.
func (s *Staging) IDOf(id StreamID, n uint64) (*url.TxID, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	e, ok := s.m[id.key()]
	if !ok {
		return nil, false
	}
	txid, ok := e.held[n]
	return txid, ok
}

// Highest is the largest number this stream has ever staged.
//
// It replaces the ledger's Received, and it is deliberately a high-water mark
// that Release does not lower: "we have seen this far" is what says a stream is
// behind, and forgetting it when the gap ahead fills would say the stream had
// never been behind at all.
func (s *Staging) Highest(id StreamID) uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	e, ok := s.m[id.key()]
	if !ok {
		return 0
	}
	return e.highest
}

// Held reports how many numbers this stream is holding. For diagnostics and
// tests; nothing decides anything from it.
func (s *Staging) Held(id StreamID) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	e, ok := s.m[id.key()]
	if !ok {
		return 0
	}
	return len(e.held)
}

// Release drops everything at or below delivered.
//
// Called when a block COMMITS, not when it advances a position: until the batch
// commits, the delivery has not happened, and dropping a staged message for a
// block that is then discarded would make the node fetch back something it
// still holds — the failure this whole change exists to remove.
func (s *Staging) Release(id StreamID, delivered uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.m[id.key()]
	if !ok {
		return
	}
	for n := range e.held {
		if n <= delivered {
			delete(e.held, n)
		}
	}
}

// Missing returns every contiguous run of numbers in (delivered, through] that
// staging does not hold, oldest first, at most maxRuns of them.
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
func (s *Staging) Missing(id StreamID, delivered, through uint64, maxRuns int) [][2]uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if through <= delivered || maxRuns <= 0 {
		return nil
	}
	e, ok := s.m[id.key()]
	if !ok {
		// Nothing staged at all: everything the source produced past our
		// watermark is missing, and it is one run.
		return [][2]uint64{{delivered + 1, through}}
	}

	var runs [][2]uint64
	open := false
	for n := delivered + 1; n <= through; n++ {
		if _, held := e.held[n]; held {
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
