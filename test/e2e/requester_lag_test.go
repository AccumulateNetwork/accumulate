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
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// A destination whose executor runs thirty blocks behind its consensus sees
// every package late, and its staging shows nothing above Delivered while the
// packages sit in its own backlog. The requester asks for the span above
// Delivered -- that is the rule, and it cannot tell a backlog from a loss --
// and the source serves what it dispatched more than a few blocks ago, so the
// destination receives some entries twice. Staging discards the duplicate:
// state stays exact, every deposit lands once, and a dropped package is
// healed. What is bounded is the asking: once per patience, never once per
// activation.
func TestRequester_LaggingDestination(t *testing.T) {
	const delay = 30                       // blocks the destination's executor runs behind
	const deposits = 80                    // one a block
	const healPatience, healCadence = 3, 4 // the requester's, in activations and blocks

	run := func(t *testing.T, dropOne bool) {
		var timestamp uint64
		var dropped, droppedEntries int
		var dropMu sync.Mutex
		globals := new(core.GlobalValues)
		globals.ExecutorVersion = ExecutorVersionLatest
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 2, 1),
			simulator.GenesisWith(GenesisTime, globals),
			simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (send bool, err error) {
				if !dropOne {
					return true, nil
				}
				dropMu.Lock()
				defer dropMu.Unlock()
				if dropped > 0 {
					return true, nil
				}
				n := 0
				for _, m := range env.Messages {
					if syn, ok := m.(*messaging.SyntheticMessage); ok {
						if seq, ok := syn.Message.(*messaging.SequencedMessage); ok {
							if tx, ok := seq.Message.(*messaging.TransactionMessage); ok &&
								tx.Transaction.Body.Type() == TransactionTypeSyntheticDepositTokens {
								n++
							}
						}
					}
				}
				if n == 0 {
					return true, nil
				}
				dropped, droppedEntries = 1, n
				return false, nil
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
		MakeLiteTokenAccount(t, sim.DatabaseFor(aliceUrl), alice[32:], AcmeUrl())

		// Bob's partition executes everything `delay` blocks late: the hook
		// holds each block's envelopes and releases the oldest once `delay`
		// blocks are held. What it holds is the executor's backlog.
		dst := sim.S.Partition(bobPart)
		var holdMu sync.Mutex
		var held [][]*messaging.Envelope
		dst.SetBlockHook(func(_ execute.BlockParams, envelopes []*messaging.Envelope) ([]*messaging.Envelope, bool) {
			holdMu.Lock()
			defer holdMu.Unlock()
			held = append(held, envelopes)
			if len(held) <= delay {
				return nil, true
			}
			out := held[0]
			held = held[1:]
			return out, true
		})

		for i := 0; i < deposits; i++ {
			sim.SubmitTxnSuccessfully(MustBuild(t,
				build.Transaction().For(aliceUrl).
					SendTokens(1, 0).To(bobUrl).
					SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))
			sim.StepN(1)
		}
		if dropOne {
			require.Equal(t, 1, dropped, "one package was dropped")
		}

		// The backlog drains, the lag clears, and every deposit lands -- the
		// dropped one through healing, after the notice.
		balance := func() uint64 {
			var bal uint64
			err := sim.DatabaseFor(bobUrl).View(func(batch *database.Batch) error {
				var lta *LiteTokenAccount
				err := batch.Account(bobUrl).Main().GetAs(&lta)
				switch {
				case err == nil:
					bal = lta.Balance.Uint64()
					return nil
				case errors.Is(err, errors.NotFound):
					return nil // the deposit that creates the account has not landed
				default:
					return err
				}
			})
			require.NoError(t, err)
			return bal
		}
		sim.StepUntilN(400, True(func(*Harness) bool { return balance() == deposits }))
		require.Equal(t, uint64(deposits), balance(), "every deposit landed")

		heals := dst.Heals()
		if dropOne {
			require.GreaterOrEqual(t, heals.Synthetic.Load(), uint64(droppedEntries), "the dropped package was healed")
		}
		// Asked at most once per patience over the whole run, for one stream.
		blocks := uint64(deposits + 400)
		require.LessOrEqual(t, heals.Requests.Load(), blocks/(healPatience*healCadence)+2, "asked at most once per patience")
		require.Equal(t, uint64(deposits), balance(), "state is exact: duplicates were discarded")
	}

	t.Run("nothing dropped", func(t *testing.T) { run(t, false) })
	t.Run("one package dropped", func(t *testing.T) { run(t, true) })
}
