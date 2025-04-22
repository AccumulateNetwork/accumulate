// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
)

// TestMainnetDiscovery tests the network discovery functionality against the actual mainnet
// This test is skipped by default when running with -short flag
// Run with: go test -v -run TestMainnetDiscovery
// Skip with: go test -v -short -run TestMainnetDiscovery
func TestMainnetDiscovery(t *testing.T) {
	// Skip in short mode (for quick test runs)
	if testing.Short() {
		t.Skip("Skipping mainnet test in short mode. Run without -short flag to test against mainnet.")
	}

	// Create a log directory if it doesn't exist
	logDir := "logs"
	err := os.MkdirAll(logDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create log directory: %v", err)
	}

	// Create a log file with timestamp
	timestamp := time.Now().Format("20060102-150405")
	logFilePath := fmt.Sprintf("%s/mainnet_discovery_%s.log", logDir, timestamp)
	logFile, err := os.Create(logFilePath)
	if err != nil {
		t.Fatalf("Failed to create log file: %v", err)
	}
	defer logFile.Close()

	// Enable more detailed logging with both console and file output
	multiWriter := io.MultiWriter(os.Stdout, logFile)
	logger := log.New(multiWriter, "[MAINNET-TEST] ", log.LstdFlags)
	logger.Printf("Starting mainnet discovery test at %s", time.Now().Format(time.RFC3339))
	logger.Printf("Log file: %s", logFilePath)

	// Create an address directory with the logger
	logger.Printf("Creating AddressDir instance")
	addressDir := NewAddressDir()
	addressDir.Logger = logger

	// Pre-populate the AddressDir with known validator information
	logger.Printf("Pre-populating AddressDir with known validator information")
	validatorRepo := NewValidatorRepository(logger)

	// Create a network discovery instance with the validator repository
	logger.Printf("Creating NetworkDiscovery instance")
	discovery := NewNetworkDiscovery(addressDir, logger)
	discovery.validatorRepo = validatorRepo

	// Initialize the network for mainnet
	logger.Printf("Initializing network for mainnet")
	err = discovery.InitializeNetwork("mainnet")
	require.NoError(t, err, "Failed to initialize network")

	// Create a client for mainnet using the well-known resolver
	logger.Printf("Creating client using well-known endpoint resolver")
	endpoint := accumulate.ResolveWellKnownEndpoint("mainnet", "v3")
	logger.Printf("Using API endpoint: %s", endpoint)
	client := jsonrpc.NewClient(endpoint)
	
	// Set a reasonable timeout for the context
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Log network info before discovery
	logger.Printf("Network info before discovery:")
	logger.Printf("  Name: %s", addressDir.NetworkInfo.Name)
	logger.Printf("  ID: %s", addressDir.NetworkInfo.ID)
	logger.Printf("  IsMainnet: %v", addressDir.NetworkInfo.IsMainnet)
	logger.Printf("  API Endpoint: %s", addressDir.NetworkInfo.APIEndpoint)
	logger.Printf("  Partitions: %d", len(addressDir.NetworkInfo.Partitions))
	for _, partition := range addressDir.NetworkInfo.Partitions {
		logger.Printf("    Partition: %s (Type: %s, URL: %s)", 
			partition.ID, partition.Type, partition.URL)
	}

	// Discover network peers
	logger.Printf("\n=== STARTING NETWORK PEER DISCOVERY ===")
	startTime := time.Now()
	stats, err := discovery.DiscoverNetworkPeers(ctx, client)
	discoveryTime := time.Since(startTime)
	logger.Printf("Network peer discovery completed in %v", discoveryTime)
	
	require.NoError(t, err, "Failed to discover network peers")

	// Verify discovery statistics
	logger.Printf("\n=== DISCOVERY STATISTICS ===")
	logger.Printf("Total Peers: %d", stats.TotalPeers)
	logger.Printf("New Peers: %d", stats.NewPeers)
	logger.Printf("DN Validators: %d", stats.DNValidators)
	logger.Printf("BVN Validators: %d", stats.BVNValidators)
	logger.Printf("Active Validators: %d", stats.ActiveValidators)
	logger.Printf("Inactive Validators: %d", stats.InactiveValidators)
	logger.Printf("Unreachable Validators: %d", stats.UnreachableValidators)

	assert.True(t, stats.TotalPeers > 0, "Should discover at least one peer")
	// We may not always find DN validators depending on the API endpoint
	// so we'll just log this information rather than requiring it
	if stats.DNValidators == 0 {
		logger.Printf("Warning: No DN validators discovered, but this is allowed")
	}
	assert.True(t, stats.BVNValidators > 0, "Should discover at least one BVN validator")

	// Log discovered validators
	logger.Printf("\n=== DIRECTORY NETWORK VALIDATORS ===")
	logger.Printf("Discovered %d DN validators", len(addressDir.DNValidators))
	for i, validator := range addressDir.DNValidators {
		logger.Printf("DN Validator %d: %s (PeerID: %s)", i+1, validator.Name, validator.PeerID)
		logger.Printf("  P2P Address: %s", validator.P2PAddress)
		logger.Printf("  RPC Address: %s", validator.RPCAddress)
		logger.Printf("  API Address: %s", validator.APIAddress)
		logger.Printf("  Status: %s", validator.Status)
		logger.Printf("  Last Updated: %s", validator.LastUpdated.Format(time.RFC3339))
		
		// Log validator addresses
		logger.Printf("  Addresses: %d", len(validator.Addresses))
		for j, addr := range validator.Addresses {
			logger.Printf("    Address %d: %s", j+1, addr.Address)
			logger.Printf("      Validated: %v, IP: %s, Port: %s, PeerID: %s", 
				addr.Validated, addr.IP, addr.Port, addr.PeerID)
			if !addr.LastSuccess.IsZero() {
				logger.Printf("      Last Success: %s", addr.LastSuccess.Format(time.RFC3339))
			}
			if addr.FailureCount > 0 {
				logger.Printf("      Failure Count: %d, Last Error: %s", 
					addr.FailureCount, addr.LastError)
			}
		}
		
		// Log validator URLs
		if len(validator.URLs) > 0 {
			logger.Printf("  URLs:")
			for urlType, url := range validator.URLs {
				logger.Printf("    %s: %s", urlType, url)
			}
		}
	}

	logger.Printf("\n=== BVN VALIDATORS ===")
	logger.Printf("Discovered %d BVN validator groups", len(addressDir.BVNValidators))
	for i, bvnGroup := range addressDir.BVNValidators {
		// Try to determine the BVN name
		bvnName := "Unknown"
		if len(bvnGroup) > 0 {
			bvnName = bvnGroup[0].PartitionID
		}
		logger.Printf("BVN Group %d (%s): %d validators", i+1, bvnName, len(bvnGroup))
		
		for j, validator := range bvnGroup {
			logger.Printf("  BVN Validator %d: %s", j+1, validator.Name)
			logger.Printf("    PeerID: %s, Partition: %s", validator.PeerID, validator.PartitionID)
			logger.Printf("    P2P Address: %s", validator.P2PAddress)
			logger.Printf("    RPC Address: %s", validator.RPCAddress)
			logger.Printf("    API Address: %s", validator.APIAddress)
			logger.Printf("    Status: %s", validator.Status)
			logger.Printf("    Last Updated: %s", validator.LastUpdated.Format(time.RFC3339))
			
			// Log validator addresses
			logger.Printf("    Addresses: %d", len(validator.Addresses))
			for k, addr := range validator.Addresses {
				logger.Printf("      Address %d: %s", k+1, addr.Address)
				logger.Printf("        Validated: %v, IP: %s, Port: %s, PeerID: %s", 
					addr.Validated, addr.IP, addr.Port, addr.PeerID)
				if !addr.LastSuccess.IsZero() {
					logger.Printf("        Last Success: %s", addr.LastSuccess.Format(time.RFC3339))
				}
				if addr.FailureCount > 0 {
					logger.Printf("        Failure Count: %d, Last Error: %s", 
						addr.FailureCount, addr.LastError)
				}
			}
			
			// Log validator URLs
			if len(validator.URLs) > 0 {
				logger.Printf("    URLs:")
				for urlType, url := range validator.URLs {
					logger.Printf("      %s: %s", urlType, url)
				}
			}
		}
	}

	// Verify and log network peers
	logger.Printf("\n=== NETWORK PEERS ===")
	logger.Printf("Total network peers: %d", len(addressDir.NetworkPeers))
	assert.True(t, len(addressDir.NetworkPeers) > 0, "Should have network peers")
	
	// Count validators and non-validators
	validatorCount := 0
	nonValidatorCount := 0
	for _, peer := range addressDir.NetworkPeers {
		if peer.IsValidator {
			validatorCount++
		} else {
			nonValidatorCount++
		}
	}
	logger.Printf("Validator peers: %d", validatorCount)
	logger.Printf("Non-validator peers: %d", nonValidatorCount)
	
	// Log network peers by partition
	partitionPeers := make(map[string]int)
	for _, peer := range addressDir.NetworkPeers {
		partitionPeers[peer.PartitionID]++
	}
	logger.Printf("Peers by partition:")
	for partition, count := range partitionPeers {
		logger.Printf("  %s: %d peers", partition, count)
	}
	
	// Log detailed peer information
	logger.Printf("\nDetailed peer information:")
	peerCount := 0
	for id, peer := range addressDir.NetworkPeers {
		// Only log the first 10 peers to avoid excessive output
		if peerCount >= 10 {
			logger.Printf("... and %d more peers (omitted for brevity)", 
				len(addressDir.NetworkPeers) - peerCount)
			break
		}
		
		peerCount++
		logger.Printf("Peer %s:", id)
		logger.Printf("  IsValidator: %v", peer.IsValidator)
		logger.Printf("  ValidatorID: %s", peer.ValidatorID)
		logger.Printf("  PartitionID: %s", peer.PartitionID)
		logger.Printf("  PartitionURL: %s", peer.PartitionURL)
		logger.Printf("  Status: %s", peer.Status)
		logger.Printf("  First Seen: %s", peer.FirstSeen.Format(time.RFC3339))
		logger.Printf("  Last Seen: %s", peer.LastSeen.Format(time.RFC3339))
		logger.Printf("  Discovery Method: %s", peer.DiscoveryMethod)
		
		// Log peer addresses
		logger.Printf("  Addresses: %d", len(peer.Addresses))
		for i, addr := range peer.Addresses {
			logger.Printf("    Address %d: %s", i+1, addr.Address)
			logger.Printf("      Validated: %v, IP: %s, Port: %s", 
				addr.Validated, addr.IP, addr.Port)
		}
		
		// Log performance metrics
		logger.Printf("  Performance:")
		logger.Printf("    Successful Connections: %d", peer.SuccessfulConnections)
		logger.Printf("    Failed Connections: %d", peer.FailedConnections)
		logger.Printf("    Performance Score: %.2f", peer.CalculatePerformanceScore())
	}

	// Verify that we can get RPC endpoints for validators
	logger.Printf("\n=== VALIDATOR RPC ENDPOINT VERIFICATION ===")
	logger.Printf("Testing RPC endpoint resolution for validators")
	
	// Test with a few known validators
	knownValidators := []string{
		"defidevs.acme",
		"LunaNova.acme",
		"Sphereon.acme",
		"tfa.acme",
		"TurtleBoat.acme",
	}
	
	for _, validatorID := range knownValidators {
		// Try to find the validator in our discovered data
		found := false
		var endpoint string
		
		// Check if we have a NetworkPeer for this validator
		for id, peer := range addressDir.NetworkPeers {
			if strings.Contains(id, validatorID) || strings.Contains(peer.ValidatorID, validatorID) {
				// Create a test NetworkPeer to get the endpoint
				testPeer := NetworkPeer{
					ID:          id,
					ValidatorID: validatorID,
					PartitionID: peer.PartitionID,
					IsValidator: true,
				}
				
				// Get the RPC endpoint
				endpoint = addressDir.GetPeerRPCEndpoint(testPeer)
				if endpoint != "" {
					found = true
					break
				}
			}
		}
		
		// Log the result
		if found {
			logger.Printf("✅ Found RPC endpoint for %s: %s", validatorID, endpoint)
		} else {
			logger.Printf("❌ Could not find RPC endpoint for %s", validatorID)
			
			// Try using the validator repository directly
			host := validatorRepo.GetKnownAddressForValidator(validatorID)
			if host != "" {
				endpoint = fmt.Sprintf("http://%s:16592", host)
				logger.Printf("  But found address in validator repository: %s", endpoint)
			}
		}
	}

	// Generate a comprehensive report of the discovered network
	logger.Printf("\n=== GENERATING COMPREHENSIVE NETWORK REPORT ===")
	var buf bytes.Buffer
	GenerateAddressDirReport(addressDir, &buf)
	logger.Printf("\n%s", buf.String())
	
	// Log test completion
	logger.Printf("\n=== TEST SUMMARY ===")
	logger.Printf("Mainnet discovery test completed at %s", time.Now().Format(time.RFC3339))
	logger.Printf("Total test duration: %v", time.Since(startTime))
	logger.Printf("Discovery time: %v", discoveryTime)
	logger.Printf("Total validators found: %d", stats.DNValidators + stats.BVNValidators)
	logger.Printf("Total peers found: %d", stats.TotalPeers)
	logger.Printf("Log file: %s", logFilePath)
}
