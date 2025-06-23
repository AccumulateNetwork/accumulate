# Accumulate MainNet Configuration

This document provides the official configuration details for the Accumulate MainNet, including network architecture, port configurations, and validator information.

## Network Architecture

The Accumulate network uses a unique architecture where ADIs (Accumulate Digital Identifiers) are distributed over a set of Tendermint networks. The network consists of:

- 1 Directory Network (DN): Serves as the central coordination network
- 3 Block Validator Networks (BVNs):
  - Apollo
  - Chandrayaan
  - Yutu

Each network uses the Tendermint consensus protocol for both the Block Validator Networks and the Directory Network.

## Core Network Ports

### Directory Network (DN) Ports:
- **16591** - DN P2P (Peer-to-Peer communication)
- **16592** - DN RPC (Remote Procedure Call)
- **16595** - DN RPC JSON (JSON-RPC interface)

### Block Validator Network (BVN) Ports:
- **16691** - BVN P2P (Peer-to-Peer communication)
- **16692** - BVN RPC (Remote Procedure Call)
- **16695** - BVN RPC JSON (JSON-RPC interface)

### Management and SSL Ports:
- **16666** - AccMan (Accumulate Manager)
- **6695** - SSL Client (HTTPS access)

## Network Configuration Details

### Network ID and Version
- Network ID: MainNet
- Network Version: 69

### Partitions
The MainNet consists of the following partitions:
- Directory (type: directory)
- Apollo (type: blockValidator)
- Chandrayaan (type: blockValidator)
- Yutu (type: blockValidator)

### Validators

The network is maintained by multiple validators operated by different entities:

| Operator | Partitions |
|----------|------------|
| kompendium.acme | Directory, Apollo |
| LunaNova.acme | Directory, Apollo |
| Factoshi.acme | - |
| TurtleBoat.acme | Directory, Chandrayaan |
| Stamp-It.acme | - |
| MusicCityNode.acme | Directory, Yutu |
| ConsensusNetworks.acme | Directory, Chandrayaan |
| defacto.acme | - |
| tfa.acme | Directory, Yutu |
| CodeForj.acme | Directory, Yutu |
| PrestigeIT.acme | Directory, Chandrayaan |
| GOI.acme | - |
| defidevs.acme | Directory, Chandrayaan, Apollo, Yutu |
| Sphereon.acme | Directory, Chandrayaan |
| ACMEMining.acme | Directory, Apollo |
| Inveniam.acme | Directory, Chandrayaan |
| HighStakes.acme | Directory, Yutu |
| FederateThis.acme | Directory, Apollo |
| DetroitLedgerTech.acme | Directory, Yutu |

### Global Parameters

#### Fee Schedule
- Create Identity Sliding Scale: [4800000, 1200000, 350000, 90000, 25000, 7000, 1800]
- Create Sub-Identity: 2500

#### System Limits
- Account Authorities: 20
- Book Pages: 20
- Data Entry Parts: 100
- Identity Accounts: 1000
- Page Entries: 100
- Pending Major Blocks: 28

#### Consensus Parameters
- Major Block Schedule: "0 */12 * * *"
- Operator Accept Threshold: 2/3
- Validator Accept Threshold: 2/3

#### Oracle
- Price: 5000

## Important Notes

- These ports are automatically configured when using the AccMan (Accumulate Manager) tool, which is the recommended method for running nodes on the Accumulate network.
- The network uses separate port ranges for the Directory Network (165xx series) and Block Validator Networks (166xx series).
- Firewall configuration is automatically handled by the Accumulate Manager, though manual iptables configuration is possible.
- These port numbers are specific to the Accumulate blockchain protocol and are essential for proper node operation, peer-to-peer communication, and RPC interactions within the network.

## Special Routing Rules

The network has special routing rules for certain accounts:

| Account | Assigned Partition |
|---------|-------------------|
| acc://staking.acme | Directory |
| acc://ACME | Directory |
| acc://bvn-Apollo.acme | Apollo |
| acc://bvn-Chandrayaan.acme | Chandrayaan |
| acc://bvn-Yutu.acme | Yutu |
| acc://dn.acme | Directory |

## Network Configuration Files

### Kermit Network Configuration

The Kermit network is a testnet configuration with the following structure:

```json
{
    "id": "Kermit",
    "globals": {
        "executorVersion": "v2-vandenberg",
        "oracle": {
            "price": 500000
        },
        "globals": {
            "feeSchedule": {
                "createIdentitySliding": [
                    400000
                ],
                "createSubIdentity": 10000,
                "bareIdentityDiscount": 10000
            },
            "limits": {
                "identityAccounts": 1000
            }
        },
        "routing": {
            "routes": [
                {
                    "length": 2,
                    "partition": "Chico"
                },
                {
                    "length": 2,
                    "partition": "Harpo",
                    "value": 1
                },
                {
                    "length": 2,
                    "partition": "Groucho",
                    "value": 2
                },
                {
                    "length": 3,
                    "partition": "Chico",
                    "value": 6
                },
                {
                    "length": 4,
                    "partition": "Harpo",
                    "value": 14
                },
                {
                    "length": 4,
                    "partition": "Groucho",
                    "value": 15
                }
            ]
        }
    },
    "bvns": [
        {
            "id": "Chico",
            "nodes": [
                {
                    "dnnType": "validator",
                    "bvnnType": "validator",
                    "basePort": 16591,
                    "advertizeAddress": "kermit-bvn0.accumulate.defidevs.io"
                }
            ]
        },
        {
            "id": "Harpo",
            "nodes": [
                {
                    "dnnType": "validator",
                    "bvnnType": "validator",
                    "basePort": 16591,
                    "advertizeAddress": "kermit-bvn1.accumulate.defidevs.io"
                }
            ]
        },
        {
            "id": "Groucho",
            "nodes": [
                {
                    "dnnType": "validator",
                    "bvnnType": "validator",
                    "basePort": 16591,
                    "advertizeAddress": "kermit-bvn2.accumulate.defidevs.io"
                }
            ]
        }
    ]
}
```

Key features of the Kermit network:
- Three BVNs named after the Marx Brothers: Chico, Harpo, and Groucho
- Each BVN has a single validator node
- Base port configuration starts at 16591
- Uses the "v2-vandenberg" executor version
- Simplified fee schedule compared to MainNet

### MainNet Network Configuration

Based on the describe.json and network architecture, here is the equivalent network.json for MainNet:

```json
{
    "id": "MainNet",
    "globals": {
        "executorVersion": "v2-vandenberg",
        "oracle": {
            "price": 5000
        },
        "globals": {
            "feeSchedule": {
                "createIdentitySliding": [
                    4800000,
                    1200000,
                    350000,
                    90000,
                    25000,
                    7000,
                    1800
                ],
                "createSubIdentity": 2500
            },
            "limits": {
                "accountAuthorities": 20,
                "bookPages": 20,
                "dataEntryParts": 100,
                "identityAccounts": 1000,
                "pageEntries": 100,
                "pendingMajorBlocks": 28
            },
            "majorBlockSchedule": "0 */12 * * *",
            "operatorAcceptThreshold": {
                "denominator": 3,
                "numerator": 2
            },
            "validatorAcceptThreshold": {
                "denominator": 3,
                "numerator": 2
            }
        },
        "routing": {
            "routes": [
                {
                    "length": 2,
                    "partition": "Apollo"
                },
                {
                    "length": 2,
                    "partition": "Yutu",
                    "value": 1
                },
                {
                    "length": 2,
                    "partition": "Chandrayaan",
                    "value": 2
                },
                {
                    "length": 3,
                    "partition": "Apollo",
                    "value": 6
                },
                {
                    "length": 4,
                    "partition": "Yutu",
                    "value": 14
                },
                {
                    "length": 4,
                    "partition": "Chandrayaan",
                    "value": 15
                }
            ],
            "overrides": [
                {
                    "account": "acc://staking.acme",
                    "partition": "Directory"
                },
                {
                    "account": "acc://ACME",
                    "partition": "Directory"
                },
                {
                    "account": "acc://bvn-Apollo.acme",
                    "partition": "Apollo"
                },
                {
                    "account": "acc://bvn-Chandrayaan.acme",
                    "partition": "Chandrayaan"
                },
                {
                    "account": "acc://bvn-Yutu.acme",
                    "partition": "Yutu"
                },
                {
                    "account": "acc://dn.acme",
                    "partition": "Directory"
                }
            ]
        }
    },
    "bvns": [
        {
            "id": "Apollo",
            "nodes": [
                {
                    "dnnType": "validator",
                    "bvnnType": "validator",
                    "basePort": 16591,
                    "advertizeAddress": "apollo.accumulate.network"
                }
            ]
        },
        {
            "id": "Chandrayaan",
            "nodes": [
                {
                    "dnnType": "validator",
                    "bvnnType": "validator",
                    "basePort": 16591,
                    "advertizeAddress": "chandrayaan.accumulate.network"
                }
            ]
        },
        {
            "id": "Yutu",
            "nodes": [
                {
                    "dnnType": "validator",
                    "bvnnType": "validator",
                    "basePort": 16591,
                    "advertizeAddress": "yutu.accumulate.network"
                }
            ]
        },
        {
            "id": "Directory",
            "nodes": [
                {
                    "dnnType": "validator",
                    "bvnnType": "none",
                    "basePort": 16591,
                    "advertizeAddress": "directory.accumulate.network"
                }
            ]
        }
    ]
}
```

Note: The MainNet configuration includes placeholder advertizeAddress values. The actual addresses would need to be confirmed with the network operators.
