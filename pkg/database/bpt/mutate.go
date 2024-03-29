// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

type mutation struct {
	applied   bool
	committed bool
	delete    bool
	key       *record.Key
	value     [32]byte
}

// Insert updates or inserts a hash for the given key. Insert may defer the
// actual update.
func (b *BPT) Insert(key *record.Key, hash [32]byte) error {
	if b.pending == nil {
		b.pending = map[[32]byte]*mutation{}
	}
	b.pending[key.Hash()] = &mutation{key: key, value: hash}
	return nil
}

// Delete removes the entry for the given key, if present. Delete may defer the
// actual update.
func (b *BPT) Delete(key *record.Key) error {
	if b.pending == nil {
		b.pending = map[[32]byte]*mutation{}
	}
	b.pending[key.Hash()] = &mutation{key: key, delete: true}
	return nil
}

// executePending pushes pending updates into the tree.
func (b *BPT) executePending() error {
	// Push the updates
	for _, e := range b.pending {
		if e.applied {
			continue
		}
		e.applied = true

		var err error
		if e.delete {
			_, err = b.getRoot().delete(e.key)
		} else {
			_, err = b.getRoot().insert(&leaf{Key: e.key, Hash: e.value})
		}
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}
	return nil
}

func (e *branch) insert(l *leaf) (updated bool, err error) {
	// Get a pointer to the left or right, and load if necessary
	f, err := e.getAt(l.Key.Hash())
	if err != nil {
		return false, errors.UnknownError.Wrap(err)
	}

	// Adopt the leaf
	l.parent = e

	switch g := (*f).(type) {
	case *emptyNode:
		// Replace the empty node
		*f = l
		e.status = branchUnhashed
		return true, nil

	case *branch:
		// Insert into the branch
		updated, err = g.insert(l)
		if !updated {
			return false, err
		}

		e.status = branchUnhashed
		return true, nil

	case *leaf:
		// If the key and hash match, there's no change. If the key matches and the
		// hash is new, update the hash and return the value to indicate it has been
		// updated. If the key does not match, the value must be split.
		if g.Key.Hash() == l.Key.Hash() {
			if g.Hash == l.Hash {
				return false, nil // No change
			}

			g.Key = l.Key             // Update the key in case its expanded now
			g.Hash = l.Hash           // Update the hash
			e.status = branchUnhashed // Mark the branch as unhashed
			return true, nil
		}

		// Create a new branch
		br, err := e.newBranch(g.Key.Hash())
		if err != nil {
			return false, errors.UnknownError.Wrap(err)
		}

		// Insert the leaves into the branch
		_, err = br.insert(g)
		if err != nil {
			return false, errors.UnknownError.Wrap(err)
		}

		_, err = br.insert(l)
		if err != nil {
			return false, errors.UnknownError.Wrap(err)
		}

		// Replace the leaf with the branch
		*f = br
		e.status = branchUnhashed
		return true, nil

	default:
		return false, errors.InternalError.WithFormat("unknown node type %T", g)
	}
}

func (e *branch) delete(key *record.Key) (updated bool, err error) {
	// Get a pointer to the left or right, and load if necessary
	f, err := e.getAt(key.Hash())
	if err != nil {
		return false, errors.UnknownError.Wrap(err)
	}

	switch g := (*f).(type) {
	case *emptyNode:
		return false, nil // Key not found

	case *leaf:
		if g.Key.Hash() != key.Hash() {
			return false, nil // Key not found
		}

		// Replace the leaf with an empty node
		*f = &emptyNode{parent: e}
		e.status = branchUnhashed
		return true, nil

	case *branch:
		// Delete from the branch
		updated, err = g.delete(key)
		if !updated {
			return false, err
		}

		// Collapse the branch
		lt, rt := g.Left.Type(), g.Right.Type()
		switch {
		case lt == nodeTypeBranch || rt == nodeTypeBranch:
			// Can't collapse if either branch is a branch
		case lt == nodeTypeEmpty:
			// Collapse to the right
			*f = g.Right.copyWith(nil, e, false) // nil is safe because the receiver can't be a branch
		case rt == nodeTypeEmpty:
			// Collapse to the left
			*f = g.Left.copyWith(nil, e, false)
		}

		e.status = branchUnhashed
		return true, nil

	default:
		return false, errors.InternalError.WithFormat("unknown node type %T", g)
	}
}
