// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

// Recovering a chain whose mark points were discarded.
//
// The 13 July 2025 mainnet restore dropped `States` — the mark points — for
// every account that was not a data account, and a merkle chain keeps its
// entry hashes in those mark points and in the head. Dropping them therefore
// discarded the hashes of every closed mark set, and the restore then carried
// no message body that nothing referenced. What survives is the open mark set:
// the head, its open mark set, and the peaks (#4270).
//
// That is enough to recover the surviving tail. Every read of it currently
// fails, because StateAt replays from the mark point that is gone.

// StateAtBoundary reconstructs the Merkle state at a mark boundary from a
// chain head, for a chain whose mark points were discarded.
//
// A peak at level k is the root of a complete subtree of 2^k leaves, and the
// peaks a state holds are exactly the set bits of its count. The state at
// boundary B is therefore the head's peaks at B's set bits — provided the
// entries added after B have not combined into them, which holds while the
// head has not reached the next power of two above B's largest peak.
//
// That proviso is not assumed. The reconstruction is a hypothesis, and
// VerifyAgainstHead is how it is discharged: replaying the head's own open set
// onto the result must reproduce the head. Where it does, the state is
// provably the one that was there; where it does not, the caller must leave
// the account alone.
//
// Returns nil if a peak the boundary needs is absent — see CanReconstruct.
func StateAtBoundary(head *State, boundary uint64) *State {
	if boundary == 0 || boundary > uint64(head.Count) {
		return nil
	}
	s := new(State)
	s.Count = int64(boundary)
	for i := 0; i < len(head.Pending); i++ {
		if boundary&(1<<uint(i)) == 0 {
			s.Pending = append(s.Pending, nil)
			continue
		}
		if head.Pending[i] == nil {
			return nil // the subtree root this boundary needs is not there
		}
		s.Pending = append(s.Pending, copyHash(head.Pending[i]))
	}
	// Every set bit of the boundary must have been covered
	for i := len(head.Pending); i < 64; i++ {
		if boundary&(1<<uint(i)) != 0 {
			return nil
		}
	}
	return s
}

// CanReconstruct reports whether the state at a boundary is recoverable from a
// head of the given count.
//
// A state holds a peak at level k exactly when bit k of its count is set. The
// boundary's peaks therefore survive in the head only while every set bit of
// the boundary is still set in the count — that is, while the boundary is a
// submask of the count.
//
// It fails when a boundary's subtree has been ABSORBED. Adding the 512th entry
// combines the two 256-subtrees into one, and the root of the first 256 ceases
// to be a peak; it is inside the 512 peak and cannot be extracted, because
// hashing does not invert. A chain sitting at exactly 512 entries therefore
// cannot have its 256 boundary recovered from the head, and its tail stays
// unreadable until a mark point is found elsewhere.
func CanReconstruct(count int64, boundary uint64) bool {
	if boundary == 0 || count <= 0 || boundary > uint64(count) {
		return false
	}
	return boundary&^uint64(count) == 0
}

// VerifyAgainstHead replays the open mark set onto a reconstructed state and
// reports whether it reproduces the head exactly. This is the proof that a
// reconstruction is correct: it is checked against state the restore did
// keep, so nothing has to be taken on trust. The open set is passed in
// because the head does not always carry it: since the Tail records
// (database.md, "The head is Count and Pending; the open mark set is
// chunked") it lives beside the head, and Chain.OpenSet reads it from
// wherever it is.
func VerifyAgainstHead(reconstructed, head *State, openSet [][]byte) bool {
	if reconstructed == nil {
		return false
	}
	s := reconstructed.Copy()
	for _, h := range openSet {
		s.AddEntry(h)
	}
	if s.Count != head.Count {
		return false
	}
	a, b := s.Anchor(), head.Anchor()
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// BoundaryFor returns the start of the open mark set for a chain of the given
// count and mark frequency — the boundary whose mark point a read of the tail
// replays from, and the one the restore discarded.
func BoundaryFor(count int64, markFreq int64) uint64 {
	if count <= 0 || markFreq <= 0 {
		return 0
	}
	return uint64((count - 1) &^ (markFreq - 1))
}
