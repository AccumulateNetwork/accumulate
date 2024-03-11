// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/helpers"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestReplay(t *testing.T) {
	alice, bob, charlie := AccountUrl("alice"), AccountUrl("bob"), AccountUrl("charlie")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)
	charlieKey := acctesting.GenerateKey(charlie)
	setup := func(t *testing.T) *simulator.Simulator {
		sim := simulator.New(t, 3)
		sim.InitFromGenesis()

		sim.CreateIdentity(alice, aliceKey[32:])
		sim.CreateIdentity(bob, bobKey[32:])
		sim.CreateIdentity(charlie, charlieKey[32:])

		sim.CreateAccount(&DataAccount{Url: alice.JoinPath("data1")})
		sim.CreateAccount(&DataAccount{Url: alice.JoinPath("data2"), AccountAuth: AccountAuth{Authorities: []AuthorityEntry{
			{Url: bob.JoinPath("book")},
		}}})

		updateAccount(sim, alice.JoinPath("book", "1"), func(p *KeyPage) {
			p.CreditBalance = 1e9
			p.AddKeySpec(&KeySpec{Delegate: charlie.JoinPath("book")})
		})
		updateAccount(sim, bob.JoinPath("book", "1"), func(p *KeyPage) {
			p.CreditBalance = 1e9
		})
		updateAccount(sim, charlie.JoinPath("book", "1"), func(p *KeyPage) {
			p.CreditBalance = 1e9
		})
		return sim
	}

	newTxn := func(account, entry string) build.TransactionBuilder {
		return build.Transaction().
			For(alice.JoinPath(account)).
			Body(&WriteData{
				WriteToState: true,
				Entry: &DoubleHashDataEntry{
					Data: [][]byte{[]byte(entry)},
				},
			})
	}

	succeeds := func(sim *simulator.Simulator, b build.SignatureBuilder) {
		t.Helper()
		env := helpers.MustBuild(t, b)
		st, _ := sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(env)...)
		for _, st := range st {
			require.False(t, st.Failed(), "Expected %x to succeed", env.Transaction[0].GetHash())
		}

		entry := env.Transaction[0].Body.(*WriteData).Entry
		account := simulator.GetAccount[*DataAccount](sim, env.Transaction[0].Header.Principal)
		require.True(t, protocol.EqualDataEntry(entry, account.Entry))
	}

	fails := func(sim *simulator.Simulator, b build.SignatureBuilder) {
		t.Helper()
		env := helpers.MustBuild(t, b)
		sts, err := sim.SubmitAndExecuteBlock(env)
		if err != nil {
			// t.Log("Failed to submit: ", err)
			return // Did fail
		}

		for _, st := range sts {
			if st.Failed() {
				// t.Log("Failed to submit: ", st.Error)
				return // Did fail
			}
		}

		_, st, produced := sim.WaitForTransaction(delivered, env.Transaction[0].GetHash(), 50)
		if st == nil {
			return // Transaction is still pending after 50 blocks
		} else if st.Failed() {
			// t.Log("Failed to execute: ", st.Error)
			return // Did fail
		}

		for _, txid := range produced {
			h := txid.Hash()
			sts, _ = sim.WaitForTransactionFlow(delivered, h[:])
			for _, st := range sts {
				if st.Failed() {
					// t.Log("Failed to submit: ", st.Error)
					return // Did fail
				}
			}
		}

		t.Fatalf("Expected %x to fail but it didn't", env.Transaction[0].GetHash())
	}

	t.Run("Uninitiated without timestamp", func(t *testing.T) {
		fails(setup(t), newTxn("data1", t.Name()).
			SignWith(alice.JoinPath("book", "1")). // Sign with alice/book/1
			Version(1).                            //
			NoInitiator().                         // Don't initiate
			Timestamp(0).                          // No timestamp
			PrivateKey(aliceKey))                  // Sign
	})

	t.Run("Uninitiated with timestamp", func(t *testing.T) {
		fails(setup(t), newTxn("data1", t.Name()).
			SignWith(alice.JoinPath("book", "1")). // Sign with alice/book/1
			Version(1).                            //
			NoInitiator().                         // Don't initiate
			Timestamp(1).                          // With timestamp
			PrivateKey(aliceKey))                  // Sign
	})

	t.Run("Direct without timestamp", func(t *testing.T) {
		fails(setup(t), newTxn("data1", t.Name()).
			SignWith(alice.JoinPath("book", "1")). // Sign with alice/book/1
			Version(1).                            //
			Timestamp(0).                          // No timestamp
			PrivateKey(aliceKey))                  // Initiate
	})

	t.Run("Direct with timestamp", func(t *testing.T) {
		succeeds(setup(t), newTxn("data1", t.Name()).
			SignWith(alice.JoinPath("book", "1")). // Sign with alice/book/1
			Version(1).                            //
			Timestamp(1).                          // With timestamp
			PrivateKey(aliceKey))                  // Initiate
	})

	t.Run("Remote without timestamp", func(t *testing.T) {
		fails(setup(t), newTxn("data2", t.Name()).
			SignWith(bob.JoinPath("book", "1")). // Sign with bob/book/1
			Version(1).                          //
			Timestamp(0).                        // No timestamp
			PrivateKey(bobKey))                  // Initiate
	})

	t.Run("Remote with timestamp", func(t *testing.T) {
		succeeds(setup(t), newTxn("data2", t.Name()).
			SignWith(bob.JoinPath("book", "1")). // Sign with bob/book/1
			Version(1).                          //
			Timestamp(1).                        // With timestamp
			PrivateKey(bobKey))                  // Initiate
	})

	t.Run("Delegated without timestamp", func(t *testing.T) {
		fails(setup(t), newTxn("data1", t.Name()).
			SignWith(charlie.JoinPath("book", "1")). // Sign with charlie/book/1
			Version(1).                              //
			Delegator(alice.JoinPath("book", "1")).  //   delegate of alice/book/1
			Timestamp(0).                            // No timestamp
			PrivateKey(charlieKey))                  // Initiate
	})

	t.Run("Delegated with timestamp", func(t *testing.T) {
		succeeds(setup(t), newTxn("data1", t.Name()).
			SignWith(charlie.JoinPath("book", "1")). // Sign with charlie/book/1
			Version(1).                              //
			Delegator(alice.JoinPath("book", "1")).  //   delegate of alice/book/1
			Timestamp(1).                            // With timestamp
			PrivateKey(charlieKey))                  // Initiate
	})
}
