// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package network_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ParseNetwork leaves the receiver unchanged when it rejects a definition
// (#4615).
func TestParseNetwork_RejectedChangesNothing(t *testing.T) {
	stale := network.GlobalValues{Network: &protocol.NetworkDefinition{Version: 3, NetworkName: "stale"}}
	g := network.GlobalValues{Network: &protocol.NetworkDefinition{Version: 3, NetworkName: "current"}}

	require.ErrorContains(t, g.ParseNetwork(stale.FormatNetwork()), "version must increase: 3 <= 3")
	require.Equal(t, "current", g.Network.NetworkName)

	require.Error(t, g.ParseNetwork(&protocol.DoubleHashDataEntry{Data: [][]byte{{1}, {2}}}))
	require.Equal(t, "current", g.Network.NetworkName)

	next := network.GlobalValues{Network: &protocol.NetworkDefinition{Version: 4, NetworkName: "next"}}
	require.NoError(t, g.ParseNetwork(next.FormatNetwork()))
	require.Equal(t, "next", g.Network.NetworkName)
}

// ParseNetworkLegacy stores a definition and then rejects it if its version
// does not increase, as ParseNetwork did before V2Canaveral. Replay depends on
// the stored definition.
func TestParseNetworkLegacy_StoresThenRejects(t *testing.T) {
	stale := network.GlobalValues{Network: &protocol.NetworkDefinition{Version: 3, NetworkName: "stale"}}
	g := network.GlobalValues{Network: &protocol.NetworkDefinition{Version: 3, NetworkName: "current"}}

	require.ErrorContains(t, g.ParseNetworkLegacy(stale.FormatNetwork()), "version must increase: 3 <= 3")
	require.Equal(t, "stale", g.Network.NetworkName)

	require.Error(t, g.ParseNetworkLegacy(&protocol.DoubleHashDataEntry{Data: [][]byte{{1}, {2}}}))
	require.Equal(t, "stale", g.Network.NetworkName, "an entry that does not parse stores nothing")

	next := network.GlobalValues{Network: &protocol.NetworkDefinition{Version: 4, NetworkName: "next"}}
	require.NoError(t, g.ParseNetworkLegacy(next.FormatNetwork()))
	require.Equal(t, "next", g.Network.NetworkName)
}
