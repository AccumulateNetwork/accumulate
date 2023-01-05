// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"io"
	"testing"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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
	host := newMockDialerHost(t)
	host.EXPECT().selfID().Return("").Maybe()
	host.EXPECT().getOwnService(mock.Anything).Return(nil, false).Maybe()
	host.EXPECT().getPeerService(mock.Anything, mock.Anything, mock.Anything).Return(fakeStream{}, nil).Maybe()
	peers := newMockDialerPeers(t)
	peers.EXPECT().getPeer(pid).Return(nil, true).Maybe()
	peers.EXPECT().getPeers(mock.Anything).Return([]*peerState{{info: &Info{ID: pid}}}).Maybe()
	peers.EXPECT().adjustPriority(mock.Anything, mock.Anything).Maybe()

	cases := map[string]struct {
		Ok   bool
		Addr string
	}{
		"peer":           {false, "/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N"},
		"partition":      {true, "/acc/query:foo"},
		"partition-peer": {true, "/acc/query:foo/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N"},
		"peer-partition": {true, "/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N/acc/query:foo"},
		"partition-tcp":  {false, "/acc/query:foo/tcp/123"},
		"tcp-partition":  {false, "/tcp/123/acc/query:foo"},
	}

	dialer := &dialer{host, peers}
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
	host.EXPECT().getOwnService(mock.Anything).Return(&service{handler: handler.Execute}, true)

	dialer := &dialer{host, nil}
	_, err := dialer.Dial(context.Background(), addr(t, "/acc/query:foo/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N"))
	require.NoError(t, err)
	<-done
}

func TestDialSelfPartition(t *testing.T) {
	// Partition dial requests that match a partition the node participates in are processed directly

	done := make(chan struct{})
	handler := NewMockMessageStreamHandler(t)
	handler.EXPECT().Execute(mock.Anything).Run(func(message.Stream) { close(done) })

	host := newMockDialerHost(t)
	host.EXPECT().getOwnService(mock.Anything).Return(&service{handler: handler.Execute}, true)

	dialer := &dialer{host, nil}
	_, err := dialer.Dial(context.Background(), addr(t, "/acc/query:foo"))
	require.NoError(t, err)
	<-done
}

func TestDialPartition(t *testing.T) {
	// The dialer dials peerList in descending order of priority
	peerList := []*peerState{
		{priority: -1, info: &Info{ID: peerId(t, "QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN")}},
		{priority: 1, info: &Info{ID: peerId(t, "QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa")}},
		{priority: 3, info: &Info{ID: peerId(t, "QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb")}},
		{priority: 0, info: &Info{ID: peerId(t, "QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt")}},
	}

	_sa := &api.ServiceAddress{Type: api.ServiceTypeQuery, Partition: "foo"}
	sa := mock.MatchedBy(func(other *api.ServiceAddress) bool { return _sa.Equal(other) })
	peers := newMockDialerPeers(t)
	peers.EXPECT().getPeers(sa).Return(append(make([]*peerState, 0, len(peerList)), peerList...)) // Copy

	host := newMockDialerHost(t)
	host.EXPECT().getOwnService(sa).Return(nil, false)

	c3 := host.EXPECT().getPeerService(mock.Anything, peerList[2].info.ID, sa).Return(nil, network.ErrNoConn)
	c1 := host.EXPECT().getPeerService(mock.Anything, peerList[1].info.ID, sa).Return(fakeStream{}, nil)
	c1.NotBefore(c3.Call)

	peers.EXPECT().adjustPriority(peerList[2], mock.Anything).NotBefore(c3.Call)
	peers.EXPECT().adjustPriority(peerList[1], mock.Anything).NotBefore(c1.Call)

	dialer := &dialer{host, peers}
	_, err := dialer.Dial(context.Background(), addr(t, "/acc/query:foo"))
	require.NoError(t, err)
}
