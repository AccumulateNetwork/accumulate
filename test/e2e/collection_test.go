// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestPackageAheadOfItsAnchor_IsCollectedThenDelivered: a synthetic package
// that reaches its destination before the Directory anchor that proves it is
// COLLECTED — held in staging at its number, sighted but not delivered — and
// executes when the anchor lands (executor spec, "Collection", "Anchor
// staging"). Before #4217 it was recorded pending outside staging and only the
// healer could bring it back.
//
// The anchor is kept away by dropping every Directory anchor addressed to the
// destination partition, healing resends included, until the collection is
// observed; then the drop lifts and anchor healing delivers the anchor.
func TestPackageAheadOfItsAnchor_IsCollectedThenDelivered(t *testing.T) {
	var timestamp uint64
	var dropping atomic.Bool
	var dropped atomic.Int32
	var dest atomic.Pointer[url.URL]

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
		simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (send bool, err error) {
			d := dest.Load()
			if !dropping.Load() || d == nil {
				return true, nil
			}
			messages, err := env.Normalize()
			if err != nil {
				return false, err
			}
			for _, msg := range messages {
				anchor, ok := msg.(*messaging.BlockAnchor)
				if !ok {
					continue
				}
				seq, ok := anchor.Anchor.(*messaging.SequencedMessage)
				if !ok || seq.Destination == nil || !seq.Destination.Equal(d) {
					continue
				}
				txn, ok := seq.Message.(*messaging.TransactionMessage)
				if !ok || txn.Transaction.Body.Type() != TransactionTypeDirectoryAnchor {
					continue
				}
				dropped.Add(1)
				return false, nil
			}
			return true, nil
		}),
	)

	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	alicePart, err := sim.Router().RouteAccount(aliceUrl)
	require.NoError(t, err)
	var bobUrl *url.URL
	var bobPart string
	for i := 0; ; i++ {
		bob := acctesting.GenerateKey("Bob", i)
		bobUrl = acctesting.AcmeLiteAddressStdPriv(bob)
		bobPart, err = sim.Router().RouteAccount(bobUrl)
		require.NoError(t, err)
		if alicePart != bobPart {
			break
		}
	}
	dest.Store(protocol.PartitionUrl(bobPart))
	MakeLiteTokenAccount(t, sim.DatabaseFor(aliceUrl), alice[32:], AcmeUrl())

	// From here the destination gets no Directory anchors, so the deposit's
	// package arrives before the anchor that proves it.
	dropping.Store(true)
	st := sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(aliceUrl).
			SendTokens(1, protocol.AcmePrecisionPower).To(bobUrl).
			SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))

	src, dst := protocol.PartitionUrl(alicePart), protocol.PartitionUrl(bobPart)
	sim.StepUntilN(50, True(func(*Harness) bool {
		received, delivered := streamLag(t, sim, dst, src)
		return received > delivered
	}))
	received, delivered := streamLag(t, sim, dst, src)
	require.Greater(t, received, delivered, "the package is held in staging, not delivered, while its anchor is missing")
	require.Greater(t, dropped.Load(), int32(0), "the anchor really was kept away")

	// Let the anchor through. Healing resends it; the waiting proof validates;
	// the collected deposit executes in its stream.
	dropping.Store(false)
	sim.StepUntilN(300,
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())
	lta := GetAccount[*LiteTokenAccount](t, sim.DatabaseFor(bobUrl), bobUrl)
	require.Equal(t, int(protocol.AcmePrecision), int(lta.Balance.Uint64()))
}
