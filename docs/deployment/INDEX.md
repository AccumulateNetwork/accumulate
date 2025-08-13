# Deployment Documentation Index

[← Back to Main Index](../INDEX.md)

## Overview
Production deployment guides, upgrade procedures, and network management documentation for Accumulate.

## Quick Start
- [Simplified Upgrade Plan](SIMPLIFIED_UPGRADE_PLAN.md) - Streamlined upgrade process
- [TestNet Upgrade Guide](TESTNET_UPGRADE_WITH_ACCMAN.md) - TestNet upgrade procedures

## Upgrade Procedures

### ACCMAN Tool
- [ACCMAN Upgrade Commands](ACCMAN_UPGRADE_COMMANDS.md) - Complete ACCMAN command reference
- Related Scripts: [`scripts/testnet/`](../../scripts/testnet/)
  - [`TESTNET_UPGRADE_SCRIPT.sh`](../../scripts/testnet/TESTNET_UPGRADE_SCRIPT.sh)
  - [`TESTNET_QUICK_UPGRADE.sh`](../../scripts/testnet/TESTNET_QUICK_UPGRADE.sh)
  - [`rollback_testnet.sh`](../../scripts/testnet/rollback_testnet.sh)

### Network-Specific Deployment

#### MainNet (Cyclops)
- [Cyclops Documentation](../cyclops/INDEX.md) - MainNet deployment
- [Cyclops Deployment Guide](../cyclops/cyclops-deployment-guide.md)
- [Cyclops Launch](../cyclops/cyclops-launch.md)

#### TestNet (Kermit)
- [Kermit Documentation](../kermit/INDEX.md) - TestNet deployment
- [Kermit Services Fix](../kermit/kermit-services-fix.md)
- [Port Configuration](../kermit/port-configuration.md)

## Deployment Workflows

### 1. Pre-Deployment
- Backup current state
- Verify network health
- Check version compatibility
- Review [Network Boot Procedures](../network/network-boot-procedures.md)

### 2. Deployment Process
- Stop nodes in sequence
- Update binaries
- Apply configuration changes
- Restart nodes
- Verify consensus

### 3. Post-Deployment
- Monitor network health
- Verify transaction processing
- Check cross-partition messaging
- Review metrics

## Configuration Management

### Network Configuration
- [Network JSON Structure](../network/network-json-structure.md)
- [Network Initialization](../network/network-initialization.md)
- Configuration Directory: [`config/`](../../config/)

### Node Configuration
- TOML configuration files
- Environment variables
- Command-line flags
- See [Cyclops TOML Configuration](../cyclops/cyclops-toml-configuration.md)

## Key Management

### Production Keys
- [Cyclops Key Management Guide](../cyclops/cyclops-key-management-guide.md)
- [P2P Key Generation](../technical/p2p-key-generation.md)

### Security Considerations
- Never commit private keys
- Use hardware security modules (HSM) when available
- Implement key rotation policies
- Monitor key usage

## Monitoring & Maintenance

### Health Checks
```bash
# Check node status
curl http://node-ip:26657/status

# Check network status
curl http://node-ip:26660/v2/status

# Check metrics
curl http://node-ip:26660/metrics
```

### Log Management
- Log rotation configuration
- Centralized logging setup
- Alert configuration

### Backup Procedures
- State backup strategies
- Snapshot management
- Recovery procedures

## Automation

### CI/CD Integration
- GitLab CI configuration: [`.gitlab-ci.yml`](../../.gitlab-ci.yml)
- Automated deployment scripts
- Health check automation

### Infrastructure as Code
- Terraform modules (if applicable)
- Ansible playbooks (if applicable)
- Docker configurations

## Network Ports

### Standard Ports
See [Accumulate Port Reference](../network/accumulate-port-reference.md)

| Service | Port | Protocol |
|---------|------|----------|
| P2P | 26656 | TCP |
| RPC | 26657 | TCP |
| API | 26660 | HTTP |
| Metrics | 26661 | HTTP |

## Troubleshooting

### Common Issues
1. **Consensus Issues**
   - Check peer connectivity
   - Verify time synchronization
   - Review validator set

2. **Performance Issues**
   - Check resource usage
   - Review [Performance Guide](../technical/performance-guide.md)
   - Optimize configuration

3. **Network Partitions**
   - Check cross-partition messaging
   - Review [Gap Recovery](../design/crosschain/GAP_RECOVERY_ACTUAL.md)
   - Monitor CCC status

### Debug Tools
- [Debug Tool Documentation](../tools/debug-tool.md)
- [Debug App Reference](../tools/debug-app-reference.md)

## Disaster Recovery

### Backup Strategies
- Regular snapshots
- Off-site backups
- Recovery testing

### Recovery Procedures
1. Stop affected nodes
2. Restore from backup
3. Resync with network
4. Verify integrity

## Security

### Security Checklist
- [ ] Firewall configured
- [ ] SSH hardened
- [ ] Keys secured
- [ ] Monitoring active
- [ ] Backups verified
- [ ] Updates applied

### Security Updates
- Monitor security advisories
- Apply patches promptly
- Test in staging first

## Scripts and Tools

### Deployment Scripts
Located in [`scripts/`](../../scripts/):
- [`CREATE_NEW_VERSION.sh`](../../scripts/CREATE_NEW_VERSION.sh) - Version creation
- [`analyze_tests.sh`](../../scripts/analyze_tests.sh) - Test analysis

### TestNet Scripts
Located in [`scripts/testnet/`](../../scripts/testnet/):
- Upgrade scripts
- Rollback procedures
- Health checks

## Related Documentation

- [Network Documentation](../network/INDEX.md) - Network architecture
- [Testing Documentation](../testing/INDEX.md) - Pre-deployment testing
- [Technical Documentation](../technical/INDEX.md) - Technical specifications
- [Design Documentation](../design/INDEX.md) - System design