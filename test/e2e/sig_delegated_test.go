// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestDoubleDelegated(t *testing.T) {
	// Tests AC-3069
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	charlie := AccountUrl("charlie")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)
	charlieKey := acctesting.GenerateKey(charlie)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWithVersion(GenesisTime, ExecutorVersionV2),
	)
	sim.VerboseConditions = true

	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")
	sim.SetRoute(charlie, "BVN2")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeIdentity(t, sim.DatabaseFor(charlie), charlie, charlieKey[32:])

	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e15)})

	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) {
		p.AddKeySpec(&KeySpec{Delegate: bob.JoinPath("book")})
		p.CreditBalance = 1e9
	})
	UpdateAccount(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), func(p *KeyPage) {
		p.AddKeySpec(&KeySpec{Delegate: charlie.JoinPath("book")})
		p.CreditBalance = 1e9
	})
	UpdateAccount(t, sim.DatabaseFor(charlie), charlie.JoinPath("book", "1"), func(p *KeyPage) {
		p.CreditBalance = 1e9
	})

	st := sim.SubmitSuccessfully(MustBuild(t,
		build.Transaction().For(alice, "tokens").
			BurnTokens(1, AcmePrecisionPower).
			SignWith(charlie, "book", "1").Version(1).Timestamp(1).PrivateKey(charlieKey).
			Delegator(bob, "book", "1").Delegator(alice, "book", "1")))

	sim.StepUntil(
		Txn(st[0].TxID).Succeeds(),
		Txn(st[0].TxID).Produced().Succeeds(),
		Sig(st[1].TxID).SingleCompletes())
}

func TestSingleDelegated(t *testing.T) {
	// Tests AC-3069
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWithVersion(GenesisTime, ExecutorVersionV2),
	)
	sim.VerboseConditions = true

	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])

	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e15)})

	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) {
		p.AddKeySpec(&KeySpec{Delegate: bob.JoinPath("book")})
		p.CreditBalance = 1e9
	})
	UpdateAccount(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), func(p *KeyPage) {
		p.CreditBalance = 1e9
	})

	st := sim.SubmitSuccessfully(MustBuild(t,
		build.Transaction().For(alice, "tokens").
			BurnTokens(1, AcmePrecisionPower).
			SignWith(bob, "book", "1").Version(1).Timestamp(1).PrivateKey(bobKey).
			Delegator(alice, "book", "1")))

	var cap *TransactionStatus
	sim.StepUntil(
		Txn(st[0].TxID).Succeeds(),
		Txn(st[0].TxID).Produced().Succeeds(),
		Sig(st[1].TxID).Succeeds(),
		Sig(st[1].TxID).AuthoritySignature().Succeeds(),
		Sig(st[1].TxID).AuthoritySignature().Produced().Succeeds(),
		Sig(st[1].TxID).SignatureRequest().Succeeds(),
		Sig(st[1].TxID).SignatureRequest().Produced().Succeeds(),
		Sig(st[1].TxID).CreditPayment().Capture(&cap).Succeeds(),
	)

	require.NotNil(t, cap)
	pay := sim.QueryMessage(cap.TxID, nil).Message.(*messaging.CreditPayment)
	require.NotZero(t, pay.Paid)
	fmt.Printf("Paid %s credits\n", FormatAmount(pay.Paid.AsUInt64(), CreditPrecisionPower))

	// Verify the transaction history
	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		history, err := batch.AccountTransaction(st[0].TxID).History().Get()
		require.NoError(t, err)

		chain := batch.Account(st[0].TxID.Account()).SignatureChain()
		var gotAuthSig, gotPayment, gotSigReq bool
		for _, index := range history {
			hash, err := chain.Entry(int64(index))
			require.NoError(t, err)

			msg, err := batch.Message2(hash).Main().Get()
			require.NoError(t, err)

			switch msg := msg.(type) {
			case *messaging.SignatureMessage:
				if msg.Signature.Type() == protocol.SignatureTypeAuthority {
					gotAuthSig = true
				}
			case *messaging.CreditPayment:
				gotPayment = true
			case *messaging.SignatureRequest:
				gotSigReq = true
			}
		}

		assert.True(t, gotAuthSig, "Expected authority signature")
		assert.True(t, gotPayment, "Expected credit payment")
		assert.True(t, gotSigReq, "Expected signature request(s)")
	})
}

func TestMultiLevelDelegation(t *testing.T) {
	// Tests AC-3069
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	charlie := AccountUrl("charlie")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)
	charlieKey := acctesting.GenerateKey(charlie)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.GenesisWithVersion(GenesisTime, ExecutorVersionV2),
	)

	// All on the same BVN
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")
	sim.SetRoute(charlie, "BVN0")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:], charlieKey[32:])
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeIdentity(t, sim.DatabaseFor(charlie), charlie, charlieKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e9)
	CreditCredits(t, sim.DatabaseFor(charlie), charlie.JoinPath("book", "1"), 1e9)
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) {
		p.AddKeySpec(&KeySpec{Delegate: bob.JoinPath("book")})
		p.AddKeySpec(&KeySpec{Delegate: charlie.JoinPath("book")})
		require.NoError(t, p.SetThreshold(3))
	})
	UpdateAccount(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), func(p *KeyPage) {
		p.AddKeySpec(&KeySpec{Delegate: charlie.JoinPath("book")})
	})

	env, err := build.
		Transaction().For(alice).
		CreateDataAccount(alice, "data").
		SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey).
		SignWith(charlie, "book", "1").Delegator(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(charlieKey).
		Done()
	require.NoError(t, err)

	// Take Charlie's signature, extract the key signature, and reconstruct
	// it as via Bob via Alice (two-layer delegation)
	sig := env.Signatures[1].(*DelegatedSignature).Signature
	sig = &DelegatedSignature{Delegator: AccountUrl("bob", "book0", "1"), Signature: sig}
	sig = &DelegatedSignature{Delegator: AccountUrl("alice", "book0", "1"), Signature: sig}
	env.Signatures = append(env.Signatures, sig)

	st := sim.Submit(env)
	require.EqualError(t, st[3].AsError(), "invalid signature")
}

func TestDelegationPath(t *testing.T) {
	// Tests #3255
	var timestamp uint64
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	charlie := AccountUrl("charlie")
	david := AccountUrl("david")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)
	charlieKey := acctesting.GenerateKey(charlie)
	davidKey1 := acctesting.GenerateKey(david, 1)
	davidKey2 := acctesting.GenerateKey(david, 2)
	davidKey3 := acctesting.GenerateKey(david, 3)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentityWithCredits(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	MakeIdentityWithCredits(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeIdentityWithCredits(t, sim.DatabaseFor(charlie), charlie, charlieKey[32:])
	MakeIdentityWithCredits(t, sim.DatabaseFor(david), david, davidKey1[32:], davidKey2[32:], davidKey3[32:])

	//                 Bob
	//             ↗         ↘
	// David (M=2)             Alice → Tokens
	//             ↘         ↗
	//               Charlie
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e15)})
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) {
		p.AddKeySpec(&KeySpec{Delegate: bob.JoinPath("book")})
		p.AddKeySpec(&KeySpec{Delegate: charlie.JoinPath("book")})
	})
	UpdateAccount(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), func(p *KeyPage) {
		p.AddKeySpec(&KeySpec{Delegate: david.JoinPath("book")})
	})
	UpdateAccount(t, sim.DatabaseFor(charlie), charlie.JoinPath("book", "1"), func(p *KeyPage) {
		p.AddKeySpec(&KeySpec{Delegate: david.JoinPath("book")})
	})
	UpdateAccount(t, sim.DatabaseFor(david), david.JoinPath("book", "1"), func(p *KeyPage) {
		p.AcceptThreshold = 2
	})

	// Submit the transaction signed David → Bob → Alice
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "tokens").
			BurnTokens(1, AcmePrecisionPower).
			SignWith(david, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(davidKey1).
			Delegator(bob, "book", "1").
			Delegator(alice, "book", "1"))
	sim.StepUntil(
		Sig(st[1].TxID).Completes())

	// Verify it is pending
	sim.Verify(
		Txn(st[0].TxID).IsPending())

	// Sign again via David → Charlie → Alice
	st = sim.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st[0].TxID).Load(sim.Query()).
			Url(david, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(davidKey2).
			Delegator(charlie, "book", "1").
			Delegator(alice, "book", "1"))
	sim.StepUntil(
		Sig(st[1].TxID).Completes())

	// Verify it is still pending
	sim.Verify(
		Txn(st[0].TxID).IsPending())

	// Sign a third time, via David → Bob → Alice
	st = sim.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st[0].TxID).Load(sim.Query()).
			Url(david, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(davidKey3).
			Delegator(charlie, "book", "1").
			Delegator(alice, "book", "1"))
	sim.StepUntil(
		Sig(st[1].TxID).Completes())

	// Verify it is executed
	sim.StepUntil(
		Txn(st[0].TxID).Succeeds())
}
