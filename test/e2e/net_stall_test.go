// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math/big"
	"strings"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestDnStall verifies that a BVN can detect if the DN stalls
func TestDnStall(t *testing.T) {
	t.Skip("Stall detection was removed")

	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")

	// Generate some history
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	st := sim.SubmitSuccessfully(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("tokens")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestamp(1).
			WithBody(&SendTokens{To: []*TokenRecipient{{Url: bob.JoinPath("tokens"), Amount: *big.NewInt(123)}}}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build())

	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	// Drop all transactions from the DN
	for _, p := range sim.Partitions() {
		sim.SetSubmitHook(p.ID, func(messages []messaging.Message) (dropTx bool, keepHook bool) {
			for _, msg := range messages {
				msg, ok := msg.(*messaging.UserSignature)
				if !ok {
					continue
				}
				sig, ok := msg.Signature.(*PartitionSignature)
				if !ok {
					continue
				}
				srcpart, ok := ParsePartitionUrl(sig.SourceNetwork)
				if !ok {
					continue
				}
				if strings.EqualFold(srcpart, Directory) {
					return true, true
				}
			}
			return false, true
		})
	}

	// Trigger another block
	sim.SubmitSuccessfully(
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("tokens")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestamp(2).
			WithBody(&SendTokens{To: []*TokenRecipient{{Url: bob.JoinPath("tokens"), Amount: *big.NewInt(123)}}}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build())

	// Run some blocks
	for i := 0; i < 50; i++ {
		sim.Step()
	}

	// // Verify that the number of acknowledged anchors is less than the number
	// // produced
	// ledger := GetAccount[*AnchorLedger](t, sim.Database("BVN0"), PartitionUrl("BVN0").JoinPath(AnchorPool)).Partition(DnUrl())
	// require.Less(t, ledger.Acknowledged, ledger.Produced)
}
