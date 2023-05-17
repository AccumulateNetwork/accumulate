// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"crypto/sha256"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// nodeType is the type of an [Node].
type nodeType uint64

// node is an node in a [BPT].
type node interface {
	// Type is the type of the node.
	Type() nodeType

	// CopyAsInterface implements [encoding.BinaryValue].
	CopyAsInterface() any

	// IsDirty returns true if the node has been modified.
	IsDirty() bool

	// getHash returns the hash of the node, recalculating it if necessary.
	getHash() [32]byte

	// copyWith copies the receiver with the given branch as the parent of the
	// new copy.
	copyWith(e *branch, put bool) node

	// merge merges the given node into the receiver.
	merge(e node, put bool) (node, error)

	// writeTo marshals the node and writes it to the writer.
	writeTo(io.Writer) error

	// readFrom reads the node from the reader and unmarshals it.
	readFrom(io.Reader) error
}

// branchStatus is the status of a branch node.
type branchStatus int

const (
	// branchClean indicates the branch has not been changed.
	branchClean branchStatus = iota

	// branchUnhashed indicates the branch has been updated and its hash is out
	// of date.
	branchUnhashed

	// branchUncommitted indicates the branch has been updated and its hash is
	// up to date but it is yet to be committed.
	branchUncommitted
)

// IsDirty returns false.
func (e *emptyNode) IsDirty() bool { return false }

// IsDirty returns false.
func (e *leaf) IsDirty() bool { return false }

// IsDirty returns true if the branch has been updated.
func (e *branch) IsDirty() bool { return e.status != branchClean }

// newBranch constructs a new child branch for the given key. newBranch updates
// the parameters' max height if appropriate. newBranch returns an error if the
// depth limit is exceeded.
func (e *branch) newBranch(key [32]byte) (*branch, error) {
	// Construct the branch
	f := new(branch)
	f.bpt = e.bpt
	f.parent = e
	f.Height = e.Height + 1
	f.Left = &emptyNode{parent: f}
	f.Right = &emptyNode{parent: f}

	// Construct the key
	var ok bool
	f.Key, ok = nodeKeyAt(f.Height, key)
	if !ok {
		return nil, errors.FatalError.With("BPT depth limit exceeded")
	}

	// Update max height
	s, err := e.bpt.getState().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load params: %w", err)
	}
	if f.Height <= s.MaxHeight {
		return f, nil
	}

	s.MaxHeight = f.Height
	err = e.bpt.getState().Put(s)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store params: %w", err)
	}
	return f, nil
}

// getHash returns an empty hash.
func (*emptyNode) getHash() [32]byte { return [32]byte{} }

// getHash returns the leaf's hash.
func (e *leaf) getHash() [32]byte { return e.Hash }

// getHash returns the branch's hash, recalculating it if the branch has been
// changed since the last getHash call.
func (e *branch) getHash() [32]byte {
	if e.status != branchUnhashed {
		return e.Hash
	}

	switch { //                                        Sort four conditions:
	case e.Left.Type() != nodeTypeEmpty && //          If we have both L and R then combine
		e.Right.Type() != nodeTypeEmpty: //
		l, r := e.Left.getHash(), e.Right.getHash() // Take the hash of L+R
		var b [64]byte                              // Use a pre-allocated array to avoid spilling to the heap
		copy(b[:], l[:])                            //
		copy(b[32:], r[:])                          //
		e.Hash = sha256.Sum256(b[:])                //
	case e.Left.Type() != nodeTypeEmpty: //            The next condition is where we only have L
		e.Hash = e.Left.getHash() //                   Just use L.  No hash required
	case e.Right.Type() != nodeTypeEmpty: //           Just have R.  Again, just use R.
		e.Hash = e.Right.getHash() //                  No Hash Required
	default: //                                        The fourth condition never happens, and bad if it does.
		panic("dead nodes should not exist") //        This is a node without a child somewhere up the tree.
	}

	e.status = branchUncommitted
	return e.Hash
}

// getAt returns a pointer to the left or right branch, depending on the key,
// and loads the branch if necessary.
func (e *branch) getAt(key [32]byte) (*node, error) {
	BIdx := byte(e.Height >> 3) // Calculate the byte index based on the height of this node in the BPT
	bitIdx := e.Height & 7      // The bit index is given by the lower 3 bits of the height
	bit := byte(0x80) >> bitIdx // The mask starts at the high end bit in the byte, shifted right by the bitIdx
	n := &e.Left                // Assume Left
	if bit&key[BIdx] == 0 {     // Check for Right
		n = &e.Right //            Change to Right
	}

	// Load the block if necessary
	err := e.load()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return n, nil
}

// load loads the branch if it has not been loaded.
func (e *branch) load() error {
	// Does the node need to be loaded?
	if e.Left != nil {
		return nil
	}

	// If this is the root node, get the hash from the parameters
	if e.Height == 0 {
		s, err := e.bpt.getState().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load params: %w", err)
		}
		e.Hash = s.RootHash
	}

	// Set the branches to empty entries. If the block is not found, the
	// branches are empty. If the block is found, this tells merge to replace
	// the branches.
	e.Left = &emptyNode{parent: e}
	e.Right = &emptyNode{parent: e}
	err := e.bpt.store.GetValue(e.bpt.key.Append(e.Key), nodeRecord{e})
	if err != nil && !errors.Is(err, errors.NotFound) {
		return errors.UnknownError.Wrap(err)
	}
	return nil
}

// copyWith returns a new empty node with parent set to the given branch.
func (e *emptyNode) copyWith(p *branch, put bool) node {
	return &emptyNode{parent: p}
}

// copyWith returns a copy of the branch with parent set to the given branch.
// copyWith copies recursively if put is true.
func (e *branch) copyWith(p *branch, put bool) node {
	f := &branch{
		// Inherit from the parent
		bpt:    p.bpt,
		parent: p,

		// Copy from the target
		status: e.status,
		Height: e.Height,
		Key:    e.Key,
		Hash:   e.Hash,
	}
	if put && e.Left != nil {
		f.Left = e.Left.copyWith(f, put)
		f.Right = e.Right.copyWith(f, put)
	}
	return f
}

// copyWith returns a copy of the leaf node with parent set to the given branch.
// If the receiver's parent is nil, copyWith returns it instead after setting
// its parent.
func (e *leaf) copyWith(p *branch, put bool) node {
	// Optimization for Insert
	if e.parent == nil {
		e.parent = p
		return e
	}

	f := *e
	f.parent = p
	return &f
}

// merge replaces the receiver with a copy of the given node.
func (e *emptyNode) merge(f node, put bool) (node, error) {
	return f.copyWith(e.parent, put), nil
}

// merge merges the given node into the the receiver. If F is an empty node,
// merge does nothing and returns nil. If F is a branch, merge returns a new
// branch constructed from the receiver and F. Otherwise (F is a leaf), merge
// updates the receiver if F's hash is different or does nothing and returns
// nil.
func (e *leaf) merge(f node, put bool) (node, error) {
	// If the key and hash match, there's no change. If the key matches and the
	// hash is new, update the hash and return the value to indicate it has been
	// updated. If the key does not match, the value must be split.
	parent := e.parent
	switch f := f.(type) {
	case *emptyNode:
		// Nothing to do
		return nil, nil

	case *leaf:
		if e.Key != f.Key {
			// Make this node an orphan to avoid an allocation when copyWith is
			// called
			e.parent = nil

			// Split
			break
		}
		if e.Hash == f.Hash {
			// No change
			return nil, nil
		}

		// Update the hash
		e.Hash = f.Hash
		return e, nil

	default:
		// Split
	}

	// Create a new branch
	br, err := parent.newBranch(e.Key)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	// Merge E and F into the branch
	_, err = br.merge(f, put)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	_, err = br.merge(e, put)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Replace the leaf with the branch
	return br, nil
}

// merge merges the given node into the receiver. If F is an empty node, merge
// does nothing and returns nil. If F is a leaf, merge merges it into the
// appropriate side of the branch. If F is a branch, merge merges left into left
// and right into right.
func (e *branch) merge(f node, put bool) (node, error) {
	switch f := f.(type) {
	case *emptyNode:
		// Nothing to do
		return nil, nil

	case *leaf:
		// Merge the leaf with one of our branches
		g, err := e.getAt(f.Key)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		new, err := (*g).merge(f, put)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		if new == nil {
			// Nothing changed
			return nil, nil
		}

		*g = new

	case *branch:
		// Verify both branches are at the same height
		if e.Height != f.Height {
			return nil, errors.InternalError.WithFormat("cannot merge %v nodes: height does not match", e.Type())
		}
		if e.Key != f.Key {
			return nil, errors.InternalError.WithFormat("cannot merge %v nodes: key does not match", e.Type())
		}

		// Merge the left
		var didUpdate bool
		if e.Left.getHash() != f.Left.getHash() {
			new, err := e.Left.merge(f.Left, put)
			if err != nil {
				return nil, errors.UnknownError.Wrap(err)
			}
			if new != nil {
				didUpdate = true
				e.Left = new
			}
		}

		// Merge the right
		if e.Right.getHash() != f.Right.getHash() {
			new, err := e.Right.merge(f.Right, put)
			if err != nil {
				return nil, errors.UnknownError.Wrap(err)
			}
			if new != nil {
				didUpdate = true
				e.Right = new
			}
		}

		if !didUpdate {
			return nil, nil
		}

	default:
		return nil, errors.InternalError.WithFormat("unknown node type %v", f.Type())
	}

	// If we're putting and something changed, set the status
	if put {
		e.status = branchUnhashed
	}
	return e, nil
}
