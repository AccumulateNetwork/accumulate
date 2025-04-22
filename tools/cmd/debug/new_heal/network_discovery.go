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
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
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
	
	// Network type (mainnet, testnet, devnet)
	networkType string
	
	// Validator repository for centralized validator information
	validatorRepo *ValidatorRepository
	
	// Error tracking
	nodeErrors map[string][]*NodeError
	
	// Retry configuration
	maxRetries int
	retryDelay time.Duration
}

// NewNetworkDiscovery creates a new NetworkDiscovery instance
func NewNetworkDiscovery(addressDir *AddressDir, logger *log.Logger) *NetworkDiscovery {
	return &NetworkDiscovery{
		addressDir:    addressDir,
		Logger:        logger,
		validatorRepo: NewValidatorRepository(logger),
		nodeErrors:    make(map[string][]*NodeError),
		maxRetries:    3,
		retryDelay:    time.Second * 2,
	}
}

// getKnownAddressForValidator returns a known IP address for a validator ID
func (nd *NetworkDiscovery) getKnownAddressForValidator(validatorID string) string {
	// Use the validator repository to get the known address
	return nd.validatorRepo.GetKnownAddressForValidator(validatorID)
}

// InitializeNetwork sets up the Network field with appropriate information
func (nd *NetworkDiscovery) InitializeNetwork(networkName string) error {
	// Set the network type
	nd.networkType = networkName
	
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
	
	// Initialize enhanced discovery statistics
	discoveryStats := EnhancedDiscoveryStats{
		TotalValidators:   0,
		DNValidators:      0,
		BVNValidators:     make(map[string]int),
		TotalNonValidators: 0,
		TotalPeers:        0,
		AddressTypeStats:  make(map[string]AddressTypeStats),
		VersionCounts:     make(map[string]int),
		ZombieNodes:       0,
		APIV3Available:    0,
		APIV3Unavailable:  0,
		NewPeers:          0,
		LostPeers:         0,
		SuccessRate:       0,
	}
	
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
	nd.log("Discovering Directory Network peers with enhanced address collection")
	dnPeers, err := nd.discoverDirectoryPeers(ctx, client)
	if err != nil {
		nd.logError("Error discovering directory peers", err, ErrorSeverityMedium)
		// Continue with partial results
	}
	stats.DNValidators = len(dnPeers)
	discoveryStats.DNValidators = len(dnPeers)
	
	// Process each DN validator to collect all address types
	for i := range dnPeers {
		validator := &dnPeers[i]
		
		// Skip validators without an IP address
		if validator.IPAddress == "" {
			for _, addr := range validator.Addresses {
				if addr.IP != "" {
					validator.IPAddress = addr.IP
					break
				}
			}
			
			// If still no IP address, try to get one from the validator repository
			if validator.IPAddress == "" {
				validator.IPAddress = nd.getKnownAddressForValidator(validator.Name)
			}
			
			// If still no IP address, skip this validator
			if validator.IPAddress == "" {
				nd.log("No IP address available for validator %s, skipping address collection", validator.Name)
				continue
			}
		}
		
		// Collect all address types
		addressStats, err := nd.collectValidatorAddresses(ctx, validator, validator.IPAddress)
		if err != nil {
			nd.logError(fmt.Sprintf("Error collecting addresses for validator %s", validator.Name), err, ErrorSeverityMedium)
			// Continue with partial results
		}
		
		// Update address type statistics
		for addrType, count := range addressStats.TotalByType {
			if _, ok := discoveryStats.AddressTypeStats[addrType]; !ok {
				discoveryStats.AddressTypeStats[addrType] = AddressTypeStats{}
			}
			stats := discoveryStats.AddressTypeStats[addrType]
			stats.Total += count
			stats.Valid += addressStats.ValidByType[addrType]
			stats.Invalid += (count - addressStats.ValidByType[addrType])
			discoveryStats.AddressTypeStats[addrType] = stats
		}
		
		// Test API v3 connectivity
		apiStatus, err := nd.testAPIv3Connectivity(ctx, validator)
		if err != nil {
			nd.log("Error testing API v3 connectivity for validator %s: %v", validator.Name, err)
		} else {
			validator.APIV3Status = apiStatus.Available
			if apiStatus.Available {
				discoveryStats.APIV3Available++
			} else {
				discoveryStats.APIV3Unavailable++
			}
		}
		
		// Collect version information
		if err := nd.collectVersionInfo(ctx, validator); err != nil {
			nd.log("Error collecting version information for validator %s: %v", validator.Name, err)
		} else if validator.Version != "" {
			discoveryStats.VersionCounts[validator.Version]++
		}
		
		// Check consensus status
		consensusStatus, err := nd.checkConsensusStatus(ctx, validator)
		if err != nil {
			nd.log("Error checking consensus status for validator %s: %v", validator.Name, err)
		} else {
			if consensusStatus.IsZombie {
				discoveryStats.ZombieNodes++
			}
		}
	}
	
	// 2. Discover partition peers for each BVN
	bvnPeers := make([][]Validator, 0)
	for _, partition := range network.Partitions {
		if partition.Type == "bvn" {
			nd.log("Discovering peers for BVN partition %s with enhanced address collection", partition.ID)
			peers, err := nd.discoverPartitionPeers(ctx, client, partition)
			if err != nil {
				nd.logError(fmt.Sprintf("Error discovering peers for partition %s", partition.ID), err, ErrorSeverityMedium)
				// Continue with partial results
			}
			
			// Process each BVN validator to collect all address types
			for i := range peers {
				validator := &peers[i]
				
				// Skip validators without an IP address
				if validator.IPAddress == "" {
					for _, addr := range validator.Addresses {
						if addr.IP != "" {
							validator.IPAddress = addr.IP
							break
						}
					}
					
					// If still no IP address, try to get one from the validator repository
					if validator.IPAddress == "" {
						validator.IPAddress = nd.getKnownAddressForValidator(validator.Name)
					}
					
					// If still no IP address, skip this validator
					if validator.IPAddress == "" {
						nd.log("No IP address available for validator %s, skipping address collection", validator.Name)
						continue
					}
				}
				
				// Collect all address types
				addressStats, err := nd.collectValidatorAddresses(ctx, validator, validator.IPAddress)
				if err != nil {
					nd.logError(fmt.Sprintf("Error collecting addresses for validator %s", validator.Name), err, ErrorSeverityMedium)
					// Continue with partial results
				}
				
				// Update address type statistics
				for addrType, count := range addressStats.TotalByType {
					if _, ok := discoveryStats.AddressTypeStats[addrType]; !ok {
						discoveryStats.AddressTypeStats[addrType] = AddressTypeStats{}
					}
					stats := discoveryStats.AddressTypeStats[addrType]
					stats.Total += count
					stats.Valid += addressStats.ValidByType[addrType]
					stats.Invalid += (count - addressStats.ValidByType[addrType])
					discoveryStats.AddressTypeStats[addrType] = stats
				}
				
				// Test API v3 connectivity
				apiStatus, err := nd.testAPIv3Connectivity(ctx, validator)
				if err != nil {
					nd.log("Error testing API v3 connectivity for validator %s: %v", validator.Name, err)
				} else {
					validator.APIV3Status = apiStatus.Available
					if apiStatus.Available {
						discoveryStats.APIV3Available++
					} else {
						discoveryStats.APIV3Unavailable++
					}
				}
				
				// Collect version information
				if err := nd.collectVersionInfo(ctx, validator); err != nil {
					nd.log("Error collecting version information for validator %s: %v", validator.Name, err)
				} else if validator.Version != "" {
					discoveryStats.VersionCounts[validator.Version]++
				}
				
				// Check consensus status
				consensusStatus, err := nd.checkConsensusStatus(ctx, validator)
				if err != nil {
					nd.log("Error checking consensus status for validator %s: %v", validator.Name, err)
				} else {
					if consensusStatus.IsZombie {
						discoveryStats.ZombieNodes++
					}
				}
			}
			
			// Add to BVN validators
			if len(peers) > 0 {
				bvnPeers = append(bvnPeers, peers)
				stats.BVNValidators += len(peers)
				discoveryStats.BVNValidators[partition.ID] = len(peers)
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
	discoveryStats.TotalValidators = stats.TotalValidators
	discoveryStats.TotalPeers = stats.TotalPeers
	
	// Calculate average response times and success rates for each address type
	for addrType, addrStats := range discoveryStats.AddressTypeStats {
		if addrStats.Total > 0 {
			addrStats.SuccessRate = float64(addrStats.Valid) / float64(addrStats.Total)
			discoveryStats.AddressTypeStats[addrType] = addrStats
		}
	}
	
	// Calculate overall success rate
	totalAddresses := 0
	totalValid := 0
	for _, addrStats := range discoveryStats.AddressTypeStats {
		totalAddresses += addrStats.Total
		totalValid += addrStats.Valid
	}
	if totalAddresses > 0 {
		discoveryStats.SuccessRate = float64(totalValid) / float64(totalAddresses)
	}
	
	// Copy enhanced stats to the legacy stats structure
	stats.ZombieNodes = discoveryStats.ZombieNodes
	stats.APIV3Available = discoveryStats.APIV3Available
	stats.APIV3Unavailable = discoveryStats.APIV3Unavailable
	
	// Log detailed discovery results
	nd.log("Discovered %d total peers (%d validators, %d non-validators)",
		stats.TotalPeers, stats.TotalValidators, stats.TotalNonValidators)
	nd.log("Address collection statistics:")
	for addrType, addrStats := range discoveryStats.AddressTypeStats {
		nd.log("  %s: %d total, %d valid (%.2f%% success rate)",
			addrType, addrStats.Total, addrStats.Valid, addrStats.SuccessRate*100)
	}
	nd.log("API v3 connectivity: %d available, %d unavailable",
		discoveryStats.APIV3Available, discoveryStats.APIV3Unavailable)
	nd.log("Zombie nodes detected: %d", discoveryStats.ZombieNodes)
	nd.log("Version distribution:")
	for version, count := range discoveryStats.VersionCounts {
		nd.log("  %s: %d validators", version, count)
	}
	
	return stats, nil
}
// discoverDirectoryPeers discovers peers from the Directory Network
func (nd *NetworkDiscovery) discoverDirectoryPeers(ctx context.Context, client api.NetworkService) ([]Validator, error) {
	nd.log("Discovering Directory Network peers")
	
	// Create a client for the directory network
	dnClient := client
	
	// Initialize network status
	var netStatus *api.NetworkStatus
	var err error
	
	// Try different approaches to get network status with retry logic
	for attempt := 0; attempt <= nd.maxRetries; attempt++ {
		if attempt > 0 {
			nd.log("Retry attempt %d/%d after waiting %v", attempt, nd.maxRetries, nd.retryDelay)
			time.Sleep(nd.retryDelay)
			// Exponential backoff
			nd.retryDelay = nd.retryDelay * 2
		}
		
		// Try approach 1: Query the directory network directly
		netStatus, err = dnClient.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: "dn"})
		if err == nil {
			break
		}
		nd.log("Error querying DN directly: %v. Trying alternative approaches", err)
		
		// Try approach 2: Query without specifying a partition
		netStatus, err = dnClient.NetworkStatus(ctx, api.NetworkStatusOptions{})
		if err == nil {
			break
		}
		nd.log("Error querying without partition: %v", err)
		
		// Try approach 3: For mainnet, use a direct query to a known validator
		if nd.networkType == "mainnet" {
			// Create a client for a known DN validator
			knownDNValidator := "defidevs.acme"
			knownAddr := nd.validatorRepo.GetKnownAddressForValidator(knownDNValidator)
			if knownAddr != "" {
				endpoints := nd.validatorRepo.GetValidatorEndpoints(knownDNValidator)
				if endpoints.RPCAddress != "" {
					nd.log("Trying direct query to known validator %s at %s", knownDNValidator, endpoints.RPCAddress)
					
					// Create a client for this validator
					validatorClient := jsonrpc.NewClient(endpoints.RPCAddress)
					{
						netStatus, err = validatorClient.NetworkStatus(ctx, api.NetworkStatusOptions{})
						if err == nil {
							break
						}
						nd.log("Error querying validator %s: %v", knownDNValidator, err)
					}
				}
			}
		}
		
		// If this is the last attempt and we still have an error
		if attempt == nd.maxRetries {
			nd.log("All attempts to query network status failed")
			return nil, fmt.Errorf("failed to get network status after %d attempts: %w", nd.maxRetries+1, err)
		}
	}
	
	// Extract validators from the network status
	validators := make([]Validator, 0)
	
	// Process the validators from the network status
	if netStatus != nil && netStatus.Network != nil && netStatus.Network.Validators != nil {
		nd.log("Found %d validators in network status", len(netStatus.Network.Validators))
		
		for _, v := range netStatus.Network.Validators {
			// Skip validators without operator information
			if v.Operator == nil {
				nd.log("Validator has no operator information, skipping")
				continue
			}
			
			// Extract validator name from the operator URL
			name := v.Operator.Authority
			
			// Check if this validator is a DN validator
			isDNValidator := false
			for _, partInfo := range v.Partitions {
				if partInfo.ID == "dn" {
					isDNValidator = true
					break
				}
			}
			
			// For mainnet, we need to be more flexible about validator detection
			if nd.networkType == "mainnet" && !isDNValidator {
				// Check if this validator is in our known DN validators list
				if nd.validatorRepo.GetKnownAddressForValidator(name) != "" {
					isDNValidator = true
				}
			}
			
			if !isDNValidator {
				nd.log("Validator %s is not an active DN validator, skipping", name)
				continue
			}
			
			// Get the validator's partition info
			var partInfo *protocol.ValidatorPartitionInfo
			for _, pi := range v.Partitions {
				if pi.ID == "dn" {
					partInfo = pi
					break
				}
			}
			
			// For mainnet, if we don't have DN partition info but we know it's a DN validator,
			// use any partition info available
			if partInfo == nil && nd.networkType == "mainnet" && len(v.Partitions) > 0 {
				partInfo = v.Partitions[0]
				nd.log("Using %s partition info for DN validator %s", partInfo.ID, name)
			}
			
			if partInfo == nil {
				nd.log("Validator %s has no partition info, skipping", name)
				continue
			}
			
			// Get known address for this validator
			knownAddr := nd.getKnownAddressForValidator(name)
			if knownAddr == "" {
				nd.log("No known address for validator %s", name)
			}
			
			// Get partition info from network status
			var addresses []string
			if partInfo != nil {
				// For validator partition info, we need to use a different method
				addresses = nd.extractAddressesFromValidatorPartition(partInfo)
			}
			if len(addresses) > 0 {
				nd.log("Found %d additional addresses for validator %s from partition info", len(addresses), name)
			}
			
			// Create validator addresses
			validatorAddresses := make([]ValidatorAddress, 0, len(addresses))
			
			// If we have a known address but no extracted addresses, create a validator address directly
			if len(addresses) == 0 && knownAddr != "" {
				// Instead of constructing a multiaddress that needs to be parsed, create the validator address directly
				valAddr := ValidatorAddress{
					Address:   fmt.Sprintf("/ip4/%s/tcp/26656", knownAddr),
					Validated: true,
					IP:        knownAddr,
					Port:      "26656",
					PeerID:    "", // Leave peer ID empty as we don't have a valid one
				}
				validatorAddresses = append(validatorAddresses, valAddr)
				nd.log("Added direct validator address for %s: %s", name, knownAddr)
			}
			for _, addr := range addresses {
				// Parse the multiaddress
				maddr, err := multiaddr.NewMultiaddr(addr)
				if err != nil {
					nd.log("Error parsing multiaddress %s: %v", addr, err)
					continue
				}
				
				// Extract components
				var ip, port, peerID string
				multiaddr.ForEach(maddr, func(c multiaddr.Component) bool {
					switch c.Protocol().Code {
					case multiaddr.P_IP4, multiaddr.P_IP6:
						ip = c.Value()
					case multiaddr.P_TCP:
						port = c.Value()
					case multiaddr.P_P2P:
						peerID = c.Value()
					}
					return true
				})
				
				// Create validator address
				validatorAddresses = append(validatorAddresses, ValidatorAddress{
					Address:   addr,
					Validated: true,
					IP:        ip,
					Port:      port,
					PeerID:    peerID,
				})
			}
			
			// Create validator
			validator := Validator{
				Name:          name,
				PartitionID:   "dn",
				PartitionType: "directory",
				Status:        "active",
				LastUpdated:   time.Now(),
				Addresses:     validatorAddresses,
			}
			
			// Add validator to the list
			validators = append(validators, validator)
			
			// Log the validator information
			nd.log("Added DN validator: %s with %d addresses", validator.Name, len(validator.Addresses))
			for i, addr := range validator.Addresses {
				nd.log("  Address %d: %s (Valid: %v, IP: %s, Port: %s)", i+1, addr.Address, addr.Validated, addr.IP, addr.Port)
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
			
			// Get the validator ID
			validatorID := valInfo.Operator.String()
			
			// Check if this validator is active in this partition
			isPartitionValidator := false
			var activePartInfo *protocol.ValidatorPartitionInfo
			
			for _, partInfo := range valInfo.Partitions {
				if partInfo.ID == partition.ID {
					isPartitionValidator = true
					activePartInfo = partInfo
					break
				}
			}
			
			if !isPartitionValidator {
				continue
			}
			
			// Create a validator entry
			validator := Validator{
				PeerID:        validatorID,
				Name:          validatorID,
				PartitionID:   partition.ID,
				PartitionType: partition.Type,
				BVN:           partition.BVNIndex,
				Status:        "active",
				LastUpdated:   time.Now(),
				URLs:          make(map[string]string),
				Addresses:     make([]ValidatorAddress, 0),
				AddressStatus: make(map[string]string),
			}
			
			// Set URLs for this validator
			network := nd.addressDir.GetNetwork()
			if network != nil {
				validator.URLs["partition"] = nd.addressDir.constructPartitionURL(partition.ID)
				validator.URLs["anchor"] = nd.addressDir.constructAnchorURL(partition.ID)
			}
			
			// First try to get a known address for this validator
			knownIP := nd.getKnownAddressForValidator(validatorID)
			if knownIP != "" {
				// Construct a multiaddress with the known IP
				addr := fmt.Sprintf("/ip4/%s/tcp/26656", knownIP)
				nd.log("Using known address for validator %s: %s", validatorID, addr)
				
				// Try to parse and validate the multiaddress
				_, port, peerID, valid := nd.addressDir.ValidateMultiaddress(addr)
				
				validatorAddr := ValidatorAddress{
					Address:   addr,
					Validated: valid,
					IP:        knownIP,
					Port:      port,
					PeerID:    peerID,
				}
				
				validator.Addresses = append(validator.Addresses, validatorAddr)
				validator.AddressStatus[addr] = "active"
				validator.P2PAddress = addr
				validator.IPAddress = knownIP
				
				// Construct standard service addresses based on IP
				validator.RPCAddress = fmt.Sprintf("http://%s:26657", knownIP)
				validator.APIAddress = fmt.Sprintf("http://%s:8080", knownIP)
				validator.MetricsAddress = fmt.Sprintf("http://%s:9090", knownIP)
			}
			
			// Also try to extract addresses from partition info
			if activePartInfo != nil {
				// Try to extract addresses from validator info
				addresses := nd.extractAddressesFromValidatorPartition(activePartInfo)
				nd.log("Found %d additional addresses for validator %s from partition info", len(addresses), validatorID)
				
				for _, addr := range addresses {
					// Skip if we already have this address
					if _, exists := validator.AddressStatus[addr]; exists {
						continue
					}
					
					// Try to parse and validate the multiaddress
					ip, port, peerID, valid := nd.addressDir.ValidateMultiaddress(addr)
					
					validatorAddr := ValidatorAddress{
						Address:   addr,
						Validated: valid,
						IP:        ip,
						Port:      port,
						PeerID:    peerID,
					}
					
					validator.Addresses = append(validator.Addresses, validatorAddr)
					validator.AddressStatus[addr] = "active"
					
					// Set the first valid address as the P2P address if we don't have one yet
					if valid && validator.P2PAddress == "" {
						validator.P2PAddress = addr
						validator.IPAddress = ip
						
						// Construct standard service addresses based on IP
						if ip != "" {
							validator.RPCAddress = fmt.Sprintf("http://%s:26657", ip)
							validator.APIAddress = fmt.Sprintf("http://%s:8080", ip)
							validator.MetricsAddress = fmt.Sprintf("http://%s:9090", ip)
						}
					}
				}
			}
			
			// Also try to extract base URL from validator info (legacy method)
			if activePartInfo != nil && len(validator.Addresses) == 0 {
				baseUrl := nd.extractBaseUrl(activePartInfo)
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
			
			// Log the validator information
			nd.log("Added BVN validator: %s with %d addresses for partition %s", validator.Name, len(validator.Addresses), partition.ID)
			for i, addr := range validator.Addresses {
				nd.log("  Address %d: %s (Valid: %v, IP: %s, Port: %s)", i+1, addr.Address, addr.Validated, addr.IP, addr.Port)
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

// extractAddressesFromPartition extracts addresses from protocol.PartitionInfo
func (nd *NetworkDiscovery) extractAddressesFromPartition(partInfo *protocol.PartitionInfo) []string {
	if partInfo == nil {
		return nil
	}

	// Get addresses from partition info
	addresses := make([]string, 0)
	
	// Extract addresses based on partition ID
	partitionID := partInfo.ID
	nd.log("Extracting addresses for partition %s", partitionID)
	
	// Add additional address extraction logic here based on the PartitionInfo structure
	// This would involve accessing any fields in the PartitionInfo that might contain address information
	
	// For mainnet, use well-known validator addresses
	if nd.networkType == "mainnet" {
		// Add some well-known mainnet validator addresses for testing
		// These are example addresses that would be replaced with actual discovery in production
		knownAddresses := map[string][]string{
			"dn": {
				"/ip4/65.108.73.121/tcp/16593/p2p/QmPublicAPI1",
				"/ip4/144.76.105.23/tcp/16593/p2p/QmPublicAPI2",
			},
			"Apollo": {
				"/ip4/65.21.231.58/tcp/26656/p2p/12D3KooWL1NF6fdTJ7N6SnEyqTrCxqCmYKxCaKJjMVc3qRNgfhQu", // Kompendium
				"/ip4/65.109.104.118/tcp/26656/p2p/12D3KooWHFrjfPdpGC5xfBLJK9TqJHnxZ1ZExJGXLBxXE7Sbx5YE", // LunaNova
				"/ip4/135.181.114.121/tcp/26656/p2p/12D3KooWBSEYV8Hk3SrGLwRMZ3QsVQmU2eNPMcuJeHUADoiRUGQk", // TurtleBoat
			},
			"Chandrayaan": {
				"/ip4/65.108.238.102/tcp/26656/p2p/12D3KooWLANUByqHFqGwzA9SvNv7NX6JwfMeg9Yx4jxGKCXoQwKm", // PrestigeIT
				"/ip4/65.108.141.109/tcp/26656/p2p/12D3KooWCMr9mU894i8JXJFHWd4nLBJfTXCwuSeCXMV4yvQdYHaJ", // Sphereon
				"/ip4/65.108.4.175/tcp/26656/p2p/12D3KooWJwzMvhDZQ5JJF9Lc3CCKyKN8B1xXWzRuUopNkRPQCRfL", // Sphereon
			},
			"Yutu": {
				"/ip4/65.108.0.22/tcp/26656/p2p/12D3KooWNMEKxFRpAzS8G3XbMGextBrWxqVMWCEXiZMP7rkiTQJC", // MusicCityNode
				"/ip4/65.109.85.226/tcp/26656/p2p/12D3KooWJvyP3VJYymTqG7e6kJ6tvzNg8MKtUuAGXQ3Gz8b3QobU", // HighStakes
				"/ip4/65.108.201.154/tcp/26656/p2p/12D3KooWRBhwfeP2Y9CDkBVUbxdRrMg3WpZx7Dk7jKaRA7SnLrjQ", // tfa
			},
		}
		
		if knownAddrs, ok := knownAddresses[partInfo.ID]; ok {
			addresses = append(addresses, knownAddrs...)
		}
	} else if nd.networkType == "testnet" {
		// Add some well-known testnet validator addresses
		knownAddresses := map[string][]string{
			"dn": {
				"/ip4/95.216.2.219/tcp/16593/p2p/QmTestnetDN1",
			},
			"Apollo": {
				"/ip4/65.108.73.121/tcp/26656/p2p/12D3KooWTestnetApollo1",
			},
		}
		
		if knownAddrs, ok := knownAddresses[partInfo.ID]; ok {
			addresses = append(addresses, knownAddrs...)
		}
	}
	
	// If we don't have any addresses yet, try to get known addresses based on partition ID
	if len(addresses) == 0 {
		// Try to get known addresses for this partition
		nd.log("No known addresses for partition %s from hardcoded list", partInfo.ID)
	}
	
	nd.log("Extracted %d addresses for partition %s", len(addresses), partInfo.ID)
	return addresses
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
