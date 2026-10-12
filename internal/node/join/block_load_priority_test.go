// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// holding is a peer whose first queries, the background's, are held until
// release is closed.
type holding struct {
	api.Querier
	hold    int64
	calls   atomic.Int64
	release chan struct{}
}

func (h *holding) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	if h.calls.Add(1) <= h.hold {
		select {
		case <-h.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return h.Querier.Query(ctx, scope, query)
}

// TestABlocksLoadIsNotQueuedBehindTheBackground — #4557, run
// 20261010T194045Z: a block's load of a section it reads queued behind the
// fill's and the look-ahead's queries to the same peer until its time was
// gone, and the block was held. With the background holding every slot it
// may, the block's load still gets the peer at once.
func TestABlocksLoadIsNotQueuedBehindTheBackground(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	peerDB := peerStore(t)
	putLedger(t, peerDB, here, 10)
	var accounts []*url.URL
	for i := 0; i < 50; i++ {
		u := protocol.AccountUrl(fmt.Sprintf("held-%03d", i), "tokens")
		accounts = append(accounts, u)
		putTokens(t, peerDB, u, int64(i))
	}

	peer := &holding{
		Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peerDB, Partition: "BVN0"}),
		release: make(chan struct{}),
	}
	gated := pull.Gated(peer, pull.NewGate())
	src := api.Querier2{Querier: gated}

	db := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = db.Close() })
	values, _ := genesisValues(t, 4)
	putNetwork(t, db, here, values)
	s, err := NewState(StateOptions{Partition: here, Database: db, Sources: &fixedSources{partition: here, srcs: []pull.Source{src}}, ExecutedBlock: 1})
	require.NoError(t, err)
	require.NoError(t, s.Pull(context.Background()))
	u := accounts[0]
	sec := database.SectionOfAccount(u)
	require.True(t, s.secs.NotLoaded(sec), "precondition: %v is not loaded", u)

	// The background takes every slot it may and holds them.
	peer.hold = peer.calls.Load() + pull.PeerInFlight - 1
	defer close(peer.release)
	bg, stop := context.WithCancel(context.Background())
	defer stop()
	for i := 0; i < pull.PeerInFlight+2; i++ {
		go func() { _, _ = gated.Query(bg, here, &api.DefaultQuery{}) }()
	}
	require.Eventually(t, func() bool { return peer.calls.Load() == peer.hold }, time.Second, time.Millisecond)

	s.secs.timeout = 2 * time.Second
	batch := db.Begin(true)
	defer batch.Discard()
	require.NoError(t, s.secs.Load(batch, sec), "the block's load queued behind the background")
}
