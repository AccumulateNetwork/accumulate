// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"encoding/binary"
	"io"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

// Retention of superseded BPT state, for AIP-58 historical account proofs.
//
// The BPT stores current state only: a node record is overwritten and the old
// value is gone. To answer "what did this account hold at block H" a node has to
// have kept the superseded nodes. This file keeps them.
//
// # Key shape
//
// History goes under a NEW key shape, never by versioning the existing one:
//
//	("BPT", "History", nodeKey)           -> the heights at which nodeKey has a
//	                                         retained version, ascending
//	("BPT", "History", nodeKey, height)   -> the value nodeKey held immediately
//	                                         BEFORE the write at that height
//
// Nothing under ("BPT", nodeKey) changes shape, so an existing database stays
// readable, no migration runs, and a node with retention disabled writes none of
// this at all.
//
// # Why the version list exists
//
// Reading nodeKey as of height R means finding the value it held at R. That is
// the value superseded by the first retained write AFTER R. Without an index,
// finding that write means scanning every retained height — and the height-0
// node, which sits on every path, is rewritten by every state-changing block.
// At a 90-day window on a busy BVN that is ~90,000 reads per node per query. The
// list turns it into one read and a binary search.
//
// A backward-linked list of versions was considered and rejected for the same
// reason: entering at the current node and walking back is O(number of times the
// node changed), which is O(window) for exactly the node every query touches.

// historyConfig is the retention configuration for a commit.
type historyConfig struct {
	// height is the block the commit is part of.
	height uint64

	// depth is how many blocks of history to keep. Zero disables retention.
	depth uint64
}

// SetHistory configures retention of superseded nodes for subsequent commits.
// height is the block being committed; depth is how many blocks to retain, and
// zero — the default — disables retention entirely, writing nothing.
//
// Retention writes only under the ("BPT", "History", ...) key shape, so it does
// not change the BPT root and is not consensus-relevant.
func (b *BPT) SetHistory(height, depth uint64) {
	if depth == 0 {
		b.history = nil
		return
	}
	b.history = &historyConfig{height: height, depth: depth}
}

// RetainedHeight reports the height retention is configured for, and whether
// retention is enabled at all.
func (b *BPT) RetainedHeight() (uint64, bool) {
	if b.history == nil {
		return 0, false
	}
	return b.history.height, true
}

// earliestKey addresses the earliest block this node can answer for.
//
// It is recorded rather than derived from configuration on purpose. Raising the
// configured depth does not retroactively create history, and a node that
// advertised its configured depth would be advertising a range it does not hold
// — which is worse than advertising nothing, because a client would believe it.
func (b *BPT) earliestKey() *record.Key {
	return b.key.Append("History").Append("Earliest")
}

// EarliestRetained returns the earliest block this node can produce historical
// state for, and whether it retains any at all.
func (b *BPT) EarliestRetained() (uint64, bool, error) {
	raw, ok, err := readRaw(b.store, b.earliestKey())
	if err != nil {
		return 0, false, errors.UnknownError.Wrap(err)
	}
	if !ok || len(raw) == 0 {
		return 0, false, nil
	}
	if len(raw) != 8 {
		return 0, false, errors.InternalError.WithFormat("earliest retained height is %d bytes, want 8", len(raw))
	}
	return binary.BigEndian.Uint64(raw), true, nil
}

// noteRetained advances the earliest retained height for a commit at the given
// height, which is the window floor once the window has filled, and the first
// height retention ran at until then.
func (b *BPT) noteRetained(height, depth uint64) error {
	floor := uint64(0)
	if height > depth {
		floor = height - depth
	}

	cur, ok, err := b.EarliestRetained()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	switch {
	case !ok:
		// The first block retention ran at. Nothing before it was kept, so that
		// is the horizon regardless of what the window would allow.
		floor = height
	case floor <= cur:
		return nil // No change; do not rewrite the record every block
	}

	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], floor)
	err = b.store.PutValue(b.earliestKey(), &rawBytes{data: buf[:], present: true})
	return errors.UnknownError.Wrap(err)
}

func (b *BPT) historyKey(nodeKey [32]byte) *record.Key {
	return b.key.Append("History").Append(nodeKey)
}

// rawBytes is a [database.Value] that carries an opaque encoded record, so a
// node's stored form can be copied without being decoded. Decoding it would
// require the BPT's parameters and would throw away the exactness that makes
// the copy trustworthy.
type rawBytes struct {
	data    []byte
	present bool
}

func (r *rawBytes) Key() *record.Key { panic(errShim()) }
func (r *rawBytes) Resolve(*record.Key) (record.Record, *record.Key, error) {
	return nil, nil, errors.InternalError.With("not supported")
}
func (r *rawBytes) IsDirty() bool { return false }
func (r *rawBytes) Commit() error { return nil }
func (r *rawBytes) Walk(database.WalkOptions, database.WalkFunc) error {
	return errors.InternalError.With("not supported")
}
func (r *rawBytes) GetValue() (encoding.BinaryValue, int, error) {
	return rawValue(r.data), 0, nil
}
func (r *rawBytes) LoadValue(v database.Value, put bool) error {
	u, _, err := v.GetValue()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	data, err := u.MarshalBinary()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	return r.LoadBytes(data, put)
}
func (r *rawBytes) LoadBytes(data []byte, _ bool) error {
	r.data = make([]byte, len(data))
	copy(r.data, data)
	r.present = true
	return nil
}

// rawValue is the [encoding.BinaryValue] side of [rawBytes].
type rawValue []byte

func (v rawValue) CopyAsInterface() any                { return v }
func (v rawValue) MarshalBinary() ([]byte, error)      { return v, nil }
func (v rawValue) UnmarshalBinary(data []byte) error   { panic(errShim()) }
func (v rawValue) UnmarshalBinaryFrom(io.Reader) error { panic(errShim()) }

// readRaw loads the encoded form of a record, reporting whether it was present.
func readRaw(store database.Store, key *record.Key) ([]byte, bool, error) {
	v := new(rawBytes)
	err := store.GetValue(key, v)
	switch {
	case err == nil:
		return v.data, v.present, nil
	case errors.Is(err, errors.NotFound):
		return nil, false, nil
	default:
		return nil, false, errors.UnknownError.Wrap(err)
	}
}

// encodeHeights and decodeHeights carry the version list. Fixed-width so a
// binary search reads it without allocating per entry.
func encodeHeights(h []uint64) []byte {
	b := make([]byte, 8*len(h))
	for i, v := range h {
		binary.BigEndian.PutUint64(b[8*i:], v)
	}
	return b
}

func decodeHeights(b []byte) ([]uint64, error) {
	if len(b)%8 != 0 {
		return nil, errors.InternalError.WithFormat("history index is %d bytes, want a multiple of 8", len(b))
	}
	h := make([]uint64, len(b)/8)
	for i := range h {
		h[i] = binary.BigEndian.Uint64(b[8*i:])
	}
	return h, nil
}

// retainSuperseded copies the value a node currently holds into history, before
// the caller overwrites it, and records the height in the node's version list.
//
// It is a no-op when retention is disabled, and when the node has no stored
// value yet — a node being written for the first time supersedes nothing.
func (b *BPT) retainSuperseded(nodeKey [32]byte, key *record.Key) error {
	if b.history == nil {
		return nil
	}
	h := b.history

	old, ok, err := readRaw(b.store, key)
	if err != nil {
		return errors.UnknownError.WithFormat("read superseded node: %w", err)
	}
	if !ok {
		return nil // Nothing to supersede
	}

	idxKey := b.historyKey(nodeKey)
	raw, _, err := readRaw(b.store, idxKey)
	if err != nil {
		return errors.UnknownError.WithFormat("read history index: %w", err)
	}
	heights, err := decodeHeights(raw)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Writing the same height twice would duplicate the entry and leave the
	// list unsorted for a binary search
	if n := len(heights); n > 0 && heights[n-1] >= h.height {
		if heights[n-1] == h.height {
			return nil
		}
		return errors.InternalError.WithFormat(
			"history for a node went backward: last retained height %d, committing %d", heights[n-1], h.height)
	}

	err = b.store.PutValue(idxKey.Append(h.height), &rawBytes{data: old, present: true})
	if err != nil {
		return errors.UnknownError.WithFormat("store superseded node: %w", err)
	}

	heights = append(heights, h.height)
	heights, dropped := pruneHeights(heights, h.height, h.depth)
	for _, d := range dropped {
		// Best effort: the value is unreachable once it leaves the index, so a
		// failure to delete leaks bytes rather than corrupting anything.
		_ = b.store.PutValue(idxKey.Append(d), &rawBytes{data: nil, present: true})
	}

	err = b.store.PutValue(idxKey, &rawBytes{data: encodeHeights(heights), present: true})
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	return b.noteRetained(h.height, h.depth)
}

// pruneHeights drops retained heights that have fallen out of the window,
// returning the heights to keep and the heights dropped.
//
// The window is the last depth blocks ending at now, so a version at height x is
// kept while x > now-depth. A version is what the node held BEFORE x, which is
// the state for every height in [previous, x), so the oldest kept version is the
// one that still covers the start of the window.
func pruneHeights(heights []uint64, now, depth uint64) (keep, dropped []uint64) {
	if depth == 0 || now < depth {
		return heights, nil
	}
	floor := now - depth

	// Keep the last entry at or below the floor: it is the version covering the
	// start of the window. Everything before it is unreachable.
	cut := sort.Search(len(heights), func(i int) bool { return heights[i] > floor })
	if cut > 0 {
		cut--
	}
	if cut == 0 {
		return heights, nil
	}
	return heights[cut:], heights[:cut]
}

// NodeAt returns the encoded form of a node as of the given block height, and
// whether history covers it.
//
// ok is false when no retained version postdates the height. That means the node
// has not changed since, so the CURRENT record is the correct answer — it is not
// an error and must not be reported as one. A height older than the retained
// window is a different matter and is the caller's to refuse; this function
// cannot distinguish the two, which is why the retained range is checked before
// it is called.
func (b *BPT) NodeAt(nodeKey [32]byte, height uint64) (data []byte, ok bool, err error) {
	idxKey := b.historyKey(nodeKey)
	raw, _, err := readRaw(b.store, idxKey)
	if err != nil {
		return nil, false, errors.UnknownError.WithFormat("read history index: %w", err)
	}
	heights, err := decodeHeights(raw)
	if err != nil {
		return nil, false, errors.UnknownError.Wrap(err)
	}

	// The value held at `height` is the one superseded by the first retained
	// write after it
	i := sort.Search(len(heights), func(i int) bool { return heights[i] > height })
	if i == len(heights) {
		return nil, false, nil
	}

	data, present, err := readRaw(b.store, idxKey.Append(heights[i]))
	if err != nil {
		return nil, false, errors.UnknownError.WithFormat("read superseded node: %w", err)
	}
	if !present || len(data) == 0 {
		return nil, false, errors.InternalError.WithFormat(
			"history index names height %d for a node with no retained value", heights[i])
	}
	return data, true, nil
}

// RetainedHeights returns the heights at which a node has a retained version,
// ascending. Exposed for tests and diagnostics.
func (b *BPT) RetainedHeights(nodeKey [32]byte) ([]uint64, error) {
	raw, _, err := readRaw(b.store, b.historyKey(nodeKey))
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return decodeHeights(raw)
}
