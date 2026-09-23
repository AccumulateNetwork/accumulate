// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"log/slog"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestAJoinOverrunIsRecordedAndResumesTheJoin: a restarted node's join buffer
// overruns — the network committed more blocks than the buffer holds while the
// node was converging — and the join must neither end there nor end silently.
//
// The daemon calls join.Run once and, when it returns an error, logs it and
// never calls it again (cmd/accumulated/run/dagbft.go, the join goroutine).
// So what join.Run does with an overrun is what the node does with it: an
// overrun that ends Run leaves the node collecting and executing nothing until
// a process restart (review e11.4362b.review.2, finding 4). This test drives
// Run the way the daemon does — once — and asks of it only the property:
//
//   - the overrun is recorded, at Warn or above, in the join's log;
//   - the join does not end at the overrun: it hands off, at a block no
//     older than the one the network had reached when collecting started
//     again, and never hands off from the buffer that overran.
//
// How it recovers — resuming the loop, or a recorded failure that is retried
// — is the implementer's choice. The one way it can start a buffer again
// through join.Buffer as it stands is StartCollecting, so that is what the
// buffer below honours once it has overrun; while it has not, a second
// StartCollecting changes nothing, as in production (#4351).
//
// The pull, the peers and the gap check are production: join.PulledState
// through join.QueryPeers with this node's own peer ID excluded (#4303),
// join.APIPeers, and join.ExecutorStage's HasGap over the node's own staging
// and database. The buffer is a stand-in because the simulator has no DAG-BFT
// consensus buffer; it numbers blocks by the peers' ledger, which is what the
// production buffer's groups are.
//
// The helpers below are this file's own, so join_overrun_test.go, which holds
// an earlier copy of this test, can be deleted without touching this one.
func TestAJoinOverrunIsRecordedAndResumesTheJoin(t *testing.T) {
	const joiner = 2

	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
		simulator.BPTHistoryDepth(1024),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(100000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	send := func(ts uint64) {
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	for i := uint64(1); i <= 5; i++ {
		send(i)
	}
	sim.StepN(10)

	part := PartitionUrl("BVN0")
	p := sim.S.Partition("BVN0")
	head := func() uint64 { return partitionBlock(t, p.NodeDatabase(0), part) }

	// The node restarts at R and starts collecting, as the daemon does before
	// the service starts. The network runs on, within what its buffer holds.
	r := partitionBlock(t, p.NodeDatabase(joiner), part)
	p.RestartNode(joiner)
	buf := &resumeBuffer{head: head, capacity: 20, collecting: true, from: r}
	for i := uint64(6); i <= 8; i++ {
		send(i)
	}
	require.LessOrEqual(t, head()-r, buf.capacity,
		"precondition: the buffer has not overrun when the join starts")

	sources := &join.QueryPeers{
		Client:  sim.S.Services(),
		Network: t.Name(),
		Router:  sim.S.Router(),
		Self:    p.NodePeerID(joiner),
	}
	pulled, err := join.NewState(join.StateOptions{
		Partition: part,
		Database:  p.NodeDatabase(joiner),
		Sources:   sources,
	})
	require.NoError(t, err)

	// The network runs on while the node pulls, which is what a join meets.
	// During the first pull it runs past what the buffer holds: the overrun
	// happens while the join is converging.
	state := &resumeSteppingState{State: pulled, step: func() {
		if !buf.BufferOverrun() && buf.restarts == nil {
			sim.StepN(int(buf.capacity) + 1)
			return
		}
		sim.StepN(1)
	}}
	stage := &join.ExecutorStage{
		Settler:  new(resumeSettler),
		Staging:  p.NodeStaging(joiner),
		Database: p.NodeDatabase(joiner),
	}
	logs := new(resumeLogRecords)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	outcome, err := join.Run(ctx, join.Options{
		Partition: "BVN0",
		Buffer:    buf,
		Stage:     stage,
		State:     state,
		Peers:     &join.APIPeers{Partition: "BVN0", Client: sim.S.Services(), Network: t.Name()},
		Logger:    slog.New(logs),
		Retry:     time.Millisecond,
	})

	require.NotZero(t, buf.overranAt, "precondition: the join buffer overran")
	require.NoError(t, err,
		"the join ended at the buffer overrun, and the daemon never runs it again: this node collects and executes nothing until a process restart")
	require.True(t, logs.mentionsOverrun(slog.LevelWarn),
		"the overrun at block %d left no record at Warn or above in the join's log; recorded:\n%s",
		buf.overranAt, logs.String())
	require.Equal(t, join.Joined, outcome)
	require.NotEmpty(t, buf.restarts, "the join handed off without collecting again after the overrun")
	require.Len(t, buf.handoffs, 1, "the join hands off once")
	from := buf.restarts[len(buf.restarts)-1]
	require.GreaterOrEqual(t, buf.handoffs[0], from,
		"the join handed off at block %d, below block %d where collecting started again: the blocks between are in no buffer",
		buf.handoffs[0], from)
	require.Greater(t, buf.handoffs[0], r+buf.capacity,
		"the join resumes from a newer block than the one the overrun passed")
}

// resumeBuffer stands for the DAG service's collecting mode, with a buffer
// that holds capacity blocks. Its blocks are the peers' blocks: from is the
// block collecting started at, so the buffer holds from+1 onward, and once the
// network is more than capacity blocks past from it has overrun.
type resumeBuffer struct {
	mu         sync.Mutex
	head       func() uint64
	capacity   uint64
	collecting bool
	from       uint64
	overran    bool
	overranAt  uint64   // the network's block when the buffer overran
	restarts   []uint64 // the block each collection started again from
	handoffs   []uint64
}

// StartCollecting changes nothing while the buffer holds every block since
// from (#4351). Once it has overrun there is nothing left in it to lose, and
// starting to collect begins a new buffer at the network's block now.
func (b *resumeBuffer) StartCollecting() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.collecting && !b.overrunLocked() {
		return
	}
	b.collecting = true
	b.overran = false
	b.from = b.head()
	b.restarts = append(b.restarts, b.from)
}

func (b *resumeBuffer) Collecting() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.collecting
}

func (b *resumeBuffer) BufferOverrun() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.overrunLocked()
}

func (b *resumeBuffer) overrunLocked() bool {
	if b.collecting && !b.overran && b.head()-b.from > b.capacity {
		b.overran, b.overranAt = true, b.head()
	}
	return b.overran
}

func (b *resumeBuffer) ApplyStaging(load func() error) error { return load() }

// Handoff hands off at q only if the buffer holds every block from q + 1 to
// the network's block: never from a buffer that overran, never below the
// block collecting started at, and never above the blocks committed so far.
func (b *resumeBuffer) Handoff(q uint64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch {
	case !b.collecting:
		return errors.NotAllowed.With("not joining")
	case b.overrunLocked():
		return errors.NotReady.With("the join buffer overran")
	case q < b.from:
		return errors.NotReady.WithFormat("block %d is behind block %d, where this buffer starts", q, b.from)
	case q > b.head():
		return errors.NotReady.WithFormat("block %d has not been committed yet", q)
	}
	b.handoffs = append(b.handoffs, q)
	b.collecting = false
	return nil
}

// resumeSteppingState is the production pull with the network running on after
// every pull.
type resumeSteppingState struct {
	join.State
	step func()
}

func (s *resumeSteppingState) Pull(ctx context.Context) error {
	err := s.State.Pull(ctx)
	s.step()
	return err
}

// resumeSettler stands for the executor's settle: the simulator does not
// expose a node's executor, and settling is not what this test is about.
type resumeSettler struct{ settled []uint64 }

func (s *resumeSettler) SettleStagingAt(q uint64) error {
	s.settled = append(s.settled, q)
	return nil
}

// resumeLogRecords keeps every record the join logs.
type resumeLogRecords struct {
	mu      sync.Mutex
	records []string
	levels  []slog.Level
}

func (h *resumeLogRecords) Enabled(context.Context, slog.Level) bool { return true }
func (h *resumeLogRecords) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *resumeLogRecords) WithGroup(string) slog.Handler            { return h }

func (h *resumeLogRecords) Handle(_ context.Context, r slog.Record) error {
	var sb strings.Builder
	sb.WriteString(r.Level.String() + " " + r.Message)
	r.Attrs(func(a slog.Attr) bool {
		sb.WriteString(" " + a.String())
		return true
	})
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, sb.String())
	h.levels = append(h.levels, r.Level)
	return nil
}

func (h *resumeLogRecords) mentionsOverrun(min slog.Level) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, r := range h.records {
		if h.levels[i] >= min && strings.Contains(strings.ToLower(r), "overr") {
			return true
		}
	}
	return false
}

func (h *resumeLogRecords) String() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return strings.Join(h.records, "\n")
}
