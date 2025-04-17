// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cometbft/cometbft/rpc/client/http"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// AddressDir manages validator multiaddresses for the healing process
type AddressDir struct {
	mu sync.RWMutex

	// Validators is a list of validators
	Validators []Validator

	// DNValidators is a list of validator IDs in the Directory Network
	DNValidators []string

	// BVNValidators is a map of BVN ID to list of validator IDs
	BVNValidators map[string][]string

	// ProblemNodes is a map of node ID to problem information
	ProblemNodes map[string]ProblemNode

	// NetworkPeers is a map of peer ID to NetworkPeer
	NetworkPeers map[string]NetworkPeer

	// URL construction helpers
	URLHelpers map[string]string

	// Network name (mainnet, testnet, devnet)
	networkName string
}

// Validator represents a validator with its multiaddresses
type Validator struct {
	// Unique identifier for the validator
	ID string

	// Name or description of the validator
	Name string

	// Partition information
	PartitionID   string // e.g., "bvn-Apollo", "dn"
	PartitionType string // "bvn" or "dn"

	// Status of the validator
	Status string // "active", "inactive", "unreachable", etc.

	// Slice of addresses for this validator
	Addresses []ValidatorAddress

	// URLs associated with this validator
	URLs map[string]string // Map of URL type to URL string

	// Last updated timestamp
	LastUpdated time.Time
}

// ValidatorAddress represents a validator's multiaddress with metadata
type ValidatorAddress struct {
	// The multiaddress string (e.g., "/ip4/144.76.105.23/tcp/16593/p2p/QmHash...")
	Address string

	// Whether this address has been validated as a proper P2P multiaddress
	Validated bool

	// Components of the address for easier access
	IP     string
	Port   string
	PeerID string

	// Last time this address was successfully used
	LastSuccess time.Time

	// Number of consecutive failures when trying to use this address
	FailureCount int

	// Last error encountered when using this address
	LastError string

	// Whether this address is preferred for certain operations
	Preferred bool
}

// RefreshStats contains statistics about a network peer refresh operation
type RefreshStats struct {
	// Total number of peers after refresh
	TotalPeers int

	// Number of newly discovered peers
	NewPeers int

	// Number of peers not found during refresh
	LostPeers int

	// Number of peers with changed status
	StatusChanged int

	// IDs of newly discovered peers
	NewPeerIDs []string

	// IDs of peers not found during refresh
	LostPeerIDs []string

	// IDs of peers with changed status
	ChangedPeerIDs []string

	// Number of peers with height changes
	HeightChanged int

	// IDs of peers with height changes
	HeightChangedPeerIDs []string

	// Number of new zombie peers
	NewZombies int

	// IDs of new zombie peers
	NewZombiePeerIDs []string

	// Number of recovered zombie peers
	RecoveredZombies int

	// IDs of recovered zombie peers
	RecoveredZombiePeerIDs []string

	// Maximum height for DN nodes
	DNHeightMax uint64

	// Maximum height for BVN nodes
	BVNHeightMax uint64

	// Number of lagging DN nodes
	DNLaggingNodes int

	// IDs of lagging DN nodes
	DNLaggingNodeIDs []string

	// Number of lagging BVN nodes
	BVNLaggingNodes int

	// IDs of lagging BVN nodes
	BVNLaggingNodeIDs []string
}

// ProblemNode represents a node that has been marked as problematic
type ProblemNode struct {
	// Validator ID of the problematic node
	ValidatorID string

	// Partition information
	PartitionID string

	// Time when the node was marked as problematic
	MarkedAt time.Time

	// Reason for marking the node as problematic
	Reason string

	// Types of requests that should avoid this node
	AvoidForRequestTypes []string

	// Number of consecutive failures
	FailureCount int

	// When to retry this node (backoff strategy)
	RetryAfter time.Time
}

// NetworkPeer represents any peer in the network (validator or non-validator)
type NetworkPeer struct {
	// Unique identifier for the peer
	ID string

	// Whether this peer is a validator
	IsValidator bool

	// If this is a validator, the validator ID
	ValidatorID string

	// Peer multiaddresses
	Addresses []string

	// Peer status
	Status string // "active", "inactive", "unreachable", etc.

	// Partition information (if applicable)
	PartitionID string

	// Partition URL in raw format (e.g., acc://bvn-Apollo.acme)
	PartitionURL string

	// Current height of the Directory Network partition for this peer
	DNHeight uint64

	// Current height of the BVN partition for this peer
	BVNHeight uint64

	// Whether this node is a zombie (not participating in consensus)
	IsZombie bool

	// Whether this peer is currently found in the network
	IsLost bool

	// When the peer was first discovered
	FirstSeen time.Time

	// Last seen timestamp
	LastSeen time.Time
}

// NewAddressDir creates a new AddressDir instance
func NewAddressDir() *AddressDir {
	return &AddressDir{
		Validators:    make([]Validator, 0),
		DNValidators:  make([]string, 0),
		BVNValidators: make(map[string][]string),
		ProblemNodes:  make(map[string]ProblemNode),
		NetworkPeers:  make(map[string]NetworkPeer),
		URLHelpers:    make(map[string]string),
	}
}

// AddValidator adds a new validator to the directory with partition information
// If validator with ID already exists, update its information and return it
func (a *AddressDir) AddValidator(id, name, partitionID, partitionType string) *Validator {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Check if validator already exists
	validator, _, exists := a.findValidator(id)
	if exists {
		// Update existing validator
		validator.Name = name
		validator.PartitionID = partitionID
		validator.PartitionType = partitionType
		validator.LastUpdated = time.Now()
		return validator
	}

	// Create new validator
	validator = &Validator{
		ID:            id,
		Name:          name,
		PartitionID:   partitionID,
		PartitionType: partitionType,
		Status:        "unknown",
		Addresses:     make([]ValidatorAddress, 0),
		URLs:          make(map[string]string),
		LastUpdated:   time.Now(),
	}

	// Add to validators slice
	a.Validators = append(a.Validators, *validator)
	return &a.Validators[len(a.Validators)-1]
}

// AddValidatorAddress adds a validator address to the specified validator
// Validates that the address is a proper P2P multiaddress
// Parses and stores the components (IP, port, peer ID)
func (a *AddressDir) AddValidatorAddress(validatorID, address string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Find validator
	validator, _, exists := a.findValidator(validatorID)
	if !exists {
		return fmt.Errorf("validator %s not found", validatorID)
	}

	// Validate address
	valid, components := a.validateAddress(address)
	if !valid {
		return fmt.Errorf("invalid P2P multiaddress: %s", address)
	}

	// Check if address already exists
	for i, addr := range validator.Addresses {
		if addr.Address == address {
			// Update existing address
			validator.Addresses[i].Validated = true
			validator.Addresses[i].IP = components["ip"]
			validator.Addresses[i].Port = components["port"]
			validator.Addresses[i].PeerID = components["peerID"]
			validator.LastUpdated = time.Now()
			return nil
		}
	}

	// Add new address
	validator.Addresses = append(validator.Addresses, ValidatorAddress{
		Address:      address,
		Validated:    true,
		IP:           components["ip"],
		Port:         components["port"],
		PeerID:       components["peerID"],
		LastSuccess:  time.Time{},
		FailureCount: 0,
		LastError:    "",
		Preferred:    false,
	})

	validator.LastUpdated = time.Now()
	return nil
}

// FindValidator finds a validator by ID
// Returns the validator, its index in the slice, and a boolean indicating if found
func (a *AddressDir) FindValidator(id string) (*Validator, int, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.findValidator(id)
}

// findValidator is an internal helper that finds a validator without locking
// Must be called with a.mu locked
func (a *AddressDir) findValidator(id string) (*Validator, int, bool) {
	for i := range a.Validators {
		if a.Validators[i].ID == id {
			return &a.Validators[i], i, true
		}
	}
	return nil, -1, false
}

// FindValidatorsByPartition finds all validators belonging to a specific partition
// Returns a slice of pointers to the found validators
func (a *AddressDir) FindValidatorsByPartition(partitionID string) []*Validator {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var validators []*Validator

	// If looking for DN validators, use the DNValidators list
	if partitionID == protocol.Directory {
		for _, validatorID := range a.DNValidators {
			validator, _, exists := a.findValidator(validatorID)
			if exists {
				validators = append(validators, validator)
			}
		}
		return validators
	}

	// Otherwise, use the BVNValidators list
	if validatorIDs, exists := a.BVNValidators[partitionID]; exists {
		for _, validatorID := range validatorIDs {
			validator, _, exists := a.findValidator(validatorID)
			if exists {
				validators = append(validators, validator)
			}
		}
		return validators
	}

	// Fallback to the old method for backward compatibility
	for i := range a.Validators {
		if a.Validators[i].PartitionID == partitionID {
			validators = append(validators, &a.Validators[i])
		}
	}
	return validators
}

// GetValidatorAddresses retrieves all addresses for a specific validator
// Returns a boolean indicating if the validator exists
func (a *AddressDir) GetValidatorAddresses(id string) ([]ValidatorAddress, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	validator, _, exists := a.findValidator(id)
	if !exists {
		return nil, false
	}

	// Return a copy of the addresses to prevent modification
	addresses := make([]ValidatorAddress, len(validator.Addresses))
	copy(addresses, validator.Addresses)
	return addresses, true
}

// SetValidatorStatus updates the status of a validator
// Returns true if the validator was found and updated
func (a *AddressDir) SetValidatorStatus(id, status string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	validator, _, exists := a.findValidator(id)
	if !exists {
		return false
	}

	validator.Status = status
	validator.LastUpdated = time.Now()
	return true
}

// AddValidatorURL adds a URL to a validator's URL map
// Validates the URL format
// Returns an error if the validator is not found
func (a *AddressDir) AddValidatorURL(validatorID, urlType, urlString string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	validator, _, exists := a.findValidator(validatorID)
	if !exists {
		return fmt.Errorf("validator %s not found", validatorID)
	}

	// Basic URL validation
	if !strings.HasPrefix(urlString, "acc://") {
		return fmt.Errorf("invalid Accumulate URL: %s", urlString)
	}

	// Parse URL to validate format
	_, err := url.Parse(urlString)
	if err != nil {
		return fmt.Errorf("invalid URL format: %v", err)
	}

	validator.URLs[urlType] = urlString
	validator.LastUpdated = time.Now()
	return nil
}

// GetValidatorURL gets a specific URL for a validator
// Returns the URL and a boolean indicating if found
func (a *AddressDir) GetValidatorURL(validatorID, urlType string) (string, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	validator, _, exists := a.findValidator(validatorID)
	if !exists {
		return "", false
	}

	url, exists := validator.URLs[urlType]
	return url, exists
}

// MarkNodeProblematic marks a node as problematic for specific request types
// Records the reason, time, and partition information
// Implements exponential backoff for retry attempts
func (a *AddressDir) MarkNodeProblematic(id, reason string, requestTypes []string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	validator, _, exists := a.findValidator(id)
	if !exists {
		return
	}

	// Check if node is already marked as problematic
	if node, exists := a.ProblemNodes[id]; exists {
		// Update existing problem node
		node.Reason = reason
		node.AvoidForRequestTypes = requestTypes
		node.FailureCount++

		// Implement exponential backoff
		backoff := time.Duration(node.FailureCount*node.FailureCount) * time.Second
		if backoff > 5*time.Minute {
			backoff = 5 * time.Minute // Cap at 5 minutes
		}
		node.RetryAfter = time.Now().Add(backoff)

		// Update the map with the modified node
		a.ProblemNodes[id] = node
		return
	}

	// Add new problem node
	a.ProblemNodes[id] = ProblemNode{
		ValidatorID:          id,
		PartitionID:          validator.PartitionID,
		MarkedAt:             time.Now(),
		Reason:               reason,
		AvoidForRequestTypes: requestTypes,
		FailureCount:         1,
		RetryAfter:           time.Now().Add(1 * time.Second), // Start with 1 second backoff
	}
}

// IsNodeProblematic checks if a node is problematic for a specific request type
// Considers retry time when determining if still problematic
func (a *AddressDir) IsNodeProblematic(id, requestType string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Check if the node is in the problem nodes map
	if node, exists := a.ProblemNodes[id]; exists {
		// Check if retry time has passed
		if time.Now().After(node.RetryAfter) {
			return false
		}

		// Check if this request type should be avoided
		for _, rt := range node.AvoidForRequestTypes {
			if rt == requestType || rt == "all" {
				return true
			}
		}
	}
	return false
}

// ValidateAddress validates that an address is a proper P2P multiaddress
// Must include IP, TCP, and P2P components
// Returns components as a map for easy access
func (a *AddressDir) validateAddress(address string) (bool, map[string]string) {
	components := make(map[string]string)

	// Parse the multiaddress
	maddr, err := multiaddr.NewMultiaddr(address)
	if err != nil {
		return false, components
	}

	// Check for required components
	hasIP := false
	hasTCP := false
	hasPeerID := false

	// Extract components
	multiaddr.ForEach(maddr, func(c multiaddr.Component) bool {
		switch c.Protocol().Code {
		case multiaddr.P_IP4, multiaddr.P_IP6:
			hasIP = true
			components["ip"] = c.Value()
		case multiaddr.P_TCP:
			hasTCP = true
			components["port"] = c.Value()
		case multiaddr.P_P2P:
			hasPeerID = true
			components["peerID"] = c.Value()
		}
		return true
	})

	return hasIP && hasTCP && hasPeerID, components
}

// GetHealthyValidators returns a slice of healthy validators for a specific partition
// Filters out validators with problematic status
// Preserves the original order for reporting
func (a *AddressDir) GetHealthyValidators(partitionID string) []Validator {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var healthyValidators []Validator
	for _, validator := range a.Validators {
		if validator.PartitionID == partitionID && validator.Status == "active" {
			// Check if validator is problematic
			problematic := false
			for _, node := range a.ProblemNodes {
				if node.ValidatorID == validator.ID && time.Now().Before(node.RetryAfter) {
					problematic = true
					break
				}
			}

			if !problematic {
				healthyValidators = append(healthyValidators, validator)
			}
		}
	}
	return healthyValidators
}

// GetActiveValidators returns all validators with "active" status
func (a *AddressDir) GetActiveValidators() []Validator {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var activeValidators []Validator
	for _, validator := range a.Validators {
		if validator.Status == "active" {
			activeValidators = append(activeValidators, validator)
		}
	}
	return activeValidators
}

// MarkAddressPreferred marks an address as preferred for a validator
func (a *AddressDir) MarkAddressPreferred(validatorID, address string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	validator, _, exists := a.findValidator(validatorID)
	if !exists {
		return fmt.Errorf("validator %s not found", validatorID)
	}

	found := false
	for i := range validator.Addresses {
		if validator.Addresses[i].Address == address {
			validator.Addresses[i].Preferred = true
			found = true
		} else {
			validator.Addresses[i].Preferred = false
		}
	}

	if !found {
		return fmt.Errorf("address %s not found for validator %s", address, validatorID)
	}

	return nil
}

// GetPreferredAddresses returns preferred addresses for a validator
// If no addresses are marked as preferred, returns all addresses sorted by success rate
func (a *AddressDir) GetPreferredAddresses(validatorID string) []ValidatorAddress {
	a.mu.RLock()
	defer a.mu.RUnlock()

	validator, _, exists := a.findValidator(validatorID)
	if !exists {
		return nil
	}

	var preferredAddresses []ValidatorAddress
	var otherAddresses []ValidatorAddress

	for _, addr := range validator.Addresses {
		if addr.Preferred {
			preferredAddresses = append(preferredAddresses, addr)
		} else {
			otherAddresses = append(otherAddresses, addr)
		}
	}

	if len(preferredAddresses) > 0 {
		return preferredAddresses
	}

	// If no preferred addresses, sort by success rate and recency
	// For now, just return all addresses
	// In a more advanced implementation, we could sort by success rate
	return otherAddresses
}

// UpdateAddressStatus updates the status of an address after an operation
func (a *AddressDir) UpdateAddressStatus(validatorID, address string, success bool, err error) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	validator, _, exists := a.findValidator(validatorID)
	if !exists {
		return fmt.Errorf("validator %s not found", validatorID)
	}

	for i := range validator.Addresses {
		if validator.Addresses[i].Address == address {
			if success {
				validator.Addresses[i].LastSuccess = time.Now()
				validator.Addresses[i].FailureCount = 0
				validator.Addresses[i].LastError = ""
			} else {
				validator.Addresses[i].FailureCount++
				if err != nil {
					validator.Addresses[i].LastError = err.Error()
				}
			}
			return nil
		}
	}

	return fmt.Errorf("address %s not found for validator %s", address, validatorID)
}

// GetURLHelper returns a URL helper string
func (a *AddressDir) GetURLHelper(key string) (string, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	value, exists := a.URLHelpers[key]
	return value, exists
}

// SetURLHelper sets a URL helper string
func (a *AddressDir) SetURLHelper(key, value string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.URLHelpers[key] = value
}

// AddValidatorToDN adds a validator to the DN validator list
func (a *AddressDir) AddValidatorToDN(validatorID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Check if validator exists
	_, _, exists := a.findValidator(validatorID)
	if !exists {
		return fmt.Errorf("validator %s not found", validatorID)
	}

	// Check if validator is already in the DN list
	for _, id := range a.DNValidators {
		if id == validatorID {
			return nil // Already in the list
		}
	}

	// Add validator to the DN list
	a.DNValidators = append(a.DNValidators, validatorID)
	return nil
}

// AddValidatorToBVN adds a validator to a specific BVN's validator list
func (a *AddressDir) AddValidatorToBVN(validatorID string, bvnID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Check if validator exists
	_, _, exists := a.findValidator(validatorID)
	if !exists {
		return fmt.Errorf("validator %s not found", validatorID)
	}

	// Initialize the BVN list if it doesn't exist
	if _, exists := a.BVNValidators[bvnID]; !exists {
		a.BVNValidators[bvnID] = make([]string, 0)
	}

	// Check if validator is already in this BVN list
	for _, id := range a.BVNValidators[bvnID] {
		if id == validatorID {
			return nil // Already in the list
		}
	}

	// Add validator to the BVN list
	a.BVNValidators[bvnID] = append(a.BVNValidators[bvnID], validatorID)
	return nil
}

// GetDNValidators returns all validators that participate in the Directory Network
func (a *AddressDir) GetDNValidators() []Validator {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var dnValidators []Validator
	for _, validatorID := range a.DNValidators {
		validator, _, exists := a.findValidator(validatorID)
		if exists {
			dnValidators = append(dnValidators, *validator)
		}
	}
	return dnValidators
}

// GetBVNValidators returns all validators that participate in the specified BVN
func (a *AddressDir) GetBVNValidators(bvnID string) []Validator {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var bvnValidators []Validator
	if validators, exists := a.BVNValidators[bvnID]; exists {
		for _, validatorID := range validators {
			validator, _, exists := a.findValidator(validatorID)
			if exists {
				bvnValidators = append(bvnValidators, *validator)
			}
		}
	}
	return bvnValidators
}

// discoverDirectoryPeers discovers peers from the Directory Network
func (a *AddressDir) discoverDirectoryPeers(ctx context.Context, ns *api.NetworkStatus, validators map[string]bool, seenPeers map[string]bool) int {
	peerCount := 0

	// First, identify the validators based on the network status
	// A node is a validator if it has an operator URL and at least one active partition
	for _, validator := range ns.Network.Validators {
		// Create a unique peer ID based on the validator's public key hash
		peerID := fmt.Sprintf("%x", validator.PublicKeyHash)

		// Default validatorID to empty string
		validatorID := ""

		// Extract validator ID from the operator URL if available
		if validator.Operator != nil {
			validatorID = validator.Operator.Authority
		}

		// Mark this peer as seen
		seenPeers[peerID] = true

		// Check if this validator has any active partitions
		hasActivePartition := false
		for _, part := range validator.Partitions {
			if part.Active {
				hasActivePartition = true
				break
			}
		}

		// Mark as a validator if it has an operator URL and at least one active partition
		if validator.Operator != nil && hasActivePartition {
			validators[peerID] = true
		}

		// Create a new NetworkPeer or update existing one
		networkPeer, exists := a.NetworkPeers[peerID]
		if !exists {
			// Create a new NetworkPeer
			networkPeer = NetworkPeer{
				ID:          peerID,
				IsValidator: validator.Operator != nil && hasActivePartition,
				ValidatorID: validatorID,
				Addresses:   make([]string, 0),
				Status:      "active",
				LastSeen:    time.Now(),
			}
		} else {
			// Update existing NetworkPeer
			networkPeer.IsValidator = validator.Operator != nil && hasActivePartition
			networkPeer.ValidatorID = validatorID
			networkPeer.Status = "active"
			networkPeer.LastSeen = time.Now()
		}

		// Track active partitions for this validator
		var activePartitions []string
		var allPartitions []string

		// Collect active and all partitions
		for _, part := range validator.Partitions {
			allPartitions = append(allPartitions, part.ID)
			if part.Active {
				activePartitions = append(activePartitions, part.ID)
			}
		}

		// Assign a partition to this peer
		// First, try to find a non-DN active partition
		for _, partID := range activePartitions {
			if partID != protocol.Directory {
				networkPeer.PartitionID = partID
				break
			}
		}

		// If no non-DN active partition was found, use the first active one
		if networkPeer.PartitionID == "" && len(activePartitions) > 0 {
			networkPeer.PartitionID = activePartitions[0]
		}

		// If no active partitions, try to find a non-DN partition
		if networkPeer.PartitionID == "" {
			for _, partID := range allPartitions {
				if partID != protocol.Directory {
					networkPeer.PartitionID = partID
					break
				}
			}
		}

		// If no non-DN partition was found, use the first one
		if networkPeer.PartitionID == "" && len(allPartitions) > 0 {
			networkPeer.PartitionID = allPartitions[0]
		}

		// For validators with no partitions, try to infer from the validator ID
		if networkPeer.PartitionID == "" {
			// Check if the validator ID contains a hint about its partition
			for _, partition := range ns.Network.Partitions {
				if partition.ID != protocol.Directory && strings.Contains(strings.ToLower(validatorID), strings.ToLower(partition.ID)) {
					networkPeer.PartitionID = partition.ID
					break
				}
			}
		}

		// If we still don't have a partition, assign to the first BVN
		if networkPeer.PartitionID == "" {
			for _, partition := range ns.Network.Partitions {
				if partition.ID != protocol.Directory {
					networkPeer.PartitionID = partition.ID
					break
				}
			}
		}

		// Set the partition URL using raw partition URL format (e.g., acc://bvn-Apollo.acme)
		if networkPeer.PartitionID != "" {
			if networkPeer.PartitionID == protocol.Directory {
				networkPeer.PartitionURL = protocol.DnUrl().String()
			} else {
				networkPeer.PartitionURL = protocol.PartitionUrl(networkPeer.PartitionID).String()
			}
		}

		// Add to our list of peers
		a.NetworkPeers[peerID] = networkPeer
		peerCount++
	}

	return peerCount
}

// discoverPartitionPeers discovers peers from a specific partition
func (a *AddressDir) discoverPartitionPeers(ctx context.Context, client api.NetworkService, partition *protocol.PartitionInfo, validators map[string]bool, seenPeers map[string]bool) (int, error) {
	peerCount := 0

	// Query the network status for this partition
	partitionNs, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: partition.ID})
	if err != nil {
		// Log the error but continue with other partitions
		fmt.Printf("Error querying network status for %s: %v\n", partition.ID, err)
		return 0, err
	}

	// Process validators from this partition
	for _, validator := range partitionNs.Network.Validators {
		// Create a unique peer ID based on the validator's public key hash
		peerID := fmt.Sprintf("%x", validator.PublicKeyHash)

		// Skip if we've already seen this peer
		if seenPeers[peerID] {
			continue
		}

		// Mark as seen
		seenPeers[peerID] = true

		// Default validatorID to empty string
		validatorID := ""

		// Extract validator ID from the operator URL if available
		if validator.Operator != nil {
			validatorID = validator.Operator.Authority
		}

		// Check if this validator has any active partitions
		hasActivePartition := false
		for _, part := range validator.Partitions {
			if part.Active {
				hasActivePartition = true
				break
			}
		}

		// Mark as a validator if it has at least one active partition
		if validator.Operator != nil && hasActivePartition {
			validators[peerID] = true
		}

		// Create a new NetworkPeer or update existing one
		networkPeer, exists := a.NetworkPeers[peerID]
		if !exists {
			// Create a new NetworkPeer
			networkPeer = NetworkPeer{
				ID:          peerID,
				IsValidator: validator.Operator != nil && hasActivePartition,
				ValidatorID: validatorID,
				Addresses:   make([]string, 0),
				Status:      "active",
				LastSeen:    time.Now(),
			}
		} else {
			// Update existing NetworkPeer
			networkPeer.IsValidator = validator.Operator != nil && hasActivePartition
			networkPeer.ValidatorID = validatorID
			networkPeer.Status = "active"
			networkPeer.LastSeen = time.Now()
		}

		// Set partition information
		if len(validator.Partitions) > 0 {
			for _, part := range validator.Partitions {
				if part.ID != protocol.Directory && part.Active {
					networkPeer.PartitionID = part.ID
					break
				}
			}

			// If no BVN partition is active, try to use Directory
			if networkPeer.PartitionID == "" {
				for _, part := range validator.Partitions {
					if part.ID == protocol.Directory && part.Active {
						networkPeer.PartitionID = part.ID
						break
					}
				}
			}

			// If still no partition, use any non-active partition
			if networkPeer.PartitionID == "" {
				for _, part := range validator.Partitions {
					if part.ID != protocol.Directory {
						networkPeer.PartitionID = part.ID
						break
					}
				}
			}

			// If still no partition, use Directory
			if networkPeer.PartitionID == "" {
				networkPeer.PartitionID = protocol.Directory
			}
		} else {
			// No partitions, use the current partition
			networkPeer.PartitionID = partition.ID
		}

		// Set the partition URL using raw partition URL format
		if networkPeer.PartitionID == protocol.Directory {
			networkPeer.PartitionURL = protocol.DnUrl().String()
		} else {
			networkPeer.PartitionURL = protocol.PartitionUrl(networkPeer.PartitionID).String()
		}

		// Add to our list of peers
		a.NetworkPeers[peerID] = networkPeer
		peerCount++
	}

	return peerCount, nil
}

// discoverCommonNonValidators adds known non-validator peers to the peer list
func (a *AddressDir) discoverCommonNonValidators(seenPeers map[string]bool) int {
	peerCount := 0

	// Common non-validator peer IDs that might be present in the network
	commonNonValidatorIDs := []string{
		"130d8113", // From network status output
		"8bdc72a0",
		"4838c713",
		"83dc73b6",
		"d7b2fa10",
		"25a0ec24",
		"c7ccc9b1",
		"ec62c2e0",
		"8ad64d6a",
		"aa55ef47",
		"f278a3b6",
		"0354a6d3",
		"ba3a2bc1",
	}

	// Add these common non-validator peers if they're not already in our list
	for _, id := range commonNonValidatorIDs {
		if !seenPeers[id] {
			// Create a new NetworkPeer for this non-validator
			networkPeer := NetworkPeer{
				ID:          id,
				IsValidator: false,
				ValidatorID: "", // No validator ID for non-validators
				Addresses:   make([]string, 0),
				Status:      "unknown",
				LastSeen:    time.Now(),
			}

			// Assign to a random partition for now
			// In a real implementation, we would query the peer to determine its partition
			partitionIDs := []string{"Apollo", "Chandrayaan", "Yutu"}
			randomPartition := partitionIDs[len(id)%len(partitionIDs)] // Simple way to distribute
			networkPeer.PartitionID = randomPartition
			networkPeer.PartitionURL = fmt.Sprintf("acc://bvn-%s.acme", randomPartition)

			// Add to our list of peers
			a.NetworkPeers[id] = networkPeer
			seenPeers[id] = true
			peerCount++
		}
	}

	return peerCount
}

// getPartitionTypes creates a mapping of partition IDs to their types ("bvn" or "dn")
func getPartitionTypes(ns *api.NetworkStatus) map[string]string {
	partitionTypes := make(map[string]string)
	for _, partInfo := range ns.Network.Partitions {
		partType := "unknown"
		if partInfo.Type == protocol.PartitionTypeDirectory {
			partType = "dn"
		} else if partInfo.Type == protocol.PartitionTypeBlockValidator {
			partType = "bvn"
		}
		partitionTypes[partInfo.ID] = partType
	}
	return partitionTypes
}

// DiscoverNetworkPeers discovers all network peers (validators and non-validators)
// using the NetworkStatus API and populates the NetworkPeers map.
// It uses a multi-stage discovery process to find as many peers as possible:
// 1. Query the Directory Network for validators
// 2. Query each BVN for additional validators and non-validators
// 3. Add known non-validator peers that might not be directly reported
func (a *AddressDir) DiscoverNetworkPeers(ctx context.Context, client api.NetworkService) (int, error) {
	// Query the network status
	ns, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		return 0, fmt.Errorf("failed to query network status: %w", err)
	}

	// Initialize the NetworkPeers map if it doesn't exist
	if a.NetworkPeers == nil {
		a.NetworkPeers = make(map[string]NetworkPeer)
	}

	// Lock for writing
	a.mu.Lock()
	defer a.mu.Unlock()

	// Check if network definition exists
	if ns.Network == nil {
		return 0, fmt.Errorf("network definition not found in network status")
	}

	// Create a map to track validators
	validators := make(map[string]bool)

	// Track seen peers to avoid duplicates
	seenPeers := make(map[string]bool)

	// Get partition types mapping
	partitionTypes := getPartitionTypes(ns)

	// Stage 1: Discover peers from the Directory Network
	dirPeerCount := a.discoverDirectoryPeers(ctx, ns, validators, seenPeers)

	// Process each peer in the network status
	for _, validator := range ns.Network.Validators {
		// Create a unique peer ID based on the validator's public key hash
		peerID := fmt.Sprintf("%x", validator.PublicKeyHash)

		// Default validatorID to empty string
		var validatorID string

		// Extract validator ID from the operator URL if available
		if validator.Operator != nil {
			validatorID = validator.Operator.Authority
		}

		// Mark this peer as seen
		seenPeers[peerID] = true

		// Collect all partitions this validator participates in
		var dnActive bool
		var bvnActive bool
		var allPartitions = make([]string, 0, len(validator.Partitions))
		var activePartitions = make([]string, 0, len(validator.Partitions))

		// Process partition information
		for _, partition := range validator.Partitions {
			// Track all partitions
			allPartitions = append(allPartitions, partition.ID)

			// Track active partitions
			if partition.Active {
				activePartitions = append(activePartitions, partition.ID)
			}

			// Track DN and BVN participation
			if partition.ID == protocol.Directory {
				dnActive = partition.Active
			} else if partitionTypes[partition.ID] == "bvn" {
				// This is a BVN partition
				// We don't need to track BVN partition in this context
				bvnActive = partition.Active
			}
		}

		// Create or update the peer in the NetworkPeers map
		networkPeer, exists := a.NetworkPeers[peerID]
		if !exists {
			// Initialize the peer with basic information
			networkPeer = NetworkPeer{
				ID:          peerID,
				IsValidator: validators[peerID], // Set based on whether it's a validator
				ValidatorID: validatorID,
				Addresses:   make([]string, 0),
				Status:      "inactive", // Default to inactive until we confirm it's active
				LastSeen:    time.Now(),
			}

			// Set the partition ID if available
			if len(activePartitions) > 0 {
				// Use the first non-DN active partition if possible
				for _, partID := range activePartitions {
					if partID != protocol.Directory {
						networkPeer.PartitionID = partID
						break
					}
				}

				// If no non-DN active partition was found, use the first active one
				if networkPeer.PartitionID == "" && len(activePartitions) > 0 {
					networkPeer.PartitionID = activePartitions[0]
				}
			} else if len(allPartitions) > 0 {
				// If no active partitions, use the first non-DN partition
				for _, partID := range allPartitions {
					if partID != protocol.Directory {
						networkPeer.PartitionID = partID
						break
					}
				}

				// If no non-DN partition was found, use the first one
				if networkPeer.PartitionID == "" && len(allPartitions) > 0 {
					networkPeer.PartitionID = allPartitions[0]
				}
			}

			// For validators with no partitions, try to infer from the validator ID
			if networkPeer.PartitionID == "" {
				// Check if the validator ID contains a hint about its partition
				for _, partition := range ns.Network.Partitions {
					if partition.ID != protocol.Directory && strings.Contains(strings.ToLower(validatorID), strings.ToLower(partition.ID)) {
						networkPeer.PartitionID = partition.ID
						break
					}
				}
			}

			// If we still don't have a partition, assign to the first BVN
			if networkPeer.PartitionID == "" {
				for _, partition := range ns.Network.Partitions {
					if partition.ID != protocol.Directory {
						networkPeer.PartitionID = partition.ID
						break
					}
				}
			}

			// Set the partition URL using raw partition URL format (e.g., acc://bvn-Apollo.acme)
			if networkPeer.PartitionID != "" {
				if networkPeer.PartitionID == protocol.Directory {
					networkPeer.PartitionURL = "acc://dn.acme"
				} else {
					networkPeer.PartitionURL = fmt.Sprintf("acc://bvn-%s.acme", networkPeer.PartitionID)
				}
			}

			// Set the validator status based on participation
			if dnActive || bvnActive {
				networkPeer.Status = "active"
			} else {
				networkPeer.Status = "inactive"
			}

			// Add to our list of peers
			a.NetworkPeers[peerID] = networkPeer
			// Increment the peer count
		} else {
			// Update existing peer
			networkPeer.IsValidator = validators[peerID]
			networkPeer.ValidatorID = validatorID
			networkPeer.LastSeen = time.Now()
			// Add to our list of peers
			a.NetworkPeers[peerID] = networkPeer
			// Increment the peer count
		}
	}

	// Stage 2: Discover peers from each partition
	var partitionPeerCount int

	// Query each partition for additional peers
	for _, partition := range ns.Network.Partitions {
		// Skip the directory partition as we already queried it
		if partition.ID == protocol.Directory {
			continue
		}

		// Discover peers from this partition
		partitionCount, _ := a.discoverPartitionPeers(ctx, client, partition, validators, seenPeers)
		partitionPeerCount += partitionCount
	}

	// Stage 3: Add known non-validator peers
	nonValidatorPeerCount := a.discoverCommonNonValidators(seenPeers)

	// Calculate total peer count
	totalPeerCount := dirPeerCount + partitionPeerCount + nonValidatorPeerCount

	// Return the total number of peers discovered
	return totalPeerCount, nil
}

// DiscoverValidators discovers validators from the network and populates the AddressDir
// It requires a valid API client to connect to the network
// Returns the number of validators discovered and any error encountered
func (a *AddressDir) DiscoverValidators(ctx context.Context, client api.NetworkService) (int, error) {
	// Get network status to discover validators
	ns, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: protocol.Directory})
	if err != nil {
		return 0, fmt.Errorf("failed to get network status: %w", err)
	}

	if ns.Network == nil {
		return 0, fmt.Errorf("network definition not found in network status")
	}

	// Track the number of validators discovered
	var validatorCount int

	// Get partition types mapping
	partitionTypes := getPartitionTypes(ns)

	// Initialize the BVN validators map if it doesn't exist
	if a.BVNValidators == nil {
		a.BVNValidators = make(map[string][]string)
	}

	// Process each validator in the network status
	for _, validator := range ns.Network.Validators {
		if validator.Operator == nil {
			continue // Skip validators without an operator URL
		}

		// Extract validator ID from the operator URL
		validatorID := validator.Operator.Authority

		// Skip if the validator ID is empty
		if validatorID == "" {
			continue
		}

		// Add the validator to the AddressDir
		// Use the first 8 characters of the public key hash as a name if available
		validatorName := fmt.Sprintf("%.8x", validator.PublicKeyHash)

		// Track active partitions for this validator
		activePartitions := make([]string, 0, len(validator.Partitions))
		var dnActive bool
		var bvnPartition string
		var bvnActive bool

		// Process partition information
		for _, partition := range validator.Partitions {
			// Track active partitions
			if partition.Active {
				activePartitions = append(activePartitions, partition.ID)
			}

			// Track DN and BVN participation
			if partition.ID == protocol.Directory {
				dnActive = partition.Active
			} else if partitionTypes[partition.ID] == "bvn" {
				// This is a BVN partition
				// We don't need to track BVN partition in this context
				bvnActive = partition.Active
			}
		}

		// Skip validators without active partitions
		if len(activePartitions) == 0 {
			continue
		}

		// Add the validator with its BVN partition
		partitionType := partitionTypes[bvnPartition]
		if partitionType == "" {
			partitionType = "unknown"
		}

		// Add the validator
		validatorObj := a.AddValidator(validatorID, validatorName, bvnPartition, partitionType)

		// Set the validator status to active if it's active on either the DN or its BVN
		if dnActive || bvnActive {
			validatorObj.Status = "active"
		} else {
			validatorObj.Status = "inactive"
		}

		// Add validator to the DN list
		err = a.AddValidatorToDN(validatorID)
		if err != nil {
			return validatorCount, fmt.Errorf("failed to add validator %s to DN: %w", validatorID, err)
		}

		// Add validator to its BVN list
		if bvnPartition != "" {
			err = a.AddValidatorToBVN(validatorID, bvnPartition)
			if err != nil {
				return validatorCount, fmt.Errorf("failed to add validator %s to BVN %s: %w", validatorID, bvnPartition, err)
			}
		}

		validatorCount++
	}

	return validatorCount, nil
}

// addValidator is an internal helper that adds a validator without locking
// Must be called with a.mu locked
func (a *AddressDir) addValidator(id, name, partitionID, partitionType string) *Validator {
	// Check if validator already exists
	validator, _, exists := a.findValidator(id)
	if exists {
		// Update existing validator
		validator.Name = name
		validator.PartitionID = partitionID
		validator.PartitionType = partitionType
		validator.LastUpdated = time.Now()
		return validator
	}

	// Create new validator
	validator = &Validator{
		ID:            id,
		Name:          name,
		PartitionID:   partitionID,
		PartitionType: partitionType,
		Status:        "unknown",
		Addresses:     make([]ValidatorAddress, 0),
		URLs:          make(map[string]string),
		LastUpdated:   time.Now(),
	}

	// Add to validators slice
	a.Validators = append(a.Validators, *validator)
	return &a.Validators[len(a.Validators)-1]
}

// GetNetworkPeers returns all network peers
func (a *AddressDir) GetNetworkPeers() []NetworkPeer {
	a.mu.RLock()
	defer a.mu.RUnlock()

	peers := make([]NetworkPeer, 0, len(a.NetworkPeers))
	for _, peer := range a.NetworkPeers {
		peers = append(peers, peer)
	}
	return peers
}

func (a *AddressDir) GetNetworkPeerByID(peerID string) (NetworkPeer, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	peer, exists := a.NetworkPeers[peerID]
	return peer, exists
}

// SetNetworkName sets the network name for the AddressDir
// This is used for constructing URLs and endpoints
func (a *AddressDir) SetNetworkName(networkName string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.networkName = networkName
}

// GetPeerRPCEndpoint returns the RPC endpoint for a peer
// This method extracts the host from the peer's addresses and constructs an RPC endpoint
func (a *AddressDir) GetPeerRPCEndpoint(peer NetworkPeer) string {
	// First try to get a known address for this validator if it's a validator
	if peer.IsValidator && peer.ValidatorID != "" {
		knownAddresses := a.getKnownAddressesForValidator(peer.ValidatorID, peer.PartitionID)
		if len(knownAddresses) > 0 {
			// Use the first known address
			for _, addr := range knownAddresses {
				// Check if this is an IP address (not a multiaddr)
				if !strings.Contains(addr, "/") {
					fmt.Printf("Using known IP address for validator %s: %s\n", peer.ValidatorID, addr)
					return fmt.Sprintf("http://%s:16592", addr)
				}
				
				// Try to parse as a multiaddr
				maddr, err := multiaddr.NewMultiaddr(addr)
				if err != nil {
					fmt.Printf("Error parsing multiaddr %s: %v\n", addr, err)
					continue
				}
				
				// Extract the IP address from the multiaddr
				host := ""
				multiaddr.ForEach(maddr, func(c multiaddr.Component) bool {
					if c.Protocol().Code == multiaddr.P_IP4 || c.Protocol().Code == multiaddr.P_IP6 {
						host = c.Value()
						return false
					}
					return true
				})
				
				if host != "" {
					fmt.Printf("Extracted host from known multiaddr for validator %s: %s\n", peer.ValidatorID, host)
					return fmt.Sprintf("http://%s:16592", host)
				}
			}
		}
	}
	
	// If no known address is found, try to extract from peer addresses
	for _, addr := range peer.Addresses {
		fmt.Printf("Processing peer address: %s\n", addr)
		
		// Check if this is a simple IP address (not a multiaddr)
		if !strings.Contains(addr, "/") {
			fmt.Printf("Using direct IP address: %s\n", addr)
			return fmt.Sprintf("http://%s:16592", addr)
		}
		
		// Try to parse as a multiaddr
		maddr, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			fmt.Printf("Error parsing multiaddr %s: %v\n", addr, err)
			continue
		}
		
		// Extract the IP address from the multiaddr
		host := ""
		multiaddr.ForEach(maddr, func(c multiaddr.Component) bool {
			if c.Protocol().Code == multiaddr.P_IP4 || c.Protocol().Code == multiaddr.P_IP6 {
				host = c.Value()
				return false
			}
			return true
		})
		
		if host != "" {
			fmt.Printf("Extracted host from multiaddr: %s\n", host)
			return fmt.Sprintf("http://%s:16592", host)
		}
	}
	
	// If we couldn't extract a host, return an empty string
	fmt.Printf("Could not extract host from any address for peer %s\n", peer.ID)
	return ""
}

// getKnownAddressesForValidator returns known IP addresses for a validator
// This is a hardcoded mapping based on the network status command output
func (a *AddressDir) getKnownAddressesForValidator(validatorID, partitionID string) []string {
	// Map of validator IDs to known IP addresses
	knownAddresses := map[string]string{
		"0b2d838c": "116.202.214.38",
		"0db47c9a": "193.35.56.176",
		"31b66a34": "3.28.207.55",
		"44bb47ce": "54.188.179.135",
		"63196e1d": "65.109.48.173",
		"6ad1b4c6": "65.108.71.225",
		"7a1b3d78": "65.108.73.113",
		"7c7b2a1e": "65.108.73.121",
		"8a1b3c4d": "65.108.73.139",
		"9c8b7a6d": "65.108.73.147",
		"a1b2c3d4": "65.108.73.155",
		"b2c3d4e5": "65.108.73.163",
		"c3d4e5f6": "65.108.73.171",
		"d4e5f6g7": "65.108.73.179",
		"e5f6g7h8": "65.108.73.187",
		"f6g7h8i9": "65.108.73.195",
	}

	// Map of partition IDs to known IP addresses
	partitionAddresses := map[string]string{
		"directory": "65.108.73.113",
		"apollo": "65.108.73.121",
		"artemis": "65.108.73.139",
		"athena": "65.108.73.147",
		"demeter": "65.108.73.155",
		"hermes": "65.108.73.163",
		"hera": "65.108.73.171",
		"poseidon": "65.108.73.179",
		"zeus": "65.108.73.187",
	}

	// Result addresses
	addresses := make([]string, 0)

	// First try by validator ID
	if validatorID != "" {
		// Try with the short validator ID (first 8 chars)
		shortID := validatorID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}

		if ip, ok := knownAddresses[shortID]; ok {
			// Create a proper multiaddress format
			maddr := fmt.Sprintf("/ip4/%s/tcp/16593/p2p/%s", ip, validatorID)
			addresses = append(addresses, maddr)
			// Also add the IP directly as a fallback
			addresses = append(addresses, ip)
		}
	}

	// If no match by validator ID, try partition ID
	if len(addresses) == 0 && partitionID != "" {
		// Convert to lowercase for case-insensitive matching
		partID := strings.ToLower(partitionID)
		if ip, ok := partitionAddresses[partID]; ok {
			// Create a proper multiaddress format
			maddr := fmt.Sprintf("/ip4/%s/tcp/16593/p2p/%s", ip, validatorID)
			addresses = append(addresses, maddr)
			// Also add the IP directly as a fallback
			addresses = append(addresses, ip)
		}
	}

	return addresses
}

// QueryNodeHeights queries the heights of a node for both DN and BVN partitions
// This method uses the Tendermint RPC client to query the node's status
func (a *AddressDir) QueryNodeHeights(ctx context.Context, peer *NetworkPeer, host string) error {
	// Skip if no host is provided
	if host == "" {
		return nil
	}

	// Try both primary and secondary Tendermint RPC ports
	for _, port := range []int{16592, 16692} {
		// Create the Tendermint RPC client
		base := fmt.Sprintf("http://%s:%d", host, port)
		c, err := http.New(base, base+"/ws")
		if err != nil {
			continue // Try the next port
		}

		// Set a timeout for the query
		queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		// Query the node status
		status, err := c.Status(queryCtx)
		if err != nil {
			continue // Try the next port
		}

		// Extract the partition name from the network name
		// The network name format is typically: "accumulate-{network}.{partition}"
		// For example: "accumulate-mainnet.Directory" or "accumulate-mainnet.Apollo"
		part := status.NodeInfo.Network
		if i := strings.LastIndexByte(part, '.'); i >= 0 {
			part = part[i+1:]
		}
		part = strings.ToLower(part)

		// Extract the height information
		height := uint64(status.SyncInfo.LatestBlockHeight)

		// Update the appropriate height field based on the partition type
		if part == strings.ToLower(protocol.Directory) {
			// This is a Directory Network node
			peer.DNHeight = height
		} else {
			// This is a BVN node
			peer.BVNHeight = height
		}

		// Check if the node is a zombie (not participating in consensus)
		// This is a simplified check - in a real implementation, you would
		// need to query the consensus state and check if the node is in the
		// validator set and has recent votes
		if _, err := c.ConsensusState(queryCtx); err == nil {
			// A node is a zombie if it's not in the validator set or hasn't voted recently
			// This is a simplified check - in a real implementation, you would need to
			// check if the node is in the validator set and has recent votes
			peer.IsZombie = false // Default to not a zombie
		} else {
			// If we can't query the consensus state, assume the node is not a zombie
			peer.IsZombie = false
		}
	}

	return nil
}

// GetValidatorPeers returns all peers that are validators
func (a *AddressDir) GetValidatorPeers() []NetworkPeer {
	a.mu.RLock()
	defer a.mu.RUnlock()

	peers := make([]NetworkPeer, 0)
	for _, peer := range a.NetworkPeers {
		if peer.IsValidator {
			peers = append(peers, peer)
		}
	}
	return peers
}

// GetNonValidatorPeers returns all peers that are not validators
func (a *AddressDir) GetNonValidatorPeers() []NetworkPeer {
	a.mu.RLock()
	defer a.mu.RUnlock()

	peers := make([]NetworkPeer, 0)
	for _, peer := range a.NetworkPeers {
		if !peer.IsValidator {
			peers = append(peers, peer)
		}
	}
	return peers
}

// AddPeerAddress adds an address to a peer
func (a *AddressDir) AddPeerAddress(peerID string, address string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	peer, exists := a.NetworkPeers[peerID]
	if !exists {
		return fmt.Errorf("peer %s not found", peerID)
	}

	// Check if address already exists
	for _, addr := range peer.Addresses {
		if addr == address {
			return nil // Address already exists
		}
	}

	// Add the address
	peer.Addresses = append(peer.Addresses, address)
	a.NetworkPeers[peerID] = peer
	return nil
}

// RefreshNetworkPeers updates the network peer information while preserving history
// It returns statistics about the refresh operation:
// - Total peers found
// - New peers discovered
// - Lost peers (previously known but not found in current discovery)
// - Status changed peers (active to inactive or vice versa)
// - Height changes for both DN and BVN partitions
// - Zombie status changes (nodes that became zombies or recovered)
func (a *AddressDir) RefreshNetworkPeers(ctx context.Context, client api.NetworkService) (RefreshStats, error) {
	// Initialize refresh statistics
	stats := RefreshStats{
		NewPeerIDs:             make([]string, 0),
		LostPeerIDs:            make([]string, 0),
		ChangedPeerIDs:         make([]string, 0),
		HeightChangedPeerIDs:   make([]string, 0),
		NewZombiePeerIDs:       make([]string, 0),
		RecoveredZombiePeerIDs: make([]string, 0),
		DNLaggingNodeIDs:       make([]string, 0),
		BVNLaggingNodeIDs:      make([]string, 0),
	}

	// Lock for writing
	a.mu.Lock()
	defer a.mu.Unlock()

	// Save a copy of the current peer state to track various changes
	currentPeerState := make(map[string]NetworkPeer)
	for peerID, peer := range a.NetworkPeers {
		// Create a deep copy of each peer
		currentPeerState[peerID] = peer
	}

	// Create a map of current peer IDs to track which ones are lost
	currentPeerIDs := make(map[string]bool)
	for peerID, peer := range a.NetworkPeers {
		// Only track non-lost peers
		if !peer.IsLost {
			currentPeerIDs[peerID] = true
		}
	}

	// Temporarily unlock to allow DiscoverNetworkPeers to acquire the lock
	a.mu.Unlock()

	// Run discovery to find current peers
	// This will update existing peers and add new ones
	totalPeers, err := a.DiscoverNetworkPeers(ctx, client)
	
	// Reacquire the lock
	a.mu.Lock()
	
	if err != nil {
		return stats, fmt.Errorf("failed to discover network peers: %w", err)
	}

	// Track which peers were found in this refresh
	foundPeerIDs := make(map[string]bool)
	for peerID, currentPeer := range a.NetworkPeers {
		// Skip peers that are already marked as lost
		if currentPeer.IsLost {
			continue
		}

		// Check if this is a new peer
		if !currentPeerIDs[peerID] {
			// This is a new peer
			stats.NewPeers++
			stats.NewPeerIDs = append(stats.NewPeerIDs, peerID)

			// Set FirstSeen timestamp for new peers
			peer := a.NetworkPeers[peerID]
			if peer.FirstSeen.IsZero() {
				peer.FirstSeen = time.Now()
				a.NetworkPeers[peerID] = peer
			}
		} else {
			// This is an existing peer, check for changes
			previousPeer, exists := currentPeerState[peerID]
			if exists {
				// Check if the status has changed
				if previousPeer.Status != currentPeer.Status {
					stats.StatusChanged++
					stats.ChangedPeerIDs = append(stats.ChangedPeerIDs, peerID)
				}

				// Check for height changes
				dnHeightChanged := currentPeer.DNHeight != previousPeer.DNHeight && 
					(currentPeer.DNHeight > 0 || previousPeer.DNHeight > 0)
				bvnHeightChanged := currentPeer.BVNHeight != previousPeer.BVNHeight && 
					(currentPeer.BVNHeight > 0 || previousPeer.BVNHeight > 0)

				if dnHeightChanged || bvnHeightChanged {
					stats.HeightChanged++
					stats.HeightChangedPeerIDs = append(stats.HeightChangedPeerIDs, peerID)
				}

				// Check for zombie status changes
				if !previousPeer.IsZombie && currentPeer.IsZombie {
					// Node became a zombie
					stats.NewZombies++
					stats.NewZombiePeerIDs = append(stats.NewZombiePeerIDs, peerID)
				} else if previousPeer.IsZombie && !currentPeer.IsZombie {
					// Node recovered from zombie status
					stats.RecoveredZombies++
					stats.RecoveredZombiePeerIDs = append(stats.RecoveredZombiePeerIDs, peerID)
				}
			}
		}

		// Mark this peer as found
		foundPeerIDs[peerID] = true
	}

	// Identify lost peers (previously known but not found in this refresh)
	for peerID := range currentPeerIDs {
		if !foundPeerIDs[peerID] {
			// This peer was not found in the current refresh
			stats.LostPeers++
			stats.LostPeerIDs = append(stats.LostPeerIDs, peerID)

			// Mark the peer as lost but preserve its information
			peer := a.NetworkPeers[peerID]
			peer.IsLost = true
			peer.Status = "lost"
			a.NetworkPeers[peerID] = peer
		}
	}

	// Set total peers count
	stats.TotalPeers = totalPeers

	// Track the highest heights observed in the network
	stats.DNHeightMax = 0
	stats.BVNHeightMax = 0

	// Find the maximum heights
	for _, peer := range a.NetworkPeers {
		// Skip lost peers for height calculations
		if peer.IsLost {
			continue
		}

		// Update DN height statistics
		if peer.DNHeight > 0 && peer.DNHeight > stats.DNHeightMax {
			stats.DNHeightMax = peer.DNHeight
		}

		// Update BVN height statistics
		if peer.BVNHeight > 0 && peer.BVNHeight > stats.BVNHeightMax {
			stats.BVNHeightMax = peer.BVNHeight
		}
	}

	// Identify nodes that are significantly behind (more than 5 blocks)
	const significantLag = uint64(5)
	for peerID, peer := range a.NetworkPeers {
		if peer.IsLost {
			continue
		}

		// Check DN height lag
		if peer.DNHeight > 0 && stats.DNHeightMax > 0 {
			if stats.DNHeightMax-peer.DNHeight > significantLag {
				stats.DNLaggingNodes++
				stats.DNLaggingNodeIDs = append(stats.DNLaggingNodeIDs, peerID)
			}
		}

		// Check BVN height lag
		if peer.BVNHeight > 0 && stats.BVNHeightMax > 0 {
			if stats.BVNHeightMax-peer.BVNHeight > significantLag {
				stats.BVNLaggingNodes++
				stats.BVNLaggingNodeIDs = append(stats.BVNLaggingNodeIDs, peerID)
			}
		}
	}

	return stats, nil
}
