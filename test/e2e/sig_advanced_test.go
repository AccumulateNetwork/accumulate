// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"crypto/ed25519"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestAdvancedSigning(t *testing.T) {
	type Vote struct {
		Vote  VoteType
		Count int
	}
	type Case struct {
		Keys     uint64
		Accept   uint64
		Reject   uint64
		Response uint64
		Votes    []Vote
		Result   errors.Status
	}
	cases := []Case{
		{8, 6, 0, 0, []Vote{{VoteTypeAccept, 6}}, errors.Delivered},                      // Accepts
		{8, 6, 0, 0, []Vote{{VoteTypeAccept, 1}, {VoteTypeReject, 6}}, errors.Rejected},  // Rejects - implicit reject threshold
		{8, 6, 0, 0, []Vote{{VoteTypeAccept, 1}, {VoteTypeAbstain, 7}}, errors.Rejected}, // Abstains
		{8, 4, 0, 0, []Vote{{VoteTypeAccept, 4}, {VoteTypeReject, 4}}, errors.Delivered}, // Accepts and rejects - FCFS
		{8, 6, 1, 0, []Vote{{VoteTypeAccept, 5}, {VoteTypeReject, 1}}, errors.Rejected},  // Rejects - explicit reject threshold
		{8, 4, 0, 6, []Vote{{VoteTypeAccept, 4}}, errors.Pending},                        // Response threshold - insufficient
		{8, 4, 0, 6, []Vote{{VoteTypeAccept, 4}, {VoteTypeReject, 2}}, errors.Delivered}, // Response threshold
		{8, 6, 0, 0, []Vote{{VoteTypeAccept, 3}, {VoteTypeReject, 3}}, errors.Rejected},  // Deadlock
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("Case %d", i), func(t *testing.T) {
			alice := AccountUrl("alice")
			alicePriv := make([]ed25519.PrivateKey, c.Keys)
			alicePub := make([][]byte, c.Keys)
			for i := range alicePriv {
				alicePriv[i] = acctesting.GenerateKey(alice, i)
				alicePub[i] = alicePriv[i][32:]
			}

			// Initialize
			sim := NewSim(t,
				simulator.MemoryDatabase,
				simulator.SimpleNetwork(t.Name(), 1, 1),
				simulator.Genesis(GenesisTime),
			)

			MakeIdentity(t, sim.DatabaseFor(alice), alice, alicePub...)
			CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
			MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
			CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))

			// Set thresholds
			st := sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(alice, "book", "1").
					UpdateKeyPage().
					SetThreshold(c.Accept).
					SetRejectThreshold(c.Reject).
					SetResponseThreshold(c.Response).
					SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(alicePriv[0]))

			sim.StepUntil(
				Txn(st.TxID).Completes())

			// Build the transaction
			txn, err := build.Transaction().For(alice, "tokens").BurnTokens(1, 0).Done()
			require.NoError(t, err)

			// Build the signatures
			var i int
			var sigs []Signature
			for _, v := range c.Votes {
				for j := 0; j < v.Count; j++ {
					env, err := build.SignatureForTransaction(txn).Url(alice, "book", "1").Timestamp(2).Version(2).
						Vote(v.Vote).PrivateKey(alicePriv[i]).
						Done()
					require.NoError(t, err)
					sigs = append(sigs, env.Signatures...)
					i++
				}
			}

			// Submit the signature
			st = sim.SubmitTxnSuccessfully(&messaging.Envelope{Transaction: []*Transaction{txn}, Signatures: sigs})

			// Wait for the signatures to propagate
			sim.StepN(50)

			// Verify
			require.ErrorIs(t, sim.QueryTransaction(st.TxID, nil).Status, c.Result)
		})
	}
}
