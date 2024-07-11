// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package badger

import (
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/kvtest"
)

func BenchmarkV4Commit(b *testing.B) {
	kvtest.BenchmarkCommit(b, newOpenerV4(b))
}

func BenchmarkV4Open(b *testing.B) {
	kvtest.BenchmarkOpen(b, newOpenerV4(b))
}

func BenchmarkV4ReadRandom(b *testing.B) {
	kvtest.BenchmarkReadRandom(b, newOpenerV4(b))
}

func TestV4Database(t *testing.T) {
	kvtest.TestDatabase(t, newOpenerV4(t))
}

func TestV4SubBatch(t *testing.T) {
	kvtest.TestSubBatch(t, newOpenerV4(t))
}

func TestV4Prefix(t *testing.T) {
	kvtest.TestPrefix(t, newOpenerV4(t))
}

func TestV4Delete(t *testing.T) {
	kvtest.TestDelete(t, newOpenerV4(t))
}

func newOpenerV4(t testing.TB) kvtest.Opener {
	path := t.TempDir()
	return func() (keyvalue.Beginner, error) {
		return OpenV4(path)
	}
}
