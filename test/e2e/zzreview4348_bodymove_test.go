// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

type acctSnap struct {
	chains  map[string]int64
	leaf    [32]byte
	body    string
	pending []string
	dir     []string
}

func snapOf(t *testing.T, db database.Viewer, us []*url.URL) map[string]acctSnap {
	t.Helper()
	out := map[string]acctSnap{}
	require.NoError(t, db.View(func(b *database.Batch) error {
		for _, u := range us {
			s := acctSnap{chains: map[string]int64{}}
			cs, err := b.Account(u).Chains().Get()
			if err != nil {
				return err
			}
			for _, cm := range cs {
				c, err := b.Account(u).ChainByName(cm.Name)
				if err != nil {
					return err
				}
				h, err := c.Head().Get()
				if err != nil {
					return err
				}
				s.chains[cm.Name] = h.Count
			}
			if h, err := b.Account(u).Hash(); err == nil {
				s.leaf = h
			}
			if m, err := b.Account(u).Main().Get(); err == nil && m != nil {
				j, _ := json.Marshal(m)
				s.body = string(j)
			}
			if p, err := b.Account(u).Pending().Get(); err == nil {
				for _, x := range p {
					s.pending = append(s.pending, x.String())
				}
			}
			if d, err := b.Account(u).Directory().Get(); err == nil {
				for _, x := range d {
					s.dir = append(s.dir, x.String())
				}
			}
			out[u.String()] = s
		}
		return nil
	}))
	return out
}

func sameChains(a, b map[string]int64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// TestReview4348_ALeafThatMovesWithNoChainMoving asks the question F5 rests
// on: can an account's state change — its body, its directory, its pending
// list, anything in its BPT leaf — with no chain of that account moving?
//
// If it can, then "every chain equal means the peer's state is as good as the
// node's" is false, and the pull takes the peer's stale state over the node's.
func TestReview4348_ALeafThatMovesWithNoChainMoving(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	key1 := acctesting.GenerateKey(alice, 1)
	key2 := acctesting.GenerateKey(alice, 2)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, key1[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) {
		p.AcceptThreshold = 2
		p.AddKeySpec(&KeySpec{PublicKeyHash: doSha(key2[32:])})
		p.CreditBalance = 1e9
	})
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	sim.StepN(5)

	watch := []*url.URL{
		alice, alice.JoinPath("book"), alice.JoinPath("book", "1"),
		alice.JoinPath("tokens"), bob.JoinPath("tokens"),
		PartitionUrl("BVN0").JoinPath(Ledger),
		PartitionUrl("BVN0").JoinPath(Synthetic),
		PartitionUrl("BVN0").JoinPath(AnchorPool),
	}
	db := sim.DatabaseFor(alice)
	before := snapOf(t, db, watch)

	// One signature of two: the transaction goes pending.
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice, "tokens").
			SendTokens(1, 0).To(bob, "tokens").
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(key1))
	sim.StepUntil(Txn(st.TxID).IsPending())
	sim.StepN(3)

	after := snapOf(t, db, watch)

	moved := 0
	for _, u := range watch {
		a, b := before[u.String()], after[u.String()]
		if a.leaf == b.leaf {
			continue
		}
		if !sameChains(a.chains, b.chains) {
			t.Logf("%v: leaf moved AND a chain moved (%v -> %v)", u, a.chains, b.chains)
			continue
		}
		moved++
		t.Logf("**** %v: LEAF MOVED WITH NO CHAIN MOVING (chains %v)", u, a.chains)
		t.Logf("      body changed: %v", a.body != b.body)
		t.Logf("      pending: %v -> %v", a.pending, b.pending)
		t.Logf("      directory: %v -> %v", a.dir, b.dir)
	}
	t.Logf("accounts whose leaf moved with no chain moving: %d", moved)

	// Now show what that costs the pull: a node that executed the signature,
	// against a peer that stopped before it. Every chain is equal, so the
	// pull is not "past", and the peer's state is taken whole.
	book := alice.JoinPath("book")
	pullOne := func(from pull.Source, into *database.Database, u *url.URL, mode pull.Mode) error {
		b := into.Begin(true)
		defer b.Discard()
		p, err := pull.Fetch(context.Background(), from, b, u,
			pull.Options{Mode: mode, Partition: PartitionUrl("BVN0")}, false)
		if err != nil {
			return err
		}
		if p.Past() {
			return fmt.Errorf("PAST")
		}
		if err := p.Keep(); err != nil {
			return err
		}
		if err := b.UpdateBPT(); err != nil {
			return err
		}
		return b.Commit()
	}
	_ = pullOne
	_ = book
}

func doSha(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}
