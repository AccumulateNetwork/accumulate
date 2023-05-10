// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package keyvalue

import "gitlab.com/accumulatenetwork/accumulate/pkg/database"

// ChangeSet is a key-value change set.
type ChangeSet interface {
	Store
	Beginner

	// Commit commits pending changes.
	Commit() error

	// Discard discards pending changes.
	Discard()
}

// A Beginner can begin key-value change sets.
type Beginner interface {
	// Begin begins a transaction or sub-transaction with a prefix applied to keys.
	Begin(prefix *database.Key, writable bool) ChangeSet
}
