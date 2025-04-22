// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package enhanced_discovery

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
)

// NetworkDiscovery is the main interface for network discovery
type NetworkDiscovery interface {
	// Initialize the network discovery for a specific network
	InitializeNetwork(network string) error
	
	// Discover the network and update the address directory
	DiscoverNetworkPeers(ctx context.Context, client api.NetworkService) (DiscoveryStats, error)
	
	// Update the network state with the latest information
	UpdateNetworkState(ctx context.Context, client api.NetworkService) (DiscoveryStats, error)
	
	// Validate the network and return validation statistics
	ValidateNetwork(ctx context.Context) (DiscoveryStats, error)
	
	// Get the network status
	GetNetworkStatus(ctx context.Context) (*api.NetworkStatus, error)
}

// NetworkDiscoveryImpl implements the NetworkDiscovery interface
type NetworkDiscoveryImpl struct {
	// The address directory to update
	AddressDir *AddressDir
	
	// Logger for discovery operations
	Logger *log.Logger
	
	// Network information
	Network NetworkInfo
	
	// Client for API calls
	Client api.NetworkService
}

// NewNetworkDiscovery creates a new network discovery instance
func NewNetworkDiscovery(addressDir *AddressDir, logger *log.Logger) *NetworkDiscoveryImpl {
	return &NetworkDiscoveryImpl{
		AddressDir: addressDir,
		Logger:     logger,
	}
}

// InitializeNetwork initializes the network information for the specified network
func (nd *NetworkDiscoveryImpl) InitializeNetwork(network string) error {
	// Set up network information
	nd.Network = NetworkInfo{
		Name:        network,
		ID:          "acme",
		APIEndpoint: ResolveWellKnownEndpoint(network, "v3"),
		PartitionMap: make(map[string]*PartitionInfo),
	}
	
	// Create a client for the network
	nd.Client = jsonrpc.NewClient(nd.Network.APIEndpoint)
	
	// Log initialization
	nd.Logger.Printf("Initialized network discovery for network: %s", network)
	nd.Logger.Printf("API endpoint: %s", nd.Network.APIEndpoint)
	
	// Initialize partitions based on network
	switch network {
	case "mainnet":
		nd.Network.IsMainnet = true
		nd.initializeMainnetPartitions()
	case "testnet":
		nd.initializeTestnetPartitions()
	case "devnet":
		nd.initializeDevnetPartitions()
	default:
		nd.Logger.Printf("Unknown network: %s, using default partitions", network)
		nd.initializeDefaultPartitions()
	}
	
	return nil
}

// initializeMainnetPartitions initializes partitions for mainnet
func (nd *NetworkDiscoveryImpl) initializeMainnetPartitions() {
	// Add Directory Network partition
	dnPartition := &PartitionInfo{
		ID:       "dn",
		Type:     "dn",
		URL:      "acc://dn.acme",
		Active:   true,
		BVNIndex: -1,
	}
	nd.Network.Partitions = append(nd.Network.Partitions, dnPartition)
	nd.Network.PartitionMap["dn"] = dnPartition
	
	// Add BVN partitions
	bvnNames := []string{"Apollo", "Chandrayaan", "Yutu"}
	for i, name := range bvnNames {
		bvnPartition := &PartitionInfo{
			ID:       fmt.Sprintf("bvn-%s", name),
			Type:     "bvn",
			URL:      fmt.Sprintf("acc://bvn-%s.acme", name),
			Active:   true,
			BVNIndex: i,
		}
		nd.Network.Partitions = append(nd.Network.Partitions, bvnPartition)
		nd.Network.PartitionMap[bvnPartition.ID] = bvnPartition
	}
}

// initializeTestnetPartitions initializes partitions for testnet
func (nd *NetworkDiscoveryImpl) initializeTestnetPartitions() {
	// Add Directory Network partition
	dnPartition := &PartitionInfo{
		ID:       "dn",
		Type:     "dn",
		URL:      "acc://dn.acme",
		Active:   true,
		BVNIndex: -1,
	}
	nd.Network.Partitions = append(nd.Network.Partitions, dnPartition)
	nd.Network.PartitionMap["dn"] = dnPartition
	
	// Add BVN partition
	bvnPartition := &PartitionInfo{
		ID:       "bvn-Apollo",
		Type:     "bvn",
		URL:      "acc://bvn-Apollo.acme",
		Active:   true,
		BVNIndex: 0,
	}
	nd.Network.Partitions = append(nd.Network.Partitions, bvnPartition)
	nd.Network.PartitionMap[bvnPartition.ID] = bvnPartition
}

// initializeDevnetPartitions initializes partitions for devnet
func (nd *NetworkDiscoveryImpl) initializeDevnetPartitions() {
	// Add Directory Network partition
	dnPartition := &PartitionInfo{
		ID:       "dn",
		Type:     "dn",
		URL:      "acc://dn.acme",
		Active:   true,
		BVNIndex: -1,
	}
	nd.Network.Partitions = append(nd.Network.Partitions, dnPartition)
	nd.Network.PartitionMap["dn"] = dnPartition
	
	// Add BVN partition
	bvnPartition := &PartitionInfo{
		ID:       "bvn-Apollo",
		Type:     "bvn",
		URL:      "acc://bvn-Apollo.acme",
		Active:   true,
		BVNIndex: 0,
	}
	nd.Network.Partitions = append(nd.Network.Partitions, bvnPartition)
	nd.Network.PartitionMap[bvnPartition.ID] = bvnPartition
}

// initializeDefaultPartitions initializes default partitions
func (nd *NetworkDiscoveryImpl) initializeDefaultPartitions() {
	// Add Directory Network partition
	dnPartition := &PartitionInfo{
		ID:       "dn",
		Type:     "dn",
		URL:      "acc://dn.acme",
		Active:   true,
		BVNIndex: -1,
	}
	nd.Network.Partitions = append(nd.Network.Partitions, dnPartition)
	nd.Network.PartitionMap["dn"] = dnPartition
	
	// Add BVN partition
	bvnPartition := &PartitionInfo{
		ID:       "bvn-Apollo",
		Type:     "bvn",
		URL:      "acc://bvn-Apollo.acme",
		Active:   true,
		BVNIndex: 0,
	}
	nd.Network.Partitions = append(nd.Network.Partitions, bvnPartition)
	nd.Network.PartitionMap[bvnPartition.ID] = bvnPartition
}

// DiscoverNetworkPeers discovers the network and updates the address directory
func (nd *NetworkDiscoveryImpl) DiscoverNetworkPeers(ctx context.Context, client api.NetworkService) (DiscoveryStats, error) {
	// Initialize stats
	stats := DiscoveryStats{
		BVNValidators:    make(map[string]int),
		AddressTypeStats: make(map[string]AddressTypeStats),
		VersionCounts:    make(map[string]int),
	}
	
	// Update discovery stats
	nd.AddressDir.DiscoveryStats.DiscoveryAttempts++
	nd.AddressDir.DiscoveryStats.LastDiscovery = time.Now()
	
	// Use the provided client if available, otherwise use the default client
	if client != nil {
		nd.Client = client
	}
	
	// Log discovery start
	nd.Logger.Printf("Starting network discovery for %s", nd.Network.Name)
	
	// Get network status
	start := time.Now()
	networkStatus, err := nd.GetNetworkStatus(ctx)
	if err != nil {
		nd.AddressDir.DiscoveryStats.FailedDiscoveries++
		return stats, fmt.Errorf("failed to get network status: %w", err)
	}
	
	// Track response time
	responseTime := time.Since(start)
	stats.AverageResponseTime = responseTime
	nd.Logger.Printf("Network status retrieved in %v", responseTime)
	
	// Store existing peer count for comparison
	previousPeerCount := len(nd.AddressDir.NetworkPeers)
	
	// Process validators from network status
	for _, validator := range networkStatus.Network.Validators {
		if validator.Operator == nil {
			continue
		}
		
		// Get validator ID and name
		validatorID := validator.Operator.String()
		validatorName := validator.Operator.Authority
		
		// Process validator partitions
		for _, partition := range validator.Partitions {
			if !partition.Active {
				continue
			}
			
			// Determine BVN index for this partition
			bvnIndex := -1
			if partition.ID != "dn" && strings.HasPrefix(partition.ID, "bvn-") {
				bvnName := strings.TrimPrefix(partition.ID, "bvn-")
				switch bvnName {
				case "Apollo":
					bvnIndex = 0
				case "Chandrayaan":
					bvnIndex = 1
				case "Voyager":
					bvnIndex = 2
				default:
					// Try to parse BVN index from name
					for i, p := range nd.Network.Partitions {
						if p.ID == partition.ID {
							bvnIndex = i - 1 // Subtract 1 because DN is at index 0
							break
						}
					}
				}
			}
			
			// Create or update validator
			var val *Validator
			existingVal, _, found := nd.AddressDir.FindValidator(validatorID)
			if found {
				val = existingVal
				nd.Logger.Printf("Updating existing validator: %s (%s)", validatorName, validatorID)
			} else {
				nd.Logger.Printf("Creating new validator: %s (%s)", validatorName, validatorID)
				val = NewValidator(validatorID, validatorName, partition.ID)
			}
			
			// Set partition information
			if partition.ID == "dn" {
				val.PartitionType = "directory"
				val.IsInDN = true
				val.BVN = -1
				
				// Add standardized URL for DN
				val.URLs["partition"] = nd.AddressDir.GetStandardizedURL("dn", "partition")
				val.URLs["anchor"] = nd.AddressDir.GetStandardizedURL("dn", "anchor")
				
				// Add to DN validators
				nd.AddressDir.AddDNValidator(val)
				stats.DNValidators++
			} else if strings.HasPrefix(partition.ID, "bvn-") {
				val.PartitionType = "bvn"
				val.BVN = bvnIndex
				
				// Get BVN name
				bvnName := strings.TrimPrefix(partition.ID, "bvn-")
				
				// Add standardized URLs for BVN
				val.URLs["partition"] = nd.AddressDir.GetStandardizedURL(partition.ID, "partition")
				val.URLs["anchor"] = nd.AddressDir.GetStandardizedURL(partition.ID, "anchor")
				
				// Track BVN membership
				if !contains(val.BVNs, bvnName) {
					val.BVNs = append(val.BVNs, bvnName)
				}
				
				// Add to BVN validators
				if bvnIndex >= 0 {
					nd.AddressDir.AddBVNValidator(bvnIndex, val)
					stats.BVNValidators[bvnName]++
				}
			}
			
			// Add to network peers
			peer := NetworkPeer{
				ID:          validatorID,
				IsValidator: true,
				ValidatorID: validatorID,
				Addresses:   make([]string, 0),
				Active:      true,
				LastSeen:    time.Now(),
			}
			nd.AddressDir.AddNetworkPeer(peer)
			
			// Collect addresses for this validator
			addressStats, err := nd.collectValidatorAddresses(ctx, val, validatorID)
			if err != nil {
				nd.Logger.Printf("Error collecting addresses for validator %s: %v", validatorName, err)
			} else {
				// Update stats
				for addrType, count := range addressStats.TotalByType {
					if _, ok := stats.AddressTypeStats[addrType]; !ok {
						stats.AddressTypeStats[addrType] = AddressTypeStats{}
					}
					typeStats := stats.AddressTypeStats[addrType]
					typeStats.Total += count
					typeStats.Valid += addressStats.ValidByType[addrType]
					
					// Add response time
					if responseTime, ok := addressStats.ResponseTimeByType[addrType]; ok {
						if typeStats.AverageResponseTime == 0 {
							typeStats.AverageResponseTime = responseTime
						} else {
							typeStats.AverageResponseTime = (typeStats.AverageResponseTime + responseTime) / 2
						}
					}
					
					stats.AddressTypeStats[addrType] = typeStats
				}
			}
			
			// Collect version information
			err = nd.collectVersionInfo(ctx, val)
			if err != nil {
				nd.Logger.Printf("Error collecting version info for validator %s: %v", validatorName, err)
			} else if val.Version != "" {
				stats.VersionCounts[val.Version]++
			}
			
			// Check consensus status
			consensusStatus, err := nd.checkConsensusStatus(ctx, val)
			if err != nil {
				nd.Logger.Printf("Error checking consensus status for validator %s: %v", validatorName, err)
			} else {
				// Update heights
				val.DNHeight = consensusStatus.DNHeight
				val.BVNHeight = consensusStatus.BVNHeight
				
				// Update consensus stats
				if consensusStatus.DNHeight > stats.ConsensusHeight {
					stats.ConsensusHeight = consensusStatus.DNHeight
				}
				if consensusStatus.IsZombie {
					stats.ZombieNodes++
					val.IsZombie = true
				}
			}
			
			// Test API v3 connectivity
			apiStatus, err := nd.testAPIv3Connectivity(ctx, val)
			if err != nil {
				nd.Logger.Printf("Error testing API v3 connectivity for validator %s: %v", validatorName, err)
			} else {
				val.APIV3Status = apiStatus.Available
				if apiStatus.Available {
					stats.APIV3Available++
				} else {
					stats.APIV3Unavailable++
				}
			}
			
			// Update validator
			nd.AddressDir.UpdateValidator(val)
		}
	}
	
	// Update total counts
	stats.TotalValidators = stats.DNValidators
	for _, count := range stats.BVNValidators {
		stats.TotalValidators += count
	}
	
	// Calculate success rates and average response times
	for addrType, typeStats := range stats.AddressTypeStats {
		if typeStats.Total > 0 {
			typeStats.SuccessRate = float64(typeStats.Valid) / float64(typeStats.Total)
			stats.AddressTypeStats[addrType] = typeStats
		}
	}
	
	// Update peer counts
	currentPeerCount := len(nd.AddressDir.NetworkPeers)
	stats.TotalPeers = currentPeerCount
	stats.NewPeers = currentPeerCount - previousPeerCount
	if stats.NewPeers < 0 {
		stats.NewPeers = 0
	}
	
	// Update discovery stats
	nd.AddressDir.DiscoveryStats.SuccessfulDiscoveries++
	nd.AddressDir.DiscoveryStats.TotalPeers = currentPeerCount
	nd.AddressDir.DiscoveryStats.TotalValidators = stats.TotalValidators
	nd.AddressDir.DiscoveryStats.DNValidators = stats.DNValidators
	for bvn, count := range stats.BVNValidators {
		nd.AddressDir.DiscoveryStats.BVNValidators[bvn] = count
	}
	
	// Now discover non-validator peers through Tendermint's P2P network
	err = nd.DiscoverNonValidatorPeers(ctx)
	if err != nil {
		nd.Logger.Printf("Error during non-validator peer discovery: %v", err)
	}
	
	// Log discovery completion
	nd.Logger.Printf("Network discovery completed. Found %d validators (%d DN, %v BVN) and %d total peers",
		stats.TotalValidators, stats.DNValidators, stats.BVNValidators, len(nd.AddressDir.NetworkPeers))
	
	// Update discovery stats
	stats.TotalPeers = len(nd.AddressDir.NetworkPeers)
	stats.NewPeers = stats.TotalPeers - previousPeerCount
	
	return stats, nil
}

// Helper function to check if a string is in a slice
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// UpdateNetworkState updates the network state with the latest information
// This is a lighter-weight operation than DiscoverNetworkPeers, focusing on
// updating existing validators rather than discovering new ones
func (nd *NetworkDiscoveryImpl) UpdateNetworkState(ctx context.Context, client api.NetworkService) (DiscoveryStats, error) {
	// Initialize stats
	stats := DiscoveryStats{
		BVNValidators:    make(map[string]int),
		AddressTypeStats: make(map[string]AddressTypeStats),
		VersionCounts:    make(map[string]int),
	}
	
	// Update discovery stats
	nd.AddressDir.DiscoveryStats.DiscoveryAttempts++
	nd.AddressDir.DiscoveryStats.LastDiscovery = time.Now()
	
	// Use the provided client if available, otherwise use the default client
	if client != nil {
		nd.Client = client
	}
	
	// Log update start
	nd.Logger.Printf("Starting network state update for %s", nd.Network.Name)
	
	// Get all validators
	allValidators := nd.AddressDir.GetAllValidators()
	nd.Logger.Printf("Updating state for %d existing validators", len(allValidators))
	
	// Update each validator
	for _, validator := range allValidators {
		// Update partition counts
		if validator.IsInDN {
			stats.DNValidators++
		}
		
		for _, bvn := range validator.BVNs {
			stats.BVNValidators[bvn]++
		}
		
		// Update consensus status
		consensusStatus, err := nd.checkConsensusStatus(ctx, validator)
		if err != nil {
			nd.Logger.Printf("Error checking consensus status for validator %s: %v", validator.Name, err)
		} else {
			// Update heights
			validator.DNHeight = consensusStatus.DNHeight
			validator.BVNHeight = consensusStatus.BVNHeight
			
			// Update consensus stats
			if consensusStatus.DNHeight > stats.ConsensusHeight {
				stats.ConsensusHeight = consensusStatus.DNHeight
			}
			if consensusStatus.IsZombie {
				stats.ZombieNodes++
				validator.IsZombie = true
			} else {
				validator.IsZombie = false
			}
		}
		
		// Test API v3 connectivity
		apiStatus, err := nd.testAPIv3Connectivity(ctx, validator)
		if err != nil {
			nd.Logger.Printf("Error testing API v3 connectivity for validator %s: %v", validator.Name, err)
		} else {
			validator.APIV3Status = apiStatus.Available
			if apiStatus.Available {
				stats.APIV3Available++
			} else {
				stats.APIV3Unavailable++
			}
		}
		
		// Update version information
		err = nd.collectVersionInfo(ctx, validator)
		if err != nil {
			nd.Logger.Printf("Error collecting version info for validator %s: %v", validator.Name, err)
		} else if validator.Version != "" {
			stats.VersionCounts[validator.Version]++
		}
		
		// Update validator in address directory
		validator.LastUpdated = time.Now()
		nd.AddressDir.UpdateValidator(validator)
	}
	
	// Update total counts
	stats.TotalValidators = stats.DNValidators
	for _, count := range stats.BVNValidators {
		stats.TotalValidators += count
	}
	stats.TotalPeers = len(nd.AddressDir.NetworkPeers)
	
	// Update discovery stats
	nd.AddressDir.DiscoveryStats.SuccessfulDiscoveries++
	nd.AddressDir.DiscoveryStats.TotalValidators = stats.TotalValidators
	nd.AddressDir.DiscoveryStats.DNValidators = stats.DNValidators
	for bvn, count := range stats.BVNValidators {
		nd.AddressDir.DiscoveryStats.BVNValidators[bvn] = count
	}
	
	// Log update completion
	nd.Logger.Printf("Network state update completed. Updated %d validators (%d DN, %d BVN)",
		stats.TotalValidators, stats.DNValidators, stats.TotalValidators-stats.DNValidators)
	
	return stats, nil
}

// GetNetworkStatus gets the network status from the API
func (nd *NetworkDiscoveryImpl) GetNetworkStatus(ctx context.Context) (*api.NetworkStatus, error) {
	if nd.Client == nil {
		return nil, fmt.Errorf("client not initialized")
	}
	
	return nd.Client.NetworkStatus(ctx, api.NetworkStatusOptions{})
}

// ValidateNetwork validates the network and returns validation statistics
func (nd *NetworkDiscoveryImpl) ValidateNetwork(ctx context.Context) (DiscoveryStats, error) {
	stats := DiscoveryStats{
		BVNValidators:   make(map[string]int),
		AddressTypeStats: make(map[string]AddressTypeStats),
		VersionCounts:    make(map[string]int),
	}
	
	// Validate DN validators
	for _, validator := range nd.AddressDir.GetDNValidators() {
		// Validate addresses
		for addrType, addr := range validator.URLs {
			// Validate address
			result, err := nd.validateAddress(ctx, validator, addrType, addr)
			if err != nil {
				nd.Logger.Printf("Error validating %s address for validator %s: %v", addrType, validator.Name, err)
				continue
			}
			
			// Update stats
			if _, ok := stats.AddressTypeStats[addrType]; !ok {
				stats.AddressTypeStats[addrType] = AddressTypeStats{}
			}
			typeStats := stats.AddressTypeStats[addrType]
			typeStats.Total++
			if result.Success {
				typeStats.Valid++
			} else {
				typeStats.Invalid++
			}
			stats.AddressTypeStats[addrType] = typeStats
		}
		
		// Check API v3 connectivity
		apiStatus, err := nd.testAPIv3Connectivity(ctx, validator)
		if err != nil {
			nd.Logger.Printf("Error testing API v3 connectivity for validator %s: %v", validator.Name, err)
		} else {
			if apiStatus.Available {
				stats.APIV3Available++
			} else {
				stats.APIV3Unavailable++
			}
		}
		
		// Check consensus status
		consensusStatus, err := nd.checkConsensusStatus(ctx, validator)
		if err != nil {
			nd.Logger.Printf("Error checking consensus status for validator %s: %v", validator.Name, err)
		} else {
			if consensusStatus.DNHeight > stats.ConsensusHeight {
				stats.ConsensusHeight = consensusStatus.DNHeight
			}
			if consensusStatus.IsZombie {
				stats.ZombieNodes++
			}
		}
		
		// Count validators
		stats.DNValidators++
	}
	
	// Validate BVN validators
	for _, validator := range nd.AddressDir.GetAllBVNValidators() {
		// Extract BVN name from partition ID
		if strings.HasPrefix(validator.PartitionID, "bvn-") {
			bvnName := strings.TrimPrefix(validator.PartitionID, "bvn-")
			stats.BVNValidators[bvnName]++
		}
	}
	
	// Update total counts
	stats.TotalValidators = stats.DNValidators
	for _, count := range stats.BVNValidators {
		stats.TotalValidators += count
	}
	
	// Calculate success rates and average response times
	for addrType, typeStats := range stats.AddressTypeStats {
		if typeStats.Total > 0 {
			typeStats.SuccessRate = float64(typeStats.Valid) / float64(typeStats.Total)
			stats.AddressTypeStats[addrType] = typeStats
		}
	}
	
	return stats, nil
}

// ResolveWellKnownEndpoint resolves a well-known endpoint for the given network and service
func ResolveWellKnownEndpoint(network, service string) string {
	switch network {
	case "mainnet":
		return fmt.Sprintf("https://mainnet.accumulatenetwork.io/%s", service)
	case "testnet":
		return fmt.Sprintf("https://testnet.accumulatenetwork.io/%s", service)
	case "devnet":
		return fmt.Sprintf("https://devnet.accumulatenetwork.io/%s", service)
	default:
		return fmt.Sprintf("https://%s.accumulatenetwork.io/%s", network, service)
	}
}
