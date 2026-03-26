# Accumulate MCP Server - Quick Start Guide

## What You'll Build

An MCP server that lets AI assistants (like Claude) interact with the Accumulate blockchain:
- Query accounts and balances
- Check transaction status
- Browse blockchain data
- Build (but not sign) transactions
- Subscribe to real-time events

**Time to complete:** 30-60 minutes

## Prerequisites

```bash
# Check Go version (need 1.21+)
go version

# Clone Accumulate repo
git clone https://gitlab.com/AccumulateNetwork/accumulate.git
cd accumulate

# Install MCP SDK
go get github.com/mark3labs/mcp-go@latest
```

## Step 1: Create Project Structure (2 minutes)

```bash
mkdir -p tools/accumulate-mcp/{server,client,config,handlers}
cd tools/accumulate-mcp
```

## Step 2: Initialize Go Module (1 minute)

```bash
# File: go.mod
cat > go.mod << 'EOF'
module gitlab.com/AccumulateNetwork/accumulate/tools/accumulate-mcp

go 1.21

require (
    github.com/mark3labs/mcp-go v0.1.0
    gitlab.com/AccumulateNetwork/accumulate v1.4.3
)
EOF

go mod tidy
```

## Step 3: Create Main Entry Point (5 minutes)

```bash
# File: main.go
cat > main.go << 'EOF'
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
    // Load config
    cfg := config.DefaultConfig()

    // Create MCP server
    s := server.NewMCPServer(
        "accumulate-mcp",
        "1.0.0",
        server.WithLogging(),
    )

    // Create handler
    handler := mcpserver.New(cfg)

    // Register tools
    handler.Register(s)

    // Start server
    if err := s.Serve(context.Background(), os.Stdin, os.Stdout); err != nil {
        log.Fatal(err)
    }
}
EOF
```

## Step 4: Configuration (5 minutes)

```bash
# File: config/config.go
cat > config/config.go << 'EOF'
package config

import "time"

type Config struct {
    Network   string
    Endpoint  string
    Timeout   time.Duration
    ReadOnly  bool
}

func DefaultConfig() *Config {
    return &Config{
        Network:  "TestNet",
        Endpoint: "https://testnet.accumulatenetwork.io/v3",
        Timeout:  30 * time.Second,
        ReadOnly: false,
    }
}
EOF
```

## Step 5: Accumulate Client Wrapper (10 minutes)

```bash
# File: client/client.go
cat > client/client.go << 'EOF'
package client

import (
    "context"
    "fmt"

    "gitlab.com/AccumulateNetwork/accumulate/pkg/api/v3"
)

type Client struct {
    endpoint string
}

func New(endpoint string) *Client {
    return &Client{endpoint: endpoint}
}

func (c *Client) QueryAccount(ctx context.Context, url string) (map[string]interface{}, error) {
    // TODO: Implement using V3 API client
    // For now, return mock data
    return map[string]interface{}{
        "url":      url,
        "type":     "tokenAccount",
        "balance":  "1000000000",
        "tokenUrl": "acc://ACME",
    }, nil
}
EOF
```

## Step 6: MCP Server Handler (15 minutes)

```bash
# File: server/server.go
cat > server/server.go << 'EOF'
package server

import (
    "context"
    "encoding/json"

    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"
    "gitlab.com/AccumulateNetwork/accumulate/tools/accumulate-mcp/client"
    "gitlab.com/AccumulateNetwork/accumulate/tools/accumulate-mcp/config"
)

type Server struct {
    config *config.Config
    client *client.Client
}

func New(cfg *config.Config) *Server {
    return &Server{
        config: cfg,
        client: client.New(cfg.Endpoint),
    }
}

func (s *Server) Register(mcpServer *server.MCPServer) error {
    // Register query_account tool
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
            },
            Required: []string{"url"},
        },
    }, s.handleQueryAccount)

    return nil
}

func (s *Server) handleQueryAccount(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    // Parse parameters
    var params struct {
        URL string `json:"url"`
    }
    if err := json.Unmarshal(request.Params.Arguments, &params); err != nil {
        return nil, err
    }

    // Query account
    result, err := s.client.QueryAccount(ctx, params.URL)
    if err != nil {
        return nil, err
    }

    // Format response
    resultJSON, _ := json.MarshalIndent(result, "", "  ")
    return &mcp.CallToolResult{
        Content: []mcp.Content{
            {
                Type: "text",
                Text: string(resultJSON),
            },
        },
    }, nil
}
EOF
```

## Step 7: Build and Test (5 minutes)

```bash
# Build
go build -o accumulate-mcp .

# Test with echo
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | ./accumulate-mcp
```

## Step 8: Test with MCP Inspector (10 minutes)

```bash
# Install MCP inspector
npm install -g @modelcontextprotocol/inspector

# Run inspector
mcp-inspector ./accumulate-mcp
```

Then in the inspector UI:
1. Click "Tools"
2. Click "accumulate_query_account"
3. Enter `{"url": "acc://ACME"}`
4. Click "Call Tool"
5. See the response!

## Step 9: Integrate with Claude Desktop (5 minutes)

### macOS

```bash
# Edit Claude config
code ~/Library/Application\ Support/Claude/claude_desktop_config.json
```

Add:
```json
{
  "mcpServers": {
    "accumulate": {
      "command": "/path/to/accumulate/tools/accumulate-mcp/accumulate-mcp"
    }
  }
}
```

### Restart Claude Desktop

Now you can ask Claude:
> "What is the balance of acc://ACME?"

Claude will use your MCP server to answer!

## Next Steps

### Add More Tools

```go
// In server.go Register() function, add:

mcpServer.AddTool(mcp.Tool{
    Name:        "accumulate_query_transaction",
    Description: "Query transaction by ID",
    InputSchema: mcp.ToolInputSchema{
        Type: "object",
        Properties: map[string]interface{}{
            "txid": map[string]interface{}{
                "type":        "string",
                "description": "Transaction ID",
            },
        },
        Required: []string{"txid"},
    },
}, s.handleQueryTransaction)
```

### Implement Real API Calls

Replace the mock client with real V3 API calls:

```go
import (
    "gitlab.com/AccumulateNetwork/accumulate/pkg/api/v3"
    "gitlab.com/AccumulateNetwork/accumulate/pkg/client"
)

func (c *Client) QueryAccount(ctx context.Context, url string) (map[string]interface{}, error) {
    // Create V3 client
    v3Client := client.NewV3Client(c.endpoint)

    // Make query
    req := &api.DefaultQuery{
        Url: url,
    }

    resp, err := v3Client.Query(ctx, req)
    if err != nil {
        return nil, err
    }

    // Convert to map
    data, _ := json.Marshal(resp)
    var result map[string]interface{}
    json.Unmarshal(data, &result)

    return result, nil
}
```

### Add Resources

```go
// Add to Register() function:

mcpServer.AddResource(mcp.Resource{
    URI:         "accumulate://account/{url}",
    Name:        "Account Information",
    Description: "Read account data",
    MimeType:    "application/json",
}, s.handleAccountResource)
```

### Add Caching

```go
import (
    "github.com/patrickmn/go-cache"
    "time"
)

type Client struct {
    endpoint string
    cache    *cache.Cache
}

func New(endpoint string) *Client {
    return &Client{
        endpoint: endpoint,
        cache:    cache.New(5*time.Second, 10*time.Second),
    }
}

func (c *Client) QueryAccount(ctx context.Context, url string) (map[string]interface{}, error) {
    // Check cache
    if cached, found := c.cache.Get(url); found {
        return cached.(map[string]interface{}), nil
    }

    // Fetch from API
    result, err := c.fetchAccount(ctx, url)
    if err != nil {
        return nil, err
    }

    // Cache result
    c.cache.Set(url, result, cache.DefaultExpiration)

    return result, nil
}
```

## Complete Tool List to Implement

Once you have the basics working, implement these tools:

### Network Tools (Priority: High)
- [ ] `accumulate_node_info`
- [ ] `accumulate_network_status`
- [ ] `accumulate_consensus_status`

### Query Tools (Priority: High)
- [x] `accumulate_query_account` ✅ (you just built this!)
- [ ] `accumulate_query_transaction`
- [ ] `accumulate_query_chain`
- [ ] `accumulate_query_directory`

### Transaction Tools (Priority: Medium)
- [ ] `accumulate_submit_transaction`
- [ ] `accumulate_validate_transaction`
- [ ] `accumulate_build_send_tokens`

### Advanced (Priority: Low)
- [ ] `accumulate_subscribe_events`
- [ ] `accumulate_query_anchors`
- [ ] `accumulate_faucet` (testnet only)

## Common Issues & Solutions

### "Package not found"
```bash
go mod tidy
go mod download
```

### "MCP server not responding"
Check stdio isn't being used by logs:
```go
// Use stderr for logs
log.SetOutput(os.Stderr)
```

### "Claude can't connect"
Verify the path in `claude_desktop_config.json`:
```bash
# Test the command directly
/path/to/accumulate-mcp
# Should wait for JSON-RPC input
```

### "API calls failing"
Test the endpoint:
```bash
curl https://testnet.accumulatenetwork.io/v3/node/info
```

## Resources

### Full Documentation
- [Complete Design Spec](./mcp-server-design.md) - Architecture & all 28 tools
- [API Mapping Reference](./api-mapping-reference.md) - Detailed endpoint docs
- [Implementation Guide](./implementation-guide.md) - Advanced implementation

### External Resources
- [MCP Specification](https://spec.modelcontextprotocol.io/)
- [MCP Go SDK](https://github.com/mark3labs/mcp-go)
- [Accumulate API Docs](https://docs.accumulatenetwork.io/)

### Getting Help
- GitLab Issues: https://gitlab.com/AccumulateNetwork/accumulate/-/issues
- Discord: https://discord.gg/accumulate

## Success Checklist

- [x] Go 1.21+ installed
- [x] Project structure created
- [x] Main entry point working
- [x] Config loaded
- [x] First tool implemented (`accumulate_query_account`)
- [x] MCP inspector test passing
- [ ] Claude Desktop integration working
- [ ] Real API calls implemented
- [ ] Multiple tools working
- [ ] Resources implemented
- [ ] Caching added
- [ ] Error handling robust
- [ ] Tests written

## What's Next?

1. **Add More Tools** - Implement the 28 tools from the design spec
2. **Real API Integration** - Replace mocks with actual V3 API calls
3. **Add Tests** - Write unit and integration tests
4. **Deploy** - Create Docker image and deployment configs
5. **Contribute** - Submit MR to include in main Accumulate repo

Congratulations! You've built a working MCP server for Accumulate! 🎉
