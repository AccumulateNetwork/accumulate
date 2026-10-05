// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	v3impl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
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
}

func (s *countingSource) QueryChainEntries(ctx context.Context, u *url.URL, q *api.ChainQuery) (*api.RecordRange[*api.ChainEntryRecord[api.Record]], error) {
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
