// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
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

// A channel (one source partition's synthetic stream into one destination) is
// SICK when it falls greatly behind: its received-but-undelivered backlog
// grows without bound while inflow is sustained, or fails to drain once
// inflow stops. Run 20260824T051249Z made the failure mode concrete: a retry
// storm drove one stream at ~27 synthetics/s while the cascade drained
// cascadeDeliveryWindow per block — a fixed count per block, NOT per second —
// so at the 3s block interval the ceiling was ~10/s and the backlog grew past
// 33,000 with the protocol executing exactly as designed (#4163, #4164).
//
// streamLag reads one destination partition's view of one source's stream.
// streamLag reports how far a stream has been sighted and how far it has been
// delivered — the two halves of where it stands, from the two records that own
// them (#4189). Delivered is the ledger's, because it is what executed;
// how far the stream has been sighted is staging's.
func streamLag(t *testing.T, sim *Sim, dst *url.URL, src *url.URL) (received, delivered uint64) {
	t.Helper()
	require.NoError(t, sim.DatabaseFor(dst).View(func(batch *database.Batch) error {
		ledgerUrl := dst.JoinPath(protocol.Synthetic)
		var ledger *SyntheticLedger
		err := batch.Account(ledgerUrl).Main().GetAs(&ledger)
		if err != nil {
			return err
		}
		delivered = ledger.Partition(src).Delivered

		received, err = execute.Sighted(batch, execute.StreamID{Ledger: ledgerUrl, Source: src})
		if err != nil {
			return err
		}
		if received < delivered {
			received = delivered
		}
		return nil
	}))
	return
}

// TestNoLaggingChannels builds a real BACKLOG on one cross-partition channel
// — the head synthetic is dropped, and hundreds of deposits to DISTINCT
// destination identities pile up pending behind the hole (distinct, so
// delivery must go through the cross-identity cascade, never the
// same-identity inline path) — then requires the backlog to DRAIN at a rate
// far above one message per block once healing fills the hole, and to reach
// zero. In-order arrivals deliver on arrival; it is exactly the backlogged
// channel that exposes the drain ceiling, and a fixed small per-block drain
// is what turned one overloaded stream into a 33,000-message wedge in run
// 20260824T051249Z.
func TestNoLaggingChannels(t *testing.T) {
	var timestamp uint64
	const (
		perBlock = 40 // deposits per block, all distinct identities
		blocks   = 10 // total backlog = perBlock*blocks = 400
	)

	// HOLD the first cross-partition deposit, then release it. Everything
	// after piles up pending behind the hole while it is held.
	//
	// It used to be dropped once and left to healing to re-deliver, and the
	// backlog was assumed to survive the settle window. That only held while
	// delivery was slow: with the backlog draining at run scale (#4169), a
	// single drop is healed and the whole queue drains before the measurement
	// starts, and the test failed on its own precondition with a lag of zero.
	//
	// So the hole is now held open until the test says otherwise. The backlog
	// is built deterministically instead of racing healing, and what is
	// measured — the drain once delivery resumes — is unchanged and is now
	// actually reached.
	//
	// The hook runs on every node's goroutine, so its state is guarded
	// (#4171).
	var holdMu sync.Mutex
	var held *url.TxID
	var released bool
	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWith(GenesisTime, globals),
		simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (bool, error) {
			holdMu.Lock()
			defer holdMu.Unlock()
			if released {
				return true, nil
			}
			messages, err := env.Normalize()
			if err != nil {
				return false, err
			}
			for _, msg := range messages {
			again:
				switch m := msg.(type) {
				case interface{ Unwrap() messaging.Message }:
					msg = m.Unwrap()
					goto again
				case messaging.MessageWithTransaction:
					if m.GetTransaction().Body.Type() != TransactionTypeSyntheticDepositTokens {
						continue
					}
					id := m.GetTransaction().ID()
					if held == nil {
						held = id
					}
					if held.Equal(id) {
						// Keep dropping THIS one, however often healing
						// resubmits it, until the test releases it.
						return false, nil
					}
				}
			}
			return true, nil
		}),
	)

	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	alicePart, err := sim.Router().RouteAccount(aliceUrl)
	require.NoError(t, err)
	MakeLiteTokenAccount(t, sim.DatabaseFor(aliceUrl), alice[32:], AcmeUrl())

	var dsts []*url.URL
	var dstPart string
	for i := 0; len(dsts) < perBlock*blocks; i++ {
		key := acctesting.GenerateKey("Bob", i)
		u := acctesting.AcmeLiteAddressStdPriv(key)
		part, err := sim.Router().RouteAccount(u)
		require.NoError(t, err)
		if part == alicePart {
			continue
		}
		if dstPart == "" {
			dstPart = part
		}
		if part != dstPart {
			continue // concentrate every deposit on ONE channel
		}
		dsts = append(dsts, u)
	}

	srcUrl := protocol.PartitionUrl(alicePart)
	dstUrl := protocol.PartitionUrl(dstPart)

	// Build the backlog: the first deposit is dropped in dispatch, so every
	// following deposit is received out of sequence and parks pending.
	next := 0
	for b := 0; b < blocks; b++ {
		env := build.Transaction().For(aliceUrl).
			SendTokens(1, 0).To(dsts[next])
		next++
		for i := 1; i < perBlock; i++ {
			env = env.And(1, 0).To(dsts[next])
			next++
		}
		sim.SubmitTxnSuccessfully(MustBuild(t,
			env.SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))
		sim.StepN(1)
	}
	holdMu.Lock()
	require.NotNil(t, held, "the head deposit must have been held back")
	holdMu.Unlock()
	sim.StepN(6) // let in-flight dispatches land and the backlog build

	recv, deliv := streamLag(t, sim, dstUrl, srcUrl)
	require.Greater(t, recv-deliv, uint64(2*perBlock),
		"precondition: a large pending backlog must exist behind the hole")

	// Release the hole. Healing re-delivers the held deposit, and everything
	// behind it becomes deliverable.
	holdMu.Lock()
	released = true
	holdMu.Unlock()

	// Healing fills the hole after some blocks — not under the test's
	// control. What IS the protocol's contract: once delivery starts moving,
	// the whole backlog must drain in a handful of blocks, not one small
	// fixed quantum per block. 400 pending at the old 32/block took 13
	// blocks; at one per block, 400. Allow four blocks from first movement.
	start := deliv
	moved := -1
	drained := -1
	for b := 0; b < 120; b++ {
		sim.StepN(1)
		recv, deliv = streamLag(t, sim, dstUrl, srcUrl)
		if moved < 0 && deliv > start {
			moved = b
		}
		if recv == deliv {
			drained = b
			break
		}
	}
	require.GreaterOrEqual(t, moved, 0, "healing never restarted delivery: received=%d delivered=%d", recv, deliv)
	require.GreaterOrEqualf(t, drained, 0,
		"channel %s -> %s never drained: received=%d delivered=%d (lag %d)",
		alicePart, dstPart, recv, deliv, recv-deliv)
	require.LessOrEqualf(t, drained-moved, 4,
		"channel %s -> %s is greatly behind: a %d-message backlog took more than %d blocks to drain after delivery resumed (first movement at block %d, drained at %d) — delivery is quantized instead of scaling with the backlog",
		alicePart, dstPart, perBlock*blocks, 6, moved, drained)

	// And every channel in the network must be clean at the end — a sick
	// channel anywhere is a sick protocol.
	var sick []string
	for _, dp := range []string{"Directory", "BVN0", "BVN1", "BVN2"} {
		du := protocol.PartitionUrl(dp)
		require.NoError(t, sim.DatabaseFor(du).View(func(batch *database.Batch) error {
			ledgerUrl := du.JoinPath(protocol.Synthetic)
			var ledger *SyntheticLedger
			err := batch.Account(ledgerUrl).Main().GetAs(&ledger)
			if err != nil {
				return nil // partition may not exist under this name; skip
			}
			for _, part := range ledger.Sequence {
				// Sighted, not the ledger: how far a stream has been seen is
				// staging's to say (#4189). A stream that never fell behind has
				// staged nothing and reports 0, which is not a lag.
				seen, err := execute.Sighted(batch, execute.StreamID{Ledger: ledgerUrl, Source: part.Url})
				if err != nil {
					return err
				}
				if seen > part.Delivered {
					sick = append(sick, fmt.Sprintf("%v -> %s: sighted=%d delivered=%d",
						part.Url, dp, seen, part.Delivered))
				}
			}
			return nil
		}))
	}
	require.Emptyf(t, sick, "channels greatly behind after quiesce: %v", sick)
}
