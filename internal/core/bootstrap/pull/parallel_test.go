// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	v3impl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// countingSource is one peer that counts the pages asked of it and how many
// are in flight at once, across every countingSource sharing the gauge.
type countingSource struct {
	*dbSource
	name     string
	pages    atomic.Int64
	inflight *atomic.Int64
	most     *atomic.Int64
	busy     func() bool
	fail     error // answered to every page, when set: a peer that cannot be reached
}

func (s *countingSource) QueryChainEntries(ctx context.Context, u *url.URL, q *api.ChainQuery) (*api.RecordRange[*api.ChainEntryRecord[api.Record]], error) {
	if s.fail != nil {
		return nil, s.fail
	}
	if s.busy != nil && s.busy() {
		return nil, errors.NotReady.With("query capacity exhausted, retry later")
	}
	s.pages.Add(1)
	n := s.inflight.Add(1)
	for {
		m := s.most.Load()
		if n <= m || s.most.CompareAndSwap(m, n) {
			break
		}
	}
	time.Sleep(2 * time.Millisecond) // long enough for pages to overlap
	defer s.inflight.Add(-1)
	return s.dbSource.QueryChainEntries(ctx, u, q)
}

// TestAChainsPagesAreFetchedInParallelAcrossThePeers — #4529, Paul
// 2026-10-05: "fetch in parallel/batched". An account taken whole or by its
// window into the node's database has its chains' pages asked for
// parallelPages at a time, each of the next peer, and ends held whole and
// verified against the peer's head; a peer that answers busy is passed for
// the next.
func TestAChainsPagesAreFetchedInParallelAcrossThePeers(t *testing.T) {
	const n = 3000
	u := protocol.DnUrl().JoinPath("anchors")
	src := newObservedDB(t)
	buildChain(t, src, u, n, 0x51)

	var inflight, most atomic.Int64
	var peers []Source
	var counting []*countingSource
	for i := 0; i < 3; i++ {
		c := &countingSource{dbSource: &dbSource{db: src}, name: string(rune('a' + i)), inflight: &inflight, most: &most}
		counting = append(counting, c)
		peers = append(peers, c)
	}
	// The source answers busy on every third ask: those pages spill to the
	// next peers, as they do against a validator's query gate.
	var asks atomic.Int64
	counting[0].busy = func() bool { return asks.Add(1)%3 == 0 }

	dst := newObservedDB(t)
	ctx := WithPeers(context.Background(), peers, 0)
	err := Account(ctx, peers[0], nil, u, Options{Mode: ModeFullSpine, Store: dst})
	require.NoError(t, err)
	requireWhole(t, src, dst, u)
	require.GreaterOrEqual(t, most.Load(), int64(2), "the pages were asked for one at a time")
	require.Positive(t, counting[0].pages.Load(), "the source was never asked")
	require.Positive(t, counting[1].pages.Load()+counting[2].pages.Load(), "no page the source answered busy went to the next peer")
}

// TestAMixedStreamThatDoesNotReplayIsTakenAgainFromTheSourceAlone — #4532.
// With a chain's pages spread across the peers, one that serves another
// chain poisons the stream: the entries never replay to the head. The stream
// is then taken again from the source alone, and the honest source serves the
// account; asked first, the liar is refused as a source and the next is tried.
func TestAMixedStreamThatDoesNotReplayIsTakenAgainFromTheSourceAlone(t *testing.T) {
	const n = 3000
	u := protocol.DnUrl().JoinPath("anchors")
	src := newObservedDB(t)
	buildChain(t, src, u, n, 0x52)
	other := newObservedDB(t)
	buildChain(t, other, u, n, 0x53)
	honest := &dbSource{db: src}
	liar := &mixedSource{pagedSource: newPagedSource(src), other: &dbSource{db: other}, below: n}

	dst := newObservedDB(t)
	ctx := WithPeers(context.Background(), []Source{honest, liar}, 0)
	require.NoError(t, Account(ctx, honest, nil, u, Options{Mode: ModeFullSpine, Store: dst}))
	requireWhole(t, src, dst, u)

	dst2 := newObservedDB(t)
	p, i, err := FetchFrom(context.Background(), []Source{liar, honest}, nil, u, Options{Mode: ModeFullSpine, Store: dst2})
	require.NoError(t, err)
	require.Equal(t, 1, i, "the liar was not refused as a source")
	require.NoError(t, p.Keep())
	requireWhole(t, src, dst2, u)
}

// TestASignatureChainsWindowLeavesItsMessagesToTheBackfill — #4532: a pool's
// signature chain is three quarters of a window's pages. Its window is taken
// by its entries alone, recorded as a messages-only gap, and the backfill
// brings the messages once the node is ACTIVE.
func TestASignatureChainsWindowLeavesItsMessagesToTheBackfill(t *testing.T) {
	const n = 1000
	u := protocol.DnUrl().JoinPath("anchors")
	src := newObservedDB(t)
	b := src.Begin(true)
	require.NoError(t, b.Account(u).Main().Put(&protocol.DataAccount{Url: u}))
	var hashes [][]byte
	for i := 0; i < n; i++ {
		h := addSignatureEntry(t, b, u, i)
		hashes = append(hashes, h[:])
	}
	require.NoError(t, b.Commit())

	dst := newObservedDB(t)
	// The production querier: a window is taken from the chain's state at
	// its start, which the entry record carries. Window zero: the mark set
	// holding the last entry, as the ledgers are taken; the pool's window is
	// only wider.
	peer := api.Querier2{Querier: v3impl.NewQuerier(v3impl.QuerierParams{Database: src, Partition: "Directory"})}
	require.NoError(t, Account(context.Background(), peer, nil, u, Options{Mode: ModeWindow, Window: 0, Store: dst, Partition: protocol.DnUrl()}))

	r := dst.Begin(false)
	rec, err := r.Account(u).SyncedEntries("signature").Get()
	require.NoError(t, err)
	from := uint64(n-1) &^ 255
	for i := from; i < n; i++ {
		_, err := r.Account(u).SignatureChain().Inner().Element(i).Get()
		require.NoError(t, err, "entry %d of the window is not held", i)
		_, err = r.Message2(hashes[i]).Main().Get()
		require.Error(t, err, "the message behind entry %d was taken with the window", i)
	}
	r.Discard()
	require.Len(t, rec.Gaps, 2, "the record names %+v", rec.Gaps)
	require.True(t, rec.Gaps[0].MessagesOnly && rec.Gaps[0].Low == from && rec.Gaps[0].High == n, "the window's messages are not recorded missing: %+v", rec.Gaps[0])

	require.NoError(t, Backfill(context.Background(), []Source{peer}, dst, u, 0, nil))
	r = dst.Begin(false)
	defer r.Discard()
	for i := 0; i < n; i++ {
		_, err = r.Message2(hashes[i]).Main().Get()
		require.NoError(t, err, "the backfill did not bring the message behind entry %d", i)
	}
	rec, err = r.Account(u).SyncedEntries("signature").Get()
	require.NoError(t, err)
	require.Empty(t, rec.Gaps)
}

// addSignatureEntry stores a signature message and appends its hash to u's
// signature chain, as execution does.
func addSignatureEntry(t testing.TB, b *database.Batch, u *url.URL, i int) [32]byte {
	t.Helper()
	txn := new(protocol.Transaction)
	txn.Header.Principal = u
	txn.Body = &protocol.WriteData{Entry: &protocol.DoubleHashDataEntry{Data: [][]byte{{byte(i), byte(i >> 8), 0x5a}}}}
	msg := &messaging.TransactionMessage{Transaction: txn}
	h := msg.Hash()
	require.NoError(t, b.Message(h).Main().Put(msg))
	require.NoError(t, b.Account(u).SignatureChain().Inner().AddEntry(h[:], false))
	return h
}

// TestARefusalEveryPeerKeepsUpFailsTheChainsAttempt — #4533, run
// 20261005T175916Z: a follower's Directory pull waited on one peer's NotReady
// for eight minutes, the page asked again after every pause. A NotReady is
// not only the gate's "busy": a peer refuses an anchor it holds unsigned, a
// BPT block not yet on the ledger's chain, everything while it joins. A page
// every peer still refuses once the pauses are spent is the chain's refusal
// from these peers: the attempt fails, as any other refusal does, and the
// caller asks the next source. Beside a peer that serves, the refuser's
// pages spill over and the account is taken.
func TestARefusalEveryPeerKeepsUpFailsTheChainsAttempt(t *testing.T) {
	defer func(p streamPauses) { streamPause = p }(streamPause)
	streamPause = streamPauses{time.Millisecond, 4 * time.Millisecond, 20 * time.Millisecond}

	u := protocol.DnUrl().JoinPath("anchors")
	src := newObservedDB(t)
	buildChain(t, src, u, 600, 0x54)
	var inflight, most atomic.Int64
	refuser := &countingSource{dbSource: &dbSource{db: src}, name: "refuser", inflight: &inflight, most: &most, busy: func() bool { return true }}
	server := &countingSource{dbSource: &dbSource{db: src}, name: "server", inflight: &inflight, most: &most}

	dst := newObservedDB(t)
	done := make(chan error, 1)
	go func() {
		ctx := WithPeers(context.Background(), []Source{refuser}, 0)
		done <- Account(ctx, refuser, nil, u, Options{Mode: ModeFullSpine, Store: dst})
	}()
	select {
	case err := <-done:
		require.Error(t, err)
		require.True(t, errors.Is(err, errors.NotReady), "the refusal was not passed up: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("the pull waited on the busy peer past its pauses")
	}

	dst2 := newObservedDB(t)
	ctx := WithPeers(context.Background(), []Source{refuser, server}, 0)
	require.NoError(t, Account(ctx, refuser, nil, u, Options{Mode: ModeFullSpine, Store: dst2}))
	requireWhole(t, src, dst2, u)
	require.Positive(t, server.pages.Load(), "the refuser's pages did not spill to the peer beside it")
}

// TestTheSpineAccountsChainsSpreadAcrossThePeersToo — #4533: the spine
// accounts' chains (anchors, ledger, synthetic: TakeChainsAt, the pages the
// spreading was for) were taken from one source at a time with no peers
// named, so no page of theirs ever went to another peer.
func TestTheSpineAccountsChainsSpreadAcrossThePeersToo(t *testing.T) {
	const n = 3000
	u := protocol.DnUrl().JoinPath("anchors")
	src := newObservedDB(t)
	// Built with its leaf, as execution leaves it: UpdateBPT hashes the
	// accounts its own batch changed.
	b := src.Begin(true)
	require.NoError(t, b.Account(u).Main().Put(&protocol.DataAccount{Url: u}))
	for i := 0; i < n; i++ {
		addTransactionEntry(t, b, u, i, 0x55)
	}
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())
	var inflight, most atomic.Int64
	var peers []Source
	var counting []*countingSource
	for i := 0; i < 2; i++ {
		c := &countingSource{dbSource: &dbSource{db: src}, name: string(rune('a' + i)), inflight: &inflight, most: &most}
		counting = append(counting, c)
		peers = append(peers, c)
	}
	// The account's state as a peer serves it; the join takes the chains to
	// its heads (TakeChainsAt), then keeps the state, heads included.
	q := api.Querier2{Querier: v3impl.NewQuerier(v3impl.QuerierParams{Database: src, Partition: "Directory"})}
	rec, err := q.QueryAccountStates(context.Background(), protocol.DnUrl(), &api.AccountStatesQuery{Accounts: []*url.URL{u}})
	require.NoError(t, err)
	require.Len(t, rec.States, 1)
	st := rec.States[0]

	dst := newObservedDB(t)
	i, err := TakeChainsAt(context.Background(), peers, dst, st, 0, true, false, 0)
	require.NoError(t, err)
	require.Equal(t, 0, i)
	require.NoError(t, KeepState(dst, st, 0))
	requireWhole(t, src, dst, u)
	require.Positive(t, counting[1].pages.Load(), "no page of the spine account went to the second peer")
	require.GreaterOrEqual(t, most.Load(), int64(2), "the pages were asked for one at a time")
}

// TestAPeerThatCannotBeReachedIsPassedForTheNext — #4533, run
// 20261005T183032Z: the Directory's sources held two followers that were
// joining and could not be dialed, and with every source spreading its pages
// across them, every source failed the anchor pool's chain ("connection
// failed" is no NotReady). A peer that fails a page in any way is passed for
// the next; only a page every peer fails is the chain's failure, and one
// nobody was busy for fails at once, with no pause.
func TestAPeerThatCannotBeReachedIsPassedForTheNext(t *testing.T) {
	defer func(p streamPauses) { streamPause = p }(streamPause)
	streamPause = streamPauses{time.Second, time.Second, 10 * time.Second}

	u := protocol.DnUrl().JoinPath("anchors")
	src := newObservedDB(t)
	buildChain(t, src, u, 1200, 0x56)
	var inflight, most atomic.Int64
	dial := errors.UnknownError.With("dial /p2p/12D3KooWdead/acc-svc/query:directory: connection failed")
	dead := &countingSource{dbSource: &dbSource{db: src}, name: "dead", inflight: &inflight, most: &most, fail: dial}
	server := &countingSource{dbSource: &dbSource{db: src}, name: "server", inflight: &inflight, most: &most}

	dst := newObservedDB(t)
	ctx := WithPeers(context.Background(), []Source{server, dead}, 0)
	require.NoError(t, Account(ctx, server, nil, u, Options{Mode: ModeFullSpine, Store: dst}))
	requireWhole(t, src, dst, u)
	require.Zero(t, dead.pages.Load())

	dst2 := newObservedDB(t)
	begun := time.Now()
	ctx = WithPeers(context.Background(), []Source{dead}, 0)
	err := Account(ctx, dead, nil, u, Options{Mode: ModeFullSpine, Store: dst2})
	require.Error(t, err)
	require.False(t, errors.Is(err, errors.NotReady), "an unreachable peer was taken for a busy one: %v", err)
	require.Contains(t, err.Error(), "connection failed")
	require.Less(t, time.Since(begun), streamPause.first, "the pull paused for a peer nobody was busy for")
}
