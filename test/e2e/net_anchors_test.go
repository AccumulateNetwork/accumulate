// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestDropInitialAnchor is a simple test that simulates adverse network
// conditions causing anchors to be dropped when they're initially sent.
func TestDropInitialAnchor(t *testing.T) {
	alice := build.
		Identity("alice").Create("book").
		Tokens("tokens").Create("ACME").Add(1e9).Identity().
		Book("book").Page(1).Create().AddCredits(1e9).Book().Identity()
	aliceKey := alice.Book("book").Page(1).
		GenerateKey(SignatureTypeED25519)

	bob := build.
		Identity("bob").Create("book").
		Tokens("tokens").Create("ACME").Identity()

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime).With(alice, bob),

		// Drop anchors when they are initially sent, instead relying on the
		// Conductor's anchor healing
		simulator.DropInitialAnchor(),
	)

	// Execute
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "tokens").
			SendTokens(123, 0).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	sim.StepUntil(
		Sig(st[1].TxID).Completes(),
		Txn(st[0].TxID).Completes())

	// Verify
	account, err := bob.Tokens("tokens").Load(sim.DatabaseFor)
	require.NoError(t, err)
	require.Equal(t, 123, int(account.Balance.Int64()))
}

// TestReuseDirectoryAnchorSignatures verifies that anchor signatures the DN sends to
// itself can be reused for DN anchors sent to a BVN.
func TestReuseDirectoryAnchorSignatures(t *testing.T) {
	const numVal, numNode = 3, 1

	var trapBlock uint64
	var dnSigs []KeySignature
	var toBVN *messaging.SequencedMessage
	anchorTrap := func(ctx context.Context, env *messaging.Envelope) (send bool, err error) {
		if trapBlock == 0 || len(env.Messages) != 1 {
			return true, nil
		}

		// Is the envelope a DN anchor signature for the target block?
		blk, ok := env.Messages[0].(*messaging.BlockAnchor)
		if !ok {
			return true, nil
		}
		seq, ok := blk.Anchor.(*messaging.SequencedMessage)
		if !ok {
			return true, nil
		}
		txn, ok := seq.Message.(*messaging.TransactionMessage)
		if !ok {
			return true, nil
		}
		body, ok := txn.Transaction.Body.(*DirectoryAnchor)
		if !ok || body.MinorBlockIndex != trapBlock {
			return true, nil
		}

		// Capture signatures sent to the DN but don't drop them
		if DnUrl().Equal(seq.Destination) {
			dnSigs = append(dnSigs, blk.Signature)
			return true, nil
		}

		// Let the first signature sent to the BVN through and drop the rest
		if toBVN == nil {
			toBVN = seq
			return true, nil
		}
		return false, nil
	}

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), numVal, numNode),
		simulator.Genesis(GenesisTime),
		simulator.CaptureDispatchedMessages(anchorTrap),
	)

	// Do something
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "book", "1").BurnCredits(1).
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	// Trap anchors
	trapBlock = sim.S.BlockIndex(Directory)
	sim.StepUntil(
		True(func(*Harness) bool {
			return toBVN != nil && len(dnSigs) == numVal*numNode
		}))

	// Verify the anchor sent to the BVN is pending
	txn := toBVN.Message.(*messaging.TransactionMessage).Transaction
	sim.StepUntil(
		Txn(txn.ID()).IsPending())

	// Take the anchors sent to the DN and send them to the BVN to resolve the
	// anchor
	for _, sig := range dnSigs {
		sim.SubmitSuccessfully(&messaging.Envelope{
			Messages: []messaging.Message{
				&messaging.BlockAnchor{
					Signature: sig,
					Anchor:    toBVN,
				},
			},
		})
	}

	// Verify the anchor completes
	sim.StepUntil(
		Txn(txn.ID()).Completes())

	// Verify the user transaction completes
	sim.StepUntil(
		Txn(st.TxID).Completes())
}
