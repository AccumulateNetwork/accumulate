# Accumulate Network Expansion Guide

## Current MainNet Status
As of August 2024, the Accumulate MainNet consists of:
- **3 Active Nodes**: apollo, yutu, chandrayaan
- **1 Validator**: (limited consensus capability)
- **1 Bootstrap Server**: bootstrap.accumulate.defidevs.io

## Network Architecture

### Bootstrap Server
The bootstrap server (`bootstrap.accumulate.defidevs.io`) is responsible for:
- Helping new nodes discover existing peers
- Maintaining a list of active nodes
- Facilitating P2P connections

**Important**: The bootstrap server should only advertise nodes that are actually active and reachable.

### Current Active Nodes

| Node Name | DNS | IP Address | Peer ID |
|-----------|-----|------------|---------|
| apollo-mainnet | apollo-mainnet.accumulate.defidevs.io | 23.22.212.106 | 12D3KooWPs19932secARrxoRR5J8ZtBMt2vqwyHH1Q9p8thYP7cn |
| yutu-mainnet | yutu-mainnet.accumulate.defidevs.io | 54.234.31.209 | 12D3KooWJqp6jpagL2cJwhBX3aWJvqCUf46ceYxpqQXKFrgPGRCT |
| chandrayaan-mainnet | chandrayaan-mainnet.accumulate.defidevs.io | 54.85.31.44 | [Peer ID needs to be determined] |

## Network Expansion Process

### Phase 1: Adding a New Node

#### 1.1 Provision Infrastructure
```bash
# Launch EC2 instance (or equivalent)
# Recommended: t3.large or better
# OS: Ubuntu 22.04 LTS
# Security Groups: Open ports 16591-16595, 16691-16695
```

#### 1.2 Install Accumulate
```bash
# SSH into the new node
ssh ubuntu@<new-node-ip>

# Install Docker (if using containerized deployment)
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh

# Or install accumulated binary directly
wget https://github.com/AccumulateNetwork/accumulate/releases/latest/download/accumulated
chmod +x accumulated
sudo mv accumulated /usr/local/bin/
```

#### 1.3 Configure the Node
Create `/etc/accumulate/accumulate.toml`:
```toml
network = "MainNet"

[[configurations]]
  type = "coreValidator"  # or "follower" for non-validator nodes
  bvn = "YourBVN"  # e.g., "Apollo", "Yutu", etc.
  listen = "/ip4/0.0.0.0/tcp/16591"
  
  # Bootstrap peers - connect to existing nodes
  dn-bootstrap-peers = [
    "/dns/apollo-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWPs19932secARrxoRR5J8ZtBMt2vqwyHH1Q9p8thYP7cn",
    "/dns/yutu-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWJqp6jpagL2cJwhBX3aWJvqCUf46ceYxpqQXKFrgPGRCT",
    "/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg"
  ]
  
  # Genesis files
  dn-genesis = "directory-genesis.snap"
  bvn-genesis = "bvn-genesis.snap"
  
  # Validator key (if validator node)
  [configurations.validator-key]
    type = "raw"
    address = "YOUR_VALIDATOR_KEY"

[p2p]
  # External address that other nodes will use to connect
  external-address = "/dns/your-node.accumulate.defidevs.io/tcp/16593"
  
  [p2p.key]
    type = "raw"
    address = "YOUR_P2P_KEY"

[logging]
  level = "info"
  format = "json"
```

#### 1.4 Start the Node
```bash
# Using systemd
sudo systemctl start accumulated
sudo systemctl enable accumulated

# Or using Docker
docker run -d \
  --name accumulate-node \
  --restart unless-stopped \
  -v /etc/accumulate:/etc/accumulate \
  -p 16591-16595:16591-16595 \
  -p 16691-16695:16691-16695 \
  registry.gitlab.com/accumulatenetwork/accumulate:latest \
  run --config /etc/accumulate/accumulate.toml
```

#### 1.5 Verify Node is Running
```bash
# Check node status
curl -X POST http://localhost:16595/v3 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"node-info","params":{},"id":1}'

# Check P2P connectivity
./debug test-p2p mainnet
```

### Phase 2: Update Bootstrap Server

Once the new node is running and stable, update the bootstrap server to include it:

#### 2.1 Get the New Node's Peer ID
```bash
# On the new node
accumulated key export --key-type p2p
# This will show the peer ID
```

#### 2.2 Update Bootstrap Configuration
SSH into the bootstrap server and update the startup script:

```bash
ssh ubuntu@bootstrap.accumulate.defidevs.io

# Edit the bootstrap script to add the new peer
cat > start-mainnet-bootstrap.sh << 'EOF'
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
    --peer /dns/apollo-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWPs19932secARrxoRR5J8ZtBMt2vqwyHH1Q9p8thYP7cn \
    --peer /dns/yutu-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWJqp6jpagL2cJwhBX3aWJvqCUf46ceYxpqQXKFrgPGRCT \
    --peer /dns/chandrayaan-mainnet.accumulate.defidevs.io/tcp/16593/p2p/[CHANDRAYAAN_PEER_ID] \
    --peer /dns/[NEW-NODE].accumulate.defidevs.io/tcp/16593/p2p/[NEW_PEER_ID] \
    --listen /ip4/0.0.0.0/tcp/16593 \
    --listen /ip4/0.0.0.0/udp/16593/quic \
    --external /dns/bootstrap.accumulate.defidevs.io/tcp/16593 \
    --external /dns/bootstrap.accumulate.defidevs.io/udp/16593/quic
EOF

# Restart bootstrap
sudo ./start-mainnet-bootstrap.sh
```

### Phase 3: DNS Configuration

#### 3.1 Add DNS Entry
Create a DNS A record for the new node:
```
new-node-mainnet.accumulate.defidevs.io -> [NEW_NODE_IP]
```

#### 3.2 Update Route53 (if using AWS)
```bash
aws route53 change-resource-record-sets \
  --hosted-zone-id [ZONE_ID] \
  --change-batch '{
    "Changes": [{
      "Action": "CREATE",
      "ResourceRecordSet": {
        "Name": "new-node-mainnet.accumulate.defidevs.io",
        "Type": "A",
        "TTL": 300,
        "ResourceRecords": [{"Value": "[NEW_NODE_IP]"}]
      }
    }]
  }'
```

### Phase 4: Monitor and Verify

#### 4.1 Check Network Connectivity
```bash
# From any node, check if the new node is visible
./debug network scan mainnet

# Check network status
./debug network status mainnet
```

#### 4.2 Monitor Logs
```bash
# On the new node
journalctl -u accumulated -f

# Or for Docker
docker logs -f accumulate-node
```

#### 4.3 Verify Consensus Participation
For validator nodes, verify they're participating in consensus:
```bash
# Check validator status
curl -X POST http://[NODE_IP]:16592/status
```

## Scaling Considerations

### Minimum Viable Network
- **3 nodes**: Minimum for basic operation
- **4+ validators**: Required for Byzantine fault tolerance
- **7+ validators**: Recommended for production

### Geographic Distribution
- Distribute nodes across multiple:
  - AWS regions
  - Cloud providers
  - Geographic locations
- This improves:
  - Network resilience
  - Latency for global users
  - Disaster recovery

### Resource Requirements
Per node recommendations:
- **CPU**: 4+ cores
- **RAM**: 8GB minimum, 16GB recommended
- **Storage**: 100GB SSD minimum
- **Network**: 100Mbps dedicated bandwidth

## Troubleshooting

### Common Issues

1. **Node can't connect to peers**
   - Check firewall rules
   - Verify bootstrap configuration
   - Ensure DNS is resolving correctly

2. **Node not participating in consensus**
   - Verify validator key is correct
   - Check if node is fully synced
   - Ensure sufficient stake

3. **High resource usage**
   - Check for network attacks
   - Verify logging isn't too verbose
   - Consider increasing resources

### Health Checks

Regular checks to perform:
```bash
# Check node height is increasing
watch -n 5 'curl -s localhost:16595/v3 -X POST -d "{\"jsonrpc\":\"2.0\",\"method\":\"network-status\",\"params\":{},\"id\":1}" | jq .result.directoryHeight'

# Check peer connections
./debug test-p2p mainnet

# Check resource usage
htop
df -h
```

## Maintenance

### Bootstrap Server Updates
The bootstrap server should be updated whenever:
- A new node is added to the network
- A node is permanently removed
- Node peer IDs change

### Regular Tasks
- Monitor node health daily
- Update node software monthly
- Review logs for errors weekly
- Test disaster recovery quarterly

## Contact

For assistance with network expansion:
- Technical issues: [Create issue in GitLab]
- Infrastructure access: [Contact DevOps team]
- Validator onboarding: [Contact Network team]