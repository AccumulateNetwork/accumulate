// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestAJoiningNodeAdvertisesItsServicesOnlyOnceActive — DIFFERENCES E30
// (Paul, 2026-10-05): a joining node's query and sequencer services refuse
// every read, so advertising them makes the node a source its peers ask and
// are refused by -- two followers joining together each waited on the
// other. The services are registered with the network when the node first
// becomes ACTIVE for the partition, once; a node that never joined registers
// at once.
func TestAJoiningNodeAdvertisesItsServicesOnlyOnceActive(t *testing.T) {
	registered := 0
	whenServing(nodestate.Always{}, func() { registered++ })
	require.Equal(t, 1, registered, "a node that never joined did not register at once")

	registered = 0
	m := nodestate.New(protocol.PartitionUrl("BVN1"))
	whenServing(m, func() { registered++ })
	require.Zero(t, registered, "a joining node registered before it was ACTIVE")
	require.True(t, m.PromoteToActive([32]byte{1}, 10))
	require.Equal(t, 1, registered, "the node did not register when it became ACTIVE")
	require.True(t, m.Demote(20))
	require.True(t, m.PromoteToActive([32]byte{2}, 30))
	require.Equal(t, 1, registered, "the node registered again on a later promotion")

	// Already ACTIVE when the service starts: registered at once.
	registered = 0
	whenServing(m, func() { registered++ })
	require.Equal(t, 1, registered)
}
