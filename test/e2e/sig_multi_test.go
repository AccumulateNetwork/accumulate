// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestSimpleMultisig(t *testing.T) {
	// Tests AC-3069
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice, 1)
	bobKey := acctesting.GenerateKey(alice, 2)
	charlieKey := acctesting.GenerateKey(alice, 3)

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:], bobKey[32:], charlieKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) {
		p.AcceptThreshold = 2
	})

	// Initiate
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice).
			CreateTokenAccount(alice, "tokens").ForToken(ACME).
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	// Wait until the principal lists the transaction as pending
	sim.StepUntil(True(func(h *Harness) bool {
		r, err := h.Query().QueryPendingIds(context.Background(), st[0].TxID.Account(), nil)
		switch {
		case err == nil:
			for _, r := range r.Records {
				if r.Value.Hash() == st[0].TxID.Hash() {
					return true
				}
			}
		case !errors.Is(err, errors.NotFound):
			require.NoError(h.TB, err)
		}
		return false
	}))

	// Sign again
	st = sim.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st[0].TxID).Load(sim.Query()).
			Url(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(bobKey))

	sim.StepUntil(
		Txn(st[0].TxID).Succeeds())
}

func TestOtherAuthority(t *testing.T) {
	// Tests AC-3069
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e12)

	// Initiate
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice).
			CreateTokenAccount(alice, "tokens").ForToken(ACME).WithAuthority(bob, "book").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	// Wait until the principal lists the transaction as pending
	sim.StepUntil(True(func(h *Harness) bool {
		r, err := h.Query().QueryPendingIds(context.Background(), st[0].TxID.Account(), nil)
		switch {
		case err == nil:
			for _, r := range r.Records {
				if r.Value.Hash() == st[0].TxID.Hash() {
					return true
				}
			}
		case !errors.Is(err, errors.NotFound):
			require.NoError(h.TB, err)
		}
		return false
	}))

	sim.StepUntil(
		Txn(st[0].TxID).IsPending())

	// Verify that payments and votes are recorded
	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		pay, err := batch.Account(alice).Transaction(st[0].TxID.Hash()).Payments().Get()
		require.NoError(t, err)
		require.NotEmpty(t, pay)
		votes, err := batch.Account(alice).Transaction(st[0].TxID.Hash()).Votes().Get()
		require.NoError(t, err)
		require.NotEmpty(t, votes)
	})

	// Sign with the other authority
	st = sim.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st[0].TxID).Load(sim.Query()).
			Url(bob, "book", "1").Version(1).Timestamp(1).PrivateKey(bobKey))

	sim.StepUntil(
		Txn(st[0].TxID).Succeeds(),
		Sig(st[1].TxID).Completes())

	// Verify that payments and votes are wiped
	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		pay, err := batch.Account(alice).Transaction(st[0].TxID.Hash()).Payments().Get()
		require.NoError(t, err)
		assert.Empty(t, pay)
		votes, err := batch.Account(alice).Transaction(st[0].TxID.Hash()).Votes().Get()
		require.NoError(t, err)
		assert.Empty(t, votes)
	})
}

func TestMultiAuthority(t *testing.T) {
	var timestamp uint64
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e12)

	// Initiate
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice).
			CreateTokenAccount(alice, "tokens").ForToken(ACME).
			WithAuthority(alice, "book").
			WithAuthority(bob, "book").
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

	// Wait for the transaction to appear (it must be pending)
	sim.StepUntil(
		Txn(st[0].TxID).IsPending())

	// Sign with the other authority
	st = sim.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st[0].TxID).Load(sim.Query()).
			Url(bob, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(bobKey))

	sim.StepUntil(
		Txn(st[0].TxID).Succeeds(),
		Sig(st[1].TxID).Completes())

	// Remove the second authority
	st = sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "tokens").
			UpdateAccountAuth().Remove(bob, "book").
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st[0].TxID).IsPending())

	// Sign with the other authority
	st = sim.BuildAndSubmitSuccessfully(
		build.SignatureForTransaction(sim.QueryTransaction(st[0].TxID, nil).Message.Transaction).
			Url(bob, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(bobKey))

	sim.StepUntil(
		Txn(st[0].TxID).Succeeds(),
		Sig(st[1].TxID).Completes())
}

func TestBadAuthDisabledLogicV1(t *testing.T) {
	var timestamp uint64
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	g := &network.GlobalValues{ExecutorVersion: ExecutorVersionV1DoubleHashEntries}
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWith(GenesisTime, g),
	)

	tokens := new(TokenAccount)
	tokens.Url = alice.JoinPath("tokens")
	tokens.TokenUrl = AcmeUrl()
	tokens.Balance = *big.NewInt(1e12)

	// Two authorities, one  of which is disabled
	tokens.AddAuthority(alice.JoinPath("book"))
	auth, _ := tokens.AddAuthority(alice.JoinPath("book2"))
	auth.Disabled = true

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	MakeAccount(t, sim.DatabaseFor(alice), tokens)
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e12)

	// Attempt to sign with Bob, who is not a valid signer
	st := sim.BuildAndSubmitTxn(
		build.Transaction().For(alice, "tokens").
			BurnTokens(1, 0).
			SignWith(bob, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(bobKey))

	// Because of the bug, the signature is accepted though the actual authority
	// is still required
	sim.StepUntil(
		Txn(st.TxID).IsPending())
}

func TestBadAuthDisabledLogicV2(t *testing.T) {
	var timestamp uint64
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	g := &network.GlobalValues{ExecutorVersion: ExecutorVersionLatest}
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWith(GenesisTime, g),
	)

	tokens := new(TokenAccount)
	tokens.Url = alice.JoinPath("tokens")
	tokens.TokenUrl = AcmeUrl()
	tokens.Balance = *big.NewInt(1e12)

	// Two authorities, one  of which is disabled
	tokens.AddAuthority(alice.JoinPath("book"))
	auth, _ := tokens.AddAuthority(alice.JoinPath("book2"))
	auth.Disabled = true

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	MakeAccount(t, sim.DatabaseFor(alice), tokens)
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e12)

	// Attempt to sign with Bob, who is not a valid signer
	st := sim.BuildAndSubmit(
		build.Transaction().For(alice, "tokens").
			BurnTokens(1, 0).
			SignWith(bob, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(bobKey))

	// The signature is accepted since the authorization check occurs on the
	// authority signature, not the user signature
	require.NoError(t, st[0].AsError())

	// But the authority signature is rejected
	sim.StepUntil(
		Sig(st[1].TxID).AuthoritySignature().Fails().
			WithError(errors.Unauthorized).
			WithMessage("acc://bob.acme/book/1 is not authorized to sign transactions for acc://alice.acme/tokens"))
}
