// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestMissingSynthTxn(t *testing.T) {
	Run(t, map[string]ExecutorVersion{
		"v1":     ExecutorVersionV1SignatureAnchoring,
		"latest": ExecutorVersionLatest,
	}, func(t *testing.T, version ExecutorVersion) {
		var timestamp uint64

		// Initialize
		globals := new(core.GlobalValues)
		globals.ExecutorVersion = version
		sim := NewSim(t,
			simulator.MemoryDatabase,
			simulator.SimpleNetwork(t.Name(), 3, 3),
			simulator.GenesisWith(GenesisTime, globals),
		)

		alice := acctesting.GenerateKey("Alice")
		aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
		bob := acctesting.GenerateKey("Bob")
		bobUrl := acctesting.AcmeLiteAddressStdPriv(bob)
		MakeLiteTokenAccount(t, sim.DatabaseFor(aliceUrl), alice[32:], AcmeUrl())

		// The first time an envelope contains a deposit, drop the first deposit
		var didDrop bool
		sim.SetSubmitHookFor(bobUrl, func(messages []messaging.Message) (drop bool, keepHook bool) {
			for _, msg := range messages {
			again:
				switch m := msg.(type) {
				case interface{ Unwrap() messaging.Message }:
					msg = m.Unwrap()
					goto again
				case messaging.MessageWithTransaction:
					if m.GetTransaction().Body.Type() == TransactionTypeSyntheticDepositTokens {
						fmt.Printf("Dropping %X\n", m.GetTransaction().GetHash()[:4])
						didDrop = true
						return true, false
					}
				}
			}
			return false, true
		})

		// Execute
		st := make([]*protocol.TransactionStatus, 5)
		for i := range st {
			st[i] = sim.SubmitSuccessfully(MustBuild(t,
				build.Transaction().For(aliceUrl).
					SendTokens(1, protocol.AcmePrecisionPower).To(bobUrl).
					SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))
		}
		sim.StepUntil(func(*Harness) bool { return didDrop })

		for _, st := range st {
			sim.StepUntil(
				Txn(st.TxID).Succeeds(),
				Txn(st.TxID).Produced().Succeeds())
		}

		// Verify
		lta := GetAccount[*LiteTokenAccount](t, sim.DatabaseFor(bobUrl), bobUrl)
		require.Equal(t, len(st)*protocol.AcmePrecision, int(lta.Balance.Uint64()))
	})
}

func TestMissingDirectoryAnchorTxn(t *testing.T) {
	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	liteKey := acctesting.GenerateKey("Lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey)
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })

	faucetKey := acctesting.GenerateKey("Faucet")
	faucet := acctesting.AcmeLiteAddressStdPriv(faucetKey)
	MakeLiteTokenAccount(t, sim.DatabaseFor(faucet), faucetKey[32:], AcmeUrl())

	// Drop the next directory anchor
	var didDrop bool
	sim.SetSubmitHookFor(alice, func(messages []messaging.Message) (drop, keepHook bool) {
		for _, msg := range messages {
			var txn messaging.MessageWithTransaction
			if encoding.SetPtr(msg, &txn) == nil && txn.GetTransaction().Body.Type() == TransactionTypeDirectoryAnchor {
				didDrop = true
				return true, false
			}
		}
		return false, true
	})

	// Cause a synthetic transaction
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(faucet).
			SendTokens(1, AcmeOraclePrecisionPower).To(lite).
			SignWith(faucet).Timestamp(1).Version(1).PrivateKey(faucetKey))
	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	require.True(t, didDrop)
}

func TestMissingBlockValidatorAnchorTxn(t *testing.T) {
	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	liteKey := acctesting.GenerateKey("Lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey)
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })

	faucetKey := acctesting.GenerateKey("Faucet")
	faucet := acctesting.AcmeLiteAddressStdPriv(faucetKey)
	MakeLiteTokenAccount(t, sim.DatabaseFor(faucet), faucetKey[32:], AcmeUrl())

	// Drop the next directory anchor
	var didDrop bool
	sim.SetSubmitHook(Directory, func(messages []messaging.Message) (drop, keepHook bool) {
		for _, msg := range messages {
			var txn messaging.MessageWithTransaction
			if encoding.SetPtr(msg, &txn) == nil && txn.GetTransaction().Body.Type() == TransactionTypeBlockValidatorAnchor {
				didDrop = true
				return true, false
			}
		}
		return false, true
	})

	// Cause a synthetic transaction
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(faucet).
			SendTokens(1, AcmeOraclePrecisionPower).To(lite).
			SignWith(faucet).Timestamp(1).Version(1).PrivateKey(faucetKey))
	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	require.True(t, didDrop)
}
