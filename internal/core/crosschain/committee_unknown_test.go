// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestUnknownCommitteeDoesNotSend — this is the anchor path's half of the
// three-valued answer (#4366, #4367).
//
// GlobalValues.MembershipOf gives three answers, not two, and each caller
// decides what UNKNOWN means for it. Here it means DO NOT SEND: a node that
// has not loaded a network definition cannot tell whether it is a validator,
// and an anchor is a proposal carried by a validator signature. The one it
// withholds is sent by the next block, once the globals are loaded. The
// submit path decides the other way — see
// internal/node/dagbft.TestSubmitter_AnUnknownCommitteeDoesNotRefuse — and
// that difference is the reason the shared answer is not a boolean.
func TestUnknownCommitteeDoesNotSend(t *testing.T) {
	for _, part := range []*protocol.PartitionInfo{
		{ID: protocol.Directory, Type: protocol.PartitionTypeDirectory},
		{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator},
	} {
		t.Run(part.ID, func(t *testing.T) {
			const block = 500
			key := acctesting.GenerateKey(t.Name(), part.ID)
			d := new(countingDispatcher)
			c := &Conductor{
				Partition:    part,
				ValidatorKey: key,
				Database:     anchorLogDB(t, part, block),
				Dispatcher:   d,
				RunTask:      func(f func()) { f() },
			}

			records := captureLog(t)
			bus := events.NewBus(nil)
			require.NoError(t, c.Start(bus))

			// Globals that EXIST — the block path returns early on nil
			// globals, so nil would prove nothing about this gate — and
			// name the partitions to anchor to, but list no validators.
			// That is what "unknown" is at this call site, and it is where
			// every node is for the first seconds of its life.
			g := new(network.GlobalValues)
			g.ExecutorVersion = protocol.ExecutorVersionLatest
			g.Network = &protocol.NetworkDefinition{
				NetworkName: "unknown-committee",
				Version:     1,
				Partitions: []*protocol.PartitionInfo{
					{ID: protocol.Directory, Type: protocol.PartitionTypeDirectory},
					{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator},
				},
			}
			require.NoError(t, bus.Publish(events.WillChangeGlobals{New: g}))
			require.Equal(t, network.CommitteeUnknown,
				c.Globals.Load().MembershipOf(
					ed25519.PrivateKey(key).Public().(ed25519.PublicKey), part.ID))

			require.NoError(t, bus.Publish(execute.WillBeginBlock{BlockParams: execute.BlockParams{
				Context: context.Background(),
				Index:   block + 1,
				Time:    time.Now(),
			}}))

			require.Empty(t, anchorLines(records),
				"a node that does not know whether it is a validator logged a send")
			require.Empty(t, d.submissions(),
				"a node that does not know whether it is a validator dispatched an anchor")
		})
	}
}

// TestKnownCommitteeStillSends — the guard on the guard: the same conductor,
// given the globals, sends. Otherwise the test above would pass on a
// conductor that never sends anything.
func TestKnownCommitteeStillSends(t *testing.T) {
	part := &protocol.PartitionInfo{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator}
	records := captureLog(t)
	d := runOneBlockAs(t, part, true)
	require.NotEmpty(t, anchorLines(records))
	require.NotEmpty(t, d.submissions())
}
