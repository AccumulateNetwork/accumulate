# Light Client Package

This package provides a modular light client for the Accumulate network that can pull down various types of accounts (ADIs, token accounts, data accounts, key books) with cryptographic proofs.

## Features

- **Account Querying**: Query any account by URL and retrieve structured data
- **Account Types**: Support for ADIs, token accounts, data accounts, key books, and key pages
- **Operators Support**: Convenient methods for retrieving network operators keybook
- **Staking Support**: Methods for retrieving staking registry and staking accounts
- **Batch Operations**: Retrieve multiple accounts in a single operation
- **Flexible API**: Use JSON-RPC 2.0 queries with HTTP POST requests

## Usage

### Basic Client Setup

```go
import "gitlab.com/AccumulateNetwork/accumulate/pkg/lightclient"

client, err := lightclient.NewClient("https://mainnet.accumulatenetwork.io")
if err != nil {
    log.Fatal(err)
}
```

### Server URL Shortcuts

The client supports several server URL shortcuts:

- `local` - http://127.0.1.1:26660
- `testnet` - https://testnet.accumulatenetwork.io
- `beta` - https://beta.testnet.accumulatenetwork.io
- `canary` - https://canary.testnet.accumulatenetwork.io
- `mainnet` - http://apollo-mainnet.accumulate.defidevs.io:16595
- `mainnet-ssl` - https://mainnet.accumulatenetwork.io

### Querying Accounts

#### Generic Account Query

```go
ctx := context.Background()
resp, err := client.Query(ctx, "acc://example.acme")
if err != nil {
    log.Fatal(err)
}

accountType, _ := resp.GetType()
data, _ := resp.GetData()
```

#### Specific Account Types

```go
// Get an ADI
adi, err := client.GetADI(ctx, "acc://example.acme")

// Get a token account
tokenAccount, err := client.GetTokenAccount(ctx, "acc://example.acme/tokens")

// Get a data account
dataAccount, err := client.GetDataAccount(ctx, "acc://example.acme/data")

// Get a key book
keyBook, err := client.GetKeyBook(ctx, "acc://example.acme/book")

// Get a key page
keyPage, err := client.GetKeyPage(ctx, "acc://example.acme/book/1")
```

### Operators KeyBook

```go
// Get complete operators information
operators, err := client.GetOperators(ctx)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("KeyBook Threshold: %d\n", operators.KeyBook.Threshold)
fmt.Printf("Number of Key Pages: %d\n", len(operators.KeyPages))
fmt.Printf("Total Keys: %d\n", len(operators.AllKeys))
```

### Staking Accounts

```go
// Get staking registry URLs
stakingURLs, err := client.GetStakingRegistry(ctx)

// Get staking accounts with details
stakingURLs, stakingAccounts, err := client.GetStakingAccounts(ctx)
```

### Batch Operations

```go
// Get information for multiple accounts
accountURLs := []string{
    "acc://example.acme",
    "acc://another.acme",
    "acc://third.acme/tokens",
}

accounts, err := client.BatchGetAccounts(ctx, accountURLs)
```

## Account Types

### ADI (Accumulate Digital Identifier)

```go
type ADI struct {
    URL         string
    Type        string
    Authorities []string
    Data        map[string]interface{}
}
```

### Token Account

```go
type TokenAccount struct {
    URL         string
    Type        string
    Balance     int64
    TokenURL    string
    Authorities []string
    Data        map[string]interface{}
}
```

### Data Account

```go
type DataAccount struct {
    URL         string
    Type        string
    Entries     []*DataEntry
    Authorities []string
    Data        map[string]interface{}
}

type DataEntry struct {
    Hash string
    Data []byte
}
```

### Key Book

```go
type KeyBook struct {
    URL       string
    Type      string
    Threshold int
    Pages     []string
    Data      map[string]interface{}
}
```

### Key Page

```go
type KeyPage struct {
    URL       string
    Type      string
    Threshold int
    Keys      []string
    Data      map[string]interface{}
}
```

## Examples

### Light Client Tool

The `tools/light-client` directory contains a command-line tool that uses this package to collect the operators keybook:

```bash
go run tools/light-client/main.go mainnet
```

### Staking Client Example

The `examples/staking-client` directory contains an example application that retrieves staking account information:

```bash
go run examples/staking-client/main.go mainnet
```

## API Details

### JSON-RPC 2.0 Format

The client uses JSON-RPC 2.0 queries with the following format:

```json
{
    "jsonrpc": "2.0",
    "method": "query",
    "params": {
        "url": "acc://example.acme"
    },
    "id": 1
}
```

### Response Format

Responses follow the standard JSON-RPC 2.0 format:

```json
{
    "jsonrpc": "2.0",
    "result": {
        "type": "identity",
        "data": {
            "url": "acc://example.acme",
            "type": "identity",
            "authorities": ["acc://example.acme/book"]
        }
    },
    "id": 1
}
```

## Error Handling

The package provides structured error handling:

- Network errors are wrapped with context
- JSON-RPC errors are extracted and returned
- HTTP errors include status codes and response bodies
- Parsing errors include details about the expected format

## Future Enhancements

- Cryptographic proof verification
- Caching mechanisms
- Parallel query optimization
- WebSocket support for real-time updates
- Transaction submission capabilities
