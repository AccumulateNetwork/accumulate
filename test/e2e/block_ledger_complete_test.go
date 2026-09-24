// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
)

// The block ledger names every account a block changes, and names none that
// holds nothing (executor spec, "The block ledger", invariants 13 and 14;
// #4437). A joining node follows the partition block by block from the
// ledger alone, so an account the ledger leaves out is an account the join
// never re-pulls; an account it names that holds nothing is a leaf no peer
// can serve.

func leafHashes(t *testing.T, db database.Viewer) map[string]string {
	t.Helper()
	m := map[string]string{}
	View(t, db, func(b *database.Batch) {
		require.NoError(t, bpt.ForEach(b.BPT(), func(k *record.Key, h []byte) error {
			if k.Len() >= 2 && k.Get(0) == "Account" {
				if u, ok := k.Get(1).(*url.URL); ok {
					m[u.String()] = string(h)
				}
			}
			return nil
		}))
	})
	return m
}

func systemLedgerIndex(t *testing.T, db database.Viewer, part string) uint64 {
	t.Helper()
	var idx uint64
	View(t, db, func(b *database.Batch) {
		var s *SystemLedger
		require.NoError(t, b.Account(PartitionUrl(part).JoinPath(Ledger)).Main().GetAs(&s))
		idx = s.Index
	})
	return idx
}

func listedBetween(t *testing.T, db database.Viewer, part string, from, to uint64) map[string]bool {
	t.Helper()
	listed := map[string]bool{}
	View(t, db, func(b *database.Batch) {
		ledger := b.Account(PartitionUrl(part).JoinPath(Ledger))
		for i := from + 1; i <= to; i++ {
			_, entries, err := indexing.LoadBlockLedger(ledger, i)
			if errors.Is(err, errors.NotFound) {
				continue
			}
			require.NoError(t, err)
			for _, e := range entries {
				listed[e.Account.String()] = true
			}
		}
	})
	return listed
}

// blockLedgerTraffic runs blocks carrying a same-partition send, a
// cross-partition send, a send to a token account that does not exist, and a
// WriteData to a principal that does not exist, and calls check after each
// step for each partition.
func blockLedgerTraffic(t *testing.T, steps int, check func(part string, db database.Viewer, before map[string]string, from, to uint64)) {
	sim, alice, key := noEmptyLeafNetwork(t)
	parts := []string{"BVN0", "BVN1", "Directory"}
	prev := map[string]map[string]string{}
	prevIdx := map[string]uint64{}
	for _, p := range parts {
		prev[p], prevIdx[p] = leafHashes(t, sim.Database(p)), systemLedgerIndex(t, sim.Database(p), p)
	}
	ts := uint64(1000)
	for step := 0; step < steps; step++ {
		for _, to := range []string{"bob/tokens", "carol/tokens", "void/tokens", "nowhere/tokens"} {
			ts++
			send(sim, alice, key, ts, url.MustParse(to))
		}
		ts++
		sim.BuildAndSubmit(build.Transaction().For(alice.JoinPath("ghostdata")).
			WriteData().Entry(&DoubleHashDataEntry{Data: [][]byte{[]byte("to a principal that does not exist")}}).
			SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(key))
		sim.StepN(1)
		for _, p := range parts {
			idx := systemLedgerIndex(t, sim.Database(p), p)
			check(p, sim.Database(p), prev[p], prevIdx[p], idx)
			prev[p], prevIdx[p] = leafHashes(t, sim.Database(p)), idx
		}
	}
}

func TestTheBlockLedgerNamesEveryAccountABlockChanges(t *testing.T) {
	blockLedgerTraffic(t, 30, func(part string, db database.Viewer, before map[string]string, from, to uint64) {
		listed := listedBetween(t, db, part, from, to)
		for u, h := range leafHashes(t, db) {
			if before[u] != h && !listed[u] {
				t.Errorf("%s blocks %d-%d: the leaf of %s changed and the block ledger does not name it", part, from+1, to, u)
			}
		}
	})
}

func TestNoBlockLedgerEntryNamesAnAccountThatHoldsNothing(t *testing.T) {
	blockLedgerTraffic(t, 20, func(part string, db database.Viewer, _ map[string]string, from, to uint64) {
		leaves := leafHashes(t, db)
		for u := range listedBetween(t, db, part, from, to) {
			View(t, db, func(b *database.Batch) {
				_, err := b.Account(url.MustParse(u)).Main().Get()
				if errors.Is(err, errors.NotFound) {
					t.Errorf("%s blocks %d-%d: the block ledger names %s, which has no main state", part, from+1, to, u)
				}
			})
			if _, ok := leaves[u]; !ok {
				t.Errorf("%s blocks %d-%d: the block ledger names %s, which has no leaf", part, from+1, to, u)
			}
		}
	})
}
