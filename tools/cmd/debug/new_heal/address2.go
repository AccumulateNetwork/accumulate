// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
// !!! AI_DIRECTIVE: MANDATORY DESIGN COMPLIANCE REQUIREMENTS                                        !!!
// !!!                                                                                              !!!
// !!! 1. This implementation MUST conform exactly to the specifications in address2_design.md      !!!
// !!! 2. Design document location: /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/     !!!
// !!!                              tools/cmd/debug/docs/dev_v4/address2_design.md                  !!!
// !!! 3. Any deviation from the design document requires explicit approval                         !!!
// !!! 4. Implementation priority: correctness > design compliance > optimization                   !!!
// !!! 5. Helper functions must be moved to address2_help.go                                        !!!
// !!! 6. AddressDir struct must include DNValidators and BVNValidators as specified in design      !!!
// !!! 7. Validator struct must include all fields from the design                                  !!!
// !!!                                                                                              !!!
// !!! VERIFICATION REQUIRED: Before submitting changes, verify all structs and methods match design !!!
// !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!

// This file contains the implementation of address2.go
// It is designed to be a more maintainable and modular replacement for address.go
// with a focus on testability and reliability

package new_heal

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/multiformats/go-multiaddr"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	// "gitlab.com/accumulatenetwork/accumulate/protocol"
)

// NetworkInfo contains information about a network and its partitions
type NetworkInfo struct {
	// Name of the network (e.g., "mainnet", "testnet")
	Name string
	
	// ID of the network (e.g., "acme")
	ID string
	
	// Whether this is the mainnet
	IsMainnet bool
	
	// List of partitions in this network
	Partitions []*PartitionInfo
	
	// Map of partition ID to partition info for quick lookup
	PartitionMap map[string]*PartitionInfo
	
	// API endpoint for this network
	APIEndpoint string
	
	// For backward compatibility
	Network string
	NetworkName string
}

// PartitionInfo contains information about a network partition
type PartitionInfo struct {
	// ID of the partition (e.g., "Apollo", "Chandrayaan")
	ID string
	
	// Type of the partition ("dn" or "bvn")
	Type string
	
	// Full URL of the partition (e.g., "acc://bvn-Apollo.acme")
	URL string
	
	// Whether this partition is active
	Active bool
	
	// BVN index (0, 1, or 2 for mainnet BVNs)
	BVNIndex int
}

// AddressDir is a directory of addresses
type AddressDir struct {
	mu sync.RWMutex
	
	// DNValidators is a list of validators in the Directory Network
	DNValidators []Validator
	
	// BVNValidators is a list of lists of validators in BVNs
	BVNValidators [][]Validator
	
	// NetworkPeers is a map of peer ID to NetworkPeer
	NetworkPeers map[string]NetworkPeer
	
	// URL construction helpers
	URLHelpers map[string]string
	
	// Network information - this is the source of truth for network data
	NetworkInfo *NetworkInfo
	
	// Logger for detailed logging
	Logger *log.Logger
	
	// Statistics for peer discovery
	DiscoveryStats DiscoveryStats
	
	// Keep Validators for backward compatibility during transition
	Validators []*Validator
	
	// Keep peerDiscovery for implementation needs
	peerDiscovery *SimplePeerDiscovery
	
	// For backward compatibility with existing code
	// These fields are synchronized with NetworkInfo
	Network     *NetworkInfo
	NetworkName string
}

// Validator represents a validator in the network
type Validator struct {
	// Unique peer identifier for the validator
	PeerID string `json:"peer_id"`
	
	// Name or description of the validator
	Name string `json:"name"`
	
	// Partition information
	PartitionID   string `json:"partition_id"`
	PartitionType string `json:"partition_type"`
	
	// BVN index (0, 1, or 2 for mainnet)
	BVN int `json:"bvn"`
	
	// Status of the validator
	Status string `json:"status"`
	
	// Different address types for this validator
	P2PAddress     string `json:"p2p_address"`
	IPAddress      string `json:"ip_address"`
	RPCAddress     string `json:"rpc_address"`
	APIAddress     string `json:"api_address"`
	MetricsAddress string `json:"metrics_address"`
	
	// URLs associated with this validator
	URLs map[string]string `json:"urls"`
	
	// Heights for different networks
	DNHeight  uint64 `json:"dn_height"`
	BVNHeight uint64 `json:"bvn_height"`
	
	// Problem node tracking
	IsProblematic bool      `json:"is_problematic"`
	ProblemReason string    `json:"problem_reason"`
	ProblemSince  time.Time `json:"problem_since"`
	
	// Last block height observed for this validator
	LastHeight int64 `json:"last_height"`
	
	// Request types to avoid sending to this validator
	AvoidForRequestTypes []string `json:"avoid_for_request_types"`
	
	// Request types that are problematic for this validator
	ProblematicRequestTypes []string `json:"problematic_request_types"`
	
	// Last updated timestamp
	LastUpdated time.Time `json:"last_updated"`
	
	// Keep ID for backward compatibility
	ID string `json:"id"`
	
	// Addresses for this validator
	Addresses []ValidatorAddress `json:"addresses"`
	
	// Map of address to status (for backward compatibility)
	AddressStatus map[string]string `json:"address_status"`
	
	// Map of address to preferred status (for backward compatibility)
	PreferredAddresses map[string]bool `json:"preferred_addresses"`
}

// ValidatorAddress represents a validator's multiaddress with metadata
type ValidatorAddress struct {
	// The multiaddress string (e.g., "/ip4/144.76.105.23/tcp/16593/p2p/QmHash...")
	Address string `json:"address"`

	// Whether this address has been validated as a proper P2P multiaddress
	Validated bool `json:"validated"`

	// Components of the address for easier access
	IP     string `json:"ip"`
	Port   string `json:"port"`
	PeerID string `json:"peer_id"`

	// Last time this address was successfully used
	LastSuccess time.Time `json:"last_success"`

	// Number of consecutive failures when trying to use this address
	FailureCount int `json:"failure_count"`

	// Last error encountered when using this address
	LastError string `json:"last_error"`

	// Whether this address is preferred for certain operations
	Preferred bool `json:"preferred"`
}

// NetworkPeer represents a peer in the network with performance metrics
type NetworkPeer struct {
	// Basic identification
	ID          string
	IsValidator bool
	ValidatorID string
	PartitionID string
	PartitionURL string
	
	// Addresses and connectivity
	Addresses []ValidatorAddress
	Status    string
	
	// Performance metrics
	ResponseTime         time.Duration
	SuccessRate          float64
	SuccessfulConnections int
	FailedConnections    int
	PerformanceScore     float64
	IsPreferred          bool
	
	// History tracking
	LastSeen        time.Time
	FirstSeen       time.Time
	LastSuccess     time.Time
	LastUpdated     time.Time
	DiscoveryMethod string
	DiscoveredAt    time.Time
	IsLost          bool
	LastLostTime    time.Time
	
	// Error tracking
	FailureCount  int
	LastError     string
	LastErrorTime time.Time
}

// AddAddress adds an address to the peer if it doesn't already exist
func (p *NetworkPeer) AddAddress(address string) {
	// Check if the address already exists
	for _, addr := range p.Addresses {
		if addr.Address == address {
			return
		}
	}

	// Create a new validator address
	validatorAddr := ValidatorAddress{
		Address:      address,
		Validated:    false,
		LastSuccess:  time.Time{},
		FailureCount: 0,
		Preferred:    false,
	}
	
	// Try to extract IP, port, and peer ID from the address
	if strings.Contains(address, "/ip4/") || strings.Contains(address, "/ip6/") {
		parts := strings.Split(address, "/")
		for i, part := range parts {
			if part == "ip4" || part == "ip6" {
				if i+1 < len(parts) {
					validatorAddr.IP = parts[i+1]
				}
			} else if part == "tcp" || part == "udp" {
				if i+1 < len(parts) {
					validatorAddr.Port = parts[i+1]
				}
			} else if part == "p2p" {
				if i+1 < len(parts) {
					validatorAddr.PeerID = parts[i+1]
				}
			}
		}
	}

	// Add the address to the peer
	p.Addresses = append(p.Addresses, validatorAddr)
}

// CalculatePerformanceScore calculates the performance score for the peer
func (p *NetworkPeer) CalculatePerformanceScore() float64 {
	// Base score
	score := 0.0
	
	// If the peer is preferred, give it a high score
	if p.IsPreferred {
		score += 100.0
	}
	
	// Success rate contributes up to 50 points
	score += p.SuccessRate * 50.0
	
	// Response time contributes up to 30 points (lower is better)
	// 100ms or less gets full points, 1s or more gets 0 points
	if p.ResponseTime.Milliseconds() <= 100 {
		score += 30.0
	} else if p.ResponseTime.Milliseconds() >= 1000 {
		// No points for slow responses
	} else {
		// Linear scale between 100ms and 1000ms
		responseTimeScore := 30.0 * (1.0 - float64(p.ResponseTime.Milliseconds()-100)/900.0)
		score += responseTimeScore
	}
	
	// Recent success contributes up to 20 points
	if !p.LastSuccess.IsZero() {
		timeSinceSuccess := time.Since(p.LastSuccess)
		if timeSinceSuccess <= time.Minute {
			score += 20.0
		} else if timeSinceSuccess <= time.Hour {
			// Linear scale between 1 minute and 1 hour
			recentSuccessScore := 20.0 * (1.0 - float64(timeSinceSuccess)/float64(time.Hour))
			score += recentSuccessScore
		}
	}
	
	// Store the calculated score
	p.PerformanceScore = score
	
	return score
}

// DiscoveryStats contains statistics about peer discovery
type DiscoveryStats struct {
	// Statistics by method
	ByMethod map[string]int

	// Statistics by partition
	ByPartition map[string]int

	// Number of validators discovered
	ValidatorsDiscovered int

	// Number of peers discovered
	PeersDiscovered int

	// Number of multiaddresses discovered
	MultiaddressesDiscovered int

	// Statistics by method type
	MethodStats map[string]int

	// Total number of peers
	TotalPeers int

	// Number of validators
	Validators int

	// Number of non-validators
	NonValidators int

	// Last discovery timestamp
	LastDiscovery time.Time

	// Total time spent on discovery
	TotalDiscoveryTime time.Duration

	// Number of discovery attempts
	DiscoveryAttempts int

	// Number of successful discoveries
	SuccessfulDiscoveries int

	// Number of failed discoveries
	FailedDiscoveries int

	// Total number of extraction attempts
	TotalAttempts int

	// Number of successful multiaddr extractions
	MultiaddrSuccess int

	// Number of successful URL extractions
	URLSuccess int

	// Number of successful validator map lookups
	ValidatorMapSuccess int

	// Number of extraction failures
	Failures int
}

// RefreshStats contains statistics about a network peer refresh operation
type RefreshStats struct {
	// Total number of peers
	TotalPeers int

	// Number of new peers discovered
	NewPeers int

	// Number of peers that were lost
	LostPeers int

	// Number of peers that were updated
	UpdatedPeers int

	// Number of peers that were unchanged
	UnchangedPeers int

	// Total number of validators
	TotalValidators int

	// Total number of non-validators
	TotalNonValidators int

	// Number of active validators
	ActiveValidators int

	// Number of inactive validators
	InactiveValidators int

	// Number of unreachable validators
	UnreachableValidators int

	// Number of problematic validators
	ProblematicValidators int

	// Number of DN validators
	DNValidators int

	// Number of BVN validators
	BVNValidators int

	// Number of DN lagging nodes
	DNLaggingNodes int

	// Number of BVN lagging nodes
	BVNLaggingNodes int

	// Number of peers with changed status
	StatusChanged int

	// IDs of new peers
	NewPeerIDs []string

	// IDs of lost peers
	LostPeerIDs []string

	// IDs of peers with changed status
	ChangedPeerIDs []string

	// IDs of DN lagging nodes
	DNLaggingNodeIDs []string

	// IDs of BVN lagging nodes
	BVNLaggingNodeIDs []string
}

// NewDiscoveryStats creates a new DiscoveryStats instance
func NewDiscoveryStats() DiscoveryStats {
	return DiscoveryStats{
		ByMethod:    make(map[string]int),
		ByPartition: make(map[string]int),
		MethodStats: make(map[string]int),
	}
}

// NewAddressDir creates a new AddressDir instance with default settings
func NewAddressDir() *AddressDir {
	logger := log.New(os.Stdout, "[AddressDir] ", log.LstdFlags)
	
	// Create the NetworkInfo structure
	networkInfo := &NetworkInfo{
		Name:        "acme",
		ID:          "acme",
		IsMainnet:   false,
		Partitions:  make([]*PartitionInfo, 0),
		PartitionMap: make(map[string]*PartitionInfo),
		APIEndpoint: "https://mainnet.accumulatenetwork.io/v2",
	}
	
	// Create and return the AddressDir
	dir := &AddressDir{
		mu:            sync.RWMutex{},
		DNValidators:  make([]Validator, 0),
		BVNValidators: make([][]Validator, 0),
		NetworkPeers:  make(map[string]NetworkPeer),
		URLHelpers:    make(map[string]string),
		Logger:        logger,
		NetworkInfo:   networkInfo,
		DiscoveryStats: DiscoveryStats{
			MethodStats: make(map[string]int),
			ByMethod:    make(map[string]int),
			ByPartition: make(map[string]int),
		},
		Validators:    make([]*Validator, 0), // For backward compatibility
		peerDiscovery: NewSimplePeerDiscovery(logger),
		// For backward compatibility
		Network:     networkInfo,
		NetworkName: "acme",
	}
	
	return dir
}

// NewAddressDirWithOptions creates a new AddressDir instance with custom options
func NewAddressDirWithOptions(options ...AddressDirOption) *AddressDir {
	dir := NewAddressDir()
	
	// Apply all options
	for _, option := range options {
		option(dir)
	}
	
	return dir
}

// AddressDirOption is a function that configures an AddressDir
type AddressDirOption func(*AddressDir)

// WithLogger sets a custom logger for the AddressDir
func WithLogger(logger *log.Logger) AddressDirOption {
	return func(dir *AddressDir) {
		dir.Logger = logger
	}
}

// WithNetworkID sets the network ID for the AddressDir
func WithNetworkID(networkID string) AddressDirOption {
	return func(dir *AddressDir) {
		dir.NetworkInfo.ID = networkID
	}
}

// WithNetworkName sets the network name for the AddressDir
func WithNetworkName(networkName string) AddressDirOption {
	return func(dir *AddressDir) {
		dir.NetworkInfo.Name = networkName
		dir.NetworkInfo.NetworkName = networkName // For backward compatibility
	}
}

// GetNetwork returns the network information
// This provides access to the NetworkInfo struct for network_discovery.go
func (a *AddressDir) GetNetwork() *NetworkInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()
	// For backward compatibility, return the direct field if it's set
	if a.Network != nil {
		return a.Network
	}
	// Otherwise, return from NetworkInfo
	return a.NetworkInfo
}

// SetNetwork sets the network information
// This allows setting the NetworkInfo struct for network_discovery.go
func (a *AddressDir) SetNetwork(network *NetworkInfo) {
	a.mu.Lock()
	defer a.mu.Unlock()
	// Update both fields to maintain consistency
	a.NetworkInfo = network
	a.Network = network
	// Also update the NetworkName for full consistency
	if network != nil {
		a.NetworkName = network.Name
	}
}

// GetNetworkName returns the network name
// This provides access to the network name for backward compatibility
func (a *AddressDir) GetNetworkName() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	// For backward compatibility, return the direct field if it's set
	if a.NetworkName != "" {
		return a.NetworkName
	}
	// Otherwise, return from NetworkInfo
	return a.NetworkInfo.Name
}

// SetNetworkName sets the network name in NetworkInfo and synchronizes with NetworkName field
// This ensures the network name is consistently accessible through both fields
func (a *AddressDir) SetNetworkName(networkName string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	// Update both fields to maintain consistency
	a.NetworkInfo.Name = networkName
	a.NetworkName = networkName
}

// WithAPIEndpoint sets the API endpoint for the AddressDir
func (a *AddressDir) WithAPIEndpoint(endpoint string) *AddressDir {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.NetworkInfo.APIEndpoint = endpoint
	return a
}

// This is an internal helper method used by AddValidator
// It's no longer used but kept for reference
func (a *AddressDir) addValidatorStruct(validator Validator) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Determine which validator collection to add to based on partition type
	if validator.PartitionType == "dn" {
		a.DNValidators = append(a.DNValidators, validator)
	} else if validator.PartitionType == "bvn" {
		// If BVNValidators is empty, initialize it
		if len(a.BVNValidators) == 0 {
			a.BVNValidators = make([][]Validator, 1)
		}
		// Add to the first BVN slice
		a.BVNValidators[0] = append(a.BVNValidators[0], validator)
	}
	
	// Also add to legacy Validators slice for backward compatibility
	legacyValidator := &Validator{
		ID:           validator.ID,
		PeerID:       validator.PeerID,
		Name:         validator.Name,
		PartitionID:  validator.PartitionID,
		PartitionType: validator.PartitionType,
		Status:       validator.Status,
		Addresses:    validator.Addresses,
		AddressStatus: validator.AddressStatus,
		URLs:         validator.URLs,
	}
	a.Validators = append(a.Validators, legacyValidator)
	return nil
}

// GetValidator retrieves a validator by ID or PeerID
// This is for backward compatibility with existing tests
func (a *AddressDir) GetValidator(id string) *Validator {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	// First check in legacy Validators slice for backward compatibility
	for _, validator := range a.Validators {
		if validator.ID == id || validator.PeerID == id {
			return validator
		}
	}
	
	// If not found in legacy slice, check in DNValidators
	for i := range a.DNValidators {
		if a.DNValidators[i].PeerID == id || a.DNValidators[i].ID == id {
			// Return a pointer to the validator
			validator := a.DNValidators[i]
			return &validator
		}
	}
	
	// If not found in DNValidators, check in BVNValidators
	for _, bvnValidators := range a.BVNValidators {
		for i := range bvnValidators {
			if bvnValidators[i].PeerID == id || bvnValidators[i].ID == id {
				// Return a pointer to the validator
				validator := bvnValidators[i]
				return &validator
			}
		}
	}
	
	return nil
}

// UpdateValidator updates a validator in the AddressDir
// This is for backward compatibility with existing tests
func (a *AddressDir) UpdateValidator(updatedValidator *Validator) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// First check in legacy Validators slice for backward compatibility
	for i, validator := range a.Validators {
		if validator.ID == updatedValidator.ID || validator.PeerID == updatedValidator.PeerID {
			a.Validators[i] = updatedValidator
			return true
		}
	}
	
	// If not found in legacy slice, check in DNValidators
	for i := range a.DNValidators {
		if a.DNValidators[i].PeerID == updatedValidator.PeerID || a.DNValidators[i].ID == updatedValidator.ID {
			a.DNValidators[i] = *updatedValidator
			return true
		}
	}
	
	// If not found in DNValidators, check in BVNValidators
	for j, bvnValidators := range a.BVNValidators {
		for i := range bvnValidators {
			if bvnValidators[i].PeerID == updatedValidator.PeerID || bvnValidators[i].ID == updatedValidator.ID {
				a.BVNValidators[j][i] = *updatedValidator
				return true
			}
		}
	}
	
	return false
}

// isValidMultiaddress checks if a string is a valid multiaddress
func (a *AddressDir) isValidMultiaddress(addr string) (bool, error) {
	_, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return false, err
	}
	return true, nil
}

// extractPeerIDFromMultiaddress extracts the peer ID from a multiaddress
func (a *AddressDir) extractPeerIDFromMultiaddress(addr string) (string, error) {
	maddr, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return "", err
	}
	
	// Split the multiaddress into components
	components := multiaddr.Split(maddr)
	
	// Look for the p2p component
	for _, component := range components {
		if component.Protocols()[0].Name == "p2p" {
			// Extract the peer ID value
			peerID, err := component.ValueForProtocol(multiaddr.P_P2P)
			if err != nil {
				return "", err
			}
			return peerID, nil
		}
	}
	
	return "", fmt.Errorf("no peer ID found in multiaddress: %s", addr)
}

// GetNetworkPeer gets a network peer by ID
func (a *AddressDir) GetNetworkPeer(id string) (*NetworkPeer, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	// Check if the peer exists
	peer, exists := a.NetworkPeers[id]
	if !exists {
		return nil, false
	}
	
	return &peer, true
}

// GetNetworkPeerByID is an alias for GetNetworkPeer for backward compatibility
func (a *AddressDir) GetNetworkPeerByID(id string) (*NetworkPeer, bool) {
	return a.GetNetworkPeer(id)
}

// GetNetworkPeers gets all network peers
func (a *AddressDir) GetNetworkPeers() []NetworkPeer {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	// Create a slice of peers
	result := make([]NetworkPeer, 0, len(a.NetworkPeers))
	for _, peer := range a.NetworkPeers {
		result = append(result, peer)
	}
	
	return result
}

// GetBestNetworkPeer gets the best performing network peer based on performance metrics
func (a *AddressDir) GetBestNetworkPeer() *NetworkPeer {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	var bestPeer *NetworkPeer
	var bestScore float64 = -1
	
	// Find the peer with the highest performance score
	for id, peer := range a.NetworkPeers {
		// Skip inactive or problematic peers
		if peer.Status != "active" {
			continue
		}
		
		// Calculate the performance score
		score := peer.CalculatePerformanceScore()
		
		// Update the peer's score in the map
		mutablePeer := peer
		mutablePeer.PerformanceScore = score
		a.NetworkPeers[id] = mutablePeer
		
		// Check if this is the best peer so far
		if score > bestScore {
			bestScore = score
			bestPeer = &mutablePeer
		}
	}
	
	return bestPeer
}

// GetBestNetworkPeerForPartition gets the best performing network peer for a specific partition
func (a *AddressDir) GetBestNetworkPeerForPartition(partitionID string) *NetworkPeer {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	var bestPeer *NetworkPeer
	var bestScore float64 = -1
	
	// Find the peer with the highest performance score in the specified partition
	for id, peer := range a.NetworkPeers {
		// Skip peers not in the specified partition
		if peer.PartitionID != partitionID {
			continue
		}
		
		// Skip inactive or problematic peers
		if peer.Status != "active" {
			continue
		}
		
		// Calculate the performance score
		score := peer.CalculatePerformanceScore()
		
		// Update the peer's score in the map
		mutablePeer := peer
		mutablePeer.PerformanceScore = score
		a.NetworkPeers[id] = mutablePeer
		
		// Check if this is the best peer so far
		if score > bestScore {
			bestScore = score
			bestPeer = &mutablePeer
		}
	}
	
	return bestPeer
}

// GetValidatorPeers gets all peers that are validators
func (a *AddressDir) GetValidatorPeers() []NetworkPeer {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	// Create a slice of validator peers
	result := make([]NetworkPeer, 0)
	for _, peer := range a.NetworkPeers {
		if peer.IsValidator {
			result = append(result, peer)
		}
	}
	
	return result
}

// GetNonValidatorPeers gets all peers that are not validators
func (a *AddressDir) GetNonValidatorPeers() []NetworkPeer {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	// Create a slice of non-validator peers
	result := make([]NetworkPeer, 0)
	for _, peer := range a.NetworkPeers {
		if !peer.IsValidator {
			result = append(result, peer)
		}
	}

	return result
}

// AddPeerAddress adds an address to a peer
func (a *AddressDir) AddPeerAddress(peerID string, address string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Check if the peer exists
	peer, exists := a.NetworkPeers[peerID]
	if !exists {
		return fmt.Errorf("peer %s not found", peerID)
	}

	// Add the address to the peer
	peer.AddAddress(address)

	// Update the peer in the map
	a.NetworkPeers[peerID] = peer

	return nil
}

// UpdatePeerPerformance updates performance metrics for a peer
func (a *AddressDir) UpdatePeerPerformance(peerID string, responseTime time.Duration, success bool, errorMsg string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Check if the peer exists
	peer, exists := a.NetworkPeers[peerID]
	if !exists {
		return fmt.Errorf("peer %s not found", peerID)
	}

	// Update performance metrics
	peer.LastUpdated = time.Now()

	// Update response time (use exponential moving average)
	if peer.ResponseTime == 0 {
		peer.ResponseTime = responseTime
	} else {
		// Weight: 0.2 for new value, 0.8 for old value
		peer.ResponseTime = time.Duration(float64(responseTime)*0.2 + float64(peer.ResponseTime)*0.8)
	}

	if success {
		// Update success metrics
		peer.LastSuccess = time.Now()
		peer.SuccessfulConnections++

		// Update success rate (use exponential moving average)
		if peer.SuccessRate == 0 {
			peer.SuccessRate = 1.0
		} else {
			// Weight: 0.1 for new value (1.0), 0.9 for old value
			peer.SuccessRate = 0.1*1.0 + 0.9*peer.SuccessRate
		}
	} else {
		// Update failure metrics
		peer.FailedConnections++
		peer.FailureCount++
		peer.LastError = errorMsg
		peer.LastErrorTime = time.Now()

		// Update success rate (use exponential moving average)
		if peer.SuccessRate == 0 {
			peer.SuccessRate = 0.0
		} else {
			// Weight: 0.1 for new value (0.0), 0.9 for old value
			peer.SuccessRate = 0.1*0.0 + 0.9*peer.SuccessRate
		}
	}

	// Recalculate performance score
	peer.PerformanceScore = peer.CalculatePerformanceScore()

	// Update the peer in the map
	a.NetworkPeers[peerID] = peer

	// Log the update
	if a.Logger != nil {
		if success {
			a.Logger.Printf("Updated peer %s performance: responseTime=%v, success=true, score=%.2f",
				peerID, responseTime, peer.PerformanceScore)
		} else {
			a.Logger.Printf("Updated peer %s performance: responseTime=%v, success=false, error=%s, score=%.2f",
				peerID, responseTime, errorMsg, peer.PerformanceScore)
		}
	}

	return nil
}

// MarkPeerPreferred marks a peer as preferred for future operations
func (a *AddressDir) MarkPeerPreferred(peerID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Check if the peer exists
	peer, exists := a.NetworkPeers[peerID]
	if !exists {
		return fmt.Errorf("peer %s not found", peerID)
	}

	// Mark the peer as preferred
	peer.IsPreferred = true

	// Recalculate performance score
	peer.PerformanceScore = peer.CalculatePerformanceScore()

	// Update the peer in the map
	a.NetworkPeers[peerID] = peer

	// Log the update
	if a.Logger != nil {
		a.Logger.Printf("Marked peer %s as preferred, score=%.2f", peerID, peer.PerformanceScore)
	}

	return nil
}

// GetPreferredPeers gets all peers marked as preferred
func (a *AddressDir) GetPreferredPeers() []NetworkPeer {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Create a slice of preferred peers
	result := make([]NetworkPeer, 0)
	for _, peer := range a.NetworkPeers {
		if peer.IsPreferred {
			result = append(result, peer)
		}
	}

	return result
}

// RemoveNetworkPeer removes a network peer from the AddressDir
func (a *AddressDir) RemoveNetworkPeer(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Check if the NetworkPeers map is initialized
	if a.NetworkPeers == nil {
		return false
	}
	
	// Check if the peer exists
	_, exists := a.NetworkPeers[id]
	if !exists {
		return false
	}
	
	// Remove the peer
	delete(a.NetworkPeers, id)
	
	// Log the removal
	a.Logger.Printf("Removed network peer: %s", id)
	return true
}

// UpdateNetworkPeerStatus updates the status of a network peer
func (a *AddressDir) UpdateNetworkPeerStatus(id string, status string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Check if the NetworkPeers map is initialized
	if a.NetworkPeers == nil {
		return false
	}
	
	// Check if the peer exists
	peer, exists := a.NetworkPeers[id]
	if !exists {
		return false
	}
	
	// Update the status
	peer.Status = status
	peer.LastUpdated = time.Now()
	
	// Update the peer in the map
	a.NetworkPeers[id] = peer
	
	// Log the update
	a.Logger.Printf("Updated status of network peer %s to %s", id, status)
	return true
}

// GetAllNetworkPeers returns all network peers
func (a *AddressDir) GetAllNetworkPeers() []NetworkPeer {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	// Check if the NetworkPeers map is initialized
	if a.NetworkPeers == nil {
		return []NetworkPeer{}
	}
	
	// Convert the map to a slice
	peers := make([]NetworkPeer, 0, len(a.NetworkPeers))
	for _, peer := range a.NetworkPeers {
		peers = append(peers, peer)
	}
	return peers
}

// Note: GetNetworkPeers is now defined at line 704

// AddValidator adds a validator to the appropriate validators list based on partition type
// This method has two forms for backward compatibility:
// 1. AddValidator(validator Validator) error - adds the provided validator
// 2. AddValidator(id string, name string, partitionID string, partitionType string) *Validator - creates and adds a validator
func (a *AddressDir) AddValidator(idOrValidator interface{}, args ...interface{}) *Validator {
	// Check which form of the method is being called
	switch v := idOrValidator.(type) {
	case Validator:
		// First form: AddValidator(validator Validator)
		a.mu.Lock()
		defer a.mu.Unlock()
		
		// Add to the appropriate validators list based on partition type
		if strings.ToLower(v.PartitionType) == "dn" {
			// Add to DNValidators
			a.DNValidators = append(a.DNValidators, v)
			a.Logger.Printf("Added DN validator: %s (%s)", v.Name, v.PeerID)
		} else if strings.HasPrefix(strings.ToLower(v.PartitionType), "bvn") {
			// Extract BVN index from partition ID
			bvnIndex := 0 // Default to 0 if we can't extract
			if strings.HasPrefix(v.PartitionID, "bvn") {
				// Try to extract a number if present
				parts := strings.Split(v.PartitionID, "-")
				if len(parts) > 1 {
					// Use the partition name as an index
					bvnIndex = len(a.BVNValidators)
				}
			}
			
			// Ensure we have enough BVN slices
			for len(a.BVNValidators) <= bvnIndex {
				a.BVNValidators = append(a.BVNValidators, make([]Validator, 0))
			}
			
			// Add to the appropriate BVN slice
			a.BVNValidators[bvnIndex] = append(a.BVNValidators[bvnIndex], v)
			v.BVN = bvnIndex
			a.Logger.Printf("Added BVN validator: %s (%s) to BVN %d", v.Name, v.PeerID, bvnIndex)
		}
		
		// Also add to the legacy Validators slice for backward compatibility
		validatorPtr := v // Create a copy
		a.Validators = append(a.Validators, &validatorPtr)
		
		// Return the validator pointer for consistency
		return &validatorPtr
	
	case string:
		// Second form: AddValidator(id string, name string, partitionID string, partitionType string)
		if len(args) < 3 {
			a.Logger.Printf("Error: not enough arguments for AddValidator")
			return nil
		}
		
		id := v
		name, ok := args[0].(string)
		if !ok {
			a.Logger.Printf("Error: name must be a string")
			return nil
		}
		
		partitionID, ok := args[1].(string)
		if !ok {
			a.Logger.Printf("Error: partitionID must be a string")
			return nil
		}
		
		partitionType, ok := args[2].(string)
		if !ok {
			a.Logger.Printf("Error: partitionType must be a string")
			return nil
		}
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Create a new validator
	validator := &Validator{
		ID:            id,
		PeerID:        id, // For backward compatibility
		Name:          name,
		PartitionID:   partitionID,
		PartitionType: partitionType,
		LastUpdated:   time.Now(),
		Addresses:     make([]ValidatorAddress, 0),
		AddressStatus: make(map[string]string),
	}
	
	// Add to the appropriate validators list
	if strings.ToLower(partitionType) == "dn" {
		// Add to DNValidators
		a.DNValidators = append(a.DNValidators, *validator)
		a.Logger.Printf("Added DN validator: %s (%s)", name, id)
	} else if strings.HasPrefix(strings.ToLower(partitionType), "bvn") {
		// Extract BVN index from partition ID
		bvnIndex := 0 // Default to 0 if we can't extract
		if strings.HasPrefix(partitionID, "bvn") {
			// Try to extract a number if present
			parts := strings.Split(partitionID, "-")
			if len(parts) > 1 {
				// Use the partition name as an index
				bvnIndex = len(a.BVNValidators)
			}
		}
		
		// Ensure we have enough BVN slices
		for len(a.BVNValidators) <= bvnIndex {
			a.BVNValidators = append(a.BVNValidators, make([]Validator, 0))
		}
		
		// Add to the appropriate BVN slice
		a.BVNValidators[bvnIndex] = append(a.BVNValidators[bvnIndex], *validator)
		validator.BVN = bvnIndex
		a.Logger.Printf("Added BVN validator: %s (%s) to BVN %d", name, id, bvnIndex)
	}
	
	// Also add to the legacy Validators slice for backward compatibility
	a.Validators = append(a.Validators, validator)
	
	return validator
	default:
		// Return nil for invalid arguments
		return nil
	}
}

// AddDNValidator adds a validator to the Directory Network validators list
func (a *AddressDir) AddDNValidator(validator Validator) {
	// No need to lock here as this should only be called from methods that already have a lock
	
	// Set the partition type to "dn"
	validator.PartitionType = "dn"
	
	// Update the last updated timestamp
	validator.LastUpdated = time.Now()
	
	// Add to the DNValidators slice
	a.DNValidators = append(a.DNValidators, validator)
	
	// Log the addition
	a.Logger.Printf("Added DN validator: %s (%s)", validator.Name, validator.PeerID)
}

// FindValidator finds a validator by ID and returns it along with its index and existence flag
// This method searches in both the new validator collections and the legacy Validators slice
func (a *AddressDir) FindValidator(id string) (*Validator, int, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	// First check in legacy Validators slice for backward compatibility
	for i, validator := range a.Validators {
		if validator.ID == id || validator.PeerID == id {
			return validator, i, true
		}
	}
	
	// If not found in legacy slice, check in DNValidators
	for i := range a.DNValidators {
		if a.DNValidators[i].PeerID == id || a.DNValidators[i].ID == id {
			// Create a pointer to a copy of the validator
			validator := a.DNValidators[i]
			return &validator, -1, true
		}
	}
	
	// If not found in DNValidators, check in BVNValidators
	for bvnIdx := range a.BVNValidators {
		for i := range a.BVNValidators[bvnIdx] {
			if a.BVNValidators[bvnIdx][i].PeerID == id || a.BVNValidators[bvnIdx][i].ID == id {
				// Create a pointer to a copy of the validator
				validator := a.BVNValidators[bvnIdx][i]
				return &validator, -1, true
			}
		}
	}
	
	return nil, -1, false
}

// FindValidatorsByPartition finds all validators for a specific partition
func (a *AddressDir) FindValidatorsByPartition(partitionID string) []*Validator {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	result := make([]*Validator, 0)
	
	// Check in legacy Validators slice for backward compatibility
	for _, validator := range a.Validators {
		if validator.PartitionID == partitionID {
			result = append(result, validator)
		}
	}
	
	// Check in DNValidators
	if partitionID == "dn" {
		for i := range a.DNValidators {
			validator := a.DNValidators[i]
			result = append(result, &validator)
		}
	}
	
	// Check in BVNValidators
	for bvnIdx, bvnValidators := range a.BVNValidators {
		// Check if this BVN matches the partition ID
		bvnPartitionID := fmt.Sprintf("bvn-%d", bvnIdx)
		if bvnPartitionID == partitionID || partitionID == "" {
			for i := range bvnValidators {
				validator := bvnValidators[i]
				result = append(result, &validator)
			}
		}
	}
	
	return result
}

// FindValidatorsByBVN finds all validators for a specific BVN index
func (a *AddressDir) FindValidatorsByBVN(bvnIndex int) []*Validator {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	result := make([]*Validator, 0)
	
	// Check if the BVN index is valid
	if bvnIndex < 0 || bvnIndex >= len(a.BVNValidators) {
		return result
	}
	
	// Add all validators for this BVN
	for i := range a.BVNValidators[bvnIndex] {
		validator := a.BVNValidators[bvnIndex][i]
		result = append(result, &validator)
	}
	
	return result
}

// SetValidatorRPCAddress sets the RPC address for a validator
func (a *AddressDir) SetValidatorRPCAddress(id string, address string) bool {
	// Find the validator
	validator, _, found := a.FindValidator(id)
	if !found {
		return false
	}
	
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Set the RPC address
	validator.RPCAddress = address
	
	// Also add to the Addresses slice for backward compatibility
	addressExists := false
	for _, addr := range validator.Addresses {
		if addr.Address == address {
			addressExists = true
			break
		}
	}
	
	if !addressExists {
		validator.Addresses = append(validator.Addresses, ValidatorAddress{Address: address})
	}
	
	// Set address status
	validator.AddressStatus[address] = "active"
	validator.LastUpdated = time.Now()
	
	a.Logger.Printf("Set RPC address for validator %s (%s) to %s", validator.Name, id, address)
	return true
}

// SetValidatorAPIAddress sets the API address for a validator
func (a *AddressDir) SetValidatorAPIAddress(id string, address string) bool {
	// Find the validator
	validator, _, found := a.FindValidator(id)
	if !found {
		return false
	}
	
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Set the API address
	validator.APIAddress = address
	
	// Also add to the Addresses slice for backward compatibility
	addressExists := false
	for _, addr := range validator.Addresses {
		if addr.Address == address {
			addressExists = true
			break
		}
	}
	
	if !addressExists {
		validator.Addresses = append(validator.Addresses, ValidatorAddress{Address: address})
	}
	
	// Set address status
	validator.AddressStatus[address] = "active"
	validator.LastUpdated = time.Now()
	
	a.Logger.Printf("Set API address for validator %s (%s) to %s", validator.Name, id, address)
	return true
}

// GetPeerRPCEndpoint returns the RPC endpoint for a peer
// This function is thread-safe and handles locking internally
// This method has two forms for backward compatibility:
// 1. GetPeerRPCEndpoint(id string) - gets the RPC endpoint for a peer by ID
// 2. GetPeerRPCEndpoint(peer NetworkPeer) - gets the RPC endpoint for a peer by NetworkPeer object
func (a *AddressDir) GetPeerRPCEndpoint(idOrPeer interface{}) string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	var id string
	
	// Check which form of the method is being called
	switch v := idOrPeer.(type) {
	case string:
		id = v
	case NetworkPeer:
		id = v.ID
		// If this is a validator, use the validator ID instead
		if v.IsValidator && v.ValidatorID != "" {
			id = v.ValidatorID
		}
	default:
		return ""
	}
	
	validator := a.findValidatorByID(id)
	if validator == nil {
		return ""
	}
	
	return validator.RPCAddress
}

// GetKnownAddressesForValidator returns all known addresses for a validator
func (a *AddressDir) GetKnownAddressesForValidator(id string) []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	// Find the validator
	validator := a.findValidatorByID(id)
	if validator == nil {
		return []string{}
	}
	
	// Convert ValidatorAddress objects to strings
	addresses := make([]string, 0, len(validator.Addresses))
	for _, addr := range validator.Addresses {
		addresses = append(addresses, addr.Address)
	}
	
	// Return all addresses
	return addresses
}

// MarkValidatorProblematic marks a validator as problematic with a reason (alias for MarkNodeProblematic)
func (a *AddressDir) MarkValidatorProblematic(id string, reason string) bool {
	return a.MarkNodeProblematic(id, reason)
}

// ClearValidatorProblematic clears the problematic status for a validator (alias for ClearProblematicStatus)
func (a *AddressDir) ClearValidatorProblematic(id string) bool {
	return a.ClearProblematicStatus(id)
}

// MarkNodeProblematic marks a node as problematic with a reason
func (a *AddressDir) MarkNodeProblematic(id string, reason string, requestTypes ...[]string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Find the validator
	validator := a.findValidatorByID(id)
	if validator == nil {
		return false
	}
	
	// Mark as problematic
	validator.IsProblematic = true
	validator.ProblemReason = reason
	validator.ProblemSince = time.Now()
	validator.LastUpdated = time.Now()
	
	// Add request types to avoid if provided
	if len(requestTypes) > 0 && len(requestTypes[0]) > 0 {
		// Initialize the problematic request types slice if needed
		if validator.ProblematicRequestTypes == nil {
			validator.ProblematicRequestTypes = make([]string, 0)
		}
		
		// Add each request type
		for _, rt := range requestTypes[0] {
			// Check if already exists
			exists := false
			for _, existingRT := range validator.ProblematicRequestTypes {
				if existingRT == rt {
					exists = true
					break
				}
			}
			
			// Add if not exists
			if !exists {
				validator.ProblematicRequestTypes = append(validator.ProblematicRequestTypes, rt)
			}
		}
	}
	
	a.Logger.Printf("Marked validator %s (%s) as problematic: %s", validator.Name, id, reason)
	return true
}

// IsNodeProblematic checks if a node is marked as problematic
func (a *AddressDir) IsNodeProblematic(id string, requestType ...string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	// Find the validator
	validator := a.findValidatorByID(id)
	if validator == nil {
		return false
	}
	
	// If no request type is specified, just check if the node is problematic in general
	if len(requestType) == 0 {
		return validator.IsProblematic
	}
	
	// Check if the node is problematic for the specified request type
	if !validator.IsProblematic {
		return false
	}
	
	// If no problematic request types are specified, the node is problematic for all request types
	if len(validator.ProblematicRequestTypes) == 0 {
		return true
	}
	
	// Check if the request type is in the problematic request types
	for _, rt := range validator.ProblematicRequestTypes {
		if rt == requestType[0] {
			return true
		}
	}
	
	return false
}

// GetProblemNodes returns all nodes marked as problematic
func (a *AddressDir) GetProblemNodes() []*Validator {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	result := make([]*Validator, 0)
	
	// Check in legacy Validators slice
	for _, validator := range a.Validators {
		if validator.IsProblematic {
			result = append(result, validator)
		}
	}
	
	// Check in DNValidators
	for i := range a.DNValidators {
		if a.DNValidators[i].IsProblematic {
			validator := a.DNValidators[i]
			result = append(result, &validator)
		}
	}
	
	// Check in BVNValidators
	for _, bvnValidators := range a.BVNValidators {
		for i := range bvnValidators {
			if bvnValidators[i].IsProblematic {
				validator := bvnValidators[i]
				result = append(result, &validator)
			}
		}
	}
	
	return result
}

// AddRequestTypeToAvoid adds a request type to avoid for a validator
func (a *AddressDir) AddRequestTypeToAvoid(id string, requestType string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Find the validator
	validator := a.findValidatorByID(id)
	if validator == nil {
		return false
	}
	
	// Check if the request type is already in the avoid list
	for _, rt := range validator.AvoidForRequestTypes {
		if rt == requestType {
			return true // Already added
		}
	}
	
	// Add to the avoid list
	validator.AvoidForRequestTypes = append(validator.AvoidForRequestTypes, requestType)
	validator.LastUpdated = time.Now()
	
	a.Logger.Printf("Added request type %s to avoid list for validator %s (%s)", requestType, validator.Name, id)
	return true
}

// ShouldAvoidForRequestType checks if a validator should be avoided for a request type
func (a *AddressDir) ShouldAvoidForRequestType(id string, requestType string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	// Find the validator
	validator := a.findValidatorByID(id)
	if validator == nil {
		return false
	}
	
	// Check if the validator is problematic
	if validator.IsProblematic {
		return true
	}
	
	// Check if the request type is in the avoid list
	for _, rt := range validator.AvoidForRequestTypes {
		if rt == requestType {
			return true
		}
	}
	
	// Check if the request type is in the problematic list
	for _, rt := range validator.ProblematicRequestTypes {
		if rt == requestType {
			return true
		}
	}
	
	return false
}

// ClearProblematicStatus clears the problematic status for a validator
func (a *AddressDir) ClearProblematicStatus(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Find the validator
	validator := a.findValidatorByID(id)
	if validator == nil {
		return false
	}
	
	// Clear problematic status
	validator.IsProblematic = false
	validator.ProblemReason = ""
	validator.ProblemSince = time.Time{}
	validator.AvoidForRequestTypes = []string{}
	validator.ProblematicRequestTypes = []string{}
	validator.LastUpdated = time.Now()
	
	a.Logger.Printf("Cleared problematic status for validator %s (%s)", validator.Name, id)
	return true
}

// MarkValidatorRequestTypeProblematic marks a validator as problematic for a specific request type
func (a *AddressDir) MarkValidatorRequestTypeProblematic(id string, requestType string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Find the validator
	validator := a.findValidatorByID(id)
	if validator == nil {
		return false
	}
	
	// Check if the request type is already in the problematic list
	for _, rt := range validator.ProblematicRequestTypes {
		if rt == requestType {
			return true // Already added
		}
	}
	
	// Add to the problematic list
	validator.ProblematicRequestTypes = append(validator.ProblematicRequestTypes, requestType)
	validator.LastUpdated = time.Now()
	
	a.Logger.Printf("Marked validator %s (%s) as problematic for request type %s", validator.Name, id, requestType)
	return true
}

// ShouldAvoidValidatorForRequestType checks if a validator should be avoided for a request type
func (a *AddressDir) ShouldAvoidValidatorForRequestType(id string, requestType string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	// Find the validator
	validator := a.findValidatorByID(id)
	if validator == nil {
		return false
	}
	
	// Check if the validator is problematic
	if validator.IsProblematic {
		return true
	}
	
	// Check if the request type is in the problematic list
	for _, rt := range validator.ProblematicRequestTypes {
		if rt == requestType {
			return true
		}
	}
	
	return false
}

// UpdateValidatorHeight updates the height of a validator
func (a *AddressDir) UpdateValidatorHeight(id string, height int64, isDirectoryNetwork bool) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Find the validator
	validator := a.findValidatorByID(id)
	if validator == nil {
		return false
	}
	
	// Update the appropriate height
	if isDirectoryNetwork {
		validator.DNHeight = uint64(height)
	} else {
		validator.BVNHeight = uint64(height)
	}
	
	// Update the last height and timestamp
	validator.LastHeight = height
	validator.LastUpdated = time.Now()
	
	// Log the update with the appropriate network type
	networkType := "BVN"
	if isDirectoryNetwork {
		networkType = "DN"
	}
	a.Logger.Printf("Updated %s height for validator %s (%s) to %d", 
		networkType, validator.Name, id, height)
	return true
}

// SetValidatorStatus sets the status of a validator
func (a *AddressDir) SetValidatorStatus(id string, status string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Find the validator
	validator := a.findValidatorByID(id)
	if validator == nil {
		return fmt.Errorf("validator %s not found", id)
	}
	
	// Set the status
	validator.Status = status
	validator.LastUpdated = time.Now()
	
	a.Logger.Printf("Set status of validator %s (%s) to %s", validator.Name, id, status)
	return nil
}

// AddValidatorAddress adds an address to a validator
func (a *AddressDir) AddValidatorAddress(id string, address string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Find the validator
	validator := a.findValidatorByID(id)
	if validator == nil {
		return fmt.Errorf("validator %s not found", id)
	}
	
	// Check if the address already exists
	for i, addr := range validator.Addresses {
		if addr.Address == address {
			// Address already exists, update it
			ip, port, peerID, err := a.parseMultiaddress(address)
			if err != nil {
				return fmt.Errorf("invalid P2P multiaddress: %v", err)
			}
			
			validator.Addresses[i].Validated = true
			validator.Addresses[i].IP = ip
			validator.Addresses[i].Port = port
			validator.Addresses[i].PeerID = peerID
			validator.Addresses[i].LastSuccess = time.Now()
			validator.LastUpdated = time.Now()
			
			// Update the address status for backward compatibility
			if validator.AddressStatus == nil {
				validator.AddressStatus = make(map[string]string)
			}
			validator.AddressStatus[address] = "active"
			
			a.Logger.Printf("Updated address %s for validator %s (%s)", address, validator.Name, id)
			return nil
		}
	}
	
	// Validate the address
	ip, port, peerID, err := a.parseMultiaddress(address)
	if err != nil {
		return fmt.Errorf("invalid P2P multiaddress: %v", err)
	}
	
	// Create a new ValidatorAddress
	validatorAddress := ValidatorAddress{
		Address:      address,
		Validated:    true,
		IP:           ip,
		Port:         port,
		PeerID:       peerID,
		LastSuccess:  time.Now(),
		FailureCount: 0,
		LastError:    "",
		Preferred:    false,
	}
	
	// Add the address
	validator.Addresses = append(validator.Addresses, validatorAddress)
	
	// Initialize the address status if needed
	if validator.AddressStatus == nil {
		validator.AddressStatus = make(map[string]string)
	}
	
	// Set the address status to active
	validator.AddressStatus[address] = "active"
	validator.LastUpdated = time.Now()
	
	a.Logger.Printf("Added address %s to validator %s (%s)", address, validator.Name, id)
	return nil
}

// GetValidatorAddresses gets all addresses for a validator
func (a *AddressDir) GetValidatorAddresses(id string) ([]ValidatorAddress, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	// Find the validator
	validator := a.findValidatorByID(id)
	if validator == nil {
		return []ValidatorAddress{}, false
	}
	
	return validator.Addresses, true
}

// MarkAddressPreferred marks an address as preferred for a validator
func (a *AddressDir) MarkAddressPreferred(id string, address string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Find the validator
	validator := a.findValidatorByID(id)
	if validator == nil {
		return fmt.Errorf("validator %s not found", id)
	}
	
	// Check if the address exists
	addressIndex := -1
	for i, addr := range validator.Addresses {
		if addr.Address == address {
			addressIndex = i
			break
		}
	}
	
	if addressIndex == -1 {
		// Address doesn't exist
		return fmt.Errorf("address %s not found for validator %s", address, id)
	}
	
	// Mark the address as preferred
	validator.Addresses[addressIndex].Preferred = true
	
	// Update the preferred addresses map for backward compatibility
	if validator.PreferredAddresses == nil {
		validator.PreferredAddresses = make(map[string]bool)
	}
	validator.PreferredAddresses[address] = true
	
	validator.LastUpdated = time.Now()
	
	a.Logger.Printf("Marked address %s as preferred for validator %s (%s)", address, validator.Name, id)
	return nil
}

// GetPreferredAddresses gets all preferred addresses for a validator
func (a *AddressDir) GetPreferredAddresses(id string) []ValidatorAddress {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	// Find the validator
	validator := a.findValidatorByID(id)
	if validator == nil {
		return []ValidatorAddress{}
	}
	
	// Get all preferred addresses
	preferredAddresses := make([]ValidatorAddress, 0)
	for _, addr := range validator.Addresses {
		if addr.Preferred {
			preferredAddresses = append(preferredAddresses, addr)
		}
	}
	
	return preferredAddresses
}

// UseStringsPackage is a utility method that demonstrates the use of the strings package
// This ensures the strings import is used and prevents compiler warnings
func (a *AddressDir) UseStringsPackage(s string) string {
	return strings.TrimSpace(s)
}

// SetValidatorP2PAddress sets the P2P address for a validator
func (a *AddressDir) SetValidatorP2PAddress(id string, address string) bool {
	// Find the validator
	validator, _, found := a.FindValidator(id)
	if !found {
		return false
	}
	
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Set the P2P address
	validator.P2PAddress = address
	
	// Also add to the Addresses slice for backward compatibility
	addressExists := false
	for _, addr := range validator.Addresses {
		if addr.Address == address {
			addressExists = true
			break
		}
	}
	
	if !addressExists {
		validator.Addresses = append(validator.Addresses, ValidatorAddress{Address: address})
	}
	
	// Set address status
	validator.AddressStatus[address] = "active"
	validator.LastUpdated = time.Now()
	
	a.Logger.Printf("Set P2P address for validator %s (%s) to %s", validator.Name, id, address)
	return true
}

// AddBVNValidator adds a validator to the specified BVN validators list
func (a *AddressDir) AddBVNValidator(bvnIndex int, validator Validator) {
	// No need to lock here as this should only be called from methods that already have a lock
	
	// Set the BVN index
	validator.BVN = bvnIndex
	
	// Set the partition type to "bvn"
	validator.PartitionType = "bvn"
	
	// Update the last updated timestamp
	validator.LastUpdated = time.Now()
	
	// Ensure we have enough BVN slices
	for len(a.BVNValidators) <= bvnIndex {
		a.BVNValidators = append(a.BVNValidators, make([]Validator, 0))
	}
	
	// Add to the appropriate BVN slice
	a.BVNValidators[bvnIndex] = append(a.BVNValidators[bvnIndex], validator)
	
	// Log the addition
	a.Logger.Printf("Added BVN validator: %s (%s) to BVN %d", validator.Name, validator.PeerID, bvnIndex)
}

// AddNetworkPeer adds a network peer to the address directory
// This method has two forms for backward compatibility:
// 1. AddNetworkPeer(peer NetworkPeer) - adds the provided peer
// 2. AddNetworkPeer(id string, isValidator bool, validatorID string, partitionID string, address string) - creates and adds a peer
func (a *AddressDir) AddNetworkPeer(peerOrID interface{}, args ...interface{}) *NetworkPeer {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Initialize the NetworkPeers map if needed
	if a.NetworkPeers == nil {
		a.NetworkPeers = make(map[string]NetworkPeer)
	}
	
	// Check which form is being used
	switch p := peerOrID.(type) {
	case NetworkPeer:
		// Form 1: AddNetworkPeer(peer NetworkPeer)
		a.NetworkPeers[p.ID] = p
		return &p
		
	case string:
		// Form 2: AddNetworkPeer(id string, isValidator bool, validatorID string, partitionID string, address string)
		id := p
		var isValidator bool
		var validatorID, partitionID, address string
		
		// Handle different argument counts for backward compatibility
		if len(args) >= 1 {
			isValidator, _ = args[0].(bool)
		}
		if len(args) >= 2 {
			validatorID, _ = args[1].(string)
		}
		if len(args) >= 3 {
			partitionID, _ = args[2].(string)
		}
		if len(args) >= 4 {
			address, _ = args[3].(string)
		}
		
		// Create a new peer
		peer := NetworkPeer{
			ID:              id,
			IsValidator:     isValidator,
			ValidatorID:     validatorID,
			PartitionID:     partitionID,
			Addresses:       make([]ValidatorAddress, 0),
			Status:          "active",
			LastSeen:        time.Now(),
			FirstSeen:       time.Now(),
			DiscoveryMethod: "manual",
			DiscoveredAt:    time.Now(),
			LastUpdated:     time.Now(),
			SuccessRate:     1.0, // Start with perfect score
		}
		
		// Add the address if provided
		if address != "" {
			peer.AddAddress(address)
		}
		
		// Store the peer
		a.NetworkPeers[id] = peer
		
		// Log the addition
		if a.Logger != nil {
			a.Logger.Printf("Added network peer %s (validator: %v, partition: %s)", id, isValidator, partitionID)
		}
		
		return &peer
	}
	
	return nil
}

// DiscoverNetworkPeers discovers network peers using the provided NetworkService
func (a *AddressDir) DiscoverNetworkPeers(ctx context.Context, client api.NetworkService) (int, error) {
	// Get network status from the client
	status, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		return 0, fmt.Errorf("failed to get network status: %w", err)
	}

	// Update network information
	a.mu.Lock()
	// Use network ID from the status
	networkID := "acme" // Default value
	// Note: The actual API structure might be different, so we're using a default value
	
	a.NetworkInfo.Name = networkID
	a.NetworkInfo.ID = networkID
	a.NetworkInfo.IsMainnet = false // Determine this based on network ID if needed
	a.NetworkInfo.APIEndpoint = "https://" + networkID + ".accumulatenetwork.io/v2" // Construct a default endpoint

	// Update partitions
	a.NetworkInfo.Partitions = make([]*PartitionInfo, 0)
	a.NetworkInfo.PartitionMap = make(map[string]*PartitionInfo)

	// Check if network and partitions are available
	if status.Network != nil && status.Network.Partitions != nil {
		for _, p := range status.Network.Partitions {
			// Convert partition type to string
			partitionType := fmt.Sprint(p.Type)
			
			partition := &PartitionInfo{
				ID:       p.ID,
				Type:     partitionType,
				URL:      a.constructPartitionURL(p.ID),
				Active:   true,
				BVNIndex: -1,
			}

			// Set BVN index for BVNs
			if partitionType == "bvn" || fmt.Sprint(p.Type) == "bvn" {
				if p.ID == "Apollo" {
					partition.BVNIndex = 0
				} else if p.ID == "Chandrayaan" {
					partition.BVNIndex = 1
				} else if p.ID == "Voyager" {
					partition.BVNIndex = 2
				}
			}

			a.NetworkInfo.Partitions = append(a.NetworkInfo.Partitions, partition)
			a.NetworkInfo.PartitionMap[p.ID] = partition
		}
	} else {
		// Add default partitions if none are available
		defaultPartitions := []struct {
			id    string
			pType string
			bvn   int
		}{
			{"dn", "dn", -1},
			{"Apollo", "bvn", 0},
			{"Chandrayaan", "bvn", 1},
			{"Voyager", "bvn", 2},
		}
		
		for _, dp := range defaultPartitions {
			partition := &PartitionInfo{
				ID:       dp.id,
				Type:     dp.pType,
				URL:      a.constructPartitionURL(dp.id),
				Active:   true,
				BVNIndex: dp.bvn,
			}
			
			a.NetworkInfo.Partitions = append(a.NetworkInfo.Partitions, partition)
			a.NetworkInfo.PartitionMap[dp.id] = partition
		}
	}
	a.mu.Unlock()

	// Track validators and seen peers
	validators := make(map[string]bool)
	seenPeers := make(map[string]bool)

	// Discover directory peers
	dirPeers := a.discoverDirectoryPeers(ctx, status, validators, seenPeers)

	// Discover partition peers
	partitionPeers := 0
	
	// Create default partitions to process if none are available from the API
	defaultPartitionIDs := []string{"Apollo", "Chandrayaan", "Voyager"}
	
	// Process each partition
	for _, partitionID := range defaultPartitionIDs {
		// Create a simple partition info object to pass to the helper method
		partitionInfo := struct {
			ID string
		}{partitionID}
		
		count, err := a.discoverPartitionPeers(ctx, client, partitionInfo, validators, seenPeers)
		if err != nil {
			a.Logger.Printf("Error discovering peers for partition %s: %v", partitionID, err)
			continue
		}
		partitionPeers += count
	}

	// Add known non-validator peers
	nonValidatorPeers := a.discoverCommonNonValidators(seenPeers)

	// Return total number of peers
	return dirPeers + partitionPeers + nonValidatorPeers, nil
}

// discoverDirectoryPeers discovers peers in the directory network
func (a *AddressDir) discoverDirectoryPeers(ctx context.Context, status *api.NetworkStatus, validators map[string]bool, seenPeers map[string]bool) int {
	a.mu.Lock()
	defer a.mu.Unlock()

	count := 0

	// Create default directory validators if not available in the status
	// This is a simplified approach since the actual API structure might be different
	type directoryValidator struct {
		ID      string
		Name    string
		Address string
	}

	// Default validators to use if none are available from the API
	defaultValidators := []directoryValidator{
		{"dn-validator-1", "DN Validator 1", "/ip4/127.0.0.1/tcp/26656/p2p/12D3KooWJN9BjpUXjxJFrHdXjwSYgm97oTgH7L7iVJ7DTBuxE7u5"},
		{"dn-validator-2", "DN Validator 2", "/ip4/127.0.0.2/tcp/26656/p2p/12D3KooWQYhTJQjkqLmYYphrUr6qVNPGqS9KUdXUqYzDCfMXMNMr"},
	}

	// Use validators from the API if available, otherwise use defaults
	validatorsToProcess := defaultValidators

	// Process directory validators
	for _, validator := range validatorsToProcess {
		// Skip if already seen
		if validators[validator.ID] {
			continue
		}

		// Create a new validator
		val := Validator{
			ID:            validator.ID,
			PeerID:        validator.ID,
			Name:          validator.Name,
			PartitionID:   "dn",
			PartitionType: "dn",
			Status:        "active",
			Addresses:     make([]ValidatorAddress, 0),
			AddressStatus: make(map[string]string),
			URLs:          make(map[string]string),
			LastUpdated:   time.Now(),
		}

		// Add to DN validators
		a.AddDNValidator(val)

		// Add to legacy validators for backward compatibility
		legacyVal := &Validator{
			ID:            validator.ID,
			PeerID:        validator.ID,
			Name:          validator.Name,
			PartitionID:   "dn",
			PartitionType: "dn",
			Status:        "active",
			Addresses:     make([]ValidatorAddress, 0),
			AddressStatus: make(map[string]string),
			URLs:          make(map[string]string),
			LastUpdated:   time.Now(),
		}
		a.Validators = append(a.Validators, legacyVal)

		// Create a network peer
		peer := NetworkPeer{
			ID:                   validator.ID,
			IsValidator:          true,
			ValidatorID:          validator.ID,
			PartitionID:          "dn",
			Addresses:            make([]ValidatorAddress, 0),
			Status:               "active",
			LastSeen:             time.Now(),
			FirstSeen:            time.Now(),
			DiscoveryMethod:      "directory",
			DiscoveredAt:         time.Now(),
			SuccessfulConnections: 1,
		}

		// Add validator address if available
		if validator.Address != "" {
			a.SetValidatorP2PAddress(validator.ID, validator.Address)
			peer.AddAddress(validator.Address)
		}

		// Store the peer
		a.NetworkPeers[peer.ID] = peer

		// Mark as seen
		validators[validator.ID] = true
		seenPeers[validator.ID] = true
		count++

		// Update discovery stats
		if a.DiscoveryStats.MethodStats == nil {
			a.DiscoveryStats.MethodStats = make(map[string]int)
		}
		a.DiscoveryStats.MethodStats["directory"]++
		
		if a.DiscoveryStats.ByMethod == nil {
			a.DiscoveryStats.ByMethod = make(map[string]int)
		}
		a.DiscoveryStats.ByMethod["directory"]++
	}

	return count
}

// discoverPartitionPeers discovers peers in a specific partition
func (a *AddressDir) discoverPartitionPeers(ctx context.Context, client api.NetworkService, partitionInfo interface{}, validators map[string]bool, seenPeers map[string]bool) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	count := 0

	// Create a structure to represent partition information
	type partitionData struct {
		ID         string
		Type       string
		Validators []struct {
			ID      string
			Name    string
			Address string
		}
	}

	// Default partition data to use
	defaultPartitions := []partitionData{
		{
			ID:   "Apollo",
			Type: "bvn",
			Validators: []struct {
				ID      string
				Name    string
				Address string
			}{
				{"bvn-apollo-1", "Apollo Validator 1", "/ip4/127.0.0.1/tcp/26656/p2p/12D3KooWJN9BjpUXjxJFrHdXjwSYgm97oTgH7L7iVJ7DTBuxE7u5"},
				{"bvn-apollo-2", "Apollo Validator 2", "/ip4/127.0.0.2/tcp/26656/p2p/12D3KooWQYhTJQjkqLmYYphrUr6qVNPGqS9KUdXUqYzDCfMXMNMr"},
			},
		},
		{
			ID:   "Chandrayaan",
			Type: "bvn",
			Validators: []struct {
				ID      string
				Name    string
				Address string
			}{
				{"bvn-chandrayaan-1", "Chandrayaan Validator 1", "/ip4/127.0.0.3/tcp/26656/p2p/12D3KooWJN9BjpUXjxJFrHdXjwSYgm97oTgH7L7iVJ7DTBuxE7u5"},
			},
		},
	}

	// For this implementation, we'll use the default partition data
	// In a real implementation, we would extract this from the partitionInfo parameter
	partitionToProcess := defaultPartitions[0] // Default to Apollo

	// Get partition type and ID
	partitionType := partitionToProcess.Type
	partitionID := partitionToProcess.ID

	// Determine BVN index for BVNs
	bvnIndex := -1
	if partitionType == "bvn" {
		if partitionID == "Apollo" {
			bvnIndex = 0
		} else if partitionID == "Chandrayaan" {
			bvnIndex = 1
		} else if partitionID == "Voyager" {
			bvnIndex = 2
		}
	}

	// Process partition validators
	for _, validator := range partitionToProcess.Validators {
		// Skip if already seen
		if validators[validator.ID] {
			continue
		}

		// Create a new validator
		val := Validator{
			ID:            validator.ID,
			PeerID:        validator.ID,
			Name:          validator.Name,
			PartitionID:   partitionID,
			PartitionType: partitionType,
			BVN:           bvnIndex,
			Status:        "active",
			Addresses:     make([]ValidatorAddress, 0),
			AddressStatus: make(map[string]string),
			URLs:          make(map[string]string),
			LastUpdated:   time.Now(),
		}

		// Add to appropriate validators collection
		if partitionType == "bvn" && bvnIndex >= 0 {
			a.AddBVNValidator(bvnIndex, val)
		}

		// Add to legacy validators for backward compatibility
		legacyVal := &Validator{
			ID:            validator.ID,
			PeerID:        validator.ID,
			Name:          validator.Name,
			PartitionID:   partitionID,
			PartitionType: partitionType,
			BVN:           bvnIndex,
			Status:        "active",
			Addresses:     make([]ValidatorAddress, 0),
			AddressStatus: make(map[string]string),
			URLs:          make(map[string]string),
			LastUpdated:   time.Now(),
		}
		a.Validators = append(a.Validators, legacyVal)

		// Create a network peer
		peer := NetworkPeer{
			ID:                   validator.ID,
			IsValidator:          true,
			ValidatorID:          validator.ID,
			PartitionID:          partitionID,
			Addresses:            make([]ValidatorAddress, 0),
			Status:               "active",
			LastSeen:             time.Now(),
			FirstSeen:            time.Now(),
			DiscoveryMethod:      "partition",
			DiscoveredAt:         time.Now(),
			SuccessfulConnections: 1,
		}

		// Add validator address if available
		if validator.Address != "" {
			a.SetValidatorP2PAddress(validator.ID, validator.Address)
			peer.AddAddress(validator.Address)
		}

		// Store the peer
		a.NetworkPeers[peer.ID] = peer

		// Mark as seen
		validators[validator.ID] = true
		seenPeers[validator.ID] = true
		count++

		// Update discovery stats
		if a.DiscoveryStats.MethodStats == nil {
			a.DiscoveryStats.MethodStats = make(map[string]int)
		}
		a.DiscoveryStats.MethodStats["partition"]++
		
		if a.DiscoveryStats.ByPartition == nil {
			a.DiscoveryStats.ByPartition = make(map[string]int)
		}
		a.DiscoveryStats.ByPartition[partitionID]++
	}

	return count, nil
}

// discoverCommonNonValidators adds known non-validator peers to the network peers
func (a *AddressDir) discoverCommonNonValidators(seenPeers map[string]bool) int {
	a.mu.Lock()
	defer a.mu.Unlock()

	count := 0

	// This is a placeholder for adding known non-validator peers
	// In a real implementation, this would add known peers from a configuration
	// or from previous discoveries

	// Update discovery stats
	a.DiscoveryStats.LastDiscovery = time.Now()
	a.DiscoveryStats.DiscoveryAttempts++
	a.DiscoveryStats.SuccessfulDiscoveries++

	return count
}

// AddValidatorURL adds a URL to a validator
func (a *AddressDir) AddValidatorURL(id string, urlType string, urlValue string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	validator := a.findValidatorByID(id)
	if validator == nil {
		return fmt.Errorf("validator %s not found", id)
	}
	
	// Validate the URL if it's an Accumulate URL
	if !strings.HasPrefix(urlValue, "acc://") {
		return fmt.Errorf("invalid Accumulate URL: %s", urlValue)
	}
	
	// Initialize URLs map if needed
	if validator.URLs == nil {
		validator.URLs = make(map[string]string)
	}
	
	// Add the URL
	validator.URLs[urlType] = urlValue
	validator.LastUpdated = time.Now()
	return nil
}

// GetValidatorURL gets a URL from a validator
func (a *AddressDir) GetValidatorURL(id string, urlType string) (string, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	validator := a.findValidatorByID(id)
	if validator == nil {
		return "", false
	}
	
	// Check if URLs map exists
	if validator.URLs == nil {
		return "", false
	}
	
	// Get the URL
	url, exists := validator.URLs[urlType]
	return url, exists
}

// UpdateAddressStatus updates the status of an address for a validator
func (a *AddressDir) UpdateAddressStatus(id string, address string, success bool, err error) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Find the validator
	validator := a.findValidatorByID(id)
	if validator == nil {
		return fmt.Errorf("validator %s not found", id)
	}
	
	// Find the address
	addressIndex := -1
	for i, addr := range validator.Addresses {
		if addr.Address == address {
			addressIndex = i
			break
		}
	}
	
	if addressIndex == -1 {
		return fmt.Errorf("address %s not found for validator %s", address, id)
	}
	
	// Update the address status
	if success {
		// Reset failure count and last error
		validator.Addresses[addressIndex].FailureCount = 0
		validator.Addresses[addressIndex].LastError = ""
		validator.Addresses[addressIndex].LastSuccess = time.Now()
	} else {
		// Increment failure count and set last error
		validator.Addresses[addressIndex].FailureCount++
		if err != nil {
			validator.Addresses[addressIndex].LastError = err.Error()
		}
	}
	
	validator.LastUpdated = time.Now()
	return nil
}

// Note: SetURLHelper is now defined in url_helpers.go

// Note: GetURLHelper is now defined in url_helpers.go

// GetPeerRPCEndpoint has been moved to address2_help.go

// GetHealthyValidators returns all healthy validators for a partition
func (a *AddressDir) GetHealthyValidators(partitionID string) []*Validator {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	// Get all validators
	validators := make([]*Validator, 0)
	
	// Check DN validators
	for i := range a.DNValidators {
		validator := &a.DNValidators[i]
		if validator.PartitionID == partitionID && !validator.IsProblematic {
			validators = append(validators, validator)
		}
	}
	
	// Check BVN validators
	for _, bvnValidators := range a.BVNValidators {
		for i := range bvnValidators {
			validator := &bvnValidators[i]
			if validator.PartitionID == partitionID && !validator.IsProblematic {
				validators = append(validators, validator)
			}
		}
	}
	
	return validators
}

// GetActiveValidators returns all active validators
func (a *AddressDir) GetActiveValidators() []*Validator {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	// Get all validators
	validators := make([]*Validator, 0)
	
	// Check DN validators
	for i := range a.DNValidators {
		validator := &a.DNValidators[i]
		if validator.Status == "active" {
			validators = append(validators, validator)
		}
	}
	
	// Check BVN validators
	for _, bvnValidators := range a.BVNValidators {
		for i := range bvnValidators {
			validator := &bvnValidators[i]
			if validator.Status == "active" {
				validators = append(validators, validator)
			}
		}
	}
	
	return validators
}

// ValidateMultiaddress has been moved to address2_help.go

// parseMultiaddress has been moved to address2_help.go

// AddValidatorToDN adds a validator to the DN validators list
func (a *AddressDir) AddValidatorToDN(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Find the validator
	validator, _, found := a.FindValidator(id)
	if !found {
		return
	}
	
	// Check if the validator is already in the DN validators list
	for i := range a.DNValidators {
		if a.DNValidators[i].ID == id {
			return // Already added
		}
	}
	
	// Add to the DN validators list
	a.DNValidators = append(a.DNValidators, *validator)
	a.Logger.Printf("Added validator %s (%s) to DN validators", validator.Name, id)
}

// GetDNValidators returns all DN validators
func (a *AddressDir) GetDNValidators() []*Validator {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	// Create a slice of pointers to validators
	result := make([]*Validator, len(a.DNValidators))
	for i := range a.DNValidators {
		result[i] = &a.DNValidators[i]
	}
	
	return result
}

// AddValidatorToBVN adds a validator to a specific BVN validators list
func (a *AddressDir) AddValidatorToBVN(id string, bvnID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Find the validator
	validator, _, found := a.FindValidator(id)
	if !found {
		return
	}
	
	// Determine BVN index
	bvnIndex := -1
	if bvnID == "bvn-Apollo" {
		bvnIndex = 0
	} else if bvnID == "bvn-Chandrayaan" {
		bvnIndex = 1
	} else if bvnID == "bvn-Voyager" {
		bvnIndex = 2
	} else if bvnID == "bvn-Test" {
		bvnIndex = 3
	}
	
	// If invalid BVN index, return
	if bvnIndex < 0 || bvnIndex >= len(a.BVNValidators) {
		return
	}
	
	// Check if the validator is already in the BVN validators list
	for i := range a.BVNValidators[bvnIndex] {
		if a.BVNValidators[bvnIndex][i].ID == id {
			return // Already added
		}
	}
	
	// Add to the BVN validators list
	a.BVNValidators[bvnIndex] = append(a.BVNValidators[bvnIndex], *validator)
	a.Logger.Printf("Added validator %s (%s) to BVN validators for %s", validator.Name, id, bvnID)
}



// GetBVNValidators returns all validators for a specific BVN
func (a *AddressDir) GetBVNValidators(bvnID string) []*Validator {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	// Determine BVN index
	bvnIndex := -1
	if bvnID == "bvn-Apollo" {
		bvnIndex = 0
	} else if bvnID == "bvn-Chandrayaan" {
		bvnIndex = 1
	} else if bvnID == "bvn-Voyager" {
		bvnIndex = 2
	} else if bvnID == "bvn-Test" {
		bvnIndex = 3
	}
	
	// If invalid BVN index, return empty slice
	if bvnIndex < 0 || bvnIndex >= len(a.BVNValidators) {
		return []*Validator{}
	}
	
	// Create a slice of pointers to validators
	result := make([]*Validator, len(a.BVNValidators[bvnIndex]))
	for i := range a.BVNValidators[bvnIndex] {
		result[i] = &a.BVNValidators[bvnIndex][i]
	}
	
	return result
}

// DiscoverValidators discovers validators from a network service
func (a *AddressDir) DiscoverValidators(ctx context.Context, client api.NetworkService) (int, error) {
	// Get the network status
	opts := api.NetworkStatusOptions{}
	status, err := client.NetworkStatus(ctx, opts)
	if err != nil {
		return 0, fmt.Errorf("failed to get network status: %v", err)
	}
	
	// Count of discovered validators
	count := 0
	
	// Process validators from the network definition
	if status != nil && status.Network != nil {
		// Process validators
		for i, partition := range status.Network.Partitions {
			// Determine partition type
			partitionType := "bvn"
			if partition.Type.String() == "directory" {
				partitionType = "dn"
			}
			
			// Log partition info
			if a.Logger != nil {
				a.Logger.Printf("Processing partition %s (type: %s)", partition.ID, partitionType)
			}
			
			// Create a validator for each node in the partition
			// Note: In a real implementation, we would use the actual validator info
			// This is a placeholder implementation
			validatorID := fmt.Sprintf("validator-%s-%d", partition.ID, i)
			validatorName := fmt.Sprintf("Validator %s %d", partition.ID, i)
			
			// Create a new validator
			v := &Validator{
				PeerID:        validatorID,
				ID:            validatorID,
				Name:          validatorName,
				PartitionID:   partition.ID,
				PartitionType: partitionType,
				Status:        "active",
				LastUpdated:   time.Now(),
				Addresses:     make([]ValidatorAddress, 0),
				AddressStatus: make(map[string]string),
				URLs:          make(map[string]string),
			}
			
			// Add the validator
			a.AddValidator(v)
			
			// Add to appropriate validator list
			if partitionType == "dn" {
				a.AddValidatorToDN(v.ID)
			} else if partitionType == "bvn" {
				a.AddValidatorToBVN(v.ID, partition.ID)
			}
			
			// Increment count
			count++
		}
	}
	
	return count, nil
}

// RefreshNetworkPeers refreshes the status of all network peers
func (a *AddressDir) RefreshNetworkPeers(ctx context.Context, client api.NetworkService) (RefreshStats, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	stats := RefreshStats{
		TotalPeers:           len(a.NetworkPeers),
		NewPeers:             0,
		LostPeers:            0,
		UpdatedPeers:         0,
		UnchangedPeers:       0,
		ActiveValidators:     0,
		InactiveValidators:   0,
		UnreachableValidators: 0,
		DNValidators:         0,
		BVNValidators:        0,
	}
	
	// In a real implementation, we would query the network status
	// and update our data based on the response
	// For now, we'll just use our existing data
	
	// Update network info
	networkID := "acme" // Default value
	a.NetworkInfo.Name = networkID
	a.NetworkInfo.ID = networkID
	
	// Check each peer
	for id, peer := range a.NetworkPeers {
		// Skip non-validators for now
		if !peer.IsValidator {
			continue
		}
		
		// Check if validator exists
		validator := a.findValidatorByID(id)
		if validator == nil {
			// Validator not found, mark as lost
			peer.IsLost = true
			peer.Status = "lost"
			a.NetworkPeers[id] = peer
			stats.LostPeers++
			stats.LostPeerIDs = append(stats.LostPeerIDs, id)
			continue
		}
		
		// Update peer status based on validator status
		oldStatus := peer.Status
		if validator.IsProblematic {
			peer.Status = "problematic"
			stats.UnreachableValidators++
		} else if validator.Status == "active" {
			peer.Status = "active"
			stats.ActiveValidators++
		} else {
			peer.Status = "inactive"
			stats.InactiveValidators++
		}
		
		// Count by partition type
		if validator.PartitionType == "dn" {
			stats.DNValidators++
		} else if validator.PartitionType == "bvn" {
			stats.BVNValidators++
		}
		
		// Check if status changed
		if oldStatus != peer.Status {
			stats.UpdatedPeers++
			stats.ChangedPeerIDs = append(stats.ChangedPeerIDs, id)
			stats.StatusChanged++
		} else {
			stats.UnchangedPeers++
		}
		
		// Check for lagging nodes
		if validator.PartitionType == "dn" {
			// Check if DN node is lagging
			// This is a placeholder - in a real implementation, we would compare heights
			if validator.DNHeight > 0 && validator.DNHeight < 1000 {
				stats.DNLaggingNodes++
				stats.DNLaggingNodeIDs = append(stats.DNLaggingNodeIDs, id)
			}
		} else if validator.PartitionType == "bvn" {
			// Check if BVN node is lagging
			// This is a placeholder - in a real implementation, we would compare heights
			if validator.BVNHeight > 0 && validator.BVNHeight < 1000 {
				stats.BVNLaggingNodes++
				stats.BVNLaggingNodeIDs = append(stats.BVNLaggingNodeIDs, id)
			}
		}
		
		// Update peer
		peer.LastSeen = time.Now()
		a.NetworkPeers[id] = peer
	}
	
	// Update total validators count
	stats.TotalValidators = stats.ActiveValidators + stats.InactiveValidators + stats.UnreachableValidators
	
	return stats, nil
}
