// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
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
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// TestSyntheticTailRecoveredByCollectionProof proves that a synthetic message
// can be recovered against an anchor the destination already holds from the
// source, with the directory taking no part.
//
// That is the generalization, and it was never anchor-specific: holding a
// validated state of ANY chain proves every entry added before it. An anchor
// from S commits to S's root chain, which commits to S's synthetic chains too —
// so a destination holding one validated anchor from S can verify S's earlier
// synthetic messages by replay, against a root it already trusts.
//
// What is dropped here is a TAIL, not a hole: adding credits burns ACME, and
// that burn is the only message on the BVN->DN stream, so nothing later ever
// arrives to expose its absence. Every gap-based check reports healthy — the
// ledger of a stream that lost its only message is indistinguishable from an
// idle one. Only reconcileInboundStreams finds it, by asking the source what it
// has produced for us and comparing (#4073). So this covers both halves: the
// reconcile notices, and the collection proof makes the answer verifiable.
//
// The test is arranged so that nothing else can account for a pass. Every
// original copy of the message is dropped, and so is any recovery carrying an
// INDIVIDUAL receipt — the per-message form, which is continued to a
// directory-anchored root. The only submission allowed through is one carrying a
// collection proof.
func TestSyntheticTailRecoveredByCollectionProof(t *testing.T) {
	alice := build.
		Identity("alice").Create("book").
		Tokens("tokens").Create("ACME").Add(1e9).Identity().
		Book("book").Page(1).Create().AddCredits(1e9).Book().Identity()
	aliceKey := alice.Book("book").Page(1).
		GenerateKey(SignatureTypeED25519)

	before := gatherCounter(t, "accumulate_crosschain_heals_total", "type", "synthetic-range")

	// The burn is sequence 1 on the BVN->DN stream. Dropped from every validator.
	const holeAt = 1
	var mu sync.Mutex
	var source *url.URL
	var dropped, recovered bool
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime).With(alice),
		simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (bool, error) {
			for _, msg := range env.Messages {
				var proof *AnnotatedReceipt
				var seq *messaging.SequencedMessage
				switch m := msg.(type) {
				case *messaging.SyntheticMessage:
					proof, seq = m.Proof, asSequenced(m.Message)
				case *messaging.BadSyntheticMessage:
					proof, seq = m.Proof, asSequenced(m.Message)
				default:
					continue
				}
				if seq == nil || seq.Number != holeAt || !DnUrl().LocalTo(seq.Destination) {
					continue
				}

				mu.Lock()
				defer mu.Unlock()
				source = seq.Source

				// A collection proof — the path under test. Let it through.
				if proof != nil && proof.ReceiptList != nil {
					recovered = true
					return true, nil
				}

				// Everything else is either the original or a per-message heal
				// continued to a directory root. Both are dropped, so neither can
				// stand in for the path being tested.
				dropped = true
				return false, nil
			}
			return true, nil
		}),
	)

	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "tokens").
			AddCredits().Spend(10).To(alice, "book", "1").WithOracle(InitialAcmeOracle).
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	sim.StepUntil(
		Sig(st[1].TxID).Completes())

	// Long enough to clear reconcileGraceBlocks — a gap must persist before the
	// reconcile believes it, or it would race normal delivery.
	sim.StepN(150)

	mu.Lock()
	src, didDrop, didRecover := source, dropped, recovered
	mu.Unlock()

	require.True(t, didDrop, "the test never dropped a synthetic message, so it proved nothing")
	require.NotNil(t, src)
	require.True(t, didRecover, "nothing was ever recovered under a collection proof")

	// And it was accepted, not merely submitted: the directory's ledger for that
	// source has to have moved past the message that was dropped.
	var ledger *SyntheticLedger
	require.NoError(t, sim.DatabaseFor(DnUrl()).View(func(batch *database.Batch) error {
		return batch.Account(DnUrl().JoinPath(Synthetic)).Main().GetAs(&ledger)
	}))
	require.GreaterOrEqual(t, ledger.Partition(src).Delivered, uint64(holeAt),
		"the message was recovered and submitted but never delivered")

	after := gatherCounter(t, "accumulate_crosschain_heals_total", "type", "synthetic-range")
	require.Greater(t, after, before,
		"the tail was filled, but not by a range request under a collection proof")
}

func asSequenced(msg messaging.Message) *messaging.SequencedMessage {
	seq, _ := msg.(*messaging.SequencedMessage)
	return seq
}
