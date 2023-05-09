// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"bytes"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

// errShim returns an error indicating that the caller is a shim and should not
// be used.
func errShim() error {
	return errors.FatalError.Skip(1).With("This is just a shim, why did you call me!")
}

// IsDirty returns true if the BPT has pending updates.
func (b *BPT) IsDirty() bool {
	return len(b.pending) > 0 || b.baseIsDirty()
}

// WalkChanges implements [record.Record].
func (b *BPT) Walk(opts database.WalkOptions, fn database.WalkFunc) error {
	// Walking the BPT is not supported
	return nil
}

// commitUpdatesDirect directly pushes pending updates into the previous batch,
// as long as the previous layer _is_ a batch and there are no other changes.
func (b *BPT) commitUpdatesDirect() (bool, error) {
	// We can't push the updates directly if any state has been updated
	if b.baseIsDirty() {
		return false, nil
	}

	// Is the store a record?
	rs, ok := b.store.(interface{ Unwrap() database.Record })
	if !ok {
		return false, nil
	}

	var err error
	r := rs.Unwrap()
	for key := b.key; key.Len() > 0; {
		r, key, err = r.Resolve(key)
		if err != nil {
			return false, errors.UnknownError.Wrap(err)
		}
	}

	// Is the record a BPT?
	c, ok := r.(*BPT)
	if !ok {
		return false, nil
	}

	// Push the updates
	for k, v := range b.pending {
		err = c.Insert(k, v)
		if err != nil {
			return false, errors.UnknownError.Wrap(err)
		}
	}

	// Optimized by the compiler
	for k := range b.pending {
		delete(b.pending, k)
	}

	return true, nil
}

// Commit commits the BPT.
func (b *BPT) Commit() error {
	// If we're not dirty there's nothing to do
	if !b.IsDirty() {
		return nil
	}

	// If no changes have been made to state - that is, nothing has been updated
	// except adding entries to the pending map - push the pending map directly
	// to the parent layer
	if ok, err := b.commitUpdatesDirect(); err != nil {
		return errors.UnknownError.Wrap(err)
	} else if ok {
		return nil
	}

	// Execute pending updates
	err := b.executePending()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Update the root hash
	s, err := b.getState().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load params: %w", err)
	}
	s.RootHash = b.getRoot().getHash()
	err = b.getState().Put(s)
	if err != nil {
		return errors.UnknownError.WithFormat("store params: %w", err)
	}

	// Commit
	return b.baseCommit()
}

// nodeRecord is a wrapper for [node] that implements [record.Record].
type nodeRecord struct{ value node }

// Assert [nodeRecord] is a value reader and writer.
var _ database.Value = nodeRecord{}

func (e nodeRecord) Key() *database.Key { panic(errShim()) }

// IsDirty implements [record.Record].
func (e nodeRecord) IsDirty() bool { return e.value.IsDirty() }

// Commit panics.
func (e nodeRecord) Commit() error { panic(errShim()) }

// Resolve implements [record.Record].
func (e nodeRecord) Resolve(key *record.Key) (record.Record, *record.Key, error) {
	if key.Len() > 0 {
		return nil, nil, errors.InternalError.With("bad key for bpt entry")
	}
	return e, nil, nil
}

// Walk implements [record.Record].
func (e nodeRecord) Walk(opts database.WalkOptions, fn database.WalkFunc) error {
	// Walking the BPT is not supported
	return nil
}

// GetValue implements [record.ValueReader].
func (e nodeRecord) GetValue() (encoding.BinaryValue, int, error) {
	return nodeValue(e), 0, nil
}

// LoadValue implements [record.ValueWriter].
func (e nodeRecord) LoadValue(value record.ValueReader, put bool) error {
	// Get the value
	v, _, err := value.GetValue()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// It must be a [nodeValue]
	u, ok := v.(nodeValue)
	if !ok {
		return errors.InternalError.WithFormat("invalid value: want %T, got %T", nodeValue{}, value)
	}

	// The node must be a branch
	f, ok := u.value.(*branch)
	if !ok {
		return errors.InternalError.WithFormat("invalid entry: want %v, got %v", nodeTypeBranch, u.value.Type())
	}

	// Merge the branch into this node
	_, err = e.value.merge(f, put)
	if err != nil {
		return errors.UnknownError.WithFormat("load node: %w", err)
	}
	return nil
}

// LoadBytes implements [record.ValueWriter].
func (e nodeRecord) LoadBytes(data []byte, put bool) error {
	// Directly writing bytes is not supported
	if put {
		return errors.FatalError.With("not supported")
	}

	// The receiver must be a branch
	f, ok := e.value.(*branch)
	if !ok {
		return errors.InternalError.WithFormat("invalid entry: want %v, got %v", nodeTypeBranch, e.value.Type())
	}

	// Load the BPT's parameters
	s, err := f.bpt.getState().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load BPT params: %w", err)
	}

	// The branch must be on a boundary
	if f.Height&s.Mask != 0 {
		return errors.FatalError.WithFormat("attempted to load a non-border node from disk")
	}

	// Read the left and right
	rd := bytes.NewBuffer(data)
	f.Left, err = readNode(rd, f)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	f.Right, err = readNode(rd, f)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	return nil
}

// nodeValue is a wrapper for [node] that implements [encoding.BinaryValue].
type nodeValue struct{ value node }

// CopyAsInterface panics.
func (e nodeValue) CopyAsInterface() any { panic(errShim()) }

// UnmarshalBinaryFrom panics.
func (e nodeValue) UnmarshalBinaryFrom(io.Reader) error { panic(errShim()) }

// UnmarshalBinary panics.
func (e nodeValue) UnmarshalBinary(data []byte) error { panic(errShim()) }

// MarshalBinary implements [encoding.BinaryValue].
func (e nodeValue) MarshalBinary() (data []byte, err error) {
	// The node must be a branch
	f, ok := e.value.(*branch)
	if !ok {
		return nil, errors.InternalError.WithFormat("attempted to store a %v", e.value.Type())
	}

	// Load the BPT's parameters
	s, err := f.bpt.getState().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load params: %w", err)
	}

	// The branch must be on a boundary
	if f.Height&s.Mask != 0 {
		return nil, errors.InternalError.WithFormat("attempted to store a non-border branch")
	}

	// Write the left and right
	buf := new(bytes.Buffer)
	writeBlock(&err, buf, f.Left, s.Mask)
	writeBlock(&err, buf, f.Right, s.Mask)
	return buf.Bytes(), errors.UnknownError.Wrap(err)
}

// rootRecord is a wrapper for the root node that implements [record.Record].
type rootRecord struct{ *branch }

var _ database.Record = (*rootRecord)(nil)

func (e *rootRecord) Key() *database.Key { return e.bpt.key.Append(e.Key) }

// Commit implements [record.Commit].
func (e *rootRecord) Commit() error {
	// Load the BPT's parameters
	s, err := e.bpt.getState().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load params: %w", err)
	}

	// Starting with the root node, while there are nodes to process...
	branches := []*branch{e.branch}
	var next []*branch
	for len(branches) > 0 {
		for _, e := range branches {
			// If the node is clean, there's nothing to do
			if !e.IsDirty() {
				continue
			}

			// If the node is on a boundary, write it
			if e.Height&s.Mask == 0 {
				err := e.bpt.store.PutValue(e.bpt.key.Append(e.Key), nodeRecord{e})
				if err != nil {
					return errors.UnknownError.Wrap(err)
				}
			}

			// If if the left/right is a branch, add it to the queue
			if f, ok := e.Left.(*branch); ok {
				next = append(next, f)
			}
			if f, ok := e.Right.(*branch); ok {
				next = append(next, f)
			}
		}

		// Swap the lists; slicing instead of setting to nil avoids unnecessary
		// allocations
		branches, next = next, branches[:0]
	}
	return nil
}

// Resolve implements [record.Commit].
func (e *rootRecord) Resolve(key *record.Key) (record.Record, *record.Key, error) {
	return nodeRecord{e.branch}.Resolve(key)
}

// WalkChanges implements [record.Commit].
func (e *rootRecord) Walk(opts database.WalkOptions, fn database.WalkFunc) error {
	return nodeRecord{e.branch}.Walk(opts, fn)
}
