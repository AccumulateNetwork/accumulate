// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Anchor staging is staging too (executor spec, "Staging in a snapshot"): a
// node restored without the proofs waiting on an anchor, or without the newest
// anchor block it executed, decides those proofs differently from its peers
// when the anchor lands.
func TestSnapshot_PreservesAnchorStaging(t *testing.T) {
	synthetic := protocol.PartitionUrl("BVN0").JoinPath(protocol.Synthetic)
	anchorPool := protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)
	source := protocol.PartitionUrl("BVN1")

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	ledger := new(protocol.SyntheticLedger)
	ledger.Url = synthetic
	require.NoError(t, batch.Account(synthetic).Main().Put(ledger))
	pool := new(protocol.AnchorLedger)
	pool.Url = anchorPool
	require.NoError(t, batch.Account(anchorPool).Main().Put(pool))

	proof := &protocol.AnnotatedReceipt{
		ReceiptList: &merkle.ReceiptList{MerkleState: new(merkle.State), Elements: [][]byte{make([]byte, 32)}},
		Anchor:      &protocol.AnchorMetadata{Account: protocol.DnUrl(), SourceBlock: 12},
	}
	acct := batch.Account(synthetic)
	require.NoError(t, acct.StagedProofs(source, 12).Add(proof))
	require.NoError(t, acct.StagedProofBlocks(source).Add(12))
	require.NoError(t, acct.StagedSources().Add(source))
	require.NoError(t, batch.Account(anchorPool).DirectoryAnchorBlock().Put(11))
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())

	buf := new(ioutil.Buffer)
	_, err := db.Collect(buf, nil, nil)
	require.NoError(t, err)

	db = database.OpenInMemory(nil)
	require.NoError(t, database.Restore(db, buf, nil))
	batch = db.Begin(false)
	defer batch.Discard()

	blocks, err := batch.Account(synthetic).StagedProofBlocks(source).Get()
	require.NoError(t, err)
	require.Equal(t, []uint64{12}, blocks, "the blocks proofs wait on survive the snapshot")
	proofs, err := batch.Account(synthetic).StagedProofs(source, 12).Get()
	require.NoError(t, err)
	require.Len(t, proofs, 1, "and so do the proofs")
	require.Equal(t, uint64(12), proofs[0].Anchor.SourceBlock)
	executed, err := batch.Account(anchorPool).DirectoryAnchorBlock().Get()
	require.NoError(t, err)
	require.Equal(t, uint64(11), executed, "and the newest executed anchor block")
}
