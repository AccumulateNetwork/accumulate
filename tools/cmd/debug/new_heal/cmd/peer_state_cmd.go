// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/tools/cmd/debug/new_heal"
)

var (
	networkFlag  string
	endpointFlag string
	timeoutFlag  int
	verboseFlag  bool
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&networkFlag, "network", "n", "mainnet", "Network to connect to (mainnet, testnet, or devnet)")
	rootCmd.PersistentFlags().StringVarP(&endpointFlag, "endpoint", "e", "", "Custom API endpoint (overrides network)")
	rootCmd.PersistentFlags().IntVarP(&timeoutFlag, "timeout", "t", 60, "Timeout in seconds for network operations")
	rootCmd.PersistentFlags().BoolVarP(&verboseFlag, "verbose", "v", false, "Enable detailed output for all peers")
}

var rootCmd = &cobra.Command{
	Use:   "peer-state-test",
	Short: "Test the PeerState functionality",
	RunE:  runPeerStateTest,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runPeerStateTest(cmd *cobra.Command, args []string) error {

	// Set up the API client
	var endpoint string
	if endpointFlag != "" {
		endpoint = endpointFlag
	} else {
		endpoint = accumulate.ResolveWellKnownEndpoint(networkFlag, "v3")
	}

	fmt.Printf("Connecting to %s (%s)...\n", networkFlag, endpoint)
	client := jsonrpc.NewClient(endpoint)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutFlag)*time.Second)
	defer cancel()

	// Create a new AddressDir
	addressDir := new_heal.NewAddressDir()

	// Set the network name explicitly
	addressDir.SetNetworkName(networkFlag)

	// Discover network peers
	fmt.Println("Discovering network peers...")
	totalPeers, err := addressDir.DiscoverNetworkPeers(ctx, client)
	if err != nil {
		fmt.Printf("Error discovering network peers: %v\n", err)
		return err
	}
	fmt.Printf("Discovered %d peers\n", totalPeers)

	// Create a new PeerState
	peerState := new_heal.NewPeerState(addressDir)

	// Update peer states
	fmt.Println("Updating peer states...")
	stats, err := peerState.UpdatePeerStates(ctx, client)
	if err != nil {
		fmt.Printf("Error updating peer states: %v\n", err)
		return err
	}

	// Print statistics
	fmt.Println("\n=== Peer State Statistics ===")
	fmt.Printf("Total peers: %d\n", stats.TotalPeers)
	fmt.Printf("Peers with updated heights: %d\n", stats.HeightUpdated)
	fmt.Printf("Maximum DN height: %d\n", stats.DNHeightMax)
	fmt.Printf("Maximum BVN height: %d\n", stats.BVNHeightMax)
	fmt.Printf("DN lagging nodes: %d\n", stats.DNLaggingNodes)
	fmt.Printf("BVN lagging nodes: %d\n", stats.BVNLaggingNodes)

	// Print detailed information if verbose mode is enabled
	if verboseFlag {
		// Get all peer states
		allPeers := peerState.GetAllPeerStates()
		fmt.Println("\n=== All Peers ===")
		for _, peer := range allPeers {
			fmt.Printf("Peer ID: %s\n", peer.ID)
			fmt.Printf("  Validator: %t\n", peer.IsValidator)
			if peer.ValidatorID != "" {
				fmt.Printf("  Validator ID: %s\n", peer.ValidatorID)
			}
			fmt.Printf("  Partition: %s\n", peer.PartitionID)
			fmt.Printf("  DN Height: %d\n", peer.DNHeight)
			fmt.Printf("  BVN Height: %d\n", peer.BVNHeight)
			fmt.Printf("  Zombie: %t\n", peer.IsZombie)
			fmt.Printf("  Last Updated: %s\n", peer.LastUpdated.Format(time.RFC3339))
			fmt.Println()
		}
	}

	// Get lagging peers (any node more than 1 block behind is considered lagging)
	laggingPeers := peerState.GetLaggingPeers(1)
	fmt.Printf("\n=== Lagging Peers (%d) ===\n", len(laggingPeers))
	for _, peer := range laggingPeers {
		fmt.Printf("Lagging peer: ID=%s, DNHeight=%d, BVNHeight=%d\n", 
			peer.ID, peer.DNHeight, peer.BVNHeight)
	}

	// Get zombie peers
	zombiePeers := peerState.GetZombiePeers()
	fmt.Printf("\n=== Zombie Peers (%d) ===\n", len(zombiePeers))
	for _, peer := range zombiePeers {
		fmt.Printf("Zombie peer: ID=%s, ValidatorID=%s\n", 
			peer.ID, peer.ValidatorID)
	}

	return nil
}
