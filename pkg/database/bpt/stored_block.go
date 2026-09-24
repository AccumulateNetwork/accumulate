// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"crypto/sha256"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// The BPT is stored in blocks of eight levels (Power = 8): the node at every
// eighth height is written as a record of its own holding the subtree down to
// the next eighth height. A joining node whose root does not match locates the
// difference along that storage (executor.md, Sync, "Two mismatches" 1;
// #4441): a peer serves, as of block B, the 256 positions eight levels below a
// prefix, the node compares them with its own, and asks again only under the
// positions that differ.

// storedBlockLevels is the depth of one stored block, which is the only depth
// the answer is defined for.
const storedBlockLevels = 8

// BlockSlot is one non-empty position eight levels below a stored block's
// prefix: the subtree of the keys whose next byte is Index.
type BlockSlot struct {
	// Index is the byte that follows the prefix for every key in the slot.
	Index byte

	// Branch reports that two or more keys share the slot, so a stored block of
	// its own sits here; its hash is Hash and the next answer is asked for
	// under prefix + Index.
	Branch bool

	// KeyHash is the key of the one leaf in the slot, when Branch is false.
	KeyHash [32]byte

	// Hash is the slot's hash: the leaf's hash, or the stored block's.
	Hash [32]byte
}

// StoredBlock is one stored block of the BPT: the hash of the node at the
// prefix and the non-empty positions eight levels below it, in ascending order
// of Index. A position that is not listed is empty.
type StoredBlock struct {
	Prefix []byte
	Hash   [32]byte
	Slots  []BlockSlot
}

// GetStoredBlock returns the stored block of the current tree under prefix.
// A nil prefix is the root's block.
func (b *BPT) GetStoredBlock(prefix []byte) (*StoredBlock, error) {
	if b.view != nil {
		return nil, errors.InternalError.With("a historical view is read with GetStoredBlockAt")
	}
	_, err := b.GetRootHash()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return b.storedBlock(prefix)
}

// GetStoredBlockAt returns the stored block under prefix as the tree stood at
// the end of the given block, whose root the caller supplies from the ledger.
//
// It refuses, with [errors.IncompleteChain], a block before the earliest this
// node retains: below the horizon the retained versions that would say what a
// node held have been pruned, and the current record read in their place is a
// later tree. Inside the window it is the historical view's walk, so every
// block loaded on the way down is checked against the block above it, and the
// root recomputed from them must equal the root supplied, or it refuses rather
// than answer from another version of the tree.
func (b *BPT) GetStoredBlockAt(height uint64, root [32]byte, prefix []byte) (*StoredBlock, error) {
	earliest, ok, err := b.EarliestRetained()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if !ok || height < earliest {
		if !ok {
			return nil, errors.IncompleteChain.WithFormat(
				"no BPT history is retained, so block %d cannot be answered", height)
		}
		return nil, errors.IncompleteChain.WithFormat(
			"block %d precedes the earliest block whose BPT history is retained, %d", height, earliest)
	}

	v := b.ViewAt(height, root)
	got, err := v.GetRootHash()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("recompute the root for block %d: %w", height, err)
	}
	if got != root {
		return nil, errors.InternalError.WithFormat(
			"reconstructed root for block %d is %x, but the ledger recorded %x", height, got, root)
	}
	return v.storedBlock(prefix)
}

// storedBlock walks to the block under prefix and reads its 256 positions. The
// positions come from the block's own record: the boundary nodes eight levels
// down carry their hash in it, so no deeper block is loaded.
func (b *BPT) storedBlock(prefix []byte) (*StoredBlock, error) {
	s, err := b.loadState()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if s.Power != storedBlockLevels {
		return nil, errors.NotAllowed.WithFormat(
			"the BPT is stored in blocks of %d levels; a stored block of %d is not defined", s.Power, storedBlockLevels)
	}
	if len(prefix) >= 32 {
		return nil, errors.BadRequest.WithFormat("a prefix of %d bytes leaves no key byte to answer for", len(prefix))
	}

	var key [32]byte
	copy(key[:], prefix)
	height := uint64(storedBlockLevels * len(prefix))
	nodeKey, ok := nodeKeyAt(height, key)
	if !ok {
		return nil, errors.BadRequest.WithFormat("no node at height %d", height)
	}

	top, err := b.getRoot().getBranch(nodeKey)
	switch {
	case err == nil:
	case errors.Is(err, errors.NotFound):
		return nil, errors.NotFound.WithFormat("no stored block under prefix %x: fewer than two keys share it", prefix)
	default:
		return nil, errors.UnknownError.Wrap(err)
	}
	err = top.load()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	out := &StoredBlock{Prefix: append([]byte{}, prefix...)}
	out.Hash, _ = top.getHash()
	err = collectSlots(top, 0, 0, len(prefix), &out.Slots)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// The slots are the block's own content, so they hash to its hash by the
	// tree's rule. If they do not, the walk above is wrong, and a wrong answer
	// here sends a joining node after the wrong accounts.
	if h := HashSlots(out.Slots); h != out.Hash {
		return nil, errors.InternalError.WithFormat(
			"the slots of the block under %x hash to %x, but the block's hash is %x", prefix, h[:8], out.Hash[:8])
	}
	return out, nil
}

// collectSlots appends the non-empty positions under n, which is `level`
// levels below the block's top and reached by the bits in idx.
func collectSlots(n node, level int, idx int, byteIdx int, out *[]BlockSlot) error {
	switch n := n.(type) {
	case *emptyNode:
		return nil

	case *leaf:
		kh := n.Key.Hash()
		// A leaf above the bottom of the block covers one position, the one
		// its own next byte names; the rest under it are empty.
		if int(kh[byteIdx])>>(storedBlockLevels-level) != idx {
			return errors.InternalError.WithFormat("leaf %x sits under the wrong branch", kh[:4])
		}
		h, _ := n.getHash()
		*out = append(*out, BlockSlot{Index: kh[byteIdx], KeyHash: kh, Hash: h})
		return nil

	case *branch:
		if level == storedBlockLevels {
			h, _ := n.getHash()
			*out = append(*out, BlockSlot{Index: byte(idx), Branch: true, Hash: h})
			return nil
		}
		if n.Left == nil {
			return errors.InternalError.WithFormat("branch %x inside a stored block was not loaded with it", n.Key[:4])
		}
		// Right is the clear bit, so it holds the lower indices
		err := collectSlots(n.Right, level+1, idx<<1, byteIdx, out)
		if err != nil {
			return err
		}
		return collectSlots(n.Left, level+1, idx<<1|1, byteIdx, out)

	default:
		return errors.InternalError.WithFormat("unknown node type %T", n)
	}
}

// HashSlots is the hash of a stored block computed from its positions by the
// BPT's rule: a set bit goes left, two non-empty sides combine left then right,
// and one non-empty side passes its hash up unchanged. A leaf that sits higher
// in the block than the bottom hashes the same as it would at the bottom, since
// everything beside it is empty, so the 256 positions determine the block's
// hash. A node checks an answer against the hash the answer above it gave for
// the same position.
func HashSlots(slots []BlockSlot) [32]byte {
	var level [256]struct {
		h  [32]byte
		ok bool
	}
	for _, s := range slots {
		level[s.Index].h, level[s.Index].ok = s.Hash, true
	}
	for n := 256; n > 1; n /= 2 {
		for i := 0; i < n/2; i++ {
			l, r := level[2*i+1], level[2*i]
			switch {
			case l.ok && r.ok:
				var b [64]byte
				copy(b[:], l.h[:])
				copy(b[32:], r.h[:])
				level[i].h, level[i].ok = sha256.Sum256(b[:]), true
			case l.ok:
				level[i] = l
			default:
				level[i] = r
			}
		}
	}
	return level[0].h
}
