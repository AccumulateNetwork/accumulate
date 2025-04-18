// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// NodeError represents an error associated with a specific node
type NodeError struct {
	// ID of the node that experienced the error
	NodeID string
	
	// Type of error (e.g., "connection", "timeout", "validation")
	ErrorType string
	
	// Detailed error message
	ErrorMessage string
	
	// When the error occurred
	Timestamp time.Time
	
	// Error severity level
	Severity int
	
	// Number of retry attempts made
	RetryCount int
}

// Error severity constants
const (
	ErrorSeverityLow    = 1 // Transient errors that can be retried
	ErrorSeverityMedium = 2 // Errors that may require attention
	ErrorSeverityHigh   = 3 // Critical errors that prevent operation
)

// NetworkDiscovery provides functionality for discovering network peers and validators
type NetworkDiscovery struct {
	// Logger for detailed logging
	Logger *log.Logger
	
	// Reference to the AddressDir that will store the discovered information
	addressDir *AddressDir
	
	// Error tracking
	nodeErrors map[string][]*NodeError
}

// NewNetworkDiscovery creates a new NetworkDiscovery instance
func NewNetworkDiscovery(addressDir *AddressDir, logger *log.Logger) *NetworkDiscovery {
	return &NetworkDiscovery{
		Logger:     logger,
		addressDir: addressDir,
		nodeErrors: make(map[string][]*NodeError),
	}
}

// InitializeNetwork sets up the Network field with appropriate information
func (nd *NetworkDiscovery) InitializeNetwork(networkName string) error {
	// Create a new NetworkInfo instance
	network := &NetworkInfo{
		Name:         networkName,
		ID:           "acme", // Default network ID
		IsMainnet:    networkName == "mainnet",
		Partitions:   make([]*PartitionInfo, 0),
		PartitionMap: make(map[string]*PartitionInfo),
	}
	
	// Set the API endpoint based on the network name
	network.APIEndpoint = nd.resolveNetworkEndpoint(networkName)
	
	// Initialize the network's partitions
	if err := nd.initializeNetworkPartitions(network); err != nil {
		return err
	}
	
	// Store the network information in the AddressDir using the accessor methods
	nd.addressDir.SetNetwork(network)
	nd.addressDir.SetNetworkName(networkName)
	
	nd.log("Initialized network %s with API endpoint %s", networkName, network.APIEndpoint)
	return nil
}

// resolveNetworkEndpoint returns the API endpoint for a network
func (nd *NetworkDiscovery) resolveNetworkEndpoint(networkName string) string {
	// Default to mainnet if no network name is provided
	if networkName == "" {
		networkName = "mainnet"
	}
	
	// Map of network names to API endpoints
	endpoints := map[string]string{
		"mainnet": "https://mainnet.accumulatenetwork.io/v2",
		"testnet": "https://testnet.accumulatenetwork.io/v2",
		"devnet":  "https://devnet.accumulatenetwork.io/v2",
	}
	
	// Return the endpoint for the specified network, or a default if not found
	if endpoint, ok := endpoints[networkName]; ok {
		return endpoint
	}
	
	// For custom networks, construct a URL based on the network name
	return fmt.Sprintf("https://%s.accumulatenetwork.io/v2", networkName)
}

// initializeNetworkPartitions sets up the partitions for a network
func (nd *NetworkDiscovery) initializeNetworkPartitions(network *NetworkInfo) error {
	// For mainnet, we know the partitions
	if network.IsMainnet {
		// Add Directory Network
		dnPartition := &PartitionInfo{
			ID:       "dn",
			Type:     "dn",
			URL:      "acc://dn.acme",
			Active:   true,
			BVNIndex: -1,
		}
		network.Partitions = append(network.Partitions, dnPartition)
		network.PartitionMap["dn"] = dnPartition
		
		// Add BVNs
		bvnNames := []string{"Apollo", "Chandrayaan", "Yutu"}
		for i, name := range bvnNames {
			bvnPartition := &PartitionInfo{
				ID:       name,
				Type:     "bvn",
				URL:      fmt.Sprintf("acc://bvn-%s.acme", name),
				Active:   true,
				BVNIndex: i,
			}
			network.Partitions = append(network.Partitions, bvnPartition)
			network.PartitionMap[name] = bvnPartition
		}
		return nil
	}
	
	// For other networks, discover partitions dynamically
	return nd.discoverNetworkPartitions(network)
}

// discoverNetworkPartitions discovers partitions for non-mainnet networks
func (nd *NetworkDiscovery) discoverNetworkPartitions(network *NetworkInfo) error {
	// This is a placeholder for dynamic partition discovery
	// In a real implementation, this would query the network for its partitions
	nd.log("Dynamic partition discovery not yet implemented for network %s", network.Name)
	
	// For now, assume a standard structure similar to mainnet
	// Add Directory Network
	dnPartition := &PartitionInfo{
		ID:       "dn",
		Type:     "dn",
		URL:      fmt.Sprintf("acc://dn.%s", network.ID),
		Active:   true,
		BVNIndex: -1,
	}
	network.Partitions = append(network.Partitions, dnPartition)
	network.PartitionMap["dn"] = dnPartition
	
	// Add a default BVN
	bvnPartition := &PartitionInfo{
		ID:       "bvn",
		Type:     "bvn",
		URL:      fmt.Sprintf("acc://bvn.%s", network.ID),
		Active:   true,
		BVNIndex: 0,
	}
	network.Partitions = append(network.Partitions, bvnPartition)
	network.PartitionMap["bvn"] = bvnPartition
	
	return nil
}

// DiscoverNetworkPeers is the main entry point for network discovery
func (nd *NetworkDiscovery) DiscoverNetworkPeers(ctx context.Context, client api.NetworkService) (RefreshStats, error) {
	startTime := time.Now()
	stats := RefreshStats{}
	
	// Update discovery stats
	defer func() {
		nd.addressDir.mu.Lock()
		nd.addressDir.DiscoveryStats.LastDiscovery = time.Now()
		nd.addressDir.DiscoveryStats.TotalDiscoveryTime += time.Since(startTime)
		nd.addressDir.DiscoveryStats.DiscoveryAttempts++
		if stats.TotalPeers > 0 {
			nd.addressDir.DiscoveryStats.SuccessfulDiscoveries++
		} else {
			nd.addressDir.DiscoveryStats.FailedDiscoveries++
		}
		nd.addressDir.mu.Unlock()
	}()
	
	// Ensure we have network information
	nd.addressDir.mu.RLock()
	network := nd.addressDir.Network
	nd.addressDir.mu.RUnlock()
	
	if network == nil {
		return stats, fmt.Errorf("network information not initialized")
	}
	
	// 1. Discover directory peers
	dnPeers, err := nd.discoverDirectoryPeers(ctx, client)
	if err != nil {
		nd.logError("Error discovering directory peers", err, ErrorSeverityMedium)
		// Continue with partial results
	}
	stats.DNValidators = len(dnPeers)
	
	// 2. Discover partition peers for each BVN
	bvnPeers := make([][]Validator, 0)
	for _, partition := range network.Partitions {
		if partition.Type == "bvn" {
			peers, err := nd.discoverPartitionPeers(ctx, client, partition)
			if err != nil {
				nd.logError(fmt.Sprintf("Error discovering peers for partition %s", partition.ID), err, ErrorSeverityMedium)
				// Continue with partial results
			}
			
			// Add to BVN validators
			if len(peers) > 0 {
				bvnPeers = append(bvnPeers, peers)
				stats.BVNValidators += len(peers)
			}
		}
	}
	
	// 3. Add common non-validators
	nonValidators, err := nd.discoverCommonNonValidators(ctx)
	if err != nil {
		nd.logError("Error discovering common non-validators", err, ErrorSeverityLow)
		// Continue with partial results
	}
	stats.TotalNonValidators = len(nonValidators)
	
	// 4. Update AddressDir with the discovered peers
	nd.addressDir.mu.Lock()
	// Update DN validators
	nd.addressDir.DNValidators = dnPeers
	// Update BVN validators
	nd.addressDir.BVNValidators = bvnPeers
	// Update network peers map
	for id, peer := range nonValidators {
		nd.addressDir.NetworkPeers[id] = peer
	}
	nd.addressDir.mu.Unlock()
	
	// Calculate total peers
	stats.TotalPeers = stats.DNValidators + stats.BVNValidators + stats.TotalNonValidators
	stats.TotalValidators = stats.DNValidators + stats.BVNValidators
	
	nd.log("Discovered %d total peers (%d validators, %d non-validators)",
		stats.TotalPeers, stats.TotalValidators, stats.TotalNonValidators)
	
	return stats, nil
}

// discoverDirectoryPeers discovers peers from the Directory Network
func (nd *NetworkDiscovery) discoverDirectoryPeers(ctx context.Context, client api.NetworkService) ([]Validator, error) {
	nd.log("Discovering Directory Network peers")
	
	// Query network status for the Directory Network
	opts := api.NetworkStatusOptions{
		Partition: "dn",
	}
	
	status, err := client.NetworkStatus(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get network status for DN: %w", err)
	}
	
	// Process validators from the network status
	validators := make([]Validator, 0)
	
	if status.Network != nil {
		for _, valInfo := range status.Network.Validators {
			// Skip validators without operator information
			if valInfo.Operator == nil {
				continue
			}
			
			// Check if this validator is active in the DN
			isDNValidator := false
			for _, partInfo := range valInfo.Partitions {
				if partInfo.ID == "dn" {
					isDNValidator = true
					break
				}
			}
			
			if !isDNValidator {
				continue
			}
			
			// Create a validator entry
			validator := Validator{
				PeerID:        valInfo.Operator.String(),
				Name:          valInfo.Operator.String(),
				PartitionID:   "dn",
				PartitionType: "dn",
				BVN:           -1,
				Status:        "active",
				LastUpdated:   time.Now(),
			}
			
			// Add multiaddresses if available
			for _, partInfo := range valInfo.Partitions {
				if partInfo.ID == "dn" {
					// Try to extract base URL from validator info
					baseUrl := nd.extractBaseUrl(partInfo)
					if baseUrl != "" {
						// Try to parse as multiaddress
						maddr, err := multiaddr.NewMultiaddr(baseUrl)
						if err == nil {
							validator.P2PAddress = baseUrl
							
							// Extract IP address
							var ip string
							multiaddr.ForEach(maddr, func(c multiaddr.Component) bool {
								if c.Protocol().Code == multiaddr.P_IP4 || c.Protocol().Code == multiaddr.P_IP6 {
									ip = c.Value()
									return false
								}
								return true
							})
							
							if ip != "" {
								validator.IPAddress = ip
								validator.RPCAddress = fmt.Sprintf("http://%s:26657", ip)
								validator.APIAddress = fmt.Sprintf("http://%s:8080", ip)
								validator.MetricsAddress = fmt.Sprintf("http://%s:9090", ip)
							}
						}
					}
				}
			}
			
			validators = append(validators, validator)
		}
	}
	
	nd.log("Discovered %d Directory Network validators", len(validators))
	return validators, nil
}

// discoverPartitionPeers discovers peers from a specific partition
func (nd *NetworkDiscovery) discoverPartitionPeers(ctx context.Context, client api.NetworkService, partition *PartitionInfo) ([]Validator, error) {
	nd.log("Discovering peers for partition %s", partition.ID)
	
	// Query network status for this partition
	opts := api.NetworkStatusOptions{
		Partition: partition.ID,
	}
	
	status, err := client.NetworkStatus(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get network status for partition %s: %w", partition.ID, err)
	}
	
	// Process validators from the network status
	validators := make([]Validator, 0)
	
	if status.Network != nil {
		for _, valInfo := range status.Network.Validators {
			// Skip validators without operator information
			if valInfo.Operator == nil {
				continue
			}
			
			// Check if this validator is active in this partition
			isPartitionValidator := false
			for _, partInfo := range valInfo.Partitions {
				if partInfo.ID == partition.ID {
					isPartitionValidator = true
					break
				}
			}
			
			if !isPartitionValidator {
				continue
			}
			
			// Create a validator entry
			validator := Validator{
				PeerID:        valInfo.Operator.String(),
				Name:          valInfo.Operator.String(),
				PartitionID:   partition.ID,
				PartitionType: partition.Type,
				BVN:           partition.BVNIndex,
				Status:        "active",
				LastUpdated:   time.Now(),
			}
			
			// Add multiaddresses if available
			for _, partInfo := range valInfo.Partitions {
				if partInfo.ID == partition.ID {
					// Try to extract base URL from validator info
					baseUrl := nd.extractBaseUrl(partInfo)
					if baseUrl != "" {
						// Try to parse as multiaddress
						maddr, err := multiaddr.NewMultiaddr(baseUrl)
						if err == nil {
							validator.P2PAddress = baseUrl
							
							// Extract IP address
							var ip string
							multiaddr.ForEach(maddr, func(c multiaddr.Component) bool {
								if c.Protocol().Code == multiaddr.P_IP4 || c.Protocol().Code == multiaddr.P_IP6 {
									ip = c.Value()
									return false
								}
								return true
							})
							
							if ip != "" {
								validator.IPAddress = ip
								validator.RPCAddress = fmt.Sprintf("http://%s:26657", ip)
								validator.APIAddress = fmt.Sprintf("http://%s:8080", ip)
								validator.MetricsAddress = fmt.Sprintf("http://%s:9090", ip)
							}
						}
					}
				}
			}
			
			validators = append(validators, validator)
		}
	}
	
	nd.log("Discovered %d validators for partition %s", len(validators), partition.ID)
	return validators, nil
}

// extractBaseUrl is a helper function to extract the base URL from a validator partition info
// Since ValidatorPartitionInfo doesn't have a direct BaseUrl field, we need to construct it
func (nd *NetworkDiscovery) extractBaseUrl(partInfo interface{}) string {
	// Try to get the partition ID
	var partitionID string
	var active bool
	
	if pi, ok := partInfo.(*protocol.ValidatorPartitionInfo); ok {
		partitionID = pi.ID
		active = pi.Active
	} else if pi, ok := partInfo.(*PartitionInfo); ok {
		partitionID = pi.ID
		active = pi.Active
	} else {
		nd.log("Unknown partition info type: %T", partInfo)
		return ""
	}
	
	// Skip inactive partitions
	if !active {
		nd.log("Partition %s is not active, skipping", partitionID)
		return ""
	}
	
	// Get the network info from the address directory using the accessor method
	network := nd.addressDir.GetNetwork()
	
	if network == nil {
		nd.log("Network information not available")
		return ""
	}
	
	// Construct the base URL based on the partition ID and network ID
	var baseUrl string
	if strings.HasPrefix(partitionID, "bvn-") {
		// For BVNs with bvn- prefix, use the ID as is
		baseUrl = fmt.Sprintf("acc://%s.%s", partitionID, network.ID)
	} else if strings.HasPrefix(partitionID, "dn") {
		// For directory network
		baseUrl = fmt.Sprintf("acc://%s.%s", partitionID, network.ID)
	} else {
		// For BVNs without prefix, add the bvn- prefix
		baseUrl = fmt.Sprintf("acc://bvn-%s.%s", partitionID, network.ID)
	}
	
	nd.log("Constructed base URL for partition %s: %s", partitionID, baseUrl)
	return baseUrl
}

// discoverCommonNonValidators adds known non-validator peers to the peer list
func (nd *NetworkDiscovery) discoverCommonNonValidators(ctx context.Context) (map[string]NetworkPeer, error) {
	nd.log("Discovering common non-validator peers")
	
	// This is a placeholder for discovering non-validator peers
	// In a real implementation, this would query known endpoints or use other discovery mechanisms
	
	// For now, just return an empty map
	nonValidators := make(map[string]NetworkPeer)
	
	// Add some well-known non-validator peers for mainnet
	// Use the accessor method to get the network information
	network := nd.addressDir.GetNetwork()
	isMainnet := network != nil && network.IsMainnet
	
	if isMainnet {
		// Example of adding a known peer
		peer := NetworkPeer{
			ID:                   "public-api-1",
			IsValidator:          false,
			PartitionID:          "dn",
			Addresses:            []ValidatorAddress{{Address: "/ip4/65.108.73.121/tcp/16593/p2p/QmPublicAPI1", Validated: true, IP: "65.108.73.121", Port: "16593", PeerID: "QmPublicAPI1"}},
			Status:               "active",
			LastSeen:             time.Now(),
			FirstSeen:            time.Now(),
			PartitionURL:         "acc://dn.acme",
			DiscoveryMethod:      "hardcoded",
			DiscoveredAt:         time.Now(),
			SuccessfulConnections: 1,
		}
		nonValidators[peer.ID] = peer
	}
	
	nd.log("Added %d common non-validator peers", len(nonValidators))
	return nonValidators, nil
}

// UpdateNetworkState updates the network state based on discovery results
func (nd *NetworkDiscovery) UpdateNetworkState(ctx context.Context, client api.NetworkService) (RefreshStats, error) {
	nd.log("Updating network state")
	
	// Perform network discovery
	stats, err := nd.DiscoverNetworkPeers(ctx, client)
	if err != nil {
		nd.logError("Error during network discovery", err, ErrorSeverityMedium)
		// Continue with partial results
	}
	
	// Prune stale information
	nd.pruneStaleInformation(&stats)
	
	return stats, nil
}

// pruneStaleInformation removes stale information from the address directory
func (nd *NetworkDiscovery) pruneStaleInformation(stats *RefreshStats) {
	nd.log("Pruning stale information")
	
	// Define the maximum age for information before it's considered stale
	const maxAge = 24 * time.Hour
	
	now := time.Now()
	
	// Lock the address directory for writing
	nd.addressDir.mu.Lock()
	defer nd.addressDir.mu.Unlock()
	
	// Prune stale network peers
	for id, peer := range nd.addressDir.NetworkPeers {
		if now.Sub(peer.LastSeen) > maxAge {
			delete(nd.addressDir.NetworkPeers, id)
			stats.LostPeers++
		}
	}
	
	nd.log("Pruned %d stale peers", stats.LostPeers)
}

// logError logs an error and adds it to the node errors map
func (nd *NetworkDiscovery) logError(message string, err error, severity int) {
	if nd.Logger != nil {
		nd.Logger.Printf("%s: %v", message, err)
	}
	
	// For now, just log the error without associating it with a specific node
	// In a real implementation, this would track errors by node ID
}

// log logs a message if the logger is available
func (nd *NetworkDiscovery) log(format string, args ...interface{}) {
	if nd.Logger != nil {
		nd.Logger.Printf(format, args...)
	}
}
