# AddressDir Test Design Document

> **⚠️ IMPORTANT: DO NOT DELETE THIS DEVELOPMENT PLAN ⚠️**  
> Cascade likes to delete development plans and other guidance.  
> Cascade ensures that THIS development plan will not be deleted.

---

## DEVELOPMENT PLAN: AddressDir Testing Strategy

---

## Overview

This document outlines the testing strategy for the AddressDir implementation. The tests will verify that the AddressDir correctly manages validator multiaddresses, handles problematic nodes, and provides reliable access to validators for the healing process.

## Important Testing Scope for dev_v3

**1. NO CACHING TESTS**: dev_v3 does NOT implement a caching system, so there will be no tests for caching functionality. Tests will focus on correct validator discovery and URL construction without caching concerns.

**2. NO TRANSACTION RETRY TESTS**: dev_v3 does NOT include transaction retry functionality. Tests will not cover automatic retry scenarios, and error handling will be tested with simplified expectations.

These scope limitations align with the core design goals of dev_v3, which focus on the essential functionality of validator discovery and management without the added complexity of caching or retry mechanisms.

## Test Objectives

1. Verify that the AddressDir struct correctly manages validator multiaddresses
2. Ensure that the DiscoverValidators method correctly discovers validators from the network
3. Validate that the GetActiveValidators method returns the correct set of validators
4. Confirm that the AddressDir struct correctly handles validator status changes

### Network Topology Considerations

The tests will account for the specific topology of the Accumulate network:

1. **All validators participate in the Directory Network (DN)**
2. **Each validator also participates in exactly one Block Validator Network (BVN)**
3. **No validator participates in multiple BVNs**

This topology simplifies our testing approach, as we only need to verify that validators are correctly associated with the DN and exactly one BVN.

## Test Categories

### 1. Unit Tests

Unit tests will verify the core functionality of the AddressDir without requiring network connectivity.

#### 1.1 Validator Management Tests

| Test Name | Description | Expected Outcome |
|-----------|-------------|------------------|
| TestAddValidator | Test adding validators to the directory | Validators are correctly added and can be retrieved |
| TestFindValidator | Test finding validators by ID | Validators are correctly found by ID |
| TestFindValidatorsByPartition | Test finding validators by partition | Validators are correctly found by partition |
| TestGetDNValidators | Test getting all validators in the DN | All validators are returned since all participate in the DN |
| TestGetBVNValidators | Test getting validators for a specific BVN | Only validators from the specified BVN are returned |
| TestSetValidatorStatus | Test setting validator status | Status is correctly updated |

#### 1.2 Address Validation Tests

| Test Name | Description | Expected Outcome |
|-----------|-------------|------------------|
| TestAddValidatorAddress | Test adding valid and invalid addresses | Valid addresses are added, invalid ones rejected |
| TestValidateAddress | Test multiaddress validation | Addresses are correctly validated and components extracted |
| TestMarkAddressPreferred | Test marking addresses as preferred | Addresses are correctly marked as preferred |
| TestGetPreferredAddresses | Test retrieving preferred addresses | Preferred addresses are correctly returned |

#### 1.3 Problem Node Tests

| Test Name | Description | Expected Outcome |
|-----------|-------------|------------------|
| TestMarkNodeProblematic | Test marking nodes as problematic | Nodes are correctly marked as problematic |
| TestIsNodeProblematic | Test checking if nodes are problematic | Problematic status is correctly determined |
| TestProblemNodeBackoff | Test exponential backoff for problematic nodes | Backoff time increases with failures |

#### 1.4 URL Management Tests

| Test Name | Description | Expected Outcome |
|-----------|-------------|------------------|
| TestAddValidatorURL | Test adding URLs to validators | URLs are correctly added and validated |
| TestGetValidatorURL | Test retrieving URLs from validators | URLs are correctly retrieved |
| TestNormalizeValidatorURL | Test URL normalization for validators | URLs are normalized to the correct format |
| TestURLConsistency | Test URL construction consistency with sequence.go | URLs match the format used in sequence.go |

#### 1.5 URL Construction Consistency Tests

| Test Name | Description | Expected Outcome |
|-----------|-------------|------------------|
| TestPartitionUrlConstruction | Test construction of partition URLs | URLs are constructed using protocol.PartitionUrl |
| TestAnchorUrlConstruction | Test construction of anchor URLs | Anchor URLs are constructed consistently |
| TestUrlNormalization | Test normalization of different URL formats | Different URL formats are normalized correctly |

### 2. Integration Tests

Integration tests will verify the AddressDir's interaction with the network. These tests require network connectivity and will be skipped in short mode.

#### 2.1 Validator Discovery Tests

| Test Name | Description | Expected Outcome |
|-----------|-------------|------------------|
| TestDiscoverValidators | Test discovering validators from a network using a mock NetworkService | Validators are correctly discovered and populated |
| TestDiscoverValidatorsMainnet | Test discovering validators from the mainnet | Validators are correctly discovered with proper partition assignments (DN and exactly one BVN) |
| TestValidatorPartitionConsistency | Test that validators are correctly assigned to partitions | All validators are in DN and exactly one BVN |
| TestURLConstructionWithDiscovery | Test that discovered validators use consistent URL construction | URLs match the format used in sequence.go |
| TestDNValidatorList | Test that all validators are added to the DN validator list | The DN validator list contains all validators |
| TestBVNValidatorLists | Test that validators are correctly added to their respective BVN lists | Each validator appears in exactly one BVN list |

### Detailed Partition Discovery Test Plan

#### Mock API Testing

We will create a mock NetworkService implementation that returns predefined validator data for testing:

```go
type MockNetworkService struct {
    // Predefined network status to return
    NetworkStatus *api.NetworkStatusResponse
}

func (m *MockNetworkService) NetworkStatus(ctx context.Context, options api.NetworkStatusOptions) (*api.NetworkStatusResponse, error) {
    return m.NetworkStatus, nil
}
```

This allows us to test various network configurations without requiring actual network connectivity.

#### Test Scenarios for Partition Discovery

1. **Basic Validator Discovery**:
   - Create a mock with validators having different partition assignments
   - Verify that all validators are discovered
   - Verify that partition assignments are correctly processed

2. **DN-Only Validators**:
   - Create a mock with validators that only participate in the DN
   - Verify that these validators are correctly identified as DN-only

3. **DN+BVN Validators**:
   - Create a mock with validators that participate in both DN and a BVN
   - Verify that these validators are correctly identified with both partitions

4. **Inactive Validators**:
   - Create a mock with validators that are inactive on one or both partitions
   - Verify that validator status is correctly determined based on activity

5. **URL Construction**:
   - Verify that partition URLs are constructed using protocol.PartitionUrl
   - Verify that URLs match the expected format (e.g., acc://bvn-Apollo.acme)

#### Mainnet Integration Testing

For mainnet testing, we will:

1. **Connect to Mainnet**:
   - Create a client connected to the mainnet API endpoint
   - Use this client to discover validators

2. **Verify Network Topology**:
   - Verify that all validators participate in the DN
   - Verify that each validator participates in exactly one BVN

3. **Verify URL Construction**:
   - Verify that partition URLs are constructed correctly
   - Verify that these URLs can be used for subsequent API calls

4. **Verify Multiaddress Discovery**:
   - Verify that validator multiaddresses are discovered and validated
   - Verify that these addresses can be used to establish connections

#### 2.2 Private API Access Tests

| Test Name | Description | Expected Outcome |
|-----------|-------------|------------------|
| TestPrivateAPIAccess | Test accessing the private API using multiaddresses | Private API is successfully accessed |

#### 2.3 Transaction Submission Tests

| Test Name | Description | Expected Outcome |
|-----------|-------------|------------------|
| TestTransactionSubmission | Test submitting transactions using the AddressDir | Transactions are successfully submitted |

## Test Implementation Plan

### Phase 1: Unit Tests

Implement the unit tests that don't require network connectivity. These tests will verify the core functionality of the AddressDir.

```go
func TestAddValidator(t *testing.T) {
    addressDir := new_heal.NewAddressDir()
    
    // Test adding a new validator
    validator := addressDir.AddValidator("validator1", "Test Validator", "bvn-Test", "bvn")
    require.NotNil(t, validator)
    require.Equal(t, "validator1", validator.ID)
    require.Equal(t, "Test Validator", validator.Name)
    require.Equal(t, "bvn-Test", validator.PartitionID)
    require.Equal(t, "bvn", validator.PartitionType)
    
    // Test updating an existing validator
    updatedValidator := addressDir.AddValidator("validator1", "Updated Validator", "bvn-Updated", "bvn")
    require.NotNil(t, updatedValidator)
    require.Equal(t, "validator1", updatedValidator.ID)
    require.Equal(t, "Updated Validator", updatedValidator.Name)
    require.Equal(t, "bvn-Updated", updatedValidator.PartitionID)
}
```

### Phase 2: Integration Tests

Implement the integration tests that require network connectivity. These tests will be skipped in short mode.

```go
func TestDiscoverValidatorsMainnet(t *testing.T) {
    // Skip in short mode
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }
    
    // Create a new AddressDir
    addressDir := new_heal.NewAddressDir()
    
    // Create a client to connect to the mainnet
    endpoint := "https://mainnet.accumulatenetwork.io/v3"
    client, err := createMainnetClient(endpoint)
    require.NoError(t, err)
    
    // Discover validators from the mainnet
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    count, err := addressDir.DiscoverValidators(ctx, client)
    require.NoError(t, err)
    require.Greater(t, count, 0, "Should discover at least one validator")
    
    // Verify that validators are correctly assigned to partitions
    // All validators should be in the DN
    dnValidators := addressDir.FindValidatorsByPartition("dn")
    require.NotEmpty(t, dnValidators, "Should have validators in the DN")
    
    // Get active validators
    activeValidators := addressDir.GetActiveValidators()
    require.NotEmpty(t, activeValidators, "Should have active validators")
    
    // Log information about the validators
    t.Logf("Found %d active validators", len(activeValidators))
    for i, validator := range activeValidators {
        t.Logf("Validator %d: ID=%s, Partition=%s", i+1, validator.ID, validator.PartitionID)
    }
}
```

### Phase 3: Test Helpers

Implement helper functions to make testing easier and more consistent.

```go
// createTestAddressDir creates a test AddressDir with predefined validators and addresses
func createTestAddressDir() *new_heal.AddressDir {
    addressDir := new_heal.NewAddressDir()
    
    // Add validators
    // Note: validator1 participates in both a BVN and the DN (implicitly)
    validator1 := addressDir.AddValidator("validator1", "Test Validator 1", "bvn-Test", "bvn")
    // validator2 participates only in the DN
    validator2 := addressDir.AddValidator("validator2", "Test Validator 2", "dn", "dn")
    
    // Set validators as active
    validator1.Status = "active"
    validator2.Status = "active"
    
    // Add addresses with valid peer ID format
    _ = addressDir.AddValidatorAddress("validator1", "/ip4/144.76.105.23/tcp/16593/p2p/12D3KooWJWEKvSFbben74C7H4YtKjhPMTDxd7gP7YeKixCHJSwXQ")
    _ = addressDir.AddValidatorAddress("validator2", "/ip4/144.76.105.24/tcp/16593/p2p/12D3KooWHFtBnuNU23vXtJ3L5PMralcyNVzfHbYWquXEXW9m7H3G")
    
    return addressDir
}

// createMockNetworkService creates a mock NetworkService for testing
func createMockNetworkService() *MockNetworkService {
    // Create a mock network definition
    networkDef := &protocol.NetworkDefinition{
        NetworkName: "testnet",
        Partitions: []*protocol.PartitionInfo{
            {ID: "dn", Type: protocol.PartitionTypeDirectory},
            {ID: "bvn-Test", Type: protocol.PartitionTypeBlockValidator},
        },
        Validators: []*protocol.ValidatorInfo{
            {
                PublicKeyHash: [32]byte{1, 2, 3, 4},
                Operator: url.MustParse("acc://validator1.acme"),
                Partitions: []*protocol.ValidatorPartitionInfo{
                    {ID: "dn", Active: true},
                    {ID: "bvn-Test", Active: true},
                },
            },
            {
                PublicKeyHash: [32]byte{5, 6, 7, 8},
                Operator: url.MustParse("acc://validator2.acme"),
                Partitions: []*protocol.ValidatorPartitionInfo{
                    {ID: "dn", Active: true},
                },
            },
        },
    }
    
    // Create a mock NetworkStatus
    networkStatus := &api.NetworkStatus{
        Network: networkDef,
    }
    
    // Return a mock NetworkService
    return &MockNetworkService{
        networkStatus: networkStatus,
    }
}

// createMainnetClient creates a client to connect to the mainnet
func createMainnetClient(endpoint string) (api.NetworkService, error) {
    // Create a JSON-RPC client
    client := jsonrpc.NewClient(endpoint)
    return client, nil
}

// normalizeUrl ensures consistent URL format for partitions
// This function standardizes URLs to use the format from sequence.go
// (e.g., acc://bvn-Apollo.acme) rather than other formats
func normalizeUrl(partitionID string) *url.URL {
    // Use the protocol.PartitionUrl function to ensure consistency
    return protocol.PartitionUrl(partitionID)
}

// testUrlConsistency tests that URLs are constructed consistently
func testUrlConsistency(t *testing.T) {
    // Test DN URL construction
    dnUrl := normalizeUrl("dn")
    require.Equal(t, "acc://dn.acme", dnUrl.String())
    
    // Test BVN URL construction
    bvnUrl := normalizeUrl("bvn-Apollo")
    require.Equal(t, "acc://bvn-Apollo.acme", bvnUrl.String())
    
    // Test that it matches protocol.PartitionUrl
    protocolDnUrl := protocol.PartitionUrl("dn")
    require.Equal(t, protocolDnUrl.String(), dnUrl.String())
    
    protocolBvnUrl := protocol.PartitionUrl("bvn-Apollo")
    require.Equal(t, protocolBvnUrl.String(), bvnUrl.String())
}

## Test Dependencies

The tests will require the following dependencies:

- `github.com/stretchr/testify/require` for assertions
- `github.com/multiformats/go-multiaddr` for multiaddress handling
- `gitlab.com/accumulatenetwork/accumulate/pkg/url` for URL parsing
- `gitlab.com/accumulatenetwork/accumulate/pkg/api/v3` for API client interactions
- `gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc` for JSON-RPC client
- `gitlab.com/accumulatenetwork/accumulate/protocol` for protocol types

## What We Are NOT Testing

To be absolutely clear, the following functionality is NOT part of dev_v3 and will NOT be tested:

1. **Caching System**: No tests for query result caching, cache invalidation, or cache hit/miss scenarios.

2. **Transaction Retry Logic**: No tests for automatic retry of failed transactions, retry policies, or backoff strategies.

3. **Problem Node Tracking**: While we track validator status, we do not implement the complex problem node tracking from previous versions.

Tests should focus solely on the core functionality of validator discovery, URL construction, and basic validator management.

## Test Configuration

The tests will be configurable through environment variables:

- `TEST_NETWORK`: The network to test against (default: "kermint")
- `TEST_ENDPOINT`: The API endpoint to use (default: "https://mainnet.accumulatenetwork.io/v3")
- `SKIP_INTEGRATION_TESTS`: Skip integration tests (default: false)

## Next Steps

1. Implement the unit tests for the core functionality
2. Implement the DiscoverValidators method consistent with the heal_v1 implementation
   - Use the NetworkStatus API to discover validators
   - Process validator information correctly, recognizing that all validators participate in the DN and exactly one BVN
   - Set validator status based on whether they're active on their partitions
3. Implement the GetActiveValidators method to return validators active on either their BVN or the DN
4. Implement URL normalization functions to ensure consistency with sequence.go
   - Use protocol.PartitionUrl for constructing partition URLs
   - Normalize URLs before using them in queries or comparisons
5. Implement the integration tests against mainnet to verify real-world behavior
6. Add specific tests for URL construction consistency
   - Test that URLs are constructed consistently across the codebase
   - Test that URL normalization works correctly for different URL formats
7. Ensure the implementation uses consistent URL formats
   - Use protocol.PartitionUrl for all URL construction
   - Normalize URLs before using them in any operations

---

> **⚠️ IMPORTANT: DO NOT DELETE THIS DEVELOPMENT PLAN ⚠️**  
> Cascade likes to delete development plans and other guidance.  
> Cascade ensures that THIS development plan (above) will not be deleted.
