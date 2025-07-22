# Accumulate Network Explorer Troubleshooting Summary

## Main Issue
The team was trying to get their blockchain explorer operational during what appears to be a network shutdown/transition period.

## Infrastructure Setup
- **API Gateway**: `api-gateway.accumulate.defidevs.io` (IP: 3.86.82.141) deployed via accman in same EC2 region as nodes
- **Node Management**: Using "accman" tool for SSL and Docker container management
- **Architecture**: Accman proxies HTTPS (443 → 16595/16695) with Let's Encrypt SSL
- **Target**: Get `mainnet.accumulatenetwork.io` working to enable the explorer

## Network Discovery Issues
- **Active Nodes Found**: 4 Cyclops nodes + 1 Chandrayaan node (Sphereon's node at 85.215.104.235)
- **Unexpected Survival**: Sphereon's node was still running despite being told they could shut down "when we shut down"
- **API v3 Problems**: Described as "fucked as per usual" due to nodes not rebooting when requested
- **Peer Connectivity**: Router failing to dial peer `12D3KooWDqFDwjHEog1bNbxai2dKSaR1aFvq2LAZ2jivSohgoSc7` at 54.234.31.209

## Technical Problems

### 1. Configuration Conflicts
- `--config` flag no longer supported in current `accumulated-http` version
- Had to remove `--config=/data/accumulate.toml` from ExtraArguments (used for Grafana telemetry)
- ExtraArguments can only be configured by manually editing `/var/lib/docker/volumes/accumulate_manager/_data/config.yaml`

### 2. SSL Certificate Issues
- Multiple certificate problems across `api-gateway.accumulate.defidevs.io` and `mainnet.accumulatenetwork.io`
- TLS handshake failures preventing HTTPS connections

### 3. Legacy Cache Problems
- Had to clean `peerdb.json` in `/var/lib/docker/volumes/acc_mainnet_http/_data/`
- Network discovery failing due to stale peer information

## Key Commands Used

### Docker Management
```bash
# View logs
docker logs acc_mainnet_http --tail 100

# Stop accman backend
docker stop accumulate_manager

# Access docker volumes (requires root)
sudo -i
# or
sudo su -
```

### File Locations
- **Accman config**: `/var/lib/docker/volumes/accumulate_manager/_data/config.yaml`
- **Peer database**: `/var/lib/docker/volumes/acc_mainnet_http/_data/peerdb.json`
- **SSH keys**: `/home/ec2-user/.ssh/authorized_keys`

### Testing Connectivity
```bash
# Test API endpoint
curl -v https://mainnet.accumulatenetwork.io/v3 -X POST -s --data-raw '{"jsonrpc": "2.0", "id": 1, "method": "query", "params": {"scope": "dn.acme" }}'

# Direct node access (bypassing SSL)
curl -v http://apollo-mainnet.accumulate.defidevs.io:16595/v3 -X POST -s --data-raw '{"jsonrpc": "2.0", "id": 1, "method": "query", "params": {"scope": "dn.acme" }}'
```

## Resolution Steps and History

1. **Cleaned peer database** to force network rediscovery
2. **Removed incompatible configuration** parameters from accman
3. **Redirected DNS**: `mainnet.accumulatenetwork.io` → `apollo-mainnet` node
4. **Updated accman configuration**: Added mainnet domain to apollo's domain list
5. **Bypassed API gateway**: Used direct node connection instead

## Final Outcome
The explorer was restored by redirecting the mainnet domain to a working node (apollo-mainnet) and properly configuring the domain routing in accman, bypassing the problematic API gateway entirely.

## Network Context
This troubleshooting session occurred during a network shutdown/transition period, with:
- Legacy infrastructure causing connectivity issues
- Third-party nodes (Sphereon) not following shutdown procedures
- Version mismatches between configuration and current software
- Multiple SSL certificate issues across the infrastructure

## Lessons Learned
- Accman's restart functionality needs improvement
- SSL certificate management across multiple domains is problematic
- Direct node access is more reliable than API gateway during transitions
- Infrastructure person hiring was justified given the complexity