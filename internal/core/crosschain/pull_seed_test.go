// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain_test

import (
	"bytes"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// The pull pair is drawn from the ledger as the executor writes it, and every
// node draws the same pair for the same block (healing.md, "Who asks, and
// when"; #4415).
//
// The seed used to be the ledger's stored anchor's RootChainAnchor, which the
// executor stores as zeros -- the root chain fields cannot be known until the
// block closes, and only the copy ConstructLastAnchor sends carries them. On a
// busy partition every block anchors, so the seed was 32 zero bytes and the
// same two validators asked for the partition's whole life. TestPullSenders
// fed made-up seeds and TestSelectionAmongTheCommitteeIsUnchanged a ledger
// with no anchor, so neither saw it. This reads each node's own store through
// each node's own conductor, on a partition kept busy so that every block
// anchors.
func TestThePullPairIsDrawnFromTheLedgerAsTheExecutorWritesIt(t *testing.T) {
	const nodes = 4
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, nodes),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	MakeAccount(t, sim.DatabaseFor(alice), &protocol.TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: protocol.AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e9))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &protocol.TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: protocol.AcmeUrl()})

	p := sim.S.Partition("BVN0")
	pairs := map[string]int{}
	anchored, zeroStored := 0, 0
	for ts := uint64(1); ts <= 40; ts++ {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.Step()

		// What the next block's hook will draw from, on every node.
		var seed0 []byte
		var selected []int
		for i := 0; i < nodes; i++ {
			View(t, p.NodeDatabase(i), func(batch *database.Batch) {
				seed, sel, err := p.NodeConductor(i).SelectionAt(batch)
				require.NoError(t, err)
				if i == 0 {
					seed0 = seed
				} else {
					require.Truef(t, bytes.Equal(seed0, seed), "block %d: node %d draws from %x, node 0 from %x", ts, i, seed, seed0)
				}
				if sel {
					selected = append(selected, i)
				}
				if i == 0 {
					var ledger *protocol.SystemLedger
					require.NoError(t, batch.Account(protocol.PartitionUrl("BVN0").JoinPath(protocol.Ledger)).Main().GetAs(&ledger))
					if ledger.Anchor != nil {
						anchored++
						if ledger.Anchor.GetPartitionAnchor().RootChainAnchor == ([32]byte{}) {
							zeroStored++
						}
					}
				}
			})
		}
		require.Lenf(t, selected, 2, "block %d: the nodes must agree on exactly one pair", ts)
		pairs[fmt.Sprint(selected)]++
	}

	t.Logf("pairs drawn over 40 blocks: %v; %d blocks anchored, %d of them with a stored RootChainAnchor of zeros", pairs, anchored, zeroStored)
	require.NotZero(t, anchored, "the partition must anchor, or this does not read the case that failed")
	require.GreaterOrEqual(t, len(pairs), 4, "the pair must rotate from block to block: of six possible pairs of four, only %v were drawn", pairs)
}
