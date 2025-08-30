// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
)

var cmdTestP2P = &cobra.Command{
	Use:   "test-p2p [network]",
	Short: "Test P2P connectivity to a network",
	Args:  cobra.ExactArgs(1),
	Run:   testP2P,
}

func init() {
	cmd.AddCommand(cmdTestP2P)
}

func testP2P(cmd *cobra.Command, args []string) {
	network := args[0]
	
	fmt.Printf("Testing P2P connectivity to %s...\n", network)
	fmt.Printf("Bootstrap peers: %v\n", bootstrap)
	
	// Create P2P client
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	opts := p2p.Options{
		BootstrapPeers: bootstrap,
	}
	
	// Set network based on argument
	switch network {
	case "mainnet", "MainNet":
		opts.Network = "MainNet"
	case "testnet", "kermit", "Kermit":
		opts.Network = "Kermit"
	default:
		opts.Network = network
	}
	
	fmt.Printf("Creating P2P client for network: %s\n", opts.Network)
	node, err := p2p.NewClient(opts)
	if err != nil {
		fatalf("Failed to create P2P client: %v", err)
	}
	defer node.Close()
	
	fmt.Println("P2P client created, testing network status...")
	
	// Test network status
	status, err := node.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		fatalf("Failed to get network status: %v", err)
	}
	
	fmt.Println("✅ P2P connection successful!")
	fmt.Printf("Network: %s\n", status.Network.NetworkName)
	fmt.Printf("Directory Height: %d\n", status.DirectoryHeight)
	fmt.Printf("Major Block Height: %d\n", status.MajorBlockHeight)
	fmt.Printf("Partitions: %d\n", len(status.Network.Partitions))
	
	// Test node info
	nodeInfo, err := node.NodeInfo(ctx, api.NodeInfoOptions{})
	if err != nil {
		fmt.Printf("Warning: Could not get node info: %v\n", err)
	} else {
		fmt.Printf("Connected to peer: %s\n", nodeInfo.PeerID)
		fmt.Printf("Node version: %s\n", nodeInfo.Version)
	}
}