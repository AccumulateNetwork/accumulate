// Copyright 2026 The Accumulate Authors
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

// A DeepBeginner can begin a change set that reads the store's whole
// history, not just the recent window it answers protocol reads from.
//
// Only a store that HAS a window implements this. BlockchainDB answers
// a permanent read from the last N to 2N blocks and calls anything
// older absent, because probing history per segment on every miss cost
// 23% of a validator's CPU and grew with the chain; the data is still
// there, and a reader that knowingly looks back -- the API, healing, a
// tool walking history -- asks for it here. Stores with no window
// (LevelDB, Badger, Bolt, memory) do not implement this and need not:
// their Begin already reads everything.
type DeepBeginner interface {
	Beginner

	// BeginDeep is Begin for a reader that reaches past the window.
	BeginDeep(prefix *database.Key, writable bool) ChangeSet
}

// Deep returns a Beginner whose change sets read the whole history, if
// the store distinguishes; otherwise it returns the store unchanged,
// which already does.
func Deep(b Beginner) Beginner {
	if d, ok := b.(DeepBeginner); ok {
		return deepBeginner{d}
	}
	return b
}

type deepBeginner struct{ d DeepBeginner }

func (b deepBeginner) Begin(prefix *database.Key, writable bool) ChangeSet {
	return b.d.BeginDeep(prefix, writable)
}
