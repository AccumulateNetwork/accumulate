// Copyright 2026 The Accumulate Authors
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
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A restored node must answer "is this hash on this chain" the same way the
// node it was restored from does: the executor asks it of the Directory anchor
// chain (admissibility) and of the proven set (collection). Both go through
// the chain's element index.
func TestSnapshot_ChainIndexOfSurvivesRestore(t *testing.T) {
	anchorPool := protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)
	synthetic := protocol.PartitionUrl("BVN0").JoinPath(protocol.Synthetic)

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()
	pool := new(protocol.AnchorLedger)
	pool.Url = anchorPool
	require.NoError(t, batch.Account(anchorPool).Main().Put(pool))
	ledger := new(protocol.SyntheticLedger)
	ledger.Url = synthetic
	require.NoError(t, batch.Account(synthetic).Main().Put(ledger))

	var roots, hashes [][]byte
	anchors, err := batch.Account(anchorPool).AnchorChain(protocol.Directory).Root().Get()
	require.NoError(t, err)
	for i := 0; i < 5; i++ {
		r := sha256.Sum256([]byte(fmt.Sprintf("root %d", i)))
		h := sha256.Sum256([]byte(fmt.Sprintf("hash %d", i)))
		roots, hashes = append(roots, r[:]), append(hashes, h[:])
		require.NoError(t, anchors.AddEntry(r[:], false))
		_ = h
	}
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())

	buf := new(ioutil.Buffer)
	_, err = db.Collect(buf, nil, nil)
	require.NoError(t, err)
	db = database.OpenInMemory(nil)
	require.NoError(t, database.Restore(db, buf, nil))
	batch = db.Begin(false)
	defer batch.Discard()

	e, err := batch.Account(anchorPool).AnchorChain(protocol.Directory).Root().Entry(3)
	require.NoError(t, err, "a chain entry must survive restore")
	require.Equal(t, roots[3], e)
	i, err := batch.Account(anchorPool).AnchorChain(protocol.Directory).Root().IndexOf(roots[3])
	require.NoError(t, err, "a Directory anchor must still be findable after restore")
	require.Equal(t, int64(3), i)
}
