// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func BenchmarkCollect(b *testing.B) {
	// db, err := database.OpenBadger(b.TempDir(), nil)
	// require.NoError(b, err)
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()
	for i := 0; i < b.N; i++ {
		v := &ADI{Url: protocol.AccountUrl(fmt.Sprintf("a-%d", i)), AccountAuth: AccountAuth{Authorities: []AuthorityEntry{{Url: protocol.AccountUrl("foo")}}}}
		account := batch.Account(v.Url)
		require.NoError(b, account.Main().Put(v))

		txn := new(protocol.Transaction)
		txn.Header.Principal = v.Url
		binary.BigEndian.PutUint64(txn.Header.Initiator[:], uint64(i))
		txn.Body = new(SendTokens)
		err := account.MainChain().Inner().AddEntry(txn.GetHash(), false)
		require.NoError(b, err)
		require.NoError(b, batch.Message2(txn.GetHash()).Main().Put(&messaging.TransactionMessage{Transaction: txn}))
	}
	require.NoError(b, batch.Commit())

	b.ResetTimer()
	_, err := db.Collect(new(ioutil.Discard), nil, &database.CollectOptions{
		BuildIndex: true,
	})
	require.NoError(b, err)
}

func TestSnapshot(t *testing.T) {
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	dir := t.TempDir()
	sim := NewSim(t,
		simulator.BadgerDatabaseFromDirectory(dir, func(err error) { require.NoError(t, err) }),
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Execute
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			SendTokens(123, 0).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	// Collect a snapshot
	buf := new(ioutil.Buffer)
	err := sim.S.Collect("BVN0", buf, nil)
	require.NoError(t, err)

	// Restore the snapshot
	db := database.OpenInMemory(nil)
	require.NoError(t, database.Restore(db, ioutil.NewBuffer(buf.Bytes()), nil))

	// Verify
	account := GetAccount[*TokenAccount](t, db, bob.JoinPath("tokens"))
	require.Equal(t, 123, int(account.Balance.Int64()))
}

// TestSnapshotRestore creates and restores a snapshot but it restores each
// record with a separate batch. This verifies that batch splitting can safely
// be done at an arbitrary boundary.
func TestSnapshotRestore(t *testing.T) {
	// Make a snapshot
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	dir := t.TempDir()
	sim := NewSim(t,
		simulator.BadgerDatabaseFromDirectory(dir, func(err error) { require.NoError(t, err) }),
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			SendTokens(123, 0).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	// Collect a snapshot
	buf := new(ioutil.Buffer)
	err := sim.S.Collect("BVN0", buf, nil)
	require.NoError(t, err)

	// Restore the snapshot **restoring each record in a separate batch**
	db := database.OpenInMemory(nil)
	require.NoError(t, database.Restore(db, ioutil.NewBuffer(buf.Bytes()), &database.RestoreOptions{BatchRecordLimit: 1}))

	// Verify
	account := GetAccount[*TokenAccount](t, db, bob.JoinPath("tokens"))
	require.Equal(t, 123, int(account.Balance.Int64()))
}

// TestCollectAndRestore runs a network, collects snapshots, reboots the network
// from the snapshots, and verifies that things still work.
func TestCollectAndRestore(t *testing.T) {
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey1 := acctesting.GenerateKey(alice, 1)
	aliceKey2 := acctesting.GenerateKey(alice, 2)
	bobKey := acctesting.GenerateKey(bob)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey1[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeKeyPage(t, sim.DatabaseFor(alice), alice.JoinPath("book"), aliceKey1[32:], aliceKey2[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "2"), func(p *KeyPage) {
		p.AcceptThreshold = 2
		p.CreditBalance = 1e9
	})
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Execute
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			SendTokens(123, 0).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey1))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	// Start a pending transaction
	st = sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			SendTokens(123, 0).To(bob, "tokens").
			SignWith(alice, "book", "2").Version(1).Timestamp(2).PrivateKey(aliceKey1))

	sim.StepUntil(
		Txn(st.TxID).IsPending())

	// Verify the major blocks index is present
	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		part, err := sim.Router().RouteAccount(alice)
		require.NoError(t, err)
		record := batch.Account(PartitionUrl(part).JoinPath(Ledger))

		hash, err := record.Events().BPT().GetRootHash()
		require.NoError(t, err)
		require.NotZero(t, hash)

		blocks, err := record.
			Events().
			Major().
			Blocks().
			Get()
		require.NoError(t, err)
		assert.NotEmpty(t, blocks)
	})

	// Give it time to settle
	sim.StepN(50)

	// Collect snapshots
	snap := map[string][]byte{}
	for _, p := range sim.Partitions() {
		buf := new(ioutil.Buffer)
		require.NoError(t, sim.S.Collect(p.ID, buf, &database.CollectOptions{
			BuildIndex: true,
		}))
		snap[p.ID] = buf.Bytes()
	}

	// Restart the simulator
	sim = NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.SnapshotMap(snap),
	)

	// Verify the major blocks index is restored
	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		part, err := sim.Router().RouteAccount(alice)
		require.NoError(t, err)
		record := batch.Account(PartitionUrl(part).JoinPath(Ledger))

		hash, err := record.Events().BPT().GetRootHash()
		require.NoError(t, err)
		require.NotZero(t, hash)

		blocks, err := record.
			Events().
			Major().
			Blocks().
			Get()
		require.NoError(t, err)
		assert.NotEmpty(t, blocks)
	})

	// Sign the pending transaction
	st = sim.BuildAndSubmitTxnSuccessfully(
		build.SignatureForTxID(st.TxID).
			Url(alice, "book", "2").Version(1).Timestamp(1).PrivateKey(aliceKey2))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())
}

func TestPreservationOfOldTransactions(t *testing.T) {
	// Some random transaction
	env, err := build.Transaction().For("alice", "tokens").BurnTokens(1, 0).
		SignWith("alice", "book", "1").Version(1).Timestamp(1).PrivateKey(make([]byte, 64)).
		Done()
	require.NoError(t, err)

	// Store it in a database
	txn := env.Transaction[0]
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()
	require.NoError(t, batch.Transaction(txn.GetHash()).Main().Put(&database.SigOrTxn{Transaction: txn}))
	require.NoError(t, batch.Account(txn.Header.Principal).MainChain().Inner().AddEntry(txn.GetHash(), false))
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())

	// Collect a snapshot
	buf := new(ioutil.Buffer)
	_, err = db.Collect(buf, nil, nil)
	require.NoError(t, err)

	// Restore the snapshot
	db = database.OpenInMemory(nil)
	require.NoError(t, database.Restore(db, buf, nil))

	// Verify the transaction still exists
	batch = db.Begin(false)
	defer batch.Discard()
	txn2, err := batch.Transaction(txn.GetHash()).Main().Get()
	require.NoError(t, err)
	require.True(t, txn.Equal(txn2.Transaction))
}

// #4155: a #4146 local delivery lives on NO chain — that is its design — so
// nothing but the queue itself references its body. The snapshot must carry
// those bodies anyway: a restored node drains the queue at its next Begin,
// and a queue entry whose body is missing fails Begin forever, with no
// eviction path and no healing (locals are invisible to healing).
func TestSnapshot_PreservesQueuedLocalDeliveryBodies(t *testing.T) {
	synthetic := PartitionUrl("BVN0").JoinPath(Synthetic)

	txn := new(protocol.Transaction)
	txn.Header.Principal = AccountUrl("bob", "tokens")
	txn.Body = &SyntheticDepositCredits{Amount: 1}
	msg := &messaging.TransactionMessage{Transaction: txn}

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Exactly what splitLocalDeliveries writes: the body in the message
	// store, on no chain, and the queue entry pointing at it.
	require.NoError(t, batch.Message(msg.Hash()).Main().Put(msg))
	require.NoError(t, batch.Account(synthetic).LocalDeliveryQueue().
		Add(txn.Header.Principal.WithTxID(msg.Hash())))
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())

	buf := new(ioutil.Buffer)
	_, err := db.Collect(buf, nil, nil)
	require.NoError(t, err)

	db = database.OpenInMemory(nil)
	require.NoError(t, database.Restore(db, buf, nil))

	batch = db.Begin(false)
	defer batch.Discard()
	ids, err := batch.Account(synthetic).LocalDeliveryQueue().Get()
	require.NoError(t, err)
	require.Len(t, ids, 1, "the queue is account state and must survive")
	_, err = batch.Message(ids[0].Hash()).Main().Get()
	require.NoError(t, err,
		"the queued BODY must survive — without it the restored node's next Begin fails forever")
}

// Staging must survive a snapshot, because a snapshot is what a new node starts
// from (#4189).
//
// Staging decides what a block executes: a block delivers the contiguous run
// from Delivered+1 taken from this block's arrivals AND from what is already
// held. A node restored without it holds nothing, so it executes a shorter run
// than its peers on the first block where a gap closes — different Delivered,
// different account state, different BPT root. That is a divergent block hash,
// not a node briefly behind.
//
// This is why the records are account STATE and not an index: collection
// ignores indices.
func TestSnapshot_PreservesStaging(t *testing.T) {
	synthetic := PartitionUrl("BVN0").JoinPath(Synthetic)
	source := PartitionUrl("BVN1")
	id := execute.StreamID{Ledger: synthetic, Source: source}

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	ledger := new(protocol.SyntheticLedger)
	ledger.Url = synthetic
	ledger.Partition(source).Delivered = 5
	require.NoError(t, batch.Account(synthetic).Main().Put(ledger))

	// Received 7 and 9; 6 and 8 are holes.
	require.NoError(t, execute.Hold(batch, id, 7, source.WithTxID([32]byte{7})))
	require.NoError(t, execute.Hold(batch, id, 9, source.WithTxID([32]byte{9})))
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())

	buf := new(ioutil.Buffer)
	_, err := db.Collect(buf, nil, nil)
	require.NoError(t, err)

	db = database.OpenInMemory(nil)
	require.NoError(t, database.Restore(db, buf, nil))

	batch = db.Begin(false)
	defer batch.Discard()

	got, ok, err := execute.IDOf(batch, id, 7)
	require.NoError(t, err)
	require.True(t, ok, "a held message must survive the snapshot")
	require.Equal(t, source.WithTxID([32]byte{7}).String(), got.String())

	high, err := execute.Sighted(batch, id)
	require.NoError(t, err)
	require.Equal(t, uint64(9), high, "and so must the high-water mark")

	runs, err := execute.Missing(batch, id, 5, high, 8)
	require.NoError(t, err)
	require.Equal(t, [][2]uint64{{6, 6}, {8, 8}}, runs,
		"the restored node must see the same gaps its peers see")
}
