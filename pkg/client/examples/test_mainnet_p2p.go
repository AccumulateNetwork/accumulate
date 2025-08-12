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

	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"github.com/multiformats/go-multiaddr"
)

func main() {
	fmt.Println("🔍 Testing Mainnet P2P Connectivity")
	fmt.Println("====================================")
	
	// Try different bootstrap configurations
	configs := []struct {
		name string
		peers []string
	}{
		{
			name: "Apollo with correct peer ID",
			peers: []string{
				"/dns/apollo-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWPs19932secARrxoRR5J8ZtBMt2vqwyHH1Q9p8thYP7cn",
			},
		},
		{
			name: "Original bootstrap server",
			peers: []string{
				"/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg",
			},
		},
		{
			name: "Apollo with TCP only (no QUIC)",
			peers: []string{
				"/dns/apollo-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWPs19932secARrxoRR5J8ZtBMt2vqwyHH1Q9p8thYP7cn",
			},
		},
	}
	
	for _, config := range configs {
		fmt.Printf("\n📡 Testing: %s\n", config.name)
		fmt.Printf("   Peers: %v\n", config.peers)
		
		// Parse multiaddrs
		var addrs []multiaddr.Multiaddr
		for _, p := range config.peers {
			addr, err := multiaddr.NewMultiaddr(p)
			if err != nil {
				fmt.Printf("   ❌ Failed to parse: %v\n", err)
				continue
			}
			addrs = append(addrs, addr)
		}
		
		// Create P2P client
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		node, err := p2p.NewClient(p2p.Options{
			Network:        "MainNet",
			BootstrapPeers: addrs,
		})
		
		if err != nil {
			fmt.Printf("   ❌ Failed to create node: %v\n", err)
			cancel()
			continue
		}
		
		// Try to get network status
		status, err := node.NetworkStatus(ctx, v3.NetworkStatusOptions{})
		cancel()
		node.Close()
		
		if err != nil {
			fmt.Printf("   ❌ Failed to get status: %v\n", err)
		} else {
			fmt.Printf("   ✅ SUCCESS! Connected via P2P\n")
			fmt.Printf("      Directory Height: %d\n", status.DirectoryHeight)
			fmt.Printf("      Major Block: %d\n", status.MajorBlockHeight)
		}
	}
	
	fmt.Println("\n🔍 Testing direct dial to Apollo...")
	
	// Try a more direct connection test
	node, err := p2p.New(p2p.Options{
		Network: "MainNet",
		BootstrapPeers: []multiaddr.Multiaddr{},
	})
	if err != nil {
		log.Fatal("Failed to create bare node:", err)
	}
	defer node.Close()
	
	// Try to dial Apollo directly
	apolloAddr, _ := multiaddr.NewMultiaddr("/dns/apollo-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWPs19932secARrxoRR5J8ZtBMt2vqwyHH1Q9p8thYP7cn")
	fmt.Printf("Attempting direct dial to: %s\n", apolloAddr)
	
	// This should expose any connection errors
	time.Sleep(2 * time.Second)
	fmt.Println("✅ Test complete")
}