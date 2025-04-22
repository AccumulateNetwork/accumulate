# AddressDir Design Specification

## Overview
The AddressDir is a comprehensive directory for managing network validators, peers, and addresses in the Accumulate network. It provides functionality for tracking validators across different network types (Directory Network and Block Validator Networks), managing their addresses, and monitoring their status.

## Core Structures

### AddressDir
The main structure that holds all validators and network peers.

```go
type AddressDir struct {
    mu sync.RWMutex
    
    // DNValidators is a list of validators in the Directory Network
    DNValidators []Validator
    
    // BVNValidators is a list of lists of validators in BVNs
    // Index 0: Apollo, Index 1: Chandrayaan, Index 2: Voyager, Index 3: Test
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
```

### Validator
Represents a validator node in the network.

```go
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
```

### ValidatorAddress
Represents a validator's multiaddress with metadata.

```go
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
```

### NetworkPeer
Represents a peer in the network.

```go
type NetworkPeer struct {
    // ID of the peer
    ID string
    
    // Whether this peer is a validator
    IsValidator bool
    
    // If this is a validator, the validator ID
    ValidatorID string
    
    // Partition information
    PartitionID string
    
    // List of addresses for this peer
    Addresses []ValidatorAddress
    
    // Current status of the peer
    Status string
    
    // Last time this peer was seen
    LastSeen time.Time
    
    // First time this peer was seen
    FirstSeen time.Time
    
    // Whether this peer is considered lost/unreachable
    IsLost bool
    
    // URL for the partition this peer belongs to
    PartitionURL string
    
    // How this peer was discovered
    DiscoveryMethod string
    
    // When this peer was discovered
    DiscoveredAt time.Time
    
    // Number of successful connections to this peer
    SuccessfulConnections int
    
    // Number of failed connection attempts to this peer
    FailedConnections int
    
    // Last error encountered when connecting to this peer
    LastError string
    
    // Last error timestamp
    LastErrorTime time.Time
}
```

### NetworkInfo
Contains information about a network and its partitions.

```go
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
```

### PartitionInfo
Contains information about a network partition.

```go
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
```

### DiscoveryStats
Contains statistics about peer discovery.

```go
type DiscoveryStats struct {
    // Statistics by method
    ByMethod map[string]int
    
    // Total number of peers discovered
    TotalPeers int
    
    // Total number of validators discovered
    TotalValidators int
    
    // Total number of non-validator peers discovered
    TotalNonValidators int
    
    // Last discovery time
    LastDiscovery time.Time
}
```

## Required Methods

### Constructor
```go
// NewAddressDir creates a new AddressDir
func NewAddressDir() *AddressDir
```

### Validator Management
```go
// AddValidator adds a validator to the AddressDir
func (a *AddressDir) AddValidator(id string, name string, partitionID string, partitionType string) *Validator

// AddValidator adds a validator to the AddressDir (overloaded version)
func (a *AddressDir) AddValidator(validator *Validator) *Validator

// GetValidator gets a validator by ID
func (a *AddressDir) GetValidator(id string) *Validator

// FindValidator finds a validator by ID
func (a *AddressDir) FindValidator(id string) (*Validator, int, bool)

// FindValidatorsByPartition finds validators by partition ID
func (a *AddressDir) FindValidatorsByPartition(partitionID string) []*Validator

// AddValidatorToDN adds a validator to the DN validators list
func (a *AddressDir) AddValidatorToDN(id string)

// GetDNValidators returns all DN validators
func (a *AddressDir) GetDNValidators() []*Validator

// AddValidatorToBVN adds a validator to a specific BVN validators list
func (a *AddressDir) AddValidatorToBVN(id string, bvnID string)

// GetBVNValidators returns all validators for a specific BVN
func (a *AddressDir) GetBVNValidators(bvnID string) []*Validator

// GetHealthyValidators returns all healthy validators for a partition
func (a *AddressDir) GetHealthyValidators(partitionID string) []*Validator

// GetActiveValidators returns all active validators
func (a *AddressDir) GetActiveValidators() []*Validator

// SetValidatorStatus sets the status of a validator
func (a *AddressDir) SetValidatorStatus(id string, status string) error
```

### Address Management
```go
// AddValidatorAddress adds an address to a validator
func (a *AddressDir) AddValidatorAddress(id string, address string) error

// GetValidatorAddresses gets all addresses for a validator
func (a *AddressDir) GetValidatorAddresses(id string) ([]ValidatorAddress, bool)

// MarkAddressPreferred marks an address as preferred for a validator
func (a *AddressDir) MarkAddressPreferred(id string, address string) error

// GetPreferredAddresses gets all preferred addresses for a validator
func (a *AddressDir) GetPreferredAddresses(id string) []ValidatorAddress

// UpdateAddressStatus updates the status of an address for a validator
func (a *AddressDir) UpdateAddressStatus(id string, address string, success bool, err error) error

// GetKnownAddressesForValidator returns all known addresses for a validator
func (a *AddressDir) GetKnownAddressesForValidator(id string) []string
```

### URL Management
```go
// AddValidatorURL adds a URL to a validator
func (a *AddressDir) AddValidatorURL(id string, urlType string, urlValue string) error

// GetValidatorURL gets a URL from a validator
func (a *AddressDir) GetValidatorURL(id string, urlType string) (string, bool)

// SetURLHelper sets a URL helper value
func (a *AddressDir) SetURLHelper(key string, value string)

// GetURLHelper gets a URL helper value
func (a *AddressDir) GetURLHelper(key string) (string, bool)
```

### Problem Node Management
```go
// MarkNodeProblematic marks a node as problematic with a reason
func (a *AddressDir) MarkNodeProblematic(id string, reason string, requestTypes ...[]string) bool

// MarkValidatorProblematic marks a validator as problematic with a reason (alias for MarkNodeProblematic)
func (a *AddressDir) MarkValidatorProblematic(id string, reason string) bool

// IsNodeProblematic checks if a node is marked as problematic
func (a *AddressDir) IsNodeProblematic(id string, requestType ...string) bool

// GetProblemNodes returns all nodes marked as problematic
func (a *AddressDir) GetProblemNodes() []*Validator

// ClearProblematicStatus clears the problematic status for a validator
func (a *AddressDir) ClearProblematicStatus(id string) bool

// AddRequestTypeToAvoid adds a request type to avoid for a validator
func (a *AddressDir) AddRequestTypeToAvoid(id string, requestType string) bool

// ShouldAvoidForRequestType checks if a validator should be avoided for a request type
func (a *AddressDir) ShouldAvoidForRequestType(id string, requestType string) bool
```

### Network Peer Management
```go
// NetworkPeer represents a peer in the network with performance metrics
type NetworkPeer struct {
    // Basic identification
    ID              string
    IsValidator     bool
    ValidatorID     string
    PartitionID     string
    
    // Addresses and connectivity
    Addresses       []ValidatorAddress
    Status          string
    
    // Performance metrics
    ResponseTime    time.Duration
    SuccessRate     float64
    LastSuccess     time.Time
    FailureCount    int
    LastError       string
    LastErrorTime   time.Time
    
    // History tracking
    LastSeen        time.Time
    FirstSeen       time.Time
    DiscoveryMethod string
    DiscoveredAt    time.Time
    
    // Performance scoring
    PerformanceScore float64
    IsPreferred      bool
}

// AddNetworkPeer adds a network peer with basic information
func (a *AddressDir) AddNetworkPeer(id string, isValidator bool, validatorID string, partitionID string) *NetworkPeer

// AddNetworkPeerWithAddress adds a network peer with an initial address
func (a *AddressDir) AddNetworkPeerWithAddress(id string, isValidator bool, validatorID string, partitionID string, address string) *NetworkPeer

// GetNetworkPeer gets a network peer by ID
func (a *AddressDir) GetNetworkPeer(id string) (*NetworkPeer, bool)

// GetBestNetworkPeer gets the best performing network peer based on performance metrics
func (a *AddressDir) GetBestNetworkPeer() *NetworkPeer

// GetBestNetworkPeerForPartition gets the best performing network peer for a specific partition
func (a *AddressDir) GetBestNetworkPeerForPartition(partitionID string) *NetworkPeer

// GetNetworkPeers gets all network peers as a slice of values
func (a *AddressDir) GetNetworkPeers() []NetworkPeer

// GetValidatorPeers gets all peers that are validators
func (a *AddressDir) GetValidatorPeers() []NetworkPeer

// GetNonValidatorPeers gets all peers that are not validators
func (a *AddressDir) GetNonValidatorPeers() []NetworkPeer

// AddPeerAddress adds an address to a peer
func (a *AddressDir) AddPeerAddress(peerID string, address string) error

// UpdatePeerPerformance updates performance metrics for a peer
func (a *AddressDir) UpdatePeerPerformance(peerID string, responseTime time.Duration, success bool, errorMsg string) error

// MarkPeerPreferred marks a peer as preferred for future operations
func (a *AddressDir) MarkPeerPreferred(peerID string) error

// GetPreferredPeers gets all peers marked as preferred
func (a *AddressDir) GetPreferredPeers() []NetworkPeer

// RemoveNetworkPeer removes a network peer
func (a *AddressDir) RemoveNetworkPeer(id string) bool

// UpdateNetworkPeerStatus updates the status of a network peer
func (a *AddressDir) UpdateNetworkPeerStatus(id string, status string) bool
```

### Network Discovery
```go
// RefreshNetworkPeers refreshes the status of all network peers
func (a *AddressDir) RefreshNetworkPeers(ctx context.Context, client api.NetworkService) (RefreshStats, error)

// DiscoverPeers discovers peers in the network
func (a *AddressDir) DiscoverPeers(ctx context.Context, client api.NetworkService) (int, error)

// DiscoverValidators discovers validators from a network service
func (a *AddressDir) DiscoverValidators(ctx context.Context, client api.NetworkService) (int, error)
```

### Helper Methods
```go
// parseMultiaddress parses a multiaddress string and extracts IP, port, and peer ID
func (a *AddressDir) parseMultiaddress(address string) (ip string, port string, peerID string, err error)

// ValidateMultiaddress validates a multiaddress string
func (a *AddressDir) ValidateMultiaddress(address string) (bool, string)

// GetPeerRPCEndpoint gets the RPC endpoint for a peer
func (a *AddressDir) GetPeerRPCEndpoint(peerID string) (string, bool)
```

## Implementation Notes

1. All methods should properly handle concurrency using the mutex.
2. Methods should validate input parameters and return appropriate errors.
3. Helper functions should be moved to address2_help.go.
4. Backward compatibility should be maintained where possible.
5. Proper logging should be implemented for all operations.
6. Error handling should be consistent across all methods.
7. Methods should follow the principle of least surprise.
