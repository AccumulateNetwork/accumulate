// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bcdb

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// TestTheAdapterResumesAtTheStoresBlockRecord: after a kill, Shard0000's
// permanent manifest can stand below the block the store is accumulating
// into -- the store records that once, in block.json (BlockchainDB#32). An
// adapter that resumed from the manifest tried to seal a block the store had
// closed, and every write on the node failed ("block 865 is already closed;
// now accumulating block 866", run 20261003T182050Z).
func TestTheAdapterResumesAtTheStoresBlockRecord(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	db, err := Open(dir)
	require.NoError(t, err)
	db.StatsEvery = 0
	for i := 0; i < 5; i++ {
		put(t, db, record.NewKey("Account", "a", i, "Main"), "x")
	}
	require.NoError(t, db.Close())

	// What the kill left: the shard manifest a block behind the record.
	manifest := filepath.Join(dir, permManifest)
	b, err := os.ReadFile(manifest)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	m["blockHeight"] = m["blockHeight"].(float64) - 1
	b, err = json.Marshal(m)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manifest, b, 0600))

	db, err = Open(dir)
	require.NoError(t, err)
	defer db.Close()
	db.StatsEvery = 0
	put(t, db, record.NewKey("Account", "b", "Main"), "y")
}
