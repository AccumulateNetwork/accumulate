# GitLab Repository Review: Accumulate SDK

## Repository Information

**Location:** `gitlab.com/accumulatenetwork/accumulate`
**Version Used:** v1.4.2
**License:** MIT
**Language:** Go

## Key Findings from SDK Review

### 1. Correct API Usage Pattern

**What the SDK Provides:**
- `pkg/api/v3/jsonrpc` - JSON-RPC client implementation
- Client created with `jsonrpc.NewClient(server string)`
- No `api.Dial()` - that was a mistaken assumption

**Correct Usage (from jsonrpc/client.go:41-46):**
```go
import "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"

client := jsonrpc.NewClient("https://mainnet.accumulatenetwork.io/v3")
client.Client.Timeout = 15 * time.Second  // Default timeout
```

**The client implements all services:**
- `api.NodeService`
- `api.ConsensusService`
- `api.NetworkService`
- `api.SnapshotService`
- `api.MetricsService`
- `api.Querier`
- `api.Submitter`
- `api.Validator`
- `api.Faucet`

### 2. Query API - The Right Way

**From client.go:72-75:**
```go
func (c *Client) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	req := &message.QueryRequest{Scope: scope, Query: query}
	return sendRequestUnmarshalWith(c, ctx, "query", req, api.UnmarshalRecordJSON)
}
```

**Key Insights:**
1. `scope` is a `*url.URL` from `pkg/url` package (NOT standard lib url)
2. `query` is typed as `api.Query` interface (union type)
3. Returns `api.Record` interface (union type)
4. Uses `api.UnmarshalRecordJSON` for proper deserialization

**Query types are structs, not string maps:**
```go
// From queries.yml
type DefaultQuery struct {
    Url *url.URL
}

type ChainQuery struct {
    Name *string
    Index *uint64
    Entry *[]byte
    Range *RangeOptions
}

type DataQuery struct {
    Index *uint64
    Entry *[]byte
    Range *RangeOptions
}
```

### 3. Submit/Transaction API - The Right Way

**From client.go:77-79:**
```go
func (c *Client) Submit(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	req := &message.SubmitRequest{Envelope: envelope, SubmitOptions: opts}
	return sendRequestUnmarshalAs[[]*api.Submission](c, ctx, "submit", req)
}
```

**Key Insights:**
1. Takes a `*messaging.Envelope` (from `pkg/types/messaging`)
2. Envelope contains `Transaction` and `Signatures`
3. Returns array of `*api.Submission` (not just hash)

**Transaction Building (from example_test.md:20-23):**
```go
build.Transaction().For(alice, "tokens").
    SendTokens(123, 0).To(bob, "tokens").
    SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey)
```

This uses a builder pattern!

### 4. Network Services - The Right Way

**All network methods use options structs (client.go:48-70):**
```go
NodeInfo(ctx, api.NodeInfoOptions) (*api.NodeInfo, error)
ConsensusStatus(ctx, api.ConsensusStatusOptions) (*api.ConsensusStatus, error)
NetworkStatus(ctx, api.NetworkStatusOptions) (*api.NetworkStatus, error)
Metrics(ctx, api.MetricsOptions) (*api.Metrics, error)
Faucet(ctx, *url.URL, api.FaucetOptions) (*api.Submission, error)
```

**Not:**
- ❌ String-based method names
- ❌ `map[string]interface{}` parameters
- ❌ Generic `interface{}` returns

### 5. URL Handling

**Critical:** Accumulate uses custom URL types, not Go's standard library:
```go
import "gitlab.com/accumulatenetwork/accumulate/pkg/url"

accountUrl, err := url.Parse("acc://alice.acme/tokens")
```

**NOT:**
```go
import "net/url"  // WRONG
```

### 6. Architecture Insights (from README.md)

**Service Design:**
- Each service has ONE method (intentional design)
- Services can be implemented independently
- Not obligated to implement all services
- Easy middleware implementation

**Transports:**
- JSON-RPC (what we need)
- P2P (libp2p network)
- WebSocket (incomplete)

**The SDK is designed for:**
- Validators expose P2P only
- API nodes expose HTTP/WS and proxy to P2P
- Clear separation of concerns

## What We Got Wrong (Confirmed)

### 1. **Not Using SDK Client** - CRITICAL

**We did:**
```go
http.Post(c.endpoint, "application/json", bytes.NewReader(data))
```

**Should be:**
```go
import "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
client := jsonrpc.NewClient(endpoint)
client.Query(ctx, scope, query)
```

### 2. **Wrong Query Format** - CRITICAL

**We did:**
```go
reqBody := map[string]interface{}{
    "queryType": "chain",
    "name": chainName,
}
```

**Should be:**
```go
import "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"

query := &api.ChainQuery{
    Name: &chainName,
}
result, err := client.Query(ctx, scopeUrl, query)
```

### 3. **Wrong URL Handling** - CRITICAL

**We did:**
```go
"url": accountURL  // string
```

**Should be:**
```go
import "gitlab.com/accumulatenetwork/accumulate/pkg/url"

accountUrl, _ := url.Parse("acc://alice.acme/tokens")
client.Query(ctx, accountUrl, query)
```

### 4. **Transaction Building Wrong** - CRITICAL

**We did:**
```go
tx := map[string]interface{}{
    "header": map[string]interface{}{
        "principal": from,
    },
    "body": map[string]interface{}{
        "type": "sendTokens",
        ...
    },
}
```

**Should use:**
```go
import "gitlab.com/accumulatenetwork/accumulate/test/testing/build"

build.Transaction().For(from).
    SendTokens(amount, 0).To(to).
    SignWith(...).PrivateKey(privateKey)
```

OR construct protocol types directly:
```go
import "gitlab.com/accumulatenetwork/accumulate/protocol"

body := &protocol.SendTokens{
    To: []*protocol.TokenRecipient{{
        Url: toUrl,
        Amount: *protocol.NewBigInt(amount),
    }},
}
txn := &protocol.Transaction{
    Header: protocol.TransactionHeader{Principal: fromUrl},
    Body: body,
}
```

## Correct Implementation Pattern

### Complete Example:

```go
package main

import (
    "context"
    "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
    "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
    "gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func main() {
    // 1. Create client
    client := jsonrpc.NewClient("https://mainnet.accumulatenetwork.io/v3")

    // 2. Parse URL
    accountUrl, _ := url.Parse("acc://alice.acme/tokens")

    // 3. Create typed query
    query := &api.DefaultQuery{Url: accountUrl}

    // 4. Execute query
    ctx := context.Background()
    record, err := client.Query(ctx, accountUrl, query)

    // 5. Type assert result
    if accountRecord, ok := record.(*api.AccountRecord); ok {
        // Use accountRecord
    }
}
```

## Action Items

1. **Rewrite client package** - Use `jsonrpc.Client`
2. **Import correct packages** - `pkg/url`, `pkg/api/v3`, `protocol`
3. **Use typed queries** - `api.ChainQuery`, etc.
4. **Use builder pattern** - For transactions
5. **Test against testnet** - Validate everything works

## Estimated Rework

**Previous estimate:** 9-13 hours
**Revised with SDK knowledge:** 6-8 hours

The SDK provides everything we need. We just need to use it correctly.
