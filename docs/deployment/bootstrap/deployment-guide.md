# Bootstrap Server Deployment Guide

## Overview

The Accumulate bootstrap server is a dedicated libp2p DHT node that helps new nodes discover peers on the network. While not strictly required (nodes can discover each other via gossipsub), it significantly speeds up initial peer discovery.

## Current Deployment

- **Host**: bootstrap.accumulate.defidevs.io (3.138.61.111)
- **Status**: Partially operational
  - ✅ Port 16593 (DN): Running
  - ❌ Port 16693 (BVN): Not running

## Recommended Configuration

### Multi-Port Deployment

The bootstrap server should listen on both DN and BVN ports to serve all network partitions:

```bash
accumulated-bootstrap \
  --key /path/to/bootstrap-key.txt \
  --listen /ip4/0.0.0.0/tcp/16593 \
  --listen /ip4/0.0.0.0/tcp/16693 \
  --external /dns/bootstrap.accumulate.defidevs.io/tcp/16593 \
  --external /dns/bootstrap.accumulate.defidevs.io/tcp/16693 \
  --prom-listen /ip4/0.0.0.0/tcp/8081/http
```

### Key Management

**IMPORTANT**: The node key must be persistent! Generate and save it once:

```bash
# Generate a new key (first time only)
accumulated key gen > bootstrap-key.txt

# Verify the key
cat bootstrap-key.txt

# The key should look like: acc://[hash]/ACME
```

**Never regenerate the key** - this changes the peer ID and breaks all existing configurations that reference the bootstrap server.

### Current Key Information

The bootstrap server peer ID: `12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx`

This peer ID is hardcoded in:
- `accumulate/mcp/server/tools_accman_artifacts.go`
- `accumulate-dual-data/accumulate.toml`
- Various deployment configurations

## Docker Deployment

### Build Image

```bash
# From accumulate repository root
docker build -t accumulated-bootstrap:latest -f cmd/accumulated-bootstrap/Dockerfile .
```

### Run Container

```bash
docker run -d \
  --name accumulate-bootstrap \
  --restart unless-stopped \
  -p 16593:16593 \
  -p 16693:16693 \
  -p 8081:8081 \
  -v /opt/accumulate/bootstrap-key.txt:/key.txt:ro \
  accumulated-bootstrap:latest \
  --key /key.txt \
  --listen /ip4/0.0.0.0/tcp/16593 \
  --listen /ip4/0.0.0.0/tcp/16693 \
  --external /dns/bootstrap.accumulate.defidevs.io/tcp/16593 \
  --external /dns/bootstrap.accumulate.defidevs.io/tcp/16693 \
  --prom-listen /ip4/0.0.0.0/tcp/8081/http
```

### Verify Deployment

```bash
# Check if ports are listening
nc -zv bootstrap.accumulate.defidevs.io 16593
nc -zv bootstrap.accumulate.defidevs.io 16693

# Check Prometheus metrics
curl http://bootstrap.accumulate.defidevs.io:8081/metrics | grep libp2p

# Check logs
docker logs accumulate-bootstrap
```

## Systemd Service (Alternative to Docker)

Create `/etc/systemd/system/accumulate-bootstrap.service`:

```ini
[Unit]
Description=Accumulate Bootstrap Server
After=network.target

[Service]
Type=simple
User=accumulate
Group=accumulate
WorkingDirectory=/opt/accumulate
ExecStart=/usr/local/bin/accumulated-bootstrap \
  --key /opt/accumulate/bootstrap-key.txt \
  --listen /ip4/0.0.0.0/tcp/16593 \
  --listen /ip4/0.0.0.0/tcp/16693 \
  --external /dns/bootstrap.accumulate.defidevs.io/tcp/16593 \
  --external /dns/bootstrap.accumulate.defidevs.io/tcp/16693 \
  --prom-listen /ip4/0.0.0.0/tcp/8081/http
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl enable accumulate-bootstrap
sudo systemctl start accumulate-bootstrap
sudo systemctl status accumulate-bootstrap
```

## Firewall Configuration

Ensure the following ports are open:

```bash
# UFW
sudo ufw allow 16593/tcp comment "Accumulate Bootstrap DN"
sudo ufw allow 16693/tcp comment "Accumulate Bootstrap BVN"
sudo ufw allow 8081/tcp comment "Prometheus Metrics"

# iptables
sudo iptables -A INPUT -p tcp --dport 16593 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 16693 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 8081 -j ACCEPT
sudo iptables-save > /etc/iptables/rules.v4
```

## Monitoring

### Prometheus Metrics

Available at `http://bootstrap.accumulate.defidevs.io:8081/metrics`

Key metrics:
- `libp2p_peers` - Number of connected peers
- `libp2p_dht_peers` - DHT routing table size
- `go_goroutines` - Number of goroutines (health indicator)

### Health Check Script

```bash
#!/bin/bash
# /opt/accumulate/check-bootstrap.sh

# Check if ports are listening
nc -z localhost 16593 || exit 1
nc -z localhost 16693 || exit 1

# Check metrics endpoint
curl -sf http://localhost:8081/metrics > /dev/null || exit 1

echo "Bootstrap server healthy"
exit 0
```

### Log Monitoring

```bash
# Journalctl (systemd)
journalctl -u accumulate-bootstrap -f

# Docker
docker logs -f accumulate-bootstrap
```

## Troubleshooting

### Port Not Listening

```bash
# Check what's using the ports
sudo lsof -i :16593
sudo lsof -i :16693

# Check if process is running
ps aux | grep accumulated-bootstrap
```

### Peer ID Mismatch

If you see errors about peer ID mismatch, the key file has changed. This requires:
1. Update all hardcoded references to the new peer ID
2. Notify all node operators to update their configurations

### No Peers Connecting

The bootstrap server doesn't require peers to connect to it. It's purely for helping others discover peers. Check that:
- The external addresses are correct
- DNS resolves properly: `dig bootstrap.accumulate.defidevs.io`
- Ports are accessible from external networks

## Updating the Deployment

To update the bootstrap server:

```bash
# Docker
docker pull accumulated-bootstrap:latest
docker stop accumulate-bootstrap
docker rm accumulate-bootstrap
# Run with new image (see "Run Container" above)

# Systemd
sudo systemctl stop accumulate-bootstrap
sudo cp /path/to/new/accumulated-bootstrap /usr/local/bin/
sudo systemctl start accumulate-bootstrap
```

**IMPORTANT**: Always preserve the key file when updating!

## Migration Checklist

To migrate from single-port (16593) to dual-port deployment:

- [ ] Backup current key file
- [ ] Stop existing bootstrap server
- [ ] Update run command to include both listen addresses
- [ ] Update firewall rules to allow port 16693
- [ ] Start bootstrap server with new configuration
- [ ] Verify both ports are listening: `nc -zv bootstrap.accumulate.defidevs.io 16593 16693`
- [ ] Monitor logs for any errors
- [ ] Update DNS if hostname changed
- [ ] Test from a follower node

## References

- Source code: `cmd/accumulated-bootstrap/main.go`
- Bootstrap architecture: `mcp/BOOTSTRAP_ARCHITECTURE.md`
- Network topology: `MAINNET_TOPOLOGY_2025-11-17.md`
- libp2p documentation: https://docs.libp2p.io/
- Kademlia DHT: https://docs.libp2p.io/concepts/discovery-routing/kaddht/

## Support

For issues or questions:
1. Check logs first
2. Verify network connectivity
3. Confirm key file is unchanged
4. Check if peers are discovering each other (bootstrap is optional)
