// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
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
	st := sim.BuildAndSubmitSuccessfully(
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
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(DnUrl(), Oracle).
			WriteData([]byte("foo")).ToState().
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
	st := sim.BuildAndSubmitSuccessfully(
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

	req := new(api.GeneralQuery)
	req.Url = bob.WithTxID(*receiptHash).AsUrl()
	resp := new(api.TransactionQueryResponse)
	part, err := sim.Router().RouteAccount(bob)
	require.NoError(t, err)
	err = sim.Router().RequestAPIv2(context.Background(), part, "query", req, resp)
	require.NoError(t, err)
}
