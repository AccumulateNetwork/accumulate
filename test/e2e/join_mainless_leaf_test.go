// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"math/big"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
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

// countingSources counts, per round, how often the join asks for each account.
type countingSources struct {
	inner join.Sources

	mu    sync.Mutex
	round int
	asked map[string]map[int]int // account -> round -> asks

	// inject, when set, is named by the peers' block ledger while injecting
	// is true (namingQuerier).
	inject    *url.URL
	injecting bool
}

func (s *countingSources) For(ctx context.Context, account *url.URL) ([]pull.Source, *url.URL, error) {
	s.mu.Lock()
	k := strings.ToLower(account.String())
	if s.asked[k] == nil {
		s.asked[k] = map[int]int{}
	}
	s.asked[k][s.round]++
	s.mu.Unlock()
	return s.inner.For(ctx, account)
}

func (s *countingSources) ValidatorsOf(ctx context.Context, partition *url.URL) ([]anchorsrc.Validator, error) {
	return s.inner.ValidatorsOf(ctx, partition)
}

func (s *countingSources) Querier(partition *url.URL) api.Querier {
	q := s.inner.Querier(partition)
	if s.inject == nil {
		return q
	}
	return &namingQuerier{Querier: q, s: s}
}

// namingQuerier is a peer whose block ledger names one account more than the
// block holds -- one no peer has a leaf for -- while s.injecting is set. It
// is the peer's word the join takes a block ledger on (#4310), and it is how
// a name every source answers NotFound for reaches the join through the
// production walk.
type namingQuerier struct {
	api.Querier
	s *countingSources
}

func (q *namingQuerier) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	r, err := q.Querier.Query(ctx, scope, query)
	bq, ok := query.(*api.BlockQuery)
	if err != nil || !ok || bq.Minor == nil || bq.EntryRange == nil || bq.EntryRange.Start != 0 {
		return r, err
	}
	q.s.mu.Lock()
	injecting := q.s.injecting
	q.s.mu.Unlock()
	mb, ok := r.(*api.MinorBlockRecord)
	if !injecting || !ok || mb.Entries == nil {
		return r, err
	}
	mb.Entries.Records = append(mb.Entries.Records, &api.ChainEntryRecord[api.Record]{
		Account: q.s.inject, Name: "main"})
	mb.Entries.Total++
	return mb, nil
}

func (s *countingSources) roundsAsked(u *url.URL) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.asked[strings.ToLower(u.String())])
}

// TestJoinFollowsPastAMainlessLeaf: a join follows a network executing the
// load generator's execution-invalid work — a WriteData to a principal that
// does not exist, and a SendTokens to a token account that does not exist.
//
// It began as the reproduction for run 20260924T052134Z's 32,875 "An account
// could not be pulled ... notFound" lines (#4397): that work used to leave the
// peers' state tree holding a leaf for an account with no main state (the
// WriteData's authority signature recorded on the missing principal's
// signature chain; the failed deposit clearing its votes and payments on the
// missing destination), which no peer could serve with a body and the join
// could never put in its own tree. Since #4437 neither write gives the account
// a leaf (executor spec, invariant 13), so the peers hold none, and the join
// matches past the failed work exactly as the control does without it.
func TestJoinFollowsPastAMainlessLeaf(t *testing.T) {
	t.Run("Control", func(t *testing.T) { joinPastFailedWork(t, false) })
	t.Run("FailedWork", func(t *testing.T) { joinPastFailedWork(t, true) })
}

func joinPastFailedWork(t *testing.T, failing bool) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	void := url.MustParse("void-9aac09e22e861b50")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")
	sim.SetRoute(void, "BVN0")
	nobody := url.MustParse("nobody-4397")
	sim.SetRoute(nobody, "BVN0")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	ts := uint64(0)
	send := func(to *url.URL) *TransactionStatus {
		ts++
		return sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(to).
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
	}
	for i := 0; i < 3; i++ {
		st := send(bob.JoinPath("tokens"))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	sim.StepN(20)

	ctx := context.Background()
	part := PartitionUrl("BVN0")
	sources := &countingSources{
		inner: &join.QueryPeers{Client: sim.S.Services(), Network: t.Name(), Router: sim.S.Router()},
		asked: map[string]map[int]int{},
	}
	local := genesisOf(t, sim, "BVN0")
	state, err := join.NewState(join.StateOptions{Partition: part, Database: local, Sources: sources})
	require.NoError(t, err)

	// Phase one: the ordinary join, to a node that holds the partition.
	var matched bool
	for round := 0; round < 40 && !matched; round++ {
		sources.round++
		require.NoError(t, state.Pull(ctx), "pull round %d", round)
		sim.StepN(3)
		_, matched, err = state.Matched(ctx)
		require.NoError(t, err)
	}
	require.True(t, matched, "phase one never matched, so phase two has nothing to start from")

	// From here the peers' block ledger also names an account no peer has a
	// leaf for, until the join matches: every source answers it NotFound.
	if failing {
		sources.mu.Lock()
		sources.inject, sources.injecting = nobody.JoinPath("tokens"), true
		sources.mu.Unlock()
	}

	// Phase two: the network executes the load generator's invalid work --
	// or, in the control, only valid work -- and the join must follow it.
	ghost := alice.JoinPath("ghostdata1")
	voidTokens := void.JoinPath("tokens")
	if failing {
		ts++
		st := sim.BuildAndSubmitTxnSuccessfully(build.Transaction().For(ghost).
			WriteData().Entry(&DoubleHashDataEntry{Data: [][]byte{[]byte("loadgen-void")}}).
			SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Fails())
		st = send(voidTokens)
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Fails())
	}
	st := send(bob.JoinPath("tokens"))
	sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	sim.StepN(5)

	var target uint64
	View(t, sim.S.Database("BVN0"), func(batch *database.Batch) {
		var ledger *SystemLedger
		require.NoError(t, batch.Account(part.JoinPath(Ledger)).Main().GetAs(&ledger))
		target = ledger.Index

		if !failing {
			return
		}
		// What the peers hold: no main state and, since #4437, no leaf.
		// Before it, both held a leaf hashing to nothing, and the join had
		// to pull a leaf with no body.
		for _, u := range []*url.URL{ghost, voidTokens} {
			_, err := batch.Account(u).Main().Get()
			require.ErrorIs(t, err, errors.NotFound, "%v has main state on the peer", u)
			_, err = batch.BPT().Get(record.NewKey("Account", u))
			require.ErrorIs(t, err, errors.NotFound, "the peer holds a leaf for %v, which does not exist (#4437)", u)
		}
	})

	var block uint64
	first := sources.round + 1
	for round := 0; round < 40 && block < target; round++ {
		sources.round++
		require.NoError(t, state.Pull(ctx), "phase two pull round %d", round)
		sim.StepN(3)
		var ok bool
		block, ok, err = state.Matched(ctx)
		require.NoError(t, err)
		if !ok {
			block = 0
		}
	}
	rounds := sources.round - first + 1
	sources.mu.Lock()
	sources.injecting = false
	sources.mu.Unlock()

	if failing {
		t.Logf("phase two ran %d rounds; %v was asked for in %d of them, %v in %d",
			rounds, ghost, sources.roundsAsked(ghost), voidTokens, sources.roundsAsked(voidTokens))
		View(t, local, func(batch *database.Batch) {
			for _, u := range []*url.URL{ghost, voidTokens} {
				_, err := batch.BPT().Get(record.NewKey("Account", u))
				t.Logf("local leaf for %v: %v", u, err)
			}
		})
	}

	// The cause: the peers' tree has a leaf the pull cannot fetch, so the
	// local root never equals an anchored root at or after the block that
	// wrote it.
	require.GreaterOrEqual(t, block, target,
		"the join never matched a block at or after %d, where the network's state holds a leaf "+
			"for an account with no main state that every peer answers notFound", target)

	// The symptom: a name every peer answers notFound is asked again every
	// pass, for the life of the process. Answered, it is not asked again.
	//
	// Counted from the match, not over phase two (#4397). Until the whole
	// local root matches, the records name every account each block writes,
	// the ghost among them, and those asks are the records', not a refusal
	// loop's: with the fix nothing is refused in this test. Once the root has
	// matched past the block that wrote both, a name still asked after that
	// is being re-asked because it was refused.
	if failing {
		nobodyTokens := nobody.JoinPath("tokens")
		require.NotZero(t, sources.roundsAsked(nobodyTokens),
			"precondition: the name no peer holds a leaf for reached the join")
		ghostBefore, voidBefore := sources.roundsAsked(ghost), sources.roundsAsked(voidTokens)
		nobodyBefore := sources.roundsAsked(nobodyTokens)
		for round := 0; round < afterMatchRounds; round++ {
			sources.round++
			require.NoError(t, state.Pull(ctx), "after-match pull round %d", round)
			sim.StepN(3)
		}
		require.Equal(t, ghostBefore, sources.roundsAsked(ghost),
			"%v was asked again after the join matched past the block that wrote it", ghost)
		require.Equal(t, voidBefore, sources.roundsAsked(voidTokens),
			"%v was asked again after the join matched past the block that wrote it", voidTokens)

		// The drop rule (#4397, clause 3): a name every source answers
		// NotFound for is dropped, so once nothing names it again it is never
		// asked again. Refused instead, it is asked at the front of every pass
		// for the life of the process.
		require.Equal(t, nobodyBefore, sources.roundsAsked(nobodyTokens),
			"%v, which every peer answers NotFound, was asked again in %d rounds after nothing named it",
			nobodyTokens, sources.roundsAsked(nobodyTokens)-nobodyBefore)
	}
}

// afterMatchRounds is how many rounds after the match the test watches for a
// name asked again. What is owed is asked once a round, so a refused name
// would show in every one of them.
const afterMatchRounds = 32
