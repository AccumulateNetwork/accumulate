# Accumulate Network Architecture

## Metadata
- **Document Type**: Index
- **Version**: 1.0
- **Last Updated**: 2025-05-17
- **Related Components**: Network Design, Consensus, P2P
- **Tags**: network, architecture, routing, consensus, index

## 1. Introduction

This section provides documentation on the network architecture of the Accumulate blockchain protocol. It covers routing, peer-to-peer networking, network initialization, peer discovery, and consensus mechanisms.

## 2. Available Sections

### [01. Routing](./01_routing/)
Documentation on Accumulate's routing architecture, which directs transactions to the appropriate partitions.

### [02. P2P Networking](./02_p2p/)
Information on the peer-to-peer networking layer that enables communication between nodes.

### [03. Network Initialization](./03_initialization/)
Details on how the network is initialized and bootstrapped.

### [04. Peer Discovery](./04_peer_discovery/)
Documentation on how nodes discover and connect to peers in the network.

### [05. Consensus](./05_consensus/)
Information on the consensus mechanism used by Accumulate, including integration with CometBFT.

## 3. Network Architecture Overview

The Accumulate network consists of multiple partitions:

- **Block Validator Networks (BVNs)**: Process transactions for specific partitions
- **Directory Network (DN)**: Coordinates between BVNs and maintains the global state

These partitions communicate through a combination of:

- **Direct P2P Communication**: For node-to-node messaging
- **Synthetic Transactions**: For cross-partition state updates
- **Anchoring**: For securing state across partitions

## 4. Key Network Features

The Accumulate network architecture provides several key features:

- **Scalability**: Multiple partitions allow for horizontal scaling
- **Resilience**: Distributed design provides fault tolerance
- **Security**: Consensus and anchoring ensure data integrity
- **Efficiency**: Routing directs transactions to the appropriate partition

## Related Documents

- [Architecture Overview](../02_architecture/01_overview.md)
- [Core Components](../03_core_components/00_index.md)
- [Implementation Details](../06_implementation/00_index.md)
