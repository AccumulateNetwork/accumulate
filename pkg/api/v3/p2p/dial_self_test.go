// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p/dial"
)

// TestDialNetwork_AnswersForItself is the guard on #4303, stated where the
// behaviour lives.
//
// DialNetwork wraps its discoverer in a selfDiscoverer unconditionally --
// "Always use self-discovery" -- so a client built on it never reaches the
// network for a service this node itself provides. That is right for a node
// answering its own reads and catastrophic for a joining node's pull: a
// joining node registers query:<partition> for every partition it serves, so a
// pull given this client reads the half-empty store the pull exists to fill,
// and is refused by it forever without one packet leaving the process.
//
// This test exists so that the next person who hands a routed client to
// something that must not read itself finds the reason stated rather than
// discovering it in a soak run. The join's own guard is
// TestSelectPeers_NeverThisNode, in package join: the pull is addressed at a
// named peer, and this node's peer ID is dropped from every list.
func TestDialNetwork_AnswersForItself(t *testing.T) {
	node, err := New(Options{
		Network: "test-self-discovery",
		Listen:  addrs(t, "/ip4/127.0.0.1/tcp/0"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = node.Close() })

	// A service this node provides -- exactly what cmd/accumulated/run/api.go
	// registers for every partition a node serves.
	service := api.ServiceTypeQuery.AddressFor("BVN0")
	require.True(t, node.RegisterService(service, func(message.Stream) {}))

	d := &selfDiscoverer{node, refusingDiscoverer{}}

	local, ok := d.DiscoverLocal("test-self-discovery", service)
	require.True(t, ok, "the node did not recognise a service it provides")
	require.NotNil(t, local)

	res, err := d.Discover(context.Background(), &dial.DiscoveryRequest{
		Network: "test-self-discovery",
		Service: service,
	})
	require.NoError(t, err)
	require.IsType(t, dial.DiscoveredLocal(nil), res,
		"discovery for a service this node provides was answered locally, without the network; "+
			"a pull given a client built on DialNetwork therefore reads this node's own store")

	// And a service it does not provide does go to the network -- which is
	// what makes the first half a statement about self-discovery and not
	// about the discoverer.
	_, err = d.Discover(context.Background(), &dial.DiscoveryRequest{
		Network: "test-self-discovery",
		Service: api.ServiceTypeQuery.AddressFor("BVN9"),
	})
	require.ErrorIs(t, err, errRefusing)
}

var errRefusing = context.Canceled

type refusingDiscoverer struct{}

func (refusingDiscoverer) Discover(context.Context, *dial.DiscoveryRequest) (dial.DiscoveryResponse, error) {
	return nil, errRefusing
}
