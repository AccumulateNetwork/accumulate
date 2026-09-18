// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator_test

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestBlockLedgerNamesSyntheticEntriesNotJustTheChain asks the question the
// branch's own test does not: the block ledger's contract is a list of
// (account, chain, INDEX) triples, and every consumer -- loadBlockEntry, the
// block query, the event stream -- reads the chain entry at that index. So
// for the producing partition's synthetic chain, does the union of the block
// ledgers' (chain, index) pairs cover the chain's entries?
//
// The workload deliberately puts MORE THAN ONE synthetic message for one
// destination in a single block, which the branch's test does not.
func TestBlockLedgerNamesSyntheticEntriesNotJustTheChain(t *testing.T) {
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

	// Several sends submitted before any step: they execute together, so one
	// block appends several entries to BVN0's synthetic chain to BVN1.
	for i := uint64(1); i <= 6; i++ {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(i).PrivateKey(aliceKey))
	}
	sim.StepN(50)

	p0 := sim.S.Partition("BVN0")
	b0 := p0.Begin(false)
	defer b0.Discard()

	synthURL := PartitionUrl("BVN0").JoinPath(Synthetic)
	ledgerURL := PartitionUrl("BVN0").JoinPath(Ledger)
	sc := b0.Account(synthURL).SyntheticChain("BVN1")
	head, err := sc.Inner().Head().Get()
	require.NoError(t, err)
	require.NotZero(t, head.Count, "the source appended to its synthetic chain")

	// What the block ledgers say about that chain.
	type named struct {
		block uint64
		index uint64
	}
	var namings []named
	acct := b0.Account(ledgerURL)
	for i := uint64(1); i <= sim.S.BlockIndex("BVN0")+2; i++ {
		var bl *database.BlockLedger
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
			if e.Account.Equal(synthURL) && e.Chain == sc.Name() {
				namings = append(namings, named{i, e.Index})
			}
		}
	}

	t.Logf("synthetic chain %s height=%d; named by block ledgers %d times", sc.Name(), head.Count, len(namings))
	for _, n := range namings {
		t.Logf("  block %d names index %d", n.block, n.index)
	}

	// What a reconstruction driven from the block ledger recovers: the chain
	// entry at each named index.
	chainObj, err := sc.Get()
	require.NoError(t, err)
	recovered := map[string]bool{}
	for _, n := range namings {
		h, err := chainObj.Entry(int64(n.index))
		require.NoError(t, err)
		recovered[fmt.Sprintf("%x", h)] = true
	}
	// What is actually on the chain.
	actual := map[string]bool{}
	for i := int64(0); i < head.Count; i++ {
		h, err := chainObj.Entry(i)
		require.NoError(t, err)
		actual[fmt.Sprintf("%x", h)] = true
	}
	t.Logf("reconstruction recovers %d of the chain's %d entries", len(recovered), len(actual))

	// The contract: every entry the partition put on its synthetic chain is
	// named, at its own index, by the ledger of the block that appended it.
	require.Equal(t, len(actual), len(recovered),
		"every synthetic chain entry is reachable from some block ledger's (chain, index)")
	require.Equal(t, int(head.Count), len(namings),
		"one block-ledger naming per chain entry, as for every other chain")
}
