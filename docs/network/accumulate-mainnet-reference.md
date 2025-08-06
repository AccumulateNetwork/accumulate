<!-- AI_DOCUMENT_TYPE: network_reference -->
<!-- AI_PRIMARY_TOPICS: mainnet_config, network_specs, port_config -->
<!-- AI_COMPLEXITY: medium -->
<!-- AI_SPLIT_RECOMMENDED: no -->
<!-- AI_LAST_UPDATED: 2025-01-05 -->

# Accumulate MainNet Reference

> **Document Type**: Network configuration reference  
> **Scope**: MainNet specifications, ports, validators, routing  
> **Target Audience**: Network operators, node administrators

## Quick Reference

| Aspect | Details |
|--------|---------|
| **Network ID** | MainNet |
| **Network Version** | 69 |
| **Partitions** | Directory + 3 BVNs (Apollo, Chandrayaan, Yutu) |
| **Consensus** | Tendermint |
| **DN Ports** | 16591 (P2P), 16592 (RPC), 16595 (JSON-RPC) |
| **BVN Ports** | 16691 (P2P), 16692 (RPC), 16695 (JSON-RPC) |

---

## Network Architecture
<!-- AI_TAG: network_topology -->

The Accumulate network uses a unique architecture where ADIs (Accumulate Digital Identifiers) are distributed over a set of Tendermint networks. The network consists of:

- **1 Directory Network (DN)**: Serves as the central coordination network
- **3 Block Validator Networks (BVNs)**:
  - Apollo
  - Chandrayaan
  - Yutu

Each network uses the Tendermint consensus protocol for both the Block Validator Networks and the Directory Network.

## Network Ports
<!-- AI_TAG: port_configuration -->

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

## Network Specifications
<!-- AI_TAG: network_specs -->

### Network Identity
- **Network ID**: MainNet
- **Network Version**: 69

### Network Partitions
<!-- AI_TAG: partition_config -->

The MainNet consists of the following partitions:

| Partition | Type | Role |
|-----------|------|------|
| Directory | directory | Central coordination network |
| Apollo | blockValidator | Block validation network |
| Chandrayaan | blockValidator | Block validation network |
| Yutu | blockValidator | Block validation network |

### Network Validators
<!-- AI_TAG: validator_config -->

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

## Global Parameters
<!-- AI_TAG: global_params -->

### Fee Schedule
- **Create Identity Sliding Scale**: [4800000, 1200000, 350000, 90000, 25000, 7000, 1800]
- **Create Sub-Identity**: 2500

### System Limits
- **Account Authorities**: 20
- **Book Pages**: 20
- **Data Entry Parts**: 100
- **Identity Accounts**: 1000
- **Page Entries**: 100
- **Pending Major Blocks**: 28

### Consensus Parameters
- **Major Block Schedule**: "0 */12 * * *"
- **Operator Accept Threshold**: 2/3
- **Validator Accept Threshold**: 2/3

### Oracle
- **Price**: 5000

## Special Routing Rules
<!-- AI_TAG: routing_rules -->

The network has special routing rules for certain accounts:

| Account | Assigned Partition |
|---------|-------------------|
| acc://staking.acme | Directory |
| acc://ACME | Directory |
| acc://bvn-Apollo.acme | Apollo |
| acc://bvn-Chandrayaan.acme | Chandrayaan |
| acc://bvn-Yutu.acme | Yutu |
| acc://dn.acme | Directory |

## Network Configuration JSON
<!-- AI_TAG: network_config_json -->

### MainNet Configuration

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

## Important Notes
<!-- AI_TAG: important_notes -->

- These ports are automatically configured when using the AccMan (Accumulate Manager) tool, which is the recommended method for running nodes on the Accumulate network.
- The network uses separate port ranges for the Directory Network (165xx series) and Block Validator Networks (166xx series).
- Firewall configuration is automatically handled by the Accumulate Manager, though manual iptables configuration is possible.
- These port numbers are specific to the Accumulate blockchain protocol and are essential for proper node operation, peer-to-peer communication, and RPC interactions within the network.
- The MainNet configuration includes placeholder advertizeAddress values. The actual addresses would need to be confirmed with the network operators.

---

## Related Documentation

- [Node Daemon Commands](./accumulated-daemon-commands.md) - `accumulated` initialization and runtime
- [Deployment Guide](./cyclops-deployment-guide.md) - Cyclops network deployment
- [Network Glossary](./accumulate-network-glossary.md) - Terminology definitions
