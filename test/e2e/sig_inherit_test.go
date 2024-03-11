// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math/big"
	"testing"

	"github.com/cometbft/cometbft/libs/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestInheritAuthority(t *testing.T) {
	var timestamp uint64
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e12)

	// Create
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice).
			CreateTokenAccount(alice, "tokens").ForToken(ACME).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st[0].TxID).Completes())

	// Verify the token account has no authority
	tokens := GetAccount[*TokenAccount](t, sim.DatabaseFor(alice), alice.JoinPath("tokens"))
	assert.Empty(t, tokens.Authorities)

	// Fund the account
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), func(a *TokenAccount) {
		a.CreditTokens(big.NewInt(1e12))
	})

	// Verify that the token account can be used with an empty authority set
	st = sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "tokens").
			BurnTokens(1, 0).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st[0].TxID).Completes())

	// Verify that a random account can't spend the tokens
	st = sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "tokens").
			BurnTokens(1, 0).
			SignWith(bob, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(bobKey))

	sim.StepUntil(
		Sig(st[1].TxID).AuthoritySignature().Fails().
			WithError(errors.Unauthorized).
			WithMessagef("%v is not authorized to sign transactions for %v", bob.JoinPath("book", "1"), alice.JoinPath("tokens")))
}

func TestRemoveLastAuthority(t *testing.T) {
	var timestamp uint64
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{
		Url:      alice.JoinPath("tokens"),
		TokenUrl: AcmeUrl(),
		Balance:  *big.NewInt(1e15),
		AccountAuth: AccountAuth{Authorities: []AuthorityEntry{
			{Url: alice.JoinPath("book")},
		}},
	})
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e12)

	// Remove the last authority
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "tokens").
			UpdateAccountAuth().
			Remove(alice.JoinPath("book")).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st[0].TxID).Completes())

	// Verify the token account has no authority
	tokens := GetAccount[*TokenAccount](t, sim.DatabaseFor(alice), alice.JoinPath("tokens"))
	assert.Empty(t, tokens.Authorities)

	// Verify that the token account can be used with an empty authority set
	st = sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "tokens").
			BurnTokens(1, 0).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st[0].TxID).Completes())

	// Verify that a random account can't spend the tokens
	st = sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "tokens").
			BurnTokens(1, 0).
			SignWith(bob, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(bobKey))

	sim.StepUntil(
		Sig(st[1].TxID).AuthoritySignature().Fails().
			WithError(errors.Unauthorized).
			WithMessagef("%v is not authorized to sign transactions for %v", bob.JoinPath("book", "1"), alice.JoinPath("tokens")))
}

func TestRemoveLastAuthorityFromRootAccount(t *testing.T) {
	var timestamp uint64
	alice := AccountUrl("alice")
	bob := AccountUrl("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e12)

	// Removing the last authority fails
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice).
			UpdateAccountAuth().
			Remove(alice.JoinPath("book")).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st[0].TxID).Fails().
			WithError(errors.BadRequest).
			WithMessage("removing the last authority from a root account is not allowed"))
}

func TestRemoveLastAuthorityCantInherit(t *testing.T) {
	var timestamp uint64
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	dbs := map[string]map[int]keyvalue.Beginner{}
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
		simulator.WithDatabase(func(partition *PartitionInfo, node int, _ log.Logger) keyvalue.Beginner {
			if dbs[partition.ID] == nil {
				dbs[partition.ID] = map[int]keyvalue.Beginner{}
			}
			dbs[partition.ID][node] = memory.New(nil)
			return dbs[partition.ID][node]
		}),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{
		Url:      alice.JoinPath("tokens"),
		TokenUrl: AcmeUrl(),
		Balance:  *big.NewInt(1e15),
		AccountAuth: AccountAuth{Authorities: []AuthorityEntry{
			{Url: alice.JoinPath("book")},
		}},
	})

	// Delete the ADI's authority set. This requires reaching into the key-value
	// store, since the data model will refuse to commit.
	adi := GetAccount[*ADI](t, sim.DatabaseFor(alice), alice)
	adi.Authorities = nil
	b, err := adi.MarshalBinary()
	require.NoError(t, err)
	for _, db := range dbs["BVN0"] {
		batch := db.Begin(nil, true)
		defer batch.Discard()
		require.NoError(t, batch.Put(record.NewKey("Account", alice, "Main"), b))
		require.NoError(t, batch.Commit())
	}

	// Removing the last authority fails
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "tokens").
			UpdateAccountAuth().
			Remove(alice.JoinPath("book")).
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st[0].TxID).Fails().
			WithError(errors.BadRequest).
			WithMessage("cannot remove last authority: no parent from which authorities can be inherited"))
}

func TestRemoveAuthoritiesFromACME(t *testing.T) {
	// Initialize
	g := new(network.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.OperatorAcceptThreshold.Set(1, 100) // Use a small number so M = 1
	g.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWith(GenesisTime, g),
	)

	acme := GetAccount[*TokenIssuer](t, sim.Database(Directory), AcmeUrl())
	require.Len(t, acme.Authorities, 1)

	// Removing the last authority fails
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(acme.Url).
			UpdateAccountAuth().
			Remove(acme.Authorities[0].Url).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0)))

	sim.StepUntil(
		Txn(st[0].TxID).Fails().
			WithError(errors.BadRequest).
			WithMessage("removing the last authority from a root account is not allowed"))
}
