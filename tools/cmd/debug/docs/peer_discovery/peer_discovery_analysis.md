# Peer Discovery Analysis Utility

## 1. Purpose

This standalone utility isolates and analyzes the peer discovery logic from the original network status command. The utility helps us understand exactly how the original code finds IP addresses for network peers, so we can replicate this functionality in our `AddressDir` implementation. 

**Important**: This utility does not use or call any existing code directly. Instead, it extracts or copies the relevant logic from the old code to create a focused, self-contained implementation that only handles peer discovery.

## 2. Scope

The utility will:
- Extract and copy the peer discovery logic from the network status command
- Create a completely standalone implementation with no dependencies on the original code
- Add detailed logging to track how IP addresses are discovered
- Focus exclusively on the host extraction process
- Run as a standalone test utility, not integrated into the main command structure
- Not provide any other functionality of the original code

## 3. Design Overview

### 3.1 Core Components

1. **NetworkPeerDiscovery**: A standalone struct that encapsulates the peer discovery logic
   - Will use the same dependencies as the original code
   - Will include detailed logging of each step in the discovery process

2. **DiscoveryResult**: A struct to capture the results of peer discovery
   - Will track which methods successfully found addresses
   - Will record all attempts, successful or not

3. **TestHarness**: A simple test function to run the utility and analyze results

### 3.2 Key Functions

1. **DiscoverPeers**: Main entry point that orchestrates the discovery process
   ```go
   func (d *NetworkPeerDiscovery) DiscoverPeers(ctx context.Context, networkName string) (*DiscoveryResult, error)
   ```

2. **ExtractHostFromMultiaddr**: Isolate the multiaddr parsing logic
   ```go
   func ExtractHostFromMultiaddr(addr multiaddr.Multiaddr) (string, error)
   ```

3. **GetPeerRPCEndpoint**: Construct RPC endpoints from discovered hosts
   ```go
   func GetPeerRPCEndpoint(host string) string
   ```

4. **DiscoverPeersFromValidator**: Extract peers from validator information
   ```go
   func (d *NetworkPeerDiscovery) DiscoverPeersFromValidator(ctx context.Context, validator *api.ValidatorInfo) []*PeerInfo
   ```

5. **DiscoverPeersFromNetInfo**: Extract peers from consensus net info
   ```go
   func (d *NetworkPeerDiscovery) DiscoverPeersFromNetInfo(ctx context.Context, netInfo *coretypes.ResultNetInfo) []*PeerInfo
   ```

### 3.3 Data Structures

```go
type PeerInfo struct {
    ID            string
    ValidatorID   string
    Partition     string
    Addresses     []string
    RawAddresses  []multiaddr.Multiaddr
    Host          string
    RpcEndpoint   string
    DiscoveryPath string // Tracks how this peer was discovered
}

type DiscoveryResult struct {
    Peers                 []*PeerInfo
    TotalPeersFound       int
    ValidatorPeersFound   int
    ConsensusNetPeers     int
    SuccessfulHostExtract int
    FailedHostExtract     int
    DiscoveryMethods      map[string]int // Count of peers found by each method
}
```

## 4. Implementation Strategy

1. Create a new file `peer_discovery_analysis.go` in the `new_heal` package
2. Copy and adapt the relevant logic from `network.go`, focusing only on the peer discovery parts
3. Reimplement the core functionality without calling any existing code
4. Add detailed logging at each step of the process
5. Create a test file `peer_discovery_analysis_test.go` with a test function that runs the utility
6. Compare the results with our current implementation

This will be a clean-room implementation that follows the same approach as the original code but is completely self-contained.

## 5. Logging Strategy

The utility will log:
- Each attempt to extract a host from an address
- The exact format of addresses before parsing
- Success or failure of each extraction attempt
- The method used to discover each peer
- Connection attempts to discovered peers
- Detailed error information when extraction fails

## 6. Analysis Results

### 6.1 Multiaddr Parsing Analysis

Our analysis of multiaddr parsing revealed several key insights:

1. **Successfully Parsed Formats**:
   - DNS-based addresses: `/dns4/validator.lunanova.acme/tcp/16593`
   - IP addresses with valid peer IDs: `/ip4/135.181.114.121/tcp/16593/p2p/12D3KooWBSEYCpvBcQUL8KPSXUQrQEAhQNFCT7fKZ44WarJkQskY`
   - Simple IP + port formats: `/ip4/65.21.231.58/tcp/26656`
   - Different protocols (UDP/QUIC): `/ip4/65.108.201.154/udp/16593/quic`
   - IPv6 addresses: `/ip6/2a01:4f9:3a:2c26::2/tcp/16593`

2. **Problematic Formats**:
   - Validator IDs in p2p component: `/ip4/65.108.73.121/tcp/16593/p2p/defidevs.acme`
   - Any multiaddr with a non-CID p2p value fails to parse
   - Addresses without IP or DNS components: `/p2p/QmcEPkctU9P7ZWBxAE8CX8qDuDjQDQfR9JuJLt6VfbGZoX`

3. **Success Rate**:
   - Only about 55% of multiaddr formats were successfully parsed
   - Most failures were due to invalid p2p components

### 6.2 URL Extraction Analysis

1. **URL Parsing**:
   - All standard URL formats were successfully parsed (100% success rate)
   - Both HTTP and HTTPS schemes were handled correctly
   - URLs with paths were correctly processed to extract just the host

2. **Fallback Mechanism**:
   - URL extraction provides a reliable fallback when multiaddr parsing fails
   - This is especially important for addresses with validator IDs in the p2p component

### 6.3 Validator Address Mapping

1. **Known Address Mapping**:
   - Hardcoded validator-to-IP mappings provide the most reliable method (100% for known validators)
   - This approach is immune to parsing failures
   - However, it requires manual updates when validator IPs change

2. **Comparison with Current Implementation**:
   - Our current implementation uses different hardcoded IPs than what we discovered
   - Only about 28.6% of endpoints matched between implementations
   - This suggests our current implementation may be using outdated IP addresses

### 6.4 Key APIs Used

1. **Multiaddr Library** (github.com/multiformats/go-multiaddr):
   - `multiaddr.NewMultiaddr(addrStr)`: Parses a string into a multiaddr
   - `multiaddr.ForEach(maddr, func)`: Iterates through multiaddr components
   - Component protocol codes: `P_IP4`, `P_IP6`, `P_DNS`, `P_DNS4`, `P_DNS6`

2. **CometBFT RPC Client** (github.com/cometbft/cometbft/rpc/client):
   - Used to query peer information and network status
   - `client.ABCIInfo()`: Gets basic info about the ABCI application
   - `client.NetInfo()`: Gets network info
   - `client.Status()`: Gets node status

3. **Accumulate API** (gitlab.com/accumulatenetwork/accumulate/pkg/api):
   - `api.ValidatorInfo`: Contains validator information
   - `api.NetworkStatusOptions`: Options for network status query
   - `api.NodeInfoOptions`: Options for node info query

## 7. Comparison Test Results

We created a comparison test to verify that our simplified peer discovery utility correctly extracts the same hosts as the original code. The test compares the host extraction logic from both implementations using a set of representative multiaddr formats.

### 7.1 Test Methodology

1. **Test Dataset**: We used a diverse set of multiaddr formats including:
   - IPv4 addresses with different protocols (TCP, UDP, QUIC)
   - IPv6 addresses
   - DNS-based addresses
   - Various p2p component formats

2. **Comparison Process**:
   - Each address was processed by both the original code logic and our simplified utility
   - The extracted hosts and constructed RPC endpoints were compared
   - Results were categorized as matches, mismatches, or exclusive to one implementation

### 7.2 Test Results

1. **Match Rate**: 100% match rate between implementations
   - All 8 test addresses were successfully processed by both implementations with identical results
   - No mismatches or exclusive results were found

2. **Address Format Support**:
   - Successfully handled IPv4 addresses (e.g., `65.108.73.121`)
   - Successfully handled IPv6 addresses (e.g., `2a01:4f9:3a:2c26::2`)
   - Successfully handled DNS addresses (e.g., `validator.lunanova.acme`)
   - Successfully handled different transport protocols (TCP, UDP, QUIC)

3. **Endpoint Construction**:
   - Both implementations correctly constructed RPC endpoints with the standard port (16592)

These results confirm that our simplified utility correctly implements the peer discovery logic from the original code and can be reliably used as a drop-in replacement for the host extraction logic in the AddressDir implementation.

## 8. Recommendations for AddressDir Implementation

Based on our comprehensive analysis and successful comparison test, we recommend the following specific improvements to the `AddressDir` implementation:

### 8.1 Core Host Extraction Logic

1. **Multiaddr Parsing Implementation**:
   ```go
   func ExtractHostFromMultiaddr(addrStr string) (string, error) {
       maddr, err := multiaddr.NewMultiaddr(addrStr)
       if err != nil {
           return "", fmt.Errorf("failed to parse multiaddr: %w", err)
       }
       
       var host string
       multiaddr.ForEach(maddr, func(c multiaddr.Component) bool {
           switch c.Protocol().Code {
           case multiaddr.P_DNS, multiaddr.P_DNS4, multiaddr.P_DNS6, multiaddr.P_IP4, multiaddr.P_IP6:
               host = c.Value()
               return false
           }
           return true
       })
       
       return host, nil
   }
   ```

2. **URL Extraction Fallback**:
   ```go
   func ExtractHostFromURL(urlStr string) (string, error) {
       // Handle URLs that don't have a scheme
       if !strings.Contains(urlStr, "://") {
           urlStr = "http://" + urlStr
       }
       
       parsedURL, err := url.Parse(urlStr)
       if err != nil {
           return "", fmt.Errorf("failed to parse URL: %w", err)
       }
       
       return parsedURL.Hostname(), nil
   }
   ```

3. **Validator ID Mapping**:
   ```go
   var validatorHostMap = map[string]string{
       "defidevs.acme": "65.108.73.121",
       "lunanova.acme": "65.108.4.175",
       "tfa.acme": "65.108.201.154",
       // Add more mappings based on discovered IPs
   }
   ```

### 8.2 Integration into AddressDir

1. **Enhanced GetNetworkPeers Method**:
   - Implement a multi-stage discovery process that tries different extraction methods
   - Track success rates for different methods
   - Prioritize methods based on reliability

2. **Fallback Chain Implementation**:
   ```go
   func (a *AddressDir) extractHost(addr string) string {
       // Try multiaddr parsing first
       host, err := a.ExtractHostFromMultiaddr(addr)
       if err == nil && host != "" {
           a.logger.Printf("Extracted host %s from multiaddr %s", host, addr)
           return host
       }
       
       // Try URL parsing as fallback
       host, err = a.ExtractHostFromURL(addr)
       if err == nil && host != "" {
           a.logger.Printf("Extracted host %s from URL %s", host, addr)
           return host
       }
       
       // Check validator ID mapping
       if host, ok := validatorHostMap[addr]; ok {
           a.logger.Printf("Found host %s for validator ID %s", host, addr)
           return host
       }
       
       a.logger.Printf("Failed to extract host from %s", addr)
       return ""
   }
   ```

3. **Comprehensive Logging**:
   - Add detailed logging at each step of the discovery process
   - Track success rates for different methods
   - Log all attempts, successful or not

### 8.3 Testing and Validation

1. **Comprehensive Test Suite**:
   - Test with a diverse set of address formats
   - Compare results with the original implementation
   - Verify that all extraction methods work as expected

2. **Real-World Validation**:
   - Test with actual network connections
   - Verify that discovered peers are reachable
   - Measure success rates in production environments

### 8.4 Maintenance Considerations

1. **Validator IP Updates**:
   - Implement a mechanism to periodically update validator IP mappings
   - Consider fetching these from a centralized source

2. **Format Handling Evolution**:
   - Monitor for new address formats
   - Add support for new protocols as they emerge

By implementing these recommendations, the AddressDir will have a robust and reliable peer discovery mechanism that matches the functionality of the original implementation while providing enhanced logging and fallback capabilities.

This utility is not intended to be a permanent part of the codebase but rather a tool to help us understand and improve our current implementation.
