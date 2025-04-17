# Healing Process in Accumulate Network

This document provides an overview of the healing process in the Accumulate Network, focusing on the different types of chains and how they are handled during healing.

## Chain Types and Healing Behavior

### Root Chains

1. **Definition and Location**:
   - Root chains are stored at the path `acc://{partition-id}.acme/ledger/root`
   - Each partition (BVN or DN) maintains its own root chain
   - They're accessed via `Account(partition.Ledger()).RootChain()`

2. **Purpose**:
   - Root chains store the root state of the ledger for each partition
   - They serve as anchoring points for other chains within the partition
   - Other chains within a partition are anchored into the root chain

3. **Construction Process**:
   - Root chains are constructed by the network itself during block processing
   - Root chains are updated during block finalization
   - Each partition maintains its own root chain independently

4. **Syncing Behavior**:
   - **Root chains are NOT synced between networks**
   - They are partition-specific and maintained independently
   - During healing, root chains are checked for diagnostic purposes only, but never modified

### Sequence Chains

#### Synthetic Sequence Chains

1. **Definition and Location**:
   - Synthetic sequence chains are stored at `acc://{partition-id}.acme/synthetic/synthetic-sequence-{0,1,index}`
   - They're accessed via `Account(ledger).SyntheticSequenceChain(partition)`

2. **Purpose**:
   - Synthetic sequence chains track the sequence of synthetic transactions between partitions
   - There are multiple sequence chains (0, 1, and index) for redundancy and indexing
   - They're used to ensure proper ordering and delivery of synthetic transactions

3. **Construction Process**:
   - Synthetic sequence chains are constructed when synthetic transactions are created
   - Each partition maintains sequence chains for synthetic transactions it sends to other partitions

4. **Syncing Behavior**:
   - **Synthetic sequence chains ARE synced between networks**
   - The healing process ensures that synthetic transactions (and their sequence chains) are properly synced between partitions
   - This is critical for maintaining transaction consistency across the network

#### Anchor Sequence Chains

1. **Similar to Synthetic Sequence Chains**:
   - Anchor sequence chains track the sequence of anchor transactions between partitions
   - They follow a similar pattern to synthetic sequence chains but are for anchoring purposes

2. **Syncing Behavior**:
   - Like synthetic sequence chains, anchor sequence chains are also synced between networks
   - The healing process ensures that anchor chains are properly maintained across partitions

## Key Differences in Healing Behavior

1. **Root Chains vs. Sequence Chains**:
   - **Root chains** are partition-specific and not synced between networks
   - **Sequence chains** (both synthetic and anchor) are synced between networks to ensure transaction delivery

2. **Healing Process**:
   - The healing process only targets synthetic and anchor chains, not root chains
   - Root chains are checked for diagnostic purposes but never modified during healing

3. **URL Construction**:
   - Root chains: `acc://{partition-id}.acme/ledger/root`
   - Synthetic sequence chains: `acc://{partition-id}.acme/synthetic/synthetic-sequence-{0,1,index}`
   - Anchor chains: `acc://{partition-id}.acme/anchors/{target-partition}`

## Implications for Diagnostics and Healing

1. **Root Chains**:
   - Included in diagnostics for comparison purposes
   - Marked as "not healed" since they're not part of the healing process
   - Used only for diagnostic purposes to understand the state of each partition

2. **Synthetic Sequence Chains**:
   - Included in diagnostics and actively healed
   - Marked as "healed" since they're part of the healing process
   - Critical for ensuring proper transaction delivery between partitions

3. **Anchor Chains**:
   - Also included in diagnostics and actively healed
   - Important for maintaining the cross-partition anchoring structure

This understanding is crucial for implementing proper diagnostics and healing processes, especially when designing the URL diagnostics table to clearly indicate which chains are actually healed and which are only checked for diagnostic purposes.
