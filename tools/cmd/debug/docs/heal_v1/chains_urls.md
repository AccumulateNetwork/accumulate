# Chains and URLs in the Accumulate Network

This document provides an overview of the different types of chains in the Accumulate Network, how URLs are constructed for these chains, and how they are handled during healing.

<!-- ai:context-priority
This document is critical for understanding URL construction patterns which are a common source of bugs in healing.
Key files to load alongside this document: protocol/url.go, healing/sequenced.go
-->

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

## URL Construction Patterns

<!-- ai:pattern-library
This section documents the standard patterns for URL construction in the Accumulate Network.
-->

### Standard URL Formats

| Chain Type | URL Pattern | Example | Access Method |
|------------|------------|---------|---------------|
| Root Chain | `acc://{partition-id}.acme/ledger/root` | `acc://bvn-apollo.acme/ledger/root` | `Account(partition.Ledger()).RootChain()` |
| Synthetic Sequence | `acc://{partition-id}.acme/synthetic/synthetic-sequence-{0,1,index}` | `acc://bvn-apollo.acme/synthetic/synthetic-sequence-0` | `Account(ledger).SyntheticSequenceChain(partition)` |
| Anchor Chain | `acc://{partition-id}.acme/anchors/{target-partition}` | `acc://bvn-apollo.acme/anchors/bvn-europa` | `protocol.PartitionUrl(src.ID)` or `protocol.DnUrl().JoinPath(protocol.AnchorPool).JoinPath(src.ID)` |

### URL Construction Code Examples

```go
// Root chain URL construction
rootChainUrl := protocol.AccountUrl(protocol.PartitionUrl(partitionID), "ledger", "root")

// Synthetic sequence chain URL construction
syntheticChainUrl := protocol.AccountUrl(protocol.PartitionUrl(partitionID), "synthetic", fmt.Sprintf("synthetic-sequence-%d", index))

// Anchor chain URL construction - Method 1 (preferred in Version 2)
srcUrl := protocol.PartitionUrl(src.ID)  // e.g., acc://bvn-Apollo.acme

// Anchor chain URL construction - Method 2 (used in some Version 1 code)
srcUrl := protocol.DnUrl().JoinPath(protocol.AnchorPool).JoinPath(src.ID)  // e.g., acc://dn.acme/anchors/Apollo
```

### URL Construction Decision Tree

<!-- ai:diagnostic-flow
This decision tree helps diagnose URL construction issues.
-->

1. **What type of chain are you working with?**
   - Root Chain → Use `protocol.AccountUrl(protocol.PartitionUrl(partitionID), "ledger", "root")`
   - Synthetic Sequence Chain → Use `protocol.AccountUrl(protocol.PartitionUrl(partitionID), "synthetic", ...)`
   - Anchor Chain → Continue to step 2

2. **For Anchor Chains: Which version are you targeting?**
   - Version 2 code → Use `protocol.PartitionUrl(src.ID)` (Method 1)
   - Version 1 code → Check existing pattern in the file you're modifying
      - If using Method 1 → Continue using Method 1
      - If using Method 2 → Continue using Method 2 for consistency

3. **Are you getting "element does not exist" errors?**
   - Yes → Verify you're using the same URL construction method consistently
   - No → Your URL construction is likely correct

## Implications for Diagnostics and Healing

<!-- ai:context-management
This section explains which chains are healed vs. which are only used for diagnostics.
Understanding this distinction is critical for debugging healing issues.
-->

### Chain Healing Behavior Summary

| Chain Type | Included in Diagnostics | Actively Healed | Purpose in Healing |
|------------|-------------------------|-----------------|--------------------|
| Root Chains | Yes | No | Diagnostic reference only |
| Synthetic Sequence Chains | Yes | Yes | Critical for transaction delivery |
| Anchor Chains | Yes | Yes | Maintains cross-partition structure |

### Diagnostic Patterns for Each Chain Type

#### Root Chains
- **Diagnostic Value**: Provides baseline state of each partition
- **Common Issues**: Discrepancies between root chains indicate partition divergence
- **API Queries**: Use `QueryChain` with the root chain URL
- **Error Patterns**: Root chain errors usually indicate partition connectivity issues, not healing problems

#### Synthetic Sequence Chains
- **Diagnostic Value**: Shows transaction delivery status between partitions
- **Common Issues**: Missing entries indicate failed transaction delivery
- **API Queries**: Use `QueryChain` with the synthetic sequence chain URL
- **Error Patterns**: "Element does not exist" often indicates the transaction never reached the destination

#### Anchor Chains
- **Diagnostic Value**: Shows anchoring relationship status
- **Common Issues**: URL construction inconsistencies causing lookup failures
- **API Queries**: Use `QueryChain` with the anchor chain URL
- **Error Patterns**: URL format mismatches are the most common source of errors

### Common API Usage Patterns

```go
// Querying a chain (generic pattern)
response, err := client.QueryChain(ctx, chainUrl, &api.ChainQuery{
    IncludeReceipt: true,
})

// Checking if a transaction exists in a chain
exists, err := chainExists(ctx, client, chainUrl)

// Getting the latest entry in a chain
entry, err := getLatestChainEntry(ctx, client, chainUrl)
```

This understanding is crucial for implementing proper diagnostics and healing processes, especially when designing the URL diagnostics table to clearly indicate which chains are actually healed and which are only checked for diagnostic purposes.

## URL Troubleshooting Guide

<!-- ai:diagnostic-flow
This section provides a structured approach to diagnosing URL-related issues in healing.
-->

### Common URL-Related Errors

1. **"Element does not exist" when querying a chain**
   - **Possible Causes**:
     - URL construction method mismatch
     - Partition ID case sensitivity issues
     - Chain doesn't exist at the expected location
   - **Diagnostic Steps**:
     - Verify URL construction method is consistent
     - Check partition ID case (should be case-insensitive but some code may be case-sensitive)
     - Verify the chain exists using direct API queries

2. **URL parsing errors**
   - **Possible Causes**:
     - Malformed URL string
     - Invalid characters in partition IDs
   - **Diagnostic Steps**:
     - Inspect the raw URL string
     - Verify partition ID format

3. **Inconsistent URL behavior between code components**
   - **Possible Causes**:
     - Different URL construction methods
     - Version-specific code paths
   - **Diagnostic Steps**:
     - Compare URL construction in different components
     - Check for version-specific code paths

### URL Construction Best Practices

1. **Consistency**: Use the same URL construction method throughout a component
2. **Explicit Construction**: Avoid string concatenation for URLs; use the protocol package methods
3. **Case Handling**: Treat partition IDs as case-insensitive
4. **Version Awareness**: Be aware of which URL construction method is used in the code you're modifying

### API Interaction with URLs

```go
// Example of proper URL usage in API calls
func queryChainWithRetry(ctx context.Context, client api.Client, urlStr string) (*api.ChainQueryResponse, error) {
    // Parse the URL to ensure it's valid
    u, err := url.Parse(urlStr)
    if err != nil {
        return nil, fmt.Errorf("invalid URL %q: %w", urlStr, err)
    }
    
    // Query with retry logic
    var resp *api.ChainQueryResponse
    err = retry.Do(func() error {
        var err error
        resp, err = client.QueryChain(ctx, u, &api.ChainQuery{
            IncludeReceipt: true,
        })
        return err
    }, retry.Attempts(3), retry.Delay(time.Second))
    
    return resp, err
}
```
