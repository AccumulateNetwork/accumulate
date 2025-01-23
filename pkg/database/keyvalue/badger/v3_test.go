// Copyright 2025 The Accumulate Authors
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

func BenchmarkV3Commit(b *testing.B) {
	kvtest.BenchmarkCommit(b, newOpenerV3(b))
}

func BenchmarkV3ReadRandom(b *testing.B) {
	kvtest.BenchmarkReadRandom(b, newOpenerV3(b))
}

func TestV3Database(t *testing.T) {
	kvtest.TestDatabase(t, newOpenerV3(t))
}

func TestV3SubBatch(t *testing.T) {
	kvtest.TestSubBatch(t, newOpenerV3(t))
}

func TestV3Prefix(t *testing.T) {
	kvtest.TestPrefix(t, newOpenerV3(t))
}

func TestV3Delete(t *testing.T) {
	kvtest.TestDelete(t, newOpenerV3(t))
}

func newOpenerV3(t testing.TB) kvtest.Opener {
	path := t.TempDir()
	return func() (keyvalue.Beginner, error) {
		return OpenV3(path)
	}
}
