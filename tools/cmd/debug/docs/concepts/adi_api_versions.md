# ADI API Version Comparison

```yaml
# AI-METADATA
document_type: api_comparison
project: accumulate_network
component: adi_api
version: current
authors:
  - accumulate_team
last_updated: 2023-11-15

# API Versions
api_versions:
  - v1:
      status: deprecated
      release_date: "2021"
      key_features: [basic_adi_operations, limited_query_capabilities]
  - v2:
      status: stable
      release_date: "2022"
      key_features: [enhanced_adi_management, improved_query_capabilities, transaction_submission]
  - v3:
      status: current
      release_date: "2023"
      key_features: [stateless_design, optimized_routing, comprehensive_adi_operations, advanced_query_capabilities]

# Core Concepts
key_concepts:
  - api_evolution:
      description: "Evolution of ADI-related APIs across versions"
      importance: high
  - version_differences:
      description: "Key differences between API versions for ADIs"
      importance: high
  - migration_paths:
      description: "Guidance for migrating between API versions"
      importance: medium

# Related Components
related_components:
  - adi:
      relationship: "Primary focus of this comparison"
      importance: critical
  - key_management:
      relationship: "Key management capabilities across versions"
      importance: high
  - transaction_submission:
      relationship: "Transaction submission differences across versions"
      importance: high

# Technical Details
language: go
dependencies:
  - gitlab.com/accumulatenetwork/accumulate/protocol
  - gitlab.com/accumulatenetwork/accumulate/pkg/api/v3
  - gitlab.com/accumulatenetwork/accumulate/internal/api/v2
  - gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/deprecated/v1
```

## Overview

This document provides a comprehensive comparison of the different API versions in Accumulate, specifically focusing on how they handle Accumulate Digital Identities (ADIs). Understanding these differences is crucial for developers migrating between versions or building applications that need to support multiple API versions.

## API Evolution Timeline

| Version | Release Date | Status | Key Focus |
|---------|-------------|--------|----------|
| v1 | 2021 | Deprecated | Basic blockchain functionality |
| v2 | 2022 | Stable | Enhanced key management, improved routing |
| v3 | 2023 | Current | Stateless design, comprehensive ADI capabilities |

## Feature Comparison

<!-- @concept api_feature_comparison -->
<!-- @importance high -->
<!-- @description Detailed comparison of ADI features across API versions -->

The following table provides a detailed comparison of ADI-related features across the three API versions:

| Feature | API v1 | API v2 | API v3 |
|---------|--------|--------|--------|
| **ADI Creation** | Basic (single key) | Enhanced (key book support) | Comprehensive (full options) |
| **Sub-ADI Support** | No | Basic | Full hierarchical support |
| **Key Management** | Direct key addition/removal | Key books and pages | Advanced key hierarchy |
| **Signature Types** | ED25519 only | ED25519, RCD1 | ED25519, RCD1, BTC, ETH, RCD |
| **Multi-signature** | Basic (threshold) | Enhanced (multiple signers) | Advanced (complex rules) |
| **Key Rotation** | Basic replacement | Page-based rotation | Comprehensive strategies |
| **Query Capabilities** | Basic lookups | Enhanced filtering | Advanced query language |
| **Transaction Submission** | Synchronous only | Sync/Async options | Fully stateless design |
| **Error Handling** | Basic errors | Enhanced error details | Comprehensive error context |
| **URL Handling** | Basic addressing | Enhanced routing | Optimized partition routing |
| **Pagination** | Limited | Enhanced | Comprehensive |
| **Batch Operations** | No | Limited | Full support |
| **Webhook Support** | No | Basic | Advanced |
| **Historical Queries** | Limited | Enhanced | Comprehensive |

<!-- @concept api_evolution -->
<!-- @importance high -->
<!-- @description Evolution of Accumulate APIs over time -->

```
┌────────────┐     ┌────────────┐     ┌────────────┐
│    API v1  │ ──▶ │    API v2  │ ──▶ │    API v3  │
│  (Legacy)  │     │ (Improved) │     │ (Current)  │
└────────────┘     └────────────┘     └────────────┘
    2021              2022              2023
```

- **API v1**: Initial implementation with basic ADI functionality
- **API v2**: Enhanced with improved query capabilities and transaction handling
- **API v3**: Complete redesign with stateless architecture and optimized routing

## Feature Comparison Matrix

| Feature | API v1 | API v2 | API v3 |
|---------|--------|--------|--------|
| **ADI Creation** | Basic | Enhanced | Comprehensive |
| **Sub-ADI Management** | Limited | Improved | Full Support |
| **Key Book Operations** | Basic | Enhanced | Advanced |
| **Key Page Management** | Limited | Improved | Comprehensive |
| **Authority Delegation** | Not Supported | Basic Support | Full Support |
| **Query Capabilities** | Basic | Improved | Advanced |
| **Transaction Submission** | Basic | Enhanced | Optimized |
| **Error Handling** | Limited | Improved | Comprehensive |
| **Pagination Support** | Basic | Enhanced | Advanced |
| **Webhook Support** | Not Supported | Basic | Advanced |
| **Stateless Design** | No | Partial | Yes |
| **Routing Optimization** | No | Partial | Yes |
| **Client Libraries** | Limited | Improved | Comprehensive |

## API v1 (Legacy)

<!-- @api v1 -->
<!-- @status deprecated -->
<!-- @description Initial API implementation with basic ADI functionality -->

### ADI Operations in v1

#### ADI Creation

```go
// API v1: Create ADI
func (c *Client) CreateADI(ctx context.Context, name string, publicKey []byte) (*protocol.TransactionStatus, error) {
    // Basic ADI creation with limited options
    tx := &protocol.Transaction{
        Header: protocol.TransactionHeader{
            Principal: protocol.AccountUrl("acme"),
        },
        Body: &protocol.CreateIdentity{
            Url: protocol.AccountUrl(name + ".acme"),
            KeyHash: publicKey,
        },
    }
    
    return c.Submit(ctx, tx)
}
```

#### Key Management

```go
// API v1: Add Key to ADI
func (c *Client) AddKey(ctx context.Context, adiUrl string, publicKey []byte) (*protocol.TransactionStatus, error) {
    // Basic key addition with limited validation
    tx := &protocol.Transaction{
        Header: protocol.TransactionHeader{
            Principal: protocol.AccountUrl(adiUrl),
        },
        Body: &protocol.AddKey{
            KeyHash: publicKey,
        },
    }
    
    return c.Submit(ctx, tx)
}
```

#### Query Capabilities

```go
// API v1: Query ADI
func (c *Client) QueryADI(ctx context.Context, adiUrl string) (*protocol.ADI, error) {
    // Basic query with limited error handling
    resp, err := c.Query(ctx, "adi", map[string]string{
        "url": adiUrl,
    })
    if err != nil {
        return nil, err
    }
    
    var adi protocol.ADI
    err = json.Unmarshal(resp, &adi)
    return &adi, err
}
```

### Limitations of v1

1. **Limited ADI Operations**: Basic creation and management only
2. **Minimal Key Management**: Simple key addition without sophisticated book/page structure
3. **Basic Query Capabilities**: Limited filtering and pagination
4. **Error Handling**: Minimal error information and recovery options
5. **No Sub-ADI Support**: Limited hierarchical identity management
6. **Stateful Design**: Requires maintaining state between operations

## API v2 (Improved)

<!-- @api v2 -->
<!-- @status stable -->
<!-- @description Enhanced API with improved ADI management capabilities -->

### ADI Operations in v2

#### ADI Creation with Key Book

```go
// API v2: Create ADI with Key Book
func (c *Client) CreateADI(ctx context.Context, name string, keys []protocol.PublicKey, threshold int) (*protocol.TransactionStatus, error) {
    // Enhanced ADI creation with key book support
    tx := &protocol.Transaction{
        Header: protocol.TransactionHeader{
            Principal: protocol.AccountUrl("acme"),
        },
        Body: &protocol.CreateIdentity{
            Url: protocol.AccountUrl(name + ".acme"),
            KeyBook: &protocol.KeyBook{
                Keys: keys,
                Threshold: threshold,
            },
        },
    }
    
    return c.Submit(ctx, tx)
}
```

#### Advanced Key Management

```go
// API v2: Create Key Page
func (c *Client) CreateKeyPage(ctx context.Context, adiUrl string, keys []protocol.PublicKey, threshold int) (*protocol.TransactionStatus, error) {
    // Create a key page with multiple keys and threshold
    bookUrl := protocol.FormatUrl(protocol.AccountUrl(adiUrl), "book")
    
    tx := &protocol.Transaction{
        Header: protocol.TransactionHeader{
            Principal: bookUrl,
        },
        Body: &protocol.CreateKeyPage{
            Keys: keys,
            Threshold: threshold,
        },
    }
    
    return c.Submit(ctx, tx)
}
```

#### Enhanced Query Capabilities

```go
// API v2: Query ADI with Options
func (c *Client) QueryADI(ctx context.Context, adiUrl string, options QueryOptions) (*protocol.ADI, error) {
    // Enhanced query with options for expansion and filtering
    params := map[string]string{
        "url": adiUrl,
    }
    
    if options.ExpandKeyBook {
        params["expand"] = "keyBook"
    }
    
    resp, err := c.Query(ctx, "adi", params)
    if err != nil {
        return nil, err
    }
    
    var adi protocol.ADI
    err = json.Unmarshal(resp, &adi)
    return &adi, err
}
```

### Improvements in v2

1. **Enhanced ADI Creation**: Support for key books and pages during creation
2. **Improved Key Management**: Hierarchical key management with books and pages
3. **Better Query Options**: Expanded filtering, sorting, and pagination
4. **Improved Error Handling**: More detailed error information
5. **Basic Sub-ADI Support**: Initial support for hierarchical identities
6. **Partial Stateless Design**: Reduced state requirements between operations

## API v3 (Current)

<!-- @api v3 -->
<!-- @status current -->
<!-- @description Modern API with comprehensive ADI functionality and stateless design -->

### ADI Operations in v3

#### Comprehensive ADI Creation

```go
// API v3: Create ADI with Full Options
func (c *Client) CreateADI(ctx context.Context, options *v3.CreateADIOptions) (*v3.TxResponse, error) {
    // Comprehensive ADI creation with extensive options
    req := &v3.CreateADI{
        Url:         options.Url,
        KeyBookUrl:  options.KeyBookUrl,
        KeyPageUrl:  options.KeyPageUrl,
        Keys:        options.Keys,
        Threshold:   options.Threshold,
        Authorities: options.Authorities,
    }
    
    return c.Execute(ctx, req)
}
```

#### Advanced Key Management

```go
// API v3: Manage Key Book Authorities
func (c *Client) UpdateKeyBookAuthorities(ctx context.Context, bookUrl *url.URL, authorities []*url.URL) (*v3.TxResponse, error) {
    // Advanced authority management for key books
    req := &v3.UpdateKeyBookAuthorities{
        KeyBookUrl:  bookUrl,
        Authorities: authorities,
        Operation:   v3.AuthorityOperationAdd,
    }
    
    return c.Execute(ctx, req)
}
```

#### Comprehensive Query Capabilities

```go
// API v3: Advanced ADI Query
func (c *Client) QueryADI(ctx context.Context, adiUrl *url.URL, options *v3.QueryOptions) (*v3.ADIResponse, error) {
    // Comprehensive query with advanced options
    req := &v3.QueryADI{
        Url:           adiUrl,
        IncludeKeyBook: options.IncludeKeyBook,
        IncludeKeyPages: options.IncludeKeyPages,
        IncludeSubAdis: options.IncludeSubAdis,
        IncludeAccounts: options.IncludeAccounts,
        Expand:        options.Expand,
    }
    
    return c.Query(ctx, req)
}
```

### Key Improvements in v3

1. **Stateless Design**: Fully stateless API design requiring no state between operations
2. **Optimized Routing**: Improved transaction routing with URL normalization
3. **Comprehensive ADI Operations**: Complete support for all ADI operations
4. **Advanced Key Management**: Sophisticated key book and page management
5. **Full Sub-ADI Support**: Complete hierarchical identity management
6. **Advanced Query Capabilities**: Comprehensive filtering, sorting, and pagination
7. **Detailed Error Handling**: Extensive error information and recovery options
8. **Webhook Improvements**: Advanced event notification system
9. **Client Libraries**: Comprehensive client libraries for multiple languages

## API Method Comparison for ADI Operations

### ADI Creation

| Operation | API v1 | API v2 | API v3 |
|-----------|--------|--------|--------|
| **Method Name** | `CreateADI` | `CreateIdentity` | `CreateADI` |
| **Key Support** | Single Key | Multiple Keys | Multiple Keys + Books |
| **Authority Options** | None | Basic | Comprehensive |
| **Validation** | Basic | Enhanced | Advanced |
| **Error Handling** | Limited | Improved | Comprehensive |

### Key Management

| Operation | API v1 | API v2 | API v3 |
|-----------|--------|--------|--------|
| **Create Key Book** | Not Supported | `CreateKeyBook` | `CreateKeyBook` |
| **Create Key Page** | Not Supported | `CreateKeyPage` | `CreateKeyPage` |
| **Add Key** | `AddKey` | `AddKeyToPage` | `UpdateKeyPage` |
| **Remove Key** | `RemoveKey` | `RemoveKeyFromPage` | `UpdateKeyPage` |
| **Update Threshold** | Not Supported | `UpdateThreshold` | `UpdateKeyPage` |
| **Manage Authorities** | Not Supported | Basic | `UpdateAuthorities` |

### Query Operations

| Operation | API v1 | API v2 | API v3 |
|-----------|--------|--------|--------|
| **Query ADI** | `QueryADI` | `QueryIdentity` | `QueryADI` |
| **Query Key Book** | Not Supported | `QueryKeyBook` | `QueryKeyBook` |
| **Query Key Page** | Not Supported | `QueryKeyPage` | `QueryKeyPage` |
| **List Sub-ADIs** | Not Supported | Limited | `QuerySubAdis` |
| **Pagination** | Basic | Enhanced | Advanced |
| **Filtering** | Limited | Improved | Comprehensive |

## Implementation Differences

### URL Handling

<!-- @concept url_handling_differences -->
<!-- @importance high -->
<!-- @description Differences in URL handling across API versions -->

```go
// API v1: Basic URL handling
adiUrl := "acc://factom.acme"

// API v2: Improved URL handling
adiUrl := protocol.AccountUrl("factom.acme")

// API v3: Comprehensive URL handling with normalization
adiUrl, err := protocol.ParseUrl("acc://factom.acme")
if err != nil {
    return err
}
normalizedUrl := normalizeUrl(adiUrl)
```

### Transaction Submission

<!-- @concept transaction_submission_differences -->
<!-- @importance high -->
<!-- @description Differences in transaction submission across API versions -->

```go
// API v1: Basic submission
status, err := client.Submit(ctx, tx)

// API v2: Enhanced submission with options
status, err := client.Submit(ctx, tx, client.WithWait(true))

// API v3: Comprehensive submission with routing
resp, err := client.Execute(ctx, tx)
```

### Error Handling

<!-- @concept error_handling_differences -->
<!-- @importance high -->
<!-- @description Differences in error handling across API versions -->

```go
// API v1: Basic error handling
if err != nil {
    log.Fatalf("Error: %v", err)
}

// API v2: Improved error handling
if err != nil {
    if apiErr, ok := err.(*api.Error); ok {
        log.Fatalf("API Error: %s (Code: %d)", apiErr.Message, apiErr.Code)
    }
    log.Fatalf("Error: %v", err)
}

// API v3: Comprehensive error handling
if err != nil {
    switch e := err.(type) {
    case *v3.Error:
        log.Fatalf("API Error: %s (Code: %d, Detail: %s)", e.Message, e.Code, e.Detail)
    case *v3.ValidationError:
        for _, field := range e.Fields {
            log.Fatalf("Validation Error: %s: %s", field.Name, field.Error)
        }
    default:
        log.Fatalf("Error: %v", err)
    }
}
```

## Migration Guide

### Migrating from v1 to v2

1. **Update URL Handling**: Replace string URLs with `protocol.AccountUrl`
2. **Enhance Key Management**: Utilize key books and pages instead of direct key operations
3. **Improve Query Parameters**: Update query parameters to use enhanced options
4. **Update Error Handling**: Implement improved error handling with API error types
5. **Utilize New Features**: Take advantage of sub-ADI support and authority management

### Migrating from v2 to v3

1. **Adopt Stateless Design**: Redesign client code to work with the stateless API
2. **Update URL Handling**: Use `protocol.ParseUrl` and implement URL normalization
3. **Enhance Transaction Submission**: Update to use the new execution model
4. **Improve Error Handling**: Implement comprehensive error handling with detailed error types
5. **Utilize Advanced Features**: Take advantage of comprehensive ADI operations and query capabilities

## Best Practices

### API v1 (If Still Required)

1. **Limit Usage**: Use only for legacy systems that cannot be updated
2. **Simple Operations**: Stick to basic ADI operations
3. **Implement Workarounds**: Create client-side workarounds for missing features
4. **Plan Migration**: Develop a plan to migrate to newer API versions

### API v2

1. **Utilize Key Books**: Take advantage of the key book and page structure
2. **Implement Proper Error Handling**: Use the enhanced error information
3. **Use Query Options**: Leverage the improved query capabilities
4. **Consider Authority Management**: Implement basic authority delegation
5. **Plan for v3**: Design with eventual migration to v3 in mind

### API v3

1. **Embrace Stateless Design**: Design client applications to work with the stateless API
2. **Implement URL Normalization**: Ensure consistent URL handling
3. **Utilize Advanced Features**: Take advantage of comprehensive ADI operations
4. **Implement Comprehensive Error Handling**: Use the detailed error information
5. **Leverage Webhooks**: Use the advanced event notification system
6. **Use Client Libraries**: Utilize the comprehensive client libraries

## Implementation Examples

### Creating an ADI with Multiple Keys

#### API v1

```go
// API v1: Create ADI with workaround for multiple keys
func createADIWithMultipleKeys(client *v1.Client, name string, keys [][]byte) error {
    // Create ADI with first key
    status, err := client.CreateADI(ctx, name, keys[0])
    if err != nil {
        return err
    }
    
    // Add additional keys one by one
    adiUrl := name + ".acme"
    for i := 1; i < len(keys); i++ {
        _, err = client.AddKey(ctx, adiUrl, keys[i])
        if err != nil {
            return err
        }
    }
    
    return nil
}
```

#### API v2

```go
// API v2: Create ADI with multiple keys
func createADIWithMultipleKeys(client *v2.Client, name string, keys []protocol.PublicKey) error {
    // Create ADI with key book containing all keys
    status, err := client.CreateADI(ctx, name, keys, 1)
    if err != nil {
        return err
    }
    
    return nil
}
```

#### API v3

```go
// API v3: Create ADI with multiple keys
func createADIWithMultipleKeys(client *v3.Client, name string, keys []protocol.PublicKey) error {
    // Create ADI with comprehensive options
    adiUrl, _ := protocol.ParseUrl("acc://" + name + ".acme")
    
    options := &v3.CreateADIOptions{
        Url:       adiUrl,
        Keys:      keys,
        Threshold: 1,
    }
    
    _, err := client.CreateADI(ctx, options)
    return err
}
```

### Querying an ADI with Key Information

#### API v1

```go
// API v1: Query ADI with workaround for key information
func queryADIWithKeys(client *v1.Client, adiUrl string) (*ADIWithKeys, error) {
    // Query basic ADI
    adi, err := client.QueryADI(ctx, adiUrl)
    if err != nil {
        return nil, err
    }
    
    // Separate query for keys (not directly supported)
    keys, err := client.QueryKeys(ctx, adiUrl)
    if err != nil {
        return nil, err
    }
    
    return &ADIWithKeys{
        ADI:  adi,
        Keys: keys,
    }, nil
}
```

#### API v2

```go
// API v2: Query ADI with key information
func queryADIWithKeys(client *v2.Client, adiUrl string) (*protocol.ADI, error) {
    // Query ADI with expanded key book
    options := QueryOptions{
        ExpandKeyBook: true,
    }
    
    return client.QueryADI(ctx, adiUrl, options)
}
```

#### API v3

```go
// API v3: Query ADI with comprehensive key information
func queryADIWithKeys(client *v3.Client, adiUrl *url.URL) (*v3.ADIResponse, error) {
    // Query ADI with comprehensive options
    options := &v3.QueryOptions{
        IncludeKeyBook:  true,
        IncludeKeyPages: true,
        Expand:         []string{"authorities", "keyBook.authorities"},
    }
    
    return client.QueryADI(ctx, adiUrl, options)
}
```

## API Method Comparison

<!-- @concept api_method_comparison -->
<!-- @importance high -->
<!-- @description Detailed comparison of ADI-related API methods across versions -->

The following tables provide a method-by-method comparison of ADI-related operations across the three API versions.

### ADI Creation and Management

| Operation | API v1 | API v2 | API v3 |
|-----------|--------|--------|--------|
| **Create ADI** | `CreateADI(name, publicKey)` | `CreateADI(name, keys, threshold)` | `CreateADI(options *CreateADIOptions)` |
| **Create Sub-ADI** | Not supported | `CreateSubADI(parent, name, keys)` | `CreateADI(options *CreateADIOptions)` with parent URL |
| **Update ADI** | Limited direct updates | `UpdateADI(url, options)` | `UpdateADI(url, options *UpdateADIOptions)` |
| **Get ADI** | `GetADI(name)` | `GetADI(url)` | `QueryUrl(url, options *QueryOptions)` |

### Key Management

| Operation | API v1 | API v2 | API v3 |
|-----------|--------|--------|--------|
| **Add Key** | `AddKey(adiName, publicKey)` | `AddKeyToBook(bookUrl, key)` | `AddKeyPage(bookUrl, options *AddKeyPageOptions)` |
| **Update Key** | `UpdateKey(adiName, oldKey, newKey)` | `UpdateKeyPage(pageUrl, options)` | `UpdateKeyPage(pageUrl, options *UpdateKeyPageOptions)` |
| **Disable Key** | `DisableKey(adiName, publicKey)` | `DisableKeyPage(pageUrl)` | `UpdateKeyPage(pageUrl, options *UpdateKeyPageOptions)` with disabled flag |
| **Create Key Book** | Not supported | `CreateKeyBook(adiUrl, keys, threshold)` | `CreateKeyBook(adiUrl, options *CreateKeyBookOptions)` |
| **Create Key Page** | Not supported | `CreateKeyPage(bookUrl, keys, threshold)` | `AddKeyPage(bookUrl, options *AddKeyPageOptions)` |

### Query Capabilities

| Operation | API v1 | API v2 | API v3 |
|-----------|--------|--------|--------|
| **Query ADI** | Basic query by name | `QueryADI(url)` | `QueryUrl(url, options *QueryOptions)` |
| **Query Key Book** | Not supported | `QueryKeyBook(url)` | `QueryUrl(url, options *QueryOptions)` |
| **Query Key Page** | Not supported | `QueryKeyPage(url)` | `QueryUrl(url, options *QueryOptions)` |
| **Query Transaction** | Basic TX query | Enhanced TX query | `QueryTx(txid, options *QueryOptions)` |
| **Query Chain** | Limited chain query | Enhanced chain query | `QueryChain(chainID, options *QueryOptions)` |

### Code Examples

#### Creating an ADI in v1
```go
func createADIv1(client *v1.Client, name string, publicKey []byte) error {
    status, err := client.CreateADI(ctx, name, publicKey)
    return err
}
```

#### Creating an ADI in v2
```go
func createADIv2(client *v2.Client, name string, keys []protocol.PublicKey, threshold int) error {
    status, err := client.CreateADI(ctx, name, keys, threshold)
    return err
}
```

#### Creating an ADI in v3
```go
func createADIv3(client *v3.Client, name string, keys []protocol.PublicKey, threshold int) error {
    adiUrl, _ := protocol.ParseUrl("acc://" + name + ".acme")
    
    options := &v3.CreateADIOptions{
        Url: adiUrl,
        KeyBookUrl: nil, // Will create a default key book
        PublicKeys: keys,
        SignatureThreshold: threshold,
    }
    
    _, err := client.CreateADI(ctx, options)
    return err
}
```

## Tendermint/CometBFT APIs and Accumulate

<!-- @concept tendermint_integration -->
<!-- @importance high -->
<!-- @description How Tendermint/CometBFT APIs integrate with Accumulate APIs -->

Accumulate uses Tendermint (now known as CometBFT) as its consensus engine. Understanding the relationship between Accumulate's APIs and Tendermint's APIs is important for developers working with the full stack.

### Tendermint API Integration

| Aspect | Description | API Version Support |
|--------|-------------|---------------------|
| **Consensus** | Tendermint provides the underlying consensus mechanism for Accumulate | All versions |
| **Transaction Submission** | Transactions can be submitted directly via Tendermint RPC or through Accumulate APIs | All versions, but usage patterns differ |
| **Block Queries** | Query blocks and block results directly from Tendermint | All versions |
| **Network Status** | Query node status, network info, and health checks | All versions |
| **Peer Discovery** | Discover and connect to peers in the network | All versions |
| **Event Subscription** | Subscribe to events from the Accumulate network | Enhanced in v2 and v3 |

### API Version Differences with Tendermint

| Feature | API v1 | API v2 | API v3 |
|---------|--------|--------|--------|
| **Tendermint Version** | Tendermint Core | Tendermint Core | CometBFT |
| **Direct Tendermint Access** | Required for some operations | Optional for advanced use cases | Abstracted for most operations |
| **Transaction Routing** | Basic routing to Tendermint nodes | Enhanced routing with partition awareness | Optimized stateless routing |
| **Error Handling** | Basic Tendermint errors | Enhanced error context | Comprehensive error mapping |
| **Event Subscription** | Basic | Enhanced | Advanced with filtering |

### When to Use Tendermint APIs Directly

1. **Low-level Consensus Operations**: When you need direct access to consensus mechanisms
2. **Custom Block Exploration**: For specialized block and transaction queries
3. **Network Diagnostics**: For detailed node and network status information
4. **Performance-Critical Applications**: When bypassing Accumulate's API layers is necessary

### Code Example: Using Both APIs

```go
// Example of using both Accumulate and Tendermint APIs
func queryWithBothAPIs(accClient *v3.Client, tendermintRPC string) error {
    // Use Accumulate API for ADI query
    adiUrl, _ := protocol.ParseUrl("acc://myidentity.acme")
    adiInfo, err := accClient.QueryUrl(ctx, adiUrl, nil)
    if err != nil {
        return err
    }
    
    // Use Tendermint API for consensus information
    tmClient, err := http.New(tendermintRPC, "/websocket")
    if err != nil {
        return err
    }
    
    status, err := tmClient.Status(ctx)
    if err != nil {
        return err
    }
    
    fmt.Printf("ADI Info: %v\nConsensus Height: %d\n", 
        adiInfo, status.SyncInfo.LatestBlockHeight)
    return nil
}
```

## Related Documentation

- [Accumulate Digital Identities (ADIs)](adi_documentation.md)
- [Key Management](adi_key_management.md)
- [URL Addressing](adi_url_addressing.md)
- [Transaction Processing](transaction_processing.md)
- [API v2 Reference](api_v2_reference.md)
- [API Migration Guide](api_migration_guide.md)
- [CometBFT Documentation](https://docs.cometbft.com/)
