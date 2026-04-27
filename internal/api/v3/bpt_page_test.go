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

// TestBptPageQuery_DispatchAndExhaust verifies BptPageQuery routes
// through the dispatcher and that paginating to exhaustion enumerates
// every leaf inserted into the BPT.
func TestBptPageQuery_DispatchAndExhaust(t *testing.T) {
	db := database.OpenInMemory(nil)

	// Insert 5 synthetic leaves with predictable key hashes.
	want := map[[32]byte][32]byte{}
	batch := db.Begin(true)
	for i := byte(1); i <= 5; i++ {
		var k, v [32]byte
		k[0] = i
		v[0] = i + 100
		require.NoError(t, batch.BPT().Insert(record.KeyFromHash(k), v[:]))
		want[k] = v
	}
	require.NoError(t, batch.Commit())

	q := dut.NewQuerier(dut.QuerierParams{Database: db, Partition: "Directory"})
	scope, _ := url.Parse(protocol.DnUrl().String())

	got := map[[32]byte][32]byte{}
	var rootSeen [32]byte
	var startHash [32]byte
	pages := 0
	for {
		pages++
		require.Less(t, pages, 100, "runaway pagination")

		rec, err := q.Query(context.Background(), scope, &api.BptPageQuery{
			StartHash: startHash,
			Count:     2, // small page to force iteration
		})
		require.NoError(t, err)
		page, ok := rec.(*api.BptPageRecord)
		require.True(t, ok, "expected *BptPageRecord, got %T", rec)

		// Root should be consistent across pages.
		if pages == 1 {
			rootSeen = page.BptRoot
		} else {
			require.Equal(t, rootSeen, page.BptRoot, "page root drifted")
		}

		for _, e := range page.Entries {
			got[e.KeyHash] = e.ValueHash
		}

		if page.Done {
			break
		}
		require.NotNil(t, page.NextStart, "non-Done page should carry NextStart")
		startHash = *page.NextStart
	}

	require.Equal(t, want, got, "exhausted enumeration should yield exactly the inserted set")
}
