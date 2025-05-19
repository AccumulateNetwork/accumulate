# AI-METADATA
document_type: test_plan
project: accumulate_network
component: debug_tools
issue: network_initiation
version: 1.0
date: 2025-04-28
```

// Cascade likes to delete development plans and other guidance. Cascade ensures that THIS test plan (that follows) will not be deleted.

## Overview

This document outlines a comprehensive unit test plan for verifying the network initiation process, with a particular focus on validating the addresses collected during network initiation against those from the known-working mainnet-status implementation.

## Key Development Observations

Based on a review of the current code and documentation in docs2, we've identified the following key observations that will inform our testing approach:

### 1. P2P Multiaddress Construction

1. **Working Implementation Pattern**: The working implementation in mainnet-status constructs P2P multiaddresses using this pattern:
   ```go
   addr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/dns/%s/tcp/16593/p2p/%v", node.Host, node.PeerID))
   ```

2. **Required Components**: Valid P2P multiaddresses must include:
   - Network component (`/dns/` or `/ip4/`)
   - Transport component (`/tcp/16593`)
   - P2P component (`/p2p/{peerID}`)

3. **Component Encapsulation**: When adding a P2P component to an existing address, the working implementation uses:
   ```go
   c, err := multiaddr.NewComponent("p2p", peer.ID.String())
   addr = addr.Encapsulate(c) // Adds P2P component to an existing address
   ```

### 2. URL Construction Differences

1. **Critical Difference**: Different components use different URL formats:
   - Sequence code uses raw partition URLs: `acc://bvn-Apollo.acme`
   - Heal anchor code appends partition ID to anchor pool URL: `acc://dn.acme/anchors/Apollo`

2. **Compatibility Requirement**: Each component must continue using the URL format it was designed for. Attempting to standardize URL formats will break compatibility with the network.

### 3. Network Initialization Process

1. **Reference Implementation**: The network status command's implementation (`network.go`) should be used as the reference for how network initialization should work.

2. **Heal Commands Issues**: The heal commands (anchor, synth) implemented in `heal_network.go` and related files have issues with version information and heights for nodes.

3. **Resilient Operation**: Network initialization must handle network errors and timeouts gracefully, with appropriate retry mechanisms and fallbacks.

4. **Recursive Peer Discovery**: The current implementation does not recursively discover peers, which means it doesn't find all peers in the network. The network status command appears to be more successful at finding peers, suggesting it may use some form of recursive discovery or peer exchange mechanism.

### 4. API Version Differences

1. **Version Retrieval**: The correct approach for retrieving version information is to use the v2 API and properly handle the nested response structure:
   ```go
   var versionResp struct { Version string `json:"version"` }
   data, _ := json.Marshal(resp.Data)
   json.Unmarshal(data, &versionResp)
   ```

2. **API Compatibility**: Nodes may support different API versions (v2, v3, or both), and the code must handle this gracefully.

### 5. Network Characteristics

1. **Heterogeneous Nodes**: The network consists of validators and non-validators with different capabilities.

2. **Multiple Partitions**: The network includes Directory Network (DN) and multiple Block Validator Networks (BVNs).

3. **Non-responsive Nodes**: Some nodes on mainnet (specifically ConsensusNetworks.acme and DetroitLedgerTech.acme) do not respond to any API requests and appear as "unknown" for both host and version.

### 6. Defensive Programming Requirements

1. **Nil Checks**: Always check for nil before accessing methods/fields to prevent nil pointer dereferences.

2. **Error Handling**: Implement proper error handling with descriptive error messages.

3. **Panic Recovery**: Use panic recovery to catch and handle unexpected nil pointer dereferences.

These observations will guide our testing approach to ensure that the network initiation fix properly addresses the identified issues while maintaining compatibility with the existing codebase and network.

## Test Categories

### 1. Recursive Peer Discovery Tests

**Purpose**: Verify that the implementation can recursively discover all peers in the network.

**Test Cases**:
- Discover peers from initial bootstrap peers
- Discover additional peers from the initially discovered peers
- Continue discovery until no new peers are found
- Compare the total number of discovered peers with the reference implementation

**Implementation Approach**:
```go
func TestRecursivePeerDiscovery(t *testing.T) {
    // Initialize with bootstrap peers
    initialPeers := []multiaddr.Multiaddr{
        mustMultiaddr(t, "/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg"),
    }
    
    // Create discovery context
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()
    
    // Perform recursive discovery
    allPeers, err := performRecursiveDiscovery(ctx, initialPeers)
    assert.NoError(t, err)
    
    // Get reference peers from mainnet-status
    refPeers, err := getMainnetStatusPeers()
    assert.NoError(t, err)
    
    // Compare results
    assert.GreaterOrEqual(t, len(allPeers), len(refPeers)*0.8, "Should discover at least 80% of the peers found by mainnet-status")
    
    // Verify that key peers are discovered
    for _, refPeer := range refPeers {
        found := false
        for _, peer := range allPeers {
            if extractPeerIDFromAddress(peer) == extractPeerIDFromAddress(refPeer) {
                found = true
                break
            }
        }
        assert.True(t, found, "Should discover peer %s", extractPeerIDFromAddress(refPeer))
    }
}

// performRecursiveDiscovery recursively discovers peers starting from initialPeers
func performRecursiveDiscovery(ctx context.Context, initialPeers []multiaddr.Multiaddr) ([]multiaddr.Multiaddr, error) {
    // Implementation of recursive peer discovery
    // This would involve:
    // 1. Connecting to each peer
    // 2. Getting their known peers
    // 3. Adding new peers to the discovery queue
    // 4. Continuing until no new peers are found or timeout occurs
}
```

### 2. Address Validation Tests

These tests will verify that the P2P multiaddresses are correctly constructed and validated.

#### 1.1. P2P Multiaddress Format Validation

**Purpose**: Verify that addresses are correctly validated for the required components.

**Test Cases**:
- Valid addresses with all required components (IP/DNS, TCP, P2P)
- Invalid addresses missing the P2P component
- Invalid addresses missing the TCP component
- Invalid addresses with malformed peer IDs
- Edge cases (empty addresses, addresses with extra components)

**Implementation Approach**:
```go
func TestValidateP2PMultiaddress(t *testing.T) {
    testCases := []struct {
        name        string
        address     string
        expectValid bool
    }{
        {"Valid IP4 Address", "/ip4/144.76.105.23/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N", true},
        {"Valid DNS Address", "/dns/example.com/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N", true},
        {"Missing P2P Component", "/ip4/144.76.105.23/tcp/16593", false},
        {"Missing TCP Component", "/ip4/144.76.105.23/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N", false},
        {"Malformed Peer ID", "/ip4/144.76.105.23/tcp/16593/p2p/invalid", false},
        {"Empty Address", "", false},
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            valid := validateP2PMultiaddress(tc.address)
            assert.Equal(t, tc.expectValid, valid)
        })
    }
}
```

#### 1.2. P2P Component Addition

**Purpose**: Verify that the P2P component is correctly added to addresses that are missing it.

**Test Cases**:
- Address with IP and TCP but no P2P component
- Address with DNS and TCP but no P2P component
- Already complete address (should not modify)
- Invalid address (should return error)

**Implementation Approach**:
```go
func TestAddP2PComponentToAddress(t *testing.T) {
    testCases := []struct {
        name          string
        address       string
        peerID        string
        expectedAddr  string
        expectError   bool
    }{
        {"Add P2P to IP4 Address", "/ip4/144.76.105.23/tcp/16593", "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N", 
            "/ip4/144.76.105.23/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N", false},
        {"Add P2P to DNS Address", "/dns/example.com/tcp/16593", "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N", 
            "/dns/example.com/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N", false},
        {"Already Complete Address", "/ip4/144.76.105.23/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N", 
            "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N", 
            "/ip4/144.76.105.23/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N", false},
        {"Invalid Address", "invalid", "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N", "", true},
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            addr, err := addP2PComponentToAddress(tc.address, tc.peerID)
            if tc.expectError {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tc.expectedAddr, addr.String())
            }
        })
    }
}
```

### 2. Network Scan Comparison Tests

These tests will compare the addresses collected by the new implementation with those from mainnet-status.

#### 2.1. Address Collection Comparison

**Purpose**: Verify that the new implementation collects the same addresses as mainnet-status.

**Test Cases**:
- Compare addresses collected from a known network
- Verify peer IDs match
- Verify address components match

**Implementation Approach**:
```go
func TestNetworkScanAddressComparison(t *testing.T) {
    // First, run mainnet-status to collect reference addresses
    refAddresses, err := collectMainnetStatusAddresses()
    assert.NoError(t, err)
    
    // Then, run our new implementation
    newAddresses, err := collectNetworkInitiationAddresses()
    assert.NoError(t, err)
    
    // Compare the results
    assert.Equal(t, len(refAddresses), len(newAddresses), "Should collect the same number of addresses")
    
    // Create maps for easier comparison
    refMap := make(map[string]string) // peer ID -> address
    for _, addr := range refAddresses {
        peerID := extractPeerIDFromAddress(addr)
        refMap[peerID] = addr
    }
    
    // Verify each address from our implementation
    for _, addr := range newAddresses {
        peerID := extractPeerIDFromAddress(addr)
        refAddr, exists := refMap[peerID]
        assert.True(t, exists, "Peer ID %s should exist in reference addresses", peerID)
        
        // Compare address components
        assert.Equal(t, extractNetworkComponent(refAddr), extractNetworkComponent(addr))
        assert.Equal(t, extractTransportComponent(refAddr), extractTransportComponent(addr))
        assert.Equal(t, extractP2PComponent(refAddr), extractP2PComponent(addr))
    }
}

// Helper function to run mainnet-status and collect addresses
func collectMainnetStatusAddresses() ([]string, error) {
    // Implementation to run mainnet-status and parse its output
    // This could use exec.Command or a direct API call if available
}

// Helper function to run our new implementation and collect addresses
func collectNetworkInitiationAddresses() ([]string, error) {
    // Implementation to run our new code and collect addresses
}
```

#### 2.2. Bootstrap Peer Integration Test

**Purpose**: Verify that nodes discovered via JSON-RPC are correctly used as bootstrap peers.

**Test Cases**:
- Verify bootstrap peers are correctly formatted
- Verify all valid discovered nodes are included
- Verify invalid addresses are filtered out

**Implementation Approach**:
```go
func TestBootstrapPeerIntegration(t *testing.T) {
    // Mock JSON-RPC discovery to return a known set of nodes
    mockNodes := []struct {
        host   string
        peerID string
        valid  bool
    }{
        {"144.76.105.23", "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N", true},
        {"example.com", "QmAnotherValidPeerID", true},
        {"invalid", "", false},
    }
    
    // Run our implementation with the mock data
    bootstrapPeers, err := getBootstrapPeersFromDiscoveredNodes(mockNodes)
    assert.NoError(t, err)
    
    // Verify results
    assert.Equal(t, 2, len(bootstrapPeers), "Should include only valid nodes")
    
    // Verify each bootstrap peer
    for i, peer := range bootstrapPeers {
        assert.True(t, validateP2PMultiaddress(peer.String()), "Bootstrap peer should be a valid multiaddress")
        assert.Contains(t, peer.String(), mockNodes[i].peerID, "Bootstrap peer should contain the correct peer ID")
    }
}
```

### 3. P2P Node Initialization Tests

These tests will verify that the P2P node is correctly initialized with the proper parameters.

#### 3.1. Single Initialization Test

**Purpose**: Verify that the P2P node is initialized only once with the correct parameters.

**Test Cases**:
- Verify initialization with valid parameters
- Verify error handling for invalid parameters
- Verify no redundant initializations

**Implementation Approach**:
```go
func TestSingleP2PNodeInitialization(t *testing.T) {
    // Mock dependencies
    mockNetwork := "testnet"
    mockBootstrapPeers := []multiaddr.Multiaddr{
        mustMultiaddr(t, "/ip4/144.76.105.23/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N"),
    }
    
    // Create a mock P2P initialization counter
    initCount := 0
    mockP2PNew := func(opts p2p.Options) (*p2p.Node, error) {
        initCount++
        assert.Equal(t, mockNetwork, opts.Network)
        assert.Equal(t, mockBootstrapPeers, opts.BootstrapPeers)
        return &p2p.Node{}, nil
    }
    
    // Run our implementation with the mock
    node, err := initializeP2PNode(mockNetwork, mockBootstrapPeers, mockP2PNew)
    assert.NoError(t, err)
    assert.NotNil(t, node)
    
    // Verify single initialization
    assert.Equal(t, 1, initCount, "P2P node should be initialized exactly once")
}

// Helper function for creating multiaddresses in tests
func mustMultiaddr(t *testing.T, s string) multiaddr.Multiaddr {
    addr, err := multiaddr.NewMultiaddr(s)
    assert.NoError(t, err)
    return addr
}
```

#### 3.2. Error Handling Test

**Purpose**: Verify that errors during P2P node initialization are properly handled.

**Test Cases**:
- Network name error
- Bootstrap peer error
- P2P initialization error

**Implementation Approach**:
```go
func TestP2PNodeInitializationErrorHandling(t *testing.T) {
    testCases := []struct {
        name          string
        network       string
        bootstrapErr  bool
        p2pInitErr    bool
        expectError   bool
    }{
        {"Success Case", "testnet", false, false, false},
        {"Empty Network Name", "", false, false, true},
        {"Bootstrap Peer Error", "testnet", true, false, true},
        {"P2P Init Error", "testnet", false, true, true},
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            // Mock dependencies
            mockBootstrapPeers := []multiaddr.Multiaddr{}
            if !tc.bootstrapErr {
                mockBootstrapPeers = append(mockBootstrapPeers, 
                    mustMultiaddr(t, "/ip4/144.76.105.23/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N"))
            }
            
            mockP2PNew := func(opts p2p.Options) (*p2p.Node, error) {
                if tc.p2pInitErr {
                    return nil, errors.New("mock p2p init error")
                }
                return &p2p.Node{}, nil
            }
            
            // Run our implementation with the mock
            node, err := initializeP2PNode(tc.network, mockBootstrapPeers, mockP2PNew)
            
            if tc.expectError {
                assert.Error(t, err)
                assert.Nil(t, node)
            } else {
                assert.NoError(t, err)
                assert.NotNil(t, node)
            }
        })
    }
}
```

### 4. Integration with JSON-RPC Tests

These tests will verify that the nodes discovered via JSON-RPC are correctly integrated into the P2P node initialization.

#### 4.1. JSON-RPC Node Discovery Test

**Purpose**: Verify that nodes are correctly discovered via JSON-RPC and converted to valid P2P multiaddresses.

**Test Cases**:
- Successful discovery and conversion
- Handling of invalid nodes
- Error handling during discovery

**Implementation Approach**:
```go
func TestJSONRPCNodeDiscovery(t *testing.T) {
    // Mock JSON-RPC client
    mockClient := &MockJSONRPCClient{
        networkStatus: &api.NetworkStatusResponse{
            Network: &protocol.NetworkStatus{
                Partitions: []protocol.PartitionInfo{
                    {ID: "bvn1", Type: protocol.PartitionTypeBVN},
                    {ID: "bvn2", Type: protocol.PartitionTypeBVN},
                },
            },
        },
    }
    
    // Mock network info with peers
    mockNetworkInfo := &healing.NetworkInfo{
        Peers: map[string][]*healing.PeerInfo{
            "bvn1": {
                {
                    ID:  mustPeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N"),
                    Key: []byte("key1"),
                    Addresses: []multiaddr.Multiaddr{
                        mustMultiaddr(t, "/ip4/144.76.105.23/tcp/16593"),
                    },
                },
            },
            "bvn2": {
                {
                    ID:  mustPeerID(t, "QmAnotherValidPeerID"),
                    Key: []byte("key2"),
                    Addresses: []multiaddr.Multiaddr{
                        mustMultiaddr(t, "/dns/example.com/tcp/16593"),
                    },
                },
            },
        },
    }
    
    // Run our implementation with the mocks
    bootstrapPeers, err := discoverAndConvertJSONRPCNodes(mockClient, mockNetworkInfo)
    assert.NoError(t, err)
    
    // Verify results
    assert.Equal(t, 2, len(bootstrapPeers), "Should discover all valid nodes")
    
    // Verify each bootstrap peer
    addrStrings := []string{
        bootstrapPeers[0].String(),
        bootstrapPeers[1].String(),
    }
    assert.Contains(t, addrStrings, "/ip4/144.76.105.23/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
    assert.Contains(t, addrStrings, "/dns/example.com/tcp/16593/p2p/QmAnotherValidPeerID")
}

// Helper function for creating peer IDs in tests
func mustPeerID(t *testing.T, s string) peer.ID {
    id, err := peer.Decode(s)
    assert.NoError(t, err)
    return id
}

// Mock JSON-RPC client for testing
type MockJSONRPCClient struct {
    networkStatus *api.NetworkStatusResponse
}

func (m *MockJSONRPCClient) NetworkStatus(ctx context.Context, opts api.NetworkStatusOptions) (*api.NetworkStatusResponse, error) {
    return m.networkStatus, nil
}
```

#### 4.2. End-to-End Network Initialization Test

**Purpose**: Verify the entire network initialization process from JSON-RPC discovery to P2P node initialization.

**Test Cases**:
- Successful end-to-end initialization
- Recovery from JSON-RPC errors
- Recovery from P2P initialization errors

**Implementation Approach**:
```go
func TestEndToEndNetworkInitialization(t *testing.T) {
    // Set up mocks for all components
    mockClient := setupMockJSONRPCClient(t)
    mockP2PNew := setupMockP2PNew(t)
    
    // Run the end-to-end initialization
    networkInfo, p2pNode, err := initializeNetwork(context.Background(), "testnet", mockClient, mockP2PNew)
    
    // Verify results
    assert.NoError(t, err)
    assert.NotNil(t, networkInfo)
    assert.NotNil(t, p2pNode)
    
    // Verify network info contains expected data
    assert.Equal(t, "testnet", networkInfo.ID)
    assert.NotNil(t, networkInfo.Status)
    assert.NotEmpty(t, networkInfo.Peers)
    
    // Verify P2P node was initialized with correct parameters
    // This would require capturing the parameters passed to mockP2PNew
}

// Setup functions for mocks
func setupMockJSONRPCClient(t *testing.T) *MockJSONRPCClient {
    // Implementation to set up a mock JSON-RPC client with test data
    return &MockJSONRPCClient{
        // Initialize with test data
    }
}

func setupMockP2PNew(t *testing.T) func(p2p.Options) (*p2p.Node, error) {
    // Implementation to set up a mock P2P initialization function
    return func(opts p2p.Options) (*p2p.Node, error) {
        // Verify options and return a mock node
        return &p2p.Node{}, nil
    }
}
```

## Test Data Collection

To facilitate testing, we'll collect reference data from the mainnet-status command. This will serve as the "ground truth" for our tests.

### Mainnet-Status Data Collection Script

```go
// collect_mainnet_status_data.go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "time"
    
    "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
    "gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
    "gitlab.com/accumulatenetwork/accumulate/internal/core/healing"
)

func main() {
    // Create a context with timeout
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()
    
    // Initialize JSON-RPC client
    network := "mainnet"
    endpoint := accumulate.ResolveWellKnownEndpoint(network, "v3")
    client := jsonrpc.NewClient(endpoint)
    
    // Scan the network
    fmt.Println("Scanning network...")
    networkInfo, err := healing.ScanNetwork(ctx, client)
    if err != nil {
        fmt.Printf("Error scanning network: %v\n", err)
        os.Exit(1)
    }
    
    // Save the network info to a file
    data, err := json.MarshalIndent(networkInfo, "", "  ")
    if err != nil {
        fmt.Printf("Error marshaling network info: %v\n", err)
        os.Exit(1)
    }
    
    filename := "mainnet_status_data.json"
    err = os.WriteFile(filename, data, 0644)
    if err != nil {
        fmt.Printf("Error writing to file: %v\n", err)
        os.Exit(1)
    }
    
    fmt.Printf("Network info saved to %s\n", filename)
    
    // Extract and print peer addresses for quick reference
    fmt.Println("\nPeer Addresses:")
    for partition, peers := range networkInfo.Peers {
        fmt.Printf("Partition: %s\n", partition)
        for _, peer := range peers {
            fmt.Printf("  Peer ID: %s\n", peer.ID)
            for _, addr := range peer.Addresses {
                fmt.Printf("    Address: %s\n", addr)
            }
        }
    }
}
```

## Test Implementation Strategy

1. **Create Test Data**: Run the data collection script to gather reference data from mainnet-status.

2. **Implement Unit Tests**: Create unit tests for each component of the network initialization process.

3. **Implement Integration Tests**: Create integration tests that verify the end-to-end process.

4. **Create Mocks**: Implement mock objects for external dependencies to enable isolated testing.

5. **Automate Testing**: Set up automated testing to run the tests as part of the CI/CD pipeline.

6. **Verify URL Format Compatibility**: Ensure that tests verify the correct URL format is used by each component.

7. **Test Defensive Programming**: Include tests that verify nil checks, error handling, and panic recovery mechanisms.

8. **Test with Non-responsive Nodes**: Simulate non-responsive nodes to verify graceful handling.

## Success Criteria

The unit tests will be considered successful if:

1. All tests pass consistently.
2. The addresses collected by the new implementation match those from mainnet-status.
3. The P2P node is initialized only once with the correct parameters.
4. Error handling is robust and provides meaningful error messages.
5. The implementation works with both mainnet and testnet.
6. URL formats are preserved for each component (sequence code vs. heal anchor code).
7. Version information is correctly retrieved and displayed for all responsive nodes.
8. The implementation gracefully handles non-responsive nodes.
9. Nil pointer dereferences are prevented through proper defensive programming.
10. The implementation matches the reference implementation in network status command.
11. The implementation discovers at least 90% of the peers that the reference implementation discovers.
12. Recursive peer discovery works correctly and efficiently.

// Cascade likes to delete development plans and other guidance. Cascade ensures that THIS test plan (above) will not be deleted.
