// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package enhanced_discovery

import (
	"time"
)

// DiscoveryStats contains statistics about peer discovery operations
type DiscoveryStats struct {
	// Total number of peers discovered
	TotalPeers int

	// Number of validators discovered
	TotalValidators int

	// Number of validators by partition
	DNValidators  int
	BVNValidators map[string]int

	// Total number of non-validators
	TotalNonValidators int

	// Address type statistics
	AddressTypeStats map[string]AddressTypeStats

	// Discovery attempts and results
	DiscoveryAttempts     int
	SuccessfulDiscoveries int
	FailedDiscoveries     int
	LastDiscovery         time.Time

	// Performance metrics
	AverageResponseTime time.Duration
	SuccessRate         float64

	// Consensus information
	ConsensusHeight uint64
	LaggingNodes    int
	ZombieNodes     int

	// Version information
	VersionCounts map[string]int

	// API v3 connectivity
	APIV3Available   int
	APIV3Unavailable int

	// New peers discovered in this operation
	NewPeers int

	// Peers not found during this operation
	LostPeers int
}

// NewDiscoveryStats creates a new discovery stats instance
func NewDiscoveryStats() *DiscoveryStats {
	return &DiscoveryStats{
		BVNValidators:    make(map[string]int),
		AddressTypeStats: make(map[string]AddressTypeStats),
		VersionCounts:    make(map[string]int),
	}
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
	IP     string
	Port   string
	PeerID string

	// Performance metrics
	ResponseTime time.Duration

	// Error information
	Error string
}

// ConsensusStatus contains consensus status information
type ConsensusStatus struct {
	// Whether the node is in consensus
	InConsensus bool

	// Whether the node is a zombie (not participating in consensus)
	IsZombie bool

	// Current heights
	DNHeight  uint64
	BVNHeight uint64

	// Time since last block
	TimeSinceLastBlock time.Duration

	// Error information
	Error string
}

// APIv3Status contains API v3 connectivity information
type APIv3Status struct {
	// Whether API v3 is available
	Available bool

	// Whether connectivity works
	ConnectivityWorks bool

	// Whether queries work
	QueriesWork bool

	// Response time
	ResponseTime time.Duration

	// Error information
	Error string
}

// NetworkInfo contains information about a network
type NetworkInfo struct {
	// Name of the network (e.g., "mainnet", "testnet")
	Name string

	// ID of the network (e.g., "acme")
	ID string

	// Whether this is the mainnet
	IsMainnet bool

	// API endpoint for this network
	APIEndpoint string

	// List of partitions in this network
	Partitions []*PartitionInfo

	// Map of partition ID to partition info for quick lookup
	PartitionMap map[string]*PartitionInfo
}

// NetworkPeer represents any peer in the network (validator or non-validator)
type NetworkPeer struct {
	// Unique identifier for the peer
	ID string

	// Whether this peer is a validator
	IsValidator bool

	// Validator ID if this peer is a validator
	ValidatorID string

	// Multiaddresses for this peer
	Addresses []string

	// Address types available for this peer
	AddressTypes []string

	// Last time this peer was seen
	LastSeen time.Time

	// Whether this peer is active
	Active bool

	// Version information for this peer
	Version string

	// Last error encountered when connecting to this peer
	LastError string

	// Last response time when connecting to this peer
	LastResponseTime time.Duration

	// Success rate when connecting to this peer
	SuccessRate float64

	// Number of successful connections
	SuccessCount int

	// Number of failed connections
	FailureCount int
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

	// Discovery statistics for this partition
	DiscoveryCount int
	ValidatorCount int
	NonValidatorCount int

	// Success/failure statistics
	SuccessCount int
	FailureCount int

	// Validator mapping statistics
	ValidatorMapAttempts int
	ValidatorMapSuccess int

	// Number of discovery failures
	Failures int

	// Statistics by discovery method
	MethodStats map[string]int
}

// Validator represents a validator with its multiaddresses
type Validator struct {
	// Unique peer identifier for the validator
	PeerID string
	
	// Name or description of the validator
	Name string
	
	// Partition information
	PartitionID   string // e.g., "bvn-Apollo", "dn"
	PartitionType string // "bvn" or "directory"
	
	// BVN index (0, 1, or 2 for mainnet)
	BVN int
	
	// Status of the validator
	Status string // "active", "inactive", "unreachable", etc.
	
	// Different address types for this validator
	P2PAddress     string // Multiaddress format: /ip4/1.2.3.4/tcp/16593/p2p/QmHash...
	IPAddress      string // Plain IP: 1.2.3.4
	RPCAddress     string // RPC endpoint: http://1.2.3.4:26657
	APIAddress     string // API endpoint: http://1.2.3.4:8080
	MetricsAddress string // Metrics endpoint: http://1.2.3.4:9090
	
	// URLs associated with this validator
	URLs map[string]string // Map of URL type to URL string
	
	// Heights for different networks
	DNHeight  uint64
	BVNHeight uint64
	BVNHeights map[string]uint64
	
	// Problem node tracking
	IsProblematic bool
	ProblemReason string
	ProblemSince  time.Time
	
	// Last block height observed for this validator
	LastHeight int64
	
	// Request types to avoid sending to this validator
	AvoidForRequestTypes []string
	
	// Last updated timestamp
	LastUpdated time.Time
	
	// Additional fields for enhanced discovery
	IsInDN      bool
	BVNs        []string
	APIV3Status bool
	Version     string
	IsZombie    bool
}

// NewValidator creates a new validator
func NewValidator(peerID, name, partitionID string) *Validator {
	partitionType := "bvn"
	if partitionID == "dn" {
		partitionType = "directory"
	}

	return &Validator{
		PeerID:        peerID,
		Name:          name,
		PartitionID:   partitionID,
		PartitionType: partitionType,
		Status:        "active",
		LastUpdated:   time.Now(),
		URLs:          make(map[string]string),
		BVNHeights:    make(map[string]uint64),
		AvoidForRequestTypes: make([]string, 0),
		BVNs:          make([]string, 0),
	}
}
