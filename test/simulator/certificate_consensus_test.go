// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator_test

import (
	"math/big"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestCertificateConsensusExecutesThroughProduceBlock pins #4142 part (b): with
// CertificateConsensus, a submitted envelope reaches the executor inside the
// batch of a certificate handed to ExecutorBridge.ProduceBlock, on every node
// of the partition, and executes.
func TestCertificateConsensusExecutesThroughProduceBlock(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	// Which nodes were handed the submitted envelope, by transaction hash.
	var mu sync.Mutex
	var txid [32]byte
	seen := map[string]bool{}
	observe := func(partition string, node int, params adapter.BlockParams) {
		mu.Lock()
		defer mu.Unlock()
		for _, b := range params.Batches {
			for _, raw := range b.Transactions {
				env := new(messaging.Envelope)
				if env.UnmarshalBinary(raw) != nil {
					continue
				}
				msgs, err := env.Normalize()
				if err != nil {
					continue
				}
				for _, m := range msgs {
					if m, ok := m.(*messaging.TransactionMessage); ok && m.Transaction.ID().Hash() == txid {
						seen[partition+"/"+string(rune('0'+node))] = true
					}
				}
			}
		}
	}

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 2),
		simulator.Genesis(GenesisTime),
		simulator.CertificateConsensus(observe),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(100000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			SendTokens(7, 0).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))
	mu.Lock()
	txid = st.TxID.Hash()
	mu.Unlock()
	sim.StepUntil(
		Txn(st.TxID).Completes(),
	)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, map[string]bool{"BVN0/0": true, "BVN0/1": true}, seen,
		"every node of the partition must be handed the envelope in a certificate")
	require.Equal(t, int64(7), GetAccount[*TokenAccount](t, sim.DatabaseFor(bob), bob.JoinPath("tokens")).Balance.Int64())
}
