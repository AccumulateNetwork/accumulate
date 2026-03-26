# Bootstrap Server Quick Fix

## Problem

Bootstrap server at `bootstrap.accumulate.defidevs.io` only listens on port 16593 (DN), not port 16693 (BVN).

## Impact

- **Bootstrap server**: ONE node to update
- **No accumulated updates needed**: All nodes already have correct hardcoded addresses
- **No network-wide deployment**: Just restart one server

## Solution

Add port 16693 to the bootstrap server's listen addresses.

## Deployment Commands

### Option 1: Docker (Recommended)

```bash
# SSH to bootstrap.accumulate.defidevs.io
ssh bootstrap.accumulate.defidevs.io

# Stop current container
docker stop accumulate-bootstrap
docker rm accumulate-bootstrap

# Run with both P2P ports and info server
docker run -d \
  --name accumulate-bootstrap \
  --restart unless-stopped \
  -p 16593:16593 \
  -p 16693:16693 \
  -p 8080:8080 \
  -v /opt/accumulate/bootstrap-key.txt:/key.txt:ro \
  registry.gitlab.com/accumulatenetwork/accumulate:latest \
  accumulated-bootstrap \
  --key /key.txt \
  --listen /ip4/0.0.0.0/tcp/16593 \
  --listen /ip4/0.0.0.0/tcp/16693 \
  --external /dns/bootstrap.accumulate.defidevs.io/tcp/16593 \
  --external /dns/bootstrap.accumulate.defidevs.io/tcp/16693
```

Note: Port 8080 is for the info server (enabled by default). Port 8081 (Prometheus) is NOT exposed for security.

### Option 2: Systemd Service

```bash
# SSH to bootstrap.accumulate.defidevs.io
ssh bootstrap.accumulate.defidevs.io

# Edit the service file
sudo systemctl stop accumulate-bootstrap
sudo nano /etc/systemd/system/accumulate-bootstrap.service

# Update ExecStart line to include both listen addresses:
# ExecStart=/usr/local/bin/accumulated-bootstrap \
#   --key /opt/accumulate/bootstrap-key.txt \
#   --listen /ip4/0.0.0.0/tcp/16593 \
#   --listen /ip4/0.0.0.0/tcp/16693 \
#   --external /dns/bootstrap.accumulate.defidevs.io/tcp/16593 \
#   --external /dns/bootstrap.accumulate.defidevs.io/tcp/16693

# Reload and restart
sudo systemctl daemon-reload
sudo systemctl start accumulate-bootstrap
sudo systemctl status accumulate-bootstrap
```

### Option 3: Direct Binary

```bash
# SSH to bootstrap.accumulate.defidevs.io
ssh bootstrap.accumulate.defidevs.io

# Stop existing process
pkill accumulated-bootstrap

# Start with both ports (adjust paths as needed)
nohup accumulated-bootstrap \
  --key /opt/accumulate/bootstrap-key.txt \
  --listen /ip4/0.0.0.0/tcp/16593 \
  --listen /ip4/0.0.0.0/tcp/16693 \
  --external /dns/bootstrap.accumulate.defidevs.io/tcp/16593 \
  --external /dns/bootstrap.accumulate.defidevs.io/tcp/16693 \
  > /var/log/bootstrap.log 2>&1 &
```

## Verification

```bash
# Check P2P ports are listening
nc -zv bootstrap.accumulate.defidevs.io 16593
nc -zv bootstrap.accumulate.defidevs.io 16693

# Should both show: "Connection to bootstrap.accumulate.defidevs.io 16593/16693 port [tcp/*] succeeded!"

# Check info server
curl http://bootstrap.accumulate.defidevs.io:8080/info | jq
curl http://bootstrap.accumulate.defidevs.io:8080/health | jq
```

## Firewall (if needed)

```bash
# Open required ports
sudo ufw allow 16693/tcp comment "Bootstrap BVN P2P"
sudo ufw allow 8080/tcp comment "Bootstrap Info Server"

# Or using iptables
sudo iptables -A INPUT -p tcp --dport 16693 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 8080 -j ACCEPT
sudo iptables-save > /etc/iptables/rules.v4
```

## That's It!

No need to:
- ❌ Update accumulated code
- ❌ Rebuild binaries
- ❌ Deploy to validators
- ❌ Update follower nodes
- ❌ Change any hardcoded values

The bootstrap server is **infrastructure** - just one machine to fix!
