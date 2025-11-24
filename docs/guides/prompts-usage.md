# MCP Prompts Usage Guide

## Overview

The Accumulate MCP server now supports **prompts** - pre-built workflow templates that combine multiple tools into guided, step-by-step instructions for common tasks.

## Available Prompts

### 1. deploy-follower-node ⭐⭐⭐ (CRITICAL)
Deploy a complete Accumulate follower node from database snapshots.

**Required Arguments:**
- `dn_database` - Path to Directory Network database snapshot
- `bvn_database` - Path to Block Validation Network database snapshot
- `work_dir` - Directory for follower configuration and data

**Optional Arguments:**
- `peer_url` - Peer BVN URL (e.g., tcp://peer.example.com:16691)
- `seed_proxy` - Seed proxy URL for network configuration
- `public_ip` - Follower's public IP address

**Example:**
```json
{
  "method": "prompts/get",
  "params": {
    "name": "deploy-follower-node",
    "arguments": {
      "dn_database": "/snapshots/dn",
      "bvn_database": "/snapshots/bvn",
      "work_dir": "/accumulate/follower",
      "peer_url": "tcp://mainnet.accumulate.defidevs.io:16691"
    }
  }
}
```

### 2. monitor-follower-health ⭐⭐⭐ (HIGH)
Quick health check and monitoring for a running follower node.

**Optional Arguments:**
- `work_dir` - Follower working directory (default: ~/.accumulate/follower)
- `endpoint` - Follower API endpoint (default: mainnet)

**Example:**
```json
{
  "method": "prompts/get",
  "params": {
    "name": "monitor-follower-health",
    "arguments": {
      "work_dir": "/accumulate/follower"
    }
  }
}
```

### 3. troubleshoot-follower-sync ⭐⭐⭐ (HIGH)
Diagnose and resolve follower node synchronization issues.

**Optional Arguments:**
- `work_dir` - Follower working directory
- `symptom` - Observed issue: `no_peers`, `not_syncing`, `slow_sync`, or `crashed`

**Example:**
```json
{
  "method": "prompts/get",
  "params": {
    "name": "troubleshoot-follower-sync",
    "arguments": {
      "symptom": "no_peers"
    }
  }
}
```

### 4. setup-dev-wallet ⭐⭐ (MEDIUM)
Set up an Accumulate wallet for development with testnet tokens.

**Optional Arguments:**
- `network` - Network to use: testnet or devnet (default: testnet)
- `wallet_dir` - Wallet directory path
- `no_password` - Initialize without password for development

**Example:**
```json
{
  "method": "prompts/get",
  "params": {
    "name": "setup-dev-wallet",
    "arguments": {
      "network": "testnet",
      "no_password": "true"
    }
  }
}
```

### 5. quick-node-status ⭐ (MEDIUM)
Fast status check for follower node (concise output).

**Optional Arguments:**
- `work_dir` - Follower working directory

**Example:**
```json
{
  "method": "prompts/get",
  "params": {
    "name": "quick-node-status",
    "arguments": {
      "work_dir": "/accumulate/follower"
    }
  }
}
```

## Using Prompts

### 1. List Available Prompts

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "prompts/list"
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "prompts": [
      {
        "name": "deploy-follower-node",
        "description": "Complete workflow for deploying an Accumulate follower node from database snapshots",
        "arguments": [...]
      },
      ...
    ]
  }
}
```

### 2. Get a Specific Prompt

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "prompts/get",
  "params": {
    "name": "deploy-follower-node",
    "arguments": {
      "dn_database": "/snapshots/dn",
      "bvn_database": "/snapshots/bvn",
      "work_dir": "/accumulate/follower"
    }
  }
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "description": "Generated prompt: deploy-follower-node",
    "messages": [
      {
        "role": "user",
        "content": {
          "type": "text",
          "text": "Deploy Accumulate follower node to: /accumulate/follower\n\n**Database Snapshots:**\n..."
        }
      }
    ]
  }
}
```

## Prompt Workflow

Prompts are designed to be used with AI assistants like Claude. The typical workflow is:

1. **User requests a task** (e.g., "Deploy a follower node")
2. **AI lists available prompts** using `prompts/list`
3. **AI selects appropriate prompt** based on user's request
4. **AI gets prompt with arguments** using `prompts/get`
5. **AI follows the prompt instructions** step-by-step, calling appropriate tools
6. **AI reports progress** back to the user

## Integration with Claude

When using Claude Code or other MCP-compatible AI assistants:

1. The assistant can discover prompts via `prompts/list`
2. The assistant can retrieve workflow templates via `prompts/get`
3. The template provides:
   - Prerequisites to check
   - Step-by-step instructions
   - Expected outputs
   - Validation checklists
   - Troubleshooting guidance
   - Related prompts for next steps

## Example: Deploy Follower Node

```bash
# User: "Deploy a follower node using snapshots in /snapshots"

# AI uses prompts/get to retrieve the deploy-follower-node template
# Then follows the instructions:

# Step 1: Check prerequisites
- Verify /snapshots/dn exists
- Verify /snapshots/bvn exists
- Check disk space

# Step 2: Initialize follower
accumulate_init_follower {
  "dn_database": "/snapshots/dn",
  "bvn_database": "/snapshots/bvn",
  "work_dir": "/accumulate/follower"
}

# Step 3: Start follower
accumulate_run_follower {
  "work_dir": "/accumulate/follower",
  "background": true
}

# Step 4: Verify startup
accumulate_follower_status {
  "work_dir": "/accumulate/follower"
}

# Step 5: Monitor sync
accumulate_node_info {...}
accumulate_network_status {...}
```

## Benefits of Prompts

1. **Consistency** - Standardized workflows ensure best practices
2. **Completeness** - No steps forgotten or skipped
3. **Troubleshooting** - Built-in guidance for common issues
4. **Validation** - Checklists ensure each step succeeded
5. **Learning** - Clear explanations help users understand the process

## Development

### Adding New Prompts

1. Define prompt in `server/prompts.go`:
```go
{
    Name:        "my-new-prompt",
    Description: "Description of what it does",
    Arguments: []PromptArgument{
        {
            Name:        "required_arg",
            Description: "What this argument is for",
            Required:    true,
        },
    },
}
```

2. Implement template generator:
```go
func generateMyNewPromptTemplate(args map[string]string, getArg func(string, string) string) string {
    var b strings.Builder
    // Build template content
    return b.String()
}
```

3. Add case to `GetPromptTemplate`:
```go
case "my-new-prompt":
    return generateMyNewPromptTemplate(args, getArg), nil
```

4. Write tests in `prompts_test.go`

## Testing

Run prompt tests:
```bash
go test -v -run TestPrompt
```

Test specific prompt:
```bash
go test -v -run TestGetPromptTemplate/deploy-follower-node
```

## Related Documentation

- [PROMPTS_DESIGN.md](PROMPTS_DESIGN.md) - Design specifications for all prompts
- [PROMPT_ANALYSIS.md](PROMPT_ANALYSIS.md) - Analysis of workflows and use cases
- [prompt-analysis-process.md](prompt-analysis-process.md) - How prompts were developed
- [FOLLOWER_SETUP_GUIDE.md](FOLLOWER_SETUP_GUIDE.md) - Manual follower setup guide
- [MCP_ARCHITECTURE.md](MCP_ARCHITECTURE.md) - MCP server architecture
