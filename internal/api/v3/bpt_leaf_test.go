// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	dut "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestBptLeafQuery_DispatchPath verifies that a BptLeafQuery routes
// through the v3 dispatcher and reaches bptproof.GetLeaf. We populate
// the BPT with one synthetic leaf, query it, and verify the proof
// anchors at the partition's BPT root.
func TestBptLeafQuery_DispatchPath(t *testing.T) {
	db := database.OpenInMemory(nil)

	// Insert a synthetic leaf. The BPT key→value pair is independent of
	// any account record; we just need *something* in the BPT to query.
	keyHash := [32]byte{0x01, 0x02, 0x03}
	valueHash := [32]byte{0xa, 0xb, 0xc}

	batch := db.Begin(true)
	require.NoError(t, batch.BPT().Insert(record.KeyFromHash(keyHash), valueHash[:]))
	require.NoError(t, batch.Commit())

	rootHash, err := db.Begin(false).GetBptRootHash()
	require.NoError(t, err)
	require.NotEqual(t, [32]byte{}, rootHash, "root should not be zero")

	q := dut.NewQuerier(dut.QuerierParams{
		Database:  db,
		Partition: "Directory",
	})

	scope, _ := url.Parse(protocol.DnUrl().String())
	rec, err := q.Query(context.Background(), scope, &api.BptLeafQuery{Key: keyHash})
	require.NoError(t, err)

	leaf, ok := rec.(*api.BptLeafRecord)
	require.True(t, ok, "expected *BptLeafRecord, got %T", rec)
	require.Equal(t, keyHash, leaf.KeyHash)
	require.Equal(t, valueHash, leaf.ValueHash)
	require.Equal(t, rootHash, leaf.BptRoot)
	require.NotNil(t, leaf.Proof)
}
