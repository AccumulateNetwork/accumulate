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
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
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

// bentBy is what the dishonest peer adds to the balance it serves. Any
// difference at all changes the account's hash, which is the point: the
// receipt it serves with it is the true one, so the only thing that can catch
// this is the verifier asking whether the state served hashes into the root
// the receipt ends at.
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

	mu     sync.Mutex
	served int
}

func (b *bendingSources) Querier(partition *url.URL) api.Querier { return b.inner.Querier(partition) }

func (b *bendingSources) For(ctx context.Context, account *url.URL) ([]pull.Source, *url.URL, error) {
	srcs, partition, err := b.inner.For(ctx, account)
	if err != nil || !account.Equal(b.target) {
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

// TestJoinRefusesStateThatDoesNotHashIntoTheAnchoredRoot is the assertion the
// join had no test for (#4318): NOTHING A PEER SERVED THAT DOES NOT HASH INTO
// THE ANCHORED ROOT IS EVER WRITTEN.
//
// The verification surface is one call — internal/node/join/state.go,
// a.pending.Settle(root) in settleBatch. Package pull has adversarial tests, but
// they drive pull.Account and pull.Settle directly and hold the verifier up on
// their own, so replacing that one call with pull.Pending.Keep — which writes
// what a peer served without verifying it — left the entire suite green,
// TestJoinPullsFromPeersAndPromotes included. The whole of
// internal/core/bootstrap/pull/verify.go was dead to its only caller and
// nothing noticed.
//
// So this test does not check that a verifier works. It checks that the JOIN
// calls one: a peer serves alice/tokens with the true receipt and a balance
// that is not the balance that receipt proves, and the join must neither write
// it nor promote on it. #4310 and #4301 both rest on "every account is still
// verified against the anchored root"; this is what makes that a property of
// the code rather than of nobody having deleted the line.
func TestJoinRefusesStateThatDoesNotHashIntoTheAnchoredRoot(t *testing.T) {
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
	for i := 0; i < 3; i++ {
		ts++
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	sim.StepN(20)

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
	}

	local := emptyDb()
	state, err := join.NewState(join.StateOptions{
		Partition: part,
		Database:  local,
		Sources:   sources,
	})
	require.NoError(t, err)

	var promoted bool
	for round := 0; round < 10 && !promoted; round++ {
		require.NoError(t, state.Pull(ctx), "pull round %d", round)
		sim.StepN(3)
		_, promoted, err = state.Matched(ctx)
		require.NoError(t, err)
	}

	// The join really ran: the dishonest peer was asked, and the honest ones
	// were believed. Without this the assertions below would hold of a join
	// that did nothing at all.
	require.NotZero(t, sources.timesServed(),
		"the peer never served the bent account, so nothing was under test")
	View(t, local, func(batch *database.Batch) {
		_, err := batch.Account(bob.JoinPath("tokens")).Main().Get()
		require.NoError(t, err, "the join pulled nothing at all, so it refused nothing")
	})

	// The two things a join must never do with state that does not hash into
	// the root the Directory anchored.
	require.False(t, promoted,
		"the join promoted while holding an account no anchored root accounts for")
	require.NotEqual(t, nodestate.StateActive, state.Machine().State(),
		"the node went ACTIVE on state it did not verify")

	View(t, local, func(batch *database.Batch) {
		var acct *TokenAccount
		err := batch.Account(tokens).Main().GetAs(&acct)
		if errors.Is(err, errors.NotFound) {
			return // refused and never written, which is the whole ask
		}
		require.NoError(t, err)

		// If it is there at all it must be the state that was proven, never
		// the state that was served.
		var want *TokenAccount
		View(t, sim.DatabaseFor(alice), func(peer *database.Batch) {
			require.NoError(t, peer.Account(tokens).Main().GetAs(&want))
		})
		require.Equal(t, 0, acct.Balance.Cmp(&want.Balance),
			"the join wrote a balance no peer proved: %v, and the peers hold %v",
			&acct.Balance, &want.Balance)
	})
}
