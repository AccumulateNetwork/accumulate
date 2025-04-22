// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package enhanced_discovery

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// AddressDir stores network validators and their addresses
type AddressDir struct {
	// Lock for thread safety
	mu sync.RWMutex
	
	// DNValidators is a list of validators in the Directory Network
	DNValidators []Validator
	
	// BVNValidators is a list of lists of validators in BVNs
	BVNValidators [][]Validator
	
	// NetworkPeers is a map of peer ID to NetworkPeer
	NetworkPeers map[string]NetworkPeer
	
	// URL construction helpers
	URLHelpers map[string]string
	
	// Network information (replaces NetworkName string)
	NetworkInfo *NetworkInfo
	
	// Logger for detailed logging
	Logger *log.Logger
	
	// Statistics for peer discovery
	DiscoveryStats *DiscoveryStats
	
	// Problem nodes by address with timestamp
	ProblemNodes map[string]time.Time
}

// NewAddressDir creates a new address directory
func NewAddressDir() *AddressDir {
	return &AddressDir{
		mu:            sync.RWMutex{},
		DNValidators:  make([]Validator, 0),
		BVNValidators: make([][]Validator, 0),
		NetworkPeers:  make(map[string]NetworkPeer),
		URLHelpers:    make(map[string]string),
		DiscoveryStats: NewDiscoveryStats(),
		ProblemNodes:  make(map[string]time.Time),
		Logger:        nil, // Will be set by caller or defaults to stdout
	}
}

// AddDNValidator adds a validator to the Directory Network
func (a *AddressDir) AddDNValidator(validator *Validator) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Set DN flag
	validator.IsInDN = true
	
	// Check if validator already exists
	for i, v := range a.DNValidators {
		if v.PeerID == validator.PeerID {
			// Update existing validator
			a.DNValidators[i] = *validator
			fmt.Printf("[AddressDir] Updated DN validator: %s (%s)\n", validator.Name, validator.PeerID)
			return
		}
	}
	
	// Add new validator
	a.DNValidators = append(a.DNValidators, *validator)
	fmt.Printf("[AddressDir] Added DN validator: %s (%s)\n", validator.Name, validator.PeerID)
}

// AddBVNValidator adds a validator to a BVN
func (a *AddressDir) AddBVNValidator(bvnIndex int, validator *Validator) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Set BVN index
	validator.BVN = bvnIndex

	// Ensure we have enough BVN slices
	for len(a.BVNValidators) <= bvnIndex {
		a.BVNValidators = append(a.BVNValidators, make([]Validator, 0))
	}

	// Check if validator already exists in this BVN
	for i, v := range a.BVNValidators[bvnIndex] {
		if v.PeerID == validator.PeerID {
			// Update existing validator
			a.BVNValidators[bvnIndex][i] = *validator
			fmt.Printf("[AddressDir] Updated BVN validator: %s (%s) in BVN index %d\n", validator.Name, validator.PeerID, bvnIndex)
			return
		}
	}

	// Add new validator to BVN
	a.BVNValidators[bvnIndex] = append(a.BVNValidators[bvnIndex], *validator)
	fmt.Printf("[AddressDir] Added BVN validator: %s (%s) to BVN index %d\n", validator.Name, validator.PeerID, bvnIndex)
}

// FindValidator finds a validator by name or peer ID
func (a *AddressDir) FindValidator(nameOrPeerID string) (*Validator, string, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Check DN validators
	for i, validator := range a.DNValidators {
		if validator.Name == nameOrPeerID || validator.PeerID == nameOrPeerID {
			return &a.DNValidators[i], "dn", true
		}
	}

	// Check BVN validators
	for bvnIndex, validators := range a.BVNValidators {
		for i, validator := range validators {
			if validator.Name == nameOrPeerID || validator.PeerID == nameOrPeerID {
				// Determine BVN name based on index
				bvnName := "unknown"
				switch bvnIndex {
				case 0:
					bvnName = "Apollo"
				case 1:
					bvnName = "Chandrayaan"
				case 2:
					bvnName = "Voyager"
				default:
					bvnName = fmt.Sprintf("bvn-%d", bvnIndex)
				}
				return &a.BVNValidators[bvnIndex][i], bvnName, true
			}
		}
	}

	return nil, "", false
}

// UpdateValidator updates a validator's information
func (a *AddressDir) UpdateValidator(validator *Validator) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Update last updated timestamp
	validator.LastUpdated = time.Now()

	// Update validator in appropriate partition
	if validator.IsInDN {
		// Find and update DN validator
		for i, v := range a.DNValidators {
			if v.PeerID == validator.PeerID {
				a.DNValidators[i] = *validator
				return
			}
		}
		
		// Not found, add as new validator
		a.DNValidators = append(a.DNValidators, *validator)
	} else if validator.BVN >= 0 && validator.BVN < len(a.BVNValidators) {
		// Find and update BVN validator
		for i, v := range a.BVNValidators[validator.BVN] {
			if v.PeerID == validator.PeerID {
				a.BVNValidators[validator.BVN][i] = *validator
				return
			}
		}
		
		// Not found, add as new validator
		a.BVNValidators[validator.BVN] = append(a.BVNValidators[validator.BVN], *validator)
	} else {
		// Invalid BVN index, try to determine from partition ID
		bvnIndex := -1
		if validator.PartitionID != "" && validator.PartitionID != "dn" {
			if len(validator.PartitionID) > 4 {
				bvnName := validator.PartitionID[4:] // Remove "bvn-" prefix
				switch bvnName {
				case "Apollo":
					bvnIndex = 0
				case "Chandrayaan":
					bvnIndex = 1
				case "Voyager":
					bvnIndex = 2
				}
			}
		}
		
		if bvnIndex >= 0 {
			// Ensure we have enough BVN slices
			for len(a.BVNValidators) <= bvnIndex {
				a.BVNValidators = append(a.BVNValidators, make([]Validator, 0))
			}
			
			// Update BVN index in validator
			validator.BVN = bvnIndex
			
			// Add to BVN validators
			a.BVNValidators[bvnIndex] = append(a.BVNValidators[bvnIndex], *validator)
		} else {
			// Can't determine BVN, add to DN validators
			validator.IsInDN = true
			a.DNValidators = append(a.DNValidators, *validator)
		}
	}
}

// MarkProblemNode marks a node as problematic
func (a *AddressDir) MarkProblemNode(address string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.ProblemNodes[address] = time.Now()
}

// IsProblemNode checks if a node is marked as problematic
func (a *AddressDir) IsProblemNode(address string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	_, ok := a.ProblemNodes[address]
	return ok
}

// ClearProblemNode clears a node's problematic status
func (a *AddressDir) ClearProblemNode(address string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.ProblemNodes, address)
}

// ClearProblemNodesOlderThan clears problem nodes older than the specified duration
func (a *AddressDir) ClearProblemNodesOlderThan(duration time.Duration) int {
	a.mu.Lock()
	defer a.mu.Unlock()

	count := 0
	now := time.Now()
	for address, timestamp := range a.ProblemNodes {
		if now.Sub(timestamp) > duration {
			delete(a.ProblemNodes, address)
			count++
		}
	}

	return count
}

// GetAllValidators returns all validators
func (a *AddressDir) GetAllValidators() []*Validator {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Calculate total capacity
	totalCapacity := len(a.DNValidators)
	for bvnIndex := range a.BVNValidators {
		totalCapacity += len(a.BVNValidators[bvnIndex])
	}

	validators := make([]*Validator, 0, totalCapacity)

	// Add DN validators
	for i := range a.DNValidators {
		validators = append(validators, &a.DNValidators[i])
	}

	// Add BVN validators
	for bvnIndex := range a.BVNValidators {
		for i := range a.BVNValidators[bvnIndex] {
			validators = append(validators, &a.BVNValidators[bvnIndex][i])
		}
	}

	return validators
}

// GetDNValidators returns all DN validators
func (a *AddressDir) GetDNValidators() []*Validator {
	a.mu.RLock()
	defer a.mu.RUnlock()

	validators := make([]*Validator, 0, len(a.DNValidators))
	for i := range a.DNValidators {
		validators = append(validators, &a.DNValidators[i])
	}

	return validators
}

// GetBVNValidators returns all validators for a specific BVN
func (a *AddressDir) GetBVNValidators(bvnName string) []*Validator {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Convert BVN name to index
	bvnIndex := -1
	switch bvnName {
	case "Apollo":
		bvnIndex = 0
	case "Chandrayaan":
		bvnIndex = 1
	case "Voyager":
		bvnIndex = 2
	}

	// Check if we have validators for this BVN
	if bvnIndex < 0 || bvnIndex >= len(a.BVNValidators) {
		return []*Validator{}
	}

	// Return validators for this BVN
	validators := make([]*Validator, 0, len(a.BVNValidators[bvnIndex]))
	for i := range a.BVNValidators[bvnIndex] {
		validators = append(validators, &a.BVNValidators[bvnIndex][i])
	}

	return validators
}

// GetAllBVNValidators returns all BVN validators
func (a *AddressDir) GetAllBVNValidators() []*Validator {
	a.mu.RLock()
	defer a.mu.RUnlock()

	validators := make([]*Validator, 0)
	for bvnIndex := range a.BVNValidators {
		for i := range a.BVNValidators[bvnIndex] {
			validators = append(validators, &a.BVNValidators[bvnIndex][i])
		}
	}

	return validators
}

// GetValidatorCount returns the total number of validators
func (a *AddressDir) GetValidatorCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()

	count := len(a.DNValidators)
	for bvnIndex := range a.BVNValidators {
		count += len(a.BVNValidators[bvnIndex])
	}

	return count
}

// GetProblemNodeCount returns the number of problem nodes
func (a *AddressDir) GetProblemNodeCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return len(a.ProblemNodes)
}

// SetNetwork sets the network information
func (a *AddressDir) SetNetwork(network *NetworkInfo) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.NetworkInfo = network
}

// GetNetwork gets the network information
func (a *AddressDir) GetNetwork() *NetworkInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.NetworkInfo
}

// AddNetworkPeer adds or updates a network peer
func (a *AddressDir) AddNetworkPeer(peer NetworkPeer) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Update last seen timestamp
	peer.LastSeen = time.Now()

	// Add or update peer
	a.NetworkPeers[peer.ID] = peer
}

// GetNetworkPeer gets a network peer by ID
func (a *AddressDir) GetNetworkPeer(id string) (NetworkPeer, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	peer, ok := a.NetworkPeers[id]
	return peer, ok
}

// GetAllNetworkPeers gets all network peers
func (a *AddressDir) GetAllNetworkPeers() []NetworkPeer {
	a.mu.RLock()
	defer a.mu.RUnlock()

	peers := make([]NetworkPeer, 0, len(a.NetworkPeers))
	for _, peer := range a.NetworkPeers {
		peers = append(peers, peer)
	}

	return peers
}

// RemoveNetworkPeer removes a network peer by ID
func (a *AddressDir) RemoveNetworkPeer(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.NetworkPeers, id)
}

// AddURLHelper adds a URL construction helper
func (a *AddressDir) AddURLHelper(key, value string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.URLHelpers[key] = value
}

// GetURLHelper gets a URL construction helper
func (a *AddressDir) GetURLHelper(key string) (string, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	value, ok := a.URLHelpers[key]
	return value, ok
}

// GetStandardizedURL returns a standardized URL for the given partition and type
// This addresses the URL construction differences between sequence.go and heal_anchor.go
func (a *AddressDir) GetStandardizedURL(partitionID, urlType string) string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Get network ID
	networkID := "acme"
	if a.NetworkInfo != nil {
		networkID = a.NetworkInfo.ID
	}

	// Standardize partition ID format
	if partitionID != "dn" && !strings.HasPrefix(partitionID, "bvn-") {
		partitionID = "bvn-" + partitionID
	}

	// Construct URL based on type
	switch urlType {
	case "partition":
		// Use raw partition URLs (sequence.go style)
		return fmt.Sprintf("acc://%s.%s", partitionID, networkID)
		
	case "anchor":
		// Use anchor pool URL with partition ID (heal_anchor.go style)
		if partitionID == "dn" {
			return fmt.Sprintf("acc://dn.%s/anchors", networkID)
		}
		
		// For BVNs, extract the BVN name
		bvnName := partitionID
		if strings.HasPrefix(partitionID, "bvn-") {
			bvnName = partitionID[4:] // Remove "bvn-" prefix
		}
		
		return fmt.Sprintf("acc://dn.%s/anchors/%s", networkID, bvnName)
		
	default:
		// Default to partition URL
		return fmt.Sprintf("acc://%s.%s", partitionID, networkID)
	}
}
