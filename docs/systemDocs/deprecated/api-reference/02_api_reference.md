# API Reference

```yaml
# AI-METADATA
document_type: reference
project: accumulate_network
component: debug_tools
topic: api_reference
audience: developers
related_files:
  - ../index.md
  - ./command-reference.md
  - ./configuration-reference.md
```

## Overview

This reference document provides details about the API interfaces used by the Accumulate Network debug tools. It covers the core API functions, data structures, and usage patterns.

## API Client

The debug tools use the Accumulate API client to interact with the network:

```go
// Client is the interface for interacting with the Accumulate API
type Client interface {
    // Query executes a query against the API
    Query(ctx context.Context, method string, params ...interface{}) (*api.Response, error)
    
    // QueryAccount queries an account
    QueryAccount(ctx context.Context, url *url.URL) (*protocol.Account, error)
    
    // QueryChain queries a chain
    QueryChain(ctx context.Context, url *url.URL, query *api.ChainQuery) (*api.ChainQueryResponse, error)
    
    // QueryTransaction queries a transaction
    QueryTransaction(ctx context.Context, txid []byte) (*api.TransactionQueryResponse, error)
    
    // QueryValidators queries validators
    QueryValidators(ctx context.Context, networkName string) ([]*api.ValidatorInfo, error)
    
    // Submit submits a transaction
    Submit(ctx context.Context, tx *protocol.Transaction) (*api.SubmitResponse, error)
}
```

## Core API Functions

### Query

```go
// Query executes a query against the API
func (c *Client) Query(ctx context.Context, method string, params ...interface{}) (*api.Response, error)
```

**Parameters**:
- `ctx`: Context for the operation
- `method`: The API method to call
- `params`: The parameters for the API method

**Returns**:
- `*api.Response`: The response from the API
- `error`: An error if the query fails

**Example**:
```go
resp, err := client.Query(ctx, "status", nil)
if err != nil {
    return err
}
```

### QueryAccount

```go
// QueryAccount queries an account
func (c *Client) QueryAccount(ctx context.Context, url *url.URL) (*protocol.Account, error)
```

**Parameters**:
- `ctx`: Context for the operation
- `url`: The URL of the account to query

**Returns**:
- `*protocol.Account`: The account information
- `error`: An error if the query fails

**Example**:
```go
account, err := client.QueryAccount(ctx, protocol.ParseUrl("acc://myaccount.acme"))
if err != nil {
    return err
}
```

### QueryChain

```go
// QueryChain queries a chain
func (c *Client) QueryChain(ctx context.Context, url *url.URL, query *api.ChainQuery) (*api.ChainQueryResponse, error)
```

**Parameters**:
- `ctx`: Context for the operation
- `url`: The URL of the chain to query
- `query`: The query parameters

**Returns**:
- `*api.ChainQueryResponse`: The chain query response
- `error`: An error if the query fails

**Example**:
```go
resp, err := client.QueryChain(ctx, protocol.ParseUrl("acc://myaccount.acme/data"), &api.ChainQuery{
    Entry: 1,
})
if err != nil {
    return err
}
```

### QueryTransaction

```go
// QueryTransaction queries a transaction
func (c *Client) QueryTransaction(ctx context.Context, txid []byte) (*api.TransactionQueryResponse, error)
```

**Parameters**:
- `ctx`: Context for the operation
- `txid`: The transaction ID to query

**Returns**:
- `*api.TransactionQueryResponse`: The transaction query response
- `error`: An error if the query fails

**Example**:
```go
resp, err := client.QueryTransaction(ctx, txid)
if err != nil {
    return err
}
```

### QueryValidators

```go
// QueryValidators queries validators
func (c *Client) QueryValidators(ctx context.Context, networkName string) ([]*api.ValidatorInfo, error)
```

**Parameters**:
- `ctx`: Context for the operation
- `networkName`: The name of the network to query validators for

**Returns**:
- `[]*api.ValidatorInfo`: The validator information
- `error`: An error if the query fails

**Example**:
```go
validators, err := client.QueryValidators(ctx, "mainnet")
if err != nil {
    return err
}
```

### Submit

```go
// Submit submits a transaction
func (c *Client) Submit(ctx context.Context, tx *protocol.Transaction) (*api.SubmitResponse, error)
```

**Parameters**:
- `ctx`: Context for the operation
- `tx`: The transaction to submit

**Returns**:
- `*api.SubmitResponse`: The submit response
- `error`: An error if the submission fails

**Example**:
```go
resp, err := client.Submit(ctx, tx)
if err != nil {
    return err
}
```

## Data Structures

### Response

```go
// Response is the response from an API query
type Response struct {
    // Type is the type of the response
    Type string `json:"type"`
    
    // Result is the result of the query
    Result json.RawMessage `json:"result"`
    
    // Error is the error message if the query failed
    Error string `json:"error,omitempty"`
}
```

### ChainQuery

```go
// ChainQuery is a query for a chain
type ChainQuery struct {
    // Entry is the entry to query
    Entry uint64 `json:"entry,omitempty"`
    
    // Range is the range of entries to query
    Range *ChainRange `json:"range,omitempty"`
    
    // Expand indicates whether to expand the entries
    Expand bool `json:"expand,omitempty"`
}
```

### ChainRange

```go
// ChainRange is a range of chain entries
type ChainRange struct {
    // Start is the start of the range
    Start uint64 `json:"start,omitempty"`
    
    // End is the end of the range
    End uint64 `json:"end,omitempty"`
    
    // Count is the number of entries to return
    Count uint64 `json:"count,omitempty"`
}
```

### ChainQueryResponse

```go
// ChainQueryResponse is the response from a chain query
type ChainQueryResponse struct {
    // Chain is the chain information
    Chain *protocol.Chain `json:"chain"`
    
    // Entry is the entry information
    Entry *protocol.IndexEntry `json:"entry,omitempty"`
    
    // Entries is the entries information
    Entries []*protocol.IndexEntry `json:"entries,omitempty"`
}
```

### TransactionQueryResponse

```go
// TransactionQueryResponse is the response from a transaction query
type TransactionQueryResponse struct {
    // Transaction is the transaction information
    Transaction *protocol.Transaction `json:"transaction"`
    
    // Status is the transaction status
    Status *protocol.TransactionStatus `json:"status"`
    
    // Receipts are the transaction receipts
    Receipts []*protocol.Receipt `json:"receipts,omitempty"`
}
```

### ValidatorInfo

```go
// ValidatorInfo is information about a validator
type ValidatorInfo struct {
    // ID is the validator ID
    ID string `json:"id"`
    
    // Partition is the partition name
    Partition string `json:"partition"`
    
    // Address is the validator address
    Address string `json:"address"`
    
    // PublicKey is the validator public key
    PublicKey []byte `json:"publicKey"`
    
    // Addresses are the validator addresses
    Addresses []string `json:"addresses"`
}
```

### SubmitResponse

```go
// SubmitResponse is the response from a transaction submission
type SubmitResponse struct {
    // TransactionHash is the hash of the transaction
    TransactionHash []byte `json:"transactionHash"`
    
    // Code is the response code
    Code uint64 `json:"code"`
    
    // Message is the response message
    Message string `json:"message,omitempty"`
}
```

## URL Handling

The debug tools use the Accumulate URL package to handle URLs:

```go
// ParseUrl parses a URL string into a URL object
func ParseUrl(urlStr string) *url.URL

// PartitionUrl creates a URL for a partition
func PartitionUrl(partition string) *url.URL

// JoinPath joins a path to a URL
func (u *URL) JoinPath(path ...string) *URL
```

**Example**:
```go
// Parse a URL
url := protocol.ParseUrl("acc://myaccount.acme")

// Create a partition URL
url := protocol.PartitionUrl("bvn-apollo")

// Join a path to a URL
url := protocol.ParseUrl("acc://myaccount.acme").JoinPath("data")
```

## Error Handling

The debug tools use the standard Go error handling approach, with some additional error wrapping:

```go
// Wrap wraps an error with a message
func Wrap(err error, message string) error

// Wrapf wraps an error with a formatted message
func Wrapf(err error, format string, args ...interface{}) error

// Unwrap unwraps an error
func Unwrap(err error) error
```

**Example**:
```go
// Wrap an error
err = errors.Wrap(err, "failed to query account")

// Wrap an error with a formatted message
err = errors.Wrapf(err, "failed to query account %s", url)

// Unwrap an error
cause := errors.Unwrap(err)
```

## API Usage Patterns

### Basic Query Pattern

```go
// Basic query pattern
func basicQuery(ctx context.Context, client api.Client, url *url.URL) error {
    // Execute the query
    resp, err := client.Query(ctx, "account", url.String())
    if err != nil {
        return fmt.Errorf("failed to query account %s: %w", url, err)
    }
    
    // Check for nil response
    if resp == nil {
        return fmt.Errorf("received nil response when querying account %s", url)
    }
    
    // Check for error
    if resp.Error != "" {
        return fmt.Errorf("error querying account %s: %s", url, resp.Error)
    }
    
    // Parse the response
    var account protocol.Account
    err = json.Unmarshal(resp.Result, &account)
    if err != nil {
        return fmt.Errorf("failed to unmarshal account %s: %w", url, err)
    }
    
    return nil
}
```

### Retry Pattern

```go
// Retry pattern
func retryQuery(ctx context.Context, client api.Client, url *url.URL, maxRetries int) (*protocol.Account, error) {
    var lastErr error
    
    for i := 0; i < maxRetries; i++ {
        // Execute the query
        account, err := client.QueryAccount(ctx, url)
        if err == nil {
            return account, nil
        }
        
        // Save the error
        lastErr = err
        
        // Wait before retrying
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        case <-time.After(time.Second * time.Duration(i+1)):
            // Exponential backoff
        }
    }
    
    return nil, fmt.Errorf("failed to query account %s after %d retries: %w", url, maxRetries, lastErr)
}
```

### Fallback Pattern

```go
// Fallback pattern
func fallbackQuery(ctx context.Context, clients []api.Client, url *url.URL) (*protocol.Account, error) {
    var lastErr error
    
    for _, client := range clients {
        // Execute the query
        account, err := client.QueryAccount(ctx, url)
        if err == nil {
            return account, nil
        }
        
        // Save the error
        lastErr = err
    }
    
    return nil, fmt.Errorf("failed to query account %s from all clients: %w", url, lastErr)
}
```

## See Also

- [Command Reference](./command-reference.md): Reference for command-line commands
- [Configuration Reference](./configuration-reference.md): Reference for configuration options
- [Getting Started Guide](../guides/getting-started.md): Basic usage of the debug tools
- [Troubleshooting Guide](../guides/troubleshooting.md): Solutions to common issues
