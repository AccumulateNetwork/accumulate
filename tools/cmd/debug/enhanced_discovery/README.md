# Enhanced Network Discovery

This package implements an enhanced network discovery system for the Accumulate Network. It provides a modular, testable approach to discovering network peers and validators, and populating the AddressDir structure with comprehensive information.

## Key Features

- **Slice-Based Validator Storage**: Uses slices for DNValidators and BVNValidators as required by the design document.
- **Comprehensive Network Information**: Stores all relevant network data in one place.
- **Partition Awareness**: Maintains a list of partitions for discovery.
- **Network-Specific Behavior**: Allows for different handling of mainnet vs. other networks.
- **API Endpoint Management**: Stores the appropriate API endpoint for the network.
- **Comprehensive Address Collection**: Collects all address types for each validator.
- **Performance Metrics**: Tracks response times and success rates for each address.
- **URL Standardization**: Ensures consistent URL construction across components.
- **Consensus Status**: Checks and stores consensus status for each validator.
- **Version Information**: Collects and stores software version information for each validator.
- **API v3 Connectivity**: Tests and reports on API v3 connectivity status.
- **Error Handling**: Provides robust error handling and recovery mechanisms.

## Core Structures

### AddressDir

The AddressDir structure is the central data store for network discovery. It maintains lists of validators, network peers, and discovery statistics.

```go
type AddressDir struct {
    // DNValidators is a list of validators in the Directory Network
    DNValidators []Validator
    
    // BVNValidators is a list of lists of validators in BVNs
    BVNValidators [][]Validator
    
    // NetworkPeers is a map of peer ID to NetworkPeer
    NetworkPeers map[string]NetworkPeer
    
    // Network information
    NetworkInfo *NetworkInfo
    
    // Statistics for peer discovery
    DiscoveryStats *DiscoveryStats
    
    // Problem nodes by address with timestamp
    ProblemNodes map[string]time.Time
}
```

### Validator

The Validator structure represents a validator in the network with comprehensive information.

```go
type Validator struct {
    // Unique peer identifier for the validator
    PeerID string
    
    // Name or description of the validator
    Name string
    
    // Partition information
    PartitionID   string
    PartitionType string
    
    // Different address types for this validator
    P2PAddress     string
    IPAddress      string
    RPCAddress     string
    APIAddress     string
    MetricsAddress string
    
    // URLs associated with this validator
    URLs map[string]string
    
    // Heights for different networks
    DNHeight  uint64
    BVNHeight uint64
    BVNHeights map[string]uint64
    
    // Additional fields for enhanced discovery
    IsInDN      bool
    BVNs        []string
    APIV3Status bool
    Version     string
    IsZombie    bool
}
```

### NetworkInfo

The NetworkInfo structure contains information about a network and its partitions.

```go
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
```

### DiscoveryStats

The DiscoveryStats structure contains statistics about peer discovery operations.

```go
type DiscoveryStats struct {
    // Total number of peers discovered
    TotalPeers int
    
    // Number of validators discovered
    TotalValidators int
    
    // Number of validators by partition
    DNValidators  int
    BVNValidators map[string]int
    
    // Address type statistics
    AddressTypeStats map[string]AddressTypeStats
    
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
}
```

## Core Functions

### NewAddressDir

Creates a new address directory with initialized collections.

```go
func NewAddressDir() *AddressDir
```

### NewNetworkDiscovery

Creates a new network discovery instance with the specified address directory and logger.

```go
func NewNetworkDiscovery(addressDir *AddressDir, logger *log.Logger) *NetworkDiscoveryImpl
```

### InitializeNetwork

Initializes the network information for the specified network.

```go
func (nd *NetworkDiscoveryImpl) InitializeNetwork(network string) error
```

### DiscoverNetworkPeers

Discovers the network and updates the address directory.

```go
func (nd *NetworkDiscoveryImpl) DiscoverNetworkPeers(ctx context.Context, client api.NetworkService) (DiscoveryStats, error)
```

### UpdateNetworkState

Updates the network state without a full discovery operation.

```go
func (nd *NetworkDiscoveryImpl) UpdateNetworkState(ctx context.Context, client api.NetworkService) (DiscoveryStats, error)
```

### ValidateNetwork

Validates the network by checking each validator.

```go
func (nd *NetworkDiscoveryImpl) ValidateNetwork(ctx context.Context) (DiscoveryStats, error)
```

## Helper Functions

### collectValidatorAddresses

Collects all address types for a validator.

```go
func (nd *NetworkDiscoveryImpl) collectValidatorAddresses(ctx context.Context, validator *Validator, validatorID string) (AddressStats, error)
```

### validateAddress

Validates a specific address type.

```go
func (nd *NetworkDiscoveryImpl) validateAddress(ctx context.Context, validator *Validator, addressType string, address string) (ValidationResult, error)
```

### collectVersionInfo

Collects version information for a validator.

```go
func (nd *NetworkDiscoveryImpl) collectVersionInfo(ctx context.Context, validator *Validator) error
```

### checkConsensusStatus

Checks the consensus status of a validator.

```go
func (nd *NetworkDiscoveryImpl) checkConsensusStatus(ctx context.Context, validator *Validator) (ConsensusStatus, error)
```

### testAPIv3Connectivity

Tests API v3 connectivity for a validator.

```go
func (nd *NetworkDiscoveryImpl) testAPIv3Connectivity(ctx context.Context, validator *Validator) (APIv3Status, error)
```

### standardizeURL

Standardizes URLs for different types.

```go
func (nd *NetworkDiscoveryImpl) standardizeURL(urlType string, partitionID string, baseURL string) string
```

## URL Standardization

The implementation provides consistent URL construction for different URL types:

### Partition URLs

For partition URLs, the implementation uses the raw partition URL format:
- DN: `acc://dn.acme`
- BVN: `acc://bvn-Apollo.acme`

### Anchor URLs

For anchor URLs, the implementation uses the heal_anchor.go approach:
- DN anchors: `acc://dn.acme/anchors`
- BVN anchors: `acc://dn.acme/anchors/Apollo`

## Usage Examples

### Basic Network Discovery

```go
// Create an address directory
addressDir := enhanced_discovery.NewAddressDir()

// Create a logger
logger := log.New(os.Stdout, "[DISCOVERY] ", log.LstdFlags)

// Create a network discovery instance
discovery := enhanced_discovery.NewNetworkDiscovery(addressDir, logger)

// Initialize for mainnet
err := discovery.InitializeNetwork("mainnet")
if err != nil {
    log.Fatalf("Failed to initialize network: %v", err)
}

// Discover network peers
ctx := context.Background()
stats, err := discovery.DiscoverNetworkPeers(ctx, nil)
if err != nil {
    log.Fatalf("Failed to discover network peers: %v", err)
}

// Print discovery statistics
fmt.Printf("Discovery Statistics:\n%s", enhanced_discovery.FormatDiscoveryStats(stats))

// Access validators
dnValidators := addressDir.GetDNValidators()
for _, validator := range dnValidators {
    fmt.Printf("DN Validator: %s (%s)\n", validator.Name, validator.PeerID)
}

// Access BVN validators
bvnValidators := addressDir.GetBVNValidators("Apollo")
for _, validator := range bvnValidators {
    fmt.Printf("BVN Validator: %s (%s)\n", validator.Name, validator.PeerID)
}
```

### Network Validation

```go
// Create an address directory with existing validators
addressDir := enhanced_discovery.CreateTestAddressDir()

// Create a network discovery instance
discovery := enhanced_discovery.NewNetworkDiscovery(addressDir, logger)

// Initialize for testnet
err := discovery.InitializeNetwork("testnet")
if err != nil {
    log.Fatalf("Failed to initialize network: %v", err)
}

// Validate the network
ctx := context.Background()
stats, err := discovery.ValidateNetwork(ctx)
if err != nil {
    log.Fatalf("Failed to validate network: %v", err)
}

// Print validation statistics
fmt.Printf("Validation Statistics:\n%s", enhanced_discovery.FormatDiscoveryStats(stats))
```

## Testing

The implementation includes comprehensive tests for all key components:

- **Unit Tests**: Test individual components and functions.
- **Integration Tests**: Test the interaction between components.
- **Edge Case Tests**: Test error handling and edge cases.
- **URL Standardization Tests**: Test URL construction for different URL types.

## Design Decisions

### Slice-Based Validator Storage

The implementation uses slices for DNValidators and BVNValidators as required by the design document. This is a critical requirement for compatibility with existing code.

### URL Standardization

The implementation standardizes URLs for different types to ensure consistent URL construction across components. This is important for anchor healing and other operations.

### Comprehensive Address Collection

The implementation collects all address types for each validator to provide a complete view of the network. This includes P2P, IP, RPC, API, and Metrics addresses.

### Performance Metrics

The implementation tracks response times and success rates for each address type to provide insights into network performance. This helps identify performance issues and optimize network operations.

### Error Handling

The implementation includes robust error handling to ensure that the discovery process continues even when some operations fail. This is important for resilience in a distributed network.

## Future Enhancements

Potential future enhancements for the implementation:

1. **Advanced Zombie Detection**: Implement more sophisticated algorithms for detecting zombie nodes.
2. **Network Visualization**: Add support for generating network visualizations.
3. **Historical Data**: Store historical data for trend analysis.
4. **Automated Healing**: Implement automated healing for network issues.
5. **Advanced Metrics**: Add more advanced performance metrics.

## References

- [Accumulate Network Documentation](https://docs.accumulatenetwork.io/)
- [Network Discovery Design Document](./docs/dev_v4/address2_network_discovery.md)
- [Address2 Design Document](./docs/dev_v4/address2_design.md)
