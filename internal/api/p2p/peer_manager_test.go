// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
)

func TestResetPriorities(t *testing.T) {
	// If priorities exceed the max, they are reset

	p1 := peerId(t, "QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN")
	p2 := peerId(t, "QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa")
	p3 := peerId(t, "QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb")
	p4 := peerId(t, "QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt")

	mgr := new(peerManager)
	mgr.mu = new(sync.RWMutex)
	mgr.peers = map[peer.ID]*peerState{
		p1: {priority: -maxPriority - 0},
		p2: {priority: -maxPriority - 2},
		p3: {priority: -maxPriority - 3},
		p4: {priority: -maxPriority - 4},
	}

	mgr.adjustPriority(mgr.peers[p1], -1)

	require.Equal(t, 3, mgr.peers[p1].priority)
	require.Equal(t, 2, mgr.peers[p2].priority)
	require.Equal(t, 1, mgr.peers[p3].priority)
	require.Equal(t, 0, mgr.peers[p4].priority)
}

func noServices() []*service { return nil } //nolint:unused

func TestPeering(t *testing.T) {
	t.Skip("Flaky")

	// Set up a seed
	logger := logging.ConsoleLoggerForTest(t, "info")
	h1, err := libp2p.New()
	require.NoError(t, err)
	defer h1.Close()
	m1, err := newPeerManager(h1, noServices, Options{Logger: logger})
	require.NoError(t, err)

	// Build an address for the seed
	c, err := multiaddr.NewComponent("p2p", h1.ID().String())
	require.NoError(t, err)
	h1addr := h1.Addrs()[0].Encapsulate(c)

	// Setup three more hosts
	h2, err := libp2p.New()
	require.NoError(t, err)
	defer h2.Close()
	h3, err := libp2p.New()
	require.NoError(t, err)
	defer h3.Close()
	h4, err := libp2p.New()
	require.NoError(t, err)
	defer h4.Close()

	m2, err := newPeerManager(h2, noServices, Options{Logger: logger, BootstrapPeers: []multiaddr.Multiaddr{h1addr}})
	require.NoError(t, err)
	m3, err := newPeerManager(h3, noServices, Options{Logger: logger, BootstrapPeers: []multiaddr.Multiaddr{h1addr}})
	require.NoError(t, err)
	m4, err := newPeerManager(h4, noServices, Options{Logger: logger, BootstrapPeers: []multiaddr.Multiaddr{h1addr}})
	require.NoError(t, err)

	// Wait until every node is peered with every other
	for len(m1.peers) < 3 &&
		len(m2.peers) < 3 &&
		len(m3.peers) < 3 &&
		len(m4.peers) < 3 {
		time.Sleep(time.Microsecond)
	}

	// Set up pubsub
	ps2, err := pubsub.NewGossipSub(context.Background(), h2)
	require.NoError(t, err)
	top2, err := ps2.Join("events")
	require.NoError(t, err)

	ps4, err := pubsub.NewGossipSub(context.Background(), h4)
	require.NoError(t, err)
	top4, err := ps4.Join("events")
	require.NoError(t, err)
	sub4, err := top4.Subscribe()
	require.NoError(t, err)

	// Wait for the sub to get connected
	for len(top2.ListPeers()) == 0 {
		time.Sleep(time.Millisecond)
	}

	// Publish
	err = top2.Publish(context.Background(), []byte("foo"))
	require.NoError(t, err)

	// Receive
	msg, err := sub4.Next(context.Background())
	require.NoError(t, err)
	require.Equal(t, "foo", string(msg.Data))
}
