# Building the Accumulate Mainnet with Cyclops BVN

This document provides a step-by-step guide for building the Accumulate mainnet with a single BVN named "cyclops", following exactly the same approach used by the devnet command but with cyclops-specific settings.

## Plan of Action

1. Prepare the cyclops network configuration and genesis snapshots
2. Build the Accumulate binary
3. Initialize the network using the devnet approach
4. Verify the directory structure and configuration
5. Start the network
6. Monitor the network

## Detailed Steps

### 1. Prepare the Cyclops Network Configuration and Genesis Snapshots

First, ensure you have the following files in your artifacts directory:

- `cyclops-network.json`: The network configuration file
- `bvn-cyclops-genesis.snap`: The BVN genesis snapshot
- `directory-genesis.snap`: The DN genesis snapshot

These files define the initial state and configuration of the cyclops network. We'll rename the genesis files to match devnet's standardized naming convention.

```json
{
  "id": "cyclops",
  "template": "[[configurations]]\n  type = \"coreValidator\"\n  enable-healing = false\n  enable-snapshots = false\n  storage-type = \"leveldb\"\n\n[storage]\n  type = \"leveldb\"\n  path = \"data/accumulate.db\"",
  "globals": {
    "oracle": {
      "price": 50000
    },
    "globals": {
      "majorBlockSchedule": "0 0 * * *",
      "executorVersion": "v2baikonur",
      "feeSchedule": {
        "createIdentitySliding": [4800000, 1200000, 350000, 90000, 25000, 7000, 1800],
        "createSubIdentity": 2500,
        "createTokenAccount": 10000,
        "createDataAccount": 10000,
        "createKeyPage": 10000,
        "createKeyBook": 10000,
        "sendTokens": 1000,
        "addCredits": 1000,
        "updateKeyPage": 1000,
        "updateKey": 1000,
        "writeData": 1000,
        "writeDataTo": 1000,
        "burnTokens": 1000,
        "issueTokens": 1000,
        "createToken": 50000,
        "updateAccountAuth": 1000,
        "syntheticDepositTokens": 1000,
        "syntheticDepositCredits": 1000,
        "syntheticBurnTokens": 1000,
        "syntheticCreateIdentity": 25000,
        "syntheticCreateTokenAccount": 10000,
        "syntheticWriteData": 1000
      },
      "limits": {
        "dataEntryParts": 100,
        "dataEntryPartsCompressed": 100,
        "dataEntrySize": 100000,
        "dataEntrySizeCompressed": 100000
      },
      "values": {
        "acmeSupply": 500000000000000,
        "acmePrecision": 8,
        "acmeIssuedSupply": 500000000000000
      }
    },
    "network": {
      "networkName": "cyclops",
      "partitionDuration": 2,
      "blockTime": 1,
      "maxWritesPerBlock": 1000,
      "maxEntriesPerBlock": 5000,
      "maxAnnouncementsPerBlock": 1000,
      "maxTransactionsPerBlock": 1000,
      "maxBlocksPerSecond": 2,
      "maxPendingTransactions": 10000,
      "maxAnchorPoints": 10000,
      "maxAnchorBuilders": 10,
      "maxDirBuilders": 10,
      "maxPendingMajorBlocks": 10,
      "maxMajorBlocksPerSecond": 0.1,
      "maxPendingMinorBlocks": 100,
      "maxMinorBlocksPerSecond": 1,
      "maxBlockValidators": 100,
      "maxBlockValidatorsPerBatch": 10,
      "maxBlockValidatorQueue": 1000,
      "maxBlockValidatorBatchSize": 10,
      "maxBlockValidatorBatchCount": 100,
      "maxBlockValidatorBatchWait": 100,
      "maxBlockValidatorProcessTime": 1000,
      "maxBlockValidatorProcessCount": 10,
      "maxBlockValidatorProcessBatch": 10
    }
  },
  "bvns": [
    {
      "id": "bvn-cyclops",
      "type": "bvn",
      "nodes": [
        {
          "address": "127.0.0.1",
          "basePort": 26656,
          "bvnnType": "validator",
          "dnnType": "validator",
          "tmHome": ".nodes/bvn-cyclops-1",
          "apiPort": 26660,
          "metricsPort": 26661,
          "prometheusPort": 26662,
          "debugPort": 26663,
          "seeds": []
        }
      ]
    }
  ],
  "dn": {
    "nodes": [
      {
        "address": "127.0.0.1",
        "basePort": 26756,
        "bvnnType": "none",
        "dnnType": "validator",
        "tmHome": ".nodes/dn-1",
        "apiPort": 26760,
        "metricsPort": 26761,
        "prometheusPort": 26762,
        "debugPort": 26763,
        "seeds": []
      }
    ]
  },
  "routing": {
    "routingPublicKey": "",
    "routingPrivateKey": ""
  },
  "bootstrapAccounts": [
    {
      "url": "acc://NodeWithNoMane.acme",
      "tokenUrl": "acc://ACME",
      "keyBookUrl": "acc://NodeWithNoMane.acme/book",
      "keyPageUrl": "acc://NodeWithNoMane.acme/book/1",
      "keys": [
        {
          "publicKey": "",
          "keyType": "ed25519"
        }
      ]
    }
  ]
}
```

### 2. Build the Accumulate Binary

Build the latest version of the Accumulate binary with our fixes:

```bash
cd /path/to/accumulate/repo
go build -o accumulated ./cmd/accumulated
```

### 3. Initialize the Network Using the Devnet Approach

To exactly match how the devnet command works, we'll create a single node directory that contains both BVN and DN components:

```bash
# Create the main directory
mkdir -p ~/accumulate-network/nodes

# Copy and rename the genesis snapshots to match devnet's standardized naming
cp ~/accumulate-network/artifacts/bvn-cyclops-genesis.snap ~/accumulate-network/nodes/bvn1-genesis.snap
cp ~/accumulate-network/artifacts/directory-genesis.snap ~/accumulate-network/nodes/dn-genesis.snap

# Create the node directory
mkdir -p ~/accumulate-network/nodes/bvn1-1

# Create the BVN and DN subdirectories
mkdir -p ~/accumulate-network/nodes/bvn1-1/bvnn/config
mkdir -p ~/accumulate-network/nodes/bvn1-1/bvnn/data
mkdir -p ~/accumulate-network/nodes/bvn1-1/dnn/config
mkdir -p ~/accumulate-network/nodes/bvn1-1/dnn/data

# Initialize the node with both BVN and DN components
./accumulated init dual \
  --work-dir ~/accumulate-network/nodes \
  --home bvn1-1 \
  --bvn-genesis ~/accumulate-network/nodes/bvn1-genesis.snap \
  --dn-genesis ~/accumulate-network/nodes/dn-genesis.snap
```

This sequence will:
- Create the exact directory structure used by devnet
- Configure the node to run both BVN and DN components in a single process
- Set up Tendermint for both partitions in separate subdirectories
- Configure the node to use the cyclops genesis files

### 4. Verify the Directory Structure and Configuration

After initialization, verify that the directory structure matches exactly the devnet layout:

```
~/accumulate-network/nodes/
├── accumulate.toml           # Main configuration file
├── bvn1-1/                  # Node directory containing both BVN and DN configurations
│   ├── accumulate.toml      # Node-specific configuration
│   ├── bvnn/                # BVN node subdirectory
│   │   ├── config/         # Tendermint config for BVN
│   │   │   ├── config.toml
│   │   │   ├── genesis.json
│   │   │   └── priv_validator_key.json
│   │   └── data/           # BVN data directory
│   └── dnn/                # DN node subdirectory
│       ├── config/         # Tendermint config for DN
│       │   ├── config.toml
│       │   ├── genesis.json
│       │   └── priv_validator_key.json
│       └── data/           # DN data directory
├── bvn1-genesis.snap       # BVN genesis snapshot (renamed from bvn-cyclops-genesis.snap)
└── dn-genesis.snap         # DN genesis snapshot (renamed from directory-genesis.snap)
```

Check the configuration files to ensure they have the correct settings:

- In the main `accumulate.toml`:
  - `network = "cyclops"`

- In the node's `accumulate.toml`, verify both BVN and DN configurations are present:
  ```toml
  [[configurations]]
  type = "coreValidator"
  listen = "/ip4/127.0.0.1/tcp/26656"
  bvn = "Directory"
  validator-key = { address = "..." }
  dn-genesis = "../dn-genesis.snap"
  bvn-genesis = "../bvn1-genesis.snap"
  
  [[configurations]]
  type = "coreValidator"
  listen = "/ip4/127.0.0.1/tcp/26657"
  bvn = "bvn-cyclops"
  validator-key = { address = "..." }
  dn-genesis = "../dn-genesis.snap"
  bvn-genesis = "../bvn1-genesis.snap"
  ```

- In the Tendermint config files:
  - BVN: `chain_id = "bvn-cyclops"`
  - DN: `chain_id = "Directory"`

### 5. Start the Network

Exactly like the devnet command, start the network with a single command that runs both BVN and DN components in one process:

```bash
cd ~/accumulate-network/nodes
./accumulated run --home bvn1-1
```

This single command will start both the BVN and DN components in a unified process, exactly as the devnet command does.

### 6. Monitor the Network

You can monitor the network using the API endpoints:

```bash
# Check node status (includes both BVN and DN)
curl http://127.0.0.1:26660/v2/status

# Check specific partition status
curl http://127.0.0.1:26660/v2/status?partition=bvn-cyclops
curl http://127.0.0.1:26660/v2/status?partition=Directory
```

## Step-by-Step Execution Guide

This section provides the exact commands to execute in order to set up the Accumulate network using the devnet approach with cyclops settings.

### 1. Clean Up Previous Installation

```bash
# Remove any existing node directories
rm -rf ~/accumulate-network/nodes
```

### 2. Prepare Directory Structure and Files

```bash
# Create the main directory
mkdir -p ~/accumulate-network/nodes

# Copy the genesis snapshot to the nodes directory
cp ~/accumulate-network/artifacts/cyclops-genesis.snap ~/accumulate-network/nodes/

# Create the node directory
mkdir -p ~/accumulate-network/nodes/bvn1-1

# Create the BVN and DN subdirectories
mkdir -p ~/accumulate-network/nodes/bvn1-1/bvnn/config
mkdir -p ~/accumulate-network/nodes/bvn1-1/bvnn/data
mkdir -p ~/accumulate-network/nodes/bvn1-1/dnn/config
mkdir -p ~/accumulate-network/nodes/bvn1-1/dnn/data
```

### 3. Initialize the Node

```bash
# Navigate to the nodes directory
cd ~/accumulate-network/nodes

# Initialize the node with both BVN and DN components
# Use the snapshot flag to specify the genesis snapshot
~/accumulate-network/artifacts/accumulated init dual bvn-cyclops.cyclops \
  --work-dir ~/accumulate-network/nodes \
  --snapshot ~/accumulate-network/nodes/cyclops-genesis.snap
```

### 4. Start the Network

```bash
# Navigate to the nodes directory
cd ~/accumulate-network/nodes

# Start the node
~/accumulate-network/artifacts/accumulated run --home bvn1-1
```

### 5. Monitor the Network

In a new terminal:

```bash
# Check overall node status
curl http://127.0.0.1:26660/v2/status

# Check BVN status
curl http://127.0.0.1:26660/v2/status?partition=bvn-cyclops

# Check DN status
curl http://127.0.0.1:26660/v2/status?partition=Directory
```

## ADI Creation Fee Sliding Scale

The Accumulate network uses a sliding scale for ADI (Accumulate Digital Identifier) creation fees. This makes shorter ADI names significantly more expensive, which helps prevent name squatting and encourages longer, more descriptive names.

In the network configuration, this is defined in the `createIdentitySliding` array within the `feeSchedule` section:

```json
"feeSchedule": {
  "createIdentitySliding": [4800000, 1200000, 350000, 90000, 25000, 7000, 1800],
  "createSubIdentity": 2500,
  // other fees...
}
```

The array works as follows:
- Index 0: Fee for 1-character ADIs (4,800,000 credits)
- Index 1: Fee for 2-character ADIs (1,200,000 credits)
- Index 2: Fee for 3-character ADIs (350,000 credits)
- And so on...

If an ADI name is longer than the array length, the default fee is used. Sub-identities (non-root ADIs) use the `createSubIdentity` fee regardless of length.

## Database Backend Configuration

The network is configured to use LevelDB as the database backend for improved performance and compatibility. This is specified in the `template` field of the network configuration:

```toml
[[configurations]]
  type = "coreValidator"
  enable-healing = false
  enable-snapshots = false
  storage-type = "leveldb"

[storage]
  type = "leveldb"
  path = "data/accumulate.db"
```

This configuration is applied to all nodes during network initialization, ensuring consistent database backend usage across the network.

## Additional Notes

1. **Multiple Validators**: For a production network, you would typically have multiple validators for both BVN and DN. Adjust the configuration file accordingly.

2. **Network Security**: For a production network, ensure proper security measures are in place, including:
   - Proper firewall rules
   - Secure key management
   - Network monitoring

3. **Backup**: Always maintain backups of your configuration files and validator keys.

4. **Upgrades**: When upgrading the network, ensure all nodes are upgraded simultaneously to avoid consensus issues.
