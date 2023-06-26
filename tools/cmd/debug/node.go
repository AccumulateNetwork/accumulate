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
	"time"

	"github.com/fatih/color"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
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
	peerAddr, err := multiaddr.NewMultiaddr("/dns/" + args[0] + "/tcp/16593/p2p/" + ni.PeerID.String())
	check(err)
	pc, err := p2p.New(p2p.Options{
		Network:        ni.Network,
		Key:            key,
		BootstrapPeers: []multiaddr.Multiaddr{peerAddr},
	})
	check(err)
	time.Sleep(time.Second)

	svcAddr, err := multiaddr.NewMultiaddr("/acc-svc/node/p2p/" + ni.PeerID.String())
	check(err)
	_, err = pc.DialNetwork().Dial(ctx, svcAddr)
	if err == nil {
		fmt.Println(color.GreenString("✔"), "Direct connection")
	} else {
		fmt.Println(color.RedString("🗴"), "Direct connection")
		fmt.Println(err)
	}
	check(pc.Close())

	// Indirect
	pc, err = p2p.New(p2p.Options{
		Network:        ni.Network,
		Key:            key,
		BootstrapPeers: api.BootstrapServers,
	})
	check(err)
	time.Sleep(time.Second)

	_, err = pc.DialNetwork().Dial(ctx, svcAddr)
	if err == nil {
		fmt.Println(color.GreenString("✔"), "Indirect connection")
	} else {
		fmt.Println(color.RedString("🗴"), "Indirect connection")
		fmt.Println(err)
	}
	check(pc.Close())
}
