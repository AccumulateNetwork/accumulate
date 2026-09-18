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
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestBlockLedgerSyntheticIndexIsAlwaysZero shows the same defect in the
// shape that misleads a reader rather than merely omitting: with one
// synthetic per block -- exactly the pacing the branch's own test uses --
// every block's ledger names the synthetic chain at index 0, so block N's
// record points at the entry block 1 appended, and the block query answers
// block N with block 1's synthetic transaction.
func TestBlockLedgerSyntheticIndexIsAlwaysZero(t *testing.T) {
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

	for i := uint64(1); i <= 4; i++ {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(i).PrivateKey(aliceKey))
		sim.Step()
	}
	sim.StepN(40)

	p0 := sim.S.Partition("BVN0")
	b0 := p0.Begin(false)
	defer b0.Discard()
	synthURL := PartitionUrl("BVN0").JoinPath(Synthetic)
	sc := b0.Account(synthURL).SyntheticChain("BVN1")
	head, err := sc.Inner().Head().Get()
	require.NoError(t, err)

	acct := b0.Account(PartitionUrl("BVN0").JoinPath(Ledger))
	seenIndex := map[uint64]uint64{} // chain index -> first block that named it
	dupes, total := 0, 0
	for i := uint64(1); i <= sim.S.BlockIndex("BVN0")+2; i++ {
		bl, err := acct.BlockLedger(i).Get()
		switch {
		case errors.Is(err, errors.NotFound):
			continue
		case err != nil:
			require.NoError(t, err)
		}
		if bl == nil {
			continue
		}
		for _, e := range bl.Entries {
			if !e.Account.Equal(synthURL) || e.Chain != sc.Name() {
				continue
			}
			total++
			if first, ok := seenIndex[e.Index]; ok {
				dupes++
				t.Logf("block %d names synthetic index %d, already claimed by block %d", i, e.Index, first)
			} else {
				seenIndex[e.Index] = i
				t.Logf("block %d names synthetic index %d", i, e.Index)
			}
		}
	}
	t.Logf("chain height %d; %d namings, %d repeat an index another block already claimed",
		head.Count, total, dupes)
	require.Zero(t, dupes,
		"two blocks claim the same synthetic chain entry: the block query answers the later block with the earlier block's transaction")
}
