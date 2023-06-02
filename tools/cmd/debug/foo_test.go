package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestFOo(t *testing.T) {
	bs, err := multiaddr.NewMultiaddr("/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg")
	require.NoError(t, err)

	node, err := p2p.New(p2p.Options{
		Network:        "MainNet",
		BootstrapPeers: []multiaddr.Multiaddr{bs},
	})
	require.NoError(t, err)
	defer func() { _ = node.Close() }()

	fmt.Printf("We are %v\n", node.ID())

	fmt.Println("Waiting for a live network service")
	svcAddr, err := api.ServiceTypeNetwork.AddressFor(protocol.Directory).MultiaddrFor("MainNet")
	require.NoError(t, err)
	require.NoError(t, node.WaitForService(context.Background(), svcAddr))

	router := new(routing.MessageRouter)
	client := &message.Client{
		Transport: &message.RoutedTransport{
			Network: "MainNet",
			Dialer:  node.DialNetwork(),
			Router:  router,
		},
	}
	ns, err := client.NetworkStatus(context.Background(), api.NetworkStatusOptions{})
	require.NoError(t, err)
	router.Router, err = routing.NewStaticRouter(ns.Routing, nil)
	require.NoError(t, err)

	peer, err := peer.Decode("12D3KooWEzhg3CRvC3xdrUBFsWETF1nG3gyYfEjx4oEJer95y1Rk")
	require.NoError(t, err)

	r, err := client.ForPeer(peer).Private().Sequence(context.Background(), protocol.PartitionUrl("Chandrayaan").JoinPath(protocol.Synthetic), protocol.DnUrl(), 607)
	require.NoError(t, err)
	b, err := json.Marshal(r.Message)
	require.NoError(t, err)
	fmt.Println(string(b))
	for _, set := range r.Signatures.Records {
		for _, sig := range set.Signatures.Records {
			b, err = json.Marshal(sig)
			require.NoError(t, err)
			fmt.Println(string(b))
		}
	}
}
