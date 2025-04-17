# Peer Discovery Implementation Guide (AI-Optimized)

## 1. Overview

This document provides a structured, machine-parsable guide for implementing peer discovery functionality in the Accumulate Network. It is specifically formatted to be easily understood by AI systems while maintaining human readability.

```metadata
{
  "document_type": "implementation_guide",
  "topic": "peer_discovery",
  "version": "1.0",
  "created_date": "2025-04-17",
  "primary_language": "go",
  "dependencies": [
    "github.com/multiformats/go-multiaddr",
    "net/url",
    "log"
  ]
}
```

## 2. Core Components

### 2.1 PeerDiscovery Struct

```go
// PeerDiscovery provides methods for extracting hosts from various address formats
type PeerDiscovery struct {
    logger *log.Logger
}

// New creates a new PeerDiscovery instance
func New(logger *log.Logger) *PeerDiscovery {
    return &PeerDiscovery{
        logger: logger,
    }
}
```

### 2.2 Host Extraction Methods

#### 2.2.1 Multiaddr Extraction

```go
// ExtractHostFromMultiaddr extracts a host from a multiaddr string
func (p *PeerDiscovery) ExtractHostFromMultiaddr(addrStr string) (string, error) {
    // Parse the multiaddress
    maddr, err := multiaddr.NewMultiaddr(addrStr)
    if err != nil {
        return "", fmt.Errorf("failed to parse multiaddr: %w", err)
    }
    
    // Extract the host component
    var host string
    multiaddr.ForEach(maddr, func(c multiaddr.Component) bool {
        switch c.Protocol().Code {
        case multiaddr.P_DNS, multiaddr.P_DNS4, multiaddr.P_DNS6, multiaddr.P_IP4, multiaddr.P_IP6:
            host = c.Value()
            return false
        }
        return true
    })
    
    if host == "" {
        return "", fmt.Errorf("no host component found in multiaddr")
    }
    
    return host, nil
}
```

#### 2.2.2 URL Extraction

```go
// ExtractHostFromURL extracts a host from a URL string
func (p *PeerDiscovery) ExtractHostFromURL(urlStr string) (string, error) {
    // Handle URLs that don't have a scheme
    if !strings.Contains(urlStr, "://") {
        urlStr = "http://" + urlStr
    }
    
    // Parse the URL
    parsedURL, err := url.Parse(urlStr)
    if err != nil {
        return "", fmt.Errorf("failed to parse URL: %w", err)
    }
    
    host := parsedURL.Hostname()
    if host == "" {
        return "", fmt.Errorf("no host found in URL")
    }
    
    return host, nil
}
```

#### 2.2.3 Validator ID Mapping

```go
// LookupValidatorHost returns a known host for a validator ID
func (p *PeerDiscovery) LookupValidatorHost(validatorID string) (string, bool) {
    // Known validator hosts
    validatorHostMap := map[string]string{
        "defidevs.acme":    "65.108.73.121",
        "lunanova.acme":    "65.108.4.175",
        "tfa.acme":         "65.108.201.154",
        "factoshi.acme":    "135.181.114.121",
        "compumatrix.acme": "65.21.231.58",
        "ertai.acme":       "65.108.201.154",
    }
    
    host, ok := validatorHostMap[validatorID]
    return host, ok
}
```

### 2.3 Combined Extraction Method

```go
// ExtractHost attempts to extract a host using all available methods
func (p *PeerDiscovery) ExtractHost(addr string) (string, string) {
    // Try multiaddr parsing first
    host, err := p.ExtractHostFromMultiaddr(addr)
    if err == nil && host != "" {
        return host, "multiaddr"
    }
    
    // Try URL parsing as fallback
    host, err = p.ExtractHostFromURL(addr)
    if err == nil && host != "" {
        return host, "url"
    }
    
    // Check validator ID mapping
    host, ok := p.LookupValidatorHost(addr)
    if ok && host != "" {
        return host, "validator_map"
    }
    
    return "", "failed"
}
```

### 2.4 RPC Endpoint Construction

```go
// GetPeerRPCEndpoint constructs an RPC endpoint from a host
func (p *PeerDiscovery) GetPeerRPCEndpoint(host string) string {
    return fmt.Sprintf("http://%s:16592", host)
}
```

## 3. Address Format Examples

```json
{
  "supported_formats": {
    "multiaddr": [
      {
        "format": "IPv4 TCP",
        "example": "/ip4/65.108.73.121/tcp/16593/p2p/12D3KooWBSEYCpvBcQUL8KPSXUQrQEAhQNFCT7fKZ44WarJkQskY",
        "expected_host": "65.108.73.121"
      },
      {
        "format": "IPv6 TCP",
        "example": "/ip6/2a01:4f9:3a:2c26::2/tcp/16593/p2p/12D3KooWRBnwN3UX8Vr7P7qN7zYAX7eCzrNLP9g5phL9jGmSvCTp",
        "expected_host": "2a01:4f9:3a:2c26::2"
      },
      {
        "format": "DNS4 TCP",
        "example": "/dns4/validator.lunanova.acme/tcp/16593/p2p/12D3KooWJ4bTHirdTFNZpCS72TAzwtdmavTBkkEXtzo6wHL25CtE",
        "expected_host": "validator.lunanova.acme"
      },
      {
        "format": "IPv4 UDP QUIC",
        "example": "/ip4/65.108.201.154/udp/16593/quic/p2p/12D3KooWRBnwN3UX8Vr7P7qN7zYAX7eCzrNLP9g5phL9jGmSvCTp",
        "expected_host": "65.108.201.154"
      }
    ],
    "url": [
      {
        "format": "Standard HTTP URL",
        "example": "http://65.108.73.121:16592",
        "expected_host": "65.108.73.121"
      },
      {
        "format": "Host:Port Format",
        "example": "validator.lunanova.acme:16592",
        "expected_host": "validator.lunanova.acme"
      },
      {
        "format": "Plain IP",
        "example": "65.108.4.175",
        "expected_host": "65.108.4.175"
      }
    ],
    "validator_id": [
      {
        "format": "Validator Name",
        "example": "defidevs.acme",
        "expected_host": "65.108.73.121"
      },
      {
        "format": "P2P-only Format",
        "example": "/p2p/defidevs.acme",
        "expected_host": "65.108.73.121",
        "note": "Handled by validator mapping after multiaddr and URL parsing fail"
      }
    ]
  }
}
```

## 4. Integration Example

```go
// Example: Integrating with AddressDir
func (a *AddressDir) GetPeerRPCEndpoint(peer *NetworkPeer) string {
    // Create a peer discovery instance
    discovery := peerdiscovery.New(a.logger)
    
    // Try to extract a host from one of the addresses
    for _, addr := range peer.Addresses {
        host, method := discovery.ExtractHost(addr)
        if host != "" {
            a.logger.Printf("Extracted host %s from %s using method %s", host, addr, method)
            return discovery.GetPeerRPCEndpoint(host)
        }
    }
    
    // If no host was found and this is a validator, try the validator ID
    if peer.IsValidator && peer.ValidatorID != "" {
        host, ok := discovery.LookupValidatorHost(peer.ValidatorID)
        if ok {
            a.logger.Printf("Found host %s for validator ID %s", host, peer.ValidatorID)
            return discovery.GetPeerRPCEndpoint(host)
        }
    }
    
    a.logger.Printf("Failed to extract host for peer %s", peer.ID)
    return ""
}
```

## 5. Testing Guide

### 5.1 Basic Test Structure

```go
func TestPeerDiscovery(t *testing.T) {
    // Create a logger
    logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
    
    // Create a new peer discovery instance
    discovery := peerdiscovery.New(logger)
    
    // Test addresses
    testAddresses := []string{
        "/ip4/65.108.73.121/tcp/16593/p2p/12D3KooWBSEYCpvBcQUL8KPSXUQrQEAhQNFCT7fKZ44WarJkQskY",
        "/ip6/2a01:4f9:3a:2c26::2/tcp/16593/p2p/12D3KooWRBnwN3UX8Vr7P7qN7zYAX7eCzrNLP9g5phL9jGmSvCTp",
        "/dns4/validator.lunanova.acme/tcp/16593/p2p/12D3KooWJ4bTHirdTFNZpCS72TAzwtdmavTBkkEXtzo6wHL25CtE",
        "/ip4/65.108.201.154/udp/16593/quic/p2p/12D3KooWRBnwN3UX8Vr7P7qN7zYAX7eCzrNLP9g5phL9jGmSvCTp",
        "/p2p/defidevs.acme",
        "validator.lunanova.acme:16592",
        "http://65.108.73.121:16592",
        "65.108.4.175",
    }
    
    // Process each address
    for _, addr := range testAddresses {
        host, method := discovery.ExtractHost(addr)
        if host != "" {
            t.Logf("✅ SUCCESS: Extracted host %s from %s using method %s", host, addr, method)
            
            // Construct endpoint
            endpoint := discovery.GetPeerRPCEndpoint(host)
            t.Logf("Constructed endpoint: %s", endpoint)
        } else {
            t.Logf("❌ FAILED: Could not extract host from %s", addr)
        }
    }
}
```

## 6. Edge Cases and Error Handling

```json
{
  "edge_cases": [
    {
      "case": "Missing scheme in URL",
      "description": "URLs without a scheme (http://, https://) need special handling",
      "solution": "Add 'http://' prefix if no scheme is present",
      "code_reference": "ExtractHostFromURL"
    },
    {
      "case": "P2P-only address",
      "description": "Addresses with only P2P component and no IP/DNS",
      "solution": "Fall back to validator ID mapping",
      "code_reference": "ExtractHost"
    },
    {
      "case": "Invalid multiaddr",
      "description": "Malformed multiaddr strings that can't be parsed",
      "solution": "Fall back to URL parsing",
      "code_reference": "ExtractHost"
    },
    {
      "case": "Empty host in URL",
      "description": "URLs that parse successfully but have no host component",
      "solution": "Return error to trigger fallback to next method",
      "code_reference": "ExtractHostFromURL"
    }
  ]
}
```

## 7. Performance Considerations

```json
{
  "performance": {
    "method_priority": [
      {
        "method": "multiaddr",
        "priority": 1,
        "reason": "Most reliable for valid P2P addresses"
      },
      {
        "method": "url",
        "priority": 2,
        "reason": "Good fallback for non-multiaddr formats"
      },
      {
        "method": "validator_map",
        "priority": 3,
        "reason": "Last resort for special cases"
      }
    ],
    "optimization_techniques": [
      {
        "technique": "Caching",
        "description": "Cache extraction results to avoid redundant processing",
        "implementation": "Use a map with the address as the key and the extracted host as the value"
      },
      {
        "technique": "Early return",
        "description": "Return as soon as a valid host is found",
        "implementation": "The ExtractHost method returns immediately after finding a valid host"
      },
      {
        "technique": "Concurrency",
        "description": "Process multiple addresses in parallel",
        "implementation": "Use goroutines for processing multiple addresses simultaneously"
      }
    ]
  }
}
```

## 8. Implementation Checklist

```markdown
- [ ] Import required dependencies
  - [ ] github.com/multiformats/go-multiaddr
  - [ ] net/url
  - [ ] log
- [ ] Create PeerDiscovery struct
- [ ] Implement ExtractHostFromMultiaddr method
- [ ] Implement ExtractHostFromURL method
- [ ] Implement LookupValidatorHost method
- [ ] Implement ExtractHost method
- [ ] Implement GetPeerRPCEndpoint method
- [ ] Add logging throughout implementation
- [ ] Create tests for all methods
- [ ] Integrate with AddressDir implementation
```

## 9. API Reference

```json
{
  "package": "peerdiscovery",
  "structs": [
    {
      "name": "PeerDiscovery",
      "fields": [
        {
          "name": "logger",
          "type": "*log.Logger",
          "description": "Logger for recording discovery process"
        }
      ],
      "methods": [
        {
          "name": "New",
          "signature": "func New(logger *log.Logger) *PeerDiscovery",
          "description": "Creates a new PeerDiscovery instance",
          "parameters": [
            {
              "name": "logger",
              "type": "*log.Logger",
              "description": "Logger for recording discovery process"
            }
          ],
          "returns": [
            {
              "type": "*PeerDiscovery",
              "description": "New PeerDiscovery instance"
            }
          ]
        },
        {
          "name": "ExtractHostFromMultiaddr",
          "signature": "func (p *PeerDiscovery) ExtractHostFromMultiaddr(addrStr string) (string, error)",
          "description": "Extracts a host from a multiaddr string",
          "parameters": [
            {
              "name": "addrStr",
              "type": "string",
              "description": "Multiaddr string to extract host from"
            }
          ],
          "returns": [
            {
              "type": "string",
              "description": "Extracted host"
            },
            {
              "type": "error",
              "description": "Error if extraction fails"
            }
          ]
        },
        {
          "name": "ExtractHostFromURL",
          "signature": "func (p *PeerDiscovery) ExtractHostFromURL(urlStr string) (string, error)",
          "description": "Extracts a host from a URL string",
          "parameters": [
            {
              "name": "urlStr",
              "type": "string",
              "description": "URL string to extract host from"
            }
          ],
          "returns": [
            {
              "type": "string",
              "description": "Extracted host"
            },
            {
              "type": "error",
              "description": "Error if extraction fails"
            }
          ]
        },
        {
          "name": "LookupValidatorHost",
          "signature": "func (p *PeerDiscovery) LookupValidatorHost(validatorID string) (string, bool)",
          "description": "Returns a known host for a validator ID",
          "parameters": [
            {
              "name": "validatorID",
              "type": "string",
              "description": "Validator ID to lookup"
            }
          ],
          "returns": [
            {
              "type": "string",
              "description": "Known host for validator"
            },
            {
              "type": "bool",
              "description": "Whether validator was found"
            }
          ]
        },
        {
          "name": "ExtractHost",
          "signature": "func (p *PeerDiscovery) ExtractHost(addr string) (string, string)",
          "description": "Attempts to extract a host using all available methods",
          "parameters": [
            {
              "name": "addr",
              "type": "string",
              "description": "Address to extract host from"
            }
          ],
          "returns": [
            {
              "type": "string",
              "description": "Extracted host"
            },
            {
              "type": "string",
              "description": "Method used for extraction"
            }
          ]
        },
        {
          "name": "GetPeerRPCEndpoint",
          "signature": "func (p *PeerDiscovery) GetPeerRPCEndpoint(host string) string",
          "description": "Constructs an RPC endpoint from a host",
          "parameters": [
            {
              "name": "host",
              "type": "string",
              "description": "Host to construct endpoint from"
            }
          ],
          "returns": [
            {
              "type": "string",
              "description": "Constructed RPC endpoint"
            }
          ]
        }
      ]
    }
  ]
}
```

## 10. Complete Implementation Example

```go
package peerdiscovery

import (
    "fmt"
    "log"
    "net/url"
    "strings"

    "github.com/multiformats/go-multiaddr"
)

// PeerDiscovery provides methods for extracting hosts from various address formats
type PeerDiscovery struct {
    logger *log.Logger
}

// New creates a new PeerDiscovery instance
func New(logger *log.Logger) *PeerDiscovery {
    return &PeerDiscovery{
        logger: logger,
    }
}

// ExtractHostFromMultiaddr extracts a host from a multiaddr string
func (p *PeerDiscovery) ExtractHostFromMultiaddr(addrStr string) (string, error) {
    if p.logger != nil {
        p.logger.Printf("Attempting to extract host from: %s", addrStr)
    }

    // Parse the multiaddress
    maddr, err := multiaddr.NewMultiaddr(addrStr)
    if err != nil {
        if p.logger != nil {
            p.logger.Printf("Failed to parse multiaddr: %v", err)
        }
        return "", fmt.Errorf("failed to parse multiaddr: %w", err)
    }

    // Extract the host component
    var host string
    multiaddr.ForEach(maddr, func(c multiaddr.Component) bool {
        switch c.Protocol().Code {
        case multiaddr.P_DNS, multiaddr.P_DNS4, multiaddr.P_DNS6, multiaddr.P_IP4, multiaddr.P_IP6:
            host = c.Value()
            if p.logger != nil {
                p.logger.Printf("Found %s component: %s", c.Protocol().Name, host)
            }
            return false
        }
        return true
    })

    if host == "" {
        if p.logger != nil {
            p.logger.Printf("No host component found in multiaddr")
        }
        return "", fmt.Errorf("no host component found in multiaddr")
    }

    return host, nil
}

// ExtractHostFromURL extracts a host from a URL string
func (p *PeerDiscovery) ExtractHostFromURL(urlStr string) (string, error) {
    if p.logger != nil {
        p.logger.Printf("Attempting to extract host from URL: %s", urlStr)
    }

    // Handle URLs that don't have a scheme
    if !strings.Contains(urlStr, "://") {
        urlStr = "http://" + urlStr
    }

    // Parse the URL
    parsedURL, err := url.Parse(urlStr)
    if err != nil {
        if p.logger != nil {
            p.logger.Printf("Failed to parse URL: %v", err)
        }
        return "", fmt.Errorf("failed to parse URL: %w", err)
    }

    host := parsedURL.Hostname()
    if host == "" {
        if p.logger != nil {
            p.logger.Printf("No host found in URL")
        }
        return "", fmt.Errorf("no host found in URL")
    }

    if p.logger != nil {
        p.logger.Printf("Extracted host from URL: %s", host)
    }
    return host, nil
}

// LookupValidatorHost returns a known host for a validator ID
func (p *PeerDiscovery) LookupValidatorHost(validatorID string) (string, bool) {
    // Known validator hosts
    validatorHostMap := map[string]string{
        "defidevs.acme":    "65.108.73.121",
        "lunanova.acme":    "65.108.4.175",
        "tfa.acme":         "65.108.201.154",
        "factoshi.acme":    "135.181.114.121",
        "compumatrix.acme": "65.21.231.58",
        "ertai.acme":       "65.108.201.154",
    }

    host, ok := validatorHostMap[validatorID]
    if ok && p.logger != nil {
        p.logger.Printf("Found known host %s for validator %s", host, validatorID)
    }
    return host, ok
}

// ExtractHost attempts to extract a host using all available methods
func (p *PeerDiscovery) ExtractHost(addr string) (string, string) {
    // Try multiaddr parsing first
    host, err := p.ExtractHostFromMultiaddr(addr)
    if err == nil && host != "" {
        return host, "multiaddr"
    }

    // Try URL parsing as fallback
    host, err = p.ExtractHostFromURL(addr)
    if err == nil && host != "" {
        return host, "url"
    }

    // Check validator ID mapping
    host, ok := p.LookupValidatorHost(addr)
    if ok && host != "" {
        return host, "validator_map"
    }

    if p.logger != nil {
        p.logger.Printf("Failed to extract host from %s using any method", addr)
    }
    return "", "failed"
}

// GetPeerRPCEndpoint constructs an RPC endpoint from a host
func (p *PeerDiscovery) GetPeerRPCEndpoint(host string) string {
    endpoint := fmt.Sprintf("http://%s:16592", host)
    if p.logger != nil {
        p.logger.Printf("Constructed RPC endpoint: %s", endpoint)
    }
    return endpoint
}
```
