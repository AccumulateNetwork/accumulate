// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestCascadeConvergence pins the #4153 fix end to end. A run of deposits to
// ONE destination identity is made pending by dropping the head of the
// sequence; when healing fills the gap, the whole pending tail is the same
// identity, so it must drain INLINE — the entire run delivered in the single
// block that fills the gap.
//
// The bug compared the pending ID's account (the local partition URL) against
// the message's principal, so for user synthetics the "same identity" branch
// was dead and the tail drained one message per block through the cascade
// queue — which never converges once inflow reaches one message per block.
//
// The signal is timing-independent: whatever block healing chooses to
// redeliver the head, that block must deliver the WHOLE tail. So the maximum
// number of deposits Bob receives in any single block distinguishes the two
// worlds — many-at-once (fixed) versus at most one (bug) — without depending
// on how many blocks healing takes to act.
func TestCascadeConvergence(t *testing.T) {
	var timestamp uint64
	const transfers = 8

	// Drop exactly the FIRST synthetic deposit, once. Every later deposit then
	// arrives out of sequence and piles up pending behind the gap.
	var didDrop bool

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWith(GenesisTime, globals),

		simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (bool, error) {
			if didDrop {
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
					if m.GetTransaction().Body.Type() == TransactionTypeSyntheticDepositTokens {
						didDrop = true
						return false, nil
					}
				}
			}
			return true, nil
		}),
	)

	// Alice and Bob on different partitions so the deposits are cross-partition
	// synthetics to a single destination identity (Bob).
	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	alicePart, err := sim.Router().RouteAccount(aliceUrl)
	require.NoError(t, err)
	var bobUrl *url.URL
	for i := 0; ; i++ {
		bob := acctesting.GenerateKey("Bob", i)
		bobUrl = acctesting.AcmeLiteAddressStdPriv(bob)
		bobPart, err := sim.Router().RouteAccount(bobUrl)
		require.NoError(t, err)
		if alicePart != bobPart {
			break
		}
	}
	MakeLiteTokenAccount(t, sim.DatabaseFor(aliceUrl), alice[32:], AcmeUrl())

	// Submit one per block so each deposit is produced and dispatched
	// separately — the head is a clean singleton to drop, and 2..N arrive as
	// distinct out-of-sequence messages rather than one package.
	st := make([]*protocol.TransactionStatus, transfers)
	for i := range st {
		st[i] = sim.SubmitTxnSuccessfully(MustBuild(t,
			build.Transaction().For(aliceUrl).
				SendTokens(1, protocol.AcmePrecisionPower).To(bobUrl).
				SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))
		sim.StepN(2)
	}
	sim.StepUntil(True(func(*Harness) bool { return didDrop }))

	bobBalance := func() int64 {
		var bal int64
		require.NoError(t, sim.DatabaseFor(bobUrl).View(func(batch *database.Batch) error {
			var lta *LiteTokenAccount
			err := batch.Account(bobUrl).Main().GetAs(&lta)
			switch {
			case err == nil:
				bal = int64(lta.Balance.Uint64())
			case errors.Is(err, errors.NotFound):
				bal = 0 // no deposit has created the account yet
			default:
				return err
			}
			return nil
		}))
		return bal
	}

	// Step one block at a time, watching Bob's balance. Healing will redeliver
	// the dropped head after some blocks; the block that does must also drain
	// the pending tail, so the largest single-block jump reveals inline drain.
	var maxJump, prev int64
	prev = bobBalance()
	for i := 0; i < 400 && bobBalance() < transfers*int64(protocol.AcmePrecision); i++ {
		sim.StepN(1)
		cur := bobBalance()
		if cur-prev > maxJump {
			maxJump = cur - prev
		}
		prev = cur
	}

	require.Equal(t, transfers*int64(protocol.AcmePrecision), bobBalance(),
		"every deposit must arrive — the backlog converges")

	// Under the bug the cascade advances one message per block and the gap
	// heal resubmits only the single missing head, so no block can ever
	// deliver more than two deposits (one cascaded, one healed). The inline
	// drain delivers the whole piled tail in the block that fills the gap, so
	// a jump of three or more is reachable only with the fix.
	require.Greater(t, maxJump, 2*int64(protocol.AcmePrecision),
		"a same-identity pending tail must drain INLINE in one block (#4153); a per-block cascade caps any block at two deposits")
}
