// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// The Directory's anchors carry receipts over its slow chains -- the root
// chain and one anchor chain per partition, one entry a block -- and a
// receipt reaches for the mark point below its start, written up to 256
// entries earlier. On the node's store the permanent layer is read through a
// window of 20 blocks; a mark point routed there reads as absent once it is
// older, and a chain that took absent for a truncated chain built its state
// from nothing. Every Directory anchor after the first mark point was then
// rejected at the BVNs ("receipt 0 is invalid: result does not match the
// anchor"), no receipt came back, and no synthetic was dispatched again
// (soaks 20260905T032333Z through 051008Z, frozen at minute 5).
//
// This runs the network on that store for long enough for the anchor chains
// to pass their first mark point and the window to move past it, then proves
// the Directory's anchors are still accepted and a cross-partition deposit
// still lands. The memory store cannot show this; it answers every read from
// all of history.
func TestDirectoryReceiptsPastTheWindow(t *testing.T) {
	if testing.Short() {
		t.Skip("runs ~350 blocks on the BlockchainDB-backed store")
	}
	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 1),
		simulator.GenesisWith(GenesisTime, globals),
		simulator.WithDatabase(simulator.BcdbDbOpener(t.TempDir(), func(err error) { require.NoError(t, err) })),
	)

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

	// The height of the Directory anchor chain at Bob's partition: it grows
	// only while the Directory's anchors are accepted there.
	dnAnchors := func() int64 {
		var count int64
		View(t, sim.DatabaseFor(bobUrl), func(batch *database.Batch) {
			head, err := batch.Account(protocol.PartitionUrl(mustPartition(t, sim, bobUrl)).JoinPath(protocol.AnchorPool)).
				AnchorChain(protocol.Directory).Root().Head().Get()
			require.NoError(t, err)
			count = head.Count
		})
		return count
	}

	// Keep every partition anchoring every block: one cross-partition send a
	// block for 340 blocks. The per-partition anchor chains at the Directory
	// pass their first mark point (256 entries) around block 256; by block
	// 300 that mark point is far behind a 20-block window.
	var timestamp uint64
	for i := 0; i < 340; i++ {
		sim.SubmitTxnSuccessfully(MustBuild(t,
			build.Transaction().For(aliceUrl).
				SendTokens(1, 0).To(bobUrl).
				SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))
		sim.StepN(1)
	}
	before := dnAnchors()
	require.Greater(t, before, int64(250), "the anchor chains are past their first mark point")

	// The Directory's anchors are still accepted -- thirty more anchoring
	// blocks land thirty more of them -- and a deposit still lands.
	for i := 0; i < 30; i++ {
		sim.SubmitTxnSuccessfully(MustBuild(t,
			build.Transaction().For(aliceUrl).
				SendTokens(1, 0).To(bobUrl).
				SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))
		sim.StepN(1)
	}
	require.GreaterOrEqual(t, dnAnchors(), before+25, "the Directory's anchors keep landing at the BVN")
	st := sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(aliceUrl).
			SendTokens(1, 0).To(bobUrl).
			SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))
	sim.StepUntilN(60,
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())
}

func mustPartition(t *testing.T, sim *Sim, u *url.URL) string {
	part, err := sim.Router().RouteAccount(u)
	require.NoError(t, err)
	return part
}
