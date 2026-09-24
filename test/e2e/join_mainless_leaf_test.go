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

// TestJoinFollowsPastAMainlessLeaf is the reproduction for the 32,875 "An
// account could not be pulled ... notFound" lines of run 20260924T052134Z
// (#4205): the load generator's execution-invalid work leaves the peers'
// state tree holding a LEAF for an account that has NO MAIN STATE.
//
//   - A WriteData whose principal does not exist (loadgen fail:data-to-void,
//     fail:sub-adi-on-void) fails at execution, but the authority signature was
//     already recorded on the principal's signature chain
//     (sig_authority.go:134, txn.RecordHistory). The account gets a
//     `signature` and `signature-index` chain, a BPT leaf, and a block ledger
//     entry (acc://alice/ghostdata1, chain signature).
//   - A SendTokens to a token account that does not exist (fail:send-to-void)
//     fails at the destination's synthetic deposit; the destination gets a
//     BPT leaf with no main state, no chains and no pending list, and no block
//     ledger entry -- only the page diff can name it.
//
// Every peer answers such an account notFound (pull.pullMain queries Main), so
// the join refuses it, re-asks it every pass for the life of the process
// (PulledState.refused), and -- the part that matters -- can never put its
// leaf in the local tree. The local root then differs from every root anchored
// after the block that wrote the leaf, and the join never matches again.
//
// The control subtest is identical with the failing work left out, and
// matches. A scratch variant (not kept) ran the void send alone, let the join
// fail to match for 40 rounds, then did nothing but MarkDirty the void account
// in the joining node's store -- putting the same empty-account leaf the peers
// hold into the local tree -- and the join matched within the next rounds
// (block 170 against a target of 50). The missing leaf is the whole of the
// blocker; the re-ask is its symptom, and dropping the name instead of
// re-asking it would leave the join exactly as stuck.
func TestJoinFollowsPastAMainlessLeaf(t *testing.T) {
	t.Run("Control", func(t *testing.T) { joinPastFailedWork(t, false) })
	t.Run("MainlessLeaf", func(t *testing.T) { joinPastFailedWork(t, true) })
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
		// What the peers hold: a leaf, and no main state.
		for _, u := range []*url.URL{ghost, voidTokens} {
			_, err := batch.BPT().Get(record.NewKey("Account", u))
			require.NoError(t, err, "the peer holds no leaf for %v", u)
			_, err = batch.Account(u).Main().Get()
			require.ErrorIs(t, err, errors.NotFound, "%v has main state on the peer", u)
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
	// local root matches, the block-ledger walk runs from the last block the
	// state was synced to and names every account written since -- by design
	// (join.PulledState.localBlock) -- so the ghost is named again by the walk
	// on every fetching pass while the void account, which only the page diff
	// can name, waits for the diff's cadence. Those asks are the walk's, not
	// the refusal loop's: with the fix nothing is refused in this test. Once
	// the root matches, the walk starts past the block that wrote both, and a
	// name still asked after that is being re-asked because it was refused.
	if failing {
		nobodyTokens := nobody.JoinPath("tokens")
		require.NotZero(t, sources.roundsAsked(nobodyTokens),
			"precondition: the name no peer holds a leaf for reached the join")
		ghostBefore, voidBefore := sources.roundsAsked(ghost), sources.roundsAsked(voidTokens)
		nobodyBefore := sources.roundsAsked(nobodyTokens)
		for round := 0; round < 4*staleEveryRounds; round++ {
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

// staleEveryRounds bounds the page diff's cadence in rounds: it runs every
// eighth fetching round (join.staleEvery), and a round that settles a pass
// does not fetch, so a count of rounds several times that covers it.
const staleEveryRounds = 8
