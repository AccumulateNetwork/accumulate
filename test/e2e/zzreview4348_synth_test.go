// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestReview4348_TheSyntheticLedgerBody — <partition>/synthetic's body is the
// sequence numbers that decide what executes next. F5 takes the peer's body
// whenever every chain is level. Does that body move while its chains stand
// still?
func TestReview4348_TheSyntheticLedgerBody(t *testing.T) {
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
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	part := PartitionUrl("BVN0")
	synth := part.JoinPath(Synthetic)
	db := sim.S.Database("BVN0")

	snap := func() (string, map[string]int64, [32]byte) {
		b := db.Begin(false)
		defer b.Discard()
		var body string
		if m, err := b.Account(synth).Main().Get(); err == nil && m != nil {
			j, _ := json.Marshal(m)
			body = string(j)
		}
		ch := map[string]int64{}
		cs, err := b.Account(synth).Chains().Get()
		require.NoError(t, err)
		for _, cm := range cs {
			c, err := b.Account(synth).ChainByName(cm.Name)
			require.NoError(t, err)
			h, err := c.Head().Get()
			require.NoError(t, err)
			ch[cm.Name] = h.Count
		}
		leaf, _ := b.Account(synth).Hash()
		return body, ch, leaf
	}

	var ts uint64
	send := func() {
		ts++
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds())
	}

	bodyOnly := 0
	pb, pc, pl := snap()
	for i := 0; i < 40; i++ {
		if i%7 == 3 {
			send()
		}
		sim.Step()
		nb, nc, nl := snap()
		if nl != pl && sameChains(pc, nc) {
			bodyOnly++
			t.Logf("**** step %d: the synthetic ledger's LEAF moved with no chain moving (chains %v)", i, nc)
			if nb != pb {
				t.Logf("      body: %s", pb)
				t.Logf("        ->: %s", nb)
			}
		}
		pb, pc, pl = nb, nc, nl
	}
	t.Logf("steps where the synthetic ledger moved with no chain moving: %d", bodyOnly)
	_ = database.Batch{}
}
