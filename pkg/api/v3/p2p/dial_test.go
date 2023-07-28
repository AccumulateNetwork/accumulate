// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	ic "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
)

type fakeStream struct{}

func (fakeStream) Read([]byte) (int, error)  { return 0, io.EOF }
func (fakeStream) Write([]byte) (int, error) { return 0, io.EOF }
func (fakeStream) Close() error              { return nil }

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

	host := newMockDialerHost(t)
	host.EXPECT().selfID().Return("").Maybe()
	host.EXPECT().getOwnService(mock.Anything, mock.Anything).Return(nil, false).Maybe()
	host.EXPECT().getPeerService(mock.Anything, mock.Anything, mock.Anything).Return(fakeStream{}, nil).Maybe()
	peers := newMockDialerPeers(t)
	peers.EXPECT().getPeers(mock.Anything, mock.Anything, mock.Anything).Return(ch, nil).Run(func(context.Context, multiaddr.Multiaddr, int) {
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

	dialer := &dialer{host: host, peers: peers, tracker: &simpleTracker{}, goodPeers: make(map[string][]peer.AddrInfo), mutex: new(sync.Mutex)}
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

func TestDialSelfPeer(t *testing.T) {
	// Node dial requests that match the node's ID are processed directly

	done := make(chan struct{})
	handler := NewMockMessageStreamHandler(t)
	handler.EXPECT().Execute(mock.Anything).Run(func(message.Stream) { close(done) })

	pid := peerId(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
	host := newMockDialerHost(t)
	host.EXPECT().selfID().Return(pid)
	host.EXPECT().getOwnService(mock.Anything, mock.Anything).Return(&serviceHandler{handler: handler.Execute}, true)

	dialer := &dialer{host: host, peers: nil, tracker: &simpleTracker{}, goodPeers: make(map[string][]peer.AddrInfo), mutex: new(sync.Mutex)}
	_, err := dialer.Dial(context.Background(), addr(t, "/acc/foo/acc-svc/query:foo/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N"))
	require.NoError(t, err)
	<-done
}

func TestDialSelfPartition(t *testing.T) {
	// Partition dial requests that match a partition the node participates in are processed directly

	done := make(chan struct{})
	handler := NewMockMessageStreamHandler(t)
	handler.EXPECT().Execute(mock.Anything).Run(func(message.Stream) { close(done) })

	host := newMockDialerHost(t)
	host.EXPECT().getOwnService(mock.Anything, mock.Anything).Return(&serviceHandler{handler: handler.Execute}, true)

	dialer := &dialer{host: host, peers: nil, tracker: &simpleTracker{}, goodPeers: make(map[string][]peer.AddrInfo), mutex: new(sync.Mutex)}
	_, err := dialer.Dial(context.Background(), addr(t, "/acc/foo/acc-svc/query:foo"))
	require.NoError(t, err)
	<-done
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

func TestSimpleTracker(t *testing.T) {
	// Set up 3 peers
	p1 := newPeer(t, 1)
	p2 := newPeer(t, 2)
	p3 := newPeer(t, 3)

	// Test cases
	type T1 struct {
		Peer  peer.ID
		Count int
	}
	cases := []struct {
		Bad  []T1
		Good map[peer.ID]bool
	}{
		// If P1 is bad twice and P2 is bad once, P3 should be the only good peer
		{Bad: []T1{{p1, 2}, {p2, 1}}, Good: map[peer.ID]bool{p3: true}},

		// If all peers are marked bad the same number of times, they should all be good
		{Bad: []T1{{p1, 1}, {p2, 1}, {p3, 1}}, Good: map[peer.ID]bool{p1: true, p2: true, p3: true}},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("Case %d", i), func(t *testing.T) {
			// Make sure the tracker knows about all of the peers
			s := new(simpleTracker)
			s.isBad(p1)
			s.isBad(p2)
			s.isBad(p3)

			for _, b := range c.Bad {
				for i := 0; i < b.Count; i++ {
					s.markBad(b.Peer)
				}
			}

			assert.Equal(t, c.Good[p1], !s.isBad(p1), "Is P1 good?")
			assert.Equal(t, c.Good[p2], !s.isBad(p2), "Is P2 good?")
			assert.Equal(t, c.Good[p3], !s.isBad(p3), "Is P3 good?")
		})
	}
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
	host := &fakeHost{
		peerID: selfPeerID,
		good:   map[string]bool{},
	}
	for _, addr := range services {
		for _, gpi := range goodPeerIDs {
			host.good[gpi.String()+"|"+addr.String()] = true
		}
	}

	// Setup the peers mock
	peers := funcPeers(func(ctx context.Context, ma multiaddr.Multiaddr, limit int) (<-chan peer.AddrInfo, error) {
		ch := make(chan peer.AddrInfo, 100)
		for _, p := range goodPeerIDs {
			ch <- peer.AddrInfo{ID: p}
		}
		for _, p := range badPeerIDs {
			ch <- peer.AddrInfo{ID: p}
		}
		return ch, nil
	})

	dialer := &dialer{host: host, peers: peers, tracker: &simpleTracker{}, goodPeers: make(map[string][]peer.AddrInfo), mutex: new(sync.Mutex)}

	start := time.Now()
	for _, service := range services {

		for i := 0; i < numGoodPeers+numBadPeers; i++ {
			for j := 0; j < 1; j++ {
				fmt.Printf("loop %d-%d\n", i, j)
				addr, e1 := service.MultiaddrFor("MainNet")
				require.NoError(t, e1)
				Stream, e2 := dialer.Dial(ctx, addr)
				require.NoError(t, e2)
				var p int
				if stream2, ok := Stream.(*stream); ok {
					p = peerMap[stream2.peer.String()]
				}
				fmt.Printf("%10d %40s %v\n", p, service.String(), time.Since(start))
			}
		}
	}
	for _, service := range services {
		addr, err := service.MultiaddrFor("MainNet")
		require.NoError(t, err)
		_ = addr
		require.Len(t, dialer.goodPeers[addr.String()], 10)
	}
}

type funcPeers func(ctx context.Context, ma multiaddr.Multiaddr, limit int) (<-chan peer.AddrInfo, error)

func (f funcPeers) getPeers(ctx context.Context, ma multiaddr.Multiaddr, limit int) (<-chan peer.AddrInfo, error) {
	return f(ctx, ma, limit)
}

type fakeHost struct {
	peerID peer.ID
	good   map[string]bool
}

func (h *fakeHost) selfID() peer.ID {
	return h.peerID
}

func (h *fakeHost) getOwnService(network string, sa *api.ServiceAddress) (*serviceHandler, bool) {
	return nil, false
}

func (h *fakeHost) getPeerService(ctx context.Context, peer peer.ID, service *api.ServiceAddress) (io.ReadWriteCloser, error) {
	if h.good[peer.String()+"|"+service.String()] {
		return fakeStream{}, nil
	}
	return nil, errors.New("bad")
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
	host := &fakeHost{
		peerID: selfPeerID,
		good:   map[string]bool{},
	}
	for _, addr := range services {
		for _, gpi := range goodPeerIDs {
			host.good[gpi.String()+"|"+addr.String()] = true
		}
	}

	// Setup the peers mock
	peers := funcPeers(func(ctx context.Context, ma multiaddr.Multiaddr, limit int) (<-chan peer.AddrInfo, error) {
		elements := len(goodPeerIDs) + len(badPeerIDs)
		ch := make(chan peer.AddrInfo, elements)
		for _, p := range goodPeerIDs {
			ch <- peer.AddrInfo{ID: p}
		}
		for _, p := range badPeerIDs {
			ch <- peer.AddrInfo{ID: p}
		}
		return ch, nil
	})

	dialer := &dialer{host: host, peers: peers, tracker: &simpleTracker{}, goodPeers: make(map[string][]peer.AddrInfo), mutex: new(sync.Mutex)}

	start := time.Now()
	service := services[0]

	dialAPeer := func() {
		addr, e1 := service.MultiaddrFor("MainNet")
		require.NoError(t, e1)
		Stream, e2 := dialer.Dial(ctx, addr)
		require.NoError(t, e2)
		var p int
		if stream2, ok := Stream.(*stream); ok {
			p = peerMap[stream2.peer.String()]
		}
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
		dialer.mutex.Lock()
		defer dialer.mutex.Unlock()
		src, dest, peer, numSrc := moveAPeer(goodPeerIDs, badPeerIDs)
		goodPeerIDs = src
		badPeerIDs = dest
		fmt.Printf("Remove %d\n", peerMap[peer.String()])
		delete(host.good, peer.String()+"|"+service.String())
		return numSrc
	}
	validateOne := func() int {
		dialer.mutex.Lock()
		defer dialer.mutex.Unlock()
		src, dest, peer, numSrc := moveAPeer(badPeerIDs, goodPeerIDs)
		badPeerIDs = src
		goodPeerIDs = dest
		fmt.Printf("Add    %d\n", peerMap[peer.String()])
		host.good[peer.String()+"|"+service.String()] = true
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
