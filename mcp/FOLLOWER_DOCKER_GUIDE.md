# Accumulate Follower Setup Guide - Docker Deployment

Complete guide for setting up an Accumulate follower node using MCP tools with Docker containers.

## Overview

The Accumulate MCP provides **Docker-based** follower node management tools:

1. **`accumulate_init_follower`** - Prepare databases and configuration
2. **`accumulate_run_follower`** - Launch follower in Docker container
3. **`accumulate_follower_status`** - Monitor follower status
4. **`accumulate_stop_follower`** - Stop the follower container
5. **`accumulate_remove_follower`** - Remove the follower container

## Prerequisites

### Required
- Docker installed and running
- Database snapshots:
  - DN: `/media/paul/Expansion/databases/2025-10-13-dn/`
  - BVN: `/media/paul/Expansion/databases/2025-10-13-bvn/`
- Sufficient disk space (~2GB for database copies)

### Optional
- Custom bootstrap peers (defaults to MainNet)
- Custom container name
- Custom Docker image

## Quick Start

### Step 1: Initialize Follower

```json
{
  "tool": "accumulate_init_follower",
  "arguments": {
    "dn_database": "/media/paul/Expansion/databases/2025-10-13-dn",
    "bvn_database": "/media/paul/Expansion/databases/2025-10-13-bvn",
    "work_dir": "/home/paul/accumulate-follower"
  }
}
```

**This will:**
1. ✅ Copy DN database to `work_dir/dnn/`
2. ✅ Copy BVN database to `work_dir/bvnn/`
3. ✅ Create `work_dir/accumulate.toml` configuration
4. ✅ Use default MainNet Cyclops BVN settings

**Response:**
```json
{
  "status": "initialized",
  "work_dir": "/home/paul/accumulate-follower",
  "dn_path": "/home/paul/accumulate-follower/dnn",
  "bvn_path": "/home/paul/accumulate-follower/bvnn",
  "config_path": "/home/paul/accumulate-follower/accumulate.toml",
  "container_name": "accumulate-follower",
  "next_steps": "Use accumulate_run_follower to start the follower node in Docker"
}
```

### Step 2: Start Follower in Docker

```json
{
  "tool": "accumulate_run_follower",
  "arguments": {
    "work_dir": "/home/paul/accumulate-follower"
  }
}
```

**This will:**
1. ✅ Validate initialization completed
2. ✅ Pull Docker image (if needed): `registry.gitlab.com/accumulatenetwork/accumulate:latest`
3. ✅ Mount databases as Docker volumes
4. ✅ Expose ports 16591-16593 (DN) and 16691-16693 (BVN)
5. ✅ Start container with `run-dual` command
6. ✅ Verify container is running

**Response:**
```json
{
  "status": "started",
  "container_id": "a1b2c3d4e5f6...",
  "container_name": "accumulate-follower",
  "work_dir": "/home/paul/accumulate-follower",
  "ports": {
    "dn_api": "16591-16593",
    "bvn_api": "16691-16693"
  },
  "message": "Follower started in Docker container",
  "check_status": "docker logs -f accumulate-follower"
}
```

### Step 3: Monitor Status

```json
{
  "tool": "accumulate_follower_status",
  "arguments": {
    "container_name": "accumulate-follower"
  }
}
```

**Response:**
```json
{
  "container_name": "accumulate-follower",
  "running": true,
  "status": "running",
  "message": "Follower is running",
  "stats": "CPU: 5.23%, Memory: 2.1GB / 16GB"
}
```

## Advanced Configuration

### Custom Bootstrap Peers

```json
{
  "tool": "accumulate_init_follower",
  "arguments": {
    "dn_database": "/media/paul/Expansion/databases/2025-10-13-dn",
    "bvn_database": "/media/paul/Expansion/databases/2025-10-13-bvn",
    "work_dir": "/home/paul/accumulate-follower",
    "network": "MainNet",
    "bvn_name": "Cyclops",
    "dn_bootstrap_peers": [
      "/ip4/23.22.212.106/tcp/16591/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD",
      "/ip4/OTHER_PEER_IP/tcp/16591/p2p/PEER_ID"
    ],
    "bvn_bootstrap_peers": [
      "/ip4/23.22.212.106/tcp/16691/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD",
      "/ip4/OTHER_PEER_IP/tcp/16691/p2p/PEER_ID"
    ]
  }
}
```

### Custom Container Name

```json
{
  "tool": "accumulate_init_follower",
  "arguments": {
    "dn_database": "/media/paul/Expansion/databases/2025-10-13-dn",
    "bvn_database": "/media/paul/Expansion/databases/2025-10-13-bvn",
    "work_dir": "/home/paul/follower-1",
    "container_name": "accumulate-follower-1"
  }
}

{
  "tool": "accumulate_run_follower",
  "arguments": {
    "work_dir": "/home/paul/follower-1",
    "container_name": "accumulate-follower-1"
  }
}
```

### Custom Docker Image

```json
{
  "tool": "accumulate_run_follower",
  "arguments": {
    "work_dir": "/home/paul/accumulate-follower",
    "docker_image": "registry.gitlab.com/accumulatenetwork/accumulate:v1.2.3"
  }
}
```

## Container Management

### Stop Follower

```json
{
  "tool": "accumulate_stop_follower",
  "arguments": {
    "container_name": "accumulate-follower"
  }
}
```

**Response:**
```json
{
  "status": "stopped",
  "container_name": "accumulate-follower",
  "message": "Follower container stopped"
}
```

### Remove Follower

Stops and removes the container (data in work_dir is preserved):

```json
{
  "tool": "accumulate_remove_follower",
  "arguments": {
    "container_name": "accumulate-follower"
  }
}
```

**Response:**
```json
{
  "status": "removed",
  "container_name": "accumulate-follower",
  "message": "Follower container removed"
}
```

## Docker Architecture

### Container Configuration

**Image:** `registry.gitlab.com/accumulatenetwork/accumulate:latest`

**Volumes:**
- `{work_dir}/dnn` → `/node/dnn` (DN database)
- `{work_dir}/bvnn` → `/node/bvnn` (BVN database)
- `{work_dir}/accumulate.toml` → `/node/accumulate.toml` (config)

**Ports:**
- `16591-16593` → DN API endpoints
- `16691-16693` → BVN API endpoints

**Command:** `run-dual /node/dnn /node/bvnn`

**Restart Policy:** `unless-stopped`

### Generated Configuration

The `accumulate.toml` file created by init:

```toml
network = "MainNet"

[[configurations]]
  type = "follower"
  mode = "dual"
  bvn = "Cyclops"
  listen = "/ip4/0.0.0.0/tcp/16591"
  storage-type = "badger"
  enable-healing = false
  enable-snapshots = false

  dn-bootstrap-peers = [
    "/ip4/23.22.212.106/tcp/16591/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD"
  ]

  bvn-bootstrap-peers = [
    "/ip4/23.22.212.106/tcp/16691/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD"
  ]

[logging]
  format = "plain"
  [[logging.rules]]
    level = "info"
```

## Database Management

### Historical Snapshots Preserved

✅ **Source databases are NEVER modified**

The tool **copies** databases from:
- `/media/paul/Expansion/databases/2025-10-13-dn/`
- `/media/paul/Expansion/databases/2025-10-13-bvn/`

To:
- `{work_dir}/dnn/`
- `{work_dir}/bvnn/`

**Benefits:**
- Original snapshots remain pristine for historical reference
- Can create multiple followers from same snapshots
- Safe experimentation - corruption only affects copies

**Cost:**
- Requires ~2GB additional disk space per follower
- Initial setup takes time to copy databases

### Multiple Followers

Create multiple independent followers:

```json
// Follower 1
{
  "tool": "accumulate_init_follower",
  "arguments": {
    "dn_database": "/media/paul/Expansion/databases/2025-10-13-dn",
    "bvn_database": "/media/paul/Expansion/databases/2025-10-13-bvn",
    "work_dir": "/home/paul/follower-1",
    "container_name": "accumulate-follower-1"
  }
}

// Follower 2
{
  "tool": "accumulate_init_follower",
  "arguments": {
    "dn_database": "/media/paul/Expansion/databases/2025-10-13-dn",
    "bvn_database": "/media/paul/Expansion/databases/2025-10-13-bvn",
    "work_dir": "/home/paul/follower-2",
    "container_name": "accumulate-follower-2"
  }
}
```

## Querying the Follower

Once running, query the follower using standard MCP tools:

```json
{
  "tool": "accumulate_query_account",
  "arguments": {
    "url": "acc://dn.acme",
    "network": "http://localhost:16592/v3"
  }
}
```

**Follower Endpoints:**
- DN v3 API: `http://localhost:16592/v3`
- BVN v3 API: `http://localhost:16692/v3`

## Monitoring and Logs

### View Live Logs

```bash
docker logs -f accumulate-follower
```

### Check Container Stats

```bash
docker stats accumulate-follower
```

### Inspect Container

```bash
docker inspect accumulate-follower
```

### Access Container Shell

```bash
docker exec -it accumulate-follower sh
```

## Troubleshooting

### Container Won't Start

**Check logs:**
```json
{
  "tool": "accumulate_follower_status",
  "arguments": {
    "container_name": "accumulate-follower"
  }
}
```

The `recent_logs` field will show why the container stopped.

**Common issues:**
- Database corruption - re-copy from snapshots
- Port conflicts - check if ports 16591-16693 are in use
- Insufficient resources - check Docker memory limits

### Port Already in Use

```bash
# Check what's using the ports
sudo lsof -i :16591-16593
sudo lsof -i :16691-16693

# Use different ports (modify after init)
# Edit work_dir/accumulate.toml
# Then restart container
```

### Database Initialization Failed

```bash
# Check source databases exist
ls -la /media/paul/Expansion/databases/2025-10-13-dn/
ls -la /media/paul/Expansion/databases/2025-10-13-bvn/

# Check disk space
df -h

# Check permissions
ls -la /home/paul/accumulate-follower/
```

### Container Exists But Not Running

```bash
# View why it stopped
docker logs accumulate-follower

# Remove and recreate
docker rm -f accumulate-follower
```

Then re-run `accumulate_run_follower`.

## Complete Workflow Example

```json
// 1. Initialize
{
  "tool": "accumulate_init_follower",
  "arguments": {
    "dn_database": "/media/paul/Expansion/databases/2025-10-13-dn",
    "bvn_database": "/media/paul/Expansion/databases/2025-10-13-bvn",
    "work_dir": "/home/paul/accumulate-follower",
    "bvn_name": "Cyclops"
  }
}

// 2. Start
{
  "tool": "accumulate_run_follower",
  "arguments": {
    "work_dir": "/home/paul/accumulate-follower"
  }
}

// 3. Check status
{
  "tool": "accumulate_follower_status",
  "arguments": {
    "container_name": "accumulate-follower"
  }
}

// 4. Query the follower
{
  "tool": "accumulate_query_account",
  "arguments": {
    "url": "acc://dn.acme",
    "network": "http://localhost:16592/v3"
  }
}

// 5. Stop when done
{
  "tool": "accumulate_stop_follower",
  "arguments": {
    "container_name": "accumulate-follower"
  }
}
```

## Integration with backupdbs MCP

Use backupdbs MCP to explore and verify databases before creating followers:

```json
// 1. Open database in backupdbs MCP
{
  "tool": "db_open",
  "arguments": {
    "path": "/media/paul/Expansion/databases/2025-10-13-dn"
  }
}

// 2. Verify database integrity
{
  "tool": "db_info",
  "arguments": {
    "session_id": "SESSION_ID_FROM_STEP_1"
  }
}

// 3. Use verified database for follower
{
  "tool": "accumulate_init_follower",
  "arguments": {
    "dn_database": "/media/paul/Expansion/databases/2025-10-13-dn",
    "bvn_database": "/media/paul/Expansion/databases/2025-10-13-bvn",
    "work_dir": "/home/paul/accumulate-follower"
  }
}
```

## Benefits of Docker Deployment

✅ **Isolation** - Follower runs in isolated container
✅ **Easy Management** - Start/stop/remove with simple commands
✅ **Consistent Environment** - Official Accumulate image
✅ **Resource Limits** - Docker can enforce memory/CPU limits
✅ **Auto-restart** - Container restarts automatically
✅ **Multiple Instances** - Run many followers easily
✅ **Clean Removal** - Remove container without affecting host

## Next Steps

- Monitor sync progress via logs
- Query follower endpoints
- Set up monitoring/alerting
- Configure custom bootstrap peers for your network

## References

- [Accumulate Documentation](https://docs.accumulatenetwork.io)
- [Docker Documentation](https://docs.docker.com)
- [Accumulate Docker Image](https://gitlab.com/accumulatenetwork/accumulate/container_registry)
