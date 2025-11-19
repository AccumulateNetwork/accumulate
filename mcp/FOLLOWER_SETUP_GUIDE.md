# Accumulate Follower Setup Guide - MCP Tools

This guide explains how to use the new MCP tools to create and launch an Accumulate follower node using database snapshots.

## Overview

The Accumulate MCP now includes three new tools for follower node management:

1. **`accumulate_init_follower`** - Initialize a follower from database snapshots
2. **`accumulate_run_follower`** - Start the follower node
3. **`accumulate_follower_status`** - Check follower status

## Prerequisites

- Database snapshots from backupdbs repository:
  - DN database: `/media/paul/Expansion/databases/2025-10-13-dn/`
  - BVN database: `/media/paul/Expansion/databases/2025-10-13-bvn/`
- Peer URL or seed proxy for network connection
- Working directory for follower data

## Step 1: Initialize the Follower

Use the `accumulate_init_follower` tool to set up the follower configuration:

```json
{
  "tool": "accumulate_init_follower",
  "arguments": {
    "dn_database": "/media/paul/Expansion/databases/2025-10-13-dn",
    "bvn_database": "/media/paul/Expansion/databases/2025-10-13-bvn",
    "work_dir": "/home/paul/accumulate-follower",
    "peer_url": "tcp://mainnet-peer.example.com:16691",
    "public_ip": "YOUR_PUBLIC_IP",
    "listen_ip": "0.0.0.0"
  }
}
```

### Parameters

**Required:**
- `dn_database` - Path to Directory Network database snapshot
- `bvn_database` - Path to Block Validation Network database snapshot
- `work_dir` - Directory where follower configuration will be created

**Optional (at least one required):**
- `peer_url` - Peer BVN URL to connect to (e.g., `tcp://peer.example.com:16691`)
- `seed_proxy` - Seed proxy URL to fetch network configuration

**Optional:**
- `public_ip` - Your follower's public IP address
- `listen_ip` - IP address to listen on (default: `0.0.0.0`)

### What This Does

1. Creates the work directory
2. Copies the DN database to `work_dir/dnn`
3. Copies the BVN database to `work_dir/bvnn`
4. Runs `accumulated init dual --follow` to create follower configuration
5. Returns initialization status

### Example Response

```json
{
  "status": "initialized",
  "work_dir": "/home/paul/accumulate-follower",
  "dn_path": "/home/paul/accumulate-follower/dnn",
  "bvn_path": "/home/paul/accumulate-follower/bvnn",
  "output": "...",
  "next_steps": "Use accumulate_run_follower to start the follower node"
}
```

## Step 2: Start the Follower

After initialization, use `accumulate_run_follower` to start the node:

```json
{
  "tool": "accumulate_run_follower",
  "arguments": {
    "work_dir": "/home/paul/accumulate-follower",
    "background": true
  }
}
```

### Parameters

- `work_dir` - Same directory used in initialization
- `background` - Run in background (default: `true`)

### What This Does

1. Validates that the work directory is properly initialized
2. Starts `accumulated run dual` with the DN and BVN directories
3. Returns process information if running in background

### Example Response

```json
{
  "status": "started",
  "pid": 12345,
  "work_dir": "/home/paul/accumulate-follower",
  "message": "Follower started in background"
}
```

## Step 3: Check Status

Monitor the follower status:

```json
{
  "tool": "accumulate_follower_status",
  "arguments": {
    "work_dir": "/home/paul/accumulate-follower"
  }
}
```

## Complete Workflow Example

### Using Mainnet Peer

```json
// 1. Initialize follower
{
  "tool": "accumulate_init_follower",
  "arguments": {
    "dn_database": "/media/paul/Expansion/databases/2025-10-13-dn",
    "bvn_database": "/media/paul/Expansion/databases/2025-10-13-bvn",
    "work_dir": "/home/paul/followers/mainnet-follower",
    "peer_url": "tcp://mainnet.accumulate.defidevs.io:16691"
  }
}

// 2. Start the follower
{
  "tool": "accumulate_run_follower",
  "arguments": {
    "work_dir": "/home/paul/followers/mainnet-follower",
    "background": true
  }
}
```

## Database Snapshot Information

**Current Snapshots (2025-10-13):**

- **DN (Directory Network):**
  - Path: `/media/paul/Expansion/databases/2025-10-13-dn/`
  - Contains: Directory partition data
  - Size: ~1GB

- **BVN (Block Validation Network - Cyclops):**
  - Path: `/media/paul/Expansion/databases/2025-10-13-bvn/`
  - Contains: Cyclops partition data
  - Size: ~1GB

## Notes

### Database Copying

The `accumulate_init_follower` tool **copies** the database directories to the work directory. This means:

- ✅ Original snapshots remain untouched
- ✅ Multiple followers can be created from the same snapshots
- ⚠️ Requires sufficient disk space (2x database size)

Alternative: The implementation includes a `linkDatabase` helper function for creating symlinks instead of copying, but this is not currently used.

### Network Configuration

You need either:
1. **Peer URL** - Direct connection to a known peer
   - Faster setup
   - Requires knowing a healthy peer address

2. **Seed Proxy** - Fetch network configuration dynamically
   - More flexible
   - Requires additional implementation (currently limited)

### Follower vs Validator

The `--follow` flag creates a **follower node**, which:
- ✅ Syncs with the network
- ✅ Can serve queries
- ✅ Has full state access
- ❌ Does **not** participate in consensus
- ❌ Does **not** create blocks

## Troubleshooting

### Initialization Fails

**Error:** "source database not found"
- Check that database paths exist
- Verify paths are absolute (not relative)

**Error:** "must provide either peer_url or seed_proxy"
- Provide at least one connection method

### Follower Won't Start

**Error:** "DN directory not found"
- Run `accumulate_init_follower` first
- Check that initialization completed successfully

### Connection Issues

- Verify peer URL is reachable
- Check firewall settings
- Ensure public IP is correct (if specified)

## Advanced Usage

### Using Multiple Followers

You can create multiple followers from the same snapshots:

```json
// Follower 1 - Mainnet
{
  "tool": "accumulate_init_follower",
  "arguments": {
    "dn_database": "/media/paul/Expansion/databases/2025-10-13-dn",
    "bvn_database": "/media/paul/Expansion/databases/2025-10-13-bvn",
    "work_dir": "/home/paul/followers/follower-1",
    "peer_url": "tcp://peer1.example.com:16691"
  }
}

// Follower 2 - Different peer
{
  "tool": "accumulate_init_follower",
  "arguments": {
    "dn_database": "/media/paul/Expansion/databases/2025-10-13-dn",
    "bvn_database": "/media/paul/Expansion/databases/2025-10-13-bvn",
    "work_dir": "/home/paul/followers/follower-2",
    "peer_url": "tcp://peer2.example.com:16691"
  }
}
```

## Integration with backupdbs MCP

The backupdbs MCP can be used to:
1. Browse available database snapshots
2. Query snapshot information
3. Verify database integrity before using

Example workflow:
```json
// 1. List databases (backupdbs MCP)
{"tool": "db_list"}

// 2. Get info about a specific database (backupdbs MCP)
{
  "tool": "db_open",
  "arguments": {
    "path": "/media/paul/Expansion/databases/2025-10-13-dn"
  }
}

// 3. Use in follower (accumulate MCP)
{
  "tool": "accumulate_init_follower",
  "arguments": {
    "dn_database": "/media/paul/Expansion/databases/2025-10-13-dn",
    "bvn_database": "/media/paul/Expansion/databases/2025-10-13-bvn",
    "work_dir": "/home/paul/follower"
  }
}
```

## Next Steps

After your follower is running:

1. Query the follower status using the accumulate MCP query tools
2. Verify sync status by comparing block heights
3. Use the follower for local development and testing

## References

- [Accumulate Documentation](https://docs.accumulatenetwork.io)
- [MCP Database Access](../mcp/MCP_DATABASE_ACCESS_INVESTIGATION.md)
- [Follower Docker Deployment](../../tools/torrent-deploy/docker-compose.follower.yml)
