// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/rpc/client/http"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
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

	r, err := client.ForPeer(peer).Private().Sequence(context.Background(), protocol.PartitionUrl("Chandrayaan").JoinPath(protocol.Synthetic), protocol.DnUrl(), 607, private.SequenceOptions{})
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
