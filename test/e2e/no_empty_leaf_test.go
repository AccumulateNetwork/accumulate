// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// The state tree holds a leaf only for an account that exists (executor
// spec, invariant 13; #4437). A transaction that failed against a missing
// principal used to leave a leaf hashing to nothing, because clearing the
// transaction's votes and payments on the principal marked it dirty and block
// close inserted every dirty account.

// noEmptyLeafNetwork is two BVNs with alice (BVN0) funded, carol on BVN0 and
// bob on BVN1, so a block can carry a same-partition send, a cross-partition
// send and a send to an account that does not exist.
func noEmptyLeafNetwork(t *testing.T) (*Sim, *url.URL, []byte) {
	alice, bob, carol := url.MustParse("alice"), url.MustParse("bob"), url.MustParse("carol")
	aliceKey := acctesting.GenerateKey(alice)
	sim := NewSim(t, simulator.SimpleNetwork(t.Name(), 2, 1), simulator.Genesis(GenesisTime))
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(carol, "BVN0")
	sim.SetRoute(bob, "BVN1")
	sim.SetRoute(url.MustParse("void"), "BVN0")
	sim.SetRoute(url.MustParse("nowhere"), "BVN1")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	MakeIdentity(t, sim.DatabaseFor(carol), carol, acctesting.GenerateKey(carol)[32:])
	MakeAccount(t, sim.DatabaseFor(carol), &TokenAccount{Url: carol.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	sim.StepN(5)
	return sim, alice, aliceKey
}

func send(sim *Sim, alice *url.URL, key []byte, ts uint64, to *url.URL) {
	sim.BuildAndSubmit(build.Transaction().For(alice, "tokens").SendTokens(1, 0).To(to).
		SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(key))
}

func TestAFailedSendToAMissingAccountLeavesNoLeaf(t *testing.T) {
	sim, alice, key := noEmptyLeafNetwork(t)

	// Same partition and across partitions: the deposit fails on the
	// destination either way.
	missing := []*url.URL{url.MustParse("void/tokens"), url.MustParse("nowhere/tokens")}
	for i, to := range missing {
		send(sim, alice, key, uint64(i+1), to)
	}
	sim.StepN(20)

	for _, u := range missing {
		View(t, sim.DatabaseFor(u), func(batch *database.Batch) {
			_, err := batch.Account(u).Main().Get()
			require.ErrorIs(t, err, errors.NotFound, "precondition: %v does not exist", u)

			h, err := batch.BPT().Get(batch.Account(u).Key())
			require.ErrorIsf(t, err, errors.NotFound,
				"a failed send to %v left a state-tree leaf %x for an account that does not exist", u, h)
		})
	}
}

func TestEveryStateTreeLeafIsAnAccountThatExists(t *testing.T) {
	sim, alice, key := noEmptyLeafNetwork(t)

	ts := uint64(100)
	for step := 0; step < 30; step++ {
		for _, to := range []string{"bob/tokens", "carol/tokens", "void/tokens", "nowhere/tokens"} {
			ts++
			send(sim, alice, key, ts, url.MustParse(to))
		}
		sim.StepN(1)
	}
	sim.StepN(20)

	for _, part := range []string{"BVN0", "BVN1", "Directory"} {
		View(t, sim.Database(part), func(batch *database.Batch) {
			var leaves, empty int
			err := bpt.ForEach(batch.BPT(), func(k *record.Key, _ []byte) error {
				if k.Len() < 2 || k.Get(0) != "Account" {
					return nil
				}
				u, ok := k.Get(1).(*url.URL)
				if !ok {
					return nil
				}
				leaves++
				_, err := batch.Account(u).Main().Get()
				if errors.Is(err, errors.NotFound) {
					empty++
					t.Errorf("%s: the state tree holds a leaf for %v, which has no main state", part, u)
					return nil
				}
				return err
			})
			require.NoError(t, err)
			require.NotZero(t, leaves, "precondition: %s's state tree has account leaves", part)
		})
	}
}
