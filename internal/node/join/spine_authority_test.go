// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
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

// TestTheSpineIsNotVerifiedUntilEveryAccountSettled.
//
// `spineSettled` is what decides whether the join has a spine it may build
// on. Marking it verified when some of it was refused would let the node go
// on with accounts it could not prove, and would stop it asking for them
// again — the re-fetch path exists precisely because a spine served at a
// block nobody anchors is given up on after four rounds.
func TestTheSpineIsNotVerifiedUntilEveryAccountSettled(t *testing.T) {
	s, _, _ := newJoiningNode(t, 4)

	s.spinePending = true
	s.spineSettled(3, 4, 1)
	require.False(t, s.spine, "the spine was marked verified with an account refused")
	require.False(t, s.spinePending, "the fetch is finished with, so the next round must be free to ask again")

	s.spinePending = true
	s.spineSettled(2, 4, 0)
	require.False(t, s.spine, "the spine was marked verified with accounts still unsettled")

	s.spinePending = true
	s.spineSettled(4, 4, 0)
	require.True(t, s.spine, "every account settled and the spine was still not verified")
	require.False(t, s.spinePending)
}

// TestTheTrustedSetsFollowTheStore.
//
// The definition a joining node verifies anchors against moves one way: out
// of its own store, which the pull only ever writes verified state into. A
// join that read it once would be stranded by the next change exactly as it
// was by the last (#4301, review finding 1), so the read is every round.
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
