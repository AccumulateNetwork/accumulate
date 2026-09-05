// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestAnchorThreshold verifies that an anchor is not executed until it is
// signed by 2/3 of the validators
func TestAnchorThreshold(t *testing.T) {
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)

	const bvnCount, valCount = 1, 3 // One BVN, three nodes
	opts := []simulator.Option{
		simulator.SimpleNetwork(t.Name(), bvnCount, valCount),
		simulator.Genesis(GenesisTime),
	}

	// Capture the BVN's anchors and verify they're the same
	// The hook runs on EVERY node's goroutine, so everything it captures is
	// shared. Two goroutines appending to `anchors` can lose an entry
	// outright, and the test then fails on a count for reasons unrelated to
	// what it is testing (#4171).
	var anchorsMu sync.Mutex
	var anchors []*messaging.BlockAnchor
	opts = append(opts, simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (send bool, err error) {
		anchorsMu.Lock()
		defer anchorsMu.Unlock()
		for _, m := range env.Messages {
			blk, ok := m.(*messaging.BlockAnchor)
			if !ok {
				continue
			}
			require.Len(t, env.Messages, 1)
			require.IsType(t, (*messaging.SequencedMessage)(nil), blk.Anchor)
			seq := blk.Anchor.(*messaging.SequencedMessage)
			require.IsType(t, (*messaging.TransactionMessage)(nil), seq.Message)
			txn := seq.Message.(*messaging.TransactionMessage)
			anchor, ok := txn.Transaction.Body.(*BlockValidatorAnchor)
			if !ok || anchor.MinorBlockIndex <= 10 {
				continue
			}

			// One copy per signer: the destination's requester pulls the
			// dropped anchor back, signed by whichever validator answers,
			// and a second copy under the same key is no second signature
			for _, have := range anchors {
				if bytes.Equal(have.Signature.GetPublicKey(), blk.Signature.GetPublicKey()) {
					return false, nil
				}
			}
			anchors = append(anchors, blk)
			return false, nil
		}
		return true, nil
	}))

	sim := NewSim(t, opts...)
	sim.SetRoute(alice, "BVN0")

	// Clear out the anchors from genesis
	sim.StepN(50)

	// Do something
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

	sim.SubmitTxnSuccessfully(
		MustBuild(t, build.Transaction().
			For(alice).
			Body(&CreateTokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(1).PrivateKey(aliceKey)),
	)

	sim.StepUntil(True(func(*Harness) bool { return len(anchors) >= valCount }))

	txid := anchors[0].Anchor.(*messaging.SequencedMessage).Message.ID()
	for _, anchor := range anchors[1:] {
		require.True(t, messaging.EqualMessage(anchors[0].Anchor, anchor.Anchor))
	}

	// Verify the anchor was captured
	View(t, sim.Database("BVN0"), func(batch *database.Batch) {
		hash := txid.Hash()
		v, err := batch.Transaction(hash[:]).Status().Get()
		require.NoError(t, err)
		require.True(t, v.Equal(new(TransactionStatus)))
	})

	// Submit one signature and verify the anchor does not execute: below its
	// quorum it is held in the anchor stream's stage, not recorded pending
	// (executor spec, "One chain per pair, one stage per chain")
	sim.SubmitSuccessfully(&messaging.Envelope{Messages: []messaging.Message{anchors[0]}})
	sim.StepN(20)
	requireNotExecuted(t, sim, txid)

	// Re-submit the first signature and verify it still does not execute
	sim.SubmitSuccessfully(&messaging.Envelope{Messages: []messaging.Message{anchors[0]}})
	sim.StepN(50)
	requireNotExecuted(t, sim, txid)

	// Submit a second signature and verify it is delivered
	sim.SubmitSuccessfully(&messaging.Envelope{Messages: []messaging.Message{anchors[1]}})
	sim.StepUntil(Txn(txid).Succeeds())
}

func TestAnchorPlaceholder(t *testing.T) {
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)

	opts := []simulator.Option{
		simulator.Genesis(GenesisTime),
	}

	// One BVN, two nodes
	opts = append(opts, simulator.SimpleNetwork(t.Name(), 1, 2))

	// Capture anchors
	var captured []*messaging.BlockAnchor
	opts = append(opts, simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (send bool, err error) {
		for _, m := range env.Messages {
			blk, ok := m.(*messaging.BlockAnchor)
			if !ok {
				continue
			}
			require.Len(t, env.Messages, 1)
			require.IsType(t, (*messaging.SequencedMessage)(nil), blk.Anchor)
			seq := blk.Anchor.(*messaging.SequencedMessage)
			require.IsType(t, (*messaging.TransactionMessage)(nil), seq.Message)
			txn := seq.Message.(*messaging.TransactionMessage)
			anchor, ok := txn.Transaction.Body.(*BlockValidatorAnchor)
			if !ok || anchor.MinorBlockIndex <= 10 {
				continue
			}

			// One copy per signer (see TestAnchorThreshold)
			for _, have := range captured {
				if bytes.Equal(have.Signature.GetPublicKey(), blk.Signature.GetPublicKey()) {
					return false, nil
				}
			}
			captured = append(captured, blk)
			return false, nil
		}
		return true, nil
	}))

	// Init
	sim := NewSim(t, opts...)
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

	// Get past the genesis anchors
	sim.StepN(50)

	// Do something and wait for anchors
	sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "book", "1").
			BurnCredits(1).
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	// Wait for anchors
	sim.StepUntil(
		True(func(h *Harness) bool { return len(captured) >= 2 }))

	// Replace the second transaction with a hash
	txn := captured[1].
		Anchor.(*messaging.SequencedMessage).
		Message.(*messaging.TransactionMessage)
	hash := txn.Hash()
	txn.Transaction.Body = &RemoteTransaction{Hash: hash}

	// Submit the first one
	sim.SubmitSuccessfully(&messaging.Envelope{Messages: []messaging.Message{captured[0]}})

	// Verify it does not execute on one signature: held in the stage, not
	// on any pending list
	sim.StepN(20)
	requireNotExecuted(t, sim, txn.ID())
	require.Empty(t, sim.QueryPendingIds(txn.ID().Account(), nil).Records, "a held anchor is not recorded pending")

	// Submit the second one
	st := sim.SubmitSuccessfully(&messaging.Envelope{Messages: []messaging.Message{captured[1]}})

	// Verify that AdjustStatusIDs changes the ID back to the old ID
	require.Equal(t, captured[1].ID().String(), st[0].TxID.String())
	adjustStatusIDs([]messaging.Message{captured[1]}, st)
	require.Equal(t, captured[1].OldID().String(), st[0].TxID.String())

	// Verify it executes
	sim.StepUntil(
		Txn(txn.ID()).Succeeds())

	// Verify the signatures succeed
	for _, msg := range captured {
		sim.Verify(
			Msg(msg.ID()).Succeeds())
	}
}

// adjustStatusIDs corrects for the fact that the block anchor's ID method was
// bad and has been changed.
func adjustStatusIDs(messages []messaging.Message, st []*TransactionStatus) {
	adjustedIDs := map[[32]byte]*url.TxID{}
	for _, msg := range messages {
		switch msg := msg.(type) {
		case *messaging.BlockAnchor:
			adjustedIDs[msg.Hash()] = msg.OldID()
		}
	}
	for _, st := range st {
		if st.TxID == nil {
			continue
		}
		id, ok := adjustedIDs[st.TxID.Hash()]
		if ok {
			st.TxID = id
		}
	}
}

// requireNotExecuted asserts a transaction has no delivered status: it may be
// held in a stage, but nothing about it has executed.
func requireNotExecuted(t *testing.T, sim *Sim, id *url.TxID) {
	t.Helper()
	View(t, sim.DatabaseFor(id.Account()), func(batch *database.Batch) {
		hash := id.Hash()
		v, err := batch.Transaction(hash[:]).Status().Get()
		require.NoError(t, err)
		require.False(t, v.Delivered(), "the anchor executed with %v", v.Code)
	})
}
