// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// expandRecording records, per account, whether a chain's entries were asked
// for with the messages behind them.
type expandRecording struct {
	pull.Source
	expanded map[string]bool
	asked    map[string]bool
}

func (r *expandRecording) QueryChainEntries(ctx context.Context, u *url.URL, q *api.ChainQuery) (*api.RecordRange[*api.ChainEntryRecord[api.Record]], error) {
	k := strings.ToLower(u.String())
	r.asked[k] = true
	if q.Range != nil && q.Range.Expand != nil && *q.Range.Expand {
		r.expanded[k] = true
	}
	return r.Source.QueryChainEntries(ctx, u, q)
}

// TestFetch_ASpineAccountIsTakenWholeInEveryPass — #4421. The block ledger
// names <partition>/anchors in every pass after the spine's, because every
// block writes the pool. Taken state-only there, its new entries arrive with
// no message behind them, and the first block the node opens fails reading
// the newest. A spine account is taken whole whichever pass names it, and a
// failure in a later pass is refused by name and asked again: the spine has
// already settled and is not failed by it.
func TestFetch_ASpineAccountIsTakenWholeInEveryPass(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	pool := here.JoinPath(protocol.AnchorPool)
	other := protocol.AccountUrl("alice", "tokens")

	peer := database.OpenInMemory(nil)
	peer.SetObserver(database.NewDatabaseObserver())
	t.Cleanup(func() { _ = peer.Close() })
	// Entries with no message behind them: a peer that serves the pool so
	// has not served it, and the fetch fails.
	writeAccount(t, peer, pool, 3)
	writeAccount(t, peer, other, 3)

	rec := &expandRecording{
		Source:   api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peer, Partition: "BVN0"})},
		expanded: map[string]bool{},
		asked:    map[string]bool{},
	}
	s := quietState(t, here, &oneSource{part: here, src: servedAt{Source: rec, db: peer}})
	values, _ := genesisValues(t, 4)
	s.anchors = noAnchorSource(t, here, values)
	s.spine = true // a pass after the spine's

	s.fetchPass(context.Background(), []*url.URL{pool, other})

	k := strings.ToLower(pool.String())
	require.True(t, rec.asked[k], "precondition: the pool was fetched")
	require.True(t, rec.expanded[k], "the pool was taken state-only in a pass after the spine's: its entries arrive with no message behind them")
	require.False(t, rec.expanded[strings.ToLower(other.String())], "an account off the spine is taken state-only")

	require.True(t, s.spine, "a later pass's failure un-settled the spine")
	require.Contains(t, urlsOf(s.refused), pool.String(), "the pool's failure was not refused by name, so it is never asked again")
	if s.pass != nil {
		require.False(t, s.pass.spineFailed)
		s.dropPass()
	}
}
