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
	copyWith(*parameters, *branch) node

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

	err := e.bpt.store.GetValue(e.bpt.key.Append(e.Key), nodeRecord{e})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errors.NotFound):
		e.Left = &emptyNode{parent: e}
		e.Right = &emptyNode{parent: e}
	default:
		return errors.UnknownError.Wrap(err)
	}
	return nil
}

// copyWith returns a new empty node with parent set to the given branch.
func (e *emptyNode) copyWith(s *parameters, p *branch) node {
	return &emptyNode{parent: p}
}

// copyWith returns a copy of the branch with parent set to the given branch.
// copyWith copies recursively if put is true.
func (e *branch) copyWith(s *parameters, p *branch) node {
	f := &branch{
		// Inherit from the new parent
		bpt:    p.bpt,
		parent: p,

		// Copy from the target
		status: e.status,
		Height: e.Height,
		Key:    e.Key,
		Hash:   e.Hash,
	}

	// If the branch is at a boundary, don't recurse
	if e.Height&s.Mask == 0 {
		return f
	}

	f.Left = e.Left.copyWith(s, f)
	f.Right = e.Right.copyWith(s, f)
	return f
}

// copyWith returns a copy of the leaf node with parent set to the given branch.
// If the receiver's parent is nil, copyWith returns it instead after setting
// its parent.
func (e *leaf) copyWith(s *parameters, p *branch) node {
	f := *e
	f.parent = p
	return &f
}
