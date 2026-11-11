// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package leveldb

import (
	"os"
	"path/filepath"

	"github.com/stretchr/testify/require"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/kvtest"
)

func open(t testing.TB) kvtest.Opener {
	dir := t.TempDir()
	return func() (keyvalue.Beginner, error) {
		return Open(dir)
	}
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

func TestDatabase(t *testing.T) {
	kvtest.TestDatabase(t, open(t))
}

func TestIsolation(t *testing.T) {
	kvtest.TestIsolation(t, open(t))
}

func TestSubBatch(t *testing.T) {
	kvtest.TestSubBatch(t, open(t))
}

func TestPrefix(t *testing.T) {
	kvtest.TestPrefix(t, open(t))
}

func TestDelete(t *testing.T) {
	kvtest.TestDelete(t, open(t))
}

func TestReadOnly(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing")
	kvtest.TestReadOnly(t,
		func() (keyvalue.Beginner, error) { return Open(filepath.Join(dir, "db")) },
		func() (keyvalue.Beginner, error) { return Open(filepath.Join(dir, "db"), ReadOnly()) },
		func() error {
			db, err := Open(missing, ReadOnly())
			if err == nil {
				_ = db.Close()
			}
			_, serr := os.Stat(missing)
			require.True(t, os.IsNotExist(serr), "read-only open created the directory")
			return err
		})
}
