// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package memory

import (
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/kvtest"
)

func open(testing.TB) kvtest.Opener {
	// Reuse the same in-memory database each time
	db := New(nil)
	return func() (keyvalue.Beginner, error) { return db, nil }
}

func TestSuite(t *testing.T) {
	kvtest.TestSuite(t, open(t))
}

func TestChangeSetAsDatabase(t *testing.T) {
	// TODO: Use a vmap for ChangeSet? This would require vmap having a root
	// view or equivalent. Is it actually useful for ChangeSet to have
	// isolation?
	t.Skip()
	db := New(nil).Begin(nil, true)
	kvtest.TestSuite(t, func() (keyvalue.Beginner, error) { return db, nil })
}

func BenchmarkCommit(b *testing.B) {
	kvtest.BenchmarkCommit(b, open(b))
}

func BenchmarkOpen(b *testing.B) {
	kvtest.BenchmarkOpen(b, open(b))
}

func BenchmarkReadRandom(b *testing.B) {
	kvtest.BenchmarkReadRandom(b, open(b))
}
