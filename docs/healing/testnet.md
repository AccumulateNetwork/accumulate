# Accumulate Testnet Healing Guide

This document provides comprehensive instructions for troubleshooting and repairing the Kermit testnet healing process. It covers network configuration, container setup, system optimization, and common issues.

## Overview

The Accumulate network healing process ensures the blockchain remains healthy by:
- Anchoring data between partitions
- Healing synthetic transactions
- Maintaining network connectivity

For testnets like Kermit, the healing process runs in Docker containers on dedicated servers.

## Healing Server Configuration

### Server Information
- **Healing Server**: `chandrayaan-follower-0.accumulate.defidevs.io`
- **Cache Directory**: `~/.accumulate/cache/`
- **Debug Tool Path**: `/root/debug`

### System Requirements

For optimal QUIC protocol performance, increase UDP buffer sizes:

```bash
# Increase UDP receive buffer size
sysctl -w net.core.rmem_max=2097152
sysctl -w net.core.rmem_default=2097152

# Increase UDP send buffer size
sysctl -w net.core.wmem_max=2097152
sysctl -w net.core.wmem_default=2097152

# Make changes persistent
echo "net.core.rmem_max=2097152" >> /etc/sysctl.conf
echo "net.core.rmem_default=2097152" >> /etc/sysctl.conf
echo "net.core.wmem_max=2097152" >> /etc/sysctl.conf
echo "net.core.wmem_default=2097152" >> /etc/sysctl.conf
```

## Debug Tool Commands

### Network Scan Command

The network scan command is used to generate network configuration files:

```bash
# Basic network scan
/root/debug network scan Kermit -j > ~/.accumulate/cache/kermit.json

# Network scan with custom bootstrap peer
/root/debug network scan Kermit -j --bootstrap /dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg > ~/.accumulate/cache/kermit.json

# Network scan with direct HTTP endpoints
/root/debug network scan Kermit -j --http http://kermit-bvn0.accumulate.defidevs.io:16692 > ~/.accumulate/cache/kermit-http.json
```

### Healing Commands

```bash
# Anchor healing
/root/debug heal anchor Kermit --max-response-age 5m --cached-scan /path/to/scan.json --peer-db /path/to/peers.json --continuous

# Synthetic transaction healing
/root/debug heal synth Kermit --cached-scan /path/to/scan.json --peer-db /path/to/peers.json --light-db /path/to/db --continuous --since 2h

# Healing specific transactions
/root/debug heal anchor Kermit txid
/root/debug heal synth Kermit txid
```

## Network Configuration

### Creating Network Configuration Files

For reliable healing, create a comprehensive network configuration file that includes both HTTP endpoints and bootstrap peers:

```json
{
  "network": "Kermit",
  "bootstrap": [
    "/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg"
  ],
  "partitions": [
    {
      "id": "directory",
      "type": "directory",
      "basePort": 16590,
      "nodes": [
        {
          "address": "http://kermit-dn.accumulate.defidevs.io:16692"
        }
      ]
    },
    {
      "id": "bvn0",
      "type": "bvn",
      "basePort": 16690,
      "nodes": [
        {
          "address": "http://kermit-bvn0.accumulate.defidevs.io:16692"
        }
      ]
    },
    {
      "id": "bvn1",
      "type": "bvn",
      "basePort": 16790,
      "nodes": [
        {
          "address": "http://kermit-bvn1.accumulate.defidevs.io:16692"
        }
      ]
    },
    {
      "id": "bvn2",
      "type": "bvn",
      "basePort": 16890,
      "nodes": [
        {
          "address": "http://kermit-bvn2.accumulate.defidevs.io:16692"
        }
      ]
    }
  ]
}
```

Save this file as `~/.accumulate/cache/kermit-bootstrap.json`.

### Peer Database

Create an empty peer database file to track peers between container restarts:

```bash
touch ~/.accumulate/cache/kermit-peerdb.json
```

## Healing Container Setup

### Anchor Healing Container

```bash
docker run -d --name kermit-heal-anchors --restart unless-stopped --pull always \
  -v "${HOME}/.accumulate/cache:/data" --entrypoint debug \
  registry.gitlab.com/accumulatenetwork/accumulate:v1-4-0-alpha-5 \
  heal anchor Kermit --max-response-age 5m \
  --cached-scan /data/kermit-bootstrap.json --peer-db /data/kermit-peerdb.json --continuous
```

### Synthetic Healing Container

```bash
docker run -d --name kermit-heal-synthetic --restart unless-stopped --pull always \
  -m 4g -v "${HOME}/.accumulate/cache:/data" --entrypoint debug \
  registry.gitlab.com/accumulatenetwork/accumulate:v1-4-0-alpha-5 \
  heal synth Kermit --cached-scan /data/kermit-bootstrap.json --peer-db /data/kermit-peerdb.json \
  --light-db /data/kermit.db --continuous --since 2h
```

## Troubleshooting

### Common Issues and Solutions

1. **Network Connectivity Issues**
   - Verify Kermit nodes are accessible: `curl http://kermit-bvn0.accumulate.defidevs.io:16692/status`
   - Check if nodes are in sync (not catching up): `curl -s http://kermit-bvn0.accumulate.defidevs.io:16692/status | grep -E 'latest_block|catching_up'`
   - Use explicit bootstrap peers in the network configuration

2. **Stale Response Warnings**
   - Increase `--max-response-age` parameter (default is 1m)
   - Ensure network time synchronization on the healing server

3. **Nil Pointer Dereference**
   - Use a comprehensive network configuration with both HTTP endpoints and bootstrap peers
   - Ensure the peer database file exists and is accessible

4. **UDP Buffer Size Warnings**
   - Increase system UDP buffer sizes as described in the System Requirements section
   - Restart containers after making buffer size changes

5. **Container Restart Issues**
   - Remove containers before recreating them: `docker rm -f kermit-heal-anchors kermit-heal-synthetic`
   - Check container logs for specific errors: `docker logs kermit-heal-anchors --tail 20`

## Monitoring

### Checking Container Status

```bash
# View container logs
docker logs kermit-heal-anchors --tail 20
docker logs kermit-heal-synthetic --tail 20

# Check container status
docker ps -a | grep kermit-heal
```

### Verifying Network Health

```bash
# Check node status
curl -s http://kermit-bvn0.accumulate.defidevs.io:16692/status

# Verify Explorer API connectivity
curl -s -X POST http://kermit-bvn0.accumulate.defidevs.io:16692/ -d '{"jsonrpc":"2.0","id":1,"method":"describe","params":{}}'
```

## API Endpoints and Explorer

### API Endpoints

The Accumulate API is accessible through JSON-RPC endpoints. The main API endpoint for Kermit testnet nodes is:

```
http://<node-address>:16692/
```

Common API methods include:

- `query`: Query account data
- `query-directory`: Get directory information
- `describe`: Get API description
- `status`: Get node status

Example API call:

```bash
curl -s -X POST http://kermit-bvn0.accumulate.defidevs.io:16692/ -d '{"jsonrpc":"2.0","id":1,"method":"query","params":{"url":"acc://dn.acme"}}'
```

### Explorer Configuration

The Accumulate Explorer connects to the Kermit testnet through API endpoints. If the Explorer shows "API call failed":

1. Verify the API endpoints are accessible
2. Check if the Explorer is configured to use the correct API endpoint
3. Ensure the healing process is running correctly
4. Verify that the nodes are synced and not catching up

The Explorer typically connects to a load balancer that distributes requests across multiple nodes. If a specific node is down, the Explorer may fail to load data.

#### Troubleshooting Explorer Issues

If the Explorer shows "API call failed":

1. **Check API Endpoint Availability**
   ```bash
   # Test basic connectivity
   curl -s http://kermit-bvn0.accumulate.defidevs.io:16692/status
   
   # Test API method
   curl -s -X POST http://kermit-bvn0.accumulate.defidevs.io:16692/ -d '{"jsonrpc":"2.0","id":1,"method":"describe","params":{}}'
   ```

2. **Check Load Balancer**
   If using a load balancer, verify all backend nodes are healthy.

3. **Check CORS Configuration**
   Explorer may fail if CORS is not properly configured on the API endpoints.

4. **Restart Explorer Service**
   If the Explorer is running as a service, try restarting it:
   ```bash
   systemctl restart accumulate-explorer
   ```

5. **Check Explorer Logs**
   ```bash
   journalctl -u accumulate-explorer -f
   ```

## Maintenance

### Updating Containers

To update to a new version of the healing containers:

```bash
docker stop kermit-heal-anchors kermit-heal-synthetic
docker rm kermit-heal-anchors kermit-heal-synthetic
# Run the container creation commands with the new version tag
```

### Container Management

```bash
# View running containers
docker ps

# View all containers (including stopped)
docker ps -a

# View container logs
docker logs kermit-heal-anchors
docker logs kermit-heal-synthetic

# Follow container logs (stream in real-time)
docker logs -f kermit-heal-anchors

# View container resource usage
docker stats kermit-heal-anchors kermit-heal-synthetic

# Execute commands inside container
docker exec -it kermit-heal-anchors /bin/sh
```

### Automatic Container Recovery

The healing containers are configured with `--restart unless-stopped` which means:

- They will automatically restart if they crash
- They will restart when the host system reboots
- They will not restart if manually stopped with `docker stop`

To change the restart policy:

```bash
docker update --restart=always kermit-heal-anchors kermit-heal-synthetic
```

### Regenerating Cache Files

If cache files become corrupted or outdated:

1. Stop healing containers
2. Remove existing cache files
3. Recreate network configuration files
4. Restart healing containers

## Advanced Configuration

### Custom Bootstrap Peers

If the default bootstrap peers are not accessible, you can specify custom bootstrap peers:

```bash
/root/debug network scan kermit -j --bootstrap /ip4/1.2.3.4/tcp/16593/p2p/12D3KooWXXXX > ~/.accumulate/cache/kermit.json
```

### Healing Specific Transactions

To heal specific transactions rather than running continuous healing:

```bash
/root/debug heal anchor Kermit txid
/root/debug heal synth Kermit txid
```

### Performance Tuning

For better performance on high-load systems:

1. **Container Resources**:
   - Increase memory allocation for synthetic healing (`-m 8g`)
   - Use CPU limits if necessary (`--cpus 2`)

2. **Healing Parameters**:
   - Adjust `--since` parameter to control how far back to look for transactions
   - Use `--max-response-age` to tune stale response sensitivity

## References

- [Accumulate Network Documentation](https://docs.accumulatenetwork.io/)
- [QUIC UDP Buffer Sizes](https://github.com/quic-go/quic-go/wiki/UDP-Buffer-Sizes)
- [Docker Documentation](https://docs.docker.com/)
- [Accumulate API Documentation](https://docs.accumulatenetwork.io/api/)
