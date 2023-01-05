// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"math/big"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
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
	st := sim.SubmitSuccessfully(MustBuild(t,
		build.Transaction().For(DnUrl(), Operators, "1").
			UpdateKeyPage().Add().Entry().Hash([32]byte{1}).FinishEntry().FinishOperation().
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0)).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(2).Signer(sim.SignWithNode(Directory, 1)).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(3).Signer(sim.SignWithNode(Directory, 2))))

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
	st := sim.SubmitSuccessfully(MustBuild(t,
		build.Transaction().For(DnUrl(), Oracle).
			WriteData([]byte("foo")).ToState().
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0)).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(2).Signer(sim.SignWithNode(Directory, 1)).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(3).Signer(sim.SignWithNode(Directory, 2))))

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

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Execute
	st := sim.SubmitSuccessfully(MustBuild(t,
		build.Transaction().For(alice, "tokens").
			SendTokens(123, 0).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey)))

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

	req := new(api.GeneralQuery)
	req.Url = bob.WithTxID(*receiptHash).AsUrl()
	resp := new(api.TransactionQueryResponse)
	part, err := sim.Router().RouteAccount(bob)
	require.NoError(t, err)
	err = sim.Router().RequestAPIv2(context.Background(), part, "query", req, resp)
	require.NoError(t, err)
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
	st := sim.SubmitSuccessfully(MustBuild(t,
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
		_, _, err = indexing.ReceiptForChainIndex(&config.Describe{PartitionId: "BVN0"}, batch, batch.Account(alicePage).SignatureChain(), 0)
		require.EqualError(t, err, "cannot create receipt for entry 0 of signature chain: index chain is empty")
	})

	// Activate the new behavior
	st = sim.SubmitSuccessfully(MustBuild(t,
		build.Transaction().For(DnUrl()).
			ActivateProtocolVersion(ExecutorVersionV1SignatureAnchoring).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0))))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// Give the anchor a few blocks to propagate
	sim.StepN(10)

	// Execute
	st = sim.SubmitSuccessfully(MustBuild(t,
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
		_, _, err = indexing.ReceiptForChainIndex(&config.Describe{PartitionId: "BVN0"}, batch, batch.Account(alicePage).SignatureChain(), 0)
		require.NoError(t, err)
	})
}
