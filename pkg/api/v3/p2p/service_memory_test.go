// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"testing"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
)

// An identify message the host consumes after a newer one replaces the
// peer's protocol set with an older one. The order the host consumes them
// in is the scheduler's, so the test plays the stale message by hand.
func staleIdentify(t *testing.T, n *Node, p peer.ID, protocols ...protocol.ID) {
	t.Helper()
	require.NoError(t, n.host.Peerstore().SetProtocols(p, protocols...))
}

func hasProtocol(n *Node, p peer.ID, id protocol.ID) bool {
	got, _ := n.host.Peerstore().SupportsProtocols(p, id)
	return len(got) > 0
}

// TestAPeersServiceSurvivesAStaleIdentifyMessage — #4610: identify pushes
// carry no sequence number and the host consumes them in whatever order its
// stream handlers run, so a push older than the one that announced a service
// can land last and take the service away; 8 of 40 four-node meshes on two
// loaded cores formed with a peer's submit service missing from the node it
// bootstrapped from (!1274, "the API mesh never formed"). A node never
// withdraws a service, so what a peer announced it still has.
func TestAPeersServiceSurvivesAStaleIdentifyMessage(t *testing.T) {
	n, err := New(Options{Network: "MemNet"})
	require.NoError(t, err)
	defer func() { _ = n.Close() }()
	p, err := peer.Decode("12D3KooWK3b4anvdKraezrJznHsMvdEzEQ49rx243j2y3cSNz1jz")
	require.NoError(t, err)
	submit := idRpc(api.ServiceTypeSubmit.AddressFor("BVN1"))
	consensus := idRpc(api.ServiceTypeConsensus.AddressFor("BVN1"))
	node := idRpc(api.ServiceTypeNode.Address())
	ping := protocol.ID("/ipfs/ping/1.0.0")

	// The push that announced the services
	staleIdentify(t, n, p, ping, node, submit, consensus)
	require.True(t, hasProtocol(n, p, submit))

	// An older push, consumed after it
	staleIdentify(t, n, p, ping, node)
	require.True(t, hasProtocol(n, p, submit), "the submit service did not survive a stale push")
	require.True(t, hasProtocol(n, p, consensus), "the consensus service did not survive a stale push")

	// The answer to the connect-time identify, from before the peer had
	// registered anything, consumed last of all
	staleIdentify(t, n, p, ping)
	require.True(t, hasProtocol(n, p, submit), "the submit service did not survive a stale identify answer")
	require.True(t, hasProtocol(n, p, node))
	require.True(t, hasProtocol(n, p, ping))

	// Only Accumulate's services are kept: a protocol the peer drops is gone
	staleIdentify(t, n, p, node)
	require.False(t, hasProtocol(n, p, ping), "a dropped libp2p protocol was kept")
}

// A peer whose last connection closes is forgotten: it may come back with
// fewer services, and what it announces then is what it has.
func TestAPeersServicesAreForgottenWhenItDisconnects(t *testing.T) {
	const network = "MemNet"
	submit := api.ServiceTypeSubmit.AddressFor("BVN1")
	server, err := New(Options{Network: network, Listen: addrs(t, "/ip4/127.0.0.1/tcp/0"), DiscoveryMode: dht.ModeServer})
	require.NoError(t, err)
	defer func() { _ = server.Close() }()
	client, err := New(Options{Network: network, Listen: addrs(t, "/ip4/127.0.0.1/tcp/0"),
		BootstrapPeers: server.Addresses(), DiscoveryMode: dht.ModeServer})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	// Registered after the client connected, as a daemon's services are: the
	// client learns it from a push, and remembers it
	require.True(t, server.RegisterService(submit, func(message.Stream) {}))
	require.Eventually(t, func() bool { return hasProtocol(client, server.ID(), idRpc(submit)) }, 10*time.Second, 50*time.Millisecond)
	require.Eventually(t, func() bool { return len(client.memory.services(server.ID())) > 0 }, 10*time.Second, 50*time.Millisecond)

	require.NoError(t, server.Close())
	require.Eventually(t, func() bool { return len(client.memory.services(server.ID())) == 0 }, 10*time.Second, 50*time.Millisecond,
		"the server's services were remembered after its connection closed")
	staleIdentify(t, client, server.ID(), idRpc(api.ServiceTypeNode.Address()))
	require.False(t, hasProtocol(client, server.ID(), idRpc(submit)), "a forgotten peer's service was put back")
}
