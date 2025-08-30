//go:build ignore
// +build ignore

// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
)

func main() {
	fmt.Println("🔍 Testing P2P Connection with Updated Bootstrap Servers")
	fmt.Println("=========================================================")
	
	// Show configured bootstrap servers
	fmt.Println("\n📡 Bootstrap servers configured:")
	for i, addr := range accumulate.BootstrapServers {
		fmt.Printf("  %d. %s\n", i+1, addr)
	}
	
	// Create P2P client node
	fmt.Println("\n🚀 Creating P2P client node...")
	node, err := p2p.NewClient(p2p.Options{
		Network:        "MainNet", 
		BootstrapPeers: accumulate.BootstrapServers,
	})
	if err != nil {
		log.Fatal("Failed to create P2P node:", err)
	}
	defer node.Close()
	
	// Wait for connections
	fmt.Println("⏳ Waiting for peer connections...")
	time.Sleep(3 * time.Second)
	
	// Test network status directly using the node
	fmt.Println("\n📊 Testing network status query...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	status, err := node.NetworkStatus(ctx, v3.NetworkStatusOptions{})
	if err != nil {
		log.Fatal("Failed to get network status:", err)
	}
	
	fmt.Println("✅ Successfully connected via P2P!")
	fmt.Printf("\n📊 Network Status:\n")
	fmt.Printf("  Directory Height: %d\n", status.DirectoryHeight)
	fmt.Printf("  Major Block: %d\n", status.MajorBlockHeight)
	if status.Network != nil {
		fmt.Printf("  Network: %s\n", status.Network.NetworkName)
		fmt.Printf("  Partitions: %d\n", len(status.Network.Partitions))
	}
	
	// Test node info
	fmt.Println("\n🔍 Getting node info...")
	nodeInfo, err := node.NodeInfo(ctx, v3.NodeInfoOptions{})
	if err != nil {
		fmt.Printf("  ⚠️ Could not get node info: %v\n", err)
	} else {
		fmt.Printf("  Peer ID: %s\n", nodeInfo.PeerID)
		fmt.Printf("  Network: %s\n", nodeInfo.Network) 
		fmt.Printf("  Version: %s\n", nodeInfo.Version)
	}
	
	fmt.Println("\n✅ P2P connection test complete!")
}