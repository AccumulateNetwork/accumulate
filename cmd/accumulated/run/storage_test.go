// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/leveldb"
)

// TestStorageMismatchDetected verifies that a database directory created by
// one backend cannot be opened by the other (#4052) — the node must fail
// loudly instead of booting an empty database.
func TestStorageMismatchDetected(t *testing.T) {
	// A fresh (nonexistent) directory is fine for either backend
	fresh := filepath.Join(t.TempDir(), "does-not-exist")
	require.NoError(t, checkStorageDir(fresh, StorageTypeBadger))
	require.NoError(t, checkStorageDir(fresh, StorageTypeLevelDB))

	// Create a real Badger database
	badgerDir := t.TempDir()
	bdb, err := badger.OpenV1(badgerDir)
	require.NoError(t, err)
	require.NoError(t, bdb.Close())

	// Create a real LevelDB database
	leveldbDir := t.TempDir()
	ldb, err := leveldb.Open(leveldbDir)
	require.NoError(t, err)
	require.NoError(t, ldb.Close())

	// Each backend accepts its own directory
	require.NoError(t, checkStorageDir(badgerDir, StorageTypeBadger))
	require.NoError(t, checkStorageDir(leveldbDir, StorageTypeLevelDB))

	// And refuses the other's
	err = checkStorageDir(badgerDir, StorageTypeLevelDB)
	require.ErrorContains(t, err, "created by badger")
	err = checkStorageDir(leveldbDir, StorageTypeBadger)
	require.ErrorContains(t, err, "created by levelDB")
}
