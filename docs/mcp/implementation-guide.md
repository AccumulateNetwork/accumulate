# Accumulate MCP Server Implementation Guide

## Overview

This guide provides step-by-step instructions for implementing the Accumulate MCP server in Go using the mark3labs/mcp-go SDK.

## Prerequisites

### Required Knowledge
- Go programming (version 1.21+)
- JSON-RPC protocol basics
- MCP protocol fundamentals
- Basic blockchain concepts

### Development Environment
```bash
# Install Go 1.21 or later
go version

# Clone Accumulate repository
git clone https://gitlab.com/AccumulateNetwork/accumulate.git
cd accumulate

# Install dependencies
go mod download
```

### MCP SDK
```bash
# Add MCP Go SDK dependency
go get github.com/mark3labs/mcp-go@latest
```

## Project Structure

```
tools/accumulate-mcp/
├── main.go                 # Entry point
├── server/
│   ├── server.go          # MCP server implementation
│   ├── tools.go           # Tool handlers
│   ├── resources.go       # Resource handlers
│   └── prompts.go         # Prompt handlers
├── client/
│   ├── accumulate.go      # Accumulate API client wrapper
│   └── cache.go           # Caching layer
├── config/
│   ├── config.go          # Configuration types
│   └── defaults.go        # Default configuration
├── handlers/
│   ├── network.go         # Network tool handlers
│   ├── query.go           # Query tool handlers
│   ├── transaction.go     # Transaction tool handlers
│   └── events.go          # Event subscription handlers
├── builders/
│   ├── transaction.go     # Transaction builders
│   └── envelope.go        # Envelope helpers
├── errors/
│   └── errors.go          # Error types and handling
└── README.md              # User documentation
```

## Step 1: Initialize MCP Server

### File: `tools/accumulate-mcp/main.go`

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/mark3labs/mcp-go/server"
    "gitlab.com/AccumulateNetwork/accumulate/tools/accumulate-mcp/config"
    mcpserver "gitlab.com/AccumulateNetwork/accumulate/tools/accumulate-mcp/server"
)

func main() {
    // Load configuration
    cfg, err := config.Load()
    if err != nil {
        log.Fatalf("Failed to load configuration: %v", err)
    }

    // Create MCP server
    s := server.NewMCPServer(
        "accumulate-mcp",
        "1.0.0",
        server.WithLogging(),
    )

    // Create Accumulate MCP handler
    handler, err := mcpserver.New(cfg)
    if err != nil {
        log.Fatalf("Failed to create handler: %v", err)
    }

    // Register tools, resources, and prompts
    if err := handler.Register(s); err != nil {
        log.Fatalf("Failed to register handlers: %v", err)
    }

    // Start server (stdio transport)
    ctx := context.Background()
    if err := s.Serve(ctx, os.Stdin, os.Stdout); err != nil {
        log.Fatalf("Server error: %v", err)
    }
}
```

## Step 2: Configuration Management

### File: `tools/accumulate-mcp/config/config.go`

```go
package config

import (
    "encoding/json"
    "os"
    "time"
)

type Config struct {
    // Network settings
    Network   string   `json:"network"`            // "MainNet", "TestNet", "DevNet"
    Endpoints []string `json:"endpoints"`          // API endpoints
    Timeout   Duration `json:"timeout"`            // Request timeout

    // Feature flags
    EnableV2Compat            bool `json:"enable_v2_compat"`
    EnableTransactionBuilding bool `json:"enable_transaction_building"`
    EnableFaucet              bool `json:"enable_faucet"`
    EnableSnapshots           bool `json:"enable_snapshots"`
    EnableEvents              bool `json:"enable_events"`

    // Security
    ReadOnly                 bool `json:"read_only"`
    AllowTransactionSubmit   bool `json:"allow_transaction_submit"`
    RequireConfirmation      bool `json:"require_confirmation"`
    MaxQueryResults          int  `json:"max_query_results"`

    // Caching
    CacheNodeInfo   Duration `json:"cache_node_info"`    // Cache duration for node info
    CacheBlockchain Duration `json:"cache_blockchain"`   // Cache duration for blockchain data
}

type Duration struct {
    time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
    return json.Marshal(d.String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
    var v string
    if err := json.Unmarshal(b, &v); err != nil {
        return err
    }
    dur, err := time.ParseDuration(v)
    if err != nil {
        return err
    }
    d.Duration = dur
    return nil
}

func Load() (*Config, error) {
    // Try to load from environment or file
    configFile := os.Getenv("ACCUMULATE_MCP_CONFIG")
    if configFile == "" {
        return DefaultConfig(), nil
    }

    data, err := os.ReadFile(configFile)
    if err != nil {
        return nil, err
    }

    var cfg Config
    if err := json.Unmarshal(data, &cfg); err != nil {
        return nil, err
    }

    return &cfg, nil
}

func DefaultConfig() *Config {
    return &Config{
        Network: "MainNet",
        Endpoints: []string{
            "https://mainnet.accumulatenetwork.io/v3",
        },
        Timeout: Duration{30 * time.Second},

        EnableV2Compat:            true,
        EnableTransactionBuilding: true,
        EnableFaucet:              false,
        EnableSnapshots:           false,
        EnableEvents:              true,

        ReadOnly:               false,
        AllowTransactionSubmit: true,
        RequireConfirmation:    true,
        MaxQueryResults:        1000,

        CacheNodeInfo:   Duration{30 * time.Second},
        CacheBlockchain: Duration{5 * time.Second},
    }
}
```

## Step 3: Accumulate API Client Wrapper

### File: `tools/accumulate-mcp/client/accumulate.go`

```go
package client

import (
    "context"
    "fmt"
    "time"

    "gitlab.com/AccumulateNetwork/accumulate/pkg/api/v3"
    "gitlab.com/AccumulateNetwork/accumulate/pkg/client"
)

type Client struct {
    v3     *client.Client
    config *Config
    cache  *Cache
}

type Config struct {
    Endpoints []string
    Timeout   time.Duration
}

func New(cfg *Config) (*Client, error) {
    // Create V3 API client
    v3Client, err := client.New(cfg.Endpoints[0])
    if err != nil {
        return nil, fmt.Errorf("failed to create V3 client: %w", err)
    }

    return &Client{
        v3:     v3Client,
        config: cfg,
        cache:  NewCache(),
    }, nil
}

// NodeInfo returns node information
func (c *Client) NodeInfo(ctx context.Context, peerID string) (*api.NodeInfoResponse, error) {
    // Check cache first
    cacheKey := fmt.Sprintf("node-info:%s", peerID)
    if cached, ok := c.cache.Get(cacheKey); ok {
        return cached.(*api.NodeInfoResponse), nil
    }

    // Make API call
    req := &api.NodeInfoRequest{
        NodeInfoOptions: api.NodeInfoOptions{
            PeerID: peerID,
        },
    }

    resp, err := c.v3.NodeInfo(ctx, req)
    if err != nil {
        return nil, err
    }

    // Cache result
    c.cache.Set(cacheKey, resp, 30*time.Second)

    return resp, nil
}

// QueryAccount queries an account by URL
func (c *Client) QueryAccount(ctx context.Context, url string, opts *QueryOptions) (*api.AccountRecord, error) {
    req := &api.DefaultQuery{
        Url:            url,
        IncludeReceipt: opts.IncludeReceipt,
        Prove:          opts.Prove,
    }

    resp, err := c.v3.Query(ctx, req)
    if err != nil {
        return nil, err
    }

    // Type assertion to AccountRecord
    if record, ok := resp.(*api.AccountRecord); ok {
        return record, nil
    }

    return nil, fmt.Errorf("unexpected response type: %T", resp)
}

// QueryTransaction queries a transaction by ID
func (c *Client) QueryTransaction(ctx context.Context, txid string, opts *QueryOptions) (*api.TxnRecord, error) {
    req := &api.DefaultQuery{
        Url:            txid,
        IncludeReceipt: opts.IncludeReceipt,
        Prove:          opts.Prove,
    }

    if opts.Wait > 0 {
        req.Wait = opts.Wait
    }

    resp, err := c.v3.Query(ctx, req)
    if err != nil {
        return nil, err
    }

    if record, ok := resp.(*api.TxnRecord); ok {
        return record, nil
    }

    return nil, fmt.Errorf("unexpected response type: %T", resp)
}

// QueryChain queries chain entries
func (c *Client) QueryChain(ctx context.Context, url, chainName string, opts *ChainQueryOptions) (*api.ChainRecord, error) {
    req := &api.ChainQuery{
        Url:  url,
        Name: chainName,
        Range: &api.RangeOptions{
            Start: opts.Start,
            Count: opts.Count,
        },
        Expand:         opts.Expand,
        IncludeReceipt: opts.IncludeReceipt,
    }

    resp, err := c.v3.Query(ctx, req)
    if err != nil {
        return nil, err
    }

    if record, ok := resp.(*api.ChainRecord); ok {
        return record, nil
    }

    return nil, fmt.Errorf("unexpected response type: %T", resp)
}

// Submit submits a transaction envelope
func (c *Client) Submit(ctx context.Context, envelope []byte, checkOnly bool) (*api.SubmitResponse, error) {
    req := &api.SubmitRequest{
        Envelope:  envelope,
        CheckOnly: checkOnly,
    }

    return c.v3.Submit(ctx, req)
}

type QueryOptions struct {
    IncludeReceipt bool
    Prove          bool
    Wait           time.Duration
}

type ChainQueryOptions struct {
    Start          uint64
    Count          uint64
    Expand         bool
    IncludeReceipt bool
}

// Add more methods for other API calls...
```

## Step 4: Implement MCP Server Handler

### File: `tools/accumulate-mcp/server/server.go`

```go
package server

import (
    "context"
    "fmt"

    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"
    "gitlab.com/AccumulateNetwork/accumulate/tools/accumulate-mcp/client"
    "gitlab.com/AccumulateNetwork/accumulate/tools/accumulate-mcp/config"
)

type Server struct {
    config *config.Config
    client *client.Client
}

func New(cfg *config.Config) (*Server, error) {
    // Create Accumulate client
    accClient, err := client.New(&client.Config{
        Endpoints: cfg.Endpoints,
        Timeout:   cfg.Timeout.Duration,
    })
    if err != nil {
        return nil, fmt.Errorf("failed to create Accumulate client: %w", err)
    }

    return &Server{
        config: cfg,
        client: accClient,
    }, nil
}

func (s *Server) Register(mcpServer *server.MCPServer) error {
    // Register tools
    if err := s.registerTools(mcpServer); err != nil {
        return fmt.Errorf("failed to register tools: %w", err)
    }

    // Register resources
    if err := s.registerResources(mcpServer); err != nil {
        return fmt.Errorf("failed to register resources: %w", err)
    }

    // Register prompts
    if err := s.registerPrompts(mcpServer); err != nil {
        return fmt.Errorf("failed to register prompts: %w", err)
    }

    return nil
}

func (s *Server) registerTools(mcpServer *server.MCPServer) error {
    // Network tools
    mcpServer.AddTool(mcp.Tool{
        Name:        "accumulate_node_info",
        Description: "Get information about an Accumulate network node",
        InputSchema: mcp.ToolInputSchema{
            Type: "object",
            Properties: map[string]interface{}{
                "peer_id": map[string]interface{}{
                    "type":        "string",
                    "description": "Specific peer ID to query (optional)",
                },
            },
        },
    }, s.handleNodeInfo)

    mcpServer.AddTool(mcp.Tool{
        Name:        "accumulate_query_account",
        Description: "Query account information by URL",
        InputSchema: mcp.ToolInputSchema{
            Type: "object",
            Properties: map[string]interface{}{
                "url": map[string]interface{}{
                    "type":        "string",
                    "description": "Account URL (e.g., acc://alice.acme/tokens)",
                },
                "include_receipt": map[string]interface{}{
                    "type":        "boolean",
                    "description": "Include Merkle receipt (optional)",
                },
                "prove": map[string]interface{}{
                    "type":        "boolean",
                    "description": "Include cryptographic proof (optional)",
                },
            },
            Required: []string{"url"},
        },
    }, s.handleQueryAccount)

    // Add more tools...

    return nil
}

func (s *Server) registerResources(mcpServer *server.MCPServer) error {
    // Register account resource
    mcpServer.AddResource(mcp.Resource{
        URI:         "accumulate://account/{url}",
        Name:        "Account Information",
        Description: "Read account information by URL",
        MimeType:    "application/json",
    }, s.handleAccountResource)

    // Add more resources...

    return nil
}

func (s *Server) registerPrompts(mcpServer *server.MCPServer) error {
    // Register prompts
    mcpServer.AddPrompt(mcp.Prompt{
        Name:        "inspect_account",
        Description: "Comprehensive account inspection workflow",
        Arguments: []mcp.PromptArgument{
            {
                Name:        "url",
                Description: "Account URL to inspect",
                Required:    true,
            },
        },
    }, s.handleInspectAccountPrompt)

    // Add more prompts...

    return nil
}
```

## Step 5: Implement Tool Handlers

### File: `tools/accumulate-mcp/server/tools.go`

```go
package server

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) handleNodeInfo(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    // Parse parameters
    var params struct {
        PeerID string `json:"peer_id"`
    }

    if err := json.Unmarshal(request.Params.Arguments, &params); err != nil {
        return nil, fmt.Errorf("invalid parameters: %w", err)
    }

    // Call Accumulate API
    resp, err := s.client.NodeInfo(ctx, params.PeerID)
    if err != nil {
        return nil, fmt.Errorf("failed to get node info: %w", err)
    }

    // Convert to JSON
    result, err := json.Marshal(resp)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal response: %w", err)
    }

    return &mcp.CallToolResult{
        Content: []mcp.Content{
            {
                Type: "text",
                Text: string(result),
            },
        },
    }, nil
}

func (s *Server) handleQueryAccount(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    // Parse parameters
    var params struct {
        URL            string `json:"url"`
        IncludeReceipt bool   `json:"include_receipt"`
        Prove          bool   `json:"prove"`
    }

    if err := json.Unmarshal(request.Params.Arguments, &params); err != nil {
        return nil, fmt.Errorf("invalid parameters: %w", err)
    }

    // Validate URL
    if params.URL == "" {
        return nil, fmt.Errorf("url parameter is required")
    }

    // Call Accumulate API
    resp, err := s.client.QueryAccount(ctx, params.URL, &client.QueryOptions{
        IncludeReceipt: params.IncludeReceipt,
        Prove:          params.Prove,
    })
    if err != nil {
        return nil, fmt.Errorf("failed to query account: %w", err)
    }

    // Convert to JSON
    result, err := json.Marshal(resp)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal response: %w", err)
    }

    return &mcp.CallToolResult{
        Content: []mcp.Content{
            {
                Type: "text",
                Text: string(result),
            },
        },
    }, nil
}

// Add more tool handlers...
```

## Step 6: Implement Resource Handlers

### File: `tools/accumulate-mcp/server/resources.go`

```go
package server

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"

    "github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) handleAccountResource(ctx context.Context, request mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
    // Parse URI: accumulate://account/{url}
    uri := request.Params.URI
    prefix := "accumulate://account/"
    if !strings.HasPrefix(uri, prefix) {
        return nil, fmt.Errorf("invalid URI format")
    }

    accountURL := strings.TrimPrefix(uri, prefix)

    // Query account
    resp, err := s.client.QueryAccount(ctx, accountURL, &client.QueryOptions{})
    if err != nil {
        return nil, fmt.Errorf("failed to query account: %w", err)
    }

    // Convert to JSON
    result, err := json.MarshalIndent(resp, "", "  ")
    if err != nil {
        return nil, fmt.Errorf("failed to marshal response: %w", err)
    }

    return &mcp.ReadResourceResult{
        Contents: []mcp.ResourceContents{
            {
                URI:      uri,
                MimeType: "application/json",
                Text:     string(result),
            },
        },
    }, nil
}

// Add more resource handlers...
```

## Step 7: Build and Test

### Build

```bash
cd tools/accumulate-mcp
go build -o accumulate-mcp .
```

### Test with MCP Inspector

```bash
# Install MCP inspector
npm install -g @modelcontextprotocol/inspector

# Run inspector
mcp-inspector ./accumulate-mcp
```

### Test Tool Invocation

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "accumulate_query_account",
    "arguments": {
      "url": "acc://ACME"
    }
  }
}
```

### Test Resource Read

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "resources/read",
  "params": {
    "uri": "accumulate://account/ACME"
  }
}
```

## Step 8: Integration with Claude Desktop

### Configuration File

**Location:** `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS)

```json
{
  "mcpServers": {
    "accumulate": {
      "command": "/path/to/accumulate-mcp",
      "env": {
        "ACCUMULATE_MCP_CONFIG": "/path/to/config.json"
      }
    }
  }
}
```

### Configuration Example

**File:** `config.json`

```json
{
  "network": "TestNet",
  "endpoints": [
    "https://testnet.accumulatenetwork.io/v3"
  ],
  "timeout": "30s",
  "enable_v2_compat": true,
  "enable_transaction_building": true,
  "enable_faucet": true,
  "enable_snapshots": false,
  "enable_events": true,
  "read_only": false,
  "allow_transaction_submit": true,
  "require_confirmation": true,
  "max_query_results": 1000,
  "cache_node_info": "30s",
  "cache_blockchain": "5s"
}
```

## Step 9: Testing Strategy

### Unit Tests

```go
// File: tools/accumulate-mcp/server/tools_test.go

package server

import (
    "context"
    "testing"

    "github.com/stretchr/testify/require"
)

func TestNodeInfo(t *testing.T) {
    // Create test server
    s := &Server{
        config: testConfig(),
        client: mockClient(),
    }

    // Create request
    req := mcp.CallToolRequest{
        Params: mcp.CallToolParams{
            Name: "accumulate_node_info",
            Arguments: json.RawMessage(`{"peer_id": ""}`),
        },
    }

    // Call handler
    result, err := s.handleNodeInfo(context.Background(), req)
    require.NoError(t, err)
    require.NotNil(t, result)

    // Validate response
    require.NotEmpty(t, result.Content)
}
```

### Integration Tests

```go
// File: tools/accumulate-mcp/integration_test.go

package main

import (
    "context"
    "testing"

    "github.com/stretchr/testify/require"
)

func TestQueryAccountIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    // Create client
    client, err := client.New(&client.Config{
        Endpoints: []string{"https://testnet.accumulatenetwork.io/v3"},
        Timeout:   30 * time.Second,
    })
    require.NoError(t, err)

    // Query ACME token issuer
    resp, err := client.QueryAccount(context.Background(), "acc://ACME", &client.QueryOptions{})
    require.NoError(t, err)
    require.NotNil(t, resp)
    require.Equal(t, "acc://ACME", resp.Account.GetUrl())
}
```

### Run Tests

```bash
# Unit tests
go test ./...

# Integration tests
go test ./... -tags=integration

# With coverage
go test ./... -cover
```

## Step 10: Documentation

### User Documentation (README.md)

```markdown
# Accumulate MCP Server

MCP server for the Accumulate blockchain protocol.

## Installation

```bash
go install gitlab.com/AccumulateNetwork/accumulate/tools/accumulate-mcp@latest
```

## Usage

### With Claude Desktop

Add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "accumulate": {
      "command": "accumulate-mcp"
    }
  }
}
```

### Available Tools

- `accumulate_node_info` - Get node information
- `accumulate_query_account` - Query account by URL
- `accumulate_query_transaction` - Query transaction
- ... (28 total tools)

### Examples

Query account balance:
> "What is the balance of acc://alice.acme/tokens?"

Submit transaction:
> "Submit this signed transaction: {...}"
```

## Advanced Topics

### Custom Caching

```go
// Implement custom cache with Redis
type RedisCache struct {
    client *redis.Client
}

func (c *RedisCache) Get(key string) (interface{}, bool) {
    val, err := c.client.Get(context.Background(), key).Result()
    if err != nil {
        return nil, false
    }
    return val, true
}
```

### Rate Limiting

```go
import "golang.org/x/time/rate"

type RateLimitedClient struct {
    client  *client.Client
    limiter *rate.Limiter
}

func (c *RateLimitedClient) Query(ctx context.Context, req interface{}) (interface{}, error) {
    if err := c.limiter.Wait(ctx); err != nil {
        return nil, err
    }
    return c.client.Query(ctx, req)
}
```

### Connection Pooling

```go
type ConnectionPool struct {
    clients []*client.Client
    current int
    mu      sync.Mutex
}

func (p *ConnectionPool) Get() *client.Client {
    p.mu.Lock()
    defer p.mu.Unlock()
    c := p.clients[p.current]
    p.current = (p.current + 1) % len(p.clients)
    return c
}
```

## Deployment

### Docker

```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY . .
RUN go build -o accumulate-mcp ./tools/accumulate-mcp

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/accumulate-mcp /usr/local/bin/
ENTRYPOINT ["accumulate-mcp"]
```

### Systemd Service

```ini
[Unit]
Description=Accumulate MCP Server
After=network.target

[Service]
Type=simple
User=accumulate
Environment="ACCUMULATE_MCP_CONFIG=/etc/accumulate-mcp/config.json"
ExecStart=/usr/local/bin/accumulate-mcp
Restart=always

[Install]
WantedBy=multi-user.target
```

## Security Considerations

1. **Never handle private keys** - Signing must be external
2. **Validate all inputs** - Prevent injection attacks
3. **Rate limit requests** - Prevent DoS
4. **Use HTTPS** - Encrypt API communications
5. **Log all transactions** - Audit trail
6. **Implement timeouts** - Prevent hanging requests

## Troubleshooting

### Connection Issues

```bash
# Test API connectivity
curl https://mainnet.accumulatenetwork.io/v3/node/info

# Check MCP server logs
journalctl -u accumulate-mcp -f
```

### Performance Issues

- Enable caching
- Use connection pooling
- Increase timeout values
- Monitor API rate limits

## Contributing

See CONTRIBUTING.md

## License

MIT License

## References

- [MCP Specification](https://spec.modelcontextprotocol.io/)
- [Accumulate Documentation](https://docs.accumulatenetwork.io/)
- [API Reference](./api-mapping-reference.md)
```

## Version History

- **v1.0** (2025-10-20): Initial implementation guide
  - Complete project structure
  - Step-by-step implementation
  - Testing strategy
  - Deployment instructions
