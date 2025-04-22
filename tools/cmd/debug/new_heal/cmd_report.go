// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
)

// RunReportCommand runs the report command
func RunReportCommand(args []string) error {
	// Parse command line flags
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	networkFlag := fs.String("network", "mainnet", "Network to generate report for (mainnet, testnet)")
	apiEndpointFlag := fs.String("api", "", "API endpoint (default: use network's default endpoint)")
	outputFileFlag := fs.String("output", "", "Output file (default: stdout)")
	timeoutFlag := fs.Duration("timeout", 30*time.Second, "Timeout for API requests")
	verboseFlag := fs.Bool("verbose", false, "Enable verbose logging")
	
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Set up logger
	logPrefix := fmt.Sprintf("[%s-REPORT] ", *networkFlag)
	logger := log.New(os.Stderr, logPrefix, log.LstdFlags)
	
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), *timeoutFlag)
	defer cancel()
	
	// Create address directory
	addressDir := NewAddressDir()
	addressDir.Logger = logger
	
	// Create network discovery
	discovery := NewNetworkDiscovery(addressDir, logger)
	
	// Initialize network
	if err := discovery.InitializeNetwork(*networkFlag); err != nil {
		return fmt.Errorf("failed to initialize network: %w", err)
	}
	
	// Set custom API endpoint if provided
	if *apiEndpointFlag != "" {
		addressDir.NetworkInfo.APIEndpoint = *apiEndpointFlag
	}
	
	// Log network info
	if *verboseFlag {
		logger.Printf("Initialized network %s with API endpoint %s", 
			addressDir.NetworkInfo.Name, 
			addressDir.NetworkInfo.APIEndpoint)
	}
	
	// Create API client
	client := jsonrpc.NewClient(addressDir.NetworkInfo.APIEndpoint)
	
	// Discover network peers
	if *verboseFlag {
		logger.Printf("Discovering network peers...")
	}
	
	startTime := time.Now()
	stats, err := discovery.DiscoverNetworkPeers(ctx, client)
	if err != nil {
		logger.Printf("Warning: Error during discovery: %v", err)
	}
	
	if *verboseFlag {
		logger.Printf("Discovery completed in %v", time.Since(startTime))
		logger.Printf("Found %d DN validators and %d BVN validators", 
			stats.DNValidators, 
			stats.BVNValidators)
	}
	
	// Determine output destination
	var output *os.File
	if *outputFileFlag == "" {
		output = os.Stdout
	} else {
		var err error
		output, err = os.Create(*outputFileFlag)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer output.Close()
	}
	
	// Generate and write the report
	GenerateAddressDirReport(addressDir, output)
	
	if *outputFileFlag != "" && *verboseFlag {
		logger.Printf("Report written to %s", *outputFileFlag)
	}
	
	return nil
}
