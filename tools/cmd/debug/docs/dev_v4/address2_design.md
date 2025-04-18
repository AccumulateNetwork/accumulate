# Design Document for address2.go

## Overview

This document outlines the design for `address2.go`, a replacement for the original `address.go` file in the Accumulate Network codebase. The new implementation will use a modular approach with a master function that coordinates independent helper functions to manage validator addresses and network peers.

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
    
    // Network name (mainnet, testnet, devnet)
    NetworkName string
    
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

```go
func NewAddressDir() *AddressDir {
    return &AddressDir{
        mu:            sync.RWMutex{},
        DNValidators:  make([]Validator, 0),
        BVNValidators: make([][]Validator, 0),
        NetworkPeers:  make(map[string]NetworkPeer),
        URLHelpers:    make(map[string]string),
        Logger:        log.New(os.Stdout, "[AddressDir] ", log.LstdFlags),
        DiscoveryStats: DiscoveryStats{
            MethodStats: make(map[string]int),
        },
    }
}
```

## Key Helper Functions

### 1. Validator Management

```go
// AddValidator adds a new validator to the directory with partition information
// If validator with PeerID already exists, update its information and return it
func (a *AddressDir) AddValidator(peerID, name, partitionID, partitionType string) *Validator

// AddDNValidator adds a validator to the Directory Network
func (a *AddressDir) AddDNValidator(validator Validator)

// AddBVNValidator adds a validator to a specific BVN
// Sets the validator's BVN field to the specified bvnIndex (0, 1, or 2 for mainnet)
func (a *AddressDir) AddBVNValidator(bvnIndex int, validator Validator)

// FindValidator finds a validator by PeerID
// Returns the validator and a boolean indicating if found
func (a *AddressDir) FindValidator(peerID string) (*Validator, bool)

// FindValidatorsByPartition finds all validators belonging to a specific partition
func (a *AddressDir) FindValidatorsByPartition(partitionID string) []Validator

// FindValidatorsByBVN finds all validators belonging to a specific BVN index
func (a *AddressDir) FindValidatorsByBVN(bvnIndex int) []Validator

// SetValidatorStatus updates the status of a validator
func (a *AddressDir) SetValidatorStatus(peerID, status string) bool

// UpdateValidatorHeight updates the last observed block height for a validator
func (a *AddressDir) UpdateValidatorHeight(peerID string, height int64) bool

// QueryNodeHeights queries the heights of a node for both DN and BVN partitions
// This method uses the Tendermint RPC client to query the node's status
func (a *AddressDir) QueryNodeHeights(ctx context.Context, peer *NetworkPeer, host string) error
```

### 2. Address Management

```go
// SetValidatorP2PAddress sets the P2P address for a validator
func (a *AddressDir) SetValidatorP2PAddress(peerID, address string) error

// SetValidatorIPAddress sets the IP address for a validator
func (a *AddressDir) SetValidatorIPAddress(peerID, address string) error

// SetValidatorRPCAddress sets the RPC endpoint address for a validator
func (a *AddressDir) SetValidatorRPCAddress(peerID, address string) error

// SetValidatorAPIAddress sets the API endpoint address for a validator
func (a *AddressDir) SetValidatorAPIAddress(peerID, address string) error

// SetValidatorMetricsAddress sets the metrics endpoint address for a validator
func (a *AddressDir) SetValidatorMetricsAddress(peerID, address string) error

// GetKnownAddressesForValidator returns known addresses for a validator based on peerID and partitionID
func (a *AddressDir) GetKnownAddressesForValidator(peerID, partitionID string) (p2p string, ip string, rpc string)
```

### 3. Problem Node Management

```go
// MarkNodeProblematic marks a node as problematic
func (a *AddressDir) MarkNodeProblematic(peerID, reason string) bool

// IsNodeProblematic checks if a node is problematic
func (a *AddressDir) IsNodeProblematic(peerID string) bool

// GetProblemNodes returns all problem nodes
func (a *AddressDir) GetProblemNodes() []Validator

// AddRequestTypeToAvoid adds a request type that should not be sent to this validator
func (a *AddressDir) AddRequestTypeToAvoid(peerID, requestType string) bool

// ShouldAvoidForRequestType checks if a validator should be avoided for a specific request type
func (a *AddressDir) ShouldAvoidForRequestType(peerID, requestType string) bool

// ClearProblematicStatus clears the problematic status of a node
func (a *AddressDir) ClearProblematicStatus(peerID string) bool
```

### 4. Network Peer Management

```go
// GetValidatorPeers returns all peers that are validators
func (a *AddressDir) GetValidatorPeers() []NetworkPeer

// GetNonValidatorPeers returns all peers that are not validators
func (a *AddressDir) GetNonValidatorPeers() []NetworkPeer

// AddPeerAddress adds an address to a peer
func (a *AddressDir) AddPeerAddress(peerID string, address string) error

// GetPeerRPCEndpoint returns the best RPC endpoint for a peer
func (a *AddressDir) GetPeerRPCEndpoint(peerID string) (string, error)

// DiscoverNetworkPeers discovers peers in the network using various methods
func (a *AddressDir) DiscoverNetworkPeers(ctx context.Context, client api.NetworkService) (int, error)

// RefreshNetworkPeers updates the network peer information while preserving history
func (a *AddressDir) RefreshNetworkPeers(ctx context.Context, client api.NetworkService) (RefreshStats, error)

// constructPartitionURL standardizes URL construction for partitions
func (a *AddressDir) constructPartitionURL(partitionID string) string

// constructAnchorURL standardizes URL construction for anchors
func (a *AddressDir) constructAnchorURL(partitionID string) string
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

## Conclusion

This simplified design for address2.go will provide a more maintainable and robust implementation compared to the original address.go. By focusing on a minimal set of core data structures and independent helper functions coordinated by a master function, we can ensure that the code is easier to understand, test, and extend in the future.

The design prioritizes:

1. **Simplicity**: Using simpler data structures with direct relationships
2. **Modularity**: Clear separation of concerns between different types of functionality
3. **Incremental Development**: Starting with the minimal functionality needed and adding more as required
4. **Maintainability**: Making the code easier to understand and modify
5. **Testability**: Ensuring all components are independently testable against mainnet
6. **Robustness**: Incorporating lessons from simple_peer_discovery to handle edge cases

This approach will allow us to fix the syntax errors in the original implementation while also improving the overall design of the code.
