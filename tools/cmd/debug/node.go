// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"

	"github.com/fatih/color"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
)

var cmdCheckNode = &cobra.Command{
	Use:   "check-node [address]",
	Short: "Check a node's connectivity",
	Args:  cobra.ExactArgs(1),
	Run:   checkNode,
}

func init() {
	cmd.AddCommand(cmdCheckNode)
}

func checkNode(_ *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jc := jsonrpc.NewClient("http://" + args[0] + ":16595/v3")
	ns, err := jc.NetworkStatus(ctx, api.NetworkStatusOptions{})
	check(err)
	router := routing.NewRouter(routing.RouterOptions{Initial: ns.Routing})

	ni, err := jc.NodeInfo(ctx, api.NodeInfoOptions{})
	check(err)
	fmt.Printf("Peer ID:\t%v\n", ni.PeerID)
	fmt.Printf("Network:\t%v\n", ni.Network)

	cs, err := jc.ConsensusStatus(ctx, api.ConsensusStatusOptions{})
	check(err)
	fmt.Printf("Validator:\t%x\n", cs.ValidatorKeyHash)

	_, val, ok := ns.Network.ValidatorByHash(cs.ValidatorKeyHash[:])
	if ok && val.Operator != nil {
		fmt.Printf("Operator:\t%v\n", val.Operator)
	}

	fmt.Println()

	// Direct
	_, key, err := ed25519.GenerateKey(rand.Reader)
	check(err)
	peerAddrTCP, err := multiaddr.NewMultiaddr("/dns/" + args[0] + "/tcp/16593/p2p/" + ni.PeerID.String())
	check(err)
	peerAddrUDP, err := multiaddr.NewMultiaddr("/dns/" + args[0] + "/udp/16593/quic/p2p/" + ni.PeerID.String())
	check(err)
	pc, err := p2p.New(p2p.Options{
		Network: ni.Network,
		Key:     key,
		BootstrapPeers: []multiaddr.Multiaddr{
			peerAddrTCP,
			peerAddrUDP,
		},
	})
	check(err)

	svcAddr, err := multiaddr.NewMultiaddr("/p2p/" + ni.PeerID.String())
	check(err)
	svcAddr = api.ServiceTypeNode.Address().Multiaddr().Encapsulate(svcAddr)
	_, err = pc.DialNetwork().Dial(ctx, svcAddr)
	if err == nil {
		fmt.Println(color.GreenString("✔"), "Can connect")
	} else {
		fmt.Println(color.RedString("🗴"), "Can connect")
		fmt.Println(err)
	}

	mc := &message.Client{Transport: &message.RoutedTransport{
		Network: ni.Network,
		Dialer:  pc.DialNetwork(),
		Router:  routing.MessageRouter{Router: router},
	}}
	_, err = mc.NodeInfo(ctx, api.NodeInfoOptions{PeerID: ni.PeerID})
	if err == nil {
		fmt.Println(color.GreenString("✔"), "Can query")
	} else {
		fmt.Println(color.RedString("🗴"), "Can query")
		fmt.Println(err)
	}

	check(pc.Close())
}
