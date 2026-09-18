// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

// EVIDENCE, NOT A FIX.
//
// Does production ever offer a DUPLICATE entry to a chain, i.e. is AddEntry's
// dedup branch ever taken? These tests answer yes, by execution, using only
// production code paths: the executor's own block-begin bookkeeping and the
// production snapshot collect/restore.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// height returns the height of the named chain of the given account.
func height(t *testing.T, db database.Viewer, account *url.URL, name string) int64 {
	t.Helper()
	var n int64
	View(t, db, func(batch *database.Batch) {
		c, err := batch.Account(account).ChainByName(name)
		require.NoError(t, err)
		head, err := c.Head().Get()
		require.NoError(t, err)
		n = head.Count
	})
	return n
}

// accountHash returns the account's BPT hash, i.e. its contribution to the
// state tree that every node must agree on.
func accountHash(t *testing.T, db database.Viewer, account *url.URL) [32]byte {
	t.Helper()
	var h [32]byte
	View(t, db, func(batch *database.Batch) {
		var err error
		h, err = batch.Account(account).Hash()
		require.NoError(t, err)
	})
	return h
}

// hasElementIndex reports whether the merkle element index of the named chain
// knows the given entry. This is the record that AddEntry's dedup branch reads
// and that a snapshot restore does not rebuild.
func hasElementIndex(t *testing.T, db database.Viewer, account *url.URL, name string, entry []byte) bool {
	t.Helper()
	var found bool
	View(t, db, func(batch *database.Batch) {
		c, err := batch.Account(account).ChainByName(name)
		require.NoError(t, err)
		_, err = c.Inner().ElementIndex(entry).Get()
		found = err == nil
	})
	return found
}

// TestDuplicateChainEntry_IsOfferedEveryBlock shows that the executor offers
// the same hash to <partition>/votes' main chain on every block: the chain
// stays at height 1 while dozens of blocks go by.
//
// The offer comes from (*Executor).captureValueAsDataEntry, called
// unconditionally from Begin. The transaction it builds has a constant
// principal, a constant initiator and a body that is the JSON of the ABCI
// CommitInfo - round plus the validator votes, with no height, no time and no
// block hash. In steady state that JSON is byte-identical every block, so the
// transaction hash is identical every block.
func TestDuplicateChainEntry_IsOfferedEveryBlock(t *testing.T) {
	acctesting.EnableDebugFeatures()

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.GenesisWith(GenesisTime, globals),
	)

	votes := DnUrl().JoinPath(Votes)

	sim.StepN(50)

	require.Greater(t, int(sim.S.BlockIndex(Directory)), 40,
		"the DN must have executed many blocks")

	h := height(t, sim.Database(Directory), votes, MainChain)
	t.Logf("DN block %d, %v main chain height %d",
		sim.S.BlockIndex(Directory), votes, h)

	// A handful of entries, not fifty: the distinct commit configurations
	// (genesis, the validator update, then steady state). Every other block
	// offered a hash the chain already had, and AddEntry's dedup branch
	// dropped it.
	require.Less(t, h, int64(10),
		"the votes chain must be deduplicated to a handful of entries")
}

// TestDuplicateChainEntry_RestoredNodeDiverges shows what that duplicate costs
// a node whose merkle element index is empty, which is what both the v1 and v2
// snapshot restores leave behind for everything predating the snapshot. The
// restored network appends the duplicate the genesis-built network drops, so
// the two end at different chain heights and different account hashes while
// executing the same blocks.
func TestDuplicateChainEntry_RestoredNodeDiverges(t *testing.T) {
	acctesting.EnableDebugFeatures()

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	network := simulator.SimpleNetwork(t.Name(), 1, 1)
	genesis := NewSim(t,
		network,
		simulator.GenesisWith(GenesisTime, globals),
	)

	votes := DnUrl().JoinPath(Votes)

	genesis.StepN(20)
	hStart := height(t, genesis.Database(Directory), votes, MainChain)
	require.Less(t, hStart, int64(10))

	// The one entry on the votes chain, and the fact that the genesis-built
	// node's element index knows it
	var entry []byte
	View(t, genesis.Database(Directory), func(batch *database.Batch) {
		c, err := batch.Account(votes).MainChain().Get()
		require.NoError(t, err)
		entry, err = c.Entry(hStart - 1)
		require.NoError(t, err)
	})
	require.True(t, hasElementIndex(t, genesis.Database(Directory), votes, MainChain, entry),
		"the genesis-built node indexes the entry")

	// Snapshot every partition with the production collect, including the
	// index-building option
	snapshots := map[string][]byte{}
	for _, p := range genesis.Partitions() {
		buf := new(ioutil.Buffer)
		require.NoError(t, genesis.S.Collect(p.ID, buf, &database.CollectOptions{
			BuildIndex: true,
		}))
		snapshots[p.ID] = buf.Bytes()
	}

	// Restore, with the production restore path
	restored := NewSim(t,
		network,
		simulator.SnapshotMap(snapshots),
	)

	// The restored node has the entry on the chain but does NOT have it in the
	// element index, so its dedup is disabled for that hash
	require.Equal(t, hStart, height(t, restored.Database(Directory), votes, MainChain),
		"the restored node has the same chain")
	require.False(t, hasElementIndex(t, restored.Database(Directory), votes, MainChain, entry),
		"the restored node's element index does not know the entry")

	// The two networks were identical at this point. Run both one more block.
	genesis.StepN(1)
	restored.StepN(1)

	hGenesis := height(t, genesis.Database(Directory), votes, MainChain)
	hRestored := height(t, restored.Database(Directory), votes, MainChain)
	t.Logf("votes main chain height after one more block: genesis-built %d (was %d), restored %d",
		hGenesis, hStart, hRestored)

	// The genesis-built node appended nothing: its element index caught the
	// replay
	require.Equal(t, hStart, hGenesis)

	// The restored node appended the replay its peer skipped
	require.Equal(t, hStart+1, hRestored,
		"the restored node must have appended the duplicate")

	// And it is a duplicate: the same hash now sits at two heights
	View(t, restored.Database(Directory), func(batch *database.Batch) {
		c, err := batch.Account(votes).MainChain().Get()
		require.NoError(t, err)
		a, err := c.Entry(hStart - 1)
		require.NoError(t, err)
		b, err := c.Entry(hStart)
		require.NoError(t, err)
		require.Equal(t, a, b, "the same hash at two heights")
		t.Logf("restored node has %x at heights %d and %d", a[:4], hStart-1, hStart)
	})

	// And so the account's state hash - its BPT entry, the thing every node
	// must agree on - differs
	require.NotEqual(t,
		accountHash(t, genesis.Database(Directory), votes),
		accountHash(t, restored.Database(Directory), votes),
		"the votes account hash must differ")

	// The chain anchors differ too
	var aGenesis, aRestored []byte
	View(t, genesis.Database(Directory), func(batch *database.Batch) {
		c, err := batch.Account(votes).MainChain().Get()
		require.NoError(t, err)
		aGenesis = c.Anchor()
	})
	View(t, restored.Database(Directory), func(batch *database.Batch) {
		c, err := batch.Account(votes).MainChain().Get()
		require.NoError(t, err)
		aRestored = c.Anchor()
	})
	t.Logf("votes chain anchor: genesis-built %x, restored %x", aGenesis[:8], aRestored[:8])
	assert.NotEqual(t, aGenesis, aRestored, "the chain anchors must differ")
}

// TestDuplicateChainEntry_RepairedIndexDoesNotDiverge is the mutation check for
// the test above: restore the same snapshot, put the missing element index
// records back, and the restored node stops appending. The empty index is the
// whole cause.
func TestDuplicateChainEntry_RepairedIndexDoesNotDiverge(t *testing.T) {
	acctesting.EnableDebugFeatures()

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	network := simulator.SimpleNetwork(t.Name(), 1, 1)
	genesis := NewSim(t,
		network,
		simulator.GenesisWith(GenesisTime, globals),
	)

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

	restored := NewSim(t, network, simulator.SnapshotMap(snapshots))

	// Rebuild the merkle element index of the votes chain, which is what the
	// restore does not do
	Update(t, restored.Database(Directory), func(batch *database.Batch) {
		c, err := batch.Account(votes).MainChain().Get()
		require.NoError(t, err)
		for i := int64(0); i < hStart; i++ {
			e, err := c.Entry(i)
			require.NoError(t, err)
			require.NoError(t, batch.Account(votes).MainChain().Inner().
				ElementIndex(e).Put(uint64(i)))
		}
	})

	restored.StepN(1)

	require.Equal(t, hStart, height(t, restored.Database(Directory), votes, MainChain),
		"with the index repaired the restored node appends nothing")
}

// TestDuplicateChainEntry_WriteDataOffersTwice shows the second producer: a
// WriteData to a data account offers the transaction hash to the account's
// main chain twice within one execution - once from addDataEntry.Execute
// (state_operation.go) and once from recordSuccessfulTransaction
// (transaction.go). The chain gets one entry, not two, only because of the
// dedup branch.
func TestDuplicateChainEntry_WriteDataOffersTwice(t *testing.T) {
	acctesting.EnableDebugFeatures()

	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.GenesisWith(GenesisTime, globals),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &DataAccount{Url: alice.JoinPath("data")})

	data := alice.JoinPath("data")
	before := height(t, sim.DatabaseFor(alice), data, MainChain)

	tx := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "data").
			WriteData().DoubleHash([]byte("foo")).
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))
	sim.StepUntil(Txn(tx.TxID).Succeeds())

	after := height(t, sim.DatabaseFor(alice), data, MainChain)
	t.Logf("%v main chain: %d -> %d for one WriteData", data, before, after)

	// One WriteData, one entry - despite two offers
	require.Equal(t, before+1, after)
}

// TestRestoredNode_CannotFindItsOwnAnchors is the other half of the empty
// element index. The index is not only the dedup oracle, it is the existence
// oracle: holdsAnchorRoot (msg_synthetic.go) and the proof checks in
// CreateTokenAccount and SetLiteAccountDelegate ask IndexOf/HeightOf whether an
// anchor is on the anchor chain. After a restore the answer is no for every
// anchor received before the snapshot, though the entries are right there on
// the chain.
func TestRestoredNode_CannotFindItsOwnAnchors(t *testing.T) {
	acctesting.EnableDebugFeatures()

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	network := simulator.SimpleNetwork(t.Name(), 1, 1)
	genesis := NewSim(t,
		network,
		simulator.GenesisWith(GenesisTime, globals),
	)

	genesis.StepN(20)

	// An anchor the BVN received from the DN before the snapshot
	bvn := genesis.Partitions()[1].ID
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

	snapshots := map[string][]byte{}
	for _, p := range genesis.Partitions() {
		buf := new(ioutil.Buffer)
		require.NoError(t, genesis.S.Collect(p.ID, buf, &database.CollectOptions{
			BuildIndex: true,
		}))
		snapshots[p.ID] = buf.Bytes()
	}
	restored := NewSim(t, network, simulator.SnapshotMap(snapshots))

	// Both have the entry on the chain
	var found [2]bool
	for i, db := range []database.Updater{genesis.Database(bvn), restored.Database(bvn)} {
		View(t, db, func(batch *database.Batch) {
			c, err := batch.Account(anchorPool).AnchorChain(Directory).Root().Get()
			require.NoError(t, err)
			require.Equal(t, n, c.Height(), "same chain height")
			e, err := c.Entry(0)
			require.NoError(t, err)
			require.Equal(t, root, e, "same entry")

			// But only one of them can find it
			_, err = c.HeightOf(root)
			found[i] = err == nil
		})
	}
	t.Logf("HeightOf(anchor 0) on %s anchor(dn)/root: genesis-built %v, restored %v",
		bvn, found[0], found[1])
	require.True(t, found[0], "the genesis-built node finds its anchor")
	require.False(t, found[1], "the restored node cannot find the anchor it holds")
}
