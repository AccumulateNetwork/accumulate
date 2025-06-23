# Accumulate Genesis Format

This document details the format of Genesis files in Accumulate and explains how snapshots are processed to create the initial state of a network partition.

## Overview

Each Accumulate network begins with a Genesis block that defines the initial state of all partitions. The Genesis block is derived from:

1. A network configuration file (JSON/YAML)
2. One or more snapshot files (optional)
3. Factom address balances (optional)

The process is initiated using the command:
```
accumulate init network <config file>
```

## Network Configuration File Format

The network configuration file defines the structure of the network and is provided in JSON or YAML format.

### Configuration File Location

Network configuration files are typically stored in the following locations:

1. **During Network Initialization**:
   - The configuration file is provided as an argument to the `accumulate init network` command
   - Example: `accumulate init network /path/to/network-config.json`

2. **After Initialization**:
   - Each node has its own configuration in `<node-directory>/accumulate.toml`
   - Genesis files are stored as:
     - Binary snapshots: `<node-directory>/<partition>-genesis.snap`
     - JSON format: `<node-directory>/<partition>.json`
   - The Directory Network genesis is stored as `directory-genesis.snap`
   - BVN genesis files are stored as `<bvn-id-lowercase>-genesis.snap`

3. **Working Directory**:
   - When using `accumulate init genesis`, files are written to the working directory specified by the `-w` flag
   - Default location is the current directory

### Example Configuration Structure

```json
{
  "id": "mainnet",
  "bvns": [
    {
      "id": "Yutu",
      "nodes": [
        {
          "basePort": 36656,
          "listenAddress": "0.0.0.0",
          "bvnnType": "validator"
        }
      ]
    },
    {
      "id": "Candrayaan",
      "nodes": [
        {
          "basePort": 46656,
          "listenAddress": "0.0.0.0",
          "bvnnType": "validator"
        }
      ]
    }
  ],
  "bootstrap": {
    "basePort": 26656,
    "listenAddress": "0.0.0.0",
    "dnnType": "validator"
  },
  "globals": {
    "majorVersion": 1,
    "minorVersion": 4,
    "patchVersion": 0,
    "timestamp": "2023-01-01T00:00:00Z",
    "networkType": "mainnet",
    "operatorAcceptThreshold": 0.67,
    "validatorAcceptThreshold": 0.67,
    "feeSchedule": {
      "createIdentity": "1 ACME",
      "createTokenAccount": "0.1 ACME",
      "createDataAccount": "0.1 ACME",
      "createTokenIssuer": "10 ACME",
      "updateAccountAuth": "0.1 ACME",
      "updateKey": "0.1 ACME",
      "sendTokens": "0.01 ACME",
      "createLiteTokenAccount": "0.1 ACME"
    }
  }
}
```

### Key Configuration Elements

1. **Network ID**: Unique identifier for the network
2. **BVNs (Block Validator Networks)**: List of partitions in the network
3. **Nodes**: Configuration for each node in a partition
4. **Globals**: Network-wide parameters including:
   - Version information
   - Network type (mainnet, testnet, devnet)
   - Consensus thresholds
   - Fee schedule

## Genesis JSON Files

The Genesis process produces two types of files:

1. **Binary Snapshot Files** (*.snap): Contains the actual data for the Genesis block
2. **JSON Genesis Files** (*.json): Human-readable representation of the Genesis data

### JSON Genesis File Structure

```json
{
  "header": {
    "rootHash": "base64-encoded-hash",
    "systemLedger": {
      "index": 0,
      "timestamp": 1672531200000000000,
      "pendingUpdates": []
    }
  },
  "records": [
    {
      "key": ["Account", "acc://acme"],
      "value": {
        "type": "tokenIssuer",
        "url": "acc://acme",
        "keyBookUrl": "acc://acme/book",
        "symbol": "ACME",
        "precision": 8,
        "supplyLimit": "1000000000.00000000"
      }
    }
  ]
}
```

## Snapshot Processing for Genesis

When creating a Genesis block from snapshots, the following process is applied:

### 1. Snapshot Extraction (`genesis.Extract`)

The `genesis.Extract` function processes snapshot data in two passes:

#### First Pass: Account Processing
- Reads all account records from the snapshot
- Filters out:
  - Faucet accounts
  - ACME token issuer
  - System accounts (partition accounts)
  - Pending transactions
- Preserves:
  - User accounts and their main state
  - Chain states for data accounts
  - Tracks hash references for messages/transactions

#### Second Pass: Message Processing
- Rewinds the snapshot and reads it again
- Processes only messages and transactions referenced by the accounts from the first pass
- Uses a hash map to quickly identify which messages to keep

### 2. Database Structure

The Genesis process creates an in-memory database that:
- Preserves the insertion order of accounts from the snapshot
- Does not calculate BPT (Binary Patricia Tree) hashes
- Uses the database's internal key-value structure for organization

### 3. Record Sorting

- **Account Records**: Not explicitly sorted; preserved in the order they appear in the input snapshots
- **Message Records**: Explicitly sorted by hash in ascending order
- This approach ensures deterministic processing while maintaining the original account structure

## Genesis File Generation (`buildGenesis`)

The `buildGenesis` function:

1. Processes any provided snapshots using `genesis.Extract`
2. Incorporates Factom address balances if specified
3. Creates a faucet account if a seed is provided
4. Generates Genesis documents for each partition (DN and BVNs)
5. Outputs both binary (.snap) and JSON (.json) versions

## Genesis Initialization Process

When initializing a network with Genesis files:

1. The network configuration is loaded
2. Genesis documents are built for each partition
3. Node configurations are generated
4. Genesis files are written to the appropriate directories

## Network Configurations

### Network Names

#### Mainnet
- **Directory Network**: (No specific name, referred to as "Directory")
- **Block Validator Networks**: Yutu, Candrayaan

#### Kermit (Testnet)
- **Directory Network**: (No specific name, referred to as "Directory")
- **Block Validator Networks**: Chico, Groucho, Harpo

### Port Usage

#### Mainnet Port Configuration

| Service | Base Port | Directory Network | BVN1 | BVN2 |
|---------|-----------|-------------------|------|------|
| Tendermint P2P | 26656 | 26656 | 36656 | 46656 |
| Tendermint RPC | 26657 | 26657 | 36657 | 46657 |
| Tendermint ABCI | 26658 | 26658 | 36658 | 46658 |
| Accumulate API | 26660 | 26660 | 36660 | 46660 |
| Prometheus | 26661 | 26661 | 36661 | 46661 |

#### Kermit Port Configuration

| Service | Base Port | Directory Network | BVN1 | BVN2 |
|---------|-----------|-------------------|------|------|
| Tendermint P2P | 26656 | 26656 | 36656 | 46656 |
| Tendermint RPC | 26657 | 26657 | 36657 | 46657 |
| Tendermint ABCI | 26658 | 26658 | 36658 | 46658 |
| Accumulate API | 26660 | 26660 | 36660 | 46660 |
| Prometheus | 26661 | 26661 | 36661 | 46661 |

## Special Considerations

### BPT Handling

- The Genesis process deliberately omits BPT calculation (`calculateBPT = false`)
- This reduces memory requirements during Genesis creation
- Each node reconstructs the BPT when loading the Genesis snapshot

### URL Handling

- URLs are stored in both a KV database for fast lookups and a binary file format for iteration
- This hybrid approach prevents memory issues with large datasets
- Lookups are performed primarily from the KV database with fallback to file-based lookup

### Record Processing Order

When a node loads a Genesis snapshot, records are processed in this order:

1. Account records first to establish the account hierarchy
2. Chain records to establish chain state
3. Transaction and message records to populate chains

This ensures that all necessary structures are in place before dependent records are processed.
