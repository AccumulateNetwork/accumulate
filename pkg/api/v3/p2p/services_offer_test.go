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
)

// TestRegisterServiceIf_AnUnofferedServiceIsStillAnswered — #4366,
// executor.md Sync step 5.
//
// A node in no committee "does not advertise the submit service for that
// partition, does not offer it to its own API ... and answers NotReady to
// Submit". Three consequences and one non-consequence, all at the one choke
// point RegisterService:
//
//   - not listed in NodeInfo, so a peer that asks this node what it serves is
//     not told submit;
//   - not resolved by this node's own dialer, so its own API's client dials a
//     validator instead of reaching a dead end locally (dialer.go:146-152);
//   - not advertised to the DHT;
//   - but the stream handler IS installed, so a peer holding a DHT provider
//     record — which lingers to its TTL whatever this node does — gets the
//     spec's NotReady from the service rather than a dial failure.
func TestRegisterServiceIf_AnUnofferedServiceIsStillAnswered(t *testing.T) {
	node, err := New(Options{
		Network: "test-offer",
		Listen:  addrs(t, "/ip4/127.0.0.1/tcp/0"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = node.Close() })

	handled := make(chan struct{}, 4)
	mark := func(message.Stream) { handled <- struct{}{} }

	query := api.ServiceTypeQuery.AddressFor("BVN3")   // offered: a follower serves reads
	submit := api.ServiceTypeSubmit.AddressFor("BVN3") // not offered: in no committee
	require.True(t, node.RegisterServiceIf(query, mark, func() bool { return true }))
	require.True(t, node.RegisterServiceIf(submit, mark, func() bool { return false }))

	// NodeInfo lists what the node offers, and only that.
	info, err := (*nodeService)(node).NodeInfo(context.Background(), api.NodeInfoOptions{})
	require.NoError(t, err)
	var listed []string
	for _, s := range info.Services {
		listed = append(listed, s.String())
	}
	require.Contains(t, listed, query.String())
	require.NotContains(t, listed, submit.String(), "an unoffered service was advertised in NodeInfo")

	// The node's own dialer resolves the one and not the other.
	d := &selfDiscoverer{node, refusingDiscoverer{}}
	_, ok := d.DiscoverLocal("test-offer", query)
	require.True(t, ok)
	_, ok = d.DiscoverLocal("test-offer", submit)
	require.False(t, ok, "the node offered its own API a service it must not serve")

	// The handler is installed either way: a stale provider record must reach
	// the service and get its NotReady, not a protocol error.
	require.True(t, node.handles(submit), "the handler for an unoffered service must still be installed")
	s, err := node.DialSelf().Dial(context.Background(), api.ServiceTypeSubmit.AddressFor("BVN3").Multiaddr())
	require.NoError(t, err, "an explicit self-dial must still reach the handler")
	require.NotNil(t, s)
	<-handled
}

// TestRegisterServiceIf_NoPredicateIsAlwaysOffered — every existing caller of
// RegisterService is unchanged.
func TestRegisterServiceIf_NoPredicateIsAlwaysOffered(t *testing.T) {
	node, err := New(Options{
		Network: "test-offer-default",
		Listen:  addrs(t, "/ip4/127.0.0.1/tcp/0"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = node.Close() })

	submit := api.ServiceTypeSubmit.AddressFor("BVN1")
	require.True(t, node.RegisterService(submit, func(message.Stream) {}))

	info, err := (*nodeService)(node).NodeInfo(context.Background(), api.NodeInfoOptions{})
	require.NoError(t, err)
	var listed []string
	for _, s := range info.Services {
		listed = append(listed, s.String())
	}
	require.Contains(t, listed, submit.String())

	d := &selfDiscoverer{node, refusingDiscoverer{}}
	_, ok := d.DiscoverLocal("test-offer-default", submit)
	require.True(t, ok)
}

// TestRegisterServiceIf_ThePredicateIsRead — the offer is a read of the
// current committee, not a latch taken at registration: a node listed again
// after its membership changes answers from the predicate. (What is NOT
// re-taken is the DHT advertisement; see #4366's note.)
func TestRegisterServiceIf_ThePredicateIsRead(t *testing.T) {
	node, err := New(Options{
		Network: "test-offer-live",
		Listen:  addrs(t, "/ip4/127.0.0.1/tcp/0"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = node.Close() })

	in := false
	submit := api.ServiceTypeSubmit.AddressFor("BVN2")
	require.True(t, node.RegisterServiceIf(submit, func(message.Stream) {}, func() bool { return in }))

	d := &selfDiscoverer{node, refusingDiscoverer{}}
	_, ok := d.DiscoverLocal("test-offer-live", submit)
	require.False(t, ok)

	in = true
	_, ok = d.DiscoverLocal("test-offer-live", submit)
	require.True(t, ok, "the offer must be read each time, not latched at registration")
}
