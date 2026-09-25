// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

import (
	"fmt"
	"math"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// OnDuplicate, when set, is called for every append of a hash the chain
// already holds, with the chain's key and whether the append was asked
// to be unique.  It is nil in production; tests install a recorder and
// assert that the only duplicates are the permitted repeats (database
// spec, "Duplicates are caught at entry").
var OnDuplicate func(chain *record.Key, unique bool)

func NewChain(logger logging.Logger, store record.Store, key *record.Key, markPower int64, typ ChainType, namefmt string) *Chain {
	c := new(Chain)
	c.logger.L = logger
	c.store = store
	c.key = key
	c.typ = typ

	// TODO markFreq = 1 << markPower?

	c.markPower = markPower                             // # levels in Merkle Tree to be indexed
	c.markFreq = int64(math.Pow(2, float64(markPower))) // The number of elements between indexes
	c.markMask = c.markFreq - 1                         // Mask to index of next mark (0 if at a mark)

	if strings.ContainsRune(namefmt, '%') {
		c.name = key.Stringf(namefmt)
	} else {
		c.name = namefmt
	}

	return c
}

func (c *Chain) Name() string     { return c.name }
func (c *Chain) Type() ChainType  { return c.typ }
func (c *Chain) MarkPower() int64 { return c.markPower }
func (c *Chain) MarkMask() int64  { return c.markMask }
func (c *Chain) MarkFreq() int64  { return c.markFreq }

func (c *Chain) getMarkPoints() ([]chainStatesKey, error) {
	head, err := c.Head().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load head: %w", err)
	}

	n := head.Count / c.markFreq
	keys := make([]chainStatesKey, 0, n)
	for i := c.markFreq; i < head.Count; i += c.markFreq {
		keys = append(keys, chainStatesKey{Index: uint64(i - 1)})
	}
	return keys, nil
}

// tailChunkSize is how many hashes one Tail record holds.
//
// The head is Count and Pending. The hashes of the OPEN mark set -- the
// entries since the last mark point, up to markFreq of them -- are what a
// receipt, a state or a range inside that set replays, and they used to be
// carried in the head: up to 256 hashes rewritten on every append, 85% of
// the bytes the dynamic layer was written (#4234). They are kept in Tail
// records instead, markFreq/tailChunkSize of them, reused every set; an
// append rewrites one chunk, and the mark point that closes the set is
// assembled from all of them. The records are mutable and live in the
// dynamic layer, so the open set is readable at any age, as it was in the
// head -- which is what a windowed store needs of it: the mark point is
// closed and the tail is read for slow chains whose elements have long
// left the permanent window.
const tailChunkSize = 8

// chunkSize is tailChunkSize, or the mark set when that is smaller.
func (c *Chain) chunkSize() int64 {
	if c.markFreq < tailChunkSize {
		return c.markFreq
	}
	return tailChunkSize
}

// getTailChunks names the Tail records the open mark set occupies, for
// walking the chain's records. A chunk past the open set holds a previous
// set's hashes and is not part of the chain's state.
func (c *Chain) getTailChunks() ([]chainTailKey, error) {
	head, err := c.Head().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load head: %w", err)
	}
	if len(head.HashList) > 0 {
		return nil, nil // A head from before the tail records carries the set itself
	}
	n := (head.Count&c.markMask + c.chunkSize() - 1) / c.chunkSize()
	keys := make([]chainTailKey, n)
	for i := range keys {
		keys[i] = chainTailKey{Index: uint64(i)}
	}
	return keys, nil
}

// appendTail records the entry at index in its Tail chunk. The chunk it
// lands in held the previous set's hashes at that position, or nothing,
// and is started over.
//
// The head is the chain's authority and Element(i) its record; a chunk is
// derived from them and is reconciled to them, never trusted over them. A
// chunk that runs past the head is cut back to it, and one that falls short
// is refilled from the elements -- a store that gives a batch no snapshot
// (the memory store) lets a reader see the head from before a commit and
// the chunk from after it, and a discarded validation batch used to read
// such pairs without noticing, because nothing compared two chain records.
// Only an element that is missing too is an error.
func (m *Chain) appendTail(index int64, hash []byte) error {
	c := m.chunkSize()
	base := index &^ (c - 1)
	k := uint64((index & m.markMask) / c)
	chunk, err := m.Tail(k).Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load tail chunk %d: %w", k, err)
	}
	if int64(chunk.Index) != base {
		chunk = &TailChunk{Index: uint64(base)}
	}
	if have := int64(len(chunk.Hashes)); have > index-base {
		chunk.Hashes = chunk.Hashes[:index-base]
	} else {
		for i := base + have; i < index; i++ {
			h, err := m.Element(uint64(i)).Get()
			if err != nil {
				return errors.InvalidRecord.WithFormat("tail chunk %d of %v holds %d hashes before entry %d and element %d is missing: %w", k, m.key, have, index, i, err)
			}
			chunk.Hashes = append(chunk.Hashes, copyHash(h))
		}
	}
	chunk.Hashes = append(chunk.Hashes, hash)
	return m.Tail(k).Put(chunk)
}

// migrateTail moves the open mark set out of a head written before the
// Tail records existed. Once per chain, on its first append after the
// change; a chain that is never appended to again is read from its head as
// before.
func (m *Chain) migrateTail(head *State) error {
	lastMark := head.Count &^ m.markMask
	if int64(len(head.HashList)) != head.Count-lastMark {
		return errors.InvalidRecord.WithFormat("head of %v: expected %d hashes since the last mark point, got %d", m.key, head.Count-lastMark, len(head.HashList))
	}
	for i, h := range head.HashList {
		if err := m.appendTail(lastMark+int64(i), h); err != nil {
			return err
		}
	}
	head.HashList = nil
	return nil
}

// tailHashes reads the entries [from, to) of the mark set the Tail records
// hold: the open set, or -- from AddEntry, at the moment it closes -- the
// set just completed. A head from before the tail records carries the open
// set itself and is read when the records do not answer.
// OpenSet is the hashes of the open mark set -- the entries since the last
// mark point, which VerifyAgainstHead replays. A head written before the
// Tail records carries the set itself; since then it is chunked beside the
// head (migrateTail, appendTail), so the head is asked and then the tail.
func (m *Chain) OpenSet(head *State) ([][]byte, error) {
	if len(head.HashList) > 0 {
		return head.HashList, nil
	}
	from := int64(BoundaryFor(head.Count, m.markFreq))
	if from >= head.Count {
		return nil, nil
	}
	return m.tailHashes(head, from, head.Count)
}

func (m *Chain) tailHashes(head *State, from, to int64) ([][]byte, error) {
	if from >= to {
		return nil, nil
	}
	c := m.chunkSize()
	hashes := make([][]byte, 0, to-from)
	for i := from; i < to; {
		base := i &^ (c - 1)
		end := base + c
		if end > to {
			end = to
		}
		k := uint64((i & m.markMask) / c)
		chunk, err := m.Tail(k).Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load tail chunk %d: %w", k, err)
		}
		if int64(chunk.Index) != base || int64(len(chunk.Hashes)) < end-base {
			return m.tailFallback(head, from, to)
		}
		hashes = append(hashes, chunk.Hashes[i-base:end-base]...)
		i = end
	}
	return hashes, nil
}

// tailFallback answers [from, to) when the Tail records do not: from a
// head that still carries its open mark set, or else from the elements,
// which are the record the chunks are derived from (see appendTail).
func (m *Chain) tailFallback(head *State, from, to int64) ([][]byte, error) {
	lastMark := head.Count &^ m.markMask
	if from >= lastMark && to <= head.Count && int64(len(head.HashList)) == head.Count-lastMark {
		return head.HashList[from-lastMark : to-lastMark], nil
	}
	hashes := make([][]byte, 0, to-from)
	for i := from; i < to; i++ {
		h, err := m.Element(uint64(i)).Get()
		if err != nil {
			return nil, errors.NotFound.WithFormat("entry %d of %v is not in the tail and its element is missing: %w", i, m.key, err)
		}
		hashes = append(hashes, h)
	}
	return hashes, nil
}

// RestoreHead sets the chain's head and open mark set from a peer, for a node
// pulling the state (executor.md, "Sync", step 3). open is the chain's entries
// from the last mark point to head.Count.
//
// The head alone is not a chain that can be appended to. An append records the
// entry in its Tail chunk, and a chunk that falls short is refilled from the
// elements, so a chain given a head and nothing else fails on its next entry
// ("tail chunk 0 ... holds 0 hashes before entry 3 and element 0 is missing").
// The open mark set is therefore restored with the head: its elements, their
// index entries, and the Tail chunks they belong to. Entries below the last
// mark point are not needed to append — the mark point that closed their set
// is closed — and a node pulling state only takes chains it needs to read in
// full (ModeFullSpine) entry by entry.
//
// The open set is checked against the head where the head allows it: replaying
// it onto the state reconstructed at the mark boundary must reproduce the
// head, which is the same discharge VerifyAgainstHead performs for a restore.
func (m *Chain) RestoreHead(head *State, open [][]byte) error {
	if head == nil {
		return errors.BadRequest.WithFormat("%v: head required", m.key)
	}
	// The set to restore is the one the next append continues: the entries
	// since the boundary at Count &^ markMask, which is what getTailChunks
	// counts and appendTail refills from. At an exact mark point that set is
	// empty; Chain.OpenSet answers with the set just closed instead, and a
	// caller passing that is taken to mean the same chain.
	lastMark := head.Count &^ m.markMask
	if lastMark == head.Count && head.Count > 0 && int64(len(open)) == m.markFreq {
		lastMark = head.Count - m.markFreq
	}
	if int64(len(open)) != head.Count-lastMark {
		return errors.BadRequest.WithFormat(
			"%v: the open mark set of a chain of height %d holds %d entries, got %d",
			m.key, head.Count, head.Count-lastMark, len(open))
	}

	var before *State
	if lastMark == 0 {
		before = new(State)
	} else {
		before = StateAtBoundary(head, uint64(lastMark))
	}
	if before != nil && !VerifyAgainstHead(before, head, open) {
		return errors.BadRequest.WithFormat("%v: the open mark set does not reproduce the head", m.key)
	}

	for i, h := range open {
		index := lastMark + int64(i)
		h = copyHash(h)
		if err := m.ElementIndex(h).Put(uint64(index)); err != nil {
			return errors.UnknownError.WithFormat("%v: put element index %d: %w", m.key, index, err)
		}
		if err := m.Element(uint64(index)).Put(h); err != nil {
			return errors.UnknownError.WithFormat("%v: put element %d: %w", m.key, index, err)
		}
		if err := m.appendTail(index, h); err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	// The open set is in the Tail records now, so the head does not carry it.
	head = head.Copy()
	head.HashList = nil
	return m.Head().Put(head)
}

// PutBelow writes hash as the entry at st.Count the way AddEntry writes it --
// its element, its index, the intermediates the cascade computes, and the mark
// point the entry closes -- without reading or writing the head, and advances
// st past it. st is the chain's state before the entry, with the entries since
// its last mark point in HashList.
//
// It is for a node that holds a head it took from a peer and brings in the
// entries under it a page at a time (executor.md, "Sync" §3; #4446): the head
// is not the running state, so nothing here reads it, and the caller checks
// st against the head once the last entry is written. indexIfAbsent leaves a
// hash the chain already indexes to the position it names: a chain the node
// has appended to since holds a later write of it (pull.Backfill).
func (m *Chain) PutBelow(st *State, hash []byte, indexIfAbsent bool) error {
	hash = copyHash(hash)
	index := uint64(st.Count)
	put := true
	if indexIfAbsent {
		_, err := m.ElementIndex(hash).Get()
		switch {
		case err == nil:
			put = false
		case !errors.Is(err, storage.ErrNotFound):
			return errors.UnknownError.WithFormat("%v: load element index: %w", m.key, err)
		}
	}
	if put {
		if err := m.ElementIndex(hash).Put(index); err != nil {
			return errors.UnknownError.WithFormat("%v: put element index %d: %w", m.key, index, err)
		}
	}
	if err := m.Element(index).Put(hash); err != nil {
		return errors.UnknownError.WithFormat("%v: put element %d: %w", m.key, index, err)
	}
	if err := m.putIntermediates(st, hash); err != nil {
		return errors.UnknownError.Wrap(err)
	}
	st.AddEntry(hash)
	if st.Count&m.markMask == 0 {
		if err := m.States(index).Put(st.Copy()); err != nil {
			return errors.UnknownError.WithFormat("%v: put mark point %d: %w", m.key, index, err)
		}
		st.HashList = nil
	}
	return nil
}

// AddEntry adds a Hash to the Chain controlled by the ChainManager. If unique is
// true, the hash will not be added if it is already in the chain.
func (m *Chain) AddEntry(hash []byte, unique bool) error {
	head, err := m.Head().Get() // Get the current state
	if err != nil {
		return err
	}

	hash = copyHash(hash) // Just to make sure hash doesn't get changed

	// The element index maps a hash to the position it was last written at.
	// A chain that does not ask for uniqueness writes it without reading: a
	// duplicate is caught where it enters the executor, by recent state, and
	// a chain that repeats by construction (a root or signature chain) keeps
	// every entry (database spec, "Duplicates are caught at entry"). Only a
	// chain the writer still asks to deduplicate reads first; reaching the
	// duplicate branch there is the writer appending the same hash twice,
	// which test builds record so the suites can assert which.
	if unique {
		_, err = m.ElementIndex(hash).Get()
		switch {
		case err == nil:
			if OnDuplicate != nil {
				OnDuplicate(m.key, unique)
			}
			return nil // Don't add duplicates
		case !errors.Is(err, storage.ErrNotFound):
			return err
		}
	}
	err = m.ElementIndex(hash).Put(uint64(head.Count))
	if err != nil {
		return err
	}

	err = m.Element(uint64(head.Count)).Put(hash)
	if err != nil {
		return err
	}

	// The open mark set is kept in the Tail records, not the head. A head
	// from before them carries the set itself; its first append moves the
	// set over, so the mark point closing it is whole.
	if len(head.HashList) > 0 {
		if err := m.migrateTail(head); err != nil {
			return err
		}
	}
	if err := m.appendTail(head.Count, hash); err != nil {
		return err
	}

	// The cascade about to run computes every intermediate hash a proof of
	// this element will need. Record them, so the proof reads them instead of
	// rebuilding the state that held them (#4263).
	if err := m.putIntermediates(head, hash); err != nil {
		return err
	}

	head.addPending(hash) // Count and Pending: the head carries no hash list
	if head.Count&m.markMask == 0 {
		// The end of the mark set: the mark point holds every hash of it,
		// as it always has, for the readers that replay a set from it.
		hashes, err := m.tailHashes(head, head.Count-m.markFreq, head.Count)
		if err != nil {
			return err
		}
		mark := head.Copy()
		mark.HashList = hashes
		err = m.States(uint64(head.Count) - 1).Put(mark) // Save Merkle State at n*MarkFreq-1
		if err != nil {
			return err
		}
	}

	err = m.Head().Put(head)
	if err != nil {
		return fmt.Errorf("error writing chain head: %v", err)
	}

	return nil
}

// IndexOf
// Get an Element of a Merkle Tree from the database
func (m *Chain) IndexOf(hash []byte) (int64, error) {
	i, err := m.ElementIndex(hash).Get()
	if err != nil {
		return 0, err
	}
	return int64(i), nil
}

// getState
// Query the database for the merkle state for a given index, i.e. the state
// Note that not every element in the Merkle Tree has a stored state;
// states are stored at the frequency indicated by the Mark Power.  We also
// store the state of the chain at the end of a block regardless, but this
// state overwrites the previous block state.
//
// If no state exists in the database for the element, getState returns nil
func (m *Chain) getState(element int64) *State {
	head, err := m.Head().Get()
	if err == nil && head.Count == 0 {
		ms := new(State)
		if eHash, err := m.Entry(element); err != nil {
			ms.AddEntry(eHash)
		}
		return ms
	}

	ms, e := m.States(uint64(element)).Get() // Get the data at this height
	if e != nil {                            // If nil, there is no state saved
		return nil //                           return nil, as no state exists
	}
	return ms.Copy() // return it
}

// StateAt
// We only store the state at MarkPoints.  This function computes a missing
// state even if one isn't stored for a particular element.
//
// The state carries the hashes since the mark point before element, as it
// always has -- a chain seeded from it (a partial tree) must close the same
// mark point -- read from the mark point that closed their set or, for the
// open set, from the Tail records (#4234).
func (m *Chain) StateAt(element int64) (ms *State, err error) {
	if element == -1 { //                                A need exists for the state before adding the first element
		ms = new(State) //                         In that case, just allocate a State
		return ms, nil  //                         And all is golden
	}
	if ms = m.getState(element); ms != nil { //          Shoot for broke. Return a state if it is in the db
		return ms, nil
	}
	head, err := m.Head().Get()
	if err != nil {
		return nil, err
	} else if element >= head.Count { //               Check to make sure element is not outside bounds
		return nil, errors.BadRequest.With("element out of range")
	}
	MIPrev := element&(^m.markMask) - 1 //               Calculate the index of the prior markpoint
	var cState *State
	if MIPrev < 0 {
		cState = new(State)
	} else if cState = m.getState(MIPrev); cState == nil {
		// The state at an element is the prior mark point's state plus
		// the hashes since. Without that mark point there is nothing to
		// build from: an empty state in its place is a different chain,
		// and a receipt built on it ends at a root nobody else holds. This
		// used to fall back to an empty state for the sake of truncated
		// chains, and a store that answered "absent" for a mark point older
		// than its window (soak 20260905T032333Z and after) got receipts
		// that every destination rejected, silently, for hours.
		return nil, errors.NotFound.WithFormat("mark point %d of %v is missing; cannot compute the state at %d", MIPrev, m.key, element)
	}
	cState.HashList = cState.HashList[:0] //             element is past the previous mark, so clear the HashList

	MINext := element&(^m.markMask) - 1 + m.markFreq //            Calculate the following mark point
	var since [][]byte                               //             The hashes after the prior mark point
	lastMark := head.Count &^ m.markMask             //
	if MINext >= head.Count {                        //             If past the end of the chain, then
		since, err = m.tailHashes(head, lastMark, element+1) //   the open mark set holds them
		if err != nil {
			return nil, err
		}
	} else {
		// Try to find the next available mark point (may be after MINext in a truncated chain)
		NMark := m.getState(MINext)
		if NMark == nil {
			// Search for the next available mark point
			for i := MINext + m.markFreq; i <= lastMark; i += m.markFreq {
				NMark = m.getState(i)
				if NMark != nil {
					break
				}
			}
		}
		if NMark != nil {
			since = NMark.HashList
		} else if since, err = m.tailHashes(head, lastMark, head.Count); err != nil { // If still not found, try the open set
			return nil, err
		}
	}
	for _, v := range since { //                                    Now iterate and add to the cState
		if element+1 == cState.Count { //                              until the loop adds the element
			break
		}
		cState.AddEntry(v)
	}
	if cState.Count&m.markMask == 0 { //                           If we progress out of the mark set,
		cState.HashList = cState.HashList[:0] //                       start over collecting hashes.
	}
	return cState, nil
}

// Entry the nth leaf node
func (m *Chain) Entry(element int64) ([]byte, error) {
	hash, err := m.Element(uint64(element)).Get() // Check the index
	switch {
	case err == nil:
		return hash, nil
	case errors.Is(err, errors.NotFound):
		// Continue
	default:
		return nil, errors.UnknownError.WithFormat("load element %d: %w", element, err)
	}

	head, err := m.Head().Get() // Load the head
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load head: %w", err)
	}
	if element >= head.Count { // Make sure element is not greater than count
		return nil, errors.NotFound.WithFormat("element %d does not exist (count=%d)", element, head.Count)
	}

	lastMark := head.Count &^ m.markMask // Last mark point
	if element >= lastMark {             // Get element from the open mark set
		hashes, err := m.tailHashes(head, element, element+1)
		if err != nil {
			return nil, err
		}
		return hashes[0], nil
	}

	elemMark := element&^m.markMask + m.markFreq // Mark point after element
	state, err := m.States(uint64(elemMark - 1)).Get()
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		return nil, errors.NotFound.WithFormat("cannot locate element %d", element)
	default:
		return nil, errors.UnknownError.WithFormat("load mark point %d: %w", elemMark-1, err)
	}

	i := element & m.markMask // Index within the mark point
	if i >= int64(len(state.HashList)) {
		return nil, errors.InvalidRecord.WithFormat("mark point %d: expected %d elements, got %d", elemMark-1, m.markFreq, len(state.HashList))
	}
	return state.HashList[i], nil
}

// getIntermediate
// Return the last two hashes that were combined to create the local
// Merkle Root at the given index.  The element provided must be odd,
// and the Pending List must be fully populated up to height specified.
// putIntermediates records the pairs the cascade is about to combine, one per
// height it carries through, so a proof reads them instead of replaying the
// state that produced them. It mirrors State.AddEntry exactly: the pair at
// height i+1 is (Pending[i], the hash accumulated so far).
func (m *Chain) putIntermediates(head *State, hash []byte) error {
	index := uint64(head.Count) // the element's index is the count before it is added
	acc := copyHash(hash)
	for i, v := range head.Pending {
		if v == nil {
			return nil // the cascade stops here
		}
		// getIntermediate(index, height) returns this pair for height i+1
		pair := make([]byte, 0, 64)
		pair = append(pair, v...)
		pair = append(pair, acc...)
		if err := m.Intermediate(index, uint64(i)+1).Put(pair); err != nil {
			return err
		}
		acc = combineHashes(v, acc)
	}
	return nil
}

// trailingOnes counts the set bits at the bottom of v, which is the number of
// heights the cascade carries through when element v is added.
func trailingOnes(v uint64) int {
	n := 0
	for v&1 == 1 {
		n++
		v >>= 1
	}
	return n
}

func (m *Chain) getIntermediate(element, height int64) (Left, Right []byte, err error) {
	// Whether a pair exists at all is arithmetic, and answering it first
	// costs nothing: adding element e carries through Pending[i] for each set
	// bit of e from the bottom, so the cascade produces intermediates for
	// heights 1..trailingOnes(e) and no higher. Above that there is nothing
	// stored and nothing to rebuild -- the replay would walk the whole of
	// Pending and return this same error, which is what Receipt.build uses to
	// change column. Asking the store first would spend a read per level to
	// learn what the index already says (#4263).
	if element >= 0 && height > int64(trailingOnes(uint64(element))) {
		return nil, nil, fmt.Errorf("no values found at height %d", height)
	}

	// The pair exists. The cascade computed it when the element was added, so
	// read it rather than rebuilding the state that held it.
	if pair, err := m.Intermediate(uint64(element), uint64(height)).Get(); err == nil && len(pair) == 64 {
		return copyHash(pair[:32]), copyHash(pair[32:]), nil
	}

	hash, e := m.Entry(element) // Get the element at this height
	if e != nil {               // Error out if we can't
		return nil, nil, e //
	} //
	s, e2 := m.StateAt(element - 1) // Get the state before the state we want
	if e2 != nil {                  // If the element doesn't exist, that's a problem
		return nil, nil, e2 //
	} //
	return getMerkleStateIntermediate(s, hash, height)
}

func getMerkleStateIntermediate(m *State, hash []byte, height int64) (left, right []byte, err error) {
	m.pad()                       // Pad Pending with a nil to remove corner cases
	for i, v := range m.Pending { // Adding the hash is like incrementing a variable
		if v == nil { //               Look for an empty slot; should not encounter one
			return nil, nil, fmt.Errorf("should not encounter a nil at height %d", height)
		}
		if i+1 == int(height) { // Found the height
			left = copyHash(v)      // Get the left and right
			right = copyHash(hash)  //
			return left, right, nil // return them
		}
		hash = combineHashes(v, hash) // If this slot isn't empty, combine the hash with the slot
	}
	return nil, nil, fmt.Errorf("no values found at height %d", height)
}
