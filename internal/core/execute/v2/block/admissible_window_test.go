// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"crypto/sha256"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/bcdb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestAProofAnchoredPastTheStoresWindowIsAdmissible: a proof whose terminal
// anchor is a Directory root this partition recorded long ago is admissible,
// however far past the store's window that root is. BlockchainDB answers a
// shallow read from its newest blocks only and calls anything older absent
// (database spec, "Windowed stores"); the admissibility check ran in the
// block's shallow batch, so a proof pulled for an entry undelivered longer
// than the window was "disproved" and dropped for good. Run
// 20261002T090546Z: BVN2->BVN1 held at entry 1896 behind a proof anchored
// at Directory block 13035 whose root every BVN1 node held at chain index
// 12289 and no shallow batch could find; 8,846 entries queued behind it.
func TestAProofAnchoredPastTheStoresWindowIsAdmissible(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	describe := execute.DescribeShim{NetworkType: protocol.PartitionTypeBlockValidator, PartitionId: "BVN1"}

	// Eighty blocks each recording one Directory root, on a store that
	// packs its finished blocks every ten: the first roots are in packed
	// history by the end. Closing waits for the maintenance in flight.
	store, err := bcdb.Open(dir)
	require.NoError(t, err)
	store.StatsEvery, store.CompressEvery, store.PackEvery = 0, 5, 10
	require.NoError(t, store.SetMergeLag(20))
	db := database.New(store, nil)
	var roots [][]byte
	for i := 0; i < 80; i++ {
		batch := db.Begin(true)
		h := sha256.Sum256([]byte{byte(i)})
		roots = append(roots, h[:])
		pool := batch.Account(describe.AnchorPool())
		require.NoError(t, pool.AnchorChain(protocol.Directory).Root().Inner().AddEntry(h[:], false))
		// The anchor pool records the newest Directory block executed, as
		// executing a Directory anchor does.
		require.NoError(t, pool.DirectoryAnchorBlock().Put(uint64(i)+1))
		require.NoError(t, batch.Commit())
	}
	require.NoError(t, store.Close())

	store, err = bcdb.Open(dir)
	require.NoError(t, err)
	defer store.Close()
	store.StatsEvery = 0
	x := &Executor{ExecutorOptions: execute.Options{Database: database.New(store, nil), Describe: describe}}
	batch := x.Database.Begin(false)
	defer batch.Discard()
	for _, c := range []struct {
		name  string
		index int64
	}{{"the newest root", 79}, {"a root in packed history", 0}} {
		proof := &protocol.AnnotatedReceipt{
			Receipt: &merkle.Receipt{Anchor: roots[c.index]},
			Anchor:  directoryAnchorMetadata(uint64(c.index) + 1),
		}
		i, ok, err := x.provingAnchorIndex(batch, proof)
		require.NoError(t, err, c.name)
		require.True(t, ok, "%s is in the chain and was not admitted", c.name)
		require.Equal(t, c.index, i, c.name)
	}
}
