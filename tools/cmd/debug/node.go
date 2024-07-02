// Copyright 2024 The Accumulate Authors
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
	"net/url"

	"github.com/fatih/color"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdCheckNode = &cobra.Command{
	Use:   "check-node [address]",
	Short: "Check a node's connectivity",
	Args:  cobra.ExactArgs(1),
	Run:   checkNode,
}

var flagCheckNode = struct {
	UseP2P  string
	Network string
}{}

func init() {
	cmd.AddCommand(cmdCheckNode)
	cmdCheckNode.Flags().StringVar(&flagCheckNode.UseP2P, "use-p2p", "", "Use libp2p for the network status check, with the given peer ID")
	cmdCheckNode.Flags().StringVar(&flagCheckNode.Network, "network", "", "The network, required for --use-p2p")

	cmdCheckNode.PreRunE = func(cmd *cobra.Command, args []string) error {
		if flagCheckNode.UseP2P != "" && flagCheckNode.Network == "" {
			return fmt.Errorf("--use-p2p requires --network")
		}
		return nil
	}
}

func checkNode(_ *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	u, err := url.Parse(args[0])
	check(err)
	if u.Host == "" {
		u, err = url.Parse("http://" + args[0])
		check(err)
	}
	if u.Path == "" {
		u.Path = "/v3"
	}

	p2pPort := "16593"
	if u.Port() == "" {
		if flagCheckNode.UseP2P != "" {
			u.Host += ":16593"
		} else {
			u.Host += ":16595"
		}
	} else if flagCheckNode.UseP2P != "" {
		p2pPort = u.Port()
	}

	peerID, network := flagCheckNode.UseP2P, flagCheckNode.Network
	if peerID == "" {
		jc := jsonrpc.NewClient(u.String())
		ns, err := jc.NetworkStatus(ctx, api.NetworkStatusOptions{})
		check(err)

		ni, err := jc.NodeInfo(ctx, api.NodeInfoOptions{})
		check(err)
		peerID, network = ni.PeerID.String(), ni.Network
		fmt.Printf("Peer ID:\t%v\n", ni.PeerID)
		fmt.Printf("Network:\t%v\n", ni.Network)

		cs, err := jc.ConsensusStatus(ctx, api.ConsensusStatusOptions{NodeID: peerID, Partition: protocol.Directory})
		check(err)
		fmt.Printf("Validator:\t%x\n", cs.ValidatorKeyHash)

		_, val, ok := ns.Network.ValidatorByHash(cs.ValidatorKeyHash[:])
		if ok && val.Operator != nil {
			fmt.Printf("Operator:\t%v\n", val.Operator)
		}

		fmt.Println()
	}

	_, key, err := ed25519.GenerateKey(rand.Reader)
	check(err)
	peerAddrTCP, err := multiaddr.NewMultiaddr("/dns/" + u.Hostname() + "/tcp/" + p2pPort + "/p2p/" + peerID)
	check(err)
	peerAddrUDP, err := multiaddr.NewMultiaddr("/dns/" + u.Hostname() + "/udp/" + p2pPort + "/quic/p2p/" + peerID)
	check(err)
	pc, err := p2p.New(p2p.Options{
		Network: network,
		Key:     key,
		BootstrapPeers: []multiaddr.Multiaddr{
			peerAddrTCP,
			peerAddrUDP,
		},
	})
	check(err)
	defer func() { check(pc.Close()) }()

	svcAddr, err := multiaddr.NewMultiaddr("/p2p/" + peerID)
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
		Network: network,
		Dialer:  pc.DialNetwork(),
		Router:  routing.MessageRouter{},
	}}
	pid, err := peer.Decode(peerID)
	check(err)
	ni, err := mc.NodeInfo(ctx, api.NodeInfoOptions{PeerID: pid})
	if err == nil {
		fmt.Println(color.GreenString("✔"), "Can query")
	} else {
		fmt.Println(color.RedString("🗴"), "Can query")
		fmt.Println(err)
	}

	if ni != nil {
		fmt.Println()

		for _, s := range ni.Services {
			fmt.Printf("Has %v\n", s)
		}
	}
}
