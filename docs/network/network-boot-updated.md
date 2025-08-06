# Building the Accumulate Mainnet with Cyclops BVN

This document provides a step-by-step guide for building the Accumulate mainnet with a single BVN named "cyclops", following the same approach used by the devnet command.

## Plan of Action

1. Create a network configuration file
2. Build the Accumulate binary
3. Initialize the network using the devnet approach
4. Start the network
5. Monitor the network

## Detailed Steps

### 1. Create a Network Configuration File

First, ensure you have the cyclops network configuration JSON file (e.g., `cyclops-network.json`) with the network configuration. This file should already exist in the artifacts directory.

### 2. Build the Accumulate Binary

Build the latest version of the Accumulate binary:

```bash
cd /path/to/accumulate/repo
go build -o accumulated ./cmd/accumulated
```

### 3. Initialize the Network Using the Devnet Approach

Instead of using the `init network` command followed by separate node initializations, we'll use the `devnet` command which creates a unified node structure that runs both BVN and DN components in a single process:

```bash
# Create the devnet with the cyclops configuration
./accumulated devnet --work-dir ~/accumulate-network/nodes \
  --name cyclops \
  --bvns 1 \
  --validators 1 \
  --database leveldb
```

This command will:
- Create a single node directory structure
- Generate all necessary keys
- Create both BVN and DN configurations within the same node
- Generate genesis files for both partitions
- Configure Tendermint for both partitions
- Set up a faucet account

### 4. Verify the Directory Structure and Configuration

After initialization, verify that the directory structure matches the expected devnet layout:

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
├── bvn1-genesis.snap       # BVN genesis snapshot
└── dn-genesis.snap         # DN genesis snapshot
```

Check the configuration files to ensure they have the correct settings:

- In the main `accumulate.toml`:
  - `network = "cyclops"`

- In the node's `accumulate.toml`, verify both BVN and DN configurations are present:
  ```toml
  [[configurations]]
  type = "coreValidator"
  listen = "/ip4/127.0.1.1/tcp/26656"
  bvn = "Directory"
  # DN-specific configuration
  
  [[configurations]]
  type = "coreValidator"
  listen = "/ip4/127.0.1.1/tcp/26657"
  bvn = "BVN1"
  # BVN-specific configuration
  ```

### 5. Start the Network

Start the network using the devnet command:

```bash
cd ~/accumulate-network/nodes
./accumulated devnet --work-dir .
```

This single command will start both the BVN and DN components in a unified process.

### 6. Monitor the Network

You can monitor the network using the API endpoints:

```bash
# Check node status (includes both BVN and DN)
curl http://127.0.0.1:26660/v2/status
```

## Adapting the Cyclops Configuration

To use the existing cyclops network configuration with the devnet approach, you may need to make these adjustments:

1. **Network ID**: Ensure the network ID is set to "cyclops"

2. **BVN ID**: The devnet uses "BVN1" format, while the cyclops config uses "bvn-cyclops". This will be handled automatically.

3. **Genesis Files**: The devnet will generate its own genesis files based on the configuration.

4. **Node Structure**: Instead of separate directories for BVN and DN nodes, the devnet approach uses a single directory with subdirectories for each partition.

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

The network is configured to use LevelDB as the database backend for improved performance and compatibility. This is specified in the devnet command with the `--database leveldb` flag.

## Additional Notes

1. **Multiple Validators**: For a production network, you would typically have multiple validators. Adjust the `--validators` flag accordingly.

2. **Network Security**: For a production network, ensure proper security measures are in place, including:
   - Proper firewall rules
   - Secure key management
   - Network monitoring

3. **Backup**: Always maintain backups of your configuration files and validator keys.

4. **Upgrades**: When upgrading the network, ensure all nodes are upgraded simultaneously to avoid consensus issues.
