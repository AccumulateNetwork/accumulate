// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package validate

import (
	"math/big"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func BenchmarkTPS(b *testing.B) {
	acctesting.DisableDebugFeatures()
	defer acctesting.EnableDebugFeatures()

	const N = 200
	var timestamp uint64
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey("bob")
	bob := acctesting.AcmeLiteAddressStdPriv(bobKey)

	// Initialize
	sim := NewSim(b,
		simulator.SimpleNetwork(b.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(b, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(b, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(b, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(b, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))

	sts := make([]*protocol.TransactionStatus, N)
	cond := make([]Condition, N)
	for i := 0; i < b.N; i++ {
		sts, cond = sts[:0], cond[:0]

		// Submit all the transactions
		for i := 0; i < N; i++ {
			txn := MustBuild(b,
				build.Transaction().For(alice, "tokens").
					SendTokens(1, 0).To(bob).
					SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

			st := sim.SubmitTxnSuccessfully(txn)
			sts = append(sts, st)
			cond = append(cond, Txn(st.TxID).Completes())
		}

		// Wait for all of them to complete
		sim.StepUntil(cond[0])
	}
}
