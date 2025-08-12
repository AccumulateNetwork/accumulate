# Bootstrap Server Management Guide

## Overview
The bootstrap server is critical infrastructure that helps nodes discover each other on the Accumulate network. It must be kept up-to-date with the actual active nodes to prevent connection failures.

## Current Bootstrap Server

**Location**: AWS EC2 Instance
- **Instance ID**: i-0e053e32862689726
- **Region**: us-east-2
- **DNS**: bootstrap.accumulate.defidevs.io
- **IP**: 3.138.61.111
- **Peer ID**: 12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg

## Access Instructions

### Method 1: EC2 Instance Connect (Temporary SSH)
```bash
# Generate temporary access (60 seconds validity)
aws ec2-instance-connect send-ssh-public-key \
    --region us-east-2 \
    --instance-id i-0e053e32862689726 \
    --instance-os-user ubuntu \
    --ssh-public-key file://~/.ssh/id_rsa.pub

# Connect immediately
ssh ubuntu@bootstrap.accumulate.defidevs.io
```

### Method 2: SSM Session Manager
```bash
aws ssm start-session \
    --region us-east-2 \
    --target i-0e053e32862689726
```

## Bootstrap Configuration

The bootstrap server runs as a Docker container with the following configuration:

### Current Startup Script
Located at: `/home/ubuntu/start-mainnet-bootstrap.sh`

```bash
#!/bin/bash
docker stop accumulate-bootstrap
docker rm accumulate-bootstrap

docker run -d \
    --restart unless-stopped \
    -v accumulate-bootstrap:/data \
    --name accumulate-bootstrap \
    -p 0.0.0.0:16593:16593 \
    -p 0.0.0.0:16693:16693 \
    --entrypoint accumulated-bootstrap \
    registry.gitlab.com/accumulatenetwork/accumulate:seed \
    --key /data/key \
    --peer [ACTIVE_PEER_1] \
    --peer [ACTIVE_PEER_2] \
    --peer [ACTIVE_PEER_3] \
    --listen /ip4/0.0.0.0/tcp/16593 \
    --listen /ip4/0.0.0.0/udp/16593/quic \
    --external /dns/bootstrap.accumulate.defidevs.io/tcp/16593 \
    --external /dns/bootstrap.accumulate.defidevs.io/udp/16593/quic
```

## Key Management Principles

### 1. Only Advertise Active Nodes
The bootstrap MUST only include `--peer` entries for nodes that are:
- Currently running
- Accessible on the network
- Have valid peer IDs

### 2. Regular Updates Required
Update the bootstrap when:
- Adding a new node to the network
- Removing a node from the network  
- Node peer IDs change
- Node addresses change

### 3. Peer Format
Each peer must be specified in multiaddr format:
```
/dns/[hostname]/tcp/[port]/p2p/[peer-id]
```

Example:
```
/dns/apollo-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWPs19932secARrxoRR5J8ZtBMt2vqwyHH1Q9p8thYP7cn
```

## Common Operations

### Check Bootstrap Status
```bash
# SSH into bootstrap server
ssh ubuntu@bootstrap.accumulate.defidevs.io

# Check container status
docker ps | grep bootstrap

# View logs
docker logs --tail 50 accumulate-bootstrap

# Check peer connections
docker logs accumulate-bootstrap 2>&1 | grep "We are"
```

### Update Peer List
```bash
# SSH into bootstrap server
ssh ubuntu@bootstrap.accumulate.defidevs.io

# Edit the startup script
nano start-mainnet-bootstrap.sh

# Add/remove --peer lines as needed
# Save and exit

# Restart bootstrap
sudo ./start-mainnet-bootstrap.sh

# Verify it's running
docker ps | grep bootstrap
```

### Get a Node's Peer ID
To add a new node to bootstrap, you need its peer ID:

```bash
# On the target node
accumulated key export --key-type p2p

# Or via API
curl -X POST http://[node-ip]:16595/v3 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"node-info","params":{},"id":1}' \
  | jq -r .result.peerID
```

### Emergency Recovery
If the bootstrap server fails:

```bash
# Restart the container
docker restart accumulate-bootstrap

# If that fails, recreate it
docker stop accumulate-bootstrap
docker rm accumulate-bootstrap
./start-mainnet-bootstrap.sh

# If the key is lost (creates new peer ID - avoid this!)
docker volume rm accumulate-bootstrap
./start-mainnet-bootstrap.sh
# Note: This changes the bootstrap peer ID and requires updating all nodes!
```

## Monitoring

### Health Checks
```bash
# Check if port is listening
netstat -tlnp | grep 16593

# Test connectivity from external host
telnet bootstrap.accumulate.defidevs.io 16593

# Test with debug tool
./debug test-p2p mainnet
```

### Log Analysis
```bash
# Check for connection errors
docker logs accumulate-bootstrap 2>&1 | grep -i error

# Monitor peer connections
docker logs -f accumulate-bootstrap 2>&1 | grep "peer"

# Check for rejected connections
docker logs accumulate-bootstrap 2>&1 | grep -i "reject\|deny\|fail"
```

## Troubleshooting

### Bootstrap Not Accepting Connections
1. Check Docker is running: `docker ps`
2. Check ports are open: `netstat -tlnp | grep 16593`
3. Check firewall/security groups in AWS
4. Verify DNS resolution: `nslookup bootstrap.accumulate.defidevs.io`

### Nodes Can't Find Peers
1. Verify bootstrap is advertising correct peers
2. Check that advertised peers are actually running
3. Verify peer IDs match actual nodes
4. Test connectivity to each advertised peer

### High CPU/Memory Usage
1. Check for connection spam: `docker logs accumulate-bootstrap | tail -1000 | grep "connection" | wc -l`
2. Restart if needed: `docker restart accumulate-bootstrap`
3. Consider rate limiting if under attack

## Best Practices

1. **Document Changes**: Always document when and why bootstrap configuration was changed
2. **Test Before Deploy**: Test new peer connections before adding to bootstrap
3. **Backup Configuration**: Keep backup of working configurations
4. **Monitor Regularly**: Check bootstrap health daily
5. **Coordinate Updates**: Notify team before making changes

## Configuration Template

Save this template for adding new nodes:

```bash
# Template for adding a new node to bootstrap
--peer /dns/[NODE_NAME]-mainnet.accumulate.defidevs.io/tcp/16593/p2p/[PEER_ID]

# Example:
--peer /dns/newnode-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWABCDEF...
```

## Contact Information

For bootstrap server issues:
- **AWS Access**: Contact DevOps team
- **Configuration Help**: See network-expansion/README.md
- **Emergency**: Contact on-call engineer

## Appendix: Current Active Nodes (August 2024)

| Node | DNS | Peer ID | Status |
|------|-----|---------|--------|
| apollo | apollo-mainnet.accumulate.defidevs.io | 12D3KooWPs19932secARrxoRR5J8ZtBMt2vqwyHH1Q9p8thYP7cn | Active |
| yutu | yutu-mainnet.accumulate.defidevs.io | 12D3KooWJqp6jpagL2cJwhBX3aWJvqCUf46ceYxpqQXKFrgPGRCT | Active |
| chandrayaan | chandrayaan-mainnet.accumulate.defidevs.io | [TBD - needs discovery] | Unknown |

**Action Required**: Determine chandrayaan's peer ID and add to bootstrap configuration.