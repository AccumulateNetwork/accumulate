# Accumulate MCP - Integration Examples

This directory contains working examples showing how to integrate with Accumulate MCP tools from various programming environments.

---

## Available Examples

### 1. **invoke-mcp-tool.sh** - Basic Tool Invocation

Simple bash helper for calling any MCP tool.

**Usage:**
```bash
./invoke-mcp-tool.sh <tool-name> <json-params>
```

**Examples:**
```bash
# Get genesis file locations
./invoke-mcp-tool.sh accumulate_get_genesis_files '{"network":"mainnet"}'

# Check follower status
./invoke-mcp-tool.sh accumulate_follower_status '{"container_name":"accumulate-follower"}'

# Get bootstrap peers
./invoke-mcp-tool.sh accumulate_get_bootstrap_peers '{"network":"mainnet"}'
```

---

### 2. **deploy-follower-complete.sh** - Full Deployment Example

Complete end-to-end follower deployment script using MCP tools.

**What it does:**
1. Verifies database snapshots exist
2. Checks for genesis files
3. Initializes follower (copies databases, creates config)
4. Starts follower in Docker
5. Verifies deployment succeeded

**Usage:**
```bash
# Using environment variables
export DN_DATABASE=/media/paul/Expansion/databases/2025-10-13-dn
export BVN_DATABASE=/media/paul/Expansion/databases/2025-10-13-bvn
export WORK_DIR=/var/lib/accumulate-follower
./deploy-follower-complete.sh

# Or inline
DN_DATABASE=/path/to/dn BVN_DATABASE=/path/to/bvn ./deploy-follower-complete.sh
```

**Prerequisites:**
- Database snapshots with complete structure (config/, data/, data/accumulate.db/)
- Docker installed and running
- MCP server binary in current directory or MCP_SERVER env var set

---

### 3. **python-mcp-client.py** - Python Integration

Python class for calling MCP tools from Python scripts.

**Usage:**
```python
from python_mcp_client import AccumulateMCPClient

client = AccumulateMCPClient()

# Get genesis files
genesis = client.call_tool("accumulate_get_genesis_files", {
    "network": "mainnet",
    "bvn": "1"
})

# Initialize follower
result = client.call_tool("accumulate_init_follower", {
    "dn_database": "/path/to/dn",
    "bvn_database": "/path/to/bvn",
    "work_dir": "/tmp/follower"
})
```

**Running the examples:**
```bash
# Check genesis files
./python-mcp-client.py

# Full deployment
./python-mcp-client.py /path/to/dn /path/to/bvn
```

---

## Integration Patterns

### Stdio Mode (Default)

MCP server reads JSON-RPC from stdin and writes responses to stdout:

```bash
echo '{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "accumulate_get_genesis_files",
    "arguments": {"network": "mainnet"}
  }
}' | ./mcp-server
```

### HTTP Mode (Optional)

If MCP server is running in HTTP mode:

```bash
curl -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "accumulate_follower_status",
      "arguments": {"container_name": "accumulate-follower"}
    }
  }'
```

---

## JSON-RPC Format

### Request Format

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "<tool-name>",
    "arguments": {
      // Tool-specific parameters
    }
  }
}
```

### Success Response

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    // Tool-specific result
  }
}
```

### Error Response

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32600,
    "message": "Invalid Request: missing method"
  }
}
```

---

## Available Tools

### Follower Management
- `accumulate_init_follower` - Initialize follower from database snapshots
- `accumulate_run_follower` - Start follower in Docker
- `accumulate_follower_status` - Get container status
- `accumulate_stop_follower` - Stop container
- `accumulate_remove_follower` - Remove container

### Configuration & Discovery
- `accumulate_get_genesis_files` - Locate genesis snapshot files
- `accumulate_get_bootstrap_peers` - Get network bootstrap peers

### Accman Artifacts
- `accumulate_prepare_accman_artifacts` - Prepare deployment artifacts
- `accumulate_create_node_archive` - Archive node directory

See tool documentation for parameter details:
- `../FOLLOWER_DOCKER_GUIDE.md`
- `../GENESIS_FILES_GUIDE.md`
- `../ACCMAN_INTEGRATION_GUIDE.md`

---

## Troubleshooting

### MCP Server Not Found

```bash
# Set MCP_SERVER environment variable
export MCP_SERVER=/path/to/mcp-server

# Or specify full path
./invoke-mcp-tool.sh accumulate_get_genesis_files '{}'
```

### Invalid JSON Error

Make sure JSON is properly quoted:
```bash
# GOOD
./invoke-mcp-tool.sh tool_name '{"key":"value"}'

# BAD (shell will interpret quotes wrong)
./invoke-mcp-tool.sh tool_name {"key":"value"}
```

### Tool Call Fails

Check the error message in the response:
```bash
./invoke-mcp-tool.sh accumulate_init_follower '{}' | jq '.error'
```

Common errors:
- Missing required parameters
- Invalid paths
- Docker not running
- Insufficient permissions

---

## Creating Your Own Integration

### Bash Template

```bash
#!/bin/bash
call_mcp() {
    local tool="$1"
    local params="$2"

    echo "{
      \"jsonrpc\": \"2.0\",
      \"id\": 1,
      \"method\": \"tools/call\",
      \"params\": {
        \"name\": \"$tool\",
        \"arguments\": $params
      }
    }" | ./mcp-server
}

# Use it
result=$(call_mcp "accumulate_get_genesis_files" '{"network":"mainnet"}')
```

### Python Template

```python
import json, subprocess

def call_mcp_tool(tool_name, arguments):
    request = {
        "jsonrpc": "2.0",
        "id": 1,
        "method": "tools/call",
        "params": {"name": tool_name, "arguments": arguments}
    }

    result = subprocess.run(
        ["./mcp-server"],
        input=json.dumps(request),
        capture_output=True,
        text=True
    )

    return json.loads(result.stdout)
```

### Go Template

```go
package main

import (
    "encoding/json"
    "os/exec"
)

type MCPRequest struct {
    JSONRPC string `json:"jsonrpc"`
    ID      int    `json:"id"`
    Method  string `json:"method"`
    Params  struct {
        Name      string                 `json:"name"`
        Arguments map[string]interface{} `json:"arguments"`
    } `json:"params"`
}

func callMCPTool(toolName string, args map[string]interface{}) (map[string]interface{}, error) {
    req := MCPRequest{
        JSONRPC: "2.0",
        ID:      1,
        Method:  "tools/call",
    }
    req.Params.Name = toolName
    req.Params.Arguments = args

    input, _ := json.Marshal(req)
    cmd := exec.Command("./mcp-server")
    cmd.Stdin = strings.NewReader(string(input))
    output, err := cmd.Output()

    var result map[string]interface{}
    json.Unmarshal(output, &result)
    return result, err
}
```

---

## Next Steps

1. Try the examples to understand MCP integration
2. Adapt the templates for your use case
3. See the main documentation for tool parameter details
4. Check `../TROUBLESHOOTING.md` for common issues

---

## Contributing

If you create useful integration examples for other languages (JavaScript, Rust, etc.), please contribute them back to this directory!
