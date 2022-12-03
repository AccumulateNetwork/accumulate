package message_test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/p2p"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

func mustMultiaddr(t *testing.T, s string) multiaddr.Multiaddr {
	a, err := multiaddr.NewMultiaddr(s)
	require.NoError(t, err)
	return a
}

func TestPeering(t *testing.T) {
	// v2c, err := client.New("http://127.0.1.1:26660")
	// require.NoError(t, err)

	// NOTE: This test connects to a node. To run it, you must launch a node and
	// copy its ID and address into BootstrapPeers and peer.Decode below.

	n, err := p2p.New(p2p.Options{
		Logger: logging.NewTestLogger(t, "plain", config.DefaultLogLevels, false),
		BootstrapPeers: []multiaddr.Multiaddr{
			mustMultiaddr(t, "/ip4/127.0.1.1/tcp/26658/p2p/12D3KooWEGiTMiUnA9xuchADrRT4TtmYYbWx6Ybp3imFKESLjSAd"),
		},
		Key:     acctesting.GenerateKey(),
		Moniker: "Client",
	})
	require.NoError(t, err)

	for len(n.Peers()) == 0 {
		time.Sleep(time.Second / 10)
	}
	fmt.Println(n.Peers())

	id, err := peer.Decode("12D3KooWEGiTMiUnA9xuchADrRT4TtmYYbWx6Ybp3imFKESLjSAd")
	require.NoError(t, err)

	client := &message.Client{
		Requester: &message.Transport{
			Source: n.NewPeerStreamer(id),
		},
	}
	ns, err := client.NodeStatus(context.Background(), api.NodeStatusOptions{})
	require.NoError(t, err)
	t.Log(ns)
}

func TestBatch(t *testing.T) {
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.LocalNetwork(t.Name(), 3, 3, net.ParseIP("127.0.1.1"), 30000),
		simulator.Genesis(GenesisTime),
	)

	peers, stop, err := sim.ListenP2P(context.Background())
	require.NoError(t, err)
	t.Cleanup(stop)

	n, err := p2p.New(p2p.Options{
		Logger:         logging.NewTestLogger(t, "plain", config.DefaultLogLevels, false),
		Key:            acctesting.GenerateKey(),
		BootstrapPeers: peers,
		Moniker:        "Sim",
	})
	require.NoError(t, err)

	for len(n.Peers()) == 0 {
		time.Sleep(time.Second / 10)
	}
	fmt.Println(n.Peers())

	client := &message.Client{
		Requester: &message.RoutedRequester{
			Attempts: 3,
			Router:   sim.Router(),
			Source:   n.NewRoutedStreamer(),
		},
	}

	batch := client.Batch(context.Background())
	q1 := batch.Query(protocol.AccountUrl("foo", "bar"), nil)
	q2 := batch.Query(protocol.AccountUrl("foo", "baz"), nil)
	require.NoError(t, batch.Send())

	r1 := <-q1
	r2 := <-q2
	t.Log(r1, r2)
}
