// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package enhanced_discovery

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
)

// TestLiveMainnetDiscovery performs a full network discovery against the live mainnet
// to find the actual number of non-validator nodes.
// This test is skipped by default when running with -short flag
// Run with: go test -v -run TestLiveMainnetDiscovery
// Skip with: go test -v -short -run TestLiveMainnetDiscovery
func TestLiveMainnetDiscovery(t *testing.T) {
	// Skip in short mode (for quick test runs)
	if testing.Short() {
		t.Skip("Skipping live mainnet discovery test in short mode. Run without -short flag to test against mainnet.")
	}

	// Create a logger
	logger := log.New(os.Stdout, "[LIVE-MAINNET-DISCOVERY] ", log.LstdFlags)
	logger.Printf("Starting live mainnet discovery test")

	// Create an address directory
	addressDir := NewAddressDir()
	addressDir.Logger = logger

	// Create a network discovery instance
	discovery := &NetworkDiscoveryImpl{
		AddressDir: addressDir,
		Logger:     logger,
	}

	// Initialize the network for mainnet
	logger.Printf("Initializing network for mainnet")
	err := discovery.InitializeNetwork("mainnet")
	require.NoError(t, err, "Failed to initialize network")

	// Set the NetworkInfo in the AddressDir
	addressDir.NetworkInfo = &discovery.Network

	// Create a client for mainnet
	logger.Printf("Creating client for mainnet")
	client := jsonrpc.NewClient(discovery.Network.APIEndpoint)

	// Get network status
	logger.Printf("Getting network status")
	// Use a longer timeout for mainnet operations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ns, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	require.NoError(t, err, "Failed to get network status")

	// Process validators from network definition
	logger.Printf("Processing validators from network status")
	for _, val := range ns.Network.Validators {
		if val.Operator == nil {
			continue
		}
		
		operatorID := val.Operator.String()
		
		// Check if this validator is active in any partition
		isActive := false
		for _, partition := range val.Partitions {
			if partition.Active {
				isActive = true
				break
			}
		}
		
		if !isActive {
			logger.Printf("Skipping inactive validator: %s", operatorID)
			continue
		}

		// Create validator
		validator := NewValidator(operatorID, operatorID, "dn")
		
			// For mainnet testing, we need to set IP addresses for validators
		// Set the validator as active
		validator.Status = "active"
		
		// Mark as a DN validator
		validator.IsInDN = true
		
		// Set IP addresses for known validators to enable peer discovery
		// These are the actual IP addresses of the mainnet validators
		validatorIPs := map[string]string{
			"kompendium.acme":       "116.202.214.38",
			"LunaNova.acme":         "144.76.97.251",
			"TurtleBoat.acme":       "65.108.41.39",
			"MusicCityNode.acme":    "159.69.186.97",
			"ConsensusNetworks.acme": "65.21.91.147",
			"tfa.acme":              "65.21.231.58",
			"CodeForj.acme":         "5.9.21.44",
			"PrestigeIT.acme":       "148.251.1.124",
			"defidevs.acme":         "65.109.33.17",
			"Sphereon.acme":         "65.108.6.185",
			"ACMEMining.acme":       "65.108.0.10",
			"Inveniam.acme":         "65.109.115.226",
			"HighStakes.acme":       "65.108.238.166",
			"FederateThis.acme":     "65.108.9.25",
			"DetroitLedgerTech.acme": "65.109.88.38",
		}
		
		// Set the IP address if we have it
		if ip, ok := validatorIPs[operatorID]; ok {
			validator.IPAddress = ip
			logger.Printf("Set IP address %s for validator %s", ip, operatorID)
		}

		// Add validator to the DN validator list
		addressDir.AddDNValidator(validator)

		// Add validator as a network peer
		peer := NetworkPeer{
			ID:          operatorID,
			IsValidator: true,
			ValidatorID: operatorID,
			LastSeen:    time.Now(),
			Active:      true,
		}
		
		// Add P2P address if available
		if validator.P2PAddress != "" {
			peer.Addresses = []string{validator.P2PAddress}
		}
		addressDir.AddNetworkPeer(peer)
	}

	// Log validator counts
	logger.Printf("Added %d DN validators", len(addressDir.DNValidators))
	totalBVN := 0
	for _, validators := range addressDir.BVNValidators {
		totalBVN += len(validators)
	}
	logger.Printf("Added %d BVN validators", totalBVN)

	// Now perform network discovery to find non-validator nodes
	logger.Printf("Starting network discovery to find non-validator nodes")
	stats, err := discovery.DiscoverNetworkPeers(ctx, client)
	if err != nil {
		logger.Printf("Warning: Network discovery encountered errors: %v", err)
	}
	
	// Log discovery statistics
	logger.Printf("Discovery Statistics:")
	logger.Printf("  Total Validators: %d", stats.TotalValidators)
	logger.Printf("  DN Validators: %d", stats.DNValidators)
	logger.Printf("  BVN Validators: %v", stats.BVNValidators)
	logger.Printf("  Total Peers: %d", stats.TotalPeers)
	logger.Printf("  Non-Validator Peers: %d", stats.TotalPeers - stats.TotalValidators)
	
	// Log all peers for analysis
	logger.Printf("\nAll Network Peers:")
	validatorPeers := 0
	nonValidatorPeers := 0
	for id, peer := range addressDir.NetworkPeers {
		if peer.IsValidator {
			validatorPeers++
			logger.Printf("  Validator Peer %d: %s", validatorPeers, id)
		} else {
			nonValidatorPeers++
			logger.Printf("  Non-Validator Peer %d: %s", nonValidatorPeers, id)
		}
		
		if len(peer.Addresses) > 0 {
			logger.Printf("    Addresses: %v", peer.Addresses)
		}
		if !peer.LastSeen.IsZero() {
			logger.Printf("    Last Seen: %s", peer.LastSeen.Format(time.RFC3339))
		}
	}
	
	// Check if all peers are being marked as validators
	logger.Printf("\nValidator Analysis:")
	logger.Printf("  Total Peers: %d", len(addressDir.NetworkPeers))
	logger.Printf("  Validator Peers: %d", validatorPeers)
	logger.Printf("  Non-Validator Peers: %d", nonValidatorPeers)
	
	// Check for validator IDs that don't match peer IDs
	logger.Printf("\nValidators not found in peer list:")
	validatorCount := 0
	validatorIDs := make(map[string]bool)
	
	// Collect all validator IDs
	for _, validator := range addressDir.DNValidators {
		validatorIDs[validator.PeerID] = true
		validatorCount++
	}
	for _, validators := range addressDir.BVNValidators {
		for _, validator := range validators {
			validatorIDs[validator.PeerID] = true
			validatorCount++
		}
	}
	
	// Check which validators are not in the peer list
	missingValidators := 0
	for validatorID := range validatorIDs {
		_, found := addressDir.NetworkPeers[validatorID]
		if !found {
			missingValidators++
			logger.Printf("  Missing Validator %d: %s", missingValidators, validatorID)
		}
	}
	
	// Check for peers that are not validators
	logger.Printf("\nNon-Validator Nodes:")
	nonValidatorCount := 0
	for id, peer := range addressDir.NetworkPeers {
		_, isValidator := validatorIDs[id]
		
		// If the peer is not marked as a validator and not in the validator list
		if !peer.IsValidator && !isValidator {
			nonValidatorCount++
			logger.Printf("  Peer %d: %s", nonValidatorCount, id)
			if len(peer.Addresses) > 0 {
				logger.Printf("    Addresses: %v", peer.Addresses)
			}
			if !peer.LastSeen.IsZero() {
				logger.Printf("    Last Seen: %s", peer.LastSeen.Format(time.RFC3339))
			}
		}
		
		// If the peer IsValidator flag doesn't match our validator list
		if peer.IsValidator != isValidator {
			logger.Printf("  Inconsistent Validator Status: %s (IsValidator=%v, In ValidatorList=%v)", 
				id, peer.IsValidator, isValidator)
		}
	}
	
	logger.Printf("\nFound %d non-validator nodes out of %d total peers", nonValidatorCount, len(addressDir.NetworkPeers))
	
	// Log test completion
	logger.Printf("Live mainnet discovery test completed")
}
