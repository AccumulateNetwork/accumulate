// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestSyntheticChainIsAnchoredExactlyOncePerBlock asserts the invariant the
// comment at synthetic.go:79 protects by argument alone: a synthetic chain
// gets ONE root-chain anchor and ONE index-chain entry per block. Two would
// leave the root position a block's proofs are built from pointing at the
// second of two anchors, and the index chain with two entries carrying the
// same block index, which a search by block can no longer resolve uniquely.
//
// Nothing else in the tree tests this. The block-ledger tests catch the
// mistake only when it ALSO duplicates the naming; drop the naming from
// anchorSynthChains at the same time and every one of them goes green while
// the chain is anchored twice a block.
func TestSyntheticChainIsAnchoredExactlyOncePerBlock(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 3),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(10000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	var ts uint64
	for i := 0; i < 5; i++ {
		for j := 0; j < 2; j++ {
			ts++
			sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(alice, "tokens").
					SendTokens(1, 0).To(bob, "tokens").
					SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		}
		sim.Step()
	}
	sim.StepN(40)

	p0 := sim.S.Partition("BVN0")
	b0 := p0.Begin(false)
	defer b0.Discard()

	sc := b0.Account(PartitionUrl("BVN0").JoinPath(Synthetic)).SyntheticChain("BVN1")
	head, err := sc.Inner().Head().Get()
	require.NoError(t, err)
	require.NotZero(t, head.Count)

	ic, err := sc.Index().Get()
	require.NoError(t, err)
	perBlock := map[uint64]int{}
	for i := int64(0); i < ic.Height(); i++ {
		raw, err := ic.Entry(i)
		require.NoError(t, err)
		ie := new(IndexEntry)
		require.NoError(t, ie.UnmarshalBinary(raw))
		perBlock[ie.BlockIndex]++
		t.Logf("index entry %d: block %d source %d anchor %d", i, ie.BlockIndex, ie.Source, ie.Anchor)
	}
	require.NotEmpty(t, perBlock)
	for blk, n := range perBlock {
		require.Equal(t, 1, n,
			"block %d anchored the synthetic chain %d times; one chain, one anchor, one index entry per block", blk, n)
	}
}
