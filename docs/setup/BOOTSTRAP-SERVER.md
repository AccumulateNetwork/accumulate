# Bootstrap Server Configuration Guide

## Overview

The bootstrap server is a critical infrastructure component that helps new nodes join the Accumulate network by providing initial peer addresses. This document details the configuration, deployment, and maintenance of bootstrap servers.

## Architecture

```
┌──────────────────────────────────────────┐
│          DNS Resolution                  │
│   bootstrap.accumulate.defidevs.io       │
└─────────────────┬────────────────────────┘
                  │
                  ▼
┌──────────────────────────────────────────┐
│        Bootstrap Server (AWS)            │
│         54.211.10.186:16593              │
│      EC2: i-0e053e32862689726            │
│         Region: us-east-2                │
└─────────────────┬────────────────────────┘
                  │
                  ▼
┌──────────────────────────────────────────┐
│      Docker Container                    │
│     accumulated-bootstrap:latest         │
│   Maintains active peer list             │
└──────────────────────────────────────────┘
```

## Configuration Files

### 1. Source Code Configuration

Location: `pkg/accumulate/api.go`

```go
var BootstrapServers = func() []multiaddr.Multiaddr {
    s := []string{
        "/dns/apollo-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWPs19932secARrxoRR5J8ZtBMt2vqwyHH1Q9p8thYP7cn",
        "/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg",
    }
    // Convert strings to multiaddr
    return parseMultiaddrs(s)
}
```

### 2. Docker Container Configuration

The bootstrap server runs as a Docker container with the following configuration:

```dockerfile
# Runtime command
docker run -d \
  --name accumulated-bootstrap \
  --restart unless-stopped \
  -p 16593:16593 \
  -v /path/to/config:/config \
  accumulated:latest bootstrap \
  --network="MainNet" \
  --listen="/ip4/0.0.0.0/tcp/16593" \
  --peer="/dns/apollo-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWPs19932secARrxoRR5J8ZtBMt2vqwyHH1Q9p8thYP7cn" \
  --peer="/dns/yutu-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWJBHQv4w5hqRxPRZDaVvLnPbRqRdKfMQx3dkfBjQgvgBP" \
  --peer="/dns/chandrayaan-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWM7cqxuGYXgAzcQR5jGQ2pPb8QMNachz7YjqFxaa9phPm"
```

## Deployment Process

### Initial Setup

1. **Launch EC2 Instance**
   ```bash
   # Instance details
   Type: t3.medium
   AMI: Ubuntu 22.04 LTS
   Security Group: Allow TCP 16593
   Region: us-east-2
   ```

2. **Install Docker**
   ```bash
   sudo apt update
   sudo apt install -y docker.io
   sudo systemctl enable docker
   sudo usermod -aG docker ubuntu
   ```

3. **Deploy Bootstrap Container**
   ```bash
   # Pull latest image
   docker pull accumulated:latest
   
   # Run bootstrap server
   docker run -d \
     --name accumulated-bootstrap \
     --restart unless-stopped \
     -p 16593:16593 \
     accumulated:latest bootstrap \
     --network="MainNet" \
     --peer="<peer-multiaddrs>"
   ```

4. **Configure DNS**
   ```
   bootstrap.accumulate.defidevs.io → 54.211.10.186
   ```

### AWS Access Configuration

The bootstrap server uses EC2 Instance Connect for temporary SSH access:

```bash
# Generate temporary key
ssh-keygen -t rsa -b 2048 -f bootstrap_key -N ''

# Send public key to instance
aws ec2-instance-connect send-ssh-public-key \
  --region us-east-2 \
  --instance-id i-0e053e32862689726 \
  --instance-os-user ubuntu \
  --ssh-public-key file://bootstrap_key.pub

# Connect (within 60 seconds)
ssh -i bootstrap_key ubuntu@54.211.10.186
```

## Maintenance Procedures

### Updating Peer List

When network topology changes:

1. **Access the server**
   ```bash
   # Use EC2 Instance Connect as shown above
   ```

2. **Stop current container**
   ```bash
   docker stop accumulated-bootstrap
   docker rm accumulated-bootstrap
   ```

3. **Start with new peer list**
   ```bash
   docker run -d \
     --name accumulated-bootstrap \
     --restart unless-stopped \
     -p 16593:16593 \
     accumulated:latest bootstrap \
     --network="MainNet" \
     --peer="<new-peer-1>" \
     --peer="<new-peer-2>" \
     --peer="<new-peer-3>"
   ```

4. **Verify operation**
   ```bash
   docker logs accumulated-bootstrap
   nc -zv localhost 16593
   ```

### Monitoring

Check bootstrap server health:

```bash
# Container status
docker ps | grep accumulated-bootstrap

# Logs
docker logs -f accumulated-bootstrap

# Network connections
ss -tunap | grep 16593

# Test connectivity from external
nc -zv bootstrap.accumulate.defidevs.io 16593
```

## Peer Address Format

Bootstrap peers use multiaddr format:

```
/dns/<hostname>/tcp/<port>/p2p/<peer-id>
```

Components:
- `/dns/`: DNS resolution protocol
- `<hostname>`: Domain name
- `/tcp/`: TCP transport
- `<port>`: Port number (usually 16593)
- `/p2p/`: P2P protocol
- `<peer-id>`: Base58 encoded peer ID

Example:
```
/dns/apollo-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWPs19932secARrxoRR5J8ZtBMt2vqwyHH1Q9p8thYP7cn
```

## Common Issues and Solutions

### Issue: Bootstrap Not Responding

**Symptoms**: New nodes can't connect to network

**Diagnosis**:
```bash
# From bootstrap server
docker ps  # Check if container is running
docker logs accumulated-bootstrap  # Check for errors

# From external
nc -zv bootstrap.accumulate.defidevs.io 16593
```

**Solutions**:
1. Restart container
2. Check AWS security groups
3. Verify DNS resolution
4. Check disk space

### Issue: Wrong Network Peers

**Symptoms**: Mainnet nodes receiving testnet peers

**Diagnosis**:
```bash
docker logs accumulated-bootstrap | grep "peer"
```

**Solution**:
Update container with correct `--network` and `--peer` flags

### Issue: Container Crashes

**Symptoms**: Container repeatedly restarts

**Diagnosis**:
```bash
docker logs accumulated-bootstrap
df -h  # Check disk space
free -h  # Check memory
```

**Solutions**:
1. Clear logs if disk full
2. Increase instance size if memory issue
3. Check for corrupted state files

## Network-Specific Configurations

### MainNet

```bash
docker run -d accumulated:latest bootstrap \
  --network="MainNet" \
  --peer="/dns/apollo-mainnet.accumulate.defidevs.io/tcp/16593/p2p/..." \
  --peer="/dns/yutu-mainnet.accumulate.defidevs.io/tcp/16593/p2p/..." \
  --peer="/dns/chandrayaan-mainnet.accumulate.defidevs.io/tcp/16593/p2p/..."
```

### TestNet

```bash
docker run -d accumulated:latest bootstrap \
  --network="TestNet" \
  --peer="/dns/testnet-node1.accumulate.defidevs.io/tcp/16593/p2p/..." \
  --peer="/dns/testnet-node2.accumulate.defidevs.io/tcp/16593/p2p/..."
```

### Local Development

```bash
docker run -d accumulated:latest bootstrap \
  --network="Local" \
  --listen="/ip4/127.0.0.1/tcp/16593" \
  --peer="/ip4/127.0.0.1/tcp/16594/p2p/..." \
  --peer="/ip4/127.0.0.1/tcp/16595/p2p/..."
```

## Security Considerations

1. **Access Control**
   - Use EC2 Instance Connect for temporary access
   - No permanent SSH keys on server
   - Security group restricts to port 16593 only

2. **Updates**
   - Regular security patches
   - Docker image updates
   - Monitor for vulnerabilities

3. **Backup**
   - No state to backup (stateless service)
   - Configuration documented in code
   - Can be quickly redeployed

## Integration with Nodes

Nodes connect to bootstrap server on startup:

```go
// In node initialization
func (n *Node) connectToBootstrap() error {
    for _, addr := range BootstrapServers {
        pi, err := peer.AddrInfoFromP2pAddr(addr)
        if err != nil {
            continue
        }
        
        ctx, cancel := context.WithTimeout(n.ctx, 30*time.Second)
        err = n.host.Connect(ctx, *pi)
        cancel()
        
        if err != nil {
            slog.Warn("Failed to connect to bootstrap", "addr", addr, "error", err)
        }
    }
    
    // Bootstrap DHT even if some connections failed
    return n.dht.Bootstrap(n.ctx)
}
```

## Scaling Considerations

For network growth:

1. **Multiple Bootstrap Servers**
   - Deploy in different regions
   - Use GeoDNS for routing
   - Load balance connections

2. **Peer List Management**
   - Implement dynamic peer discovery
   - Regular health checks
   - Automatic dead peer removal

3. **Resource Requirements**
   - Current: t3.medium (2 vCPU, 4GB RAM)
   - 100 nodes: t3.large (2 vCPU, 8GB RAM)
   - 1000 nodes: t3.xlarge (4 vCPU, 16GB RAM)

## Automation Scripts

### Health Check Script

```bash
#!/bin/bash
# check_bootstrap.sh

BOOTSTRAP_HOST="bootstrap.accumulate.defidevs.io"
BOOTSTRAP_PORT="16593"

# Check DNS
if ! host $BOOTSTRAP_HOST > /dev/null 2>&1; then
    echo "ERROR: DNS resolution failed"
    exit 1
fi

# Check port
if ! nc -zv $BOOTSTRAP_HOST $BOOTSTRAP_PORT 2>&1 | grep succeeded > /dev/null; then
    echo "ERROR: Port $BOOTSTRAP_PORT not accessible"
    exit 1
fi

echo "Bootstrap server healthy"
```

### Deployment Script

```bash
#!/bin/bash
# deploy_bootstrap.sh

INSTANCE_ID="i-0e053e32862689726"
REGION="us-east-2"
KEY_FILE="/tmp/bootstrap_key"

# Generate key
ssh-keygen -t rsa -b 2048 -f $KEY_FILE -N '' -q

# Send key
aws ec2-instance-connect send-ssh-public-key \
  --region $REGION \
  --instance-id $INSTANCE_ID \
  --instance-os-user ubuntu \
  --ssh-public-key file://${KEY_FILE}.pub

# Deploy
ssh -i $KEY_FILE ubuntu@54.211.10.186 << 'EOF'
docker stop accumulated-bootstrap
docker rm accumulated-bootstrap
docker pull accumulated:latest
docker run -d \
  --name accumulated-bootstrap \
  --restart unless-stopped \
  -p 16593:16593 \
  accumulated:latest bootstrap \
  --network="MainNet" \
  --peer="..." 
EOF

# Cleanup
rm -f ${KEY_FILE}*
```

## Related Documentation

- [P2P Architecture](./P2P-ARCHITECTURE.md)
- [Network Expansion Guide](./network-expansion/README.md)
- [AWS EC2 Instance Connect](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-connect.html)
- [Docker Documentation](https://docs.docker.com/)