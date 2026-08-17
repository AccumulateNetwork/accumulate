// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// TestAnchorHoleRecoveredByRange exercises the destination-side pull, which
// needs a HOLE rather than a tail.
//
// The distinction is the whole point. A tail — the most recent anchors never
// arriving — is invisible to the destination: its ledger looks like an idle
// stream, so only the source can notice. A hole is different: a later anchor
// arrives and leaves a gap behind it, which is evidence the destination can act
// on. That is when it can pull the missing run with one range request under one
// collection proof, verifying against a root it already holds — no signature
// quorum, and no cost proportional to the size of the gap.
//
// This drops exactly one anchor mid-stream and lets everything after it through,
// then asserts recovery happened AND that it happened by range.
//
// The source-side push is disabled for the duration. Both paths normally run,
// and the push repairs a hole before the destination's next scan even sees it —
// measured while writing this test: every scan observed pending=0. Leaving it
// enabled would make this test pass whether or not the pull works, which is the
// failure mode worth guarding, since the pull is reached through a runtime type
// assertion that can quietly miss.
func TestAnchorHoleRecoveredByRange(t *testing.T) {
	alice := build.
		Identity("alice").Create("book").
		Tokens("tokens").Create("ACME").Add(1e9).Identity().
		Book("book").Page(1).Create().AddCredits(1e9).Book().Identity()
	aliceKey := alice.Book("book").Page(1).
		GenerateKey(SignatureTypeED25519)

	bob := build.
		Identity("bob").Create("book").
		Tokens("tokens").Create("ACME").Identity()

	before := gatherCounter(t, "accumulate_crosschain_heals_total", "type", "anchor-range")

	// Drop one anchor SEQUENCE NUMBER, from every validator.
	//
	// Dropping a single envelope is not enough: each validator sends its own
	// copy of an anchor, so the others still arrive and reach quorum and no gap
	// ever forms. Dropping every copy of number 2, and nothing else, leaves a
	// hole that later anchors expose.
	//
	// Every partition pair has its own stream, so the hole — and the single
	// recovered copy allowed to repair it — is per direction.
	const holeAt = 2
	var dropped atomic.Bool
	var mu sync.Mutex
	recovered := map[string]bool{}
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime).With(alice, bob),
		simulator.DisableAnchorHealing(),
		simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (bool, error) {
			for _, msg := range env.Messages {
				ba, ok := msg.(*messaging.BlockAnchor)
				if !ok {
					continue
				}
				seq, ok := ba.Anchor.(*messaging.SequencedMessage)
				if !ok || seq.Number != holeAt {
					continue
				}
				// Drop every ORIGINAL copy — that is the hole.
				if ba.Proof == nil {
					dropped.Store(true)
					return false, nil // Do not send
				}

				// Let exactly ONE recovered copy through, and drop the rest.
				//
				// Without this the test proves less than it appears to. Every
				// destination validator runs its own conductor, so each pulls the
				// missing anchor and submits its own signed copy — three copies,
				// three signatures, quorum reached, and the anchor executes by the
				// ordinary path whether or not the proof was ever consulted. One
				// copy carries one signature, which is below the threshold of 2,
				// so completion can only mean the collection proof authorized it.
				key := seq.Source.String() + " -> " + seq.Destination.String()
				mu.Lock()
				defer mu.Unlock()
				if recovered[key] {
					return false, nil
				}
				recovered[key] = true
				return true, nil
			}
			return true, nil
		}),
	)

	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "tokens").
			SendTokens(123, 0).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))

	sim.StepUntil(
		Sig(st[1].TxID).Completes(),
		Txn(st[0].TxID).Completes())

	require.True(t, dropped.Load(), "the test never dropped an anchor, so it proved nothing")

	account, err := bob.Tokens("tokens").Load(sim.DatabaseFor)
	require.NoError(t, err)
	require.Equal(t, 123, int(account.Balance.Int64()))

	after := gatherCounter(t, "accumulate_crosschain_heals_total", "type", "anchor-range")
	require.Greater(t, after, before,
		"the hole was filled, but not by a range request — the destination-side "+
			"pull never ran and the source-side push covered for it")
}
