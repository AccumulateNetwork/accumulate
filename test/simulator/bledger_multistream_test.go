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

// TestBlockLedgerAgreesWhenABlockFeedsTwoPartitions pins the order of the
// entries anchorSynthChains adds to the block ledger.
//
// The entries are built from a MAP of streams, and the record they go into is
// marshaled, hashed onto the block-ledger chain and folded into the ledger
// account's hash, which is in the BPT. Two nodes that list the same entries in
// a different order have different state roots and the partition forks. One
// destination per block cannot show that — there is only one order — so this
// workload sends to two partitions in the same block.
func TestBlockLedgerAgreesWhenABlockFeedsTwoPartitions(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	charlie := url.MustParse("charlie")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)
	charlieKey := acctesting.GenerateKey(charlie)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")
	sim.SetRoute(charlie, "BVN2")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(10000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	MakeIdentity(t, sim.DatabaseFor(charlie), charlie, charlieKey[32:])
	MakeAccount(t, sim.DatabaseFor(charlie), &TokenAccount{Url: charlie.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Each round sends to BOTH partitions before stepping, so one block
	// appends to two of BVN0's synthetic chains.
	var ts uint64
	for i := 0; i < 12; i++ {
		for _, to := range []*url.URL{bob, charlie} {
			for j := 0; j < 2; j++ {
				ts++
				sim.BuildAndSubmitTxnSuccessfully(
					build.Transaction().For(alice, "tokens").
						SendTokens(1, 0).To(to, "tokens").
						SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
			}
		}
		sim.Step()
	}
	sim.StepN(60)

	p := sim.S.Partition("BVN0")
	synth := PartitionUrl("BVN0").JoinPath(Synthetic)
	ledgerURL := PartitionUrl("BVN0").JoinPath(Ledger)
	last := sim.S.BlockIndex("BVN0") + 2

	// Read each node's block ledgers as an ORDERED list of what they name for
	// the synthetic account, and its state root.
	read := func(n int) (string, int, [32]byte) {
		var s string
		var multi int
		var root [32]byte
		require.NoError(t, p.NodeDatabase(n).View(func(b *database.Batch) error {
			acct := b.Account(ledgerURL)
			for i := uint64(1); i <= last; i++ {
				bl, err := acct.BlockLedger(i).Get()
				switch {
				case errors.Is(err, errors.NotFound):
					continue
				case err != nil:
					return err
				}
				if bl == nil {
					continue
				}
				chains := map[string]bool{}
				for _, e := range bl.Entries {
					if !e.Account.Equal(synth) {
						continue
					}
					s += fmt.Sprintf("%d:%s#%d ", i, e.Chain, e.Index)
					chains[e.Chain] = true
				}
				if len(chains) > 1 {
					multi++
				}
			}
			var err error
			root, err = b.GetBptRootHash()
			return err
		}))
		return s, multi, root
	}

	first, multi, root0 := read(0)
	t.Logf("BVN0 node 0 names: %s", first)
	t.Logf("BVN0 node 0 state root %x; %d blocks appended to more than one synthetic chain", root0, multi)
	require.NotZero(t, multi,
		"precondition: some block appended to two different synthetic chains, or there is only one order to get right")

	for n := 1; n < p.NodeCount(); n++ {
		s, _, root := read(n)
		require.Equal(t, first, s, "BVN0 node %d lists the block ledger's synthetic entries in a different order", n)
		require.Equal(t, root0, root, "BVN0 node %d disagrees with node 0 on the state root", n)
	}
}
