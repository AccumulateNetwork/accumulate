// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
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
	const bvnCount, valCount = 1, 3 // One BVN, three nodes
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), bvnCount, valCount),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")

	// Clear out the anchors from genesis
	sim.StepN(50)

	// Do something
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

	sim.SubmitSuccessfully(
		acctesting.NewTransaction().
			WithPrincipal(alice).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestamp(1).
			WithBody(&CreateTokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build())

	// Capture the BVN's anchors and verify they're the same
	var anchors []*messaging.Envelope
	sim.SetSubmitHook("Directory", func(messages []messaging.Message) (drop bool, keepHook bool) {
		for _, msg := range messages {
			seq, ok := msg.(*messaging.SequencedMessage)
			if !ok {
				continue
			}
			txn, ok := seq.Message.(*messaging.UserTransaction)
			if !ok {
				continue
			}
			if txn.Transaction.Body.Type() != TransactionTypeBlockValidatorAnchor {
				continue
			}
			anchors = append(anchors, &messaging.Envelope{Messages: messages})
			return true, len(anchors) < valCount
		}
		return false, true
	})

	sim.StepUntil(func(*Harness) bool { return len(anchors) >= valCount })

	txid := anchors[0].Messages[0].ID()
	for _, anchor := range anchors[1:] {
		require.True(t, messaging.EqualMessage(anchors[0].Messages[0], anchor.Messages[0]))
	}

	// Verify the anchor was captured
	View(t, sim.Database("BVN0"), func(batch *database.Batch) {
		hash := txid.Hash()
		v, err := batch.Transaction(hash[:]).Status().Get()
		require.NoError(t, err)
		require.True(t, v.Equal(new(TransactionStatus)))
	})

	// Submit one signature and verify it is pending
	sim.SubmitSuccessfully(anchors[0])
	sim.StepUntil(Txn(txid).IsPending())

	// Re-submit the first signature and verify it is still pending
	sim.SubmitSuccessfully(anchors[0])
	sim.StepN(50)
	require.True(t, sim.QueryTransaction(txid, nil).Status.Pending())

	// Submit a second signature and verify it is delivered
	sim.SubmitSuccessfully(anchors[1])
	sim.StepUntil(Txn(txid).Succeeds())
}
