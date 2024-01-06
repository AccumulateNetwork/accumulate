// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/rpc/client/http"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/fatih/color"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/apiutil"
	"gitlab.com/accumulatenetwork/accumulate/exp/light"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/healing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/bolt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestIndexReceivedAnchors(t *testing.T) {
	t.Skip("Manual")

	const network = "kermit"
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	node, err := p2p.New(p2p.Options{
		Network:           network,
		BootstrapPeers:    accumulate.BootstrapServers,
		PeerDatabase:      peerDb,
		EnablePeerTracker: true,

		// Use the peer tracker, but don't update it between reboots
		PeerScanFrequency:    -1,
		PeerPersistFrequency: -1,
	})
	checkf(err, "start p2p node")
	t.Cleanup(func() { require.NoError(t, node.Close()) })

	router, err := apiutil.InitRouter(ctx, node, network)
	check(err)

	dialer := node.DialNetwork()
	mc := &message.Client{
		Transport: &message.RoutedTransport{
			Network: network,
			Dialer:  dialer,
			Router:  routing.MessageRouter{Router: router},
		},
	}

	cu, err := user.Current()
	require.NoError(t, err)
	db, err := bolt.Open(filepath.Join(cu.HomeDir, ".accumulate", "cache", network+".db"), bolt.WithPlainKeys)
	require.NoError(t, err)

	lc, err := light.NewClient(
		light.Store(db, ""),
		light.Server(accumulate.ResolveWellKnownEndpoint(network, "v2")),
		light.Querier(mc),
		light.Router(router),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lc.Close() })

	// Profile this
	err = lc.IndexReceivedAnchors(ctx, url.MustParse("dn.acme"))
	require.NoError(t, err)
}

func TestSyntheticSequence(t *testing.T) {
	b, err := hex.DecodeString("01e35b2000000000000000000000000000000000000000000000000000000000")
	require.NoError(t, err)
	e := new(protocol.IndexEntry)
	require.NoError(t, e.UnmarshalBinary(b))

	b, err = json.MarshalIndent(e, "", "  ")
	require.NoError(t, err)
	fmt.Println(string(b))
}

func TestCheckTransactions(t *testing.T) {
	t.Skip("Manual")

	node, err := p2p.New(p2p.Options{
		Network:        "MainNet",
		BootstrapPeers: accumulate.BootstrapServers,
	})
	require.NoError(t, err)
	defer func() { _ = node.Close() }()

	cu, err := user.Current()
	require.NoError(t, err)
	var netinfo *healing.NetworkInfo
	data, err := os.ReadFile(filepath.Join(cu.HomeDir, ".accumulate", "cache", "mainnet.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &netinfo))

	router := new(routing.MessageRouter)
	router.Router, err = routing.NewStaticRouter(netinfo.Status.Routing, nil)
	require.NoError(t, err)

	req := []struct {
		Partition string
		Account   string
		Name      string
		Start     int
	}{
		{"apollo", "reesor.acme/rewards", "main", 77},
		{"yutu", "ethan.acme/tokens", "main", 2},
		{"chandrayaan", "tfa.acme/staking-yield", "main", 39},
		{"directory", "ACME", "main", 28774},
	}

	for _, r := range req {
		fmt.Println(r.Partition)

		nodes, err := node.Services().FindService(context.Background(), api.FindServiceOptions{Network: "MainNet", Service: api.ServiceTypeConsensus.AddressFor(r.Partition), Timeout: 10 * time.Second})
		require.NoError(t, err)

		for _, p := range nodes {
			var addr string
			for _, a := range p.Addresses {
				s, err := a.ValueForProtocol(multiaddr.P_IP4)
				if err != nil {
					continue
				}
				ip := net.ParseIP(s)
				if ip == nil || isPrivateIP(ip) {
					continue
				}
				addr = s
				break
			}
			if addr == "" {
				fmt.Printf("  ! %v (no address)\n", p.PeerID)
				continue
			}

			var port = 16595
			if !strings.EqualFold(r.Partition, protocol.Directory) {
				port = 16695
			}
			c := jsonrpc.NewClient(fmt.Sprintf("http://%s:%d/v3", addr, port))
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			var expand = true
			var count uint64 = 2
			var failure error
			r, err := api.Querier2{Querier: c}.QueryChainEntries(ctx, url.MustParse(r.Account), &api.ChainQuery{Name: r.Name, Range: &api.RangeOptions{Start: uint64(r.Start), Expand: &expand, Count: &count}})
			switch {
			case err == nil:
				for _, r := range r.Records {
					err, ok := r.Value.(*api.ErrorRecord)
					if ok {
						failure = errors.Code(err.Value)
					}
				}
			default:
				failure = err
			}

			s := " "
			if failure == nil {
				s += " " + color.GreenString("✔")
			} else {
				s += " " + color.RedString("🗴")
			}
			s += " " + p.PeerID.String()
			if failure != nil {
				s += " (" + failure.Error() + ")"
			}
			fmt.Println(s)
		}
	}
}

var privateIPBlocks []*net.IPNet

func init() {
	for _, cidr := range []string{
		"127.0.0.0/8",    // IPv4 loopback
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"169.254.0.0/16", // RFC3927 link-local
		"::1/128",        // IPv6 loopback
		"fe80::/10",      // IPv6 link-local
		"fc00::/7",       // IPv6 unique local addr
	} {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Errorf("parse error on %q: %v", cidr, err))
		}
		privateIPBlocks = append(privateIPBlocks, block)
	}
}

func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

func TestHealSynth(t *testing.T) {
	t.Skip("Manual")

	cu, err := user.Current()
	require.NoError(t, err)

	node, err := p2p.New(p2p.Options{
		Network:           "MainNet",
		BootstrapPeers:    accumulate.BootstrapServers,
		PeerDatabase:      filepath.Join(cu.HomeDir, ".accumulate", "cache", "peerdb.json"),
		PeerScanFrequency: -1,
	})
	require.NoError(t, err)
	defer func() { _ = node.Close() }()

	fmt.Printf("We are %v\n", node.ID())

	router, err := apiutil.InitRouter(context.Background(), node, "MainNet")
	require.NoError(t, err)

	client := &message.Client{
		Transport: &message.RoutedTransport{
			Network: "MainNet",
			Dialer:  node.DialNetwork(),
			Router:  routing.MessageRouter{Router: router},
		},
	}

	peer, err := peer.Decode("12D3KooWLx5DFZXHc6XycUnUkN7eGseZXtdu9ZVfWNC44k8kFwtS")
	require.NoError(t, err)

	r, err := client.ForPeer(peer).Private().Sequence(context.Background(), protocol.PartitionUrl("Chandrayaan").JoinPath(protocol.Synthetic), protocol.PartitionUrl("Yutu"), 5073, private.SequenceOptions{})
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
	t.Skip("Manual")

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

	r, err := client.ForPeer(peer).Private().Sequence(context.Background(),
		protocol.PartitionUrl("Apollo").JoinPath(protocol.Synthetic),
		protocol.PartitionUrl("Apollo"),
		3044,
		private.SequenceOptions{})
	require.NoError(t, err)
	b, err := json.Marshal(r.Message)
	require.NoError(t, err)
	fmt.Println(string(b))
	// for _, set := range r.Signatures.Records {
	// 	for _, sig := range set.Signatures.Records {
	// 		_ = sig
	// 		fmt.Println("got signature")
	// 	}
	// }
}

func TestHealAnchors(t *testing.T) {
	t.Skip("Manual")

	const network = "Kermit"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bs, err := multiaddr.NewMultiaddr("/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg")
	require.NoError(t, err)

	node, err := p2p.New(p2p.Options{
		Network:        network,
		BootstrapPeers: []multiaddr.Multiaddr{bs},
	})
	require.NoError(t, err)
	defer func() { _ = node.Close() }()

	fmt.Printf("We are %v\n", node.ID())

	fmt.Println("Waiting for a live network service")
	svcAddr, err := api.ServiceTypeNetwork.AddressFor(protocol.Directory).MultiaddrFor(network)
	require.NoError(t, err)
	require.NoError(t, node.WaitForService(ctx, svcAddr))

	router := new(routing.MessageRouter)
	client := &message.Client{
		Transport: &message.RoutedTransport{
			Network: network,
			Dialer:  node.DialNetwork(),
			Router:  router,
		},
	}
	ns, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	require.NoError(t, err)
	router.Router, err = routing.NewStaticRouter(ns.Routing, nil)
	require.NoError(t, err)

	peers, err := client.FindService(ctx, api.FindServiceOptions{
		Network: network,
		Service: api.ServiceTypeConsensus.AddressFor(protocol.Directory),
	})
	require.NoError(t, err)

	env := new(messaging.Envelope)
	for _, peer := range peers {
		r, err := client.ForPeer(peer.PeerID).Private().Sequence(ctx, protocol.DnUrl().JoinPath(protocol.AnchorPool), protocol.PartitionUrl("Groucho"), 1, private.SequenceOptions{})
		require.NoError(t, err)

		env.Transaction = []*protocol.Transaction{r.Message.(*messaging.TransactionMessage).Transaction}
		for _, set := range r.Signatures.Records {
			for _, sig := range set.Signatures.Records {
				sig, ok := sig.Message.(messaging.MessageWithSignature)
				if ok {
					env.Signatures = append(env.Signatures, sig.GetSignature())
				}
			}
		}
	}

	sub, err := client.Submit(ctx, env, api.SubmitOptions{})
	require.NoError(t, err)
	for _, st := range sub {
		assert.NoError(t, st.Status.AsError())
		assert.True(t, st.Success, st.Message)
	}
}

func TestBlockResults(t *testing.T) {
	t.Skip("Manual")

	first, err := http.New("http://apollo-mainnet.accumulate.defidevs.io:16592", "http://apollo-mainnet.accumulate.defidevs.io:16592/ws")
	require.NoError(t, err)

	ni, err := first.NetInfo(context.Background())
	require.NoError(t, err)

	var nodes []*http.HTTP
	for _, peer := range ni.Peers {
		u := fmt.Sprintf("http://%s:16592", peer.RemoteIP)
		n, err := http.New(u, u+"/ws")
		require.NoError(t, err)
		nodes = append(nodes, n)
	}

	var block int64 = 12614506
	firstResult, err := first.BlockResults(context.Background(), &block)
	require.NoError(t, err)

	for _, r := range firstResult.TxsResults {
		rs := new(protocol.TransactionResultSet)
		require.NoError(t, rs.UnmarshalBinary(r.Data))
		for _, r := range rs.Results {
			if r.Result == nil {
				fmt.Println("nil result")
			}
		}
	}
	t.FailNow()

	for i, n := range nodes {
		t.Run(ni.Peers[i].RemoteIP, func(t *testing.T) {
			r, err := n.BlockResults(context.Background(), &block)
			require.NoError(t, err)
			equalResults(t, firstResult, r)
		})
	}
}

func equalResults(t *testing.T, a, b *coretypes.ResultBlockResults) {
	if len(a.TxsResults) != len(b.TxsResults) {
		t.Fatal("Different number of results")
	}

	for i, a := range a.TxsResults {
		b := b.TxsResults[i]
		if a.Code != b.Code {
			t.Log("different Code for", i)
			t.Fail()
		}
		if !bytes.Equal(a.Data, b.Data) {
			// t.Log("different Data for", i)
			t.Fail()

			c := new(protocol.TransactionResultSet)
			require.NoError(t, c.UnmarshalBinary(a.Data))
			d := new(protocol.TransactionResultSet)
			require.NoError(t, d.UnmarshalBinary(b.Data))

			if len(c.Results) != len(d.Results) {
				t.Log("different number of results for", i)
			} else {
				for j, a := range c.Results {
					b := d.Results[j]
					if a.Equal(b) {
						continue
					}

					cmpEq(t, a.TxID, b.TxID, i, j, "TxID")
					cmp2(t, a.Code, b.Code, i, j, "Code")
					cmpEq(t, a.Error, b.Error, i, j, "Error")
					cmp3(t, a.Result, b.Result, i, j, "Result")
					cmp2(t, a.Received, b.Received, i, j, "Received")
					cmpEq(t, a.Initiator, b.Initiator, i, j, "Initiator")
					cmp3(t, a.Signers, b.Signers, i, j, "Signers")
					cmpEq(t, a.SourceNetwork, b.SourceNetwork, i, j, "SourceNetwork")
					cmpEq(t, a.DestinationNetwork, b.DestinationNetwork, i, j, "DestinationNetwork")
					cmp2(t, a.SequenceNumber, b.SequenceNumber, i, j, "SequenceNumber")
					cmp2(t, a.GotDirectoryReceipt, b.GotDirectoryReceipt, i, j, "GotDirectoryReceipt")
					cmpEq(t, a.Proof, b.Proof, i, j, "Proof")
					cmp3(t, a.AnchorSigners, b.AnchorSigners, i, j, "AnchorSigners")
				}
			}
		}
		if a.Log != b.Log {
			t.Log("different Log for", i)
			t.Fail()
		}
		if a.Info != b.Info {
			t.Log("different Info for", i)
			t.Fail()
		}
		if a.GasWanted != b.GasWanted {
			t.Log("different GasWanted for", i)
			t.Fail()
		}
		if a.GasUsed != b.GasUsed {
			t.Log("different GasUsed for", i)
			t.Fail()
		}
		if a.Codespace != b.Codespace {
			t.Log("different Codespace for", i)
			t.Fail()
		}
		if t.Failed() {
			printTxResult(t, "A", a)
			printTxResult(t, "B", b)
			// t.FailNow()
		}
	}
}

func cmpEq[T interface {
	Equal(T) bool
	comparable
}](t *testing.T, a, b T, i, j int, field string) {
	var z T
	if a == b {
		return
	}
	switch {
	case a == z:
		t.Logf("Different result for %d part %d: field %s: A is zero but B is not", i, j, field)
	case b == z:
		t.Logf("Different result for %d part %d: field %s: B is zero but A is not", i, j, field)
	case !a.Equal(b):
		t.Logf("Different result for %d part %d: field %s: A and B are both set but not equal", i, j, field)
	}
}

func cmp2[T comparable](t *testing.T, a, b T, i, j int, field string) {
	if a == b {
		return
	}
	t.Logf("Different result for %d part %d: field %s", i, j, field)
}

func cmp3(t *testing.T, a, b any, i, j int, field string) {
	if assert.ObjectsAreEqual(a, b) {
		return
	}
	switch {
	case a == nil:
		t.Logf("Different result for %d part %d: field %s: A is zero but B is not", i, j, field)
	case b == nil:
		t.Logf("Different result for %d part %d: field %s: B is zero but A is not", i, j, field)
	case !assert.ObjectsAreEqual(a, b):
		t.Logf("Different result for %d part %d: field %s: A and B are both set but not equal", i, j, field)
	}
}

func printTxResult(t *testing.T, prefix string, r *abci.ExecTxResult) {
	// fmt.Printf("Code:\t%v\n", r.Code)
	// // fmt.Printf("Data:\t%v\n", r.Data)
	// fmt.Printf("Log:\t%v\n", r.Log)
	// fmt.Printf("Info:\t%v\n", r.Info)
	// fmt.Printf("GasWanted:\t%v\n", r.GasWanted)
	// fmt.Printf("GasUsed:\t%v\n", r.GasUsed)
	// // fmt.Printf("Events:\t%v\n", r.Events)
	// fmt.Printf("Codespace:\t%v\n", r.Codespace)

	rset := new(protocol.TransactionResultSet)
	require.NoError(t, rset.UnmarshalBinary(r.Data))
	// fmt.Println("Data:")
	for _, r := range rset.Results {

		b, err := json.Marshal(r)
		require.NoError(t, err)
		fmt.Printf("  %s %s\n", prefix, b)
	}
}

func TestCheckForZombie(t *testing.T) {
	t.Skip("Manual")

	base := "http://35.183.112.161:16692"
	c, err := http.New(base, base+"/ws")
	require.NoError(t, err)

	ok, err := nodeIsZombie(context.Background(), c)
	require.NoError(t, err)
	fmt.Println("Zombie:", ok)
}
