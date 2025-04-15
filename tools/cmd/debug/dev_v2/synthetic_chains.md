# Understanding Synthetic Chains in Accumulate

## Overview

Synthetic chains are a critical component of the Accumulate network's architecture, enabling cross-partition communication and transaction processing. This document explains the purpose of synthetic chains, how they are structured, and which chains are healed during the healing process.

## Synthetic Transaction Chains

### Purpose

Synthetic transactions are generated when a transaction on one partition (the source) needs to affect state on another partition (the destination). The synthetic transaction system ensures that:

1. Cross-partition effects are properly propagated
2. Transaction ordering is maintained
3. State consistency is preserved across the network

### Chain Types

The Accumulate network uses several types of synthetic chains:

1. **Synthetic Sequence Chains**: These chains track synthetic transactions between partition pairs. Each partition maintains separate synthetic sequence chains for each other partition it interacts with.

2. **Synthetic Transaction Chains**: These chains store the actual synthetic transaction data.

3. **Root Chains**: These chains store the root state of the ledger but are not directly related to synthetic transactions.

## Partition-Specific Synthetic Sequence Chains

Each account in Accumulate maintains partition-specific synthetic sequence chains. This is evident from the code:

```go
func (c *Account) SyntheticSequenceChain(partition string) *Chain2 {
    return c.getSyntheticSequenceChain(strings.ToLower(partition))
}

func (c *Account) newSyntheticSequenceChain(k accountSyntheticSequenceChainKey) *Chain2 {
    return newChain2(c, c.logger.L, c.store, c.key.Append("SyntheticSequenceChain", k.Partition), "synthetic-sequence(%[4]v)")
}
```

The chain name is formatted as `synthetic-sequence(partition)`, creating a separate synthetic sequence chain for each partition that it interacts with.

## Chain Naming in the Diagnostics Table

In the URL diagnostics table, you'll see chains named:

1. `synthetic-sequence-0`
2. `synthetic-sequence-1`
3. `synthetic-sequence-index`

These represent different synthetic sequence chains for different partitions. The `-0` and `-1` suffixes likely represent different partitions that the account interacts with.

## Healing Process

### Chains That Are Healed

During the healing process, only certain chains are healed:

1. **Synthetic Transaction Chains**: These chains store synthetic transactions and are healed to ensure cross-partition consistency.

2. **Synthetic Sequence Chains**: These chains track the sequence of synthetic transactions and are healed to ensure proper ordering.

### Chains That Are Not Healed

Some chains are checked for diagnostic purposes but are not actually healed:

1. **Root Chains**: These chains store the root state of the ledger and are not healed during the synthetic transaction healing process.

2. **Main Chains**: These chains store the main account data and are not directly related to synthetic transactions.

## URL Construction

The URL construction for synthetic chains follows this pattern:

1. **Base URL**: `acc://{partition-id}.acme`
   - Example: `acc://bvn-Apollo.acme`

2. **Synthetic Chain URL**: `acc://{partition-id}.acme/synthetic`
   - Example: `acc://bvn-Apollo.acme/synthetic`

3. **Root Chain URL**: `acc://{partition-id}.acme/ledger`
   - Example: `acc://bvn-Apollo.acme/ledger`

## Implications for Healing

When healing synthetic transactions, it's important to understand:

1. We only heal synthetic transaction chains, not root chains
2. Each partition pair (source → destination) has its own set of synthetic chains
3. The URL construction must be consistent between different parts of the code

The URL diagnostics table helps identify inconsistencies in URL construction that might cause healing to fail.

## Conclusion

Understanding the structure and purpose of synthetic chains is essential for implementing an effective healing process. By focusing on the chains that need to be healed and ensuring consistent URL construction, we can improve the reliability and efficiency of the healing process.
