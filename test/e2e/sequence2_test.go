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
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
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
		"v1": ExecutorVersionV1SignatureAnchoring,
		// "latest": ExecutorVersionLatest,
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
			st[i] = sim.SubmitTxnSuccessfully(MustBuild(t,
				build.Transaction().For(aliceUrl).
					SendTokens(1, protocol.AcmePrecisionPower).To(bobUrl).
					SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))
		}
		sim.StepUntil(True(func(*Harness) bool { return didDrop }))

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
	const bvnCount, valCount = 1, 1 // Anchor healing doesn't work with more than one validator
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), bvnCount, valCount),
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
	var anchors int
	sim.SetSubmitHookFor(alice, func(messages []messaging.Message) (drop bool, keepHook bool) {
		for _, msg := range messages {
			anchor, ok := msg.(*messaging.BlockAnchor)
			if !ok {
				continue
			}
			txn := anchor.Anchor.(*messaging.SequencedMessage).Message.(*messaging.TransactionMessage)
			if txn.Transaction.Body.Type() == TransactionTypeDirectoryAnchor {
				anchors++
				drop = true
			}
		}
		return drop, anchors < valCount*bvnCount
	})

	sim.StepUntil(True(func(*Harness) bool { return anchors >= valCount*bvnCount }))

	// Cause a synthetic transaction
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(faucet).
			SendTokens(1, AcmeOraclePrecisionPower).To(lite).
			SignWith(faucet).Timestamp(1).Version(1).PrivateKey(faucetKey))
	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())
}

func TestMissingBlockValidatorAnchorTxn(t *testing.T) {
	t.Skip("Flaky TODO FIXME")

	// Initialize
	const bvnCount, valCount = 3, 3
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), bvnCount, valCount),
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

	// Drop the next block validator anchor
	var anchors int
	sim.SetBlockHook(Directory, func(_ execute.BlockParams, envelopes []*messaging.Envelope) (_ []*messaging.Envelope, keepHook bool) {
		for _, env := range envelopes {
			for i := len(env.Messages) - 1; i >= 0; i-- {
				anchor, ok := env.Messages[i].(*messaging.BlockAnchor)
				if !ok {
					continue
				}
				txn := anchor.Anchor.(*messaging.SequencedMessage).Message.(*messaging.TransactionMessage)
				if txn.Transaction.Body.Type() == TransactionTypeBlockValidatorAnchor {
					anchors++
					env.Messages = append(env.Messages[:i], env.Messages[i+1:]...)
				}
			}
		}
		return envelopes, anchors < valCount
	})

	sim.StepUntil(True(func(*Harness) bool { return anchors >= valCount }))

	// Cause a synthetic transaction
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(faucet).
			SendTokens(1, AcmeOraclePrecisionPower).To(lite).
			SignWith(faucet).Timestamp(1).Version(1).PrivateKey(faucetKey))
	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())
}
