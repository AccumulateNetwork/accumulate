// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// EXPERIMENT, NOT A FIX. What does each candidate rebuild write for a hash that
// sits at several positions on a chain, and what does the node that BUILT the
// chain have?

// buildChainWithRepeat writes A, B, C, C onto the Directory anchor root chain
// of a BVN's anchor pool, using the same AddEntry the executor uses, and
// returns the database.
func buildChainWithRepeat(t *testing.T) (*database.Database, *protocol.AnchorLedger, [][]byte) {
	t.Helper()
	anchorPool := protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	pool := new(protocol.AnchorLedger)
	pool.Url = anchorPool
	require.NoError(t, batch.Account(anchorPool).Main().Put(pool))

	c, err := batch.Account(anchorPool).AnchorChain(protocol.Directory).Root().Get()
	require.NoError(t, err)

	var hashes [][]byte
	for _, name := range []string{"A", "B", "C", "C"} {
		h := sha256.Sum256([]byte(fmt.Sprintf("root %s", name)))
		hashes = append(hashes, h[:])
		// unique=false: exactly what block_begin.go:250, synthetic.go:201,
		// block_end.go:90, utils.go:95 and utils.go:114 pass.
		require.NoError(t, c.AddEntry(h[:], false))
	}
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
	return db, pool, hashes
}

func indexOf(t *testing.T, db *database.Database, hash []byte) (int64, error) {
	t.Helper()
	anchorPool := protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)
	batch := db.Begin(false)
	defer batch.Discard()
	return batch.Account(anchorPool).AnchorChain(protocol.Directory).Root().IndexOf(hash)
}

func chainHeight(t *testing.T, db *database.Database) int64 {
	t.Helper()
	anchorPool := protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)
	batch := db.Begin(false)
	defer batch.Discard()
	c, err := batch.Account(anchorPool).AnchorChain(protocol.Directory).Root().Get()
	require.NoError(t, err)
	return c.Height()
}

// TestRebuildVariant_RepeatedHashIndexValue is the arithmetic of the claim:
// for a hash at heights 2 and 3, what does the index say?
func TestRebuildVariant_RepeatedHashIndexValue(t *testing.T) {
	built, _, hashes := buildChainWithRepeat(t)
	C := hashes[2]
	require.Equal(t, C, hashes[3], "C must be the repeated hash")
	require.Equal(t, int64(4), chainHeight(t, built), "the duplicate must have been appended")

	// What the node that built the chain has
	i, err := indexOf(t, built, C)
	require.NoError(t, err, "the building node indexes the repeated hash")
	t.Logf("BUILT (main AddEntry):        IndexOf(C) = %d", i)
	require.Equal(t, int64(2), i, "main's AddEntry records the FIRST occurrence")

	// Snapshot once, restore three ways
	buf := new(ioutil.Buffer)
	batch := built.Begin(false)
	_, err = batch.Collect(buf, nil, nil)
	require.NoError(t, err)
	batch.Discard()
	snap := buf.Bytes()

	type result struct {
		name  string
		mode  database.RebuildChainIndexModeT
		index int64
		err   error
	}
	var results []result
	for _, tc := range []struct {
		name string
		mode database.RebuildChainIndexModeT
	}{
		{"NONE (main today)", database.RebuildNone},
		{"VERBATIM (DI port)", database.RebuildVerbatim},
		{"FIRST-OCCURRENCE", database.RebuildFirstOccurrence},
	} {
		func() {
			old := database.RebuildChainIndexMode
			database.RebuildChainIndexMode = tc.mode
			defer func() { database.RebuildChainIndexMode = old }()

			db := database.OpenInMemory(nil)
			require.NoError(t, database.Restore(db, ioutil.NewBuffer(snap), nil))
			require.Equal(t, int64(4), chainHeight(t, db), "the chain must survive restore")
			i, err := indexOf(t, db, C)
			results = append(results, result{tc.name, tc.mode, i, err})
		}()
	}

	for _, r := range results {
		if r.err != nil {
			t.Logf("RESTORED %-22s IndexOf(C) = <%v>", r.name, r.err)
		} else {
			t.Logf("RESTORED %-22s IndexOf(C) = %d", r.name, r.index)
		}
	}

	// NONE: missing
	require.Error(t, results[0].err)
	require.True(t, errors.Is(results[0].err, errors.NotFound), "NONE leaves the index missing")

	// VERBATIM: present, but names the LAST occurrence
	require.NoError(t, results[1].err)
	require.Equal(t, int64(3), results[1].index,
		"the verbatim port writes the LAST occurrence")

	// FIRST-OCCURRENCE: present, and agrees with the building node
	require.NoError(t, results[2].err)
	require.Equal(t, int64(2), results[2].index,
		"the first-occurrence port agrees with the building node")

	// The load-bearing comparison: does the restored node's answer match the
	// node that built the chain?
	require.NotEqual(t, i, results[1].index, "VERBATIM disagrees with the builder")
	require.Equal(t, i, results[2].index, "FIRST-OCCURRENCE agrees with the builder")
}

// TestRebuildVariant_PresenceIsRestoredByBoth separates the two properties: a
// consumer that reads the index for PRESENCE only cannot tell the variants
// apart. All three named consumers (holdsAnchorRoot, CreateTokenAccount,
// SetLiteAccountDelegate) discard the value.
func TestRebuildVariant_PresenceIsRestoredByBoth(t *testing.T) {
	built, _, hashes := buildChainWithRepeat(t)
	buf := new(ioutil.Buffer)
	batch := built.Begin(false)
	_, err := batch.Collect(buf, nil, nil)
	require.NoError(t, err)
	batch.Discard()
	snap := buf.Bytes()

	for _, tc := range []struct {
		name    string
		mode    database.RebuildChainIndexModeT
		present bool
	}{
		{"NONE", database.RebuildNone, false},
		{"VERBATIM", database.RebuildVerbatim, true},
		{"FIRST-OCCURRENCE", database.RebuildFirstOccurrence, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := database.RebuildChainIndexMode
			database.RebuildChainIndexMode = tc.mode
			defer func() { database.RebuildChainIndexMode = old }()

			db := database.OpenInMemory(nil)
			require.NoError(t, database.Restore(db, ioutil.NewBuffer(snap), nil))
			for j, h := range hashes {
				_, err := indexOf(t, db, h)
				require.Equal(t, tc.present, err == nil,
					"entry %d presence", j)
			}
		})
	}
}
