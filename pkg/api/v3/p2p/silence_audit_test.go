// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

// AUDIT ONLY (no production change). Paul's rule: while a node is syncing it
// sends nothing to anyone. The first thing a joining node sends is its own
// advertisement -- and it sends it before it has executed a single block.

import (
	"context"
	"testing"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
)

// TestAudit_AJoiningNodeAdvertisesItselfAsAProvider.
//
// cmd/accumulated/run/dagbft.go registers the querier, sequencer, submitter,
// validator, consensus and proof services on the p2p host while the node's
// nodestate.Machine is still BOOTING. RegisterService
// (pkg/api/v3/p2p/services.go:36) takes no state and consults none: it
// installs the stream handler (:42) and then puts the node in the DHT as a
// PROVIDER of the service (:55 advertizeNewService -> peer_manager.go:171
// util.Advertise) and broadcasts a whoami (peer_manager.go:178-180).
//
// So other nodes can find, route to and dial a node that has executed nothing.
// The refusals the services then give are a second line of defence; the node
// is already in everyone's peer list, and a caller that picked it spent a
// round trip and a retry slot on a node that cannot help.
//
// This test is the daemon's sequence, minus the daemon: register, then look.
func TestAudit_AJoiningNodeAdvertisesItselfAsAProvider(t *testing.T) {
	const network = "SilenceAudit"

	// The joining node. Nothing here is told it is joining, because there is
	// nothing to tell.
	joining, err := New(Options{Network: network, Listen: addrs(t, "/ip4/127.0.0.1/tcp/0"), DiscoveryMode: dht.ModeServer})
	require.NoError(t, err)
	defer func() { _ = joining.Close() }()

	// Exactly what registerAPIServices publishes, in the same order.
	published := []*api.ServiceAddress{
		api.ServiceTypeQuery.AddressFor("BVN1"),
		private.ServiceTypeSequencer.AddressFor("BVN1"),
		api.ServiceTypeSubmit.AddressFor("BVN1"),
		api.ServiceTypeValidate.AddressFor("BVN1"),
		api.ServiceTypeConsensus.AddressFor("BVN1"),
	}
	for _, sa := range published {
		require.True(t, joining.RegisterService(sa, func(message.Stream) {}),
			"%v", sa)
	}

	// Any other node on the network.
	peer, err := New(Options{Network: network, Listen: addrs(t, "/ip4/127.0.0.1/tcp/0"),
		BootstrapPeers: joining.Addresses(), DiscoveryMode: dht.ModeServer})
	require.NoError(t, err)
	defer func() { _ = peer.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	find := func(sa *api.ServiceAddress) []*api.FindServiceResult {
		t.Helper()
		var last []*api.FindServiceResult
		for i := 0; i < 60; i++ {
			r, err := peer.Services().FindService(ctx, api.FindServiceOptions{Network: network, Service: sa})
			require.NoError(t, err)
			last = r
			if len(r) > 0 {
				return r
			}
			time.Sleep(250 * time.Millisecond)
		}
		return last
	}

	for _, sa := range published {
		found := find(sa)
		require.NotEmpty(t, found, "%v: a joining node was not discoverable", sa)
		var sawIt bool
		for _, r := range found {
			if r.PeerID == joining.ID() {
				sawIt = true
			}
		}
		require.True(t, sawIt,
			"%v: the network does not list the joining node as a provider; the audit is out of date", sa)
	}

	// And it says so itself: NodeInfo lists everything it has registered,
	// with no indication that it cannot answer for any of it.
	info, err := joining.Services().NodeInfo(ctx, api.NodeInfoOptions{})
	require.NoError(t, err)
	listed := map[string]bool{}
	for _, sa := range info.Services {
		listed[sa.String()] = true
	}
	for _, sa := range published {
		require.True(t, listed[sa.String()],
			"NodeInfo does not advertise %v while joining; the audit is out of date", sa)
	}
}
