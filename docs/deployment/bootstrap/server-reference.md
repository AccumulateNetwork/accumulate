# Bootstrap Servers - AI-Optimized Documentation

## Quick Reference

```yaml
purpose: Accumulate Network Bootstrap Server Operations
scope: libp2p DHT peer discovery infrastructure
criticality: optional (network functions without it, but slower peer discovery)
deployment_type: single_infrastructure_node
code_changes_required: false
network_wide_deployment_required: false
```

## System Architecture

### Dual P2P System

Accumulate uses TWO separate P2P systems:

```yaml
p2p_systems:
  consensus_layer:
    name: "CometBFT P2P"
    purpose: "Block synchronization and consensus"
    protocol: "CometBFT/Tendermint"
    peer_id_format: "hex (64 characters)"
    example_peer_id: "ebb29bee942723271a39217bd0ed62f7827245de"
    query_endpoint: "/net_info"
    ports:
      dn: {p2p: 16591, rpc: 16592}
      bvn: {p2p: 16691, rpc: 16692}
    server: "apollo-mainnet.accumulate.defidevs.io"

  application_layer:
    name: "libp2p DHT"
    purpose: "Peer discovery via Kademlia DHT"
    protocol: "libp2p"
    peer_id_format: "base58 (starts with 12D3Koo or Qm)"
    example_peer_id: "12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx"
    query_method: "DHT routing queries"
    ports:
      dn: 16593
      bvn: 16693
    server: "bootstrap.accumulate.defidevs.io"
```

### Bootstrap Server Role

```yaml
bootstrap_server:
  type: "libp2p_dht_node"
  runs_full_accumulate_node: false
  runs_cometbft: false
  runs_consensus: false

  functionality:
    - "Kademlia DHT routing"
    - "Peer discovery acceleration"
    - "Initial DHT bootstrap point"

  not_functionality:
    - "Block validation"
    - "Transaction processing"
    - "State management"
    - "Consensus participation"

  failure_impact: "non_critical"
  failure_behavior: "peer_discovery_slower_but_functional"
```

## Current Deployment Status

```yaml
deployment:
  hostname: "bootstrap.accumulate.defidevs.io"
  ip_address: "3.138.61.111"
  peer_id: "12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx"

  port_status:
    16593:
      name: "DN libp2p"
      status: "OPEN"
      accessible: true
      issue: "none"
    16693:
      name: "BVN libp2p"
      status: "CLOSED"
      accessible: false
      issue: "not_listening"
    8081:
      name: "Prometheus metrics"
      status: "CLOSED"
      accessible: false
      issue: "not_accessible"

  issue_summary: "Bootstrap server only listening on port 16593, missing port 16693"
  fix_required: true
  fix_scope: "infrastructure_only"
  nodes_to_update: 1
```

## Network Configuration

### Hardcoded Bootstrap Peers (CORRECT)

```yaml
mainnet_bootstrap_peers:
  directory_network:
    multiaddr: "/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx"
    network: "Directory"
    port: 16593
    status: "working"

  bvn_cyclops:
    multiaddr: "/dns/bootstrap.accumulate.defidevs.io/tcp/16693/p2p/12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx"
    network: "Cyclops"
    port: 16693
    status: "not_working_server_not_listening"

hardcoded_locations:
  - "mcp/server/tools_accman_artifacts.go"
  - "accumulate-dual-data/accumulate.toml"
```

### CometBFT Persistent Peers (Separate System)

```yaml
cometbft_peers:
  cyclops_validator:
    hostname: "apollo-mainnet.accumulate.defidevs.io"
    ip: "3.138.61.111"
    node_id: "3029240e829e58e399bc7b6115bb6bc947cc24c7"
    ports:
      dn: {p2p: 16591, rpc: 16592}
      bvn: {p2p: 16691, rpc: 16692}
    role: "validator"
    purpose: "block_sync_and_consensus"
```

## Deployment Fix

### Problem Statement

```yaml
problem:
  description: "Bootstrap server only listening on port 16593 (DN), not 16693 (BVN)"
  impact: "BVN nodes cannot use bootstrap server for peer discovery"
  workaround: "BVN peer discovery works via gossipsub and DHT routing"
  fix_type: "infrastructure_configuration"
  fix_scope: "single_server"
```

### Solution

```yaml
solution:
  action: "Add port 16693 to bootstrap server listen addresses"
  deployment_scope: "ONE infrastructure node"
  code_changes: "NONE"
  network_deployment: "NOT REQUIRED"
  validator_updates: "NOT REQUIRED"
  follower_updates: "NOT REQUIRED"
```

### Deployment Command

```bash
# Docker deployment (recommended)
docker stop accumulate-bootstrap
docker rm accumulate-bootstrap

docker run -d \
  --name accumulate-bootstrap \
  --restart unless-stopped \
  -p 16593:16593 \
  -p 16693:16693 \
  -p 8081:8081 \
  -v /opt/accumulate/bootstrap-key.txt:/key.txt:ro \
  registry.gitlab.com/accumulatenetwork/accumulate:latest \
  accumulated-bootstrap \
  --key /key.txt \
  --listen /ip4/0.0.0.0/tcp/16593 \
  --listen /ip4/0.0.0.0/tcp/16693 \
  --external /dns/bootstrap.accumulate.defidevs.io/tcp/16593 \
  --external /dns/bootstrap.accumulate.defidevs.io/tcp/16693
```

### Systemd Service Alternative

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

### Direct Binary Alternative

```bash
# Stop existing process
pkill accumulated-bootstrap

# Start with both ports
nohup accumulated-bootstrap \
  --key /opt/accumulate/bootstrap-key.txt \
  --listen /ip4/0.0.0.0/tcp/16593 \
  --listen /ip4/0.0.0.0/tcp/16693 \
  --external /dns/bootstrap.accumulate.defidevs.io/tcp/16593 \
  --external /dns/bootstrap.accumulate.defidevs.io/tcp/16693 \
  > /var/log/bootstrap.log 2>&1 &
```

## Key Management

```yaml
bootstrap_key:
  location: "/opt/accumulate/bootstrap-key.txt"
  format: "acc://[hash]/ACME"
  peer_id_derived_from: "key"
  regeneration_policy: "NEVER"
  regeneration_impact: "breaks_all_existing_configurations"

  current_key:
    peer_id: "12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx"
    status: "stable_do_not_change"
```

## Verification

### Port Connectivity Tests

```yaml
verification_tests:
  port_16593:
    command: "nc -zv bootstrap.accumulate.defidevs.io 16593"
    expected_output: "Connection to bootstrap.accumulate.defidevs.io 16593 port [tcp/*] succeeded!"
    current_status: "PASS"

  port_16693:
    command: "nc -zv bootstrap.accumulate.defidevs.io 16693"
    expected_output: "Connection to bootstrap.accumulate.defidevs.io 16693 port [tcp/*] succeeded!"
    current_status: "FAIL"

  metrics:
    command: "curl http://bootstrap.accumulate.defidevs.io:8081/metrics"
    expected_output: "libp2p_* metrics"
    current_status: "FAIL"
```

### Health Check Script

```bash
#!/bin/bash
# /opt/accumulate/check-bootstrap.sh

check_port() {
  local port=$1
  nc -z localhost $port || return 1
}

# Check ports
check_port 16593 || exit 1
check_port 16693 || exit 1

# Check metrics endpoint
curl -sf http://localhost:8081/metrics > /dev/null || exit 1

echo "Bootstrap server healthy"
exit 0
```

## Firewall Configuration

```yaml
required_firewall_rules:
  port_16593:
    protocol: "tcp"
    direction: "inbound"
    purpose: "libp2p DN bootstrap"
    required: true

  port_16693:
    protocol: "tcp"
    direction: "inbound"
    purpose: "libp2p BVN bootstrap"
    required: true

  port_8081:
    protocol: "tcp"
    direction: "inbound"
    purpose: "Prometheus metrics"
    required: false

ufw_commands:
  - "sudo ufw allow 16593/tcp comment 'Accumulate Bootstrap DN'"
  - "sudo ufw allow 16693/tcp comment 'Accumulate Bootstrap BVN'"
  - "sudo ufw allow 8081/tcp comment 'Prometheus Metrics'"

iptables_commands:
  - "sudo iptables -A INPUT -p tcp --dport 16593 -j ACCEPT"
  - "sudo iptables -A INPUT -p tcp --dport 16693 -j ACCEPT"
  - "sudo iptables -A INPUT -p tcp --dport 8081 -j ACCEPT"
  - "sudo iptables-save > /etc/iptables/rules.v4"
```

## Monitoring

```yaml
prometheus_metrics:
  endpoint: "http://bootstrap.accumulate.defidevs.io:8081/metrics"

  key_metrics:
    libp2p_peers:
      description: "Number of connected peers"
      type: "gauge"

    libp2p_dht_peers:
      description: "DHT routing table size"
      type: "gauge"

    go_goroutines:
      description: "Number of goroutines (health indicator)"
      type: "gauge"

log_monitoring:
  systemd: "journalctl -u accumulate-bootstrap -f"
  docker: "docker logs -f accumulate-bootstrap"
```

## Troubleshooting

```yaml
common_issues:
  port_not_listening:
    check_commands:
      - "sudo lsof -i :16593"
      - "sudo lsof -i :16693"
      - "ps aux | grep accumulated-bootstrap"

  peer_id_mismatch:
    cause: "Key file changed"
    impact: "All hardcoded references must be updated"
    prevention: "Never regenerate key file"

  no_peers_connecting:
    expected_behavior: "Bootstrap server doesn't require peers to connect"
    purpose: "Helps others discover peers"
    checks:
      - "DNS resolution: dig bootstrap.accumulate.defidevs.io"
      - "External accessibility from public internet"
```

## Peer Discovery Mechanisms

```yaml
peer_discovery_methods:
  bootstrap_dht:
    priority: 1
    description: "Connect to bootstrap peer, query DHT"
    failure_impact: "info_level_log"
    fatal: false

  dht_routing:
    priority: 2
    description: "Use existing DHT connections to find more peers"
    failure_impact: "none"
    fatal: false

  gossipsub:
    priority: 3
    description: "Discover peers via gossip protocol"
    failure_impact: "none"
    fatal: false

  pubsub_announcements:
    priority: 4
    description: "Nodes announce themselves on pubsub topics"
    failure_impact: "none"
    fatal: false

conclusion: "Bootstrap server failure is NON-FATAL - network continues functioning"
```

## Code References

```yaml
source_code:
  bootstrap_server:
    file: "cmd/accumulated-bootstrap/main.go"
    lines: 76
    description: "Standalone libp2p DHT bootstrap node"

  peer_discovery:
    file: "pkg/api/v3/p2p/discovery.go"
    description: "Peer discovery implementation"

  peer_manager:
    file: "pkg/api/v3/p2p/peer_manager.go"
    description: "Peer connection management"

  mcp_tools:
    bootstrap_comparison: "mcp/server/bootstrap_client.go"
    binary_building: "mcp/server/tools_build.go"
    accman_artifacts: "mcp/server/tools_accman_artifacts.go"
```

## MCP Server Tools

```yaml
mcp_tools:
  accumulate_build_binary:
    purpose: "Build accumulated binary from source"
    inputs:
      repo_path: "Path to accumulate repository"
      ref: "Optional git branch/tag/commit"
      output_path: "Optional output path for binary"
    outputs:
      binary_path: "Path to compiled binary"
      git_ref: "Git reference used"
      commit_hash: "Commit SHA"
      file_size: "Binary size in bytes"
      version: "Accumulated version string"

  accumulate_compare_bootstrap_peers:
    purpose: "Compare live bootstrap peers with hardcoded values"
    inputs:
      network: "mainnet|testnet|devnet"
      partition: "dn|bvn"
    outputs:
      source: "bootstrap|hardcoded"
      bootstrap_peers: "List of peers from bootstrap query"
      hardcoded_peers: "List of hardcoded peers"
      peers_match: "Boolean indicating if they match"
      matching_count: "Number of matching peers"
      in_bootstrap_only: "Peers only in bootstrap"
      in_hardcoded_only: "Peers only in hardcoded"
```

## Deployment Checklist

```yaml
pre_deployment:
  - action: "Backup current key file"
    command: "cp /opt/accumulate/bootstrap-key.txt /opt/accumulate/bootstrap-key.txt.backup"

  - action: "Verify key file exists"
    command: "cat /opt/accumulate/bootstrap-key.txt"

  - action: "Check current peer ID"
    expected: "12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx"

deployment:
  - action: "Stop existing bootstrap server"
    docker: "docker stop accumulate-bootstrap && docker rm accumulate-bootstrap"
    systemd: "sudo systemctl stop accumulate-bootstrap"
    binary: "pkill accumulated-bootstrap"

  - action: "Update configuration to include both listen addresses"
    ports: [16593, 16693]

  - action: "Update firewall rules if needed"
    command: "sudo ufw allow 16693/tcp"

  - action: "Start bootstrap server with new configuration"
    verify: "Check process is running"

post_deployment:
  - action: "Verify port 16593 listening"
    command: "nc -zv bootstrap.accumulate.defidevs.io 16593"
    expected: "succeeded"

  - action: "Verify port 16693 listening"
    command: "nc -zv bootstrap.accumulate.defidevs.io 16693"
    expected: "succeeded"

  - action: "Check logs for errors"
    docker: "docker logs accumulate-bootstrap"
    systemd: "journalctl -u accumulate-bootstrap -n 50"

  - action: "Test from follower node"
    description: "Deploy a follower and verify it discovers peers"
```

## Update Procedures

```yaml
updating_bootstrap_server:
  key_preservation: "CRITICAL"
  key_file_location: "/opt/accumulate/bootstrap-key.txt"

  docker_update:
    - "docker pull accumulated-bootstrap:latest"
    - "docker stop accumulate-bootstrap"
    - "docker rm accumulate-bootstrap"
    - "docker run -d [with preserved key volume]"

  systemd_update:
    - "sudo systemctl stop accumulate-bootstrap"
    - "sudo cp /path/to/new/accumulated-bootstrap /usr/local/bin/"
    - "sudo systemctl start accumulate-bootstrap"

  verification_after_update:
    - "Verify peer ID unchanged"
    - "Verify both ports listening"
    - "Check logs for errors"
```

## Important Distinctions

```yaml
do_not_confuse:
  bootstrap_server_vs_validator:
    bootstrap_server:
      role: "peer_discovery_infrastructure"
      runs: "libp2p_dht_only"
      participates_in_consensus: false
      updates_required: "infrastructure_only"

    validator:
      role: "consensus_and_state"
      runs: "full_accumulate_node"
      participates_in_consensus: true
      updates_required: "network_wide_coordination"

  libp2p_vs_cometbft:
    libp2p:
      layer: "application"
      purpose: "peer_discovery"
      ports: [16593, 16693]
      peer_id_format: "base58"

    cometbft:
      layer: "consensus"
      purpose: "block_sync_and_consensus"
      ports: [16591, 16691]
      peer_id_format: "hex"
```

## References

```yaml
documentation:
  - "cmd/accumulated-bootstrap/QUICK_FIX.md"
  - "cmd/accumulated-bootstrap/DEPLOYMENT.md"
  - "mcp/BOOTSTRAP_ARCHITECTURE.md"
  - "MAINNET_TOPOLOGY_2025-11-17.md"
  - "follower_deployment_session_2025-11-16.md"

external_docs:
  - url: "https://docs.libp2p.io/"
    topic: "libp2p protocol"
  - url: "https://docs.libp2p.io/concepts/discovery-routing/kaddht/"
    topic: "Kademlia DHT"
  - url: "https://docs.cometbft.com/"
    topic: "CometBFT consensus"
```

## Summary for AI Systems

```yaml
key_facts:
  - "Bootstrap server is OPTIONAL infrastructure"
  - "Bootstrap server does NOT run Accumulate consensus"
  - "Bootstrap server runs ONLY libp2p DHT"
  - "Updating bootstrap server requires NO code changes"
  - "Updating bootstrap server affects ONE node only"
  - "Current issue: port 16693 not listening"
  - "Fix: Add --listen flag for port 16693"
  - "Network functions WITHOUT bootstrap server (slower discovery)"
  - "Hardcoded bootstrap peers are CORRECT"
  - "libp2p and CometBFT are SEPARATE P2P systems"

action_required:
  description: "SSH to bootstrap.accumulate.defidevs.io and restart with both listen ports"
  scope: "infrastructure_only"
  risk: "low"
  downtime: "minimal"
  rollback: "simple (restore previous config)"
```
