# Peer Discovery Testing Guide

```yaml
# AI-METADATA
document_type: testing_guide
project: accumulate_network
component: peer_discovery
version: v4
test_types:
  - unit_tests
  - integration_tests
  - performance_tests
key_test_functions:
  - TestExtractHostFromMultiaddr
  - TestExtractHostFromURL
  - TestLookupValidatorHost
  - BenchmarkHostExtraction
dependencies:
  - github.com/stretchr/testify/assert
  - github.com/stretchr/testify/require
  - github.com/multiformats/go-multiaddr
related_files:
  - peer_discovery_analysis.md
  - ../../code_examples.md#url-normalization
  - ../../dev_v3/network_status.md
```

> **Related Topics:**
> - [Peer Discovery Analysis](peer_discovery_analysis.md)
> - [URL Handling and Normalization](../../code_examples.md#url-normalization)
> - [Multiaddress Handling](../../dev_v3/network_status.md#multiaddress-handling)
> - [URL Construction Standardization](../../dev_v3/DEVELOPMENT_PLAN.md#url-construction-standardization)

This document provides comprehensive guidance on testing the peer discovery utility to ensure it correctly extracts hosts from various address formats and constructs valid RPC endpoints.

## Test Setup

### Required Dependencies

```go
import (
    "fmt"
    "log"
    "os"
    "testing"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "gitlab.com/accumulatenetwork/accumulate/tools/cmd/debug/new_heal/peerdiscovery"
)
```

### Basic Test Structure

```go
func TestPeerDiscovery(t *testing.T) {
    // Create a logger
    logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
    
    // Create a new peer discovery instance
    discovery := peerdiscovery.New(logger)
    
    // Run tests...
}
```

## Test Cases

### 1. Multiaddr Extraction Test

This test verifies that the utility can correctly extract hosts from various multiaddr formats:

```go
func TestMultiaddrExtraction(t *testing.T) {
    logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
    discovery := peerdiscovery.New(logger)
    
    testCases := []struct {
        name     string
        addr     string
        expected string
    }{
        {
            name:     "IPv4 TCP",
            addr:     "/ip4/65.108.73.121/tcp/16593/p2p/12D3KooWBSEYCpvBcQUL8KPSXUQrQEAhQNFCT7fKZ44WarJkQskY",
            expected: "65.108.73.121",
        },
        {
            name:     "IPv6 TCP",
            addr:     "/ip6/2a01:4f9:3a:2c26::2/tcp/16593/p2p/12D3KooWRBnwN3UX8Vr7P7qN7zYAX7eCzrNLP9g5phL9jGmSvCTp",
            expected: "2a01:4f9:3a:2c26::2",
        },
        {
            name:     "DNS4 TCP",
            addr:     "/dns4/validator.lunanova.acme/tcp/16593/p2p/12D3KooWJ4bTHirdTFNZpCS72TAzwtdmavTBkkEXtzo6wHL25CtE",
            expected: "validator.lunanova.acme",
        },
        {
            name:     "IPv4 UDP QUIC",
            addr:     "/ip4/65.108.201.154/udp/16593/quic/p2p/12D3KooWRBnwN3UX8Vr7P7qN7zYAX7eCzrNLP9g5phL9jGmSvCTp",
            expected: "65.108.201.154",
        },
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            host, err := discovery.ExtractHostFromMultiaddr(tc.addr)
            require.NoError(t, err)
            assert.Equal(t, tc.expected, host)
        })
    }
}
```

### 2. URL Extraction Test

This test verifies that the utility can correctly extract hosts from various URL formats:

```go
func TestURLExtraction(t *testing.T) {
    logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
    discovery := peerdiscovery.New(logger)
    
    testCases := []struct {
        name     string
        url      string
        expected string
    }{
        {
            name:     "Standard HTTP URL",
            url:      "http://65.108.73.121:16592",
            expected: "65.108.73.121",
        },
        {
            name:     "Host:Port Format",
            url:      "validator.lunanova.acme:16592",
            expected: "validator.lunanova.acme",
        },
        {
            name:     "Plain IP",
            url:      "65.108.4.175",
            expected: "65.108.4.175",
        },
        {
            name:     "HTTPS URL",
            url:      "https://validator.tfa.acme:16592/rpc",
            expected: "validator.tfa.acme",
        },
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            host, err := discovery.ExtractHostFromURL(tc.url)
            require.NoError(t, err)
            assert.Equal(t, tc.expected, host)
        })
    }
}
```

### 3. Validator ID Lookup Test

This test verifies that the utility can correctly map validator IDs to known hosts:

```go
func TestValidatorIDLookup(t *testing.T) {
    logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
    discovery := peerdiscovery.New(logger)
    
    testCases := []struct {
        name       string
        validatorID string
        expected   string
        found      bool
    }{
        {
            name:       "Known Validator",
            validatorID: "defidevs.acme",
            expected:   "65.108.73.121",
            found:      true,
        },
        {
            name:       "Another Known Validator",
            validatorID: "lunanova.acme",
            expected:   "65.108.4.175",
            found:      true,
        },
        {
            name:       "Unknown Validator",
            validatorID: "unknown.acme",
            expected:   "",
            found:      false,
        },
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            host, found := discovery.LookupValidatorHost(tc.validatorID)
            assert.Equal(t, tc.found, found)
            assert.Equal(t, tc.expected, host)
        })
    }
}
```

### 4. Comprehensive Extraction Test

This test verifies that the utility can extract hosts from various address formats using all available methods:

```go
func TestExtractHost(t *testing.T) {
    logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
    discovery := peerdiscovery.New(logger)
    
    testCases := []struct {
        name           string
        addr           string
        expectedHost   string
        expectedMethod string
    }{
        {
            name:           "Multiaddr Format",
            addr:           "/ip4/65.108.73.121/tcp/16593/p2p/12D3KooWBSEYCpvBcQUL8KPSXUQrQEAhQNFCT7fKZ44WarJkQskY",
            expectedHost:   "65.108.73.121",
            expectedMethod: "multiaddr",
        },
        {
            name:           "URL Format",
            addr:           "http://65.108.73.121:16592",
            expectedHost:   "65.108.73.121",
            expectedMethod: "url",
        },
        {
            name:           "Validator ID",
            addr:           "defidevs.acme",
            expectedHost:   "65.108.73.121",
            expectedMethod: "validator_map",
        },
        {
            name:           "P2P-only Format",
            addr:           "/p2p/defidevs.acme",
            expectedHost:   "65.108.73.121",
            expectedMethod: "validator_map",
        },
        {
            name:           "Invalid Format",
            addr:           "invalid-format",
            expectedHost:   "",
            expectedMethod: "failed",
        },
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            host, method := discovery.ExtractHost(tc.addr)
            assert.Equal(t, tc.expectedHost, host)
            assert.Equal(t, tc.expectedMethod, method)
        })
    }
}
```

### 5. RPC Endpoint Construction Test

This test verifies that the utility correctly constructs RPC endpoints from hosts:

```go
func TestGetPeerRPCEndpoint(t *testing.T) {
    logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
    discovery := peerdiscovery.New(logger)
    
    testCases := []struct {
        name     string
        host     string
        expected string
    }{
        {
            name:     "IPv4 Host",
            host:     "65.108.73.121",
            expected: "http://65.108.73.121:16592",
        },
        {
            name:     "IPv6 Host",
            host:     "2a01:4f9:3a:2c26::2",
            expected: "http://2a01:4f9:3a:2c26::2:16592",
        },
        {
            name:     "DNS Host",
            host:     "validator.lunanova.acme",
            expected: "http://validator.lunanova.acme:16592",
        },
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            endpoint := discovery.GetPeerRPCEndpoint(tc.host)
            assert.Equal(t, tc.expected, endpoint)
        })
    }
}
```

## Integration Testing

### AddressDir Integration Test

This test demonstrates how to integrate the peer discovery utility with an AddressDir-like implementation:

```go
// NetworkPeer represents a network peer for testing
type NetworkPeer struct {
    ID          string
    IsValidator bool
    ValidatorID string
    Addresses   []string
    Host        string
    RPCEndpoint string
}

func TestAddressDirIntegration(t *testing.T) {
    logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
    discovery := peerdiscovery.New(logger)
    
    // Create test peers with various address formats
    testPeers := []NetworkPeer{
        {
            ID:          "peer1",
            IsValidator: true,
            ValidatorID: "validator1",
            Addresses: []string{
                "/ip4/65.108.73.121/tcp/16593/p2p/12D3KooWBSEYCpvBcQUL8KPSXUQrQEAhQNFCT7fKZ44WarJkQskY",
                "/dns4/validator.lunanova.acme/tcp/16593/p2p/12D3KooWJ4bTHirdTFNZpCS72TAzwtdmavTBkkEXtzo6wHL25CtE",
            },
        },
        {
            ID:          "peer2",
            IsValidator: false,
            Addresses: []string{
                "/ip4/65.108.201.154/udp/16593/quic/p2p/12D3KooWRBnwN3UX8Vr7P7qN7zYAX7eCzrNLP9g5phL9jGmSvCTp",
                "/ip6/2a01:4f9:3a:2c26::2/tcp/16593/p2p/12D3KooWRBnwN3UX8Vr7P7qN7zYAX7eCzrNLP9g5phL9jGmSvCTp",
            },
        },
        {
            ID:          "peer3",
            IsValidator: true,
            ValidatorID: "defidevs.acme",
            Addresses: []string{
                "/p2p/defidevs.acme", // This is a problematic format that should be handled by validator mapping
            },
        },
    }
    
    // Process each peer
    for i, peer := range testPeers {
        // Extract host and construct RPC endpoint
        var host string
        var method string
        
        // Try to extract a host from one of the addresses
        for _, addr := range peer.Addresses {
            // Use our peer discovery utility
            extractedHost, extractionMethod := discovery.ExtractHost(addr)
            if extractedHost != "" {
                host = extractedHost
                method = extractionMethod
                break
            }
        }
        
        // If no host was found and this is a validator, try the validator ID
        if host == "" && peer.IsValidator && peer.ValidatorID != "" {
            extractedHost, ok := discovery.LookupValidatorHost(peer.ValidatorID)
            if ok {
                host = extractedHost
                method = "validator_id_lookup"
            }
        }
        
        if host != "" {
            endpoint := discovery.GetPeerRPCEndpoint(host)
            
            // Update the peer with the extracted information
            testPeers[i].Host = host
            testPeers[i].RPCEndpoint = endpoint
            
            // Verify that the host and endpoint are not empty
            assert.NotEmpty(t, host)
            assert.NotEmpty(t, endpoint)
        }
    }
    
    // Verify that all peers have a host and RPC endpoint
    for _, peer := range testPeers {
        if peer.IsValidator {
            assert.NotEmpty(t, peer.Host, "Validator peer should have a host")
            assert.NotEmpty(t, peer.RPCEndpoint, "Validator peer should have an RPC endpoint")
        }
    }
}
```

## Performance Testing

### Benchmark Test

This test benchmarks the performance of the peer discovery utility:

```go
func BenchmarkExtractHost(b *testing.B) {
    logger := log.New(io.Discard, "", 0) // Discard logs during benchmarking
    discovery := peerdiscovery.New(logger)
    
    addresses := []string{
        "/ip4/65.108.73.121/tcp/16593/p2p/12D3KooWBSEYCpvBcQUL8KPSXUQrQEAhQNFCT7fKZ44WarJkQskY",
        "http://65.108.73.121:16592",
        "defidevs.acme",
        "/p2p/defidevs.acme",
    }
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        addr := addresses[i%len(addresses)]
        discovery.ExtractHost(addr)
    }
}
```

## Conclusion

These tests provide comprehensive coverage of the peer discovery utility's functionality and performance. By running these tests, you can ensure that the utility correctly extracts hosts from various address formats and constructs valid RPC endpoints.

When implementing the peer discovery utility in your own code, you can use these tests as a reference to ensure that your implementation meets the same standards of functionality and reliability.
