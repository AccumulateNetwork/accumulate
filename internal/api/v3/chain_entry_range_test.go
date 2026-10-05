// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// chainOf writes n transactions of size bytes each to u's main chain.
func chainOf(t *testing.T, db *database.Database, u *url.URL, n, size int) {
	t.Helper()
	batch := db.Begin(true)
	defer batch.Discard()
	require.NoError(t, batch.Account(u).Main().Put(&protocol.DataAccount{Url: u}))
	for i := 0; i < n; i++ {
		data := make([]byte, size)
		data[0], data[1] = byte(i), byte(i>>8)
		txn := new(protocol.Transaction)
		txn.Header.Principal = u
		txn.Body = &protocol.WriteData{Entry: &protocol.DoubleHashDataEntry{Data: [][]byte{data}}}
		msg := &messaging.TransactionMessage{Transaction: txn}
		h := msg.Hash()
		require.NoError(t, batch.Message(h).Main().Put(msg))
		require.NoError(t, batch.Account(u).MainChain().Inner().AddEntry(h[:], false))
	}
	require.NoError(t, batch.Commit())
}

// entriesFrom asks for count of u's main chain entries from start, with the
// messages behind them if expand, and returns how many were answered and
// their size.
func entriesFrom(t *testing.T, q *Querier, u *url.URL, start, count uint64, expand bool) (int, int) {
	t.Helper()
	n, size, _ := entriesOf(t, q, u, &api.RangeOptions{Start: start, Count: &count, Expand: &expand})
	return n, size
}

// entriesOf asks for a range of u's main chain entries and returns how many
// were answered, their size, and the first index answered.
func entriesOf(t *testing.T, q *Querier, u *url.URL, opts *api.RangeOptions) (int, int, uint64) {
	t.Helper()
	start := opts.Start
	r, err := q.Query(context.Background(), u, &api.ChainQuery{
		Name:  "main",
		Range: opts,
	})
	require.NoError(t, err)
	rr, ok := r.(*api.RecordRange[*api.ChainEntryRecord[api.Record]])
	require.True(t, ok, "answered a %T", r)
	if opts.FromEnd {
		start = rr.Start
	}
	var size int
	for i, e := range rr.Records {
		require.Equal(t, start+uint64(i), e.Index, "the answer is not the entries from the start")
		b, err := e.MarshalBinary()
		require.NoError(t, err)
		size += len(b)
	}
	return len(rr.Records), size, rr.Start
}

// TestAChainEntryRangeIsAtMost200EntriesAnd50000Bytes — #4493, Paul
// 2026-10-05: "use it to pull 200 entries at a time. If sending, return the
// entries returned so we can limit the size to 50k if elements are bigger
// than expected." A range of a chain's entries is answered with up to 200
// entries -- it was clamped to 100 -- and stops before it passes 50,000
// bytes, always holding at least one.
func TestAChainEntryRangeIsAtMost200EntriesAnd50000Bytes(t *testing.T) {
	db := database.OpenInMemory(nil)
	q := NewQuerier(QuerierParams{Database: db, Partition: "Test"})

	small := protocol.DnUrl().JoinPath("small")
	chainOf(t, db, small, 300, 8)
	n, size := entriesFrom(t, q, small, 0, 300, false)
	require.Equal(t, 200, n, "entries answered for 300 asked without their messages")
	require.LessOrEqual(t, size, maxChainEntryBytes)
	// With its message even a small transaction is some 340 bytes: 200 of
	// them pass the cap, and the bytes decide.
	n, size = entriesFrom(t, q, small, 0, 300, true)
	require.Less(t, n, 200)
	require.Greater(t, n, 100, "entries answered for 300 asked with their messages")
	require.LessOrEqual(t, size, maxChainEntryBytes)

	large := protocol.DnUrl().JoinPath("large")
	chainOf(t, db, large, 100, 2000)
	n, size = entriesFrom(t, q, large, 0, 100, true)
	require.Less(t, n, 100, "a range of 2 kB entries was answered past 50,000 bytes")
	require.Greater(t, n, 1)
	require.LessOrEqual(t, size, maxChainEntryBytes)
	// Asked from the end, an answer that does not fit keeps the newest
	// entries and says where it starts: the explorer's newest-first lists
	// lost the newest entries when it kept the oldest.
	count, expand := uint64(100), true
	m, size, first := entriesOf(t, q, large, &api.RangeOptions{Count: &count, Expand: &expand, FromEnd: true})
	require.Less(t, m, 100)
	require.LessOrEqual(t, size, maxChainEntryBytes)
	require.Equal(t, uint64(100-m), first, "the answer from the end does not end at the newest entry")

	// The next answer starts where this one stopped.
	m, _ = entriesFrom(t, q, large, uint64(n), 100, true)
	require.Greater(t, m, 0)

	huge := protocol.DnUrl().JoinPath("huge")
	chainOf(t, db, huge, 3, 60_000)
	n, _ = entriesFrom(t, q, huge, 0, 3, true)
	require.Equal(t, 1, n, "an entry larger than the cap is answered alone")
}
