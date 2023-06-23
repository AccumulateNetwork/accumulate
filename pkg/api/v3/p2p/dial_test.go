// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"testing"

	ic "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
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

	dialer := &dialer{host, peers, &simpleTracker{}}
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

	dialer := &dialer{host, nil, &simpleTracker{}}
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

	dialer := &dialer{host, nil, &simpleTracker{}}
	_, err := dialer.Dial(context.Background(), addr(t, "/acc/foo/acc-svc/query:foo"))
	require.NoError(t, err)
	<-done
}

func TestSimpleTracker(t *testing.T) {
	newPeer := func(seed ...any) peer.ID {
		h := storage.MakeKey(seed...)
		std := ed25519.NewKeyFromSeed(h[:])
		k, _, err := ic.KeyPairFromStdKey(&std)
		require.NoError(t, err)
		id, err := peer.IDFromPrivateKey(k)
		require.NoError(t, err)
		return id
	}

	// Set up 3 peers
	p1 := newPeer(1)
	p2 := newPeer(2)
	p3 := newPeer(3)

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
