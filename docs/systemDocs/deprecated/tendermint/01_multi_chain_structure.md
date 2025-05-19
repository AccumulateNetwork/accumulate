---
title: Tendermint in Accumulate's Multi-Chain Structure
description: Detailed explanation of how multiple Tendermint instances operate in Accumulate's partitioned architecture
tags: [tendermint, multi-chain, consensus, partitions]
created: 2025-05-16
version: 1.0
updated: 2025-05-17
---

> **DEPRECATED**: This document has been moved to the new documentation structure. Please refer to the [new location](../new_structure/04_network/05_consensus/02_multi_chain.md) for the most up-to-date version.

# Tendermint in Accumulate's Multi-Chain Structure

## Introduction

Accumulate employs a unique multi-chain architecture where each partition (Directory Network and Block Validator Networks) runs its own independent Tendermint instance. This document provides a detailed explanation of how these multiple Tendermint instances are configured, coordinated, and managed within Accumulate's partitioned architecture.

## Partition Types and Tendermint Instances

Accumulate's network consists of three types of partitions, each running its own Tendermint instance:

### Directory Network (DN)

The Directory Network is the central coordination layer of Accumulate:

- **Purpose**: Manages global state, routes transactions, and coordinates between BVNs
- **Tendermint Configuration**: Uses base port configuration (PortOffsetDirectory = 0)
- **Consensus Participants**: All validator nodes participate in DN consensus
- **State Management**: Maintains the global routing table and ADI registry

### Block Validator Networks (BVNs)

BVNs are responsible for processing transactions for a subset of accounts:

- **Purpose**: Process transactions, maintain account states, and execute smart contracts
- **Tendermint Configuration**: Uses offset ports (PortOffsetBlockValidator = 100)
- **Consensus Participants**: Subset of validator nodes assigned to each BVN
- **State Management**: Maintains account states for its partition

### Block Summary Network (BSN)

The Block Summary Network (when enabled) provides aggregated data:

- **Purpose**: Aggregates and summarizes data from all partitions
- **Tendermint Configuration**: Uses offset ports (PortOffsetBlockSummary = 200)
- **Consensus Participants**: Typically all validator nodes
- **State Management**: Maintains summary data and cross-references

## Port Configuration System

Accumulate uses a port offset system to manage multiple Tendermint instances on the same node:

```go
const PortOffsetDirectory = 0
const PortOffsetBlockValidator = 100
const PortOffsetBlockSummary = 200

const PortOffsetTendermintP2P = 0
const PortOffsetTendermintRpc = 10
```

Each Tendermint instance uses several ports:
- P2P communication (base + PortOffsetTendermintP2P)
- RPC interface (base + PortOffsetTendermintRpc)
- Metrics and other services

When a node participates in multiple partitions, each Tendermint instance uses a different port range based on these offsets.

## Node Types and Tendermint Roles

Accumulate supports different node types, each with different Tendermint configurations:

### Validator Nodes

- Run Tendermint in validator mode
- Participate in consensus
- Sign blocks
- Maintain private validator keys
- Run multiple Tendermint instances (one per partition they validate)

### Follower Nodes

- Run Tendermint in non-validator mode
- Do not participate in consensus
- Sync blocks from validators
- May run multiple Tendermint instances to follow multiple partitions

## Cross-Chain Communication

Tendermint instances do not directly communicate across partitions. Instead, Accumulate implements cross-chain communication through:

### Anchoring

- Each partition's Tendermint instance produces blocks independently
- Block data is anchored between partitions via synthetic transactions
- The Directory Network receives anchors from all BVNs
- The Directory Network sends anchors back to all BVNs

### Transaction Routing

- Transactions are routed to the appropriate partition based on the account URL
- The dispatcher component determines which Tendermint instance should receive each transaction
- Cross-partition transactions generate synthetic follow-up transactions

## Configuration Management

Each Tendermint instance requires its own configuration:

### File Structure

```
/config/
  /directory/
    tendermint.toml
    accumulate.toml
  /bvn0/
    tendermint.toml
    accumulate.toml
  /bvn1/
    tendermint.toml
    accumulate.toml
```

### Configuration Parameters

Key Tendermint configuration parameters include:

- **Moniker**: Unique name for the node within the network
- **Seeds**: Bootstrap nodes for P2P discovery
- **Persistent Peers**: Reliable nodes to maintain connections with
- **Consensus Parameters**: Block time, size limits, etc.
- **P2P Parameters**: Connection limits, timeouts, etc.

## Genesis Configuration

Each Tendermint instance requires its own genesis file:

- **Validator Set**: Initial set of validators and their voting power
- **Chain ID**: Unique identifier for the partition (e.g., "MainNet.Directory", "MainNet.BVN0")
- **Genesis Time**: Official start time of the chain
- **Consensus Parameters**: Initial consensus parameters

## State Synchronization

Tendermint instances can be synchronized in different ways:

### Block Sync

- Traditional block-by-block synchronization
- Used for validators and when state sync is not available

### State Sync

- Fast sync using a trusted recent state
- Requires snapshot nodes
- Configured with trusted height and hash values

## Monitoring and Metrics

Each Tendermint instance exposes metrics:

- **Consensus Metrics**: Block production rate, voting power distribution
- **P2P Metrics**: Peer count, bandwidth usage
- **Mempool Metrics**: Transaction queue size, processing time

These metrics are prefixed with the partition identifier for clarity.

## Security Considerations

Running multiple Tendermint instances introduces security considerations:

- **Resource Isolation**: Ensuring one instance cannot starve others
- **Key Management**: Secure handling of multiple validator keys
- **Network Segmentation**: Proper firewall rules for different instances
- **Monitoring**: Detecting issues across all instances

## References

- [Accumulate Overview](../01_accumulate_overview.md)
- [Tendermint Integration Overview](../05_tendermint_integration.md)
- [ABCI Application](02_abci_application.md)
- [Transaction Processing](03_transaction_processing.md)
- [Network Communication](04_network_communication.md)
