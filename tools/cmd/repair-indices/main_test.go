package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

func TestRepairIndices(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	network := simulator.SimpleNetwork(t.Name(), 1, 1)
	sim := NewSim(t,
		simulator.MemoryDatabase,
		network,
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &DataAccount{Url: alice.JoinPath("data")})

	// Execute
	tx1 := sim.SubmitSuccessfully(MustBuild(t,
		build.Transaction().For(alice, "data").
			WriteData([]byte("foo")).
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey),
	))

	tx2 := sim.SubmitSuccessfully(MustBuild(t,
		build.Transaction().For(alice, "data").
			WriteData([]byte("bar")).Scratch().
			SignWith(alice, "book", "1").Version(1).Timestamp(2).PrivateKey(aliceKey),
	))

	sim.StepUntil(
		Txn(tx1.TxID).Succeeds(),
		Txn(tx2.TxID).Succeeds())

	tx3 := sim.SubmitSuccessfully(MustBuild(t,
		build.Transaction().For(alice, "data").
			WriteData([]byte("baz")).
			SignWith(alice, "book", "1").Version(1).Timestamp(3).PrivateKey(aliceKey),
	))

	sim.StepUntil(
		Txn(tx3.TxID).Succeeds())

	// Verify
	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		data := indexing.Data(batch, alice.JoinPath("data"))
		require.Equal(t, 3, int(MustGet0(t, data.Count)))
		require.Equal(t, tx1.Result.(*WriteDataResult).EntryHash[:], MustGet1(t, data.Entry, 0))
		require.Equal(t, tx2.Result.(*WriteDataResult).EntryHash[:], MustGet1(t, data.Entry, 1))
		require.Equal(t, tx3.Result.(*WriteDataResult).EntryHash[:], MustGet1(t, data.Entry, 2))
	})

	// Save to and restore from snapshot
	logger := logging.NewTestLogger(t, "plain", acctesting.DefaultLogLevels, false)
	snapshots := map[string]ioutil2.SectionReader{}
	for _, p := range sim.Partitions() {
		View(t, sim.Database(p.ID), func(batch *database.Batch) {
			buf := new(ioutil2.Buffer)
			err := snapshot.FullCollect(batch, buf, config.NetworkUrl{URL: PartitionUrl(p.ID)}, logger, true)
			require.NoError(t, err)
			snapshots[p.ID] = buf
		})
	}

	// Renitialize
	sim = NewSim(t,
		simulator.MemoryDatabase,
		network,
		simulator.SnapshotMap(snapshots),
	)

	// Rebuild
	for _, p := range sim.Partitions() {
		Update(t, sim.Database(p.ID), func(batch *database.Batch) {
			require.NoError(t, rebuildIndices(batch, config.NetworkUrl{URL: PartitionUrl(p.ID)}))
		})
	}

	// Verify
	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		data := indexing.Data(batch, alice.JoinPath("data"))
		require.Equal(t, 3, int(MustGet0(t, data.Count)))
		require.Equal(t, tx1.Result.(*WriteDataResult).EntryHash[:], MustGet1(t, data.Entry, 0))
		require.Equal(t, tx2.Result.(*WriteDataResult).EntryHash[:], MustGet1(t, data.Entry, 1))
		require.Equal(t, tx3.Result.(*WriteDataResult).EntryHash[:], MustGet1(t, data.Entry, 2))
	})
}