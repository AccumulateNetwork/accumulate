// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestAnAnchorsStateTreeAnchorIsTheRootOfItsBlock pins which block's root a
// signed anchor carries, because the join proves a root by equality with it,
// or by the bpt chain's history from it to the root, held to a later signed
// anchor (executor spec, "Sync", step 2; anchorsrc.ProveRoot).
//
// An anchor for block N is built at the start of block N+1 from the BPT root
// as it then stands (crosschain/anchoring.go), which is the root block N
// committed: the root a peer's state is CURRENT at while its ledger says N.
// The same value is what block_end.go records on the ledger's bpt chain as
// the next non-empty block's PreviousStateHash, so it is also a bpt chain
// entry; empty blocks write nothing and move no root.
//
// The roots are read from the partition's own store after each block, the
// anchors from the Directory's pool through the API, the way a join reads
// them.
func TestAnAnchorsStateTreeAnchorIsTheRootOfItsBlock(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(100000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// The root each partition committed at each block its ledger names.
	partitions := []string{Directory, "BVN0"}
	roots := map[string]map[uint64][32]byte{}
	for _, part := range partitions {
		roots[part] = map[uint64][32]byte{}
	}
	record := func() {
		for _, part := range partitions {
			View(t, sim.Database(part), func(batch *database.Batch) {
				var ledger *SystemLedger
				require.NoError(t, batch.Account(PartitionUrl(part).JoinPath(Ledger)).Main().GetAs(&ledger))
				root, err := batch.GetBptRootHash()
				require.NoError(t, err)
				if prev, ok := roots[part][ledger.Index]; ok {
					require.Equal(t, prev, root, "%s: the root changed while the ledger stayed at %d", part, ledger.Index)
				}
				roots[part][ledger.Index] = root
			})
		}
	}

	// Traffic on most blocks, so the BVN anchors for its own reasons and the
	// heartbeat covers the rest.
	for i := uint64(1); i <= 30; i++ {
		if i%3 != 0 {
			sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(alice, "tokens").
					SendTokens(1, 0).To(bob, "tokens").
					SignWith(alice, "book", "1").Version(1).Timestamp(i).PrivateKey(aliceKey))
		}
		require.NoError(t, sim.S.Step())
		record()
	}
	// Let the last anchors land and the last roots reach the bpt chain.
	for i := 0; i < 10; i++ {
		require.NoError(t, sim.S.Step())
		record()
	}

	// Every anchor in the Directory's pool, read as anchorsrc reads it.
	q := api.Querier2{Querier: sim.S.Services()}
	pool := DnUrl().JoinPath(AnchorPool)
	count, expand := uint64(1000), true
	rec, err := q.QueryMainChainEntries(context.Background(), pool, &api.ChainQuery{
		Name:  "main",
		Range: &api.RangeOptions{Start: 0, Count: &count, Expand: &expand},
	})
	require.NoError(t, err)

	checked := map[string]int{}
	onBptChain := map[string]int{}
	for _, entry := range rec.Records {
		body, ok := entry.Value.Message.Transaction.Body.(AnchorBody)
		if !ok {
			continue
		}
		pa := body.GetPartitionAnchor()
		part, ok := ParsePartitionUrl(pa.Source)
		require.True(t, ok)
		byBlock, ok := roots[part]
		if !ok {
			continue
		}
		root, ok := byBlock[pa.MinorBlockIndex]
		if !ok {
			continue // A block that ran inside genesis, before the loop recorded
		}
		require.Equal(t, root, pa.StateTreeAnchor,
			"%s's anchor for block %d does not carry the root that block committed", part, pa.MinorBlockIndex)
		checked[part]++

		// And it is an entry of the producer's bpt chain once a later
		// non-empty block has run.
		View(t, sim.Database(part), func(batch *database.Batch) {
			var ledger *SystemLedger
			require.NoError(t, batch.Account(PartitionUrl(part).JoinPath(Ledger)).Main().GetAs(&ledger))
			if ledger.Index <= pa.MinorBlockIndex {
				return
			}
			bpt, err := batch.Account(PartitionUrl(part).JoinPath(Ledger)).BptChain().Get()
			require.NoError(t, err)
			_, err = bpt.HeightOf(pa.StateTreeAnchor[:])
			require.NoError(t, err, "%s's anchor for block %d carries a root the bpt chain never recorded", part, pa.MinorBlockIndex)
			onBptChain[part]++
		})
	}
	for _, part := range partitions {
		require.GreaterOrEqual(t, checked[part], 5, "%s: too few anchors were checked against a recorded root", part)
		require.GreaterOrEqual(t, onBptChain[part], 5, "%s: too few anchors were checked against the bpt chain", part)
	}
}
