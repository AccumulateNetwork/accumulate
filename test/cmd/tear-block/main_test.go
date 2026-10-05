// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestTearBlockTearsAStoredBlockOfARealStore drives the tool against a badger
// store found through a node's accumulate.toml, as it finds a follower's: a
// store that retained twenty blocks of BPT history has one stored block put
// back to a retained version, and that block then fails its own check.
func TestTearBlockTearsAStoredBlockOfARealStore(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "accumulate.toml"), []byte(`
network = "TearTest"

[[services]]
  name = "BVN0"
  type = "storage"
  [services.storage]
    path = "data/accumulate.db"
    type = "badger"
`), 0600))

	store, _, closeStore, err := run.OpenPartitionStore(context.Background(), dir, "BVN0")
	require.NoError(t, err)
	db := database.New(store, nil)
	for h := uint64(1); h <= 20; h++ {
		b := db.Begin(true)
		b.BPT().SetHistory(h, 1024)
		for i := 0; i < 300; i++ {
			u := protocol.AccountUrl(fmt.Sprintf("tear%03d", i), "tokens")
			a := &protocol.TokenAccount{Url: u, TokenUrl: protocol.AcmeUrl()}
			a.Balance.SetUint64(h*1000 + uint64(i))
			require.NoError(t, b.Account(u).Main().Put(a))
		}
		require.NoError(t, b.UpdateBPT())
		require.NoError(t, b.Commit())
	}
	require.NoError(t, closeStore())

	require.NoError(t, tear(dir, "BVN0", "", false))
}
