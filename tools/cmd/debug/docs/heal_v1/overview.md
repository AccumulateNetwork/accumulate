# Healing Overview {#healing-overview}

This document provides a high-level overview of the approaches used to heal the Accumulate network. For detailed information about specific aspects of healing, refer to the linked documents.

## What is Healing? {#what-is-healing}

Healing is the process of ensuring that transactions are properly delivered across partitions in the Accumulate network. When a transaction needs to be processed by multiple partitions (e.g., a transaction originating in one BVN that affects an account in another BVN), the network uses anchors and synthetic transactions to ensure consistency across partitions.

For a detailed explanation of the different chain types (root chains, synthetic sequence chains, anchor chains) and how they are handled during the healing process, please refer to the [Chains and URLs](./chains_urls.md) document.

Sometimes these cross-partition transactions can fail to be delivered due to network issues or other problems. The healing process identifies these failed deliveries and resubmits the necessary transactions to ensure they are properly processed.

## Two Healing Approaches {#two-approaches-to-healing}

The Accumulate network implements two distinct approaches to healing:

### 1. heal_anchor {#heal-anchor}

The `heal_anchor` approach focuses on healing anchor transactions - transactions that anchor one partition's state to another. This approach:

- Does not require a database or Light client
- Queries validator nodes directly for signatures
- Collects signatures from validators and submits them to complete the anchor
- Uses different submission methods based on network version

[Detailed Documentation: transactions.md](./transactions.md#heal_anchor-transaction-creation)

### 2. heal_synth {#heal-synth}

The `heal_synth` approach focuses on healing synthetic transactions - transactions that were anchored but not delivered to their destination. This approach:

- Requires a database and Light client to build Merkle proofs
- Constructs receipts to prove transaction inclusion in the source partition
- Creates synthetic messages with Merkle proofs
- Submits directly to nodes in the destination partition

[Detailed Documentation: transactions.md](./transactions.md#heal_synth-transaction-creation)

## Light Client as Infrastructure {#light-client}

The Light client is a critical infrastructure component used by the `heal_synth` approach to facilitate healing. It serves as a bridge between the healing code and the database, providing a consistent API for accessing account data, chain data, and transaction data. The Light client is used to build Merkle proofs for synthetic transactions, enabling the healing process to verify transaction inclusion in the source partition.

The Light client is explicitly disabled for the `heal_anchor` approach, as it is not required for anchor transaction healing.

[Detailed Documentation: light_client.md](./light_client.md#light-client-overview)

## Common Components {#common-components}

Both healing approaches share some common components:

- **Network Information**: Both approaches use the same network information structure to understand the network topology
- **URL Construction**: Both approaches need to handle URL construction for different chain types, as detailed in [Chains and URLs](./chains_urls.md)
- **Submission Process**: Both approaches use a common channel-based submission process
- **Transaction Identification**: Both approaches use the same `SequencedInfo` structure to identify transactions

[Detailed Documentation: implementation.md](./implementation.md#shared-components)

## Choosing an Approach {#choosing-an-approach}

The appropriate healing approach depends on your specific needs:

- **heal_anchor**: Use when you need to heal anchor transactions and don't want to set up a database
- **heal_synth**: Use when you need to heal synthetic transactions and have a database available

## Next Steps

For more detailed information about specific aspects of healing, refer to the following documents:

- [Chains and URLs](./chains_urls.md)
- [Database Requirements](./database.md)
- [Transaction Creation](./transactions.md)
- [Light Client Implementation](./light_client.md)
- [Implementation Guidelines](./implementation.md)
