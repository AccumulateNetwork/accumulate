// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	v2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestBadOperatorPageUpdate(t *testing.T) {
	// Tests AC-3238

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
	)

	before := GetAccount[*KeyPage](t, sim.Database(Directory), DnUrl().JoinPath(Operators, "1"))

	// Execute
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(DnUrl(), Operators, "1").
			UpdateKeyPage().Add().Entry().Hash([32]byte{1}).FinishEntry().FinishOperation().
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0)).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(2).Signer(sim.SignWithNode(Directory, 1)).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(3).Signer(sim.SignWithNode(Directory, 2)))

	sim.StepUntil(
		Txn(st.TxID).Fails())

	// Verify the page did not change
	after := GetAccount[*KeyPage](t, sim.Database(Directory), DnUrl().JoinPath(Operators, "1"))
	require.Equal(t, before.AcceptThreshold, after.AcceptThreshold)
	require.Equal(t, len(before.Keys), len(after.Keys))
}

func TestBadOracleUpdate(t *testing.T) {
	// Tests AC-3238

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
	)

	before := GetAccount[*DataAccount](t, sim.Database(Directory), DnUrl().JoinPath(Oracle))
	v := new(AcmeOracle)
	require.NoError(t, v.UnmarshalBinary(before.Entry.GetData()[0]))

	// Execute
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(DnUrl(), Oracle).
			WriteData().DoubleHash([]byte("foo")).ToState().
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0)).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(2).Signer(sim.SignWithNode(Directory, 1)).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(3).Signer(sim.SignWithNode(Directory, 2)))

	sim.StepUntil(
		Txn(st.TxID).Fails())

	// Verify the entry did not change
	after := GetAccount[*DataAccount](t, sim.Database(Directory), DnUrl().JoinPath(Oracle))
	v = new(AcmeOracle)
	require.NoError(t, v.UnmarshalBinary(after.Entry.GetData()[0]))
	require.True(t, before.Equal(after))
}

func TestDirectlyQueryReceiptSignature(t *testing.T) {
	// Tests AC-3254

	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// Use version 1 because version 2 eliminates receipt signatures
	g := new(core.GlobalValues)
	g.ExecutorVersion = ExecutorVersionV1

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, g),
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

	// Verify the receipt signature can be queried directly
	var synthHash *url.TxID
	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		h := st.TxID.Hash()
		p, err := batch.Transaction(h[:]).Produced().Get()
		require.NoError(t, err)
		require.Len(t, p, 1)
		synthHash = p[0]
	})

	var receiptHash *[32]byte
	View(t, sim.DatabaseFor(bob), func(batch *database.Batch) {
		h := synthHash.Hash()
		sigs, err := batch.Transaction(h[:]).ReadSignatures(DnUrl().JoinPath(Network))
		require.NoError(t, err)
		for _, entry := range sigs.Entries() {
			entry := entry
			s, err := batch.Transaction(entry.SignatureHash[:]).Main().Get()
			require.NoError(t, err)
			_, ok := s.Signature.(*ReceiptSignature)
			if ok {
				receiptHash = &entry.SignatureHash
			}
		}
		require.NotNil(t, receiptHash)
	})

	req := new(v2.GeneralQuery)
	req.Url = bob.WithTxID(*receiptHash).AsUrl()
	resp := new(v2.TransactionQueryResponse)
	part, err := sim.Router().RouteAccount(bob)
	require.NoError(t, err)
	err = sim.Router().RequestAPIv2(context.Background(), part, "query", req, resp)
	require.NoError(t, err)
}

func TestSendDirectToWrongPartition(t *testing.T) {
	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
	)

	// Create the lite addresses and one account
	aliceKey, bobKey := acctesting.GenerateKey("alice"), acctesting.GenerateKey("bob")
	alice, bob := acctesting.AcmeLiteAddressStdPriv(aliceKey), acctesting.AcmeLiteAddressStdPriv(bobKey)

	Update(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, tmed25519.PrivKey(aliceKey), 1e6, 1e9))
	})

	goodBvn, err := sim.Router().RouteAccount(alice)
	require.NoError(t, err)

	// Set route to something else
	var badBvn string
	for _, partition := range sim.Partitions() {
		if partition.Type != PartitionTypeDirectory && !strings.EqualFold(partition.ID, goodBvn) {
			badBvn = partition.ID
			break
		}
	}

	// Create the transaction
	env :=
		MustBuild(t, build.Transaction().
			For(alice).
			Body(&SendTokens{
				To: []*TokenRecipient{{
					Url:    bob,
					Amount: *big.NewInt(1),
				}},
			}).
			SignWith(alice).Version(1).Timestamp(1).PrivateKey(aliceKey))

	// Submit the transaction directly to the wrong BVN
	st, err := sim.SubmitTo(badBvn, env)
	require.NoError(t, err)
	require.EqualError(t, st[1].AsError(), fmt.Sprintf("signature submitted to %s instead of %s", badBvn, goodBvn))
}

func TestAnchoring(t *testing.T) {
	// Verifies that the solution to #3149 doesn't create duplicate entries.
	// Expect one entry per block regardless of how many transactions were
	// added.

	var timestamp uint64
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &DataAccount{Url: alice.JoinPath("data")})

	// Execute 1
	st := sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(alice, "data").
			WriteData().DoubleHash([]byte("foo")).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	st = sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(alice, "data").
			WriteData().DoubleHash([]byte("bar")).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// Execute 2
	st = sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(alice, "data").
			WriteData().DoubleHash([]byte("baz")).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	st = sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(alice, "data").
			WriteData().DoubleHash([]byte("bat")).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// Verify that the latest block has a single entry for alice.acme/data#chain/main, and that entry has index = 3
	ledger := GetAccount[*SystemLedger](t, sim.DatabaseFor(alice), PartitionUrl("BVN0").JoinPath(Ledger))
	block := GetAccount[*BlockLedger](t, sim.DatabaseFor(alice), PartitionUrl("BVN0").JoinPath(Ledger, fmt.Sprint(ledger.Index)))

	var entries []uint64
	for _, e := range block.Entries {
		if alice.JoinPath("data").Equal(e.Account) && e.Chain == "main" {
			entries = append(entries, e.Index)
		}
	}
	require.Equal(t, []uint64{3}, entries)
}

func TestSignatureChainAnchoring(t *testing.T) {
	// Tests #3149

	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Start with executor version 0
	values := new(core.GlobalValues)

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.GenesisWith(GenesisTime, values),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

	// Execute
	st := sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(alice).
			CreateDataAccount(alice, "foo").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey)))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	includesChain := func(block *BlockLedger, account *url.URL, name string) bool {
		for _, entry := range block.Entries {
			if entry.Account.Equal(account) && strings.EqualFold(name, entry.Chain) {
				return true
			}
		}
		return false
	}

	// Verify that the buggy behavior is retained
	alicePage := alice.JoinPath("book", "1")
	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		var ledger *SystemLedger
		require.NoError(t, batch.Account(PartitionUrl("BVN0").JoinPath(Ledger)).Main().GetAs(&ledger))

		var block *BlockLedger
		require.NoError(t, batch.Account(PartitionUrl("BVN0").JoinPath(Ledger, strconv.FormatUint(ledger.Index, 10))).Main().GetAs(&block))

		require.False(t, includesChain(block, alicePage, "signature"), "%v#chain/signature was anchored", alicePage)
		c, err := batch.Account(alicePage).SignatureChain().Index().Get()
		require.NoError(t, err)
		require.Zero(t, c.Height(), "%v#chain/signature was indexed", alicePage)
		_, _, _, err = indexing.ReceiptForChainIndex(config.NetworkUrl{URL: PartitionUrl("BVN0")}, batch, batch.Account(alicePage).SignatureChain(), 0)
		require.EqualError(t, err, "cannot create receipt for entry 0 of signature chain: index chain is empty")
	})

	// Activate the new behavior
	st = sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(DnUrl()).
			ActivateProtocolVersion(ExecutorVersionV1SignatureAnchoring).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0))))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// Give the anchor a few blocks to propagate
	sim.StepN(10)

	// Execute
	st = sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(alice).
			CreateDataAccount(alice, "bar").
			SignWith(alice, "book", "1").Version(1).Timestamp(2).PrivateKey(aliceKey)))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// Verify the new behavior
	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		var ledger *SystemLedger
		require.NoError(t, batch.Account(PartitionUrl("BVN0").JoinPath(Ledger)).Main().GetAs(&ledger))

		var block *BlockLedger
		require.NoError(t, batch.Account(PartitionUrl("BVN0").JoinPath(Ledger, strconv.FormatUint(ledger.Index, 10))).Main().GetAs(&block))

		require.True(t, includesChain(block, alicePage, "signature"), "%v#chain/signature was not anchored", alicePage)
		c, err := batch.Account(alicePage).SignatureChain().Index().Get()
		require.NoError(t, err)
		require.NotZero(t, c.Height(), "%v#chain/signature was not indexed", alicePage)
		_, _, _, err = indexing.ReceiptForChainIndex(config.NetworkUrl{URL: PartitionUrl("BVN0")}, batch, batch.Account(alicePage).SignatureChain(), 0)
		require.NoError(t, err)
	})

	// Verify that the latest version can be re-activated
	st = sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(DnUrl()).
			ActivateProtocolVersion(ExecutorVersionV1SignatureAnchoring).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(2).Signer(sim.SignWithNode(Directory, 0))))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())
}

func TestProtocolVersionReactivation(t *testing.T) {
	values := new(core.GlobalValues)
	values.ExecutorVersion = ExecutorVersionLatest

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.GenesisWith(GenesisTime, values),
	)

	// Reactivate the current version
	st := sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(DnUrl()).
			ActivateProtocolVersion(values.ExecutorVersion).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0))))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())
}

func TestUpdateKeyWithDelegate(t *testing.T) {
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	// On different BVNs
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e9)

	UpdateAccount(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), func(p *KeyPage) { p.Keys = nil; p.AddKeySpec(&KeySpec{Delegate: alice.JoinPath("book")}) })

	st := sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(bob, "book", "1").
			UpdateKey(aliceKey, SignatureTypeED25519).
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey)))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	hash := sha256.Sum256(aliceKey[32:])
	p := GetAccount[*KeyPage](t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"))
	require.Len(t, p.Keys, 1)
	require.Equal(t, hash[:], p.Keys[0].PublicKeyHash)
	require.True(t, p.Keys[0].Delegate.Equal(alice.JoinPath("book")))
}

func TestRemoteAuthorityInitiator(t *testing.T) {
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	charlie := AccountUrl("charlie")
	aliceKey1 := acctesting.GenerateKey(alice, 1)
	aliceKey2 := acctesting.GenerateKey(alice, 2)
	bobKey := acctesting.GenerateKey(bob)
	charlieKey := acctesting.GenerateKey(charlie)

	setup := func(t *testing.T, v ExecutorVersion) (*Sim, *messaging.Envelope) {
		// Initialize with V1+sig
		sim := NewSim(t,
			simulator.MemoryDatabase,
			simulator.SimpleNetwork(t.Name(), 3, 1),
			simulator.GenesisWith(GenesisTime, &core.GlobalValues{ExecutorVersion: v}),
		)

		// The account (charlie) and authority (bob) are on one partition and the
		// delegate (alice) is on another
		sim.SetRoute(alice, "BVN0")
		sim.SetRoute(bob, "BVN1")
		sim.SetRoute(charlie, "BVN1")

		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey1[32:])
		UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) {
			p.CreditBalance = 1e9
			hash := sha256.Sum256(aliceKey2[32:])
			p.AddKeySpec(&KeySpec{PublicKeyHash: hash[:]})
		})
		MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
		UpdateAccount(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), func(p *KeyPage) {
			p.CreditBalance = 1e9
			p.AddKeySpec(&KeySpec{Delegate: alice.JoinPath("book")})
			p.AcceptThreshold = 2
		})
		MakeIdentity(t, sim.DatabaseFor(charlie), charlie, charlieKey[32:])
		CreditCredits(t, sim.DatabaseFor(charlie), charlie.JoinPath("book", "1"), 1e9)
		MakeAccount(t, sim.DatabaseFor(charlie), &TokenAccount{Url: charlie.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(1), AccountAuth: AccountAuth{Authorities: []AuthorityEntry{{Url: bob.JoinPath("book")}}}})

		// Initiate but do not submit the transaction with alice
		delivery := MustBuild(t,
			build.Transaction().For(charlie, "tokens").
				SendTokens(1, 0).To("foo").
				SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey1).Delegator(bob, "book", "1"))

		return sim, delivery
	}

	outOfOrder := func(sim *Sim, delivery *messaging.Envelope) *TransactionStatus {
		// Sign and submit the transaction with bob
		st := sim.SubmitTxnSuccessfully(MustBuild(t,
			build.SignatureForTransaction(delivery.Transaction[0]).
				Url(bob, "book", "1").Version(1).Timestamp(1).PrivateKey(bobKey)))

		sim.StepUntil(
			Txn(st.TxID).Received())

		// Submit alice's signature
		sim.SubmitTxnSuccessfully(delivery)

		return st
	}

	extraSig := func(sim *Sim, delivery *messaging.Envelope) *TransactionStatus {
		// Submit alice's signature
		st := sim.SubmitTxnSuccessfully(delivery)

		sim.StepUntil(
			Txn(st.TxID).Received())

		// Submit with alice's other key
		sim.SubmitTxnSuccessfully(MustBuild(t,
			build.SignatureForTransaction(delivery.Transaction[0]).
				Url(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey2).Delegator(bob, "book", "1")))

		sim.StepN(50)

		// Sign and submit the transaction with bob
		sim.SubmitTxnSuccessfully(MustBuild(t,
			build.SignatureForTransaction(delivery.Transaction[0]).
				Url(bob, "book", "1").Version(1).Timestamp(1).PrivateKey(bobKey)))

		return st
	}

	captureFwd := func(sim *Sim) func() *url.TxID {
		var sigId *url.TxID
		sim.SetSubmitHook("BVN1", func(messages []messaging.Message) (dropTx bool, keepHook bool) {
			for _, msg := range messages {
				msg, ok := msg.(*messaging.TransactionMessage)
				if !ok {
					continue
				}
				fwd, ok := msg.Transaction.Body.(*SyntheticForwardTransaction)
				if !ok || len(fwd.Signatures) != 1 || !fwd.Signatures[0].Destination.Equal(charlie.JoinPath("tokens")) {
					continue
				}
				sig := fwd.Signatures[0]
				sigId = sig.Destination.WithTxID(*(*[32]byte)(sig.Signature.Hash()))
				return false, false
			}
			return false, true
		})
		return func() *url.TxID {
			for sigId == nil {
				sim.Step()
			}
			return sigId
		}
	}

	fwdFails := func(sim *Sim, sigId *url.TxID, errstr string) {
		sim.StepUntil(
			Txn(sigId).Fails())
		msg := sim.QuerySignature(sigId, nil)
		require.EqualError(sim.TB, msg.AsError(), errstr)
	}

	// Broken in V1
	t.Run("V1", func(t *testing.T) {
		t.Run("Out of order", func(t *testing.T) {
			sim, delivery := setup(t, ExecutorVersionV1)
			waitForFwd := captureFwd(sim)

			outOfOrder(sim, delivery)

			// Fails
			fwdFails(sim, waitForFwd(), "initiator is already set and does not match the signature")
		})

		t.Run("Extra signature", func(t *testing.T) {
			sim, delivery := setup(t, ExecutorVersionV1)
			waitForFwd := captureFwd(sim)

			extraSig(sim, delivery)

			// Fails
			fwdFails(sim, waitForFwd(), "initiator is already set and does not match the signature")
		})
	})

	// Fixed in V1+sig
	t.Run("V1+sig", func(t *testing.T) {
		t.Run("Out of order", func(t *testing.T) {
			sim, delivery := setup(t, ExecutorVersionV1SignatureAnchoring)

			st := outOfOrder(sim, delivery)

			// Succeeds
			sim.StepUntil(
				Txn(st.TxID).Succeeds())
		})

		t.Run("Extra signature", func(t *testing.T) {
			sim, delivery := setup(t, ExecutorVersionV1SignatureAnchoring)

			st := extraSig(sim, delivery)

			// Succeeds
			sim.StepUntil(
				Txn(st.TxID).Succeeds())
		})
	})
}

func TestSignerOverwritten(t *testing.T) {
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// Initialize with V1+sig
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWith(GenesisTime, &core.GlobalValues{ExecutorVersion: ExecutorVersionV1SignatureAnchoring}),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) {
		p.CreditBalance = 1e9
		p.AddKeySpec(&KeySpec{Delegate: alice.JoinPath("book")})
		p.AcceptThreshold = 2
	})
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(1), AccountAuth: AccountAuth{Authorities: []AuthorityEntry{{Url: alice.JoinPath("book")}}}})

	st := sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(bob, "tokens").
			SendTokens(1, 0).To("foo").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey)))

	h := st.TxID.Hash()
	st = new(TransactionStatus)
	for i := 0; st.Code == 0 && i < 50; i++ {
		sim.Step()

		View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
			var err error
			st, err = batch.Transaction(h[:]).Status().Get()
			require.NoError(t, err)
		})
	}
	if st.Code == 0 {
		t.Fatal("Condition not met after 50 blocks")
	}

	// Ensure Alice is added to the signers
	require.Len(t, st.Signers, 1)
	require.Equal(t, "alice.acme/book/1", st.Signers[0].GetUrl().ShortString())
}

func TestMissingPrincipal(t *testing.T) {
	liteKey := acctesting.GenerateKey()
	liteUrl := acctesting.AcmeLiteAddressStdPriv(liteKey)

	// Initialize with V1+sig
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeLiteTokenAccount(t, sim.DatabaseFor(liteUrl), liteKey[32:], AcmeUrl())

	txn := new(Transaction)
	txn.Body = new(SendTokens)
	sig, err := new(signing.Builder).
		SetUrl(liteUrl).
		SetVersion(1).
		SetTimestamp(1).
		SetPrivateKey(liteKey).
		Initiate(txn)
	require.NoError(t, err)

	st := sim.SubmitTxn(&messaging.Envelope{Transaction: []*Transaction{txn}, Signatures: []Signature{sig}})
	require.EqualError(t, st.AsError(), "missing principal")
}

// TestOldExec runs a basic simulator test with the V1 executor to ensure that
// everything is copacetic. This was motivated by a change to
// [messaging.Envelope.Normalize] that caused problems.
func TestOldExec(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	g := new(core.GlobalValues)
	g.ExecutorVersion = ExecutorVersionV1
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, g),
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

	// Verify
	account := GetAccount[*TokenAccount](t, sim.DatabaseFor(bob), bob.JoinPath("tokens"))
	require.Equal(t, 123, int(account.Balance.Int64()))
}

func TestBadGlobalErrorMessage(t *testing.T) {
	var timestamp uint64

	// Initialize
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.OperatorAcceptThreshold.Set(1, 100) // Use a small number so M = 1
	g.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, g),
	)

	// Update
	ns := sim.NetworkStatus(v3.NetworkStatusOptions{Partition: Directory})
	g = &core.GlobalValues{Oracle: ns.Oracle}
	g.Oracle.Price = 123456

	// Construct a transaction but DO NOT write to state
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(DnUrl(), Oracle).
			WriteData(&DoubleHashDataEntry{Data: g.FormatOracle().GetData()}).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(&timestamp).Signer(sim.SignWithNode(Directory, 1)))

	sim.StepUntil(
		Txn(st.TxID).Capture(&st).Fails().
			WithError(errors.BadRequest).
			WithMessage("updates to acc://dn.acme/oracle must write to state"))
}

// TestDifferentValidatorSignaturesV1 shows that, with executor v1, differences
// in which validators sign an anchor are not reflected in the BPT. They are
// reflected in the transaction results, which causes a consensus failure
// (manually disabled here), but they really should be reflected in the BPT.
func TestDifferentValidatorSignaturesV1(t *testing.T) {
	// This test requires that the simulator's dispatch of transactions is
	// extremely predictable. That appears to no longer be the case so this test
	// won't work until that has been fixed.
	t.Skip("https://gitlab.com/accumulatenetwork/accumulate/-/issues/3322")

	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)

	g := new(core.GlobalValues)
	g.ExecutorVersion = ExecutorVersionV1
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.GenesisWith(GenesisTime, g),
	)
	sim.S.IgnoreDeliverResults = true

	sim.StepN(10)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))

	// Drop one anchor signature, but a different signature on each node
	sim.SetNodeBlockHook(Directory, func(i int, b execute.BlockParams, envelopes []*messaging.Envelope) (_ []*messaging.Envelope, keepHook bool) {
		var other, anchors []*messaging.Envelope
		for _, env := range envelopes {
			if env.Transaction[0].Body.Type().IsAnchor() {
				require.Len(t, env.Transaction, 1)
				require.Len(t, env.Signatures, 2)
				require.Implements(t, (*protocol.KeySignature)(nil), env.Signatures[1])
				anchors = append(anchors, env)
			} else {
				other = append(other, env)
			}
		}
		if len(anchors) > 0 {
			print("")
		}
		sort.Slice(anchors, func(i, j int) bool {
			a, b := anchors[i].Signatures[1].(protocol.KeySignature), anchors[j].Signatures[1].(protocol.KeySignature)
			return bytes.Compare(a.GetPublicKey(), b.GetPublicKey()) < 0
		})
		for j, anchor := range anchors {
			if i != j {
				other = append(other, anchor)
			}
		}
		return other, true
	})

	// Execute something
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			BurnTokens(1, 0).
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())
}

func TestDifferentValidatorSignaturesV2(t *testing.T) {
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)

	g := new(core.GlobalValues)
	g.ExecutorVersion = ExecutorVersionV2
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.GenesisWith(GenesisTime, g),
	)
	sim.S.IgnoreDeliverResults = true

	sim.StepN(10)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))

	// Drop one anchor signature, but a different signature on each node
	sim.SetNodeBlockHook(Directory, func(i int, _ execute.BlockParams, envelopes []*messaging.Envelope) (_ []*messaging.Envelope, keepHook bool) {
		var other, anchors []*messaging.Envelope
		for _, env := range envelopes {
			if len(env.Messages) != 1 {
				other = append(other, env)
				continue
			}

			if _, ok := env.Messages[0].(*messaging.BlockAnchor); ok {
				anchors = append(anchors, env)
			} else {
				other = append(other, env)
			}
		}
		sort.Slice(anchors, func(i, j int) bool {
			a, b := anchors[i].Messages[0].(*messaging.BlockAnchor), anchors[j].Messages[0].(*messaging.BlockAnchor)
			return bytes.Compare(a.Signature.GetPublicKey(), b.Signature.GetPublicKey()) < 0
		})
		for j, anchor := range anchors {
			if i != j {
				other = append(other, anchor)
			}
		}
		return other, true
	})

	// Execute something
	sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			BurnTokens(1, 0).
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	var err error
	for i := 0; i < 50 && err == nil; i++ {
		err = sim.S.Step()
	}
	require.Error(t, err, "Expected consensus failure within 50 blocks")
	require.IsType(t, (simulator.CommitConsensusError)(nil), err)
}

func TestMessageCompat(t *testing.T) {
	// https://gitlab.com/accumulatenetwork/accumulate/-/work_items/3312

	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize with executor v1
	g := new(core.GlobalValues)
	g.ExecutorVersion = ExecutorVersionV1
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.GenesisWith(GenesisTime, g),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))

	// Settle
	sim.StepN(10)

	// Load the system ledger
	ledger1 := GetAccount[*SystemLedger](t, sim.Database(Directory), DnUrl().JoinPath(Ledger))

	// Construct an envelope
	env := MustBuild(t,
		build.Transaction().For(alice, "tokens").
			BurnTokens(1, 0).
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	// Submit as messages
	msgs, err := env.Normalize()
	require.NoError(t, err)
	st := sim.Submit(&messaging.Envelope{Messages: msgs})

	// Verify that the messages field is ignored and no messages are processed
	// (by verifying no blocks have been recorded)
	require.Len(t, st, 0)
	sim.StepN(10)
	ledger2 := GetAccount[*SystemLedger](t, sim.Database(Directory), DnUrl().JoinPath(Ledger))
	require.Equal(t, ledger1.Index, ledger2.Index)
}

func TestProofOverride(t *testing.T) {
	// https://gitlab.com/accumulatenetwork/accumulate/-/issues/3351

	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize with executor v1
	g := new(core.GlobalValues)
	g.ExecutorVersion = ExecutorVersionV1
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.GenesisWith(GenesisTime, g),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))

	// Trigger a synthetic transaction
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			BurnTokens(1, 0).
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	// Replace the proof with garbage
	var synth *messaging.Envelope
	var synthId *url.TxID
	h123 := [32]byte{1, 2, 3}
	sim.SetBlockHookFor(protocol.AcmeUrl(), func(block execute.BlockParams, envelopes []*messaging.Envelope) (_ []*messaging.Envelope, keepHook bool) {
		for _, env := range envelopes {
			require.Len(t, env.Transaction, 1)
			if env.Transaction[0].Body.Type() != TransactionTypeSyntheticBurnTokens {
				continue
			}

			for _, sig := range env.Signatures {
				sig, ok := sig.(*ReceiptSignature)
				if !ok || !sig.SourceNetwork.Equal(DnUrl()) {
					continue
				}

				synth = env.Copy()
				synthId = env.Transaction[0].ID()

				anchor := sha256.Sum256(append(h123[:], sig.Proof.Start...))
				sig.Proof.End = nil
				sig.Proof.EndIndex = 0
				sig.Proof.Anchor = anchor[:]
				sig.Proof.Entries = []*merkle.ReceiptEntry{{Hash: h123[:]}}
				return envelopes, false
			}
		}
		return envelopes, true
	})

	// Wait for the synthetic transaction to be sent
	sim.StepUntil(True(func(h *Harness) bool { return synth != nil }))

	// Verify the synthetic transaction is pending and the proof is garbage
	View(t, sim.Database(Directory), func(batch *database.Batch) {
		status, err := batch.Transaction(synthId.HashSlice()).Status().Get()
		require.NoError(t, err)
		require.Equal(t, errors.Pending.String(), status.Code.String())

		require.NotNil(t, status.Proof)
		require.NotEmpty(t, status.Proof.Entries)
		require.Equal(t, h123[:], status.Proof.Entries[len(status.Proof.Entries)-1].Hash)
	})

	// Construct a valid proof
	h := synthId.Hash()
	r1 := sim.QueryChainEntry(PartitionUrl("bvn0").JoinPath(Synthetic), &v3.ChainQuery{Name: "main", Entry: h[:], IncludeReceipt: true})
	require.NotNil(t, r1.Receipt)
	r2 := sim.SearchForAnchor(DnUrl().JoinPath(AnchorPool), &v3.AnchorSearchQuery{Anchor: r1.Receipt.Anchor, IncludeReceipt: true})
	require.NotEmpty(t, r2.Records)
	require.NotEmpty(t, r2.Records[0].Receipt)

	receipt, err := r1.Receipt.Combine(&r2.Records[0].Receipt.Receipt)
	require.NoError(t, err)

	// Replace the proofs
	var found bool
	sigs := synth.Signatures
	synth.Signatures = nil
	for _, sig := range sigs {
		rsig, ok := sig.(*ReceiptSignature)
		if !ok {
			synth.Signatures = append(synth.Signatures, sig)
			continue
		}
		if !rsig.SourceNetwork.Equal(DnUrl()) {
			continue
		}

		require.False(t, found)
		found = true
		rsig.Proof = *receipt
		synth.Signatures = append(synth.Signatures, rsig)
	}
	require.True(t, found)

	// Submit with the fixed proof
	sts, err := sim.SubmitTo(Directory, synth)
	require.NoError(t, err)
	for _, st := range sts {
		require.NoError(t, st.AsError())
	}

	// Verify the burn completes
	sim.StepUntil(Txn(st.TxID).Completes())
	sim.StepUntil(Txn(synthId).Succeeds())
}

// TestChainUpdateAnchor verifies that any chain update triggers an anchor.
//
// https://gitlab.com/accumulatenetwork/accumulate/-/issues/3370
func TestChainUpdateAnchor(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	bob := AccountUrl("bob")
	bobKey := acctesting.GenerateKey(bob)

	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) {
		// Set the threshold to 2 and add an empty key spec (since we're not
		// going to use it)
		p.AcceptThreshold = 2
		p.AddKeySpec(&KeySpec{})
	})
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens")})

	// Do something that produces a credit payment, but don't actually complete
	// the transaction. Alice is not an authority of Bob's account, but that
	// doesn't matter since our goal is just to send the credit payment. Also
	// the test is invalidated if the initial signature request produces a
	// synthetic message, since that produced message will cause the block to be
	// anchored.

	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(bob, "tokens").
			BurnTokens(1, 0).
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))[1]

	// Step until the signature is processed, but the produced message have not
	// been processed
	sim.StepUntil(
		Sig(st.TxID).Succeeds())

	// Get the list of chains modified, but only for the last block that is
	// executed
	var chains []string
	events.SubscribeSync(sim.S.EventBus("BVN0"), func(e execute.WillCommitBlock) error {
		chains = nil
		return e.Block.ChangeSet().Walk(record.WalkOptions{
			Modified:      true,
			IgnoreIndices: true,
		}, func(r record.Record) (skip bool, err error) {
			if chain, ok := r.(*database.MerkleManager); ok && chain.Type() != ChainTypeIndex {
				chains = append(chains, chain.Key().String())
			}
			return false, nil
		})
	})

	// Wait for the credit payment
	sim.StepUntil(
		Sig(st.TxID).CreditPayment().Capture(&st).Completes())

	h := st.TxID.Hash()
	r1 := sim.QueryChainEntry(bob.JoinPath("tokens"), &v3.ChainQuery{
		Name:           "signature",
		Entry:          h[:],
		IncludeReceipt: true,
	})

	// Verify the credit payment was processed in the last block, otherwise the
	// subsequent checks are invalid
	ledger := GetAccount[*SystemLedger](t, sim.DatabaseFor(bob), PartitionUrl("BVN0").JoinPath(Ledger))
	require.Equal(t, ledger.Index, r1.Receipt.LocalBlock)

	// Verify that nothing happened except the credit payment and signature
	// request, and verify the second signature request produced by the initial
	// signature request was executed locally (i.e. did not produce a synthetic
	// message). This is fragile but necessary to ensure the validity of this
	// test.
	require.ElementsMatch(t, []string{
		// The credit payment and initial signature request are added to the
		// principal's signature chain
		"Account.acc://bob.acme/tokens.SignatureChain",

		// The secondary signature request is added to the key book's signature
		// chain
		"Account.acc://bob.acme/book.SignatureChain",

		// Those chains are anchored into the root chain
		"Account.acc://bvn-BVN0.acme/ledger.RootChain",

		// No other chains are modified
	}, chains)

	// Verify an anchor was produced
	require.NotNil(t, ledger.Anchor)

	// Execute another block, since anchors are sent in the next block
	sim.Step()

	var count uint64 = 1
	var expand = true
	r2 := sim.QueryMainChainEntries(PartitionUrl("BVN0").JoinPath(AnchorPool), &v3.ChainQuery{Name: "anchor-sequence", Range: &v3.RangeOptions{FromEnd: true, Count: &count, Start: 0, Expand: &expand}})
	require.Len(t, r2.Records, 1)
	txn := r2.Records[0].Value.Message.Transaction
	require.IsType(t, (*BlockValidatorAnchor)(nil), txn.Body)
	anchor := txn.Body.(*BlockValidatorAnchor)

	// Verify the anchor matches the receipt
	require.Equal(t, r1.Receipt.LocalBlock, anchor.MinorBlockIndex)
}
