// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"time"
)

// EnhancedDiscoveryStats contains detailed statistics about a network discovery operation
type EnhancedDiscoveryStats struct {
	// Total number of validators discovered
	TotalValidators int
	
	// Number of validators by partition
	DNValidators int
	BVNValidators map[string]int
	
	// Total number of non-validators
	TotalNonValidators int
	
	// Total number of peers (validators + non-validators)
	TotalPeers int
	
	// Address type statistics
	AddressTypeStats map[string]AddressTypeStats
	
	// Performance metrics
	AverageResponseTime time.Duration
	SuccessRate float64
	
	// Consensus information
	ConsensusHeight uint64
	LaggingNodes int
	ZombieNodes int
	
	// Version information
	VersionCounts map[string]int
	
	// API v3 connectivity
	APIV3Available int
	APIV3Unavailable int
	
	// New peers discovered in this operation
	NewPeers int
	
	// Peers not found during this operation
	LostPeers int
}

// AddressTypeStats contains statistics for a specific address type
type AddressTypeStats struct {
	// Total addresses of this type
	Total int
	
	// Number of valid addresses
	Valid int
	
	// Number of invalid addresses
	Invalid int
	
	// Average response time
	AverageResponseTime time.Duration
	
	// Success rate
	SuccessRate float64
}

// AddressStats contains statistics about address collection
type AddressStats struct {
	// Total addresses collected by type
	TotalByType map[string]int
	
	// Validation success by type
	ValidByType map[string]int
	
	// Response times by type
	ResponseTimeByType map[string]time.Duration
}

// ValidationResult contains the result of address validation
type ValidationResult struct {
	// Whether validation succeeded
	Success bool
	
	// Components extracted from address
	IP string
	Port string
	PeerID string
	
	// Performance metrics
	ResponseTime time.Duration
	
	// Error information
	Error string
}

// ConsensusStatus contains consensus status information
type ConsensusStatus struct {
	// Whether node is in consensus
	InConsensus bool
	
	// Block heights
	DNHeight uint64
	BVNHeight uint64
	BVNHeights map[string]uint64
	
	// Whether node is a zombie
	IsZombie bool
	
	// Time since last block
	TimeSinceLastBlock time.Duration
}

// APIv3Status contains API v3 connectivity status
type APIv3Status struct {
	// Whether API v3 is available
	Available bool
	
	// Whether basic connectivity works
	ConnectivityWorks bool
	
	// Whether queries work
	QueriesWork bool
	
	// Response time
	ResponseTime time.Duration
	
	// Error information
	Error string
}
