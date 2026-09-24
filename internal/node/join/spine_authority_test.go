// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// newJoiningNode is a node with its own genesis network accounts and nothing
// else, wired the way NewState wires one.
func newJoiningNode(t *testing.T, n int) (*PulledState, *database.Database, *core.GlobalValues) {
	t.Helper()
	here := protocol.PartitionUrl("BVN0")
	db := database.OpenInMemory(nil)
	db.SetObserver(database.NewDatabaseObserver())
	t.Cleanup(func() { _ = db.Close() })

	values, _ := genesisValues(t, n)
	putNetwork(t, db, here, values)

	s, err := NewState(StateOptions{
		Partition:     here,
		Database:      db,
		Sources:       noSources{},
		ExecutedBlock: 1,
	})
	require.NoError(t, err)
	return s, db, values
}

// TestTheTrustedSetsFollowTheStore.
//
// The definition a joining node verifies anchors against moves one way: out
// of its own store, when that store is proven -- at the start, and at a
// match. A newer definition moves it; an older one never rolls it back.
func TestTheTrustedSetsFollowTheStore(t *testing.T) {
	s, db, values := newJoiningNode(t, 4)
	here := protocol.PartitionUrl("BVN0")

	require.Equal(t, uint64(1), s.TrustedVersion())

	// Nothing changed: the sets do not move, and nothing is logged as a move.
	s.refreshAuthority()
	require.Equal(t, uint64(1), s.TrustedVersion())

	// A newer definition, as the pull would have written it.
	next := &core.GlobalValues{Network: values.Network.Copy(), Globals: values.Globals}
	next.Network.Version = 2
	putNetwork(t, db, here, next)

	s.refreshAuthority()
	require.Equal(t, uint64(2), s.TrustedVersion(),
		"the node is still verifying against the definition it started with")

	// And an older one does not roll it back, however it got there.
	older := &core.GlobalValues{Network: values.Network.Copy(), Globals: values.Globals}
	older.Network.Version = 1
	putNetwork(t, db, here, older)
	s.refreshAuthority()
	require.Equal(t, uint64(2), s.TrustedVersion(), "the trusted set was rolled back")
}

// TestTheTrustedSetsMoveOnlyAtAMatch: the pull writes what peers serve and
// proves none of it before the match (executor spec, "Sync", "The algorithm",
// step 3), so a network definition it wrote is a peer's word until the local
// root equals a root the trusted set signed. Read before that, a peer could
// name the validators that sign the very anchors the match is judged against
// (#4301). The pull rounds leave the set alone; the match moves it.
func TestTheTrustedSetsMoveOnlyAtAMatch(t *testing.T) {
	s, db, values := newJoiningNode(t, 4)
	here := protocol.PartitionUrl("BVN0")
	ctx := context.Background()

	// A newer definition, as a pull round writes one a peer served.
	next := &core.GlobalValues{Network: values.Network.Copy(), Globals: values.Globals}
	next.Network.Version = 2
	putNetwork(t, db, here, next)

	require.NoError(t, s.Pull(ctx))
	require.Equal(t, uint64(1), s.TrustedVersion(),
		"a pull round took the validator sets out of state nothing has proven")

	// The local root now equals a root the trusted set signed for block 9:
	// the state is proven, the definition with it.
	batch := db.Begin(false)
	root, err := batch.GetBptRootHash()
	batch.Discard()
	require.NoError(t, err)
	s.tracker.Observe(here, 9, root)

	block, ok, err := s.Matched(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, uint64(9), block)
	require.Equal(t, uint64(2), s.TrustedVersion(), "the match did not move the trusted set to the state it proved")
}
