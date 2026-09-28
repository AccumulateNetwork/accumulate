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
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// provenFixture is a BVN0 whose node 1 restarted 30 blocks behind a partition
// that changes in every block: a send within BVN0 (alice to carol) and one to
// BVN1 (alice to bob), every block.
type provenFixture struct {
	name    string
	sim     *Sim
	p       *simulator.Partition
	part    *url.URL
	carol   *url.URL
	traffic func()
	sts     []*TransactionStatus

	// early is carol's account state as its peers served it before node 1
	// restarted.
	early *api.AccountState
}

const provenJoiner = 1

func newProvenFixture(t *testing.T, opts ...simulator.Option) *provenFixture {
	return newProvenFixtureOn(t, "BVN0", opts...)
}

// newProvenFixtureOn is the fixture with the node that restarts on the named
// partition.
func newProvenFixtureOn(t *testing.T, partition string, opts ...simulator.Option) *provenFixture {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	carol := url.MustParse("carol")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t, append([]simulator.Option{
		simulator.SimpleNetwork(t.Name(), 2, 3),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
	}, opts...)...)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(carol, "BVN0")
	sim.SetRoute(bob, "BVN1")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	MakeIdentity(t, sim.DatabaseFor(carol), carol, acctesting.GenerateKey(carol)[32:])
	MakeAccount(t, sim.DatabaseFor(carol), &TokenAccount{Url: carol.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	f := &provenFixture{name: partition, sim: sim, p: sim.S.Partition(partition), part: PartitionUrl(partition), carol: carol.JoinPath("tokens")}
	var ts uint64
	f.traffic = func() {
		for _, to := range []*url.URL{carol, bob} {
			ts++
			f.sts = append(f.sts, sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(alice, "tokens").
					SendTokens(1, 0).To(to, "tokens").
					SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey)))
		}
		sim.Step()
	}
	for i := 0; i < 10; i++ {
		f.traffic()
	}
	rec, err := sim.Query().QueryAccountStates(context.Background(), f.part, &api.AccountStatesQuery{Accounts: []*url.URL{f.carol}})
	require.NoError(t, err)
	require.Len(t, rec.States, 1)
	f.early = rec.States[0]
	f.p.RestartNode(provenJoiner)
	for i := 0; i < 30; i++ {
		f.traffic()
	}
	f.p.RestartNode(provenJoiner)
	return f
}

// join runs the production join for node 1 -- join.Run with the simulator
// node's buffer, the executor's stage and the node's own join state -- with
// traffic in every round, until the node is ACTIVE, stop says so, or
// maxRounds have passed. It returns what join.Run returned.
func (f *provenFixture) join(t *testing.T, maxRounds int, stop func(*provenState) bool) (*provenState, error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := &provenState{}
	st.steppingState = steppingState{cancel: cancel, step: func(round int) {
		f.traffic()
		if round >= maxRounds || stop != nil && stop(st) {
			cancel()
		}
	}}
	st.State = f.p.NodeJoinState(provenJoiner)
	settler, ok := f.p.NodeExecutor(provenJoiner).(join.Settler)
	require.True(t, ok)
	_, err := join.Run(ctx, join.Options{
		Partition: f.name,
		Buffer:    f.p.NodeJoin(provenJoiner),
		Stage:     &join.ExecutorStage{Settler: settler, Staging: f.p.NodeStaging(provenJoiner), Database: f.p.NodeDatabase(provenJoiner), Partition: f.part},
		State:     st,
		Peers:     &join.APIPeers{Partition: f.name, Client: f.sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	return st, err
}

// requireInAgreement steps the network on and requires every node of BVN0 on
// one root chain.
func (f *provenFixture) requireInAgreement(t *testing.T) {
	t.Helper()
	for i := 0; i < 10; i++ {
		f.traffic()
	}
	for _, s := range f.sts {
		f.sim.StepUntil(Txn(s.TxID).Succeeds())
	}
	f.sim.StepN(10)
	var anchors [][]byte
	for i := 0; i < f.p.NodeCount(); i++ {
		View(t, f.p.NodeDatabase(i), func(batch *database.Batch) {
			a, err := batch.Account(f.part.JoinPath(Ledger)).RootChain().Anchor()
			require.NoError(t, err)
			anchors = append(anchors, a)
		})
	}
	for i := 1; i < len(anchors); i++ {
		require.Equal(t, anchors[0], anchors[i], "BVN0 node %d's root chain differs from node 0's after the join", i)
	}
}

// TestAJoinUnderLoadIsProvenAtVBeforeItExecutes is #4442: a follower joining a
// partition that moves in every block, and moves again on every account the
// join asks for, is proven at a block V -- its root equal to V's signed
// anchor, once the walk has covered the tree and every block-ledger record
// through V is applied -- and executes nothing before that proof. It hands off
// at V, ACTIVE there, and executes V + 1 onward in agreement with its peers.
//
// A join that handed off from the pulled state unproven (the Ready path)
// reached the handoff BOOTING and was promoted, if ever, by the root watch at
// a block it had already executed.
func TestAJoinUnderLoadIsProvenAtVBeforeItExecutes(t *testing.T) {
	f := newProvenFixture(t)

	// The partition moves on every account the join asks the peers of: the
	// accounts the join takes by their window or whole -- the ledger, the
	// anchor pool, the synthetic ledger -- are read at a block after the
	// records' view.
	f.p.SetPullHook(provenJoiner, f.traffic)
	st, err := f.join(t, 150, nil)
	f.p.SetPullHook(provenJoiner, nil)

	require.NotEmpty(t, st.handoffs, "the join never handed off")
	h := st.handoffs[0]
	require.Equal(t, nodestate.StateActive, h.State,
		"the join handed off at block %d unproven: it executes before its state is proven", h.block)
	require.Equal(t, h.block, h.SinceBlock, "the join is proven at a block other than the one it handed off at")
	require.Zero(t, st.diverged, "a block executed after the proof diverged")
	require.NoError(t, err)
	f.requireInAgreement(t)
}

// TestAJoinDoesNotExecuteABlockWhoseRecordsBeforeItAreUnapplied is #4442's
// second rule (Paul, 2026-09-28): "You can't execute any txs at height n until
// we have processed all blockledger accounts and have the complete BPT for
// n-1." The network moves every time the join reads the partition ledger, so
// the ledger it holds always names a block the records have not reached. The
// join hands off at q only once every record through q is applied.
//
// The Ready path handed off at the block the pulled ledger named, with the
// records of the blocks before it still unapplied.
func TestAJoinDoesNotExecuteABlockWhoseRecordsBeforeItAreUnapplied(t *testing.T) {
	f := newProvenFixture(t)
	ledger := f.part.JoinPath(Ledger)
	f.p.SetSourceWrap(provenJoiner, func(srcs []pull.Source) []pull.Source {
		out := make([]pull.Source, len(srcs))
		for i, s := range srcs {
			out[i] = movingLedger{s, ledger, f.traffic}
		}
		return out
	})
	st, err := f.join(t, 150, nil)
	f.p.SetSourceWrap(provenJoiner, nil)

	require.NotEmpty(t, st.handoffs, "the join never handed off")
	for _, h := range st.handoffs {
		require.GreaterOrEqual(t, h.records, h.block,
			"the join handed off at block %d, executing %d, with the records applied only through %d", h.block, h.block+1, h.records)
		require.Equal(t, nodestate.StateActive, h.State, "the join handed off at block %d unproven", h.block)
	}
	require.NoError(t, err)
	f.requireInAgreement(t)
}

// TestAMismatchAtVNamesTheAccountServedWrong is #4442's third step: a pull
// fault -- the peers' answers carry carol's account as it was before the node
// restarted, while the records name it in every block -- leaves the
// local root off V's. The join walks the peers' trees as of V against its own
// (#4441) and names exactly carol's account, with the leaf it holds and the
// leaf the peer holds.
func TestAMismatchAtVNamesTheAccountServedWrong(t *testing.T) {
	f := newProvenFixture(t, simulator.BPTHistoryDepth(4096))
	stale := &staleAccount{account: f.carol, first: f.early}
	f.p.SetSourceWrap(provenJoiner, func(srcs []pull.Source) []pull.Source {
		out := make([]pull.Source, len(srcs))
		for i, s := range srcs {
			out[i] = staleSource{s, stale}
		}
		return out
	})
	state := f.p.NodeJoinState(provenJoiner)
	_, err := f.join(t, 60, func(*provenState) bool {
		_, diffs := state.Mismatches()
		return len(diffs) > 0
	})
	if err != nil {
		// Stopped before it joined, as the test asked.
		require.ErrorContains(t, err, context.Canceled.Error())
	}
	f.p.SetSourceWrap(provenJoiner, nil)

	v, diffs := state.Mismatches()
	require.NotZero(t, v, "the join never compared a complete state with a signed root")
	require.Len(t, diffs, 1, "the mismatch at %d names %v", v, diffs)
	d := diffs[0]
	require.NotNil(t, d.Account, "the mismatch names no account")
	require.True(t, d.Account.Equal(f.carol), "the mismatch names %v, not %v", d.Account, f.carol)
	require.Equal(t, [32]byte(record.NewKey("Account", f.carol).Hash()), d.KeyHash)
	require.Equal(t, stale.leaf(), d.Local, "the local leaf is not the one the stale answer carried")
	require.NotEqual(t, d.Local, d.Peer)
}

// provenState records the node's state and the records the join had applied
// at every handoff, and every divergence the root watch reports.
type provenState struct {
	steppingState
	handoffs []handoffAt
	diverged int
}

type handoffAt struct {
	block   uint64
	records uint64
	nodestate.Advertisement
}

func (s *provenState) HandedOff(q uint64) {
	m := s.State.(interface{ Machine() *nodestate.Machine }).Machine()
	records := s.State.(interface{ RecordsThrough() uint64 }).RecordsThrough()
	s.handoffs = append(s.handoffs, handoffAt{q, records, m.Get()})
	s.steppingState.HandedOff(q)
}

func (s *provenState) Diverged(ctx context.Context) (uint64, bool, error) {
	n, d, err := s.steppingState.Diverged(ctx)
	if d {
		s.diverged++
	}
	return n, d, err
}

// movingLedger steps the network before every read of the partition ledger.
type movingLedger struct {
	pull.Source
	ledger *url.URL
	move   func()
}

func (m movingLedger) String() string { return sourceName(m.Source) }

func (m movingLedger) QueryAccount(ctx context.Context, u *url.URL, q *api.DefaultQuery) (*api.AccountRecord, error) {
	if u.Equal(m.ledger) {
		m.move()
	}
	return m.Source.QueryAccount(ctx, u, q)
}

func (m movingLedger) QueryChainEntries(ctx context.Context, u *url.URL, q *api.ChainQuery) (*api.RecordRange[*api.ChainEntryRecord[api.Record]], error) {
	if u.Equal(m.ledger) {
		m.move()
	}
	return m.Source.QueryChainEntries(ctx, u, q)
}

// staleAccount is an old answer for one account, served in place of every
// answer for it.
type staleAccount struct {
	mu      sync.Mutex
	account *url.URL
	first   *api.AccountState
}

func (s *staleAccount) replace(rec *api.AccountStatesRecord) *api.AccountStatesRecord {
	if rec == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := *rec
	out.States = make([]*api.AccountState, len(rec.States))
	for i, st := range rec.States {
		out.States[i] = st
		if st == nil || st.Url == nil || !st.Url.Equal(s.account) || st.NoLeaf {
			continue
		}
		if s.first == nil {
			s.first = st
		}
		out.States[i] = s.first
	}
	return &out
}

func (s *staleAccount) leaf() [32]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.first == nil {
		return [32]byte{}
	}
	return s.first.LeafHash
}

// staleSource serves one account as it was, every time.
type staleSource struct {
	pull.Source
	stale *staleAccount
}

func (s staleSource) String() string { return sourceName(s.Source) }

func (s staleSource) QueryChangedAccountStates(ctx context.Context, scope *url.URL, q *api.ChangedAccountStatesQuery) (*api.AccountStatesRecord, error) {
	rec, err := s.Source.QueryChangedAccountStates(ctx, scope, q)
	return s.stale.replace(rec), err
}

func (s staleSource) QueryAccountStates(ctx context.Context, scope *url.URL, q *api.AccountStatesQuery) (*api.AccountStatesRecord, error) {
	rec, err := s.Source.QueryAccountStates(ctx, scope, q)
	return s.stale.replace(rec), err
}

func sourceName(s pull.Source) string {
	if n, ok := s.(interface{ String() string }); ok {
		return n.String()
	}
	return "a peer"
}

// TestAnExecutedBlockThatMissesNamesTheAccountItDiffersIn is #4442's follow-up
// from the Docker run 20260928T051402Z: followers proven at V missed a few
// blocks later, and the root watch said only which block. The watch now
// locates the accounts the executed block's root differs in, as the match at V
// does: the node's tree as of that block, from the history it retains,
// against a peer's as of the same block (#4441). Here the follower's copy of
// carol's account is changed behind its executor's back after the handoff,
// and the watch names carol, and nothing else.
func TestAnExecutedBlockThatMissesNamesTheAccountItDiffersIn(t *testing.T) {
	f := newProvenFixture(t, simulator.BPTHistoryDepth(4096))
	_, err := f.join(t, 150, nil)
	require.NoError(t, err)
	state := f.p.NodeJoinState(provenJoiner)
	require.Equal(t, nodestate.StateActive, state.Machine().State(), "precondition: the node joined")

	// carol's balance, on the follower only.
	db := f.p.NodeDatabase(provenJoiner)
	b := db.Begin(true)
	var acct *TokenAccount
	require.NoError(t, b.Account(f.carol).Main().GetAs(&acct))
	acct.Balance.Add(&acct.Balance, big.NewInt(1000))
	require.NoError(t, b.Account(f.carol).Main().Put(acct))
	require.NoError(t, b.Commit())

	diverged := false
	for i := 0; i < 20 && !diverged; i++ {
		f.traffic()
		_, diverged, err = state.Diverged(context.Background())
		require.NoError(t, err)
	}
	require.True(t, diverged, "precondition: the follower's root left its peers'")
	n, diffs := state.Mismatches()
	require.NotZero(t, n)
	require.Len(t, diffs, 1, "the executed block %d's miss names %v", n, diffs)
	require.NotNil(t, diffs[0].Account)
	require.True(t, diffs[0].Account.Equal(f.carol), "the miss names %v, not %v", diffs[0].Account, f.carol)
}
