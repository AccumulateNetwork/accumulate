// Copyright 2023 The Accumulate Authors
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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestSendTokens_DuplicateRecipients tests sending two outputs to the same
// recipient.
func TestSendTokens_DuplicateRecipients(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	cases := map[string]struct {
		Recipient *url.URL
		Amounts   [2]int
	}{
		"Same domain, same amount":           {Recipient: alice, Amounts: [2]int{1, 1}},
		"Same domain, different amount":      {Recipient: alice, Amounts: [2]int{1, 2}},
		"Different domain, same amount":      {Recipient: bob, Amounts: [2]int{1, 1}},
		"Different domain, different amount": {Recipient: bob, Amounts: [2]int{1, 2}},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			// Initialize
			sim := NewSim(t,
				simulator.SimpleNetwork(t.Name(), 1, 1),
				simulator.Genesis(GenesisTime),
			)

			total := c.Amounts[0] + c.Amounts[1]

			MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
			CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
			MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
			CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(int64(total)))
			MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
			MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

			// Execute
			st := sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(alice, "tokens").
					SendTokens(c.Amounts[0], 0).To(c.Recipient, "tokens").
					And(c.Amounts[1], 0).To(c.Recipient, "tokens").
					SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

			sim.StepUntil(
				Txn(st.TxID).Succeeds(),
				Txn(st.TxID).Produced().Succeeds())

			// Give some time for all the synthetic messages to settle
			sim.StepN(10)

			// Verify the ledger
			part := PartitionUrl("BVN0")
			ledger := GetAccount[*SyntheticLedger](t, sim.Database("BVN0"), part.JoinPath(Synthetic)).Partition(part)
			require.Equal(t, ledger.Delivered, ledger.Produced)

			// Verify the amount
			account := GetAccount[*TokenAccount](t, sim.DatabaseFor(c.Recipient), c.Recipient.JoinPath("tokens"))
			require.Equal(t, total, int(account.Balance.Int64()))
		})
	}
}
