// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

// EXPERIMENT, NOT A FIX.
//
// The divergence scenario of dup_chain_entry_test.go, run once per candidate
// rebuild variant, through production Collect and production Restore. Which
// variant makes a restored node execute the next block the same way the node it
// was restored from does?

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func bptRoot(t *testing.T, db database.Viewer) [32]byte {
	t.Helper()
	var h [32]byte
	View(t, db, func(batch *database.Batch) {
		var err error
		h, err = batch.GetBptRootHash()
		require.NoError(t, err)
	})
	return h
}

// chainDuplicates reports, for every account and every chain of a partition,
// the entries that appear at more than one height.
func chainDuplicates(t *testing.T, db database.Viewer) map[string][]int64 {
	t.Helper()
	dups := map[string][]int64{}
	View(t, db, func(batch *database.Batch) {
		require.NoError(t, batch.ForEachAccount(func(account *database.Account, _ [32]byte) error {
			chains, err := account.Chains().Get()
			require.NoError(t, err)
			for _, meta := range chains {
				c, err := account.ChainByName(meta.Name)
				require.NoError(t, err)
				head, err := c.Inner().Head().Get()
				require.NoError(t, err)
				seen := map[[32]byte]int64{}
				for i := int64(0); i < head.Count; i++ {
					e, err := c.Inner().Entry(i)
					require.NoError(t, err)
					var k [32]byte
					copy(k[:], e)
					if first, ok := seen[k]; ok {
						key := account.Url().String() + "#" + meta.Name
						dups[key] = append(dups[key], first, i)
					} else {
						seen[k] = i
					}
				}
			}
			return nil
		}))
	})
	return dups
}

// TestRebuildVariant_DuplicateCensus asks whether a healthy, genesis-built node
// ever holds the same hash at two heights of one chain - because that is the
// only case in which the two rebuild variants can write different values.
func TestRebuildVariant_DuplicateCensus(t *testing.T) {
	acctesting.EnableDebugFeatures()
	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.GenesisWith(GenesisTime, globals),
	)
	sim.StepN(50)

	total := 0
	for _, p := range sim.Partitions() {
		d := chainDuplicates(t, sim.Database(p.ID))
		for k, v := range d {
			t.Logf("%s: duplicate entries at heights %v", k, v)
			total += len(v) / 2
		}
	}
	t.Logf("genesis-built network after 50 steps: %d duplicated chain entries in total", total)
}

func runRebuildVariant(t *testing.T, mode database.RebuildChainIndexModeT) {
	acctesting.EnableDebugFeatures()

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	network := simulator.SimpleNetwork(t.Name(), 1, 1)
	genesis := NewSim(t, network, simulator.GenesisWith(GenesisTime, globals))

	votes := DnUrl().JoinPath(Votes)
	genesis.StepN(20)
	hStart := height(t, genesis.Database(Directory), votes, MainChain)

	snapshots := map[string][]byte{}
	for _, p := range genesis.Partitions() {
		buf := new(ioutil.Buffer)
		require.NoError(t, genesis.S.Collect(p.ID, buf, &database.CollectOptions{
			BuildIndex: true,
		}))
		snapshots[p.ID] = buf.Bytes()
	}

	old := database.RebuildChainIndexMode
	database.RebuildChainIndexMode = mode
	restored := NewSim(t, network, simulator.SnapshotMap(snapshots))
	database.RebuildChainIndexMode = old

	// Same chain at the start
	require.Equal(t, hStart, height(t, restored.Database(Directory), votes, MainChain))

	// The accounts the two disagree about BEFORE either executes another
	// block. This is not empty on any variant: acc://ACME differs on a plain
	// restore, independently of the element index (see
	// TestRebuildVariant_WhatDiffersAtRestore). Anything BEYOND this set is
	// caused by the extra block.
	before := differingAccounts(t, genesis.Database(Directory), restored.Database(Directory))
	t.Logf("differing accounts before the extra block: %v", before)

	// One more block on each
	genesis.StepN(1)
	restored.StepN(1)

	hG := height(t, genesis.Database(Directory), votes, MainChain)
	hR := height(t, restored.Database(Directory), votes, MainChain)
	rG := bptRoot(t, genesis.Database(Directory))
	rR := bptRoot(t, restored.Database(Directory))
	after := differingAccounts(t, genesis.Database(Directory), restored.Database(Directory))

	t.Logf("votes main chain height: genesis-built %d (was %d), restored %d", hG, hStart, hR)
	t.Logf("DN BPT root: genesis-built %x, restored %x", rG[:8], rR[:8])
	t.Logf("differing accounts after the extra block: %v", after)

	newlyDiffering := []string{}
	for _, a := range after {
		found := false
		for _, b := range before {
			if a == b {
				found = true
			}
		}
		if !found {
			newlyDiffering = append(newlyDiffering, a)
		}
	}
	t.Logf("accounts that STARTED differing because of the extra block: %v", newlyDiffering)

	if mode == database.RebuildNone {
		require.Equal(t, hStart+1, hR, "NONE must append the duplicate")
		require.Equal(t, hStart, hG, "the genesis-built node must append nothing")
		require.Contains(t, newlyDiffering, votes.String(), "NONE must diverge on the votes account")
		return
	}
	require.Equal(t, hG, hR, "chain height must match")
	require.Empty(t, newlyDiffering, "the extra block must not make any account diverge")
}

// differingAccounts returns the URLs of the accounts whose BPT hash differs
// between the two databases.
func differingAccounts(t *testing.T, a, b database.Viewer) []string {
	t.Helper()
	hashes := func(db database.Viewer) map[string][32]byte {
		m := map[string][32]byte{}
		View(t, db, func(batch *database.Batch) {
			require.NoError(t, batch.ForEachAccount(func(acct *database.Account, h [32]byte) error {
				m[acct.Url().String()] = h
				return nil
			}))
		})
		return m
	}
	x, y := hashes(a), hashes(b)
	var out []string
	for k, v := range x {
		if w, ok := y[k]; !ok || v != w {
			out = append(out, k)
		}
	}
	for k := range y {
		if _, ok := x[k]; !ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// TestRebuildVariant_WhatDiffersAtRestore is a diagnostic: which accounts does
// a freshly restored simulator disagree with its source about, before any block
// is executed?
func TestRebuildVariant_WhatDiffersAtRestore(t *testing.T) {
	acctesting.EnableDebugFeatures()
	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	network := simulator.SimpleNetwork(t.Name(), 1, 1)
	genesis := NewSim(t, network, simulator.GenesisWith(GenesisTime, globals))
	genesis.StepN(20)

	snapshots := map[string][]byte{}
	for _, p := range genesis.Partitions() {
		buf := new(ioutil.Buffer)
		require.NoError(t, genesis.S.Collect(p.ID, buf, &database.CollectOptions{BuildIndex: true}))
		snapshots[p.ID] = buf.Bytes()
	}
	restored := NewSim(t, network, simulator.SnapshotMap(snapshots))

	hashes := func(db database.Viewer) map[string][32]byte {
		m := map[string][32]byte{}
		View(t, db, func(batch *database.Batch) {
			require.NoError(t, batch.ForEachAccount(func(a *database.Account, h [32]byte) error {
				m[a.Url().String()] = h
				return nil
			}))
		})
		return m
	}
	g := hashes(genesis.Database(Directory))
	r := hashes(restored.Database(Directory))
	for k, v := range g {
		w, ok := r[k]
		switch {
		case !ok:
			t.Logf("MISSING from restored: %s", k)
		case v != w:
			t.Logf("DIFFERS: %s genesis %x restored %x", k, v[:8], w[:8])
		}
	}
	for k := range r {
		if _, ok := g[k]; !ok {
			t.Logf("EXTRA in restored: %s", k)
		}
	}
	t.Logf("accounts: genesis %d, restored %d", len(g), len(r))
}

// TestRebuildVariant_ReceiptForDuplicatedEntry is the consumer that reads the
// VALUE, not the presence. indexing.getIndexedChainReceipt (receipts.go:110-117)
// does exactly this: HeightOf(entry), then Receipt(thatHeight, anchorIndex),
// where anchorIndex comes from the index chain entry for the block the entry
// was added in. If HeightOf names a LATER occurrence than that anchor, the
// receipt cannot be built at all.
//
// The duplicate is real and produced by production: <partition>/ledger's root
// chain holds the same anchor at many heights (see
// TestRebuildVariant_DuplicateCensus).
func TestRebuildVariant_ReceiptForDuplicatedEntry(t *testing.T) {
	acctesting.EnableDebugFeatures()
	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	network := simulator.SimpleNetwork(t.Name(), 1, 1)
	genesis := NewSim(t, network, simulator.GenesisWith(GenesisTime, globals))
	genesis.StepN(50)

	ledger := DnUrl().JoinPath(Ledger)

	// Find a root chain entry that repeats, and the two heights it sits at
	var entry []byte
	var first, last int64 = -1, -1
	View(t, genesis.Database(Directory), func(batch *database.Batch) {
		c, err := batch.Account(ledger).RootChain().Get()
		require.NoError(t, err)
		seen := map[[32]byte]int64{}
		for i := int64(0); i < c.Height(); i++ {
			e, err := c.Entry(i)
			require.NoError(t, err)
			var k [32]byte
			copy(k[:], e)
			if f, ok := seen[k]; ok {
				entry, first, last = e, f, i
			} else {
				seen[k] = i
			}
		}
	})
	require.NotNil(t, entry, "production must produce a repeated root chain entry")
	t.Logf("%v root chain holds %x... at heights %d and %d", ledger, entry[:4], first, last)

	// An anchor BETWEEN the two occurrences - which is what the index chain
	// names for the block the entry was first added in.
	anchorIndex := (first + last) / 2
	require.Greater(t, anchorIndex, first)
	require.Less(t, anchorIndex, last)

	snapshots := map[string][]byte{}
	for _, p := range genesis.Partitions() {
		buf := new(ioutil.Buffer)
		require.NoError(t, genesis.S.Collect(p.ID, buf, &database.CollectOptions{BuildIndex: true}))
		snapshots[p.ID] = buf.Bytes()
	}

	// What getIndexedChainReceipt does, against each node
	try := func(db database.Viewer) (int64, error, error) {
		var h int64
		var errHeight, errReceipt error
		View(t, db, func(batch *database.Batch) {
			c, err := batch.Account(ledger).RootChain().Get()
			require.NoError(t, err)
			h, errHeight = c.HeightOf(entry)
			if errHeight != nil {
				return
			}
			_, errReceipt = c.Receipt(h, anchorIndex)
		})
		return h, errHeight, errReceipt
	}

	// The live API consumer: querier.go:544 queryChainEntryByValue does
	// IndexOf(value) and reports that index, and the block the index lands in.
	// The root chain's own index chain is the map from root-chain index to
	// block.
	blockOf := func(db database.Viewer, index int64) uint64 {
		var block uint64
		View(t, db, func(batch *database.Batch) {
			idx := batch.Account(ledger).RootChain().Index()
			_, e, err := indexing.SearchIndexChain2(idx, 0, indexing.MatchAfter,
				indexing.SearchIndexChainBySource(uint64(index)))
			require.NoError(t, err)
			block = e.BlockIndex
		})
		return block
	}
	t.Logf("a query for entry %x... would report index %d / block %d (first occurrence) "+
		"or index %d / block %d (last occurrence)",
		entry[:4], first, blockOf(genesis.Database(Directory), first),
		last, blockOf(genesis.Database(Directory), last))

	h, eh, er := try(genesis.Database(Directory))
	t.Logf("GENESIS-BUILT     HeightOf=%d err=%v; Receipt(%d,%d) err=%v", h, eh, h, anchorIndex, er)
	require.NoError(t, eh)
	require.NoError(t, er, "the genesis-built node builds the receipt")
	require.Equal(t, first, h)

	for _, tc := range []struct {
		name string
		mode database.RebuildChainIndexModeT
	}{
		{"NONE", database.RebuildNone},
		{"VERBATIM", database.RebuildVerbatim},
		{"FIRST-OCCURRENCE", database.RebuildFirstOccurrence},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := database.RebuildChainIndexMode
			database.RebuildChainIndexMode = tc.mode
			restored := NewSim(t, network, simulator.SnapshotMap(snapshots))
			database.RebuildChainIndexMode = old

			h, eh, er := try(restored.Database(Directory))
			t.Logf("RESTORED %-17s HeightOf=%d err=%v; Receipt(%d,%d) err=%v", tc.name, h, eh, h, anchorIndex, er)

			switch tc.mode {
			case database.RebuildNone:
				require.Error(t, eh, "NONE cannot find the entry at all")
			case database.RebuildVerbatim:
				require.NoError(t, eh)
				require.Equal(t, last, h, "VERBATIM names the LAST occurrence")
				require.Error(t, er, "VERBATIM cannot build the receipt: start is after the anchor")
			case database.RebuildFirstOccurrence:
				require.NoError(t, eh)
				require.Equal(t, first, h, "FIRST-OCCURRENCE agrees with the genesis-built node")
				require.NoError(t, er, "FIRST-OCCURRENCE builds the same receipt")
			}
		})
	}
}

func TestRebuildVariant_None(t *testing.T) {
	runRebuildVariant(t, database.RebuildNone)
}

func TestRebuildVariant_Verbatim(t *testing.T) {
	runRebuildVariant(t, database.RebuildVerbatim)
}

func TestRebuildVariant_FirstOccurrence(t *testing.T) {
	runRebuildVariant(t, database.RebuildFirstOccurrence)
}
