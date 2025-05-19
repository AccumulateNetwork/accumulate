---
title: Accumulate Test Environments
description: Overview of the different test environments used in Accumulate development
tags: [accumulate, testing, environments, testnet, devnet]
created: 2025-05-17
version: 1.0
---

# Accumulate Test Environments

## 1. Introduction

This document describes the various test environments used in Accumulate development. Each environment serves a specific purpose in the testing lifecycle, from local development to production-like validation. Understanding these environments helps developers choose the right context for their testing needs.

## 2. Environment Overview

Accumulate uses a progressive testing approach with multiple environments:

| Environment | Purpose | Characteristics | When to Use |
|-------------|---------|-----------------|------------|
| Local Simulator | Rapid development and unit testing | Single machine, controlled | During development |
| DevNet | Integration testing | Small-scale network | Feature integration |
| TestNet | System testing, user acceptance | Production-like | Pre-release validation |
| MainNet Shadow | Production validation | Real data, no user impact | Final validation |

## 3. Local Development Environment

### 3.1 Simulator

The [Accumulate Simulator](./02_simulator_guide.md) provides a controlled environment for local testing:

- **Setup**: Run locally on a developer's machine
- **Scale**: Configurable (typically 1-3 BVNs with 1-3 validators each)
- **Persistence**: In-memory or local database
- **Purpose**: Rapid development, unit testing, debugging
- **Access**: Local API endpoints

### 3.2 Docker Development Environment

For more complex local testing:

- **Setup**: Docker Compose configuration
- **Scale**: Multiple containers simulating a small network
- **Persistence**: Docker volumes
- **Purpose**: Testing interactions between components
- **Access**: Exposed ports on localhost

```bash
# Start a Docker-based development environment
docker-compose -f docker/dev/docker-compose.yml up
```

## 4. DevNet Environment

### 4.1 Purpose and Use Cases

DevNet is a small-scale, controlled network environment:

- Testing new features before TestNet deployment
- Integration testing between components
- Performance testing of specific components
- Continuous integration testing

### 4.2 Setup and Configuration

DevNet is typically configured as:

- 2-3 BVNs with 3 validators each
- 1 DN with 3 validators
- Deployed in a cloud environment
- Regular resets (daily or weekly)
- Controlled access for developers

### 4.3 Accessing DevNet

```bash
# Configure CLI for DevNet
accumulate --url https://devnet.accumulatenetwork.io/v2 account list

# Submit a transaction to DevNet
curl -X POST https://devnet.accumulatenetwork.io/v2 \
  -d '{"jsonrpc":"2.0","id":1,"method":"submit","params":{"transaction":"..."}}'
```

### 4.4 Limitations

DevNet has some limitations to be aware of:

- Limited resources compared to TestNet/MainNet
- Periodic resets that clear state
- Potential instability during active development
- Not suitable for long-term testing

## 5. TestNet Environment

### 5.1 Purpose and Use Cases

TestNet is a production-like environment for system testing:

- End-to-end testing of features
- User acceptance testing
- Performance and scalability testing
- Long-running tests and observations
- Community testing and feedback

### 5.2 Setup and Configuration

TestNet is configured to closely mirror MainNet:

- Multiple BVNs with multiple validators
- Full DN configuration
- Persistent state (less frequent resets)
- Public access
- Monitoring and alerting

### 5.3 Accessing TestNet

```bash
# Configure CLI for TestNet
accumulate --url https://testnet.accumulatenetwork.io/v2 account list

# Get tokens from the TestNet faucet
curl -X POST https://testnet-faucet.accumulatenetwork.io/faucet \
  -d '{"url":"acc://myaccount.acme"}'
```

### 5.4 Best Practices

When using TestNet:

1. **Resource Consideration**: Be mindful of resource usage
2. **Data Persistence**: Don't rely on long-term data persistence
3. **Realistic Testing**: Test with realistic transaction volumes
4. **Monitoring**: Monitor your transactions and accounts
5. **Reporting**: Report issues through the appropriate channels

## 6. MainNet Shadow Environment

### 6.1 Purpose and Use Cases

MainNet Shadow is a specialized environment that processes MainNet transactions without affecting the actual network:

- Testing protocol upgrades against real-world data
- Validating performance optimizations
- Identifying potential issues before MainNet deployment
- Analyzing behavior with real transaction patterns

### 6.2 Setup and Configuration

The MainNet Shadow environment:

- Mirrors the MainNet configuration
- Processes copies of MainNet transactions
- Runs in parallel with MainNet
- Has no user-visible impact
- Is used primarily by core developers

### 6.3 Deployment Process

The process for using MainNet Shadow:

1. Deploy the new code to shadow nodes
2. Configure shadow nodes to follow MainNet
3. Monitor behavior and performance
4. Compare results with MainNet
5. Analyze any discrepancies

### 6.4 Limitations

MainNet Shadow has some limitations:

- Read-only (cannot affect MainNet)
- Limited to existing transaction patterns
- Requires significant resources
- Not suitable for feature testing

## 7. Environment Management

### 7.1 Environment Provisioning

Environments are provisioned using:

- Terraform for infrastructure
- Ansible for configuration
- Docker for containerization
- Kubernetes for orchestration

```bash
# Example of provisioning a test environment
terraform -chdir=deploy/terraform/testnet apply
```

### 7.2 Configuration Management

Configuration is managed through:

- Git repositories for version control
- Environment-specific configuration files
- Secret management for sensitive data
- Infrastructure as Code principles

### 7.3 Monitoring and Observability

All environments include:

- Metrics collection (Prometheus)
- Logging (Loki/Grafana)
- Alerting for critical issues
- Performance dashboards

## 8. Choosing the Right Environment

Guidelines for selecting the appropriate test environment:

| Testing Need | Recommended Environment |
|--------------|-------------------------|
| Rapid development and debugging | Local Simulator |
| Component integration | DevNet |
| End-to-end testing | TestNet |
| Performance validation | TestNet or MainNet Shadow |
| User acceptance testing | TestNet |
| Protocol upgrade validation | MainNet Shadow |

## 9. Environment Lifecycle

### 9.1 Environment Creation

New environments are created:

- On a schedule (for regular resets)
- For specific testing needs
- For major releases
- For specialized testing campaigns

### 9.2 Environment Maintenance

Ongoing maintenance includes:

- Security updates
- Performance tuning
- Resource scaling
- Data management

### 9.3 Environment Retirement

Environments are retired:

- After testing completion
- During regular reset cycles
- When no longer needed
- When replaced by newer environments

## 10. Future Environment Plans

Planned improvements to test environments:

1. **Automated Environment Provisioning**: Self-service environment creation
2. **Enhanced Simulation Capabilities**: More realistic network conditions
3. **Hybrid Testing**: Combining real and simulated components
4. **Chaos Testing**: Introducing controlled failures
5. **Geographic Distribution**: Testing with globally distributed nodes

## 11. Related Documentation

- [Testing Overview](./01_testing_overview.md)
- [Simulator Guide](./02_simulator_guide.md)
- [Automated Testing](./04_automated_testing.md)
- [Operations](../00_index.md)
- [Network](../../04_network/00_index.md)
