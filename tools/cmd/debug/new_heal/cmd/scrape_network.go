// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
)

var (
	networkFlag  string
	endpointFlag string
	outputFlag   string
	timeoutFlag  int
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&networkFlag, "network", "n", "mainnet", "Network to connect to (mainnet, testnet, or devnet)")
	rootCmd.PersistentFlags().StringVarP(&endpointFlag, "endpoint", "e", "", "Custom API endpoint (overrides network)")
	rootCmd.PersistentFlags().StringVarP(&outputFlag, "output", "o", "network_data.json", "Output file path")
	rootCmd.PersistentFlags().IntVarP(&timeoutFlag, "timeout", "t", 60, "Timeout in seconds for network operations")
}

var rootCmd = &cobra.Command{
	Use:   "scrape-network",
	Short: "Scrape network information for testing",
	RunE:  scrapeNetwork,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// NetworkData represents the collected network information
type NetworkData struct {
	NetworkStatus *api.NetworkStatus `json:"networkStatus"`
	Partitions    map[string]*api.NetworkStatus `json:"partitions"`
	Timestamp     time.Time `json:"timestamp"`
}

func scrapeNetwork(cmd *cobra.Command, args []string) error {
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

	// Get the main network status
	fmt.Println("Fetching main network status...")
	ns, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		return fmt.Errorf("failed to get network status: %v", err)
	}

	// Initialize the network data
	networkData := NetworkData{
		NetworkStatus: ns,
		Partitions:    make(map[string]*api.NetworkStatus),
		Timestamp:     time.Now(),
	}

	// Get network status for each partition
	fmt.Println("Fetching partition information...")
	for _, partition := range ns.Network.Partitions {
		fmt.Printf("  Fetching data for partition %s...\n", partition.ID)
		partitionNs, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: partition.ID})
		if err != nil {
			fmt.Printf("  Error fetching data for partition %s: %v\n", partition.ID, err)
			continue
		}
		networkData.Partitions[partition.ID] = partitionNs
	}

	// Save to JSON file
	fmt.Printf("Saving network data to %s...\n", outputFlag)
	data, err := json.MarshalIndent(networkData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal network data: %v", err)
	}

	err = os.WriteFile(outputFlag, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write network data to file: %v", err)
	}

	fmt.Printf("Successfully saved network data to %s\n", outputFlag)
	return nil
}
