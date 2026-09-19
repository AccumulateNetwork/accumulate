// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
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

type sweepSnap struct {
	leaf   [32]byte
	chains string
	body   [32]byte
	bodyJs string
	dir    string
	pend   string
}

func sweep(t *testing.T, db database.Beginner) map[string]sweepSnap {
	t.Helper()
	out := map[string]sweepSnap{}
	b := db.Begin(false)
	defer b.Discard()
	require.NoError(t, b.ForEachAccount(func(a *database.Account, leaf [32]byte) error {
		s := sweepSnap{leaf: leaf}
		cs, err := a.Chains().Get()
		if err != nil {
			return err
		}
		var names []string
		for _, cm := range cs {
			c, err := a.ChainByName(cm.Name)
			if err != nil {
				return err
			}
			h, err := c.Head().Get()
			if err != nil {
				return err
			}
			names = append(names, fmt.Sprintf("%s=%d", cm.Name, h.Count))
		}
		sort.Strings(names)
		s.chains = fmt.Sprint(names)
		if m, err := a.Main().Get(); err == nil && m != nil {
			j, _ := json.Marshal(m)
			s.body = sha256.Sum256(j)
			s.bodyJs = string(j)
		}
		if d, err := a.Directory().Get(); err == nil {
			s.dir = fmt.Sprint(d)
		}
		if p, err := a.Pending().Get(); err == nil {
			s.pend = fmt.Sprint(p)
		}
		out[a.Url().String()] = s
		return nil
	}))
	return out
}

// TestReview4348_SweepForLeavesThatMoveWithNoChain walks every account of a
// partition, block by block, under ordinary traffic, and reports every account
// whose BPT leaf moved while none of its chains did — and which part of the
// account it was. That is the set F5 assumes is empty.
func TestReview4348_SweepForLeavesThatMoveWithNoChain(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	liteKey := acctesting.GenerateKey("sweep-lite")
	liteId := LiteAuthorityForKey(liteKey[32:], SignatureTypeED25519)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")
	sim.SetRoute(liteId, "BVN0")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(100000))
	MakeAccount(t, sim.DatabaseFor(alice), &TokenIssuer{Url: alice.JoinPath("foo"), Symbol: "FOO", Precision: 1})
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	MakeAccount(t, sim.DatabaseFor(liteId), &LiteIdentity{Url: liteId})
	MakeAccount(t, sim.DatabaseFor(liteId), &LiteTokenAccount{Url: liteId.JoinPath(ACME), TokenUrl: AcmeUrl()})

	db := sim.S.Database("BVN0")
	var ts uint64
	send := func() {
		ts++
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
	}
	issue := func(to *url.URL) {
		ts++
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "foo").
				Body(&IssueTokens{Recipient: to, Amount: *big.NewInt(5)}).
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
	}
	deposit := func() {
		ts++
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(liteId, ACME).
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
	}

	prev := sweep(t, db)
	found := map[string]string{}
	for i := 0; i < 60; i++ {
		switch i % 10 {
		case 2:
			send()
		case 5:
			deposit()
		case 7:
			issue(liteId.JoinPath(alice.ShortString(), "foo"))
		}
		sim.Step()
		now := sweep(t, db)
		for u, n := range now {
			p, ok := prev[u]
			if !ok || p.leaf == n.leaf || p.chains != n.chains {
				continue
			}
			what := ""
			if p.body != n.body {
				what += "body "
				t.Logf("   BODY %v\n     was: %s\n     now: %s", u, p.bodyJs, n.bodyJs)
			}
			if p.dir != n.dir {
				what += "directory "
			}
			if p.pend != n.pend {
				what += "pending "
			}
			if what == "" {
				what = "something else (events/queues/tx records) "
			}
			found[u] = fmt.Sprintf("step %d: %swith chains %s", i, what, n.chains)
		}
		prev = now
	}
	if len(found) == 0 {
		t.Log("no account's leaf moved without one of its chains moving")
	}
	keys := make([]string, 0, len(found))
	for u := range found {
		keys = append(keys, u)
	}
	sort.Strings(keys)
	for _, u := range keys {
		t.Logf("**** %v: %s", u, found[u])
	}
}
