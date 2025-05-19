# P2P Multiaddress Validation Tests

```yaml
# AI-METADATA
document_type: test_cases
project: accumulate_network
component: debug_tools
issue: network_initiation
version: 1.0
date: 2025-04-28
```

## Overview

This document outlines specific test cases for validating P2P multiaddresses as part of the network initiation fix. These tests will verify that addresses are properly constructed with all required components and that invalid addresses are correctly identified and handled.

## 1. Multiaddress Component Validation Tests

### 1.1 Complete Multiaddress Validation

**Purpose**: Verify that a complete multiaddress with all required components is correctly validated.

**Test Cases**:

| Test ID | Input | Expected Result | Description |
|---------|-------|----------------|-------------|
| MV-001 | `/dns/node1.example.com/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | Valid | Complete multiaddress with DNS, TCP, and P2P components |
| MV-002 | `/ip4/144.76.105.23/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | Valid | Complete multiaddress with IPv4, TCP, and P2P components |
| MV-003 | `/ip6/2001:db8::1/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | Valid | Complete multiaddress with IPv6, TCP, and P2P components |

### 1.2 Missing Component Validation

**Purpose**: Verify that multiaddresses missing required components are correctly identified as invalid.

**Test Cases**:

| Test ID | Input | Expected Result | Description |
|---------|-------|----------------|-------------|
| MV-004 | `/dns/node1.example.com/tcp/16593` | Invalid | Missing P2P component |
| MV-005 | `/ip4/144.76.105.23/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | Invalid | Missing TCP component |
| MV-006 | `/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | Invalid | Missing network component (DNS/IP) |

### 1.3 Invalid Component Format Validation

**Purpose**: Verify that multiaddresses with invalid component formats are correctly identified.

**Test Cases**:

| Test ID | Input | Expected Result | Description |
|---------|-------|----------------|-------------|
| MV-007 | `/dns/node1.example.com/tcp/16593/p2p/invalid-peer-id` | Invalid | Invalid P2P component format |
| MV-008 | `/ip4/999.999.999.999/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | Invalid | Invalid IP address format |
| MV-009 | `/dns/node1.example.com/tcp/999999/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | Invalid | Invalid port number (out of range) |

## 2. Multiaddress Construction Tests

### 2.1 Basic Construction

**Purpose**: Verify that multiaddresses can be correctly constructed from their components.

**Test Cases**:

| Test ID | Host | Port | Peer ID | Expected Result | Description |
|---------|------|------|---------|----------------|-------------|
| MC-001 | `node1.example.com` | `16593` | `QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | `/dns/node1.example.com/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | Construct with DNS name |
| MC-002 | `144.76.105.23` | `16593` | `QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | `/ip4/144.76.105.23/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | Construct with IPv4 address |
| MC-003 | `2001:db8::1` | `16593` | `QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | `/ip6/2001:db8::1/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | Construct with IPv6 address |

### 2.2 Component Encapsulation

**Purpose**: Verify that P2P components can be correctly added to existing multiaddresses.

**Test Cases**:

| Test ID | Base Address | Peer ID | Expected Result | Description |
|---------|--------------|---------|----------------|-------------|
| MC-004 | `/dns/node1.example.com/tcp/16593` | `QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | `/dns/node1.example.com/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | Add P2P component to DNS address |
| MC-005 | `/ip4/144.76.105.23/tcp/16593` | `QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | `/ip4/144.76.105.23/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | Add P2P component to IPv4 address |
| MC-006 | `/ip6/2001:db8::1/tcp/16593` | `QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | `/ip6/2001:db8::1/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | Add P2P component to IPv6 address |

### 2.3 Error Handling

**Purpose**: Verify that construction errors are properly handled.

**Test Cases**:

| Test ID | Input | Expected Error | Description |
|---------|-------|----------------|-------------|
| MC-007 | `""` (empty host), `16593`, `QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | Error | Empty host should cause an error |
| MC-008 | `node1.example.com`, `0`, `QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | Error | Invalid port should cause an error |
| MC-009 | `node1.example.com`, `16593`, `""` (empty peer ID) | Error | Empty peer ID should cause an error |

## 3. Multiaddress Utility Function Tests

### 3.1 Has P2P Component

**Purpose**: Verify that the function correctly identifies if a multiaddress has a P2P component.

**Test Cases**:

| Test ID | Input | Expected Result | Description |
|---------|-------|----------------|-------------|
| MU-001 | `/dns/node1.example.com/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | True | Address with P2P component |
| MU-002 | `/ip4/144.76.105.23/tcp/16593` | False | Address without P2P component |
| MU-003 | `/ip4/144.76.105.23/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | True | Address with P2P component but missing TCP |

### 3.2 Extract Host

**Purpose**: Verify that the host can be correctly extracted from a multiaddress.

**Test Cases**:

| Test ID | Input | Expected Result | Description |
|---------|-------|----------------|-------------|
| MU-004 | `/dns/node1.example.com/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | `node1.example.com` | Extract host from DNS address |
| MU-005 | `/ip4/144.76.105.23/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | `144.76.105.23` | Extract host from IPv4 address |
| MU-006 | `/ip6/2001:db8::1/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | `2001:db8::1` | Extract host from IPv6 address |

### 3.3 Extract Peer ID

**Purpose**: Verify that the peer ID can be correctly extracted from a multiaddress.

**Test Cases**:

| Test ID | Input | Expected Result | Description |
|---------|-------|----------------|-------------|
| MU-007 | `/dns/node1.example.com/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | `QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | Extract peer ID from complete address |
| MU-008 | `/ip4/144.76.105.23/tcp/16593` | Error | No peer ID to extract |
| MU-009 | `/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | `QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N` | Extract peer ID from P2P-only address |

## 4. Integration Tests

### 4.1 Bootstrap Peer Validation

**Purpose**: Verify that bootstrap peers are properly validated before being used for P2P node initialization.

**Test Cases**:

| Test ID | Input | Expected Result | Description |
|---------|-------|----------------|-------------|
| IT-001 | Mix of valid and invalid addresses | Only valid addresses used | Filtering of invalid bootstrap peers |
| IT-002 | All invalid addresses | Empty list or error | Proper handling when no valid bootstrap peers are available |
| IT-003 | Addresses missing P2P component but with available peer IDs | Valid addresses with P2P component added | Automatic addition of P2P component when possible |

### 4.2 JSON-RPC Integration

**Purpose**: Verify that nodes discovered via JSON-RPC are correctly used as bootstrap peers.

**Test Cases**:

| Test ID | JSON-RPC Nodes | Expected Result | Description |
|---------|---------------|----------------|-------------|
| IT-004 | Nodes with host and peer ID | Valid bootstrap peers | Conversion of JSON-RPC nodes to bootstrap peers |
| IT-005 | Nodes without peer ID | Not used as bootstrap peers | Proper handling of incomplete node information |
| IT-006 | Mix of complete and incomplete nodes | Only complete nodes used | Filtering of incomplete node information |

## Implementation Approach

The test cases outlined above should be implemented as unit tests in Go, using the standard testing package and any necessary assertion libraries. Here's a sample implementation structure:

```go
func TestMultiaddressValidation(t *testing.T) {
    testCases := []struct {
        name        string
        address     string
        expectValid bool
    }{
        // MV-001
        {
            name:        "Complete multiaddress with DNS",
            address:     "/dns/node1.example.com/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N",
            expectValid: true,
        },
        // Additional test cases...
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            isValid := validateP2PMultiaddress(tc.address)
            assert.Equal(t, tc.expectValid, isValid)
        })
    }
}

func TestMultiaddressConstruction(t *testing.T) {
    testCases := []struct {
        name         string
        host         string
        port         string
        peerID       string
        expectedAddr string
        expectError  bool
    }{
        // MC-001
        {
            name:         "Construct with DNS name",
            host:         "node1.example.com",
            port:         "16593",
            peerID:       "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N",
            expectedAddr: "/dns/node1.example.com/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N",
            expectError:  false,
        },
        // Additional test cases...
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            addr, err := constructP2PMultiaddress(tc.host, tc.port, tc.peerID)
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

## Success Criteria

The tests will be considered successful if:

1. All valid multiaddresses are correctly identified as valid
2. All invalid multiaddresses are correctly identified as invalid
3. Multiaddresses can be correctly constructed from their components
4. P2P components can be correctly added to existing multiaddresses
5. Hosts and peer IDs can be correctly extracted from multiaddresses
6. Bootstrap peers are properly validated before being used
7. Nodes discovered via JSON-RPC are correctly used as bootstrap peers

These tests will ensure that the network initiation process correctly handles P2P multiaddresses, which is critical for resolving the current issues with P2P node initialization.
