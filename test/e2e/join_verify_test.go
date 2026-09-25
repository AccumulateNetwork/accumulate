// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"math/big"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// bentBy is what the dishonest peer adds to the balance it serves. Any
// difference at all changes the account's hash, which is the point: the
// receipt it serves with it is the true one, so the only thing that can catch
// this is the local root failing to equal any root the Directory anchored.
const bentBy = 7

// bendingSources serves one account's body bent and everything else honestly,
// with the TRUE receipt in both cases.
//
// It wraps a real join.Sources rather than replacing it, so the join runs on
// the production wiring — join.QueryPeers, FindService, one peer addressed by
// ID — and only the bytes one peer puts on the wire for one account differ.
type bendingSources struct {
	inner  join.Sources
	target *url.URL
	bend   bool

	mu     sync.Mutex
	served int
}

func (b *bendingSources) ValidatorsOf(ctx context.Context, partition *url.URL) ([]anchorsrc.Validator, error) {
	return b.inner.ValidatorsOf(ctx, partition)
}

func (b *bendingSources) Querier(partition *url.URL) api.Querier { return b.inner.Querier(partition) }

func (b *bendingSources) For(ctx context.Context, account *url.URL) ([]pull.Source, *url.URL, error) {
	srcs, partition, err := b.inner.For(ctx, account)
	if err != nil || !b.bend || !account.Equal(b.target) {
		return srcs, partition, err
	}
	bent := make([]pull.Source, len(srcs))
	for i, s := range srcs {
		bent[i] = &bendingSource{Source: s, owner: b}
	}
	return bent, partition, nil
}

func (b *bendingSources) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.served++
	return b.served
}

func (b *bendingSources) timesServed() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.served
}

type bendingSource struct {
	pull.Source
	owner *bendingSources
}

// QueryAccount serves the account the peer really holds, with the receipt the
// peer really built for it, and a balance that is not the one that was proven.
func (s *bendingSource) QueryAccount(ctx context.Context, scope *url.URL, query *api.DefaultQuery) (*api.AccountRecord, error) {
	rec, err := s.Source.QueryAccount(ctx, scope, query)
	if err != nil || rec == nil || rec.Account == nil || !scope.Equal(s.owner.target) {
		return rec, err
	}
	acct, ok := rec.Account.(*TokenAccount)
	if !ok {
		return rec, err
	}
	bent := acct.Copy()
	bent.Balance = *new(big.Int).Add(&acct.Balance, big.NewInt(bentBy))
	rec.Account = bent
	s.owner.count()
	return rec, nil
}

// TestJoinDoesNotMatchOnStateThatDoesNotHashIntoTheAnchoredRoot: a peer serves
// alice/tokens with the true receipt and a balance that is not the balance
// that receipt proves, and the join must never match or promote on it.
//
// The join writes what it pulls and proves nothing account by account: its
// one proof is the whole local root equal to a verified anchor's
// StateTreeAnchor (executor spec, "Sync", "The algorithm", step 3; #4438). So
// the bent balance is written -- the assertion this test made before #4438,
// that it is never written, belonged to the per-account proof the algorithm
// retired -- and what the proof must do is keep the node from matching while
// it holds it. The honest arm is the control: the same wiring, the same
// traffic, the same number of rounds, and it matches, so the bent arm's
// "never matched" is not a join that could not match anything.
//
// Every block carries a send, so every block sends an anchor and a match is
// possible at whatever block the pull stands at.
func TestJoinDoesNotMatchOnStateThatDoesNotHashIntoTheAnchoredRoot(t *testing.T) {
	for _, bend := range []bool{true, false} {
		name := "a peer bends a body"
		if !bend {
			name = "control: every peer honest"
		}
		t.Run(name, func(t *testing.T) { joinAgainstABentBody(t, bend) })
	}
}

func joinAgainstABentBody(t *testing.T, bend bool) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
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

	var ts uint64
	step := func() {
		ts++
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.Step()
	}
	for i := 0; i < 20; i++ {
		step()
	}

	ctx := context.Background()
	part := PartitionUrl("BVN0")
	tokens := alice.JoinPath("tokens")

	// The production wiring, with one account's body bent on the wire.
	sources := &bendingSources{
		inner: &join.QueryPeers{
			Client:  sim.S.Services(),
			Network: t.Name(),
			Router:  sim.S.Router(),
		},
		target: tokens,
		bend:   bend,
	}

	// What a joining node really holds before it asks anybody anything: its
	// own genesis network accounts. They are the keys it verifies anchors
	// against (#4301).
	local := genesisOf(t, sim, "BVN0")
	state, err := join.NewState(join.StateOptions{
		Partition: part,
		Database:  local,
		Sources:   sources,
	})
	require.NoError(t, err)

	var matched bool
	for round := 0; round < 30 && !matched; round++ {
		require.NoError(t, state.Pull(ctx), "pull round %d", round)
		step()
		step()
		_, matched, err = state.Matched(ctx)
		require.NoError(t, err)
	}

	if !bend {
		require.True(t, matched, "the control never matched, so the bent arm's refusal to match proves nothing")
		return
	}

	// The join really ran: the dishonest peer was asked, and the honest ones
	// were believed.
	require.NotZero(t, sources.timesServed(),
		"the peer never served the bent account, so nothing was under test")
	View(t, local, func(batch *database.Batch) {
		_, err := batch.Account(bob.JoinPath("tokens")).Main().Get()
		require.NoError(t, err, "the join pulled nothing at all")
	})

	require.False(t, matched,
		"the join matched while holding an account no anchored root accounts for")
	require.NotEqual(t, nodestate.StateActive, state.Machine().State(),
		"the node went ACTIVE on state no anchored root accounts for")
}
