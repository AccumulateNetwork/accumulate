// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

func (v *stateData) CopyAsInterface() any { return v.Copy() }

// getRoot returns the root branch node, creating it if necessary.
func (b *BPT) getRoot() *branch {
	return values.GetOrCreate(b, &b.root, (*BPT).newRoot).branch
}

func (b *BPT) newRoot() *rootRecord {
	e := new(branch)
	e.bpt = b
	e.Height = 0
	e.Key, _ = nodeKeyAt(0, [32]byte{})
	return &rootRecord{e}
}

// Resolve implements [database.Record].
func (b *BPT) Resolve(key *database.Key) (database.Record, *database.Key, error) {
	if key.Len() == 0 {
		return nil, nil, errors.InternalError.With("bad key for bpt")
	}

	// Execute any pending updates
	err := b.executePending()
	if err != nil {
		return nil, nil, errors.UnknownError.Wrap(err)
	}

	if key.Get(0) == "Root" {
		return b.getState(), key.SliceI(1), nil
	}

	nodeKey, ok := key.Get(0).([32]byte)
	if !ok {
		return nil, nil, errors.InternalError.With("bad key for bpt")
	}
	e, err := b.getRoot().getBranch(nodeKey)
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("bad key for BPT: %x", err)
	}

	// Ensure the node is loaded
	err = e.load()
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("load node: %w", err)
	}

	return nodeRecord{e}, key.SliceI(1), nil
}
