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
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// statesSource is one peer that counts the AccountStates calls asked of it
// and how many are in flight at once across every statesSource sharing the
// gauge, each call taking long enough for calls to overlap.
type statesSource struct {
	pull.Source
	calls    atomic.Int64
	inflight *atomic.Int64
	most     *atomic.Int64
}

func (s *statesSource) QueryAccountStates(ctx context.Context, u *url.URL, q *api.AccountStatesQuery) (*api.AccountStatesRecord, error) {
	s.calls.Add(1)
	n := s.inflight.Add(1)
	defer s.inflight.Add(-1)
	for {
		m := s.most.Load()
		if n <= m || s.most.CompareAndSwap(m, n) {
			break
		}
	}
	time.Sleep(2 * time.Millisecond)
	return s.Source.QueryAccountStates(ctx, u, q)
}

// TestTheStatesOfManyAccountsAreAskedAcrossThePeersAtOnce — #4535, run
// 20261005T185718Z: a validator serves an account's state in ~17 ms, and a
// round's stale accounts were asked of one peer, one call after another, so
// a pull against a tree of 29k accounts chased the head for sixteen minutes
// and no follower joined. The accounts are asked in chunks, statesInFlight
// at a time of every peer, and every state is kept.
func TestTheStatesOfManyAccountsAreAskedAcrossThePeersAtOnce(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	peer := openDisk(t, t.TempDir())
	t.Cleanup(func() { _ = peer.Close() })
	const n = 300
	accounts := make([]*url.URL, n)
	for i := range accounts {
		accounts[i] = protocol.AccountUrl(fmt.Sprintf("many-%03d", i), "data")
		writeAccount(t, peer, accounts[i], 3)
	}
	var inflight, most atomic.Int64
	served := api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peer, Partition: "BVN0"})}
	a := &statesSource{Source: served, inflight: &inflight, most: &most}
	b := &statesSource{Source: served, inflight: &inflight, most: &most}

	node := openDisk(t, t.TempDir())
	t.Cleanup(func() { _ = node.Close() })
	s := diskState(node, here, a)
	p := newSyncing()
	s.takeAccounts(context.Background(), p, []pull.Source{a, b}, accounts)

	require.Empty(t, p.retry, "accounts were left owed")
	batch := node.Begin(false)
	defer batch.Discard()
	for _, u := range accounts {
		_, err := batch.Account(u).Main().Get()
		require.NoError(t, err, "%v was not kept", u)
	}
	require.Positive(t, a.calls.Load())
	require.Positive(t, b.calls.Load(), "the second peer was never asked")
	require.GreaterOrEqual(t, most.Load(), int64(2), "the states were asked for one call at a time")
	require.LessOrEqual(t, a.calls.Load()+b.calls.Load(), int64(n/statesChunk+2), "the accounts were not asked in chunks of %d", statesChunk)
}
