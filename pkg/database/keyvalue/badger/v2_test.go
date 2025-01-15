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

func BenchmarkV2Commit(b *testing.B) {
	kvtest.BenchmarkCommit(b, newOpenerV2(b))
}

func BenchmarkV2ReadRandom(b *testing.B) {
	kvtest.BenchmarkReadRandom(b, newOpenerV2(b))
}

func TestV2Database(t *testing.T) {
	kvtest.TestDatabase(t, newOpenerV2(t))
}

func TestV2SubBatch(t *testing.T) {
	kvtest.TestSubBatch(t, newOpenerV2(t))
}

func TestV2Prefix(t *testing.T) {
	kvtest.TestPrefix(t, newOpenerV2(t))
}

func TestV2Delete(t *testing.T) {
	kvtest.TestDelete(t, newOpenerV2(t))
}

func newOpenerV2(t testing.TB) kvtest.Opener {
	path := t.TempDir()
	return func() (keyvalue.Beginner, error) {
		return OpenV2(path)
	}
}
