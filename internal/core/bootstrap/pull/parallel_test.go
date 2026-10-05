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
