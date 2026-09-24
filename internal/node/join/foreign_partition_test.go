// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// routingSources answers with the partition it is told to, so a test can put an
// account somewhere other than the joining node's own partition.
type routingSources struct {
	noValidators
	partition *url.URL
}

func (r *routingSources) For(context.Context, *url.URL) ([]pull.Source, *url.URL, error) {
	return nil, r.partition, nil
}

func (r *routingSources) Querier(*url.URL) api.Querier { return nil }

func quietState(t *testing.T, partition *url.URL, srcs Sources) *PulledState {
	t.Helper()
	db := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = db.Close() })
	return &PulledState{
		partition: partition,
		db:        db,
		sources:   srcs,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// TestFetch_DropsAnAccountOfAnotherPartition drives the production fetch loop,
// not the helper under it.
//
// A peer names the accounts a joining node pulls -- in a block ledger record,
// or as a BPT page entry -- and nothing about that name has to belong to this
// partition. The account then verifies perfectly: it is fetched from its own
// partition's honest peers and settles against the root the Directory anchored
// for THAT partition, so no check downstream catches it. Writing it puts a leaf
// in this partition's BPT that no peer of this partition holds, the local root
// leaves the anchored series, and because the leaf is durable a restart does
// not clear it -- the node can never join again without wiping its database.
//
// The account must therefore be DROPPED and not owed. What is owed is asked again
// for the life of the process, and this name is never going to become this
// partition's.
func TestFetch_DropsAnAccountOfAnotherPartition(t *testing.T) {
	ctx := context.Background()
	here := protocol.PartitionUrl("BVN0")
	elsewhere := protocol.PartitionUrl("BVN1")

	s := quietState(t, here, &routingSources{partition: elsewhere})
	p := newSyncing()

	require.Equal(t, dropped, s.pullOne(ctx, p, protocol.AccountUrl("alice", "tokens")),
		"nothing of another partition is ever pulled")
	require.Empty(t, p.retry,
		"an account of another partition is dropped, not owed: owing it asks "+
			"every peer for it again every round, for the life of the process")
}

// TestFetch_StillOwesAnAccountOfThisPartition is the control on the test
// above. Without it, "nothing is owed" would also pass if pullOne had stopped
// owing anything at all, and the assertion would prove nothing.
func TestFetch_StillOwesAnAccountOfThisPartition(t *testing.T) {
	ctx := context.Background()
	here := protocol.PartitionUrl("BVN0")

	// Routed here, but with no source that can answer, so the fetch fails the
	// ordinary way and the account is asked for again next round.
	s := quietState(t, here, &routingSources{partition: here})
	p := newSyncing()

	require.Equal(t, owed, s.pullOne(ctx, p, protocol.AccountUrl("alice", "tokens")))
	require.Len(t, p.retry, 1,
		"an account of this partition that could not be pulled is asked for again")
}

// TestSourcesFor_RefusesAnotherPartition pins the decision itself, so that the
// reason pullOne drops the account is visible where it is made.
func TestSourcesFor_RefusesAnotherPartition(t *testing.T) {
	ctx := context.Background()
	here := protocol.PartitionUrl("BVN0")
	elsewhere := protocol.PartitionUrl("BVN1")

	s := quietState(t, here, &routingSources{partition: elsewhere})

	_, _, err := s.sourcesFor(ctx, protocol.AccountUrl("alice", "tokens"))
	require.Error(t, err)
	require.True(t, errors.Is(err, errNotThisPartition),
		"the caller distinguishes this from a refusal, so it must be this error")

	// And an account of this partition still resolves.
	s2 := quietState(t, here, &routingSources{partition: here})
	_, partition, err := s2.sourcesFor(ctx, protocol.AccountUrl("alice", "tokens"))
	require.NoError(t, err)
	require.True(t, partition.Equal(here))
}
