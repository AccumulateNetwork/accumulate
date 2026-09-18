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
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestBlockQueryReportsTheWrongSyntheticTransaction drives the same data
// through the public block query, which is what the block ledger exists to
// answer (executor.md, "The block ledger"). Every producing block after the
// first reports the FIRST block's synthetic transaction as its own.
func TestBlockQueryReportsTheWrongSyntheticTransaction(t *testing.T) {
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

	ctx := context.Background()
	part := PartitionUrl("BVN0")
	q := api.Querier2{Querier: sim.S.Services()}
	synthURL := part.JoinPath(Synthetic)

	seen := map[[32]byte]uint64{}
	repeats := 0
	for block := uint64(1); block <= sim.S.BlockIndex("BVN0"); block++ {
		b := block
		rec, err := q.QueryMinorBlock(ctx, part, &api.BlockQuery{Minor: &b})
		if err != nil {
			if errors.Is(err, errors.NotFound) {
				continue
			}
			require.NoError(t, err)
		}
		if rec == nil || rec.Entries == nil {
			continue
		}
		for _, e := range rec.Entries.Records {
			if e.Account == nil || !e.Account.Equal(synthURL) {
				continue
			}
			t.Logf("block %d: query reports synthetic chain %s index %d entry %x",
				block, e.Name, e.Index, e.Entry[:4])
			if first, ok := seen[e.Entry]; ok {
				repeats++
				t.Logf("   ^ that is the entry block %d already reported", first)
			} else {
				seen[e.Entry] = block
			}
		}
	}
	require.Zero(t, repeats,
		"the block query reported the same synthetic transaction as the content of more than one block")
}
