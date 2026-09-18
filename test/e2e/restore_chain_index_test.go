// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

// A snapshot restore must leave a node that executes the next block exactly as
// the node it was restored from does, and that answers the same questions about
// what it holds. Both depend on the merkle element index, which a v2 snapshot
// does not carry and which nothing rebuilt before #4328 / #4330.

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// newGenesisSim starts a network from genesis.
func newGenesisSim(t *testing.T, network simulator.Option) *Sim {
	t.Helper()
	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	return NewSim(t, network, simulator.GenesisWith(GenesisTime, globals))
}

// collectPartitions snapshots every partition with the production collect.
func collectPartitions(t *testing.T, sim *Sim) map[string][]byte {
	t.Helper()
	snapshots := map[string][]byte{}
	for _, p := range sim.Partitions() {
		buf := new(ioutil.Buffer)
		require.NoError(t, sim.S.Collect(p.ID, buf, &database.CollectOptions{BuildIndex: true}))
		snapshots[p.ID] = buf.Bytes()
	}
	return snapshots
}

// restoreSim starts a network from the given snapshots, through the production
// restore.
//
// InitialAcmeSupply(nil) is not about this defect: the simulator adds its
// initial supply to acc://ACME after init, so without it the restored network
// would add the supply a second time on top of the snapshot's and acc://ACME
// would differ for reasons that have nothing to do with the element index.
func restoreSim(t *testing.T, network simulator.Option, snapshots map[string][]byte) *Sim {
	t.Helper()
	return NewSim(t, network,
		simulator.SnapshotMap(snapshots),
		simulator.InitialAcmeSupply(nil))
}

func bptRootHash(t *testing.T, db database.Viewer) [32]byte {
	t.Helper()
	var h [32]byte
	View(t, db, func(batch *database.Batch) {
		var err error
		h, err = batch.GetBptRootHash()
		require.NoError(t, err)
	})
	return h
}

// accountsThatDiffer names the accounts whose BPT hash is not the same in both
// databases, so a failure says which account diverged rather than only that the
// root did.
func accountsThatDiffer(t *testing.T, a, b database.Viewer) []string {
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

func chainHeightOf(t *testing.T, db database.Viewer, account *url.URL, name string) int64 {
	t.Helper()
	var n int64
	View(t, db, func(batch *database.Batch) {
		c, err := batch.Account(account).ChainByName(name)
		require.NoError(t, err)
		head, err := c.Inner().Head().Get()
		require.NoError(t, err)
		n = head.Count
	})
	return n
}

// TestRestoredNodeAgreesOnTheNextBlock is the consensus half of #4328. Snapshot
// a network, restore it, execute one more block on both, and the two must hold
// the same state.
//
// They do not, without the rebuild: the executor captures the ABCI CommitInfo
// into <partition>/votes every block (block_begin.go), and that transaction's
// hash has no height, no time, no block hash and no signature in it, so in
// steady state it is byte-identical block after block. The node that built its
// chain skips the duplicate because its element index knows the hash. The
// restored node has no index, appends it, and is one entry and one BPT root
// away from its peers.
func TestRestoredNodeAgreesOnTheNextBlock(t *testing.T) {
	acctesting.EnableDebugFeatures()

	network := simulator.SimpleNetwork(t.Name(), 1, 1)
	genesis := newGenesisSim(t, network)
	genesis.StepN(20)

	votes := DnUrl().JoinPath(Votes)
	hStart := chainHeightOf(t, genesis.Database(Directory), votes, MainChain)
	require.Less(t, hStart, int64(10),
		"the genesis-built node must have deduplicated the votes chain")

	snapshots := collectPartitions(t, genesis)
	restored := restoreSim(t, network, snapshots)

	// The two are identical before either executes another block. If this ever
	// fails the test below proves nothing, so assert it.
	for _, p := range genesis.Partitions() {
		require.Emptyf(t, accountsThatDiffer(t, genesis.Database(p.ID), restored.Database(p.ID)),
			"%s must be identical at restore", p.ID)
		require.Equalf(t, bptRootHash(t, genesis.Database(p.ID)), bptRootHash(t, restored.Database(p.ID)),
			"%s BPT root must be identical at restore", p.ID)
	}

	// One more block on each
	genesis.StepN(1)
	restored.StepN(1)

	for _, p := range genesis.Partitions() {
		g, r := genesis.Database(p.ID), restored.Database(p.ID)
		differ := accountsThatDiffer(t, g, r)
		rg, rr := bptRootHash(t, g), bptRootHash(t, r)
		t.Logf("%s after one more block: BPT root genesis-built %x, restored %x; differing accounts %v",
			p.ID, rg[:8], rr[:8], differ)
		require.Emptyf(t, differ, "%s: no account may diverge", p.ID)
		require.Equalf(t, rg, rr, "%s: the BPT roots must match", p.ID)
	}

	// Name the mechanism, so a failure here is not mistaken for something else
	require.Equal(t, chainHeightOf(t, genesis.Database(Directory), votes, MainChain),
		chainHeightOf(t, restored.Database(Directory), votes, MainChain),
		"the restored node must skip the duplicate the genesis-built node skips")
}

// TestRestoredNodeFindsItsOwnAnchors is #4330. The element index is the
// existence oracle during execution: holdsAnchorRoot (msg_synthetic.go) asks it
// via IndexOf, and create_token_account.go and set_lite_account_delegate.go ask
// it via HeightOf. A restored node held the anchor on the chain and answered
// "I never received that anchor".
func TestRestoredNodeFindsItsOwnAnchors(t *testing.T) {
	acctesting.EnableDebugFeatures()

	network := simulator.SimpleNetwork(t.Name(), 1, 1)
	genesis := newGenesisSim(t, network)
	genesis.StepN(20)

	// An anchor the BVN received from the DN before the snapshot
	var bvn string
	for _, p := range genesis.Partitions() {
		if p.Type == PartitionTypeBlockValidator {
			bvn = p.ID
			break
		}
	}
	require.NotEmpty(t, bvn, "the network must have a BVN")
	anchorPool := PartitionUrl(bvn).JoinPath(AnchorPool)

	var root []byte
	var n int64
	View(t, genesis.Database(bvn), func(batch *database.Batch) {
		c, err := batch.Account(anchorPool).AnchorChain(Directory).Root().Get()
		require.NoError(t, err)
		n = c.Height()
		require.Greater(t, n, int64(0), "the BVN must have received DN anchors")
		root, err = c.Entry(0)
		require.NoError(t, err)
	})

	snapshots := collectPartitions(t, genesis)
	restored := restoreSim(t, network, snapshots)

	ask := func(db database.Viewer) (int64, error) {
		var h int64
		var err error
		View(t, db, func(batch *database.Batch) {
			c, e := batch.Account(anchorPool).AnchorChain(Directory).Root().Get()
			require.NoError(t, e)
			require.Equal(t, n, c.Height(), "same chain height")
			entry, e := c.Entry(0)
			require.NoError(t, e)
			require.Equal(t, root, entry, "same entry")
			h, err = c.HeightOf(root)
		})
		return h, err
	}

	hg, eg := ask(genesis.Database(bvn))
	hr, er := ask(restored.Database(bvn))
	t.Logf("HeightOf(anchor 0) on %s anchor(dn)/root: genesis-built %d/%v, restored %d/%v",
		bvn, hg, eg, hr, er)

	require.NoError(t, eg, "the genesis-built node finds its anchor")
	require.NoError(t, er, "the restored node must find the anchor it is holding")
	require.Equal(t, hg, hr, "and must agree with the genesis-built node about where it is")

	// Every entry, not only the first
	for i := int64(0); i < n; i++ {
		var entry []byte
		View(t, genesis.Database(bvn), func(batch *database.Batch) {
			c, err := batch.Account(anchorPool).AnchorChain(Directory).Root().Get()
			require.NoError(t, err)
			entry, err = c.Entry(i)
			require.NoError(t, err)
		})
		var hg, hr int64
		var eg, er error
		View(t, genesis.Database(bvn), func(batch *database.Batch) {
			c, _ := batch.Account(anchorPool).AnchorChain(Directory).Root().Get()
			hg, eg = c.HeightOf(entry)
		})
		View(t, restored.Database(bvn), func(batch *database.Batch) {
			c, _ := batch.Account(anchorPool).AnchorChain(Directory).Root().Get()
			hr, er = c.HeightOf(entry)
		})
		require.NoErrorf(t, eg, "genesis-built, entry %d", i)
		require.NoErrorf(t, er, "restored, entry %d", i)
		require.Equalf(t, hg, hr, "entry %d", i)
	}
}

// TestRestoredNodeBuildsTheSameReceipt is the consumer that reads the index's
// VALUE rather than its presence, and it is the reason the rebuild must name the
// FIRST occurrence of a repeated hash.
//
// indexing.getIndexedChainReceipt (receipts.go:110) does HeightOf(entry) and
// then Receipt(thatHeight, anchorIndex), where anchorIndex comes from the index
// chain entry for the block the entry was added in. A rebuild that writes the
// LAST occurrence - which is what dagbft-integration's rebuildChainIndexes does,
// correctly for DI, whose AddEntry rewrites the index on every append - names a
// height after that anchor, and Receipt fails outright with "invalid range".
//
// The repeated entry here is not contrived: <partition>/ledger's root chain
// holds the same anchor at several heights in ordinary operation.
func TestRestoredNodeBuildsTheSameReceipt(t *testing.T) {
	acctesting.EnableDebugFeatures()

	network := simulator.SimpleNetwork(t.Name(), 1, 1)
	genesis := newGenesisSim(t, network)
	genesis.StepN(50)

	ledger := DnUrl().JoinPath(Ledger)

	// Find a root chain entry that repeats, and the heights it sits at
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

	// An anchor between the two occurrences, which is the shape of what the
	// index chain names for the block the entry was first added in
	anchorIndex := (first + last) / 2
	require.Greater(t, anchorIndex, first)
	require.Less(t, anchorIndex, last)

	snapshots := collectPartitions(t, genesis)
	restored := restoreSim(t, network, snapshots)

	// What getIndexedChainReceipt does
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

	hg, ehg, erg := try(genesis.Database(Directory))
	t.Logf("GENESIS-BUILT HeightOf=%d err=%v; Receipt(%d,%d) err=%v", hg, ehg, hg, anchorIndex, erg)
	require.NoError(t, ehg)
	require.NoError(t, erg, "the genesis-built node builds the receipt")
	require.Equal(t, first, hg, "and names the first occurrence")

	hr, ehr, err := try(restored.Database(Directory))
	t.Logf("RESTORED      HeightOf=%d err=%v; Receipt(%d,%d) err=%v", hr, ehr, hr, anchorIndex, err)
	require.NoError(t, ehr, "the restored node must find the entry")
	require.Equal(t, first, hr,
		"the restored node must name the FIRST occurrence: naming the last breaks the receipt")
	require.NoError(t, err, "the restored node must build the same receipt")
}
