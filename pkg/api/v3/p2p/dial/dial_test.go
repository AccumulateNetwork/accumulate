// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dial

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"runtime"
	"sync"
	"testing"
	"time"

	ic "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
)

type fakeStream struct{}

func (fakeStream) Read() (message.Message, error) { return nil, io.EOF }
func (fakeStream) Write(message.Message) error    { return io.EOF }
func (fakeStream) Close() error                   { return nil }

func peerId(t testing.TB, s string) peer.ID {
	id, err := peer.Decode(s)
	require.NoError(t, err)
	return id
}

func addr(t testing.TB, s string) multiaddr.Multiaddr {
	addr, err := multiaddr.NewMultiaddr(s)
	require.NoError(t, err)
	return addr
}

func TestDialAddress(t *testing.T) {
	// 1. The partition must be specified (/acc/{id})
	// 2. The node may be specified (/p2p/{id})
	// 3. The order of the node and partition do not matter
	// 4. No other protocols are allowed

	pid := peerId(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
	ch := make(chan peer.AddrInfo, 1)

	host := NewMockConnector(t)
	host.EXPECT().Connect(mock.Anything, mock.Anything).Return(fakeStream{}, nil).Maybe()
	peers := NewMockDiscoverer(t)
	peers.EXPECT().Discover(mock.Anything, mock.Anything).Return(DiscoveredPeers(ch), nil).Run(func(context.Context, *DiscoveryRequest) {
		ch <- peer.AddrInfo{ID: pid}
	}).Maybe()

	cases := map[string]struct {
		Ok   bool
		Addr string
	}{
		"peer":                  {false, "/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N"},
		"network-partition":     {true, "/acc/foo/acc-svc/query:foo"},
		"partition-network":     {true, "/acc-svc/query:foo/acc/foo"},
		"partition-peer":        {true, "/acc-svc/query:foo/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N"},
		"peer-partition":        {true, "/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N/acc-svc/query:foo"},
		"network-partition-tcp": {false, "/acc/foo/acc-svc/query:foo/tcp/123"},
		"tcp-network-partition": {false, "/tcp/123/acc/foo/acc-svc/query:foo"},
	}

	dialer := &dialer{host: host, peers: peers, tracker: fakeTracker{}}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := dialer.Dial(context.Background(), addr(t, c.Addr))
			if c.Ok {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func newPeer(t *testing.T, seed ...any) peer.ID {
	h := storage.MakeKey(seed...)
	std := ed25519.NewKeyFromSeed(h[:])
	k, _, err := ic.KeyPairFromStdKey(&std)
	require.NoError(t, err)
	id, err := peer.IDFromPrivateKey(k)
	require.NoError(t, err)
	return id
}

func TestDialServices1(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	services := []*api.ServiceAddress{
		api.ServiceTypeNode.Address(),
		api.ServiceTypeConsensus.AddressFor("Directory"),
		api.ServiceTypeNetwork.AddressFor("Directory"),
		api.ServiceTypeMetrics.AddressFor("Directory"),
		api.ServiceTypeQuery.AddressFor("Directory"),
		api.ServiceTypeEvent.AddressFor("Directory"),
		api.ServiceTypeSubmit.AddressFor("Directory"),
		api.ServiceTypeValidate.AddressFor("Directory"),
	}

	// Create some random peer IDs
	selfPeerID := newPeer(t, 1)
	var badPeerIDs, goodPeerIDs []peer.ID

	numGoodPeers := 10
	numBadPeers := 13

	peerMap := make(map[string]int)

	for i := 0; i < numGoodPeers; i++ {
		peerID := newPeer(t, i+2)
		peerMap[peerID.String()] = i + 2
		goodPeerIDs = append(goodPeerIDs, peerID)
	}
	for i := 0; i < numBadPeers; i++ {
		peerID := newPeer(t, i+10000)
		peerMap[peerID.String()] = i + 10000
		badPeerIDs = append(badPeerIDs, peerID)
	}

	// Set up the host mock
	host := newFakeHost(selfPeerID)
	for _, addr := range services {
		for _, gpi := range goodPeerIDs {
			host.set(gpi.String() + "|" + addr.String())
		}
	}

	// Setup the peers mock
	peers := staticPeers(func(ctx context.Context, req *DiscoveryRequest) []peer.AddrInfo {
		var all []peer.AddrInfo
		for _, p := range goodPeerIDs {
			all = append(all, peer.AddrInfo{ID: p})
		}
		for _, p := range badPeerIDs {
			all = append(all, peer.AddrInfo{ID: p})
		}
		return all
	})

	tracker := new(SimpleTracker)
	dialer := &dialer{host: host, peers: peers, tracker: tracker}

	start := time.Now()
	wg := new(sync.WaitGroup)
	for _, service := range services {
		for i := 0; i < numGoodPeers+numBadPeers; i++ {
			for j := 0; j < 1; j++ {
				fmt.Printf("loop %d-%d\n", i, j)
				s, err := dialer.newNetworkStream(ctx, service, "MainNet", wg)
				require.NoError(t, err)
				p := peerMap[s.(*stream).peer.String()]
				fmt.Printf("%10d %40s %v\n", p, service.String(), time.Since(start))
				runtime.Gosched()
			}
		}
	}

	// Wait for async operations to finish
	wg.Wait()

	for _, service := range services {
		addr, err := service.MultiaddrFor("MainNet")
		require.NoError(t, err)
		require.Len(t, tracker.good[addr.String()].All(), 10)
	}
}

type staticPeers func(ctx context.Context, req *DiscoveryRequest) []peer.AddrInfo

func (f staticPeers) Discover(ctx context.Context, req *DiscoveryRequest) (DiscoveryResponse, error) {
	list := f(ctx, req)
	if len(list) > req.Limit {
		list = list[:req.Limit]
	}
	ch := make(chan peer.AddrInfo, len(list))
	for _, x := range list {
		ch <- x
	}
	close(ch)
	return DiscoveredPeers(ch), nil
}

type fakeHost struct {
	sync.RWMutex
	peerID peer.ID
	good   map[string]bool
}

func newFakeHost(self peer.ID) *fakeHost {
	return &fakeHost{
		peerID: self,
		good:   map[string]bool{},
	}
}

func (h *fakeHost) set(id string) {
	h.Lock()
	defer h.Unlock()
	h.good[id] = true
}

func (h *fakeHost) clear(id string) {
	h.Lock()
	defer h.Unlock()
	delete(h.good, id)
}

func (h *fakeHost) Connect(ctx context.Context, req *ConnectionRequest) (message.Stream, error) {
	h.RLock()
	defer h.RUnlock()
	if h.good[req.PeerID.String()+"|"+req.Service.String()] {
		return fakeStream{}, nil
	}
	// The specific error doesn't matter but this one won't get logged
	return nil, swarm.ErrDialBackoff
}

func TestDialServices2(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	services := []*api.ServiceAddress{
		api.ServiceTypeNode.Address(),
		api.ServiceTypeConsensus.AddressFor("Directory"),
		api.ServiceTypeNetwork.AddressFor("Directory"),
		api.ServiceTypeMetrics.AddressFor("Directory"),
		api.ServiceTypeQuery.AddressFor("Directory"),
		api.ServiceTypeEvent.AddressFor("Directory"),
		api.ServiceTypeSubmit.AddressFor("Directory"),
		api.ServiceTypeValidate.AddressFor("Directory"),
	}

	// Create some random peer IDs
	selfPeerID := newPeer(t, 1)
	var badPeerIDs, goodPeerIDs []peer.ID

	numPeers := 20

	peerMap := make(map[string]int) // Tracking our peers by the indexes in this

	for i := 0; i < numPeers; i++ {
		peerID := newPeer(t, i+2)
		peerMap[peerID.String()] = i + 2
		goodPeerIDs = append(goodPeerIDs, peerID)
	}

	// Set up the host mock
	host := newFakeHost(selfPeerID)
	for _, addr := range services {
		for _, gpi := range goodPeerIDs {
			host.set(gpi.String() + "|" + addr.String())
		}
	}

	// Setup the peers mock
	peers := staticPeers(func(ctx context.Context, req *DiscoveryRequest) []peer.AddrInfo {
		var all []peer.AddrInfo
		for _, p := range goodPeerIDs {
			all = append(all, peer.AddrInfo{ID: p})
		}
		for _, p := range badPeerIDs {
			all = append(all, peer.AddrInfo{ID: p})
		}
		return all
	})

	dialer := &dialer{host: host, peers: peers, tracker: fakeTracker{}}

	start := time.Now()
	service := services[0]

	dialAPeer := func() {
		addr, e1 := service.MultiaddrFor("MainNet")
		require.NoError(t, e1)
		Stream, e2 := dialer.Dial(ctx, addr)
		require.NoError(t, e2)
		p := peerMap[Stream.(*stream).peer.String()]
		fmt.Printf("%10d %40s %v\n", p, service.String(), time.Since(start))
	}
	moveAPeer := func(src, dest []peer.ID) (source, destination []peer.ID, peer peer.ID, sourceLen int) {
		numSrc := len(src)
		if numSrc > 0 {
			peer = src[0]               // peer moved
			dest = append(dest, src[0]) // Add that peer to our bad peer list
			src[0] = src[numSrc-1]      // Remove the peer from the good peer list
			src = src[:numSrc-1]
			numSrc--
		}
		return src, dest, peer, numSrc
	}
	invalidateOne := func() int {
		src, dest, peer, numSrc := moveAPeer(goodPeerIDs, badPeerIDs)
		goodPeerIDs = src
		badPeerIDs = dest
		fmt.Printf("Remove %d\n", peerMap[peer.String()])
		host.clear(peer.String() + "|" + service.String())
		return numSrc
	}
	validateOne := func() int {
		src, dest, peer, numSrc := moveAPeer(badPeerIDs, goodPeerIDs)
		badPeerIDs = src
		goodPeerIDs = dest
		fmt.Printf("Add    %d\n", peerMap[peer.String()])
		host.set(peer.String() + "|" + service.String())
		return numSrc
	}

	grow := false
	for i := 0; i < 550; i++ {
		fmt.Println("test ", i+1)
		dialAPeer()
		if grow {
			if n := validateOne(); n == 0 {
				grow = false
			}
		} else {
			if n := invalidateOne(); n == 1 {
				grow = true
			}
		}
	}

	for _, service := range services {
		addr, err := service.MultiaddrFor("MainNet")
		require.NoError(t, err)
		_ = addr
		//		require.Len(t, dialer.goodPeers[addr.String()], 10)
	}
}
