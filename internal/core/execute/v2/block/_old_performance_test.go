// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"encoding/hex"
	"fmt"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
)

func BenchmarkBlock(b *testing.B) {
	// Disable debug features for the duration
	acctesting.DisableDebugFeatures()
	defer acctesting.EnableDebugFeatures()

	// bvnCount := []int{1, 2, 4, 8}
	bvnCount := []int{1}
	blockSize := []int{50, 100, 200, 500, 1000}
	// blockSize := []int{200}
	scenarios := map[string][]executor{
		"no-op": {
			{protocol.TransactionTypeAddCredits, func(st *chain.StateManager, tx *chain.Delivery) error {
				return nil
			}},
		},
		"create account": {
			{protocol.TransactionTypeAddCredits, func(st *chain.StateManager, tx *chain.Delivery) error {
				u := &url.URL{Authority: hex.EncodeToString(tx.Transaction.GetHash())}
				return st.Create(&protocol.UnknownAccount{Url: u})
			}},
		},
		"synth txn": {
			{protocol.TransactionTypeAddCredits, func(st *chain.StateManager, tx *chain.Delivery) error {
				u := &url.URL{Authority: hex.EncodeToString(tx.Transaction.GetHash())}
				st.Submit(u, &protocol.SyntheticDepositCredits{})
				return nil
			}},
			{protocol.TransactionTypeSyntheticDepositCredits, func(st *chain.StateManager, tx *chain.Delivery) error {
				return nil
			}},
		},
	}

	alice := protocol.AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	for scname, scenario := range scenarios {
		for _, bvnCount := range bvnCount {
			sim := simulator.New(b, bvnCount)
			sim.InitFromGenesis()
			sim.CreateIdentity(alice, aliceKey[32:])

			for _, x := range sim.Executors {
				for _, exec := range scenario {
					x.Executor.SetExecutor_TESTONLY(exec)
				}
			}

			for _, blockSize := range blockSize {
				var timestamp uint64
				envs := make([]*messaging.Envelope, blockSize)
				for j := range envs {
					envs[j] = acctesting.NewTransaction().
						WithPrincipal(alice).
						WithSigner(alice.JoinPath("book", "1"), 1).
						WithTimestampVar(&timestamp).
						WithBody(&protocol.AddCredits{}).
						Initiate(protocol.SignatureTypeED25519, aliceKey).
						Build()
				}

				b.Run(fmt.Sprintf("%d BVNs, %d txns, %s", bvnCount, blockSize, scname), func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						sim.RunAndReset(func() {
							sim.MustSubmitAndExecuteBlock(envs...)
							sim.WaitForTransactions(delivered, envs...)
						})
					}
				})
			}
		}
	}
}
