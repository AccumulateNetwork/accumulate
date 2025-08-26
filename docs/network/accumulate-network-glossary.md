<!-- AI_DOCUMENT_TYPE: glossary -->
<!-- AI_PRIMARY_TOPICS: terminology, definitions, network_concepts -->
<!-- AI_COMPLEXITY: low -->
<!-- AI_SPLIT_RECOMMENDED: no -->
<!-- AI_LAST_UPDATED: 2025-01-05 -->

# Accumulate Network Glossary

> **Document Type**: Terminology reference  
> **Scope**: Network concepts, commands, and technical terms  
> **Target Audience**: All users of Accumulate documentation

## Core Network Concepts
<!-- AI_TAG: network_concepts -->

### ADI (Accumulate Digital Identifier)
Digital identities distributed across the Accumulate network partitions. ADIs are the fundamental building blocks of the Accumulate protocol.

### BVN (Block Validator Network)
Individual networks that validate transactions and maintain blockchain state. The MainNet has three BVNs: Apollo, Chandrayaan, and Yutu.

### Directory Network (DN)
The central coordination network that manages routing, consensus, and inter-partition communication across all BVNs.

### Partition
A logical division of the network. Each partition (Directory + BVNs) operates as an independent Tendermint network while coordinating with others.

### Tendermint
The consensus protocol used by both Directory Network and Block Validator Networks for Byzantine fault-tolerant consensus.

## Node Daemon Components
<!-- AI_TAG: daemon_components -->

### `accumulated`
The **Accumulate Node Daemon** - the core server binary that runs Accumulate network nodes. This is NOT the CLI wallet application.

### Node Types
- **Validator Node**: Participates in consensus and block validation
- **Follower Node**: Syncs with network but does not participate in consensus
- **Dual Node**: Runs both Directory Network and Block Validator Network components

### Working Directory
The directory containing node configuration files, databases, and genesis snapshots. Specified with `--work-dir` flag.

## Network Configuration
<!-- AI_TAG: network_config -->

### Genesis Snapshot
Binary file (`.snap`) containing the initial state of a network partition, including all accounts, transactions, and consensus parameters.

### Network Configuration JSON
JSON file defining network topology, partitions, validators, routing rules, and global parameters.

### accumulate.toml
Primary configuration file for individual nodes, containing network settings, storage configuration, P2P settings, and consensus parameters.

## Command Categories
<!-- AI_TAG: command_categories -->

### Initialization Commands
Commands that set up network or node configurations:
- `init network` - Complete network initialization
- `init genesis` - Genesis-only generation
- `init node` - Single node setup
- `init dual` - Dual node configuration

### Runtime Commands
Commands that operate running networks:
- `run` - Start node daemon
- `run devnet` - Development network (not for production)

### Utility Commands
Commands for data processing:
- `init prepare-genesis` - Consolidate multiple snapshots

## Network Ports
<!-- AI_TAG: network_ports -->

### Directory Network Ports
- **16591**: P2P communication
- **16592**: RPC interface
- **16595**: JSON-RPC interface

### Block Validator Network Ports
- **16691**: P2P communication
- **16692**: RPC interface
- **16695**: JSON-RPC interface

### Management Ports
- **16666**: AccMan (Accumulate Manager)
- **6695**: SSL Client access

## File Types and Extensions
<!-- AI_TAG: file_types -->

### Configuration Files
- **`.toml`**: Configuration files (accumulate.toml)
- **`.json`**: Network configuration and genesis files

### Snapshot Files
- **`.snap`**: Binary genesis snapshots
- **`.json`**: Human-readable genesis files (converted from .snap)

### Key Files
- **`priv_validator_key.json`**: Validator private key
- **`node_key.json`**: Node identity key

## Deployment Terminology
<!-- AI_TAG: deployment_terms -->

### Cyclops Network
A specific network configuration used for testing and development, with simplified topology compared to MainNet.

### Partition Extraction
Process of extracting specific partition data from a complete network snapshot into separate partition-specific snapshots.

### Network Initialization
Process of creating genesis files, node configurations, and network topology from a network configuration file.

### Dual Node Setup
Configuration where a single physical node runs both Directory Network and Block Validator Network components.

## Error Categories
<!-- AI_TAG: error_categories -->

### Command Syntax Errors
Errors related to incorrect command usage, missing arguments, or invalid flag combinations.

### Configuration Errors
Errors in configuration files, missing required files, or invalid network parameters.

### Network Connectivity Errors
Issues with peer connections, port conflicts, or network communication failures.

### Resource Errors
Problems with disk space, memory allocation, or file permissions.

## MainNet Specific Terms
<!-- AI_TAG: mainnet_terms -->

### Network Partitions
- **Directory**: Central coordination partition
- **Apollo**: Block validator partition
- **Chandrayaan**: Block validator partition  
- **Yutu**: Block validator partition

### Special Accounts
Accounts with specific routing rules:
- `acc://staking.acme` - Staking operations
- `acc://ACME` - Native token
- `acc://dn.acme` - Directory Network identity

### Validator Operators
Organizations running network validators (e.g., defidevs.acme, kompendium.acme, etc.)

## Development vs Production
<!-- AI_TAG: dev_vs_prod -->

### DevNet
Development network configuration with simplified settings. **Not suitable for production use** due to:
- Simplified consensus mechanisms
- Reduced security validations
- Development-only network topology

### Production Networks
Networks suitable for live operation:
- **MainNet**: Primary production network
- **TestNet**: Testing network with production-like settings

## Automation Tools
<!-- AI_TAG: automation_tools -->

### AccMan (Accumulate Manager)
Tool for automated node management, firewall configuration, and network operations.

### Deployment Scripts
Automated scripts for network deployment:
- `deploy-cyclops-network.sh` - Cyclops network deployment automation

### Extraction Tools
Tools for processing network snapshots:
- `extract` - Partition snapshot extraction tool

## Common Abbreviations
<!-- AI_TAG: abbreviations -->

- **ADI**: Accumulate Digital Identifier
- **BVN**: Block Validator Network
- **DN**: Directory Network
- **P2P**: Peer-to-Peer
- **RPC**: Remote Procedure Call
- **JSON-RPC**: JSON Remote Procedure Call
- **SSL**: Secure Sockets Layer
- **TOML**: Tom's Obvious, Minimal Language

## Flag and Parameter Types
<!-- AI_TAG: flag_types -->

### Required Parameters
Parameters that must be provided for command execution:
- `<network-config.json>` - Network configuration file
- `--work-dir` - Working directory path

### Optional Flags
Parameters that modify command behavior:
- `--follow` - Follower mode (non-voting)
- `--public` - Public IP address
- `--listen` - Listen address and port

### Inherited Flags
Flags inherited from parent commands (e.g., `init` command flags available to all `init` subcommands).

---

## Related Documentation

- [MainNet Reference](./accumulate-mainnet-reference.md) - Network specifications and configuration
- [Node Daemon Commands](../api/accumulated-daemon-commands.md) - Command reference and usage
- [Deployment Guide](../deployment/cyclops-deployment-guide.md) - Deployment automation procedures
