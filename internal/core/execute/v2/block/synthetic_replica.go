// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4140: a collection proof is a signed, accepted state of a source chain —
// the ReceiptList carries the merkle state at start-1 plus the elements, and
// replaying them reproduces the committed anchor. So the receiver does not
// need a separate record of "what was proven"; it needs the chain itself,
// and building that chain IS the copy. Anchors already have this
// (AnchorChain(Directory).Root); this gives synthetics the same replica.
//
// The replica is keyed on STREAM identity — a string, a partition ID today
// and possibly (partition, worker) later — so the key survives that change
// without a state migration. It is seeded from any accepted proof: the
// proof's merkle state stands in for everything before it, which is also
// what makes discarding old entries safe. "Proven through N" is simply the
// replica's height, and a message whose hash the replica already contains
// needs no proof and no signature of its own.

// countFromPending derives a merkle state's element count from the structure
// of its pending list: each occupied level is a set bit. The Count field is
// not trusted on its own (#4106) — a state whose count disagrees with its
// structure is rejected rather than seeded.
func countFromPending(s *merkle.State) int64 {
	// A real chain needs fewer than 63 levels for 2^62 entries; more is a
	// crafted state, and 1<<63 would go negative (#4152). Refuse rather
	// than rely on downstream comparisons happening to no-op.
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

// replicaStream returns the stream identity for a source partition URL.
func replicaStream(source *url.URL) (string, bool) {
	return protocol.ParsePartitionUrl(source)
}

// seedSyntheticReplica extends the destination's replica of the source's
// synthetic main chain from an accepted collection proof. The caller is
// responsible for the proof being VALID and ANCHORED — this must only be
// called after the terminal-anchor check passes. Seeding is idempotent:
// overlapping proofs extend only the missing tail, and a proof entirely
// behind the replica's height is a no-op.
func (x *Executor) seedSyntheticReplica(batch *database.Batch, source *url.URL, list *merkle.ReceiptList) error {
	stream, ok := replicaStream(source)
	if !ok {
		return errors.BadRequest.WithFormat("%v is not a partition", source)
	}

	// The state's count must agree with its structure (#4106), and both
	// must be sane — a negative count could only come from a crafted state
	// (#4152).
	start := countFromPending(list.MerkleState)
	if start < 0 || start != list.MerkleState.Count {
		return errors.BadRequest.WithFormat("collection proof state is inconsistent: count is %d but the structure holds %d", list.MerkleState.Count, start)
	}
	end := start + int64(len(list.Elements))

	record := batch.Account(x.Describe.Synthetic()).SyntheticReplica(stream)
	// Get registers the chain in the account's chain index, which is what
	// makes the replica part of the account's BPT entry — replica state is
	// consensus state.
	chain, err := record.Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load synthetic replica for %s: %w", stream, err)
	}

	elements := list.Elements
	switch height := chain.Height(); {
	case height >= end:
		return nil // Already proven through here

	case height >= start:
		// Extend with the missing tail
		elements = elements[height-start:]

	default:
		// The proof begins past the replica's height (or the replica is
		// fresh). Re-seed from the proof's state: everything behind it is
		// proven by the state itself, which is exactly what makes discarding
		// it safe.
		err = record.Inner().Head().Put(list.MerkleState.Copy())
		if err != nil {
			return errors.UnknownError.WithFormat("seed synthetic replica for %s: %w", stream, err)
		}
		chain, err = record.Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load synthetic replica for %s: %w", stream, err)
		}
	}

	for _, el := range elements {
		err = chain.AddEntry(el, false)
		if err != nil {
			return errors.UnknownError.WithFormat("extend synthetic replica for %s: %w", stream, err)
		}
	}
	return nil
}

// replicaIncludes reports whether the destination's replica of the source's
// stream already contains the given hash — in which case the message is
// proven by a proof this partition has already accepted and anchored, and
// needs no proof and no signature of its own. Matching is by inclusion, not
// position: one stream's chain interleaves messages for many destinations.
func (x *Executor) replicaIncludes(batch *database.Batch, source *url.URL, hash []byte) bool {
	stream, ok := replicaStream(source)
	if !ok {
		return false
	}
	_, err := batch.Account(x.Describe.Synthetic()).SyntheticReplica(stream).IndexOf(hash)
	return err == nil
}
