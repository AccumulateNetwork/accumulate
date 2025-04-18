# Design Document for address2.go

## Overview

This document outlines the design for `address2.go`, a replacement for the original `address.go` file in the Accumulate Network codebase. The new implementation will use a modular approach with a master function that coordinates independent helper functions to manage validator addresses and network peers.

## Required Imports

```go
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
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)
```

## Goals

1. Fix the syntax errors present in the original implementation
2. Create a more maintainable and modular codebase
3. Support the functionality required by peer_state.go
4. Integrate with simple_peer_discovery for collecting validator addresses
5. Improve reliability of network operations through robust address management

## Core Data Structures

### AddressDir

The central structure that manages validator multiaddresses:

```go
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
    
    // Network information (replaces NetworkName string)
    NetworkInfo *NetworkInfo
    
    // Logger for detailed logging
    Logger *log.Logger
    
    // Statistics for peer discovery
    DiscoveryStats DiscoveryStats
}
```

### Validator

Represents a validator with its multiaddresses:

```go
type Validator struct {
    // Unique peer identifier for the validator
    PeerID string
    
    // Name or description of the validator
    Name string
    
    // Partition information
    PartitionID   string // e.g., "bvn-Apollo", "dn"
    PartitionType string // "bvn" or "dn"
    
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
    DNHeight uint64
    BVNHeight uint64
    
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
}
```

### ValidatorAddress

Represents a validator's multiaddress with metadata:

```go
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
```

### NetworkPeer

Represents any peer in the network (validator or non-validator):

```go
type NetworkPeer struct {
    // Unique identifier for the peer
    ID string

    // Whether this peer is a validator
    IsValidator bool

    // Validator ID if this peer is a validator
    ValidatorID string

    // Multiaddresses for this peer
    Addresses []string

    // Status of the peer
    Status string

    // Partition information
    PartitionID string
    PartitionURL string

    // Heights for different networks
    DNHeight uint64
    BVNHeight uint64

    // Whether this peer is a zombie (unreachable but not marked as lost)
    IsZombie bool

    // Whether this peer was not found in the latest refresh
    IsLost bool

    // When this peer was first seen
    FirstSeen time.Time

    // When this peer was last seen
    LastSeen time.Time
}
```

### NetworkInfo

Contains information about a network and its partitions:

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
}
```

### PartitionInfo

Contains information about a network partition:

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

Tracks statistics about peer discovery:

```go
type DiscoveryStats struct {
    // Total number of discovery attempts
    TotalAttempts int
    
    // Number of successful multiaddress discoveries
    MultiaddrSuccess int
    
    // Number of successful URL discoveries
    URLSuccess int
    
    // Number of successful validator map discoveries
    ValidatorMapSuccess int
    
    // Number of discovery failures
    Failures int
    
    // Statistics by discovery method
    MethodStats map[string]int
}
```

### RefreshStats

Contains statistics about a network peer refresh operation:

```go
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
```

### ProblemNode

Represents a node that has been marked as problematic:

```go
type ProblemNode struct {
    // Validator ID of the problematic node
    ValidatorID string

    // Partition ID of the problematic node
    PartitionID string

    // When the node was marked as problematic
    MarkedAt time.Time

    // Reason for marking the node as problematic
    Reason string

    // Request types to avoid using this node for
    AvoidForRequestTypes []string

    // Number of failures encountered with this node
    FailureCount int

    // Time after which to retry using this node
    RetryAfter time.Time
}
```

## Master Function

The master function will be responsible for initializing the AddressDir and coordinating the various helper functions:

### NewAddressDir

```go
func NewAddressDir() *AddressDir {
    logger := log.New(os.Stdout, "[AddressDir] ", log.LstdFlags)
    return &AddressDir{
        mu:            sync.RWMutex{},
        DNValidators:  make([]Validator, 0),
        BVNValidators: make([][]Validator, 0),
        NetworkPeers:  make(map[string]NetworkPeer),
        URLHelpers:    make(map[string]string),
        Logger:        logger,
        NetworkInfo: &NetworkInfo{
            Name:        "acme",
            ID:          "acme",
            IsMainnet:   false,
            Partitions:  make([]*PartitionInfo, 0),
            PartitionMap: make(map[string]*PartitionInfo),
            APIEndpoint: "https://mainnet.accumulatenetwork.io/v2",
        },
        DiscoveryStats: DiscoveryStats{
            MethodStats: make(map[string]int),
        },
        Validators:    make([]*Validator, 0), // For backward compatibility
        peerDiscovery: NewSimplePeerDiscovery(logger),
    }
}
```

**Implementation Notes:**
- Creates a logger with standard formatting
- Initializes all slices and maps to empty collections
- Sets default network information for "acme" network
- Initializes discovery statistics
- Creates a simple peer discovery instance

### NewAddressDirWithOptions

```go
func NewAddressDirWithOptions(options AddressDirOptions) *AddressDir {
    dir := NewAddressDir()
    
    if options.Logger != nil {
        dir.Logger = options.Logger
    }
    
    if options.NetworkInfo != nil {
        dir.NetworkInfo = options.NetworkInfo
    }
    
    if options.MaxDiscoveryAttempts > 0 {
        dir.peerDiscovery.SetMaxAttempts(options.MaxDiscoveryAttempts)
    }
    
    return dir
}

type AddressDirOptions struct {
    Logger              *log.Logger
    NetworkInfo         *NetworkInfo
    MaxDiscoveryAttempts int
}
```

**Implementation Notes:**
- Provides customization options for the AddressDir
- Allows setting a custom logger, network info, and discovery parameters
- Uses the default NewAddressDir as a base and applies customizations

## Key Helper Functions

### 1. Validator Management

```go
// AddValidator adds a new validator to the directory with partition information
// If validator with PeerID already exists, update its information and return it
func (a *AddressDir) AddValidator(peerID, name, partitionID, partitionType string) *Validator {
    a.mu.Lock()
    defer a.mu.Unlock()
    
    // Check if validator already exists
    if validator, found := a.FindValidator(peerID); found {
        // Update existing validator
        validator.Name = name
        validator.PartitionID = partitionID
        validator.PartitionType = partitionType
        validator.LastUpdated = time.Now()
        return validator
    }
    
    // Create new validator
    validator := &Validator{
        ID:            peerID, // For backward compatibility
        PeerID:        peerID,
        Name:          name,
        PartitionID:   partitionID,
        PartitionType: partitionType,
        Status:        "unknown",
        Addresses:     make([]string, 0),
        AddressStatus: make(map[string]string),
        URLs:          make(map[string]string),
        LastUpdated:   time.Now(),
    }
    
    // Add to appropriate collections based on partition type
    if partitionType == "dn" {
        a.AddDNValidator(*validator)
    } else if strings.HasPrefix(partitionType, "bvn") {
        // Extract BVN index from partition type or ID
        bvnIndex := 0
        if partitionID == "Apollo" {
            bvnIndex = 0
        } else if partitionID == "Chandrayaan" {
            bvnIndex = 1
        } else if partitionID == "Voyager" {
            bvnIndex = 2
        }
        a.AddBVNValidator(bvnIndex, *validator)
    }
    
    // Add to legacy Validators slice for backward compatibility
    a.Validators = append(a.Validators, validator)
    
    return validator
}

// AddDNValidator adds a validator to the Directory Network
func (a *AddressDir) AddDNValidator(validator Validator) {
    a.mu.Lock()
    defer a.mu.Unlock()
    
    // Check if validator already exists in DNValidators
    for i, v := range a.DNValidators {
        if v.PeerID == validator.PeerID || v.ID == validator.PeerID {
            // Update existing validator
            a.DNValidators[i] = validator
            return
        }
    }
    
    // Add new validator
    validator.PartitionType = "dn"
    a.DNValidators = append(a.DNValidators, validator)
}

// AddBVNValidator adds a validator to a specific BVN
// Sets the validator's BVN field to the specified bvnIndex (0, 1, or 2 for mainnet)
func (a *AddressDir) AddBVNValidator(bvnIndex int, validator Validator) {
    a.mu.Lock()
    defer a.mu.Unlock()
    
    // Ensure BVNValidators has enough capacity
    for len(a.BVNValidators) <= bvnIndex {
        a.BVNValidators = append(a.BVNValidators, make([]Validator, 0))
    }
    
    // Check if validator already exists in this BVN
    for i, v := range a.BVNValidators[bvnIndex] {
        if v.PeerID == validator.PeerID || v.ID == validator.PeerID {
            // Update existing validator
            a.BVNValidators[bvnIndex][i] = validator
            return
        }
    }
    
    // Set BVN index and add to collection
    validator.BVN = bvnIndex
    validator.PartitionType = fmt.Sprintf("bvn-%d", bvnIndex)
    a.BVNValidators[bvnIndex] = append(a.BVNValidators[bvnIndex], validator)
}

// FindValidator finds a validator by PeerID
// Returns the validator and a boolean indicating if found
func (a *AddressDir) FindValidator(peerID string) (*Validator, bool) {
    a.mu.RLock()
    defer a.mu.RUnlock()
    
    // Search in DN validators
    for i := range a.DNValidators {
        if a.DNValidators[i].PeerID == peerID || a.DNValidators[i].ID == peerID {
            return &a.DNValidators[i], true
        }
    }
    
    // Search in BVN validators
    for _, bvnList := range a.BVNValidators {
        for i := range bvnList {
            if bvnList[i].PeerID == peerID || bvnList[i].ID == peerID {
                return &bvnList[i], true
            }
        }
    }
    
    // For backward compatibility, also check the Validators slice
    for _, validator := range a.Validators {
        if validator.ID == peerID || validator.PeerID == peerID {
            return validator, true
        }
    }
    
    return nil, false
}

// FindValidatorsByPartition finds all validators for a specific partition
func (a *AddressDir) FindValidatorsByPartition(partitionID string) []Validator {
    a.mu.RLock()
    defer a.mu.RUnlock()
    
    result := make([]Validator, 0)
    
    // Check if this is the directory network
    if partitionID == "dn" {
        return append(result, a.DNValidators...)
    }
    
    // Search in BVN validators
    for _, bvnList := range a.BVNValidators {
        for _, validator := range bvnList {
            if validator.PartitionID == partitionID {
                result = append(result, validator)
            }
        }
    }
    
    return result
}

// FindValidatorsByBVN finds all validators for a specific BVN index
func (a *AddressDir) FindValidatorsByBVN(bvnIndex int) []Validator {
    a.mu.RLock()
    defer a.mu.RUnlock()
    
    if bvnIndex < 0 || bvnIndex >= len(a.BVNValidators) {
        return make([]Validator, 0)
    }
    
    return append(make([]Validator, 0), a.BVNValidators[bvnIndex]...)
}

// SetValidatorStatus updates the status of a validator
func (a *AddressDir) SetValidatorStatus(peerID, status string) bool {
    validator, found := a.FindValidator(peerID)
    if !found {
        return false
    }
    
    a.mu.Lock()
    defer a.mu.Unlock()
    
    validator.Status = status
    validator.LastUpdated = time.Now()
    return true
}

// UpdateValidatorHeight updates the height of a validator
func (a *AddressDir) UpdateValidatorHeight(peerID string, dnHeight, bvnHeight uint64) bool {
    validator, found := a.FindValidator(peerID)
    if !found {
        return false
    }
    
    a.mu.Lock()
    defer a.mu.Unlock()
    
    validator.DNHeight = dnHeight
    validator.BVNHeight = bvnHeight
    validator.LastUpdated = time.Now()
    return true
}
```

**Implementation Notes:**
- All methods use proper locking to ensure thread safety
- AddValidator acts as a high-level function that delegates to AddDNValidator or AddBVNValidator based on partition type
- FindValidator searches through all validator collections (DN, BVN, and legacy Validators)
- Partition-specific methods allow efficient filtering of validators
- Height tracking methods allow monitoring of validator sync status

// QueryNodeHeights queries the heights of a node for both DN and BVN partitions
// This method uses the Tendermint RPC client to query the node's status
func (a *AddressDir) QueryNodeHeights(ctx context.Context, peer *NetworkPeer, host string) error
```


### 2. Address Management

```go
// SetValidatorP2PAddress sets the P2P address for a validator
func (a *AddressDir) SetValidatorP2PAddress(peerID, address string) bool {
    validator, found := a.FindValidator(peerID)
    if !found {
        return false
    }
    
    a.mu.Lock()
    defer a.mu.Unlock()
    
    validator.P2PAddress = address
    
    // Also add to the Addresses slice for backward compatibility
    addressExists := false
    for _, addr := range validator.Addresses {
        if addr == address {
            addressExists = true
            break
        }
    }
    
    if !addressExists {
        validator.Addresses = append(validator.Addresses, address)
    }
    
    // Set address status
    validator.AddressStatus[address] = "active"
    validator.LastUpdated = time.Now()
    return true
}

// SetValidatorRPCAddress sets the RPC address for a validator
func (a *AddressDir) SetValidatorRPCAddress(peerID, address string) bool {
    validator, found := a.FindValidator(peerID)
    if !found {
        return false
    }
    
    a.mu.Lock()
    defer a.mu.Unlock()
    
    validator.RPCAddress = address
    validator.LastUpdated = time.Now()
    return true
}

// SetValidatorAPIAddress sets the API address for a validator
func (a *AddressDir) SetValidatorAPIAddress(peerID, address string) bool {
    validator, found := a.FindValidator(peerID)
    if !found {
        return false
    }
    
    a.mu.Lock()
    defer a.mu.Unlock()
    
    validator.APIAddress = address
    validator.LastUpdated = time.Now()
    return true
}

// SetValidatorIPAddress sets the IP address for a validator
func (a *AddressDir) SetValidatorIPAddress(peerID, address string) bool {
    validator, found := a.FindValidator(peerID)
    if !found {
        return false
    }
    
    a.mu.Lock()
    defer a.mu.Unlock()
    
    validator.IPAddress = address
    validator.LastUpdated = time.Now()
    return true
}

// SetValidatorMetricsAddress sets the metrics address for a validator
func (a *AddressDir) SetValidatorMetricsAddress(peerID, address string) bool {
    validator, found := a.FindValidator(peerID)
    if !found {
        return false
    }
    
    a.mu.Lock()
    defer a.mu.Unlock()
    
    validator.MetricsAddress = address
    validator.LastUpdated = time.Now()
    return true
}

// GetKnownAddressesForValidator returns all known addresses for a validator
func (a *AddressDir) GetKnownAddressesForValidator(peerID string) map[string]string {
    validator, found := a.FindValidator(peerID)
    if !found {
        return make(map[string]string)
    }
    
    a.mu.RLock()
    defer a.mu.RUnlock()
    
    addresses := make(map[string]string)
    
    if validator.P2PAddress != "" {
        addresses["p2p"] = validator.P2PAddress
    }
    
    if validator.RPCAddress != "" {
        addresses["rpc"] = validator.RPCAddress
    }
    
    if validator.APIAddress != "" {
        addresses["api"] = validator.APIAddress
    }
    
    if validator.IPAddress != "" {
        addresses["ip"] = validator.IPAddress
    }
    
    if validator.MetricsAddress != "" {
        addresses["metrics"] = validator.MetricsAddress
    }
    
    return addresses
}
```

**Implementation Notes:**
- Each address type has a dedicated setter method
- P2P addresses are also added to the legacy Addresses slice for backward compatibility
- All methods use proper locking to ensure thread safety
- GetKnownAddressesForValidator provides a convenient way to access all address types

### 3. Problem Node Management

```go
// MarkNodeProblematic marks a node as problematic with a reason
func (a *AddressDir) MarkNodeProblematic(peerID, reason string) bool {
    validator, found := a.FindValidator(peerID)
    if !found {
        return false
    }
    
    a.mu.Lock()
    defer a.mu.Unlock()
    
    validator.IsProblematic = true
    validator.ProblemReason = reason
    validator.ProblemSince = time.Now()
    validator.LastUpdated = time.Now()
    return true
}

// IsNodeProblematic checks if a node is marked as problematic
func (a *AddressDir) IsNodeProblematic(peerID string) (bool, string) {
    validator, found := a.FindValidator(peerID)
    if !found {
        return false, ""
    }
    
    a.mu.RLock()
    defer a.mu.RUnlock()
    
    return validator.IsProblematic, validator.ProblemReason
}

// GetProblemNodes returns a list of all problematic nodes
func (a *AddressDir) GetProblemNodes() []*ProblemNode {
    a.mu.RLock()
    defer a.mu.RUnlock()
    
    result := make([]*ProblemNode, 0)
    
    // Check DN validators
    for i := range a.DNValidators {
        if a.DNValidators[i].IsProblematic {
            node := &ProblemNode{
                ValidatorID: a.DNValidators[i].PeerID,
                PartitionID: a.DNValidators[i].PartitionID,
                MarkedAt:    a.DNValidators[i].ProblemSince,
                Reason:      a.DNValidators[i].ProblemReason,
                AvoidForRequestTypes: a.DNValidators[i].AvoidForRequestTypes,
            }
            result = append(result, node)
        }
    }
    
    // Check BVN validators
    for _, bvnList := range a.BVNValidators {
        for i := range bvnList {
            if bvnList[i].IsProblematic {
                node := &ProblemNode{
                    ValidatorID: bvnList[i].PeerID,
                    PartitionID: bvnList[i].PartitionID,
                    MarkedAt:    bvnList[i].ProblemSince,
                    Reason:      bvnList[i].ProblemReason,
                    AvoidForRequestTypes: bvnList[i].AvoidForRequestTypes,
                }
                result = append(result, node)
            }
        }
    }
    
    return result
}

// AddRequestTypeToAvoid adds a request type to avoid for a problematic node
func (a *AddressDir) AddRequestTypeToAvoid(peerID, requestType string) bool {
    validator, found := a.FindValidator(peerID)
    if !found {
        return false
    }
    
    a.mu.Lock()
    defer a.mu.Unlock()
    
    // Check if request type is already in the list
    for _, rt := range validator.AvoidForRequestTypes {
        if rt == requestType {
            return true // Already in the list
        }
    }
    
    validator.AvoidForRequestTypes = append(validator.AvoidForRequestTypes, requestType)
    validator.LastUpdated = time.Now()
    return true
}

// ShouldAvoidForRequestType checks if a node should be avoided for a specific request type
func (a *AddressDir) ShouldAvoidForRequestType(peerID, requestType string) bool {
    validator, found := a.FindValidator(peerID)
    if !found {
        return false
    }
    
    a.mu.RLock()
    defer a.mu.RUnlock()
    
    if !validator.IsProblematic {
        return false
    }
    
    // Check if this request type should be avoided
    for _, rt := range validator.AvoidForRequestTypes {
        if rt == requestType || rt == "*" {
            return true
        }
    }
    
    return false
}

// ClearProblematicStatus clears the problematic status of a node
func (a *AddressDir) ClearProblematicStatus(peerID string) bool {
    validator, found := a.FindValidator(peerID)
    if !found {
        return false
    }
    
    a.mu.Lock()
    defer a.mu.Unlock()
    
    validator.IsProblematic = false
    validator.ProblemReason = ""
    validator.AvoidForRequestTypes = make([]string, 0)
    validator.LastUpdated = time.Now()
    return true
}
```

**Implementation Notes:**
- Problem node management allows tracking of problematic validators
- Request type avoidance allows fine-grained control over which operations to avoid on specific nodes
- GetProblemNodes provides a consolidated view of all problematic nodes across partitions
- All methods use proper locking to ensure thread safety

### 4. Network Peer Discovery

```go
// DiscoverNetworkPeers discovers peers in the network using the provided client
// Returns the number of peers discovered and any error encountered
func (a *AddressDir) DiscoverNetworkPeers(ctx context.Context, client api.NetworkService) (int, error) {
    // Get network status from the client
    status, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
    if err != nil {
        return 0, fmt.Errorf("failed to get network status: %w", err)
    }
    
    // Update network information
    a.mu.Lock()
    a.NetworkInfo.Name = status.Network.Name
    a.NetworkInfo.ID = status.Network.ID
    a.NetworkInfo.IsMainnet = status.Network.IsMainnet
    a.NetworkInfo.APIEndpoint = client.BaseURL()
    
    // Update partitions
    a.NetworkInfo.Partitions = make([]*PartitionInfo, 0, len(status.Network.Partitions))
    a.NetworkInfo.PartitionMap = make(map[string]*PartitionInfo)
    
    for _, p := range status.Network.Partitions {
        partition := &PartitionInfo{
            ID:       p.ID,
            Type:     p.Type,
            URL:      a.constructPartitionURL(p.ID),
            Active:   true,
            BVNIndex: -1,
        }
        
        // Set BVN index for BVNs
        if p.Type == "bvn" {
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
    a.mu.Unlock()
    
    // Track validators and seen peers
    validators := make(map[string]bool)
    seenPeers := make(map[string]bool)
    
    // Discover directory peers
    dirPeers := a.discoverDirectoryPeers(ctx, status, validators, seenPeers)
    
    // Discover partition peers
    partitionPeers := 0
    for _, partition := range status.Network.Partitions {
        count, err := a.discoverPartitionPeers(ctx, client, partition, validators, seenPeers)
        if err != nil {
            a.Logger.Printf("Error discovering peers for partition %s: %v", partition.ID, err)
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
    
    // Process directory validators
    for _, validator := range status.Directory.Validators {
        // Skip if already seen
        if validators[validator.ID] {
            continue
        }
        
        // Add validator
        v := a.AddValidator(validator.ID, validator.Name, "dn", "dn")
        
        // Add validator addresses
        if validator.Address != "" {
            a.SetValidatorP2PAddress(validator.ID, validator.Address)
        }
        
        // Mark as seen
        validators[validator.ID] = true
        seenPeers[validator.ID] = true
        count++
        
        // Add to discovery stats
        a.DiscoveryStats.TotalValidators++
        a.DiscoveryStats.MethodStats["directory"]++
    }
    
    return count
}

// discoverPartitionPeers discovers peers in a specific partition
func (a *AddressDir) discoverPartitionPeers(ctx context.Context, client api.NetworkService, partition api.PartitionInfo, validators map[string]bool, seenPeers map[string]bool) (int, error) {
    a.mu.Lock()
    defer a.mu.Unlock()
    
    count := 0
    bvnIndex := -1
    
    // Set BVN index for BVNs
    if partition.Type == "bvn" {
        if partition.ID == "Apollo" {
            bvnIndex = 0
        } else if partition.ID == "Chandrayaan" {
            bvnIndex = 1
        } else if partition.ID == "Voyager" {
            bvnIndex = 2
        }
    }
    
    // Get partition info
    partitionInfo, err := client.PartitionInfo(ctx, api.PartitionInfoOptions{Partition: partition.ID})
    if err != nil {
        return 0, fmt.Errorf("failed to get partition info: %w", err)
    }
    
    // Process validators
    for _, validator := range partitionInfo.Validators {
        // Skip if already seen
        if validators[validator.ID] {
            continue
        }
        
        // Add validator
        v := a.AddValidator(validator.ID, validator.Name, partition.ID, partition.Type)
        
        // Set BVN index for BVN validators
        if bvnIndex >= 0 {
            v.BVN = bvnIndex
        }
        
        // Add validator addresses
        if validator.Address != "" {
            a.SetValidatorP2PAddress(validator.ID, validator.Address)
        }
        
        // Mark as seen
        validators[validator.ID] = true
        seenPeers[validator.ID] = true
        count++
        
        // Add to discovery stats
        a.DiscoveryStats.TotalValidators++
        a.DiscoveryStats.MethodStats["partition"]++
    }
    
    return count, nil
}

// discoverCommonNonValidators adds known non-validator peers
func (a *AddressDir) discoverCommonNonValidators(seenPeers map[string]bool) int {
    a.mu.Lock()
    defer a.mu.Unlock()
    
    count := 0
    
    // Add common non-validator peers (e.g., seed nodes, bootstrap nodes)
    commonPeers := []struct {
        id      string
        address string
    }{
        {"seed-1", "/ip4/1.2.3.4/tcp/26656/p2p/12D3KooWL4Ck5aNMzfk9xM9M8MhTKfQPMVTFQQyKnQkTMGnKj9Td"},
        {"seed-2", "/ip4/5.6.7.8/tcp/26656/p2p/12D3KooWA9Cuph1ULM1QmkXrNiHeFKZFsVMCpHp5XQJBYTUxdXFZ"},
        // Add more common peers as needed
    }
    
    for _, peer := range commonPeers {
        // Skip if already seen
        if seenPeers[peer.id] {
            continue
        }
        
        // Create network peer
        networkPeer := NetworkPeer{
            ID:              peer.id,
            IsValidator:     false,
            Addresses:       []string{peer.address},
            Status:          "active",
            LastSeen:        time.Now(),
            FirstSeen:       time.Now(),
            DiscoveryMethod: "static",
            DiscoveredAt:    time.Now(),
        }
        
        // Add to network peers
        a.NetworkPeers[peer.id] = networkPeer
        seenPeers[peer.id] = true
        count++
        
        // Add to discovery stats
        a.DiscoveryStats.TotalNonValidators++
        a.DiscoveryStats.MethodStats["static"]++
    }
    
    return count
}

// RefreshNetworkPeers refreshes the status of known network peers
// Returns statistics about the refresh operation
func (a *AddressDir) RefreshNetworkPeers(ctx context.Context) RefreshStats {
    a.mu.Lock()
    defer a.mu.Unlock()
    
    stats := RefreshStats{
        TotalPeers:        len(a.NetworkPeers),
        NewPeers:          0,
        LostPeers:         0,
        UpdatedPeers:      0,
        UnchangedPeers:    0,
        TotalValidators:   0,
        TotalNonValidators: 0,
        ActiveValidators:  0,
        InactiveValidators: 0,
        UnreachableValidators: 0,
    }
    
    // Check each peer
    for id, peer := range a.NetworkPeers {
        // Skip non-validators for now
        if !peer.IsValidator {
            stats.TotalNonValidators++
            continue
        }
        
        stats.TotalValidators++
        
        // Check if validator exists
        validator, found := a.FindValidator(id)
        if !found {
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
        
        // Check if status changed
        if oldStatus != peer.Status {
            stats.UpdatedPeers++
            stats.ChangedPeerIDs = append(stats.ChangedPeerIDs, id)
        } else {
            stats.UnchangedPeers++
        }
        
        // Update peer
        peer.LastSeen = time.Now()
        a.NetworkPeers[id] = peer
    }
    
    return stats
}
```

**Implementation Notes:**
- DiscoverNetworkPeers is the main entry point for peer discovery
- It uses helper methods for discovering different types of peers:
  - discoverDirectoryPeers for directory network validators
  - discoverPartitionPeers for partition-specific validators
  - discoverCommonNonValidators for known non-validator peers
- The discovery process updates network information, partitions, and validators
- RefreshNetworkPeers updates the status of known peers and provides statistics
```

### 5. Fixed getKnownAddressesForValidator Function

This function will be implemented correctly to avoid the syntax errors in the original:

```go
// getKnownAddressesForValidator returns known IP addresses for a validator
func (a *AddressDir) getKnownAddressesForValidator(peerID, partitionID string) []string {
    // Map of peer IDs to known IP addresses
    knownAddresses := map[string]string{
        "0b2d838c": "116.202.214.38",
        "0db47c9a": "193.35.56.176",
        // ... other addresses
    }

    // Map of partition IDs to known IP addresses
    partitionAddresses := map[string]string{
        "directory": "65.108.73.113",
        "apollo":    "65.108.73.121",
        // ... other addresses
    }

    // Result addresses
    addresses := make([]string, 0)

    // First try by peer ID
    if peerID != "" {
        // Try with the short peer ID (first 8 chars)
        shortID := peerID
        if len(shortID) > 8 {
            shortID = shortID[:8]
        }

        if ip, ok := knownAddresses[shortID]; ok {
            // Create a proper multiaddress format
            maddr := fmt.Sprintf("/ip4/%s/tcp/16593/p2p/%s", ip, peerID)
            addresses = append(addresses, maddr)
            // Also add the IP directly as a fallback
            addresses = append(addresses, ip)
        }
    }

    // If no match by peer ID, try partition ID
    if len(addresses) == 0 && partitionID != "" {
        // Convert to lowercase for case-insensitive matching
        partID := strings.ToLower(partitionID)
        if ip, ok := partitionAddresses[partID]; ok {
            // Create a proper multiaddress format if peerID is provided
            if peerID != "" {
                maddr := fmt.Sprintf("/ip4/%s/tcp/16593/p2p/%s", ip, peerID)
                addresses = append(addresses, maddr)
            }
            // Also add the IP directly as a fallback
            addresses = append(addresses, ip)
        }
    }

    return addresses
}
```

## Integration with simple_peer_discovery

The `DiscoverNetworkPeers` function will use simple_peer_discovery to collect validator addresses:

```go
func (a *AddressDir) DiscoverNetworkPeers(ctx context.Context, client api.NetworkService) (int, error) {
    // Use simple_peer_discovery to get network status and peers
    ns, err := client.NetworkStatus(ctx, &api.NetworkStatusRequest{})
    if err != nil {
        return 0, fmt.Errorf("failed to get network status: %w", err)
    }

    // Track validators and seen peers
    validators := make(map[string]bool)
    seenPeers := make(map[string]bool)

    // Discover directory peers
    dirPeers := a.discoverDirectoryPeers(ctx, ns, validators, seenPeers)

    // Discover partition peers
    partitionPeers := 0
    for _, partition := range ns.Network.Partitions {
        count, err := a.discoverPartitionPeers(ctx, client, partition, validators, seenPeers)
        if err != nil {
            a.logger.Printf("Error discovering peers for partition %s: %v", partition.ID, err)
            continue
        }
        partitionPeers += count
    }

    // Add known non-validator peers
    nonValidatorPeers := a.discoverCommonNonValidators(seenPeers)

    // Return total number of peers
    return dirPeers + partitionPeers + nonValidatorPeers, nil
}
```

## Support for peer_state.go

To support peer_state.go, the following functions will be implemented with priority:

1. `GetValidatorPeers()`
2. `GetNonValidatorPeers()`
3. `RefreshNetworkPeers()`
4. `AddPeerAddress()`
5. `GetPeerRPCEndpoint()`

These functions are essential for peer_state.go to function correctly and will be implemented first.

## Note on Network Operations

It's important to note that healing and monitoring of Accumulate simply can't be improved with caching. The blockchain's state is constantly changing, and each query needs to fetch the current state to ensure accuracy. Instead, the focus should be on:

1. **Reliable Node Selection**: The `IsProblematic` and `AvoidForRequestTypes` fields in the Validator structure allow operations to avoid problematic nodes for specific request types.

2. **Consistent URL Construction**: The URL construction standardization functions ensure consistent URL formats across the codebase, preventing errors due to format discrepancies.

3. **Accurate Address Management**: The address management functions provide up-to-date information about validator addresses, helping direct requests to the most reliable nodes.

By focusing on these aspects, address2.go will improve the reliability and efficiency of network operations without relying on caching mechanisms.

## Implementation Plan to Align with Design

### Phase 1: Structural Preparation ✅

1. **Move Helper Functions to address2_help.go** ✅
   - ✅ Created address2_help.go file
   - ✅ Moved the following helper functions with updates to work with the new structure:
     - constructPartitionURL
     - constructAnchorURL
     - findValidatorByID (updated to search in DNValidators and BVNValidators)
     - ValidateMultiaddress
     - parseMultiaddress
     - GetPeerRPCEndpoint
   - ✅ Created comprehensive tests in address2_help_test.go

2. **Update AddressDir Structure** ✅
   - ✅ Modified the AddressDir struct to include:
     ```go
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
         
         // Network information
         NetworkInfo *NetworkInfo
         
         // Logger for detailed logging
         Logger *log.Logger
         
         // Statistics for peer discovery
         DiscoveryStats DiscoveryStats
         
         // Keep Validators for backward compatibility during transition
         Validators []*Validator
         
         // Keep peerDiscovery for implementation needs
         peerDiscovery *SimplePeerDiscovery
     }
     ```

3. **Update Validator Structure** ✅
   - ✅ Modified the Validator struct to include all fields from the design:
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
         
         // Last updated timestamp
         LastUpdated time.Time `json:"last_updated"`
         
         // Keep ID for backward compatibility
         ID string `json:"id"`
         
         // Keep Addresses and AddressStatus for compatibility
         Addresses     []string         `json:"addresses"`
         AddressStatus map[string]string `json:"address_status"`
     }
     ```

### Phase 2: Method Implementation ⏳

1. **Update NewAddressDir and NewTestAddressDir** ⏳
   - Implement NewAddressDir according to the design:
     ```go
     func NewAddressDir() *AddressDir {
         logger := log.New(os.Stdout, "[AddressDir] ", log.LstdFlags)
         return &AddressDir{
             mu:            sync.RWMutex{},
             DNValidators:  make([]Validator, 0),
             BVNValidators: make([][]Validator, 0),
             NetworkPeers:  make(map[string]NetworkPeer),
             URLHelpers:    make(map[string]string),
             Logger:        logger,
             NetworkInfo: &NetworkInfo{
                 Name:        "acme",
                 ID:          "acme",
                 IsMainnet:   false,
                 Partitions:  make([]*PartitionInfo, 0),
                 PartitionMap: make(map[string]*PartitionInfo),
                 APIEndpoint: "https://mainnet.accumulatenetwork.io/v2",
             },
             DiscoveryStats: DiscoveryStats{
                 MethodStats: make(map[string]int),
             },
             Validators:    make([]*Validator, 0), // For backward compatibility
             peerDiscovery: NewSimplePeerDiscovery(logger),
         }
     }
     ```
   - Add NewAddressDirWithOptions for customization options

2. **Implement Validator Management Methods** ⏳
   - Implement AddValidator to work with both DNValidators and BVNValidators:
     ```go
     func (a *AddressDir) AddValidator(peerID, name, partitionID, partitionType string) *Validator {
         a.mu.Lock()
         defer a.mu.Unlock()
         
         // Check if validator already exists
         if validator, found := a.FindValidator(peerID); found {
             // Update existing validator
             validator.Name = name
             validator.PartitionID = partitionID
             validator.PartitionType = partitionType
             validator.LastUpdated = time.Now()
             return validator
         }
         
         // Create new validator
         validator := &Validator{
             ID:            peerID, // For backward compatibility
             PeerID:        peerID,
             Name:          name,
             PartitionID:   partitionID,
             PartitionType: partitionType,
             Status:        "unknown",
             Addresses:     make([]string, 0),
             AddressStatus: make(map[string]string),
             URLs:          make(map[string]string),
             LastUpdated:   time.Now(),
         }
         
         // Add to appropriate collections based on partition type
         if partitionType == "dn" {
             a.AddDNValidator(*validator)
         } else if strings.HasPrefix(partitionType, "bvn") {
             // Extract BVN index from partition type or ID
             bvnIndex := 0
             if partitionID == "Apollo" {
                 bvnIndex = 0
             } else if partitionID == "Chandrayaan" {
                 bvnIndex = 1
             } else if partitionID == "Voyager" {
                 bvnIndex = 2
             }
             a.AddBVNValidator(bvnIndex, *validator)
         }
         
         // Add to legacy Validators slice for backward compatibility
         a.Validators = append(a.Validators, validator)
         
         return validator
     }
     ```
   - Implement AddDNValidator, AddBVNValidator, FindValidatorsByPartition, FindValidatorsByBVN, and UpdateValidatorHeight
   - Update FindValidator to search in DNValidators, BVNValidators, and legacy Validators

3. **Implement Address Management Methods** ⏳
   - Implement address setters with proper locking and validation:
     ```go
     func (a *AddressDir) SetValidatorP2PAddress(peerID, address string) bool {
         validator, found := a.FindValidator(peerID)
         if !found {
             return false
         }
         
         a.mu.Lock()
         defer a.mu.Unlock()
         
         validator.P2PAddress = address
         
         // Also add to the Addresses slice for backward compatibility
         addressExists := false
         for _, addr := range validator.Addresses {
             if addr == address {
                 addressExists = true
                 break
             }
         }
         
         if !addressExists {
             validator.Addresses = append(validator.Addresses, address)
         }
         
         // Set address status
         validator.AddressStatus[address] = "active"
         validator.LastUpdated = time.Now()
         return true
     }
     ```
   - Implement similar methods for other address types (RPC, API, IP, Metrics)
   - Implement GetKnownAddressesForValidator to return all address types in a map

4. **Implement Problem Node Management Methods** ⏳
   - Implement methods for tracking problematic nodes:
     ```go
     func (a *AddressDir) MarkNodeProblematic(peerID, reason string) bool {
         validator, found := a.FindValidator(peerID)
         if !found {
             return false
         }
         
         a.mu.Lock()
         defer a.mu.Unlock()
         
         validator.IsProblematic = true
         validator.ProblemReason = reason
         validator.ProblemSince = time.Now()
         validator.LastUpdated = time.Now()
         return true
     }
     ```
   - Implement methods for request type avoidance (AddRequestTypeToAvoid, ShouldAvoidForRequestType)
   - Implement GetProblemNodes to return a consolidated list of problematic nodes

### Phase 3: Network Peer Discovery ⏳

1. **Update DiscoverNetworkPeers** ⏳
   - Implement according to the design with detailed network status processing:
     ```go
     func (a *AddressDir) DiscoverNetworkPeers(ctx context.Context, client api.NetworkService) (int, error) {
         // Get network status from the client
         status, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
         if err != nil {
             return 0, fmt.Errorf("failed to get network status: %w", err)
         }
         
         // Update network information
         a.mu.Lock()
         a.NetworkInfo.Name = status.Network.Name
         a.NetworkInfo.ID = status.Network.ID
         a.NetworkInfo.IsMainnet = status.Network.IsMainnet
         a.NetworkInfo.APIEndpoint = client.BaseURL()
         
         // Update partitions
         a.NetworkInfo.Partitions = make([]*PartitionInfo, 0, len(status.Network.Partitions))
         a.NetworkInfo.PartitionMap = make(map[string]*PartitionInfo)
         
         for _, p := range status.Network.Partitions {
             partition := &PartitionInfo{
                 ID:       p.ID,
                 Type:     p.Type,
                 URL:      a.constructPartitionURL(p.ID),
                 Active:   true,
                 BVNIndex: -1,
             }
             
             // Set BVN index for BVNs
             if p.Type == "bvn" {
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
         a.mu.Unlock()
         
         // Track validators and seen peers
         validators := make(map[string]bool)
         seenPeers := make(map[string]bool)
         
         // Discover directory peers
         dirPeers := a.discoverDirectoryPeers(ctx, status, validators, seenPeers)
         
         // Discover partition peers
         partitionPeers := 0
         for _, partition := range status.Network.Partitions {
             count, err := a.discoverPartitionPeers(ctx, client, partition, validators, seenPeers)
             if err != nil {
                 a.Logger.Printf("Error discovering peers for partition %s: %v", partition.ID, err)
                 continue
             }
             partitionPeers += count
         }
         
         // Add known non-validator peers
         nonValidatorPeers := a.discoverCommonNonValidators(seenPeers)
         
         // Return total number of peers
         return dirPeers + partitionPeers + nonValidatorPeers, nil
     }
     ```

2. **Implement Helper Methods** ⏳
   - Implement discoverDirectoryPeers to process directory validators:
     ```go
     func (a *AddressDir) discoverDirectoryPeers(ctx context.Context, status *api.NetworkStatus, validators map[string]bool, seenPeers map[string]bool) int {
         a.mu.Lock()
         defer a.mu.Unlock()
         
         count := 0
         
         // Process directory validators
         for _, validator := range status.Directory.Validators {
             // Skip if already seen
             if validators[validator.ID] {
                 continue
             }
             
             // Add validator
             v := a.AddValidator(validator.ID, validator.Name, "dn", "dn")
             
             // Add validator addresses
             if validator.Address != "" {
                 a.SetValidatorP2PAddress(validator.ID, validator.Address)
             }
             
             // Mark as seen
             validators[validator.ID] = true
             seenPeers[validator.ID] = true
             count++
             
             // Add to discovery stats
             a.DiscoveryStats.TotalValidators++
             a.DiscoveryStats.MethodStats["directory"]++
         }
         
         return count
     }
     ```
   - Implement discoverPartitionPeers to process partition-specific validators
   - Implement discoverCommonNonValidators to add known non-validator peers

3. **Implement RefreshNetworkPeers** ⏳
   - Implement the RefreshNetworkPeers method with detailed status tracking:
     ```go
     func (a *AddressDir) RefreshNetworkPeers(ctx context.Context) RefreshStats {
         a.mu.Lock()
         defer a.mu.Unlock()
         
         stats := RefreshStats{
             TotalPeers:        len(a.NetworkPeers),
             NewPeers:          0,
             LostPeers:         0,
             UpdatedPeers:      0,
             UnchangedPeers:    0,
             TotalValidators:   0,
             TotalNonValidators: 0,
             ActiveValidators:  0,
             InactiveValidators: 0,
             UnreachableValidators: 0,
         }
         
         // Check each peer
         for id, peer := range a.NetworkPeers {
             // Skip non-validators for now
             if !peer.IsValidator {
                 stats.TotalNonValidators++
                 continue
             }
             
             stats.TotalValidators++
             
             // Check if validator exists
             validator, found := a.FindValidator(id)
             if !found {
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
             
             // Check if status changed
             if oldStatus != peer.Status {
                 stats.UpdatedPeers++
                 stats.ChangedPeerIDs = append(stats.ChangedPeerIDs, id)
             } else {
                 stats.UnchangedPeers++
             }
             
             // Update peer
             peer.LastSeen = time.Now()
             a.NetworkPeers[id] = peer
         }
         
         return stats
     }
     ```

### Phase 4: URL Construction Standardization ⏳

1. **Standardize URL Construction** ⏳
   - Ensure consistent URL construction across the codebase using the helper methods:
     ```go
     func (a *AddressDir) constructPartitionURL(partitionID string) string {
         return fmt.Sprintf("acc://%s.%s", partitionID, a.NetworkInfo.ID)
     }
     
     func (a *AddressDir) constructAnchorURL(partitionID string) string {
         return fmt.Sprintf("acc://dn.%s/anchors/%s", a.NetworkInfo.ID, partitionID)
     }
     ```
   - Update all code that constructs URLs to use these helper methods
   - Ensure heal_anchor.go uses the same URL construction logic as sequence.go

2. **Add URL Construction Tests** ⏳
   - Create tests to verify URL construction consistency
   - Test both partition URLs and anchor URLs
   - Test with different network IDs and partition IDs

3. **Update Caching System** ⏳
   - Ensure the caching system is aware of the standardized URL format
   - Update any code that queries URLs to use the standardized format

### Phase 5: Testing and Finalization ⏳

1. **Update Tests** ⏳
   - Update existing tests to work with the new structure
   - Add tests for new functionality:
     - Validator management tests
     - Address management tests
     - Problem node management tests
     - Network peer discovery tests

2. **Add Comprehensive Test Cases** ⏳
   - Test edge cases and error conditions
   - Test with different network configurations
   - Test with problematic nodes and request type avoidance

3. **Final Verification** ⏳
   - Verify all structs and methods match the design
   - Ensure all tests pass
   - Verify no regressions in functionality
   - Verify URL construction standardization

### Phase 6: Documentation and Integration ⏳

1. **Update Documentation** ⏳
   - Update inline documentation to reflect the changes
   - Add examples for new methods
   - Document the URL construction standardization

2. **Integration with Anchor Healing** ⏳
   - Ensure the updated AddressDir works correctly with the anchor healing process
   - Verify that the caching system works with the standardized URLs
   - Test the integration with real network data

## URL Construction Standardization

To address the URL construction differences between sequence.go and heal_anchor.go, we'll implement a standardized URL construction approach:

```go
func (a *AddressDir) constructPartitionURL(partitionID string) string {
    // Use the sequence.go approach (raw partition URLs)
    return fmt.Sprintf("acc://%s.acme", partitionID)
}

func (a *AddressDir) constructAnchorURL(partitionID string) string {
    // Use the sequence.go approach instead of appending to anchor pool URL
    return fmt.Sprintf("acc://%s.acme", partitionID)
}
```

This standardization will ensure consistency across the codebase and prevent anchor healing failures due to URL format discrepancies.

## Testability

A key design goal is to ensure all helper functions are independently testable against mainnet. This modular approach allows for incremental development and testing, with each function verifiable in isolation before integration.

### Testing Categories

1. **Pure Functions**
   - URL construction functions
   - Address parsing and validation
   - Utility functions
   - These can be tested with known inputs and expected outputs

2. **State Management Functions**
   - Validator addition and retrieval
   - Problem node tracking
   - Address setting and retrieval
   - These can be tested with a controlled AddressDir instance

3. **Network Interaction Functions**
   - Peer discovery
   - Height querying
   - Network refresh
   - These can be tested directly against mainnet

### Additional Helper Functions for Testability

To enhance testability, we'll add these helper functions:

```go
// NewTestAddressDir creates an AddressDir pre-populated with test data
func NewTestAddressDir() *AddressDir

// LoadFromMainnet populates an AddressDir with data from mainnet
func (a *AddressDir) LoadFromMainnet(ctx context.Context, client api.NetworkService) error

// ValidateState performs consistency checks on the AddressDir state
func (a *AddressDir) ValidateState() []error

// DumpState returns a string representation of the AddressDir state for debugging
func (a *AddressDir) DumpState() string
```

## Detailed Implementation Plan

We will build all the helper functions incrementally, creating tests as we go. This approach allows us to validate each component independently before integration.

### Phase 1: Foundation and Pure Functions

1. **Setup Project Structure**
   - Create address2.go file with basic imports and package declaration
   - Create address2_test.go file with test framework
   - Implement core data structures (AddressDir, Validator, NetworkPeer)

2. **URL Construction Functions**
   - Implement `constructPartitionURL` and `constructAnchorURL`
   - Create tests with known partition IDs from mainnet
   - Verify URL formats match expected standards

3. **Address Parsing and Validation**
   - Implement `parseMultiaddress` and `ValidateMultiaddress`
   - Create tests with various multiaddress formats from mainnet
   - Test edge cases (invalid formats, missing components)

### Phase 2: Validator Management

4. **Basic Validator Functions**
   - Implement `AddValidator`, `FindValidator`, and `FindValidatorsByPartition`
   - Create tests that add and retrieve validators
   - Verify thread safety with concurrent operations

5. **Network-Specific Validator Functions**
   - Implement `AddDNValidator`, `AddBVNValidator`, and `FindValidatorsByBVN`
   - Create tests that organize validators by network
   - Test with real validator IDs from mainnet

6. **Validator Status and Height Management**
   - Implement `SetValidatorStatus` and `UpdateValidatorHeight`
   - Create tests for status transitions and height updates
   - Test with simulated network events

### Phase 3: Address Management

7. **Address Setting Functions**
   - Implement address type setters (P2P, IP, RPC, API, Metrics)
   - Create tests for each address type
   - Verify address format validation

8. **Known Address Functions**
   - Implement `getKnownAddressesForValidator` (fixed version)
   - Create tests with known validator IDs from mainnet
   - Verify fallback mechanisms work correctly

9. **Address Retrieval Functions**
   - Implement functions to get best addresses by type
   - Create tests for address selection logic
   - Test with various network scenarios

### Phase 4: Problem Node Management

10. **Problem Node Tracking**
    - Implement `MarkNodeProblematic` and `IsNodeProblematic`
    - Create tests for problem node identification
    - Test problem reason tracking

11. **Request Type Avoidance**
    - Implement `AddRequestTypeToAvoid` and `ShouldAvoidForRequestType`
    - Create tests for request type filtering
    - Test with various request scenarios

12. **Problem Node Recovery**
    - Implement `ClearProblematicStatus`
    - Create tests for node recovery
    - Test automatic and manual recovery paths

### Phase 5: Network Peer Management

13. **Peer Discovery**
    - Implement `DiscoverNetworkPeers`
    - Create tests with mock network service
    - Test against mainnet (read-only)

14. **Peer Refresh**
    - Implement `RefreshNetworkPeers`
    - Create tests for peer state updates
    - Verify statistics collection

15. **Peer Endpoint Management**
    - Implement `GetPeerRPCEndpoint`
    - Create tests for endpoint selection
    - Test fallback mechanisms

### Phase 6: Integration and Advanced Testing

16. **Testability Helpers**
    - Implement `NewTestAddressDir`, `LoadFromMainnet`, `ValidateState`, and `DumpState`
    - Create meta-tests that use these helpers
    - Verify they simplify testing of other components

17. **peer_state.go Integration**
    - Implement any additional functions needed for peer_state.go
    - Create integration tests with peer_state.go
    - Verify seamless interaction

18. **Comprehensive Mainnet Testing**
    - Create end-to-end tests against mainnet
    - Test all functions in real-world scenarios
    - Verify performance and reliability

### Testing Strategy

For each helper function, we will:

1. **Write Unit Tests First**
   - Define expected behavior before implementation
   - Cover normal operation, edge cases, and error conditions
   - Use table-driven tests for comprehensive coverage

2. **Implement the Function**
   - Follow the design specifications
   - Add detailed comments explaining the logic
   - Ensure thread safety where needed

3. **Test Against Mainnet**
   - Verify behavior with real network data
   - Compare results with existing implementation
   - Document any discrepancies

4. **Refine as Needed**
   - Adjust implementation based on test results
   - Optimize for performance if necessary
   - Add more test cases for discovered edge cases

This incremental approach ensures that each component is thoroughly tested before moving on to the next, resulting in a robust and reliable implementation.

## Lessons Learned from simple_peer_discovery

The implementation and testing of simple_peer_discovery provided several valuable insights that will inform our address2.go design:

1. **Multiaddress Parsing Complexity**:
   - Multiaddresses have various formats and components (/ip4, /ip6, /dns, /tcp, /p2p, etc.)
   - Robust parsing requires handling multiple protocols and edge cases
   - The go-multiaddr library provides reliable parsing but needs careful error handling

2. **Validator ID to Address Mapping**:
   - Hardcoded mappings provide a reliable fallback for known validators
   - Different naming conventions exist for validators (e.g., "defidevs.acme" vs "0b2d838c")
   - Both peer ID and partition ID can be used to identify validators

3. **URL and Endpoint Construction**:
   - Different services use different port conventions (16592 for RPC, 16593 for P2P)
   - URL construction needs to handle various schemes (http://, https://, acc://)
   - Standardized endpoint construction improves interoperability

4. **Comprehensive Logging**:
   - Detailed logging is essential for debugging network issues
   - Multi-writer logging (file + stdout) provides both persistence and immediate visibility
   - Structured logging with timestamps and categories improves analysis

5. **Implementation Comparison Testing**:
   - Comparing new implementations with existing ones helps identify discrepancies
   - Edge case testing is crucial for network-related code
   - Statistical analysis of success/failure rates provides valuable insights

6. **Format Analysis Tools**:
   - Tools for analyzing URL and multiaddress formats help understand the data
   - Different components (host, protocol, port, path) need specific handling
   - Validation functions should provide detailed error messages

These lessons will be incorporated into the address2.go implementation to ensure robust handling of validator addresses and network peers.

## Network Usage and Management

### Network Information Usage

The `Network` field in `AddressDir` is central to the operation of the address management system. It replaces the simple `NetworkName` string and provides comprehensive information about the network and its partitions.

```go
// InitializeNetwork sets up the Network field with appropriate information
func (a *AddressDir) InitializeNetwork(networkName string) error {
    // Create a new NetworkInfo instance
    network := &NetworkInfo{
        Name:         networkName,
        ID:           "acme", // Default network ID
        IsMainnet:    networkName == "mainnet",
        Partitions:   make([]*PartitionInfo, 0),
        PartitionMap: make(map[string]*PartitionInfo),
    }
    
    // Set the API endpoint based on the network name
    network.APIEndpoint = resolveNetworkEndpoint(networkName)
    
    // Initialize the network's partitions
    if err := a.initializeNetworkPartitions(network); err != nil {
        return err
    }
    
    // Set the Network field
    a.Network = network
    return nil
}
```

The Network field is used to:

1. **Determine API Endpoints**: The `APIEndpoint` field provides the base URL for API requests
2. **Identify Network Type**: The `IsMainnet` flag allows for network-specific behavior
3. **Access Partition Information**: The `Partitions` and `PartitionMap` fields provide partition data
4. **Standardize URL Construction**: Ensures consistent URL formats across the codebase

### Error Handling Strategy

Error handling in address2.go follows these principles:

1. **Non-blocking Operation**: Discovery processes should never hang or panic
2. **Error Classification**: Errors are categorized by type and severity
3. **Node-specific Error Tracking**: Errors are associated with specific nodes
4. **Automatic Recovery**: The system attempts to recover from transient errors
5. **Error Reporting**: Comprehensive error statistics are maintained

```go
// Error classification constants
const (
    ErrorSeverityLow    = 1 // Transient errors that can be retried
    ErrorSeverityMedium = 2 // Errors that may require attention
    ErrorSeverityHigh   = 3 // Critical errors that prevent operation
)

// NodeError represents an error associated with a specific node
type NodeError struct {
    NodeID      string    // ID of the node that experienced the error
    ErrorType   string    // Type of error (e.g., "connection", "timeout", "validation")
    ErrorMessage string    // Detailed error message
    Timestamp   time.Time // When the error occurred
    Severity    int       // Error severity level
    RetryCount  int       // Number of retry attempts made
}
```

When errors occur during discovery:

1. The error is logged with appropriate context
2. The error is associated with the specific node
3. The node's error count and last error are updated
4. For problematic nodes, they may be marked as such
5. The discovery process continues with other nodes

### Update Strategy

When performing network discovery, the system follows a clear update strategy to maintain the most current state of the blockchain network:

1. **Incremental Updates**: Only changed information is updated
2. **Timestamp Tracking**: All updates include timestamps for freshness evaluation
3. **Conflict Resolution**: When conflicting information is received, resolution rules apply:
   - Newer information takes precedence over older information
   - Information from more reliable nodes is prioritized
   - Consensus-based resolution for critical information

```go
// UpdateNetworkState updates the network state based on discovery results
func (a *AddressDir) UpdateNetworkState(ctx context.Context, client api.NetworkService) (RefreshStats, error) {
    stats := RefreshStats{}
    
    // 1. Discover network peers
    peerCount, err := a.DiscoverNetworkPeers(ctx, client)
    if err != nil {
        // Log error but continue with partial results
        a.Logger.Printf("Error discovering network peers: %v", err)
    }
    stats.TotalPeers = peerCount
    
    // 2. Update validator information
    a.updateValidatorInformation(ctx, client, &stats)
    
    // 3. Update partition information
    a.updatePartitionInformation(ctx, client, &stats)
    
    // 4. Prune stale information
    a.pruneStaleInformation(&stats)
    
    return stats, nil
}
```

This update strategy ensures that all processes have access to the best current state of the blockchain network, which is critical for proper operation.

### Partition Management

Partition management is driven entirely by the `NetworkInfo` structure. The system:

1. **Discovers Partitions**: Automatically identifies all partitions in the network
2. **Tracks Partition Status**: Monitors the active/inactive status of partitions
3. **Standardizes URLs**: Uses consistent URL formats for all partitions
4. **Maps Validators to Partitions**: Maintains the relationship between validators and partitions

```go
// initializeNetworkPartitions sets up the partitions for a network
func (a *AddressDir) initializeNetworkPartitions(network *NetworkInfo) error {
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
    return a.discoverNetworkPartitions(network)
}
```

This approach ensures that partition selection and management are consistent across the codebase, addressing the URL construction differences identified in the development plan.

## Implementation Plan

### Phase 1: Core Structure Implementation

1. **Define Data Structures**
   - Implement `NetworkInfo` and `PartitionInfo` structures
   - Update `AddressDir` to use `Network` instead of `NetworkName`
   - Implement error tracking structures

2. **Network Initialization**
   - Implement `InitializeNetwork` function
   - Implement partition initialization for different network types
   - Add support for custom network configurations

### Phase 2: Network Discovery Implementation

1. **Directory Network Discovery**
   - Implement `discoverDirectoryPeers` function
   - Add support for validator identification and tracking
   - Implement error handling for discovery failures

2. **Partition Discovery**
   - Implement `discoverPartitionPeers` function
   - Add support for BVN-specific discovery
   - Implement partition status tracking

3. **Non-validator Discovery**
   - Implement `discoverCommonNonValidators` function
   - Add support for known peer tracking

### Phase 3: Update and Maintenance

1. **Network State Updates**
   - Implement `UpdateNetworkState` function
   - Add support for incremental updates
   - Implement conflict resolution logic

2. **Error Management**
   - Implement error classification system
   - Add support for node-specific error tracking
   - Implement automatic recovery mechanisms

3. **Partition Management**
   - Implement URL standardization
   - Add support for partition-validator mapping
   - Implement partition status monitoring

### Phase 4: Testing and Validation

1. **Unit Testing**
   - Test each component in isolation
   - Verify error handling behavior
   - Validate update strategies

2. **Integration Testing**
   - Test network discovery against mock networks
   - Verify interaction between components
   - Validate partition management

3. **Mainnet Testing**
   - Test against the live mainnet
   - Verify discovery of all validators and partitions
   - Validate error handling in real-world scenarios

## Conclusion

This enhanced design for address2.go provides a comprehensive approach to network discovery and management. By focusing on a robust Network structure, clear error handling, and consistent update strategies, we can ensure that the code is reliable, maintainable, and effective in managing the Accumulate network.

The design prioritizes:

1. **Clarity**: Clear definition of network structures and relationships
2. **Reliability**: Robust error handling and recovery mechanisms
3. **Consistency**: Standardized URL construction and partition management
4. **Currency**: Always maintaining the most up-to-date network state
5. **Modularity**: Clear separation of concerns between different components
6. **Testability**: Ensuring all components are independently testable

This approach will address the URL construction differences identified in the development plan and provide a solid foundation for the anchor healing process.
