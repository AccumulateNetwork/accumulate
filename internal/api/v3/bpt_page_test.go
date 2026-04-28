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

// TestBptPageQuery_DispatchPath verifies that BptPageQuery routes
// through the v3 dispatcher to bptproof.GetPage. Builds a synthetic
// BPT, queries the first page, confirms the contents match.
func TestBptPageQuery_DispatchPath(t *testing.T) {
	db := database.OpenInMemory(nil)

	// Insert 3 synthetic leaves into the BPT.
	type leaf struct {
		key, val [32]byte
	}
	leaves := []leaf{
		{[32]byte{0x10, 0x01}, [32]byte{0xa, 0xb, 0xc}},
		{[32]byte{0x20, 0x02}, [32]byte{0xd, 0xe, 0xf}},
		{[32]byte{0x30, 0x03}, [32]byte{0x1, 0x2, 0x3}},
	}
	batch := db.Begin(true)
	for _, l := range leaves {
		require.NoError(t, batch.BPT().Insert(record.KeyFromHash(l.key), l.val[:]))
	}
	require.NoError(t, batch.Commit())

	// Read the network's BPT root.
	roBatch := db.Begin(false)
	expectedRoot, err := roBatch.GetBptRootHash()
	require.NoError(t, err)
	require.NotEqual(t, [32]byte{}, expectedRoot)
	roBatch.Discard()

	q := dut.NewQuerier(dut.QuerierParams{
		Database:  db,
		Partition: "Directory",
	})

	scope, _ := url.Parse(protocol.DnUrl().String())
	rec, err := q.Query(context.Background(), scope, &api.BptPageQuery{Count: 100})
	require.NoError(t, err)

	page, ok := rec.(*api.BptPageRecord)
	require.True(t, ok, "expected *BptPageRecord, got %T", rec)
	require.Equal(t, expectedRoot, page.BptRoot)
	require.True(t, page.Done, "expected Done=true when count > leaves")
	require.Len(t, page.Entries, len(leaves))
}

// TestBptPageQuery_PaginationAcrossCalls walks a synthetic BPT one
// page at a time, accumulates all (key, value) pairs, and asserts
// completeness. Mirrors the consumer flow in bootstrap-v3.
func TestBptPageQuery_PaginationAcrossCalls(t *testing.T) {
	db := database.OpenInMemory(nil)

	const total = 30
	batch := db.Begin(true)
	for i := 0; i < total; i++ {
		var k [32]byte
		k[0] = byte(i)
		k[31] = 0xff
		var v [32]byte
		v[0] = byte(i)
		require.NoError(t, batch.BPT().Insert(record.KeyFromHash(k), v[:]))
	}
	require.NoError(t, batch.Commit())

	q := dut.NewQuerier(dut.QuerierParams{
		Database:  db,
		Partition: "Directory",
	})
	scope, _ := url.Parse(protocol.DnUrl().String())

	seen := make(map[[32]byte]bool)
	var start [32]byte
	for pages := 0; pages < 100; pages++ {
		rec, err := q.Query(context.Background(), scope, &api.BptPageQuery{
			StartHash: start,
			Count:     7,
		})
		require.NoError(t, err)
		page, ok := rec.(*api.BptPageRecord)
		require.True(t, ok)
		for _, e := range page.Entries {
			require.False(t, seen[e.KeyHash], "duplicate key across pages")
			seen[e.KeyHash] = true
		}
		if page.Done {
			break
		}
		start = page.NextStart
	}
	require.Len(t, seen, total, "all leaves should be seen across pages")
}
