# API Reference

[![Go Documentation](https://img.shields.io/badge/go-documentation-blue.svg)](https://pkg.go.dev)
[![API Version](https://img.shields.io/badge/api-v1.0-green.svg)](#versioning)
[![Stability](https://img.shields.io/badge/stability-stable-brightgreen.svg)](#stability-guarantees)

Complete API reference for the Accumulate Lite Client, providing detailed documentation for all public interfaces, methods, and data structures.

## 📋 Table of Contents

- [Client Creation](#client-creation)
- [Account Operations](#account-operations)
- [Proof Operations](#proof-operations)
- [Cache Management](#cache-management)
- [Configuration](#configuration)
- [Data Structures](#data-structures)
- [Error Handling](#error-handling)
- [Examples](#examples)

## 🚀 Client Creation

### NewClient

Creates a new lite client instance with the specified configuration.

```go
func NewClient(config *Config) (*Client, error)
```

**Parameters:**
- `config *Config`: Client configuration (see [Configuration](#configuration))

**Returns:**
- `*Client`: Configured lite client instance
- `error`: Configuration validation error, if any

**Example:**
```go
// Create client with mainnet configuration
client, err := NewClient(DefaultMainnetConfig())
if err != nil {
    log.Fatal("Failed to create client:", err)
}
defer client.Close()
```

### NewClientWithNetwork

Creates a client with predefined network configuration.

```go
func NewClientWithNetwork(network string) (*Client, error)
```

**Parameters:**
- `network string`: Network name (`"mainnet"`, `"testnet"`, `"devnet"`)

**Returns:**
- `*Client`: Configured lite client instance
- `error`: Invalid network error, if any

**Example:**
```go
// Quick setup for testnet
client, err := NewClientWithNetwork("testnet")
if err != nil {
    log.Fatal("Failed to create testnet client:", err)
}
```

### NewCustomClient

Creates a client with custom server endpoints.

```go
func NewCustomClient(serverURL string, options ...ClientOption) (*Client, error)
```

**Parameters:**
- `serverURL string`: Primary Accumulate server URL
- `options ...ClientOption`: Additional configuration options

**Returns:**
- `*Client`: Configured lite client instance
- `error`: Configuration error, if any

**Example:**
```go
// Custom client with specific endpoint
client, err := NewCustomClient("https://my-node.example.com:443",
    WithCacheTTL(10*time.Minute),
    WithMaxConcurrency(20),
)
```

## 🏦 Account Operations

### ProcessADIs

Processes multiple ADIs and generates cryptographic proofs for all associated accounts.

```go
func (c *Client) ProcessADIs(ctx context.Context, adiURLs []string) ([]*VerifiedAccount, error)
```

**Parameters:**
- `ctx context.Context`: Request context for cancellation and timeouts
- `adiURLs []string`: List of ADI URLs to process

**Returns:**
- `[]*VerifiedAccount`: List of verified accounts with proofs
- `error`: Processing error, if any

**Example:**
```go
adis := []string{
    "acc://my-adi.acme",
    "acc://another-adi.acme",
}

accounts, err := client.ProcessADIs(ctx, adis)
if err != nil {
    log.Printf("Failed to process ADIs: %v", err)
    return
}

for _, account := range accounts {
    fmt.Printf("Account: %s, Valid: %t\n", 
        account.URL, account.ProofValid)
}
```

### GetAccountInfo

Retrieves detailed information for a specific account.

```go
func (c *Client) GetAccountInfo(ctx context.Context, accountURL string) (*AccountInfo, error)
```

**Parameters:**
- `ctx context.Context`: Request context
- `accountURL string`: Account URL to query

**Returns:**
- `*AccountInfo`: Account information and metadata
- `error`: Query error, if any

**Example:**
```go
info, err := client.GetAccountInfo(ctx, "acc://my-adi.acme/token")
if err != nil {
    log.Printf("Failed to get account info: %v", err)
    return
}

fmt.Printf("Account Type: %s\n", info.Type)
fmt.Printf("Balance: %s\n", info.Balance)
fmt.Printf("Last Updated: %s\n", info.LastUpdated)
```

### GetTransactionHistory

Retrieves transaction history for an account.

```go
func (c *Client) GetTransactionHistory(ctx context.Context, accountURL string, limit int) ([]*TransactionInfo, error)
```

**Parameters:**
- `ctx context.Context`: Request context
- `accountURL string`: Account URL to query
- `limit int`: Maximum number of transactions to return

**Returns:**
- `[]*TransactionInfo`: List of transaction information
- `error`: Query error, if any

**Example:**
```go
transactions, err := client.GetTransactionHistory(ctx, 
    "acc://my-adi.acme/token", 50)
if err != nil {
    log.Printf("Failed to get transaction history: %v", err)
    return
}

for _, tx := range transactions {
    fmt.Printf("TX: %s, Amount: %s, Status: %s\n",
        tx.TxID, tx.Amount, tx.Status)
}
```

## 🔐 Proof Operations

### ValidateProof

Validates a cryptographic proof for account state.

```go
func (c *Client) ValidateProof(ctx context.Context, accountURL string, knownRoot []byte) (*ProofValidationResult, error)
```

**Parameters:**
- `ctx context.Context`: Request context
- `accountURL string`: Account URL to validate
- `knownRoot []byte`: Known root hash for validation

**Returns:**
- `*ProofValidationResult`: Validation result with details
- `error`: Validation error, if any

**Example:**
```go
// Validate proof against known root
result, err := client.ValidateProof(ctx, 
    "acc://my-adi.acme/token", knownRootHash)
if err != nil {
    log.Printf("Proof validation failed: %v", err)
    return
}

if result.Valid {
    fmt.Printf("✓ Proof valid: %s\n", result.Details)
} else {
    fmt.Printf("✗ Proof invalid: %s\n", result.Error)
}
```

### GenerateProof

Generates a cryptographic proof for account state.

```go
func (c *Client) GenerateProof(ctx context.Context, accountURL string) (*VerifiedAccount, error)
```

**Parameters:**
- `ctx context.Context`: Request context
- `accountURL string`: Account URL to generate proof for

**Returns:**
- `*VerifiedAccount`: Account with generated proof
- `error`: Proof generation error, if any

**Example:**
```go
verified, err := client.GenerateProof(ctx, "acc://my-adi.acme/token")
if err != nil {
    log.Printf("Failed to generate proof: %v", err)
    return
}

fmt.Printf("Proof generated: %t\n", verified.ProofValid)
fmt.Printf("Proof size: %d bytes\n", len(verified.Proof))
```

## 🗄️ Cache Management

### GetCacheStats

Retrieves current cache statistics.

```go
func (c *Client) GetCacheStats() *CacheStats
```

**Returns:**
- `*CacheStats`: Current cache statistics

**Example:**
```go
stats := client.GetCacheStats()
fmt.Printf("Cache entries: %d\n", stats.TotalEntries)
fmt.Printf("Hit rate: %.2f%%\n", stats.HitRate*100)
fmt.Printf("Memory usage: %d bytes\n", stats.MemoryUsage)
```

### ClearCache

Clears all cached data.

```go
func (c *Client) ClearCache() error
```

**Returns:**
- `error`: Cache clear error, if any

**Example:**
```go
if err := client.ClearCache(); err != nil {
    log.Printf("Failed to clear cache: %v", err)
} else {
    fmt.Println("Cache cleared successfully")
}
```

### UpdateNetworkEndpoint

Updates the network endpoint at runtime.

```go
func (c *Client) UpdateNetworkEndpoint(serverURL string) error
```

**Parameters:**
- `serverURL string`: New server URL

**Returns:**
- `error`: Update error, if any

**Example:**
```go
// Switch to different endpoint
err := client.UpdateNetworkEndpoint("https://backup-node.example.com:443")
if err != nil {
    log.Printf("Failed to update endpoint: %v", err)
} else {
    fmt.Println("Endpoint updated successfully")
}
```

## ⚙️ Configuration

### Config Structure

```go
type Config struct {
    Network NetworkConfig `json:"network"`
    Cache   CacheConfig   `json:"cache"`
    API     APIConfig     `json:"api"`
    Debug   DebugConfig   `json:"debug"`
}
```

### NetworkConfig

```go
type NetworkConfig struct {
    ServerURL       string        `json:"server_url"`
    BackupServers   []string      `json:"backup_servers,omitempty"`
    Timeout         time.Duration `json:"timeout"`
    RetryAttempts   int           `json:"retry_attempts"`
    RetryDelay      time.Duration `json:"retry_delay"`
}
```

### CacheConfig

```go
type CacheConfig struct {
    TTL             time.Duration `json:"ttl"`
    MaxEntries      int           `json:"max_entries"`
    PersistentCache bool          `json:"persistent_cache"`
    CacheDir        string        `json:"cache_dir,omitempty"`
}
```

### APIConfig

```go
type APIConfig struct {
    MaxConcurrency int           `json:"max_concurrency"`
    RateLimit      int           `json:"rate_limit"`
    RequestTimeout time.Duration `json:"request_timeout"`
}
```

### DebugConfig

```go
type DebugConfig struct {
    EnableLogging bool   `json:"enable_logging"`
    LogLevel      string `json:"log_level"`
    VerboseErrors bool   `json:"verbose_errors"`
}
```

## 📊 Data Structures

### VerifiedAccount

```go
type VerifiedAccount struct {
    URL         string          `json:"url"`
    AccountData *AccountData    `json:"account_data"`
    Proof       []byte          `json:"proof"`
    ProofValid  bool            `json:"proof_valid"`
    Timestamp   time.Time       `json:"timestamp"`
    Cached      bool            `json:"cached"`
}
```

### AccountInfo

```go
type AccountInfo struct {
    URL         string                 `json:"url"`
    Type        string                 `json:"type"`
    Balance     string                 `json:"balance,omitempty"`
    TokenURL    string                 `json:"token_url,omitempty"`
    LastUpdated time.Time              `json:"last_updated"`
    Metadata    map[string]interface{} `json:"metadata,omitempty"`
}
```

### TransactionInfo

```go
type TransactionInfo struct {
    TxID      string                 `json:"tx_id"`
    Type      string                 `json:"type"`
    Status    string                 `json:"status"`
    Timestamp time.Time              `json:"timestamp"`
    Amount    string                 `json:"amount,omitempty"`
    From      string                 `json:"from,omitempty"`
    To        string                 `json:"to,omitempty"`
    Data      map[string]interface{} `json:"data,omitempty"`
}
```

### ProofValidationResult

```go
type ProofValidationResult struct {
    Valid      bool      `json:"valid"`
    Details    string    `json:"details"`
    Error      string    `json:"error,omitempty"`
    Timestamp  time.Time `json:"timestamp"`
    ProofSize  int       `json:"proof_size"`
}
```

### CacheStats

```go
type CacheStats struct {
    TotalEntries   int           `json:"total_entries"`
    HitRate        float64       `json:"hit_rate"`
    MemoryUsage    int64         `json:"memory_usage"`
    LastCleared    time.Time     `json:"last_cleared"`
    OldestEntry    time.Time     `json:"oldest_entry"`
    AverageAge     time.Duration `json:"average_age"`
}
```

## ❌ Error Handling

### Error Types

The lite client defines specific error types for different failure scenarios:

```go
// Network-related errors
type NetworkError struct {
    URL     string
    Message string
    Cause   error
}

// Validation errors
type ValidationError struct {
    AccountURL string
    Reason     string
    Details    map[string]interface{}
}

// Cache errors
type CacheError struct {
    Operation string
    Message   string
}

// Configuration errors
type ConfigError struct {
    Field   string
    Value   interface{}
    Message string
}
```

### Error Checking Patterns

```go
// Check for specific error types
if netErr, ok := err.(*NetworkError); ok {
    log.Printf("Network error for %s: %s", netErr.URL, netErr.Message)
    // Handle network-specific error
}

// Check for validation errors
if valErr, ok := err.(*ValidationError); ok {
    log.Printf("Validation failed for %s: %s", valErr.AccountURL, valErr.Reason)
    // Handle validation-specific error
}

// Generic error handling
if err != nil {
    log.Printf("Operation failed: %v", err)
    return
}
```

## 📚 Examples

### Complete Workflow Example

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"
    
    liteclient "gitlab.com/accumulatenetwork/accumulate/exp/lite-client"
)

func main() {
    // Create client with custom configuration
    config := liteclient.DefaultMainnetConfig()
    config.Cache.TTL = 15 * time.Minute
    config.API.MaxConcurrency = 10
    
    client, err := liteclient.NewClient(config)
    if err != nil {
        log.Fatal("Failed to create client:", err)
    }
    defer client.Close()
    
    ctx := context.Background()
    
    // Process multiple ADIs
    adis := []string{
        "acc://my-company.acme",
        "acc://partner-org.acme",
    }
    
    accounts, err := client.ProcessADIs(ctx, adis)
    if err != nil {
        log.Printf("Failed to process ADIs: %v", err)
        return
    }
    
    // Display results
    for _, account := range accounts {
        fmt.Printf("Account: %s\n", account.URL)
        fmt.Printf("  Proof Valid: %t\n", account.ProofValid)
        fmt.Printf("  Cached: %t\n", account.Cached)
        fmt.Printf("  Timestamp: %s\n", account.Timestamp)
        
        // Get detailed account info
        info, err := client.GetAccountInfo(ctx, account.URL)
        if err != nil {
            log.Printf("Failed to get account info: %v", err)
            continue
        }
        
        fmt.Printf("  Type: %s\n", info.Type)
        if info.Balance != "" {
            fmt.Printf("  Balance: %s\n", info.Balance)
        }
        
        // Get recent transactions
        txs, err := client.GetTransactionHistory(ctx, account.URL, 10)
        if err != nil {
            log.Printf("Failed to get transactions: %v", err)
            continue
        }
        
        fmt.Printf("  Recent Transactions: %d\n", len(txs))
        for _, tx := range txs {
            fmt.Printf("    %s: %s (%s)\n", tx.TxID, tx.Type, tx.Status)
        }
        
        fmt.Println()
    }
    
    // Display cache statistics
    stats := client.GetCacheStats()
    fmt.Printf("Cache Statistics:\n")
    fmt.Printf("  Total Entries: %d\n", stats.TotalEntries)
    fmt.Printf("  Hit Rate: %.2f%%\n", stats.HitRate*100)
    fmt.Printf("  Memory Usage: %d bytes\n", stats.MemoryUsage)
}
```

### Batch Processing Example

```go
func processBatchAccounts(client *liteclient.Client, accountURLs []string) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer cancel()
    
    // Process accounts in batches
    batchSize := 10
    for i := 0; i < len(accountURLs); i += batchSize {
        end := i + batchSize
        if end > len(accountURLs) {
            end = len(accountURLs)
        }
        
        batch := accountURLs[i:end]
        fmt.Printf("Processing batch %d-%d...\n", i+1, end)
        
        // Process batch
        results := make([]*liteclient.VerifiedAccount, len(batch))
        for j, url := range batch {
            result, err := client.GenerateProof(ctx, url)
            if err != nil {
                log.Printf("Failed to process %s: %v", url, err)
                continue
            }
            results[j] = result
        }
        
        // Validate results
        for _, result := range results {
            if result != nil && result.ProofValid {
                fmt.Printf("✓ %s: Valid proof\n", result.URL)
            } else if result != nil {
                fmt.Printf("✗ %s: Invalid proof\n", result.URL)
            }
        }
    }
}
```

## 🔄 Versioning

The API follows semantic versioning (SemVer):

- **Major version**: Breaking changes to public API
- **Minor version**: New features, backward compatible
- **Patch version**: Bug fixes, backward compatible

Current version: **v1.0.0**

## 🛡️ Stability Guarantees

- **Stable APIs**: All documented public APIs are stable
- **Backward Compatibility**: Minor versions maintain compatibility
- **Deprecation Policy**: 6-month notice for breaking changes
- **Migration Guides**: Provided for major version upgrades

## 📞 Support

For API questions and support:

- **Documentation**: [docs/](../docs/)
- **Examples**: [examples/](../examples/)
- **Issues**: [GitHub Issues](https://github.com/AccumulateNetwork/accumulate/issues)
- **Community**: [Discord](https://discord.gg/accumulate)
