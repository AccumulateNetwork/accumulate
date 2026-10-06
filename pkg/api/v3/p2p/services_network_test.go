// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

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

// A node advertises a service under its network's key, so a lookup that does
// not name a network searches a key nobody advertises and answers "nobody
// serves that" -- with no error, in a millisecond. Every caller inside a node
// means its own network; the join and the conductor did not say so and got
// silence, which the join read as "the whole network restarted" and executed
// from its own empty staging (#4296, run 20260918T124530Z; the divergence of
// #4290). A lookup with no network named now means this node's network.
func TestFindService_NoNetworkNamedMeansThisNodesNetwork(t *testing.T) {
	const network = "ProbeNet"
	sequencer := private.ServiceTypeSequencer.AddressFor("BVN1")

	// A server that advertises the sequencer, and a client that looks for it
	server, err := New(Options{Network: network, Listen: addrs(t, "/ip4/127.0.0.1/tcp/0"), DiscoveryMode: dht.ModeServer})
	require.NoError(t, err)
	defer func() { _ = server.Close() }()
	require.True(t, server.RegisterService(sequencer, func(message.Stream) {}))

	client, err := New(Options{Network: network, Listen: addrs(t, "/ip4/127.0.0.1/tcp/0"),
		BootstrapPeers: server.Addresses(), DiscoveryMode: dht.ModeServer})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	find := func(opts api.FindServiceOptions) []*api.FindServiceResult {
		t.Helper()
		var last []*api.FindServiceResult
		for i := 0; i < 40; i++ {
			r, err := client.Services().FindService(ctx, opts)
			require.NoError(t, err)
			last = r
			if len(r) > 0 {
				return r
			}
			time.Sleep(250 * time.Millisecond)
		}
		return last
	}

	named := find(api.FindServiceOptions{Network: network, Service: sequencer})
	require.NotEmpty(t, named, "a search that names the network finds the server")

	unnamed := find(api.FindServiceOptions{Service: sequencer})
	require.NotEmpty(t, unnamed, "a search that names no network means this node's network, and finds the same server")
}

// TestFindService_FindsAConnectedServerBeforeTheDHTKnowsIt — #4537: two
// followers of run 20261006T031201Z found no validator of their partition for
// twenty seconds after start while connected to every one of them: their DHT
// routing tables were still warming, and FindService asked the DHT alone. A
// search lists the connected peers that serve the service, by the protocols
// identify reported, before anything the DHT says. Here the DHT is given no
// time to answer at all, and the connected server is found all the same.
func TestFindService_FindsAConnectedServerBeforeTheDHTKnowsIt(t *testing.T) {
	const network = "ProbeNet"
	sequencer := private.ServiceTypeSequencer.AddressFor("BVN1")
	server, err := New(Options{Network: network, Listen: addrs(t, "/ip4/127.0.0.1/tcp/0"), DiscoveryMode: dht.ModeServer})
	require.NoError(t, err)
	defer func() { _ = server.Close() }()
	require.True(t, server.RegisterService(sequencer, func(message.Stream) {}))
	client, err := New(Options{Network: network, Listen: addrs(t, "/ip4/127.0.0.1/tcp/0"),
		BootstrapPeers: server.Addresses(), DiscoveryMode: dht.ModeServer})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var last []*api.FindServiceResult
	for i := 0; i < 80 && len(last) == 0; i++ {
		// A nanosecond: the DHT has no time to answer.
		last, err = client.Services().FindService(ctx, api.FindServiceOptions{Network: network, Service: sequencer, Timeout: time.Nanosecond})
		require.NoError(t, err)
		if len(last) == 0 {
			time.Sleep(250 * time.Millisecond)
		}
	}
	require.NotEmpty(t, last, "the connected server was not found without the DHT")
	require.Equal(t, server.ID(), last[0].PeerID)
}
