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

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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
	host.EXPECT().getOwnService(mock.Anything, mock.Anything).Return(&service{handler: handler.Execute}, true)

	dialer := &dialer{host, nil}
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
	host.EXPECT().getOwnService(mock.Anything, mock.Anything).Return(&service{handler: handler.Execute}, true)

	dialer := &dialer{host, nil}
	_, err := dialer.Dial(context.Background(), addr(t, "/acc/foo/acc-svc/query:foo"))
	require.NoError(t, err)
	<-done
}
