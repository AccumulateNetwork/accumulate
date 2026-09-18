// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A snapshot restore rebuilds ElementIndex for system accounts
// (internal/database/snapshot/restore.go:136-190) from the collected mark
// points. Does the rebuilt index name the position each entry occupies?
//
// This exercises CollectChain + RestoreElementIndexFrom{MarkPoints,Head}
// directly, which is what restore.go calls.
func Test4312_SnapshotMarkPointIndexArithmetic(t *testing.T) {
	const n = 600 // two mark points (256 each) plus an open set

	src := database.OpenInMemory(nil)
	b := src.Begin(true)
	defer b.Discard()

	u := protocol.DnUrl().JoinPath(protocol.Ledger)
	require.NoError(t, b.Account(u).Main().Put(&protocol.SystemLedger{Url: u}))
	sc, err := b.Account(u).MainChain().Get()
	require.NoError(t, err)

	var rh common.RandHash
	var all [][]byte
	for i := 0; i < n; i++ {
		h := rh.NextList()
		all = append(all, h)
		require.NoError(t, sc.AddEntry(h, false))
	}
	require.NoError(t, b.Commit())

	b = src.Begin(true)
	defer b.Discard()
	sc, err = b.Account(u).MainChain().Get()
	require.NoError(t, err)

	col, err := snapshot.CollectChain(b.Account(u).MainChain().Inner())
	require.NoError(t, err)
	t.Logf("collected: head.Count=%d markPoints=%d openSet=%d",
		col.Head.Count, len(col.MarkPoints), len(col.Head.HashList))
	require.NotEmpty(t, col.MarkPoints, "no mark points collected; the test proves nothing")

	// Restore into a fresh chain, exactly as restore.go does.
	dst := database.OpenInMemory(nil)
	db := dst.Begin(true)
	defer db.Discard()
	require.NoError(t, db.Account(u).Main().Put(&protocol.SystemLedger{Url: u}))
	dc := db.Account(u).MainChain().Inner()

	require.NoError(t, col.RestoreSnapshot(dc))
	require.NoError(t, col.RestoreElementIndexFromMarkPoints(dc, 0, len(col.MarkPoints)))
	require.NoError(t, col.RestoreElementIndexFromHead(dc))

	head, err := dc.Head().Get()
	require.NoError(t, err)
	t.Logf("restored head.Count=%d", head.Count)

	bad, past := 0, 0
	for i := int64(0); i < n; i++ {
		got, err := dc.IndexOf(all[i])
		if err != nil {
			continue // not restored; below the last mark point is expected to be partial
		}
		if got != i {
			bad++
			if bad <= 5 {
				t.Logf("entry %3d: restored index says %d (off by %+d)", i, got, got-i)
			}
		}
		if got >= head.Count {
			past++
			if past <= 5 {
				t.Logf("entry %3d: indexed at %d, PAST the head of %d", i, got, head.Count)
			}
		}
	}
	t.Logf("entries with a wrong restored index: %d of %d", bad, n)
	t.Logf("index records naming a position at or past the head: %d", past)

	require.Zero(t, bad, "the restored element index does not name the positions the entries occupy")
	require.Zero(t, past, "index records name positions the chain does not have")
}
