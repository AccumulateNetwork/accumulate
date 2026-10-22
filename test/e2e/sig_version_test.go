// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// signerVersionCases lists, per executor version, whether a signature is
// checked against the signer's page as it stands when its transaction
// executes (#4614). Before Tanegashima a signature is checked against the page
// only when the signature itself is processed; v2-kourou is the version Kermit
// runs, and its behavior must not change. From Tanegashima the authority's vote
// is checked again when it is consumed.
var signerVersionCases = []struct {
	Version ExecutorVersion
	Refused bool
}{
	{ExecutorVersionV2Kourou, false},
	{ExecutorVersionV2Tanegashima, true},
}

// TestSignerVersionAtExecution submits two updates to one page, both signed
// against version 1, in one envelope so that one block orders them. The first
// moves the page to version 2. From Tanegashima the second is then refused with
// badSignerVersion, exactly as it is when it arrives in a later block.
func TestSignerVersionAtExecution(t *testing.T) {
	alice := AccountUrl("alice")
	page := alice.JoinPath("book", "1")
	key1 := acctesting.GenerateKey(alice, 1)
	key2 := acctesting.GenerateKey(alice, 2)
	key3 := acctesting.GenerateKey(alice, 3)

	for _, c := range signerVersionCases {
		t.Run(c.Version.String(), func(t *testing.T) {
			// Initialize
			sim := NewSim(t,
				simulator.SimpleNetwork(t.Name(), 1, 1),
				simulator.GenesisWithVersion(GenesisTime, c.Version),
			)

			MakeIdentity(t, sim.DatabaseFor(alice), alice, key1[32:])
			CreditCredits(t, sim.DatabaseFor(alice), page, 1e9)

			// Two updates to the page, both signed by key 1 against version 1.
			// The first replaces key 1 with key 2; the second adds key 3.
			env1 := MustBuild(t,
				build.Transaction().For(page).
					UpdateKeyPage().
					Update().
					Entry().Key(key1, SignatureTypeED25519).FinishEntry().
					To().Key(key2, SignatureTypeED25519).FinishEntry().
					FinishOperation().
					SignWith(page).Version(1).Timestamp(1).PrivateKey(key1))
			env2 := MustBuild(t,
				build.Transaction().For(page).
					UpdateKeyPage().
					Add().Entry().Key(key3, SignatureTypeED25519).FinishEntry().FinishOperation().
					SignWith(page).Version(1).Timestamp(2).PrivateKey(key1))

			// Submit both in one envelope so that one block orders them. The
			// statuses come back in message order: both transactions, then
			// both signatures.
			st := sim.SubmitSuccessfully(&messaging.Envelope{
				Transaction: append(env1.Transaction, env2.Transaction...),
				Signatures:  append(env1.Signatures, env2.Signatures...),
			})
			require.Len(t, st, 4)
			txn1, txn2, sig2 := st[0].TxID, st[1].TxID, st[3].TxID

			// The first update executes
			sim.StepUntil(
				Txn(txn1).Succeeds())

			if !c.Refused {
				// Before Tanegashima the second executes too
				sim.StepUntil(
					Txn(txn2).Succeeds())

				kp := GetAccount[*KeyPage](t, sim.DatabaseFor(alice), page)
				require.Equal(t, uint64(3), kp.Version)
				require.Len(t, kp.Keys, 2)
				return
			}

			// From Tanegashima the second does not: its authority signature is
			// refused because the page is no longer at the version the
			// signature was checked against
			var authSt *TransactionStatus
			sim.StepUntil(
				Sig(sig2).Succeeds(),
				Sig(sig2).AuthoritySignature().Capture(&authSt).IsDelivered())
			require.NotNil(t, authSt)
			require.Equal(t, errors.BadSignerVersion, authSt.Code, "authority signature status: %v", authSt.AsError())

			// Give the second update every chance to execute, then verify it
			// has not
			sim.StepN(20)
			require.Equal(t, errors.Pending, sim.QueryTransaction(txn2, nil).Status, "the second update must not execute")

			// The page holds exactly what the first update left: version 2,
			// key 2 only
			kp := GetAccount[*KeyPage](t, sim.DatabaseFor(alice), page)
			require.Equal(t, uint64(2), kp.Version)
			require.Len(t, kp.Keys, 1)
			_, _, ok := kp.EntryByKey(key2[32:])
			require.True(t, ok, "key 2 must be on the page")
			_, _, ok = kp.EntryByKey(key3[32:])
			require.False(t, ok, "key 3 must not be on the page")
		})
	}
}

// TestSignerVersionAtExecutionDelegated verifies the same for a delegated
// signature, at each hop of the delegation (#4614). Alice's second book is a
// delegate of her first. A key of the second book signs a transaction for
// Alice, and in the same envelope a page on the delegation path is updated by a
// signature against the same version: in one case the second book's page,
// before the first book's page consumes its vote; in the other the first book's
// page, before its vote is consumed to execute the transaction. From
// Tanegashima the vote from the page that changed is refused with
// badSignerVersion and the transaction does not execute.
//
// Both books belong to Alice so that every message stays on her account's
// domain and is processed within the block (a vote produced on one domain for
// another is dispatched as a synthetic message instead).
func TestSignerVersionAtExecutionDelegated(t *testing.T) {
	alice := AccountUrl("alice")
	page1 := alice.JoinPath("book", "1")
	page2 := alice.JoinPath("book2", "1")
	key1 := acctesting.GenerateKey(alice, 1)
	key2 := acctesting.GenerateKey(alice, 2)
	otherKey := acctesting.GenerateKey(alice, 3)

	setup := func(t *testing.T, version ExecutorVersion) *Sim {
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 1, 1),
			simulator.GenesisWithVersion(GenesisTime, version),
		)

		MakeIdentity(t, sim.DatabaseFor(alice), alice, key1[32:])
		MakeKeyBook(t, sim.DatabaseFor(alice), alice.JoinPath("book2"), key2[32:])
		MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e15)})
		UpdateAccount(t, sim.DatabaseFor(alice), page1, func(p *KeyPage) {
			p.AddKeySpec(&KeySpec{Delegate: alice.JoinPath("book2")})
			p.CreditBalance = 1e9
		})
		UpdateAccount(t, sim.DatabaseFor(alice), page2, func(p *KeyPage) {
			p.CreditBalance = 1e9
		})
		return sim
	}

	// Key 2 burns one ACME of Alice's tokens, by delegation, against version 1
	// of page 2
	burn := func(t *testing.T, timestamp uint64) *messaging.Envelope {
		return MustBuild(t,
			build.Transaction().For(alice, "tokens").
				BurnTokens(1, AcmePrecisionPower).
				SignWith(page2).Delegator(page1).Version(1).Timestamp(timestamp).PrivateKey(key2))
	}

	// verify checks that the burn executed if and only if the refused vote's
	// version is not checked, and that the balance agrees
	verify := func(t *testing.T, sim *Sim, burn *url.TxID, refused bool) {
		balance := big.NewInt(1e15)
		if !refused {
			sim.StepUntil(
				Txn(burn).Succeeds())
			balance.Sub(balance, big.NewInt(AcmePrecision))
		} else {
			sim.StepN(20)
			require.Equal(t, errors.Pending, sim.QueryTransaction(burn, nil).Status, "the burn must not execute")
		}
		tokens := GetAccount[*TokenAccount](t, sim.DatabaseFor(alice), alice.JoinPath("tokens"))
		require.Equal(t, balance, &tokens.Balance)
	}

	for _, c := range signerVersionCases {
		t.Run(c.Version.String(), func(t *testing.T) {
			t.Run("Delegate", func(t *testing.T) {
				sim := setup(t, c.Version)

				// Page 2 is updated, then key 2 burns Alice's tokens. Both are
				// signed by key 2 against version 1 of page 2.
				env1 := MustBuild(t,
					build.Transaction().For(page2).
						UpdateKeyPage().
						Add().Entry().Key(otherKey, SignatureTypeED25519).FinishEntry().FinishOperation().
						SignWith(page2).Version(1).Timestamp(1).PrivateKey(key2))
				env2 := burn(t, 2)

				st := sim.SubmitSuccessfully(&messaging.Envelope{
					Transaction: append(env1.Transaction, env2.Transaction...),
					Signatures:  append(env1.Signatures, env2.Signatures...),
				})
				require.Len(t, st, 4)
				update, burn, burnSig := st[0].TxID, st[1].TxID, st[3].TxID

				// The update executes and page 2 moves to version 2
				sim.StepUntil(
					Txn(update).Succeeds())

				// From Tanegashima, page 2's vote on the burn is refused by
				// page 1: page 2 is no longer at the version it voted at
				if c.Refused {
					var authSt *TransactionStatus
					sim.StepUntil(
						Sig(burnSig).Succeeds(),
						Sig(burnSig).AuthoritySignature().Capture(&authSt).IsDelivered())
					require.NotNil(t, authSt)
					require.Equal(t, errors.BadSignerVersion, authSt.Code, "authority signature status: %v", authSt.AsError())
				}

				verify(t, sim, burn, c.Refused)
			})

			t.Run("Delegator", func(t *testing.T) {
				sim := setup(t, c.Version)

				// Key 2 burns Alice's tokens, then key 1 removes book 2 from
				// page 1. Each is signed against version 1 of its page.
				env1 := burn(t, 1)
				env2 := MustBuild(t,
					build.Transaction().For(page1).
						UpdateKeyPage().
						Remove().Entry().Owner(alice, "book2").FinishEntry().FinishOperation().
						SignWith(page1).Version(1).Timestamp(1).PrivateKey(key1))

				st := sim.SubmitSuccessfully(&messaging.Envelope{
					Transaction: append(env1.Transaction, env2.Transaction...),
					Signatures:  append(env1.Signatures, env2.Signatures...),
				})
				require.Len(t, st, 4)
				burn, update, burnSig := st[0].TxID, st[1].TxID, st[2].TxID

				// The update executes and page 1 moves to version 2
				sim.StepUntil(
					Txn(update).Succeeds())

				// Page 2's vote is accepted by page 1. From Tanegashima, page
				// 1's vote on the burn is refused when it is consumed: page 1
				// is no longer at the version it voted at
				if c.Refused {
					var authSt *TransactionStatus
					sim.StepUntil(
						Sig(burnSig).Succeeds(),
						Sig(burnSig).AuthoritySignature().Succeeds(),
						Sig(burnSig).AuthoritySignature().Produced().Capture(&authSt).IsDelivered())
					require.NotNil(t, authSt)
					require.Equal(t, errors.BadSignerVersion, authSt.Code, "authority signature status: %v", authSt.AsError())
				}

				verify(t, sim, burn, c.Refused)
			})
		})
	}
}
