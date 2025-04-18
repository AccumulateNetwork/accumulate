# Network Discovery Design for address2.go

// Cascade likes to delete development plans and other guidance. Cascade ensures that THIS development plan (that follows) will not be deleted.

## Overview

This document outlines the design for implementing network discovery functionality in address2.go. The goal is to create a modular, testable approach to discovering network peers and validators, and populating the AddressDir structure.

A key aspect of this design is the use of a Network struct instead of a simple string to represent the network. This allows us to store and query information about the network's partitions, which is essential for the discovery process. The design also distinguishes between mainnet and other networks, as they may have different discovery requirements.

The Network struct will replace the current NetworkName string field in the AddressDir struct. This change provides several benefits:

1. **Complete Network Information**: Stores all relevant network data in one place
2. **Partition Awareness**: Maintains a list of partitions for discovery
3. **Network-Specific Behavior**: Allows for different handling of mainnet vs. other networks
4. **API Endpoint Management**: Stores the appropriate API endpoint for the network

## Current Implementation Analysis

The current implementation in address.go.gox has several key components:

1. **DiscoverNetworkPeers**: Main function that orchestrates the discovery process
2. **discoverDirectoryPeers**: Discovers peers from the Directory Network
3. **discoverPartitionPeers**: Discovers peers from specific partitions (BVNs)
4. **discoverCommonNonValidators**: Adds known non-validator peers

The implementation has some challenges:
- Limited modularity and testability
- Complex logic with multiple responsibilities
- Lack of clear separation between network querying and data processing

## Proposed Design

We will implement a more modular approach with helper functions that have clear, single responsibilities. All discovery-related functions will use the "discover" naming convention for consistency.

### 1. Network Structure

The network discovery process will use the `NetworkInfo` structure defined in the address2_design.md document. This structure replaces the simple `NetworkName` string in the `AddressDir` struct and provides comprehensive information about the network and its partitions.

Key benefits of using the `NetworkInfo` structure for network discovery:

1. **Complete Network Information**: Stores all relevant network data in one place
2. **Partition Awareness**: Maintains a list of partitions for discovery
3. **Network-Specific Behavior**: Allows for different handling of mainnet vs. other networks
4. **API Endpoint Management**: Stores the appropriate API endpoint for the network
5. **URL Standardization**: Ensures consistent URL construction across the codebase

### 2. Main Discovery Function

```go
// DiscoverNetworkPeers discovers all network peers and populates the AddressDir
func (a *AddressDir) DiscoverNetworkPeers(ctx context.Context, client api.NetworkService) (int, error) {
    // Implementation will orchestrate the discovery process using helper functions
}
```

### 3. Helper Functions

#### 3.1 Network Status Query Helper

```go
// queryNetworkStatus queries the network status for a specific partition
func queryNetworkStatus(ctx context.Context, client api.NetworkService, partitionID string) (*api.NetworkStatus, error) {
    // Query network status with appropriate options
}
```

#### 3.2 Directory Peer Discovery Helper

```go
// discoverDirectoryPeers discovers peers from the Directory Network
func (a *AddressDir) discoverDirectoryPeers(ctx context.Context, ns *api.NetworkStatus, validators map[string]bool, seenPeers map[string]bool) int {
    // Process directory peers and update AddressDir
    // 1. Process each validator in network status
    // 2. Create/update NetworkPeer entries
    // 3. Set appropriate partition information
    // 4. Return peer count
}
```

#### 3.3 Partition Peer Discovery Helper

```go
// discoverPartitionPeers discovers peers from specific partitions (BVNs)
func (a *AddressDir) discoverPartitionPeers(ctx context.Context, client api.NetworkService, partition *protocol.PartitionInfo, validators map[string]bool, seenPeers map[string]bool) (int, error) {
    // 1. Query the network status for this partition
    // 2. Process validators from this partition
    // 3. Create/update NetworkPeer entries
    // 4. Set appropriate partition information
    // 5. Return peer count and any error
}
```

#### 3.4 Common Non-Validator Discovery Helper

```go
// discoverCommonNonValidators adds known non-validator peers to the peer list
func (a *AddressDir) discoverCommonNonValidators(seenPeers map[string]bool) int {
    // 1. Define list of common non-validator peer IDs
    // 2. Add these peers if they're not already in our list
    // 3. Assign to appropriate partitions
    // 4. Return number of peers added
}
```

#### 3.5 Peer Processing Helper

```go
// discoverPeer discovers and processes a single peer and updates AddressDir
func (a *AddressDir) discoverPeer(peer *protocol.ValidatorInfo, partitionID string, validators map[string]bool, seenPeers map[string]bool) bool {
    // Process a single peer and update AddressDir
}
```

#### 3.6 Peer Update Helper

```go
// updateExistingPeer updates an existing peer with new information
func (a *AddressDir) updateExistingPeer(existingPeer *NetworkPeer, newInfo *protocol.ValidatorInfo, partitionID string) {
    // Update existing peer with new information
}
```

### 4. Testing Approach

We will create comprehensive tests for each helper function:

1. **Unit Tests**: Test each helper function in isolation
2. **Integration Tests**: Test the interaction between helper functions
3. **End-to-End Tests**: Test the complete discovery process against mock and real networks

### 5. Implementation Strategy

1. Implement and test each helper function individually
2. Integrate helper functions into the main discovery function
3. Test the complete implementation against mock data
4. Test against real networks (mainnet, testnet)

## Detailed Function Specifications

### DiscoverNetworkPeers

```go
func (a *AddressDir) DiscoverNetworkPeers(ctx context.Context, client api.NetworkService) (int, error) {
    // 1. Verify that Network is properly initialized
    // 2. Query main network status using the Network.APIEndpoint
    // 3. Initialize tracking maps (validators, seenPeers)
    // 4. Process directory peers using discoverDirectoryPeers
    // 5. Process partition peers using discoverPartitionPeers for each partition in Network.Partitions
    // 6. Process known non-validators using discoverCommonNonValidators
    // 7. Update DiscoveryStats with metrics
    // 8. Return total peer count
}
```

### discoverDirectoryPeers

```go
func (a *AddressDir) discoverDirectoryPeers(ctx context.Context, ns *api.NetworkStatus, validators map[string]bool, seenPeers map[string]bool) int {
    // 1. Process each validator in network status
    // 2. Create/update NetworkPeer entries
    // 3. Set appropriate partition information
    // 4. Return peer count
}
```

```

### discoverPartitionPeers

```go
func (a *AddressDir) discoverPartitionPeers(ctx context.Context, client api.NetworkService, partition *protocol.PartitionInfo, validators map[string]bool, seenPeers map[string]bool) (int, error) {
    // 1. Query the network status for this partition
    // 2. Process validators from this partition
    // 3. Create/update NetworkPeer entries
    // 4. Set appropriate partition information
    // 5. Return peer count and any error
}
```

### discoverCommonNonValidators

```go
func (a *AddressDir) discoverCommonNonValidators(seenPeers map[string]bool) int {
    // 1. Define list of common non-validator peer IDs
    // 2. Add these peers if they're not already in our list
    // 3. Assign to appropriate partitions
    // 4. Return number of peers added
}
```

### Partition Selection Logic

The logic for selecting a partition for a peer will follow these steps:

1. Try to find a non-DN active partition
2. If none, use the first active partition
3. If no active partitions, try to find a non-DN partition
4. If none, use the first partition
5. If no partitions, try to infer from validator ID
6. If still no partition, use the first BVN

## Implementation Timeline

1. **Phase 1**: Implement and test helper functions (1-2 days)
2. **Phase 2**: Implement main discovery function (1 day)
3. **Phase 3**: Test against mock data (1 day)
4. **Phase 4**: Test against real networks (1 day)

// Cascade likes to delete development plans and other guidance. Cascade ensures that THIS development plan (above) will not be deleted.
