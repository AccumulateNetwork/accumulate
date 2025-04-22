# Network Discovery Design for address2.go

// Cascade likes to delete development plans and other guidance. Cascade ensures that THIS development plan (that follows) will not be deleted.

## Overview

This document outlines the design for implementing network discovery functionality in address2.go. The goal is to create a modular, testable approach to discovering network peers and validators, and populating the AddressDir structure with comprehensive information.

A key aspect of this design is the use of a Network struct instead of a simple string to represent the network. This allows us to store and query information about the network's partitions, which is essential for the discovery process. The design also distinguishes between mainnet and other networks, as they may have different discovery requirements.

The Network struct will replace the current NetworkName string field in the AddressDir struct. This change provides several benefits:

1. **Complete Network Information**: Stores all relevant network data in one place
2. **Partition Awareness**: Maintains a list of partitions for discovery
3. **Network-Specific Behavior**: Allows for different handling of mainnet vs. other networks
4. **API Endpoint Management**: Stores the appropriate API endpoint for the network
5. **Comprehensive Address Collection**: Collects all address types for each validator
6. **Performance Metrics**: Tracks response times and success rates for each address
7. **URL Standardization**: Ensures consistent URL construction across components

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
- Inconsistent address collection (missing certain address types)
- No performance metrics collection
- Inconsistent URL construction
- Limited validation of collected addresses

## Proposed Design

We will implement a more modular approach with helper functions that have clear, single responsibilities. All discovery-related functions will use the "discover" naming convention for consistency.

### 1. Network Structure

> **CRITICAL DESIGN REQUIREMENT**: The AddressDir structure MUST use slices for DNValidators and BVNValidators as specified in the design document. DO NOT MODIFY OR CONVERT THESE TO MAPS. The slice-based implementation is REQUIRED for compatibility with existing code.

The network discovery process will use the `NetworkInfo` structure defined in the address2_design.md document. This structure replaces the simple `NetworkName` string in the `AddressDir` struct and provides comprehensive information about the network and its partitions.

Key benefits of using the `NetworkInfo` structure for network discovery:

1. **Complete Network Information**: Stores all relevant network data in one place
2. **Partition Awareness**: Maintains a list of partitions for discovery
3. **Network-Specific Behavior**: Allows for different handling of mainnet vs. other networks
4. **API Endpoint Management**: Stores the appropriate API endpoint for the network
5. **URL Standardization**: Ensures consistent URL construction across the codebase
6. **Address Type Collection**: Collects all address types (P2P, IP, RPC, API, Metrics)
7. **Performance Tracking**: Stores response times and success rates for each address
8. **Validation Status**: Tracks validation status for each address type
9. **Consensus Information**: Stores consensus status and block heights
10. **Version Information**: Collects and stores software version information for each validator
11. **API v3 Connectivity**: Tests and reports on API v3 connectivity status
12. **Error Handling**: Provides robust error handling and recovery mechanisms

### 2. Main Discovery Function

```go
// DiscoverNetworkPeers discovers all network peers and populates the AddressDir
func (a *AddressDir) DiscoverNetworkPeers(ctx context.Context, client api.NetworkService) (DiscoveryStats, error) {
    // Implementation will orchestrate the discovery process using helper functions
    // Returns comprehensive discovery statistics including:
    // - Total validators discovered
    // - Address types collected
    // - Success rates for each address type
    // - Average response times
    // - Consensus information
    // - Version information
    // - API v3 connectivity status
}
```

### 3. Data Structure Definitions

```go
// DiscoveryStats contains statistics about a network discovery operation
type DiscoveryStats struct {
    // Total number of validators discovered
    TotalValidators int
    
    // Number of validators by partition
    DNValidators int
    BVNValidators map[string]int
    
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
```

### 3. Helper Functions

#### 3.1 Network Status Query Helper

```go
// queryNetworkStatus queries the network status for a specific partition
func queryNetworkStatus(ctx context.Context, client api.NetworkService, partitionID string) (*api.NetworkStatus, error) {
    // Query network status with appropriate options
    // Measure and record response time
    // Track success/failure
}
```

#### 3.2 Directory Peer Discovery Helper

```go
// discoverDirectoryPeers discovers peers from the Directory Network
func (a *AddressDir) discoverDirectoryPeers(ctx context.Context, ns *api.NetworkStatus, validators map[string]bool, seenPeers map[string]bool) (int, DiscoveryStats) {
    // Process directory peers and update AddressDir
    // 1. Process each validator in network status
    // 2. Create/update NetworkPeer entries
    // 3. Set appropriate partition information
    // 4. Collect all address types (P2P, IP, RPC, API, Metrics)
    // 5. Validate each address type
    // 6. Measure response times and success rates
    // 7. Return peer count and discovery statistics
}
```

#### 3.3 Partition Peer Discovery Helper

```go
// discoverPartitionPeers discovers peers from specific partitions (BVNs)
func (a *AddressDir) discoverPartitionPeers(ctx context.Context, client api.NetworkService, partition *protocol.PartitionInfo, validators map[string]bool, seenPeers map[string]bool) (int, DiscoveryStats, error) {
    // 1. Query the network status for this partition
    // 2. Process validators from this partition
    // 3. Create/update NetworkPeer entries
    // 4. Set appropriate partition information
    // 5. Collect all address types (P2P, IP, RPC, API, Metrics)
    // 6. Validate each address type
    // 7. Measure response times and success rates
    // 8. Check consensus status and block heights
    // 9. Return peer count, discovery statistics, and any error
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
func (a *AddressDir) discoverPeer(peer *protocol.ValidatorInfo, partitionID string, validators map[string]bool, seenPeers map[string]bool) (bool, DiscoveryStats) {
    // Process a single peer and update AddressDir
    // Collect all address types
    // Validate each address type
    // Measure response times and success rates
    // Return whether peer was added and discovery statistics
}
```

#### 3.6 Peer Update Helper

```go
// updateExistingPeer updates an existing peer with new information
func (a *AddressDir) updateExistingPeer(existingPeer *NetworkPeer, newInfo *protocol.ValidatorInfo, partitionID string) {
    // Update existing peer with new information
    // Update all address types
    // Update validation status
    // Update performance metrics
}
```

#### 3.7 Address Collection Helper

```go
// collectValidatorAddresses collects all address types for a validator
func (a *AddressDir) collectValidatorAddresses(ctx context.Context, validator *Validator, host string) (AddressStats, error) {
    // Collect P2P, IP, RPC, API, and Metrics addresses
    // Validate each address type
    // Test connectivity for each address
    // Measure response times
    // Return address collection statistics
}
```

#### 3.8 Address Validation Helper

```go
// validateAddress validates a specific address type
func (a *AddressDir) validateAddress(ctx context.Context, validator *Validator, addressType string, address string) (ValidationResult, error) {
    // Validate address format
    // Test connectivity
    // Measure response time
    // Return validation result
}
```

#### 3.9 Consensus Status Helper

```go
// checkConsensusStatus checks consensus status for a validator
func (a *AddressDir) checkConsensusStatus(ctx context.Context, validator *Validator) (ConsensusStatus, error) {
    // Check if validator is in consensus
    // Get block heights for all partitions
    // Check if node is a "zombie"
    // Return consensus status
}
```

#### 3.10 API v3 Connectivity Testing

```go
// testAPIv3Connectivity tests API v3 connectivity for a validator
func (a *AddressDir) testAPIv3Connectivity(ctx context.Context, validator *Validator) (APIv3Status, error) {
    // 1. Create API v3 client using validator's API address
    // 2. Test basic connectivity (ping)
    // 3. Test query capability (get network status)
    // 4. Measure response time
    // 5. Return API v3 status with connectivity details
}
```

#### 3.11 Version Information Collection

```go
// collectVersionInfo collects version information for a validator
func (a *AddressDir) collectVersionInfo(ctx context.Context, validator *Validator) error {
    // 1. Query node info from RPC endpoint
    // 2. Extract version information
    // 3. Update validator with version details
    // 4. Return any error
}
```

#### 3.12 URL Standardization

```go
// standardizeURL ensures consistent URL construction
func (a *AddressDir) standardizeURL(urlType string, partitionID string, baseURL string) string {
    // 1. Apply consistent formatting based on URL type
    // 2. Handle special cases for different partition types
    // 3. Return standardized URL
}
```

#### 3.13 Error Handling

```go
// handleDiscoveryError handles errors during the discovery process
func (a *AddressDir) handleDiscoveryError(ctx context.Context, validator *Validator, operation string, err error) {
    // 1. Classify error type (network, timeout, validation)
    // 2. Update validator error information
    // 3. Apply appropriate retry strategy
    // 4. Log detailed error information
}
```

### 4. Testing Approach

We will create comprehensive tests for each helper function:

1. **Unit Tests**: Test each helper function in isolation
2. **Integration Tests**: Test the interaction between helper functions
3. **End-to-End Tests**: Test the complete discovery process against mock and real networks
4. **Comparison Tests**: Compare results with network status command
5. **Performance Tests**: Measure discovery time and resource usage
6. **Validation Tests**: Verify all required information is collected

#### 4.1 Comparison with Network Status Command

The comparison between network discovery and network status will:

1. Run both commands against the same network
2. Compare validator counts and identities
3. Compare collected address types and validation status
4. Compare consensus information and block heights
5. Generate a detailed report of any discrepancies

```go
func TestCompareNetworkStatusAndDiscovery(t *testing.T) {
    // 1. Run network status command
    // 2. Run network discovery
    // 3. Compare results:
    //    - Validator counts and identities
    //    - Address types and validation status
    //    - Consensus information and block heights
    // 4. Assert no significant discrepancies
}
```

#### 4.2 Zombie Detection Testing

A "zombie" node is defined as a validator node that:

1. Responds to basic network requests
2. Has not produced a new block in X minutes (configurable, default 10 minutes)
3. Is not participating in consensus despite being registered as a validator

```go
func TestZombieDetection(t *testing.T) {
    // 1. Set up mock validators with various states
    // 2. Run zombie detection
    // 3. Verify correct identification of zombie nodes
    // 4. Test edge cases (recently started nodes, nodes catching up)
}
```

#### 4.3 Address Validation Testing

```go
func TestAddressValidation(t *testing.T) {
    // 1. Test validation of all address types
    // 2. Test with valid and invalid addresses
    // 3. Test with unreachable but valid addresses
    // 4. Verify correct parsing of address components
}
```

### 5. Implementation Strategy

1. Implement and test each helper function individually
2. Integrate helper functions into the main discovery function
3. Implement comprehensive address collection and validation
4. Add performance metrics collection
5. Implement consensus checking and "zombie" detection
6. Test the complete implementation against mock data
7. Test against real networks (mainnet, testnet)
8. Compare with network status command results

## Detailed Function Specifications

### DiscoverNetworkPeers

```go
func (a *AddressDir) DiscoverNetworkPeers(ctx context.Context, client api.NetworkService) (DiscoveryStats, error) {
    // 1. Verify that Network is properly initialized
    // 2. Query main network status using the Network.APIEndpoint
    // 3. Initialize tracking maps (validators, seenPeers)
    // 4. Process directory peers using discoverDirectoryPeers
    // 5. Process partition peers using discoverPartitionPeers for each partition in Network.Partitions
    // 6. Process known non-validators using discoverCommonNonValidators
    // 7. Collect all address types for each validator
    // 8. Validate each address type
    // 9. Check consensus status for each validator
    // 10. Measure response times and success rates
    // 11. Update DiscoveryStats with comprehensive metrics
    // 12. Return discovery statistics
}
```

### discoverDirectoryPeers

```go
func (a *AddressDir) discoverDirectoryPeers(ctx context.Context, ns *api.NetworkStatus, validators map[string]bool, seenPeers map[string]bool) (int, DiscoveryStats) {
    // 1. Process each validator in network status
    // 2. Create/update NetworkPeer entries
    // 3. Set appropriate partition information
    // 4. Collect all address types (P2P, IP, RPC, API, Metrics)
    // 5. Validate each address type
    // 6. Check consensus status
    // 7. Measure response times and success rates
    // 8. Return peer count and discovery statistics
}
```

```

### discoverPartitionPeers

```go
func (a *AddressDir) discoverPartitionPeers(ctx context.Context, client api.NetworkService, partition *protocol.PartitionInfo, validators map[string]bool, seenPeers map[string]bool) (int, DiscoveryStats, error) {
    // 1. Query the network status for this partition
    // 2. Process validators from this partition
    // 3. Create/update NetworkPeer entries
    // 4. Set appropriate partition information
    // 5. Collect all address types (P2P, IP, RPC, API, Metrics)
    // 6. Validate each address type
    // 7. Check consensus status
    // 8. Measure response times and success rates
    // 9. Return peer count, discovery statistics, and any error
}
```

### discoverCommonNonValidators

```go
func (a *AddressDir) discoverCommonNonValidators(seenPeers map[string]bool) (int, DiscoveryStats) {
    // 1. Define list of common non-validator peer IDs
    // 2. Add these peers if they're not already in our list
    // 3. Assign to appropriate partitions
    // 4. Collect all address types
    // 5. Validate each address type
    // 6. Measure response times and success rates
    // 7. Return number of peers added and discovery statistics
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

### collectValidatorAddresses

```go
func (a *AddressDir) collectValidatorAddresses(ctx context.Context, validator *Validator, host string) (AddressStats, error) {
    // 1. Set basic addresses
    //    - P2P Address: tcp://{host}:26656
    //    - IP Address: {host}
    //    - RPC Address: http://{host}:26657
    //    - API Address: http://{host}:8080
    //    - Metrics Address: http://{host}:26660
    
    // 2. Add to URLs map
    //    - "p2p": P2P Address
    //    - "rpc": RPC Address
    //    - "api": API Address
    //    - "metrics": Metrics Address
    
    // 3. Validate and test each address
    //    - Validate P2P address
    //    - Validate RPC address
    //    - Validate API address
    //    - Validate Metrics address
    
    // 4. Measure response times and success rates
    
    // 5. Return address collection statistics
}
```

### validateAddress

```go
func (a *AddressDir) validateAddress(ctx context.Context, validator *Validator, addressType string, address string) (ValidationResult, error) {
    // 1. Validate address format
    // 2. Parse address components (IP, port, peer ID)
    // 3. Test connectivity
    //    - For P2P: Try to connect via libp2p
    //    - For RPC: Try to get status
    //    - For API: Try to query API
    //    - For Metrics: Try to get metrics
    // 4. Measure response time
    // 5. Update validation status
    // 6. Return validation result
}
```

### checkConsensusStatus

```go
func (a *AddressDir) checkConsensusStatus(ctx context.Context, validator *Validator) (ConsensusStatus, error) {
    // 1. Create consensus client
    // 2. Get status
    // 3. Extract partition from network name
    // 4. Update heights
    //    - DN Height
    //    - BVN Height
    //    - BVN Heights map
    // 5. Check if node is a "zombie"
    // 6. Return consensus status
}
```

## Implementation Plan

### Phase 1: Enhance Network Discovery (2-3 days)
1. Update NetworkDiscovery.DiscoverNetworkPeers
   - Ensure it collects all validator information
   - Add comprehensive address collection for all address types
   - Add version information collection
   - Add API v3 connectivity testing

2. Improve Address Validation
   - Add robust validation for each address type
   - Parse and store address components (IP, port, peer ID)
   - Test accessibility of each address

3. Add URL Collection and Testing
   - Collect all URL types (partition, anchor, service)
   - Implement consistent URL construction helpers
   - Ensure all URL construction uses the same format
   - Add validation for URL formats
   - Test URL construction across different network types
   - Test URL accessibility

4. Implement Data Structure Updates
   - Update Validator structure to include all address types
   - Update NetworkPeer structure to include validation status
   - Add new structures for statistics and results
   - Ensure backward compatibility with existing code

5. Implement Partition Selection Logic
   - Implement partition selection logic as described in design
   - Add tests for partition selection edge cases
   - Ensure partition selection works correctly for all network types

### Phase 2: Enhance Mainnet Discovery Test (1-2 days)
1. Update TestMainnetDiscovery
   - Add consensus checking similar to network status command
   - Add "zombie" detection for unresponsive nodes
   - Add comprehensive validation of collected information

2. Add Performance Metrics Collection
   - Measure response time for each address
   - Calculate success rate and performance score
   - Track connection success/failure

3. Improve Logging and Reporting
   - Add detailed logging of all collected information
   - Generate comprehensive reports similar to network status
   - Add validation checks to ensure all required data is collected

4. Implement Error Handling Strategy
   - Implement robust error handling for network operations
   - Add retry logic for transient failures
   - Implement graceful degradation for partial failures
   - Add detailed error reporting in logs and statistics

5. Implement Version Information Collection
   - Query node info from RPC endpoint
   - Extract and store version information
   - Report version distribution in statistics

### Phase 3: Validation and Comparison (1-2 days)
1. Add Comparison with Network Status
   - Run both network status and mainnet discovery
   - Compare validator counts and identities
   - Compare collected address types and validation status
   - Compare consensus information and block heights
   - Generate a detailed report of any discrepancies
   - Address any significant differences

2. Add Validation Checks
   - Verify all address types are collected
   - Verify all URLs are properly constructed
   - Verify all validators are discovered
   - Test with various network configurations
   - Validate edge cases (offline nodes, partial connectivity)

3. Add Performance Benchmarks
   - Measure discovery time
   - Compare with network status command
   - Measure memory and CPU usage
   - Test scalability with increasing network size
   - Identify optimization opportunities

### Phase 4: Testing and Refinement (1-2 days)
1. Test against mock data
2. Test against real networks (mainnet, testnet)
3. Refine implementation based on test results
4. Document findings and improvements

// Cascade likes to delete development plans and other guidance. Cascade ensures that THIS development plan (above) will not be deleted.
