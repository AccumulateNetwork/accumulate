// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestHealSynth(t *testing.T) {
	t.Skip("Manual")

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

	r, err := client.ForPeer(peer).Private().Sequence(context.Background(), protocol.PartitionUrl("Yutu").JoinPath(protocol.Synthetic), protocol.DnUrl(), 1075)
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

func TestHealQueryAnchor(t *testing.T) {
	// t.Skip("Manual")

	peer, err := peer.Decode("12D3KooWAgrBYpWEXRViTnToNmpCoC3dvHdmR6m1FmyKjDn1NYpj")
	require.NoError(t, err)

	var mainnetAddrs = func() []multiaddr.Multiaddr {
		s := []string{
			"/dns/apollo-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWAgrBYpWEXRViTnToNmpCoC3dvHdmR6m1FmyKjDn1NYpj",
			"/dns/yutu-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWDqFDwjHEog1bNbxai2dKSaR1aFvq2LAZ2jivSohgoSc7",
			"/dns/chandrayaan-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWHzjkoeAqe7L55tAaepCbMbhvNu9v52ayZNVQobdEE1RL",
			"/ip4/116.202.214.38/tcp/16593/p2p/12D3KooWBkJQiuvotpMemWBYfAe4ctsVHi7fLvT8RT83oXJ5dsgV",
			"/ip4/83.97.19.82/tcp/16593/p2p/12D3KooWHSbqS6K52d4ReauHAg4n8MFbAKkdEAae2fZXnzRYi9ce",
			"/ip4/206.189.97.165/tcp/16593/p2p/12D3KooWHyA7zgAVqGvCBBJejgvKzv7DQZ3LabJMWqmCQ9wFbT3o",
			"/ip4/144.76.105.23/tcp/16593/p2p/12D3KooWS2Adojqun5RV1Xy4k6vKXWpRQ3VdzXnW8SbW7ERzqKie",
			"/ip4/18.190.77.236/tcp/16593/p2p/12D3KooWP1d9vUJCzqX5bTv13tCHmVssJrgK3EnJCC2C5Ep2SXbS",
			"/ip4/3.28.207.55/tcp/16593/p2p/12D3KooWEzhg3CRvC3xdrUBFsWETF1nG3gyYfEjx4oEJer95y1Rk",
			"/ip4/38.135.195.81/tcp/16593/p2p/12D3KooWDWCHGAyeUWdP8yuuSYvMoUfaPoGu4p3gJb51diqNQz6j",
			// "/ip4/50.17.246.3/tcp/16593/p2p/12D3KooWKkNsxkHJqvSje2viyqKVxtqvbTpFrbASD3q1uv6td1pW",
			"/dns/validator-eu01.acme.sphereon.com/tcp/16593/p2p/12D3KooWKYTWKJ5jeuZmbbwiN7PoinJ2yJLoQtZyfWi2ihjBnSUR",
			"/ip4/35.86.120.53/tcp/16593/p2p/12D3KooWKJuspMDC5GXzLYJs9nHwYfqst9QAW4m5FakXNHVMNiq7",
			"/ip4/65.109.48.173/tcp/16593/p2p/12D3KooWHkUtGcHY96bNavZMCP2k5ps5mC7GrF1hBC1CsyGJZSPY",
			"/dns/accumulate.detroitledger.tech/tcp/16593/p2p/12D3KooWNe1QNh5mKAa8iAEP8vFwvmWFxaCLNcAdE1sH38Bz8sc9",
			"/ip4/3.135.9.97/tcp/16593/p2p/12D3KooWEQG3X528Ct2Kd3kxhv6WZDBqaAoEw7AKiPoK1NmWJgx1",
			// "/ip4/3.86.85.133/tcp/16593/p2p/12D3KooWJvReA1SuLkppyXKXq6fifVPLqvNtzsvPUqagVjvYe7qe",
			"/ip4/193.35.56.176/tcp/16593/p2p/12D3KooWJevZUFLqN7zAamDh2EEYNQZPvxGFwiFVyPXfuXZNjg1J",
			"/ip4/35.177.70.195/tcp/16593/p2p/12D3KooWPzpRp1UCu4nvXT9h8jKvmBmCADrMnoF72DrEbUrWrB2G",
			"/ip4/3.99.81.122/tcp/16593/p2p/12D3KooWLL5kAbD7nhv6CM9x9L1zjxSnc6hdMVKcsK9wzMGBo99X",
			"/ip4/34.219.75.234/tcp/16593/p2p/12D3KooWKHjS5nzG9dipBXn31pYEnfa8g5UzvkSYEsuiukGHzPvt",
			"/ip4/3.122.254.53/tcp/16593/p2p/12D3KooWRU8obVzgfw6TsUHjoy2FDD3Vd7swrPNTM7DMFs8JG4dx",
			"/ip4/35.92.228.236/tcp/16593/p2p/12D3KooWQqMqbyJ2Zay9KHeEDgDMAxQpKD1ypiBX5ByQAA2XpsZL",
			"/ip4/3.135.184.194/tcp/16593/p2p/12D3KooWHcxyiE3AGdPnhtj87tByfLnJZVR6mLefadWccbMByrBa",
			"/ip4/18.133.170.113/tcp/16593/p2p/12D3KooWFbWY2NhBEWTLHUCwwPmNHm4BoJXbojnrJJfuDCVoqrFY",
			// "/ip4/44.204.224.126/tcp/16593/p2p/12D3KooWAiJJxdgsB39up5h6fz6TSfBz4HsLKTFiBXUrbwA8o54m",
			"/ip4/35.92.21.90/tcp/16593/p2p/12D3KooWLTV3pTN2NbKeFeseCGHyMXuAkQv68KfCeK4uqJzJMfhZ",
			"/ip4/3.99.166.147/tcp/16593/p2p/12D3KooWGYUf93iYWsUibSvKdxsYUY1p7fC1nQotCpUcDXD1ABvR",
			"/ip4/16.171.4.135/tcp/16593/p2p/12D3KooWEMpAxKnXJPkcEXpDmrnjrZ5iFMZvvQtimmTTxuoRGkXV",
			"/ip4/54.237.244.42/tcp/16593/p2p/12D3KooWLoMkrgW862Gs152jLt6FiZZs4GkY24Su4QojnvMoSNaQ",
			// "/ip4/3.238.124.43/tcp/16593/p2p/12D3KooWJ8CA8pacTnKWVgBSEav4QG1zJpyeSSME47RugpDUrZp8",
			"/ip4/13.53.125.115/tcp/16593/p2p/12D3KooWBJk52fQExXHWhFNk692hP7JvTxNTvUMdVne8tbJ3DBf3",
			"/ip4/13.59.241.224/tcp/16593/p2p/12D3KooWKjYKqg2TgUSLq8CZAP8G6LhjXUWTcQBd9qYL2JHug9HW",
			"/ip4/18.168.202.86/tcp/16593/p2p/12D3KooWDiKGbUZg1rB5EufRCkRPiDCEPMjyvTfTVR9qsKVVkcuC",
			"/ip4/35.183.112.161/tcp/16593/p2p/12D3KooWFPKeXzKMd3jtoeG6ts6ADKmVV8rVkXR9k9YkQPgpLzd6",
		}
		addrs := make([]multiaddr.Multiaddr, len(s))
		for i, s := range s {
			addr, err := multiaddr.NewMultiaddr(s)
			if err != nil {
				panic(err)
			}
			addrs[i] = addr
		}
		return addrs
	}()

	node, err := p2p.New(p2p.Options{
		Network: "MainNet",
		// BootstrapPeers: api.BootstrapServers,
		BootstrapPeers: mainnetAddrs,
	})
	require.NoError(t, err)
	defer func() { _ = node.Close() }()

	fmt.Printf("We are %v\n", node.ID())
	time.Sleep(time.Second)

	// fmt.Println("Waiting for a live network service")
	// svcAddr, err := api.ServiceTypeNetwork.AddressFor(protocol.Directory).MultiaddrFor("MainNet")
	// require.NoError(t, err)
	// require.NoError(t, node.WaitForService(context.Background(), svcAddr))

	router := new(routing.MessageRouter)
	client := &message.Client{
		Transport: &message.RoutedTransport{
			Network: "MainNet",
			Dialer:  node.DialNetwork(),
			Router:  router,
		},
	}
	// ns, err := client.NetworkStatus(context.Background(), api.NetworkStatusOptions{})
	// require.NoError(t, err)
	// router.Router, err = routing.NewStaticRouter(ns.Routing, nil)
	// require.NoError(t, err)

	r, err := client.ForPeer(peer).Private().Sequence(context.Background(), protocol.DnUrl().JoinPath(protocol.AnchorPool), protocol.PartitionUrl("Apollo"), 70911)
	require.NoError(t, err)
	// b, err := json.Marshal(r.Message)
	// require.NoError(t, err)
	// fmt.Println(string(b))
	for _, set := range r.Signatures.Records {
		for _, sig := range set.Signatures.Records {
			_ = sig
			fmt.Println("got signature")
		}
	}
}
