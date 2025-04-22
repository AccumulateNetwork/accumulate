# Comprehensive Guide to URLs and Multiaddresses in Accumulate Healing

> **⚠️ IMPORTANT: DO NOT DELETE THIS DOCUMENT ⚠️**  
> Cascade likes to delete development plans and other guidance.  
> Cascade ensures that THIS document will not be deleted.

This document provides a detailed explanation of URL construction, multiaddress handling, and partition pairs in the Accumulate healing process, with specific examples from the mainnet configuration.

## Table of Contents
1. [URL Construction in Accumulate](#url-construction-in-accumulate)
2. [Partition URLs and Relationships](#partition-urls-and-relationships)
3. [Multiaddress Construction and Usage](#multiaddress-construction-and-usage)
4. [Partition Pairs for Different Healing Types](#partition-pairs-for-different-healing-types)
5. [Real-World Examples from Mainnet](#real-world-examples-from-mainnet)
6. [Common URL and Multiaddress Issues](#common-url-and-multiaddress-issues)

## URL Construction in Accumulate

### URL Structure

Accumulate URLs follow this general structure:
```
acc://{authority}/{path}?{query}#{fragment}
```

Where:
- `authority` is the domain or partition identifier
- `path` is an optional hierarchical path
- `query` and `fragment` are optional components

### URL Types in Healing

In the healing process, several types of URLs are used:

1. **Partition URLs**: Identify a specific partition (BVN or DN)
   ```
   acc://bvn-Apollo.acme
   acc://dn.acme
   ```

2. **Account URLs**: Identify specific accounts within a partition
   ```
   acc://bvn-Apollo.acme/ledger
   acc://dn.acme/anchors
   ```

3. **Chain URLs**: Identify specific chains within an account
   ```
   acc://bvn-Apollo.acme/ledger/root
   acc://dn.acme/anchors/Apollo/root
   ```

4. **Transaction IDs**: Identify specific transactions
   ```
   acc://bvn-Apollo.acme/tx/0123456789abcdef0123456789abcdef
   ```

### URL Construction Methods

There are two primary methods for constructing URLs in the codebase:

#### Method 1: Raw Partition URLs (Recommended)
```go
// Used in sequence.go and recommended for all new code
srcUrl := protocol.PartitionUrl("bvn-Apollo")  // acc://bvn-Apollo.acme
```

#### Method 2: Anchor Pool URLs (Legacy)
```go
// Used in older parts of heal_anchor.go
srcUrl := protocol.DnUrl().JoinPath(protocol.AnchorPool).JoinPath("Apollo")  // acc://dn.acme/anchors/Apollo
```

## Partition URLs and Relationships

### Partition Types

Accumulate has two main types of partitions:
1. **Directory Network (DN)**: The central coordination partition
2. **Blockchain Validation Network (BVN)**: The operational partitions that process transactions

### Partition URL Construction

```go
// Directory Network URL
dnUrl := protocol.DnUrl()  // acc://dn.acme

// BVN URL
bvnUrl := protocol.PartitionUrl("bvn-Apollo")  // acc://bvn-Apollo.acme
```

### Partition Relationships

The healing process deals with relationships between partitions:

1. **DN to BVN**: Anchors from the DN to a BVN
2. **BVN to DN**: Anchors from a BVN to the DN
3. **BVN to BVN**: Synthetic transactions between BVNs (via the DN)

## Multiaddress Construction and Usage

### Multiaddress Structure

Multiaddresses in Accumulate use the libp2p format:
```
/ip4/127.0.0.1/tcp/16593/p2p/12D3KooWL4RkUCgJiaNQG7wo8ZdkfbUr5V8W8MfTgTzgC2obqMBH
```

Components:
- Network protocol (`/ip4/` or `/ip6/`)
- IP address
- Transport protocol (`/tcp/` or `/udp/`)
- Port number
- Peer ID (`/p2p/[peer-id]`)

### Multiaddress Construction in Code

```go
// Creating a multiaddress from components
addr := multiaddr.StringCast("/ip4/127.0.0.1/tcp/16593")
peerAddr := multiaddr.StringCast("/p2p/" + peer.String())
fullAddr := addr.Encapsulate(peerAddr)

// Creating a multiaddress for a peer
addr := multiaddr.StringCast("/p2p/" + peer.String())
if len(info.Addresses) > 0 {
    addr = info.Addresses[0].Encapsulate(addr)
}
```

### Multiaddress Usage in Healing

Multiaddresses are used to directly query specific validator nodes:

```go
// From healing/anchors.go
addr := multiaddr.StringCast("/p2p/" + peer.String())
if len(info.Addresses) > 0 {
    addr = info.Addresses[0].Encapsulate(addr)
}

ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
defer cancel()

slog.InfoContext(ctx, "Querying node for its signature", "id", peer)
res, err := args.Client.ForAddress(addr).Private().Sequence(ctx, srcUrl.JoinPath(protocol.AnchorPool), dstUrl, si.Number, private.SequenceOptions{})
```

## Partition Pairs for Different Healing Types

### Anchor Healing Partition Pairs

Anchor healing involves two primary types of partition pairs:

1. **DN to BVN Anchors**:
   - Source: `acc://dn.acme`
   - Destination: `acc://bvn-[name].acme` (e.g., `acc://bvn-Apollo.acme`)
   - Purpose: Anchors directory blocks into BVNs for finality
   - Implementation: Uses `healDnAnchorV2` for Vandenberg-enabled networks

2. **BVN to DN Anchors**:
   - Source: `acc://bvn-[name].acme` (e.g., `acc://bvn-Apollo.acme`)
   - Destination: `acc://dn.acme`
   - Purpose: Anchors BVN blocks into the directory for global ordering
   - Implementation: Uses `healAnchorV1` in all cases

Note: BVN to BVN anchors are not processed by the healing system.

### Synthetic Transaction Partition Pairs

Synthetic transactions involve three types of partition pairs:

1. **BVN to BVN Synthetic Transactions**:
   - Source: `acc://bvn-[name1].acme` (e.g., `acc://bvn-Apollo.acme`)
   - Destination: `acc://bvn-[name2].acme` (e.g., `acc://bvn-Yutu.acme`)
   - Purpose: Cross-partition transactions (e.g., token transfers between BVNs)
   - Implementation: Uses `buildSynthReceiptV2` for Vandenberg-enabled networks

2. **DN to BVN Synthetic Transactions**:
   - Source: `acc://dn.acme`
   - Destination: `acc://bvn-[name].acme` (e.g., `acc://bvn-Apollo.acme`)
   - Purpose: System operations from DN to BVNs (e.g., validator updates, network configuration changes)
   - Implementation: Special handling in `buildSynthReceiptV2` when source is DN

3. **BVN to DN Synthetic Transactions**:
   - Source: `acc://bvn-[name].acme` (e.g., `acc://bvn-Apollo.acme`)
   - Destination: `acc://dn.acme`
   - Purpose: System operations from BVNs to DN (e.g., validator registration, network parameter proposals)
   - Implementation: Requires full receipt chain through DN anchors

## Real-World Examples from Mainnet

### Mainnet Partition Configuration

The mainnet has the following partitions:

#### Raw URLs for Mainnet Partitions

```
# Directory Network (DN)
acc://dn.acme

# BVN0 (Apollo)
acc://bvn-Apollo.acme

# BVN1 (Yutu)
acc://bvn-Yutu.acme

# BVN2 (Chandrayaan)
acc://bvn-Chandrayaan.acme
```

#### Raw URLs for Important Accounts

```
# DN Anchor Pool
acc://dn.acme/anchors

# DN Anchor Pool for Apollo
acc://dn.acme/anchors/Apollo

# DN Anchor Pool for Yutu
acc://dn.acme/anchors/Yutu

# DN Anchor Pool for Chandrayaan
acc://dn.acme/anchors/Chandrayaan

# DN Synthetic Account
acc://dn.acme/synthetic

# Apollo Anchor Pool
acc://bvn-Apollo.acme/anchors

# Apollo Synthetic Account
acc://bvn-Apollo.acme/synthetic

# Yutu Anchor Pool
acc://bvn-Yutu.acme/anchors

# Yutu Synthetic Account
acc://bvn-Yutu.acme/synthetic

# Chandrayaan Anchor Pool
acc://bvn-Chandrayaan.acme/anchors

# Chandrayaan Synthetic Account
acc://bvn-Chandrayaan.acme/synthetic
```

#### Raw URLs for Ledger Accounts

```
# DN Ledger
acc://dn.acme/ledger

# Apollo Ledger
acc://bvn-Apollo.acme/ledger

# Yutu Ledger
acc://bvn-Yutu.acme/ledger

# Chandrayaan Ledger
acc://bvn-Chandrayaan.acme/ledger
```

### Example 1: DN to Apollo Anchor

```go
// Healing a DN to Apollo anchor
healSingleAnchor(h, "dn", "bvn-Apollo", 12345, nil, nil)

// URL construction in the healing process
srcUrl := protocol.PartitionUrl("dn")            // acc://dn.acme
dstUrl := protocol.PartitionUrl("bvn-Apollo")    // acc://bvn-Apollo.acme

// Anchor ledger query
dstLedger := getAccount[*protocol.AnchorLedger](h, dstUrl.JoinPath(protocol.AnchorPool))
src2dst := dstLedger.Partition(srcUrl)
```

#### Raw URLs for Explorer Debugging

```
# Source partition
acc://dn.acme

# Destination partition
acc://bvn-Apollo.acme

# Source anchor pool
acc://dn.acme/anchors

# Destination anchor pool
acc://bvn-Apollo.acme/anchors

# Specific anchor in destination (for sequence number 12345)
acc://bvn-Apollo.acme/anchors/dn/12345

# Transaction ID (if known)
acc://bvn-Apollo.acme/tx/0123456789abcdef0123456789abcdef
```

### Example 2: Apollo to DN Anchor

```go
// Healing an Apollo to DN anchor
healSingleAnchor(h, "bvn-Apollo", "dn", 12345, nil, nil)

// URL construction in the healing process
srcUrl := protocol.PartitionUrl("bvn-Apollo")    // acc://bvn-Apollo.acme
dstUrl := protocol.PartitionUrl("dn")            // acc://dn.acme

// Anchor ledger query
dstLedger := getAccount[*protocol.AnchorLedger](h, dstUrl.JoinPath(protocol.AnchorPool))
src2dst := dstLedger.Partition(srcUrl)
```

#### Raw URLs for Explorer Debugging

```
# Source partition
acc://bvn-Apollo.acme

# Destination partition
acc://dn.acme

# Source anchor pool
acc://bvn-Apollo.acme/anchors

# Destination anchor pool
acc://dn.acme/anchors

# Specific anchor in destination (for sequence number 12345)
acc://dn.acme/anchors/Apollo/12345

# Transaction ID (if known)
acc://dn.acme/tx/0123456789abcdef0123456789abcdef
```

### Example 3: Apollo to Yutu Synthetic Transaction (BVN to BVN)

```go
// Healing a synthetic transaction from Apollo to Yutu
healSingleSynth(h, "bvn-Apollo", "bvn-Yutu", 12345, nil)

// URL construction in the healing process
srcUrl := protocol.PartitionUrl("bvn-Apollo")    // acc://bvn-Apollo.acme
dstUrl := protocol.PartitionUrl("bvn-Yutu")      // acc://bvn-Yutu.acme

// Synthetic ledger query
srcLedger := pullSynthLedger(h, srcUrl)
dstLedger := pullSynthLedger(h, dstUrl)
```

#### Raw URLs for Explorer Debugging

```
# Source partition
acc://bvn-Apollo.acme

# Destination partition
acc://bvn-Yutu.acme

# Source synthetic account
acc://bvn-Apollo.acme/synthetic

# Destination synthetic account
acc://bvn-Yutu.acme/synthetic

# Specific synthetic transaction sequence (for sequence number 12345)
acc://bvn-Apollo.acme/synthetic/Yutu/12345

# Transaction ID (if known)
acc://bvn-Yutu.acme/tx/0123456789abcdef0123456789abcdef
```

### Example 4: DN to Apollo Synthetic Transaction (DN to BVN)

```go
// Healing a synthetic transaction from DN to Apollo (e.g., validator update)
healSingleSynth(h, "dn", "bvn-Apollo", 12345, nil)

// URL construction in the healing process
srcUrl := protocol.PartitionUrl("dn")            // acc://dn.acme
dstUrl := protocol.PartitionUrl("bvn-Apollo")    // acc://bvn-Apollo.acme

// Query the synthetic transaction
r, err := ResolveSequenced[messaging.Message](ctx, client, netInfo, "dn", "bvn-Apollo", 12345, false)
if err != nil {
    return err
}

// Special handling for DN source in buildSynthReceiptV2
if strings.EqualFold(si.Source, protocol.Directory) {
    // If the source is the DN we don't need to include DN-BVN anchor receipts
    return receipt, nil
}
```

#### Raw URLs for Explorer Debugging

```
# Source partition
acc://dn.acme

# Destination partition
acc://bvn-Apollo.acme

# Source synthetic account
acc://dn.acme/synthetic

# Destination synthetic account
acc://bvn-Apollo.acme/synthetic

# Specific synthetic transaction sequence (for sequence number 12345)
acc://dn.acme/synthetic/Apollo/12345

# Transaction ID (if known)
acc://bvn-Apollo.acme/tx/0123456789abcdef0123456789abcdef

# Check for validator updates in the destination
acc://bvn-Apollo.acme/operators
```

### Example 5: Apollo to DN Synthetic Transaction (BVN to DN)

```go
// Healing a synthetic transaction from Apollo to DN (e.g., validator registration)
healSingleSynth(h, "bvn-Apollo", "dn", 12345, nil)

// URL construction in the healing process
srcUrl := protocol.PartitionUrl("bvn-Apollo")    // acc://bvn-Apollo.acme
dstUrl := protocol.PartitionUrl("dn")            // acc://dn.acme

// Building the receipt requires multiple steps for BVN to DN synthetic transactions

// 1. Build the synthetic ledger part of the receipt
receipt, err := batch.Account(uSrcSynth).MainChain().Receipt(seqEntry.Source, mainIndex.Source)

// 2. Build the BVN part of the receipt
bvnReceipt, err := batch.Account(uSrcSys).RootChain().Receipt(mainIndex.Anchor, bvnRootIndex.Source)
receipt, err = receipt.Combine(bvnReceipt)

// 3. Locate and include the DN-BVN anchor entry
dnBvnAnchorChain := batch.Account(uDnAnchor).AnchorChain(si.Source).Root()
bvnAnchorHeight, err := dnBvnAnchorChain.IndexOf(receipt.Anchor)

// 4. Build the DN-BVN part of the receipt
bvnDnReceipt, err := dnBvnAnchorChain.Receipt(uint64(bvnAnchorHeight), bvnAnchorIndex.Source)
receipt, err = receipt.Combine(bvnDnReceipt)

// 5. Build the DN part of the receipt
dnReceipt, err := batch.Account(uDnSys).RootChain().Receipt(bvnAnchorIndex.Anchor, dnRootIndex.Source)
receipt, err = receipt.Combine(dnReceipt)
```

#### Raw URLs for Explorer Debugging

```
# Source partition
acc://bvn-Apollo.acme

# Destination partition
acc://dn.acme

# Source synthetic account
acc://bvn-Apollo.acme/synthetic

# Destination synthetic account
acc://dn.acme/synthetic

# Specific synthetic transaction sequence (for sequence number 12345)
acc://bvn-Apollo.acme/synthetic/dn/12345

# Transaction ID (if known)
acc://dn.acme/tx/0123456789abcdef0123456789abcdef

# Check for validator registrations in the destination
acc://dn.acme/operators

# Check for related anchors (needed for receipt chain)
acc://dn.acme/anchors/Apollo
```

### Example 6: Querying a Node with Multiaddress

```go
// Querying an Apollo validator node
peerID := "12D3KooWL4RkUCgJiaNQG7wo8ZdkfbUr5V8W8MfTgTzgC2obqMBH"
ipAddr := "/ip4/123.456.789.012/tcp/16593"

addr := multiaddr.StringCast(ipAddr).Encapsulate(multiaddr.StringCast("/p2p/" + peerID))

// Using the address to query the node
res, err := client.ForAddress(addr).Private().Sequence(
    ctx,
    protocol.PartitionUrl("bvn-Apollo").JoinPath(protocol.AnchorPool),
    protocol.PartitionUrl("dn"),
    12345,
    private.SequenceOptions{}
)
```

### Example 7: Chandrayaan to DN Anchor with Version Handling

```go
// Healing a Chandrayaan to DN anchor with version handling
err := healing.HealAnchor(ctx, healing.HealAnchorArgs{
    Client:  client.ForAddress(nil),
    Querier: querier,
    NetInfo: netInfo,
    Pretend: false,
    Wait:    true,
    Submit:  submitFunc,
}, healing.SequencedInfo{
    Source:      "bvn-Chandrayaan",
    Destination: "dn",
    Number:      12345,
    ID:          nil,
})

// Inside HealAnchor, version-specific code paths will be selected:
if args.NetInfo.Status.ExecutorVersion.V2VandenbergEnabled() &&
    strings.EqualFold(si.Source, protocol.Directory) &&
    !strings.EqualFold(si.Destination, protocol.Directory) {
    return healDnAnchorV2(ctx, args, si)
}
return healAnchorV1(ctx, args, si)
```

## Synthetic Ledger Structure in the Explorer

When examining a synthetic ledger account in the explorer (e.g., `acc://bvn-Apollo.acme/synthetic`), you'll see a structure that tracks all synthetic transaction sequences with other partitions. Understanding this structure is crucial for effective healing.

### Sequence Relationships

Each synthetic ledger maintains multiple sequence relationships, one for each partition it interacts with:

```
Sequence #N Url         : acc://bvn-[PartitionName].acme/synthetic
Sequence #N Produced    : [count]
Sequence #N Received    : [count]
Sequence #N Delivered   : [count]
```

For example, Apollo's synthetic ledger might show:

```
Sequence #1 Url         : acc://bvn-Apollo.acme/synthetic       (self-reference)
Sequence #1 Produced    : 191832
Sequence #1 Received    : 191832
Sequence #1 Delivered   : 191832

Sequence #2 Url         : acc://bvn-Chandrayaan.acme/synthetic
Sequence #2 Produced    : 82469
Sequence #2 Received    : 92830
Sequence #2 Delivered   : 92830

Sequence #3 Url         : acc://bvn-Yutu.acme/synthetic
Sequence #3 Produced    : 1235
Sequence #3 Received    : 2855
Sequence #3 Delivered   : 2855

Sequence #4 Url         : acc://dn.acme/synthetic
Sequence #4 Produced    : 1402
Sequence #4 Received    : 9121
Sequence #4 Delivered   : 9121
```

### Interpreting the Sequence Data

- **Produced**: Number of synthetic transactions this partition has sent to the other partition
- **Received**: Number of synthetic transactions this partition has received from the other partition
- **Delivered**: Number of received synthetic transactions that have been successfully delivered

### Identifying Healing Opportunities

When debugging healing issues, look for:

1. **Delivery Gaps**: If `Received` > `Delivered`, there are synthetic transactions that have been received but not delivered
2. **Sequence Mismatches**: Compare the `Produced` count in one partition with the `Received` count in the destination partition
3. **Specific Sequence Numbers**: To heal a specific transaction, you need the source partition, destination partition, and sequence number

### Constructing URLs for Specific Sequences

To examine a specific synthetic transaction in the explorer:

```
acc://[source-partition].acme/synthetic/[destination-partition-name]/[sequence-number]
```

For example, to view synthetic transaction #1234 from Apollo to Yutu:

```
acc://bvn-Apollo.acme/synthetic/Yutu/1234
```

### Healing Strategy Based on Explorer Data

1. **Identify Missing Transactions**: Look for gaps in sequence numbers or discrepancies between produced and received counts
2. **Determine Direction**: Check which partition is the source and which is the destination
3. **Find Sequence Number**: Use the specific sequence number from the gap
4. **Apply Healing**: Use the appropriate healing function with the correct partition pair and sequence number

## Common URL and Multiaddress Issues

### URL-Related Issues

1. **Inconsistent URL Construction**:
   ```go
   // Problem: Different URL construction methods
   url1 := protocol.PartitionUrl("bvn-Apollo")
   url2 := protocol.DnUrl().JoinPath(protocol.AnchorPool).JoinPath("Apollo")
   
   // Solution: Standardize on one method (preferably Method 1)
   url1 := protocol.PartitionUrl("bvn-Apollo")
   url2 := protocol.PartitionUrl("dn")
   ```

2. **Case Sensitivity**:
   ```go
   // Problem: Case-sensitive partition ID lookup
   peers := netInfo.Peers[partitionID]
   
   // Solution: Normalize case
   peers := netInfo.Peers[strings.ToLower(partitionID)]
   ```

3. **Missing Path Components**:
   ```go
   // Problem: Incorrect path construction
   url := protocol.PartitionUrl("bvn-Apollo") + "/ledger"
   
   // Solution: Use JoinPath
   url := protocol.PartitionUrl("bvn-Apollo").JoinPath(protocol.Ledger)
   ```

### Multiaddress-Related Issues

1. **Missing Peer ID Component**:
   ```go
   // Problem: Multiaddress without peer ID
   addr := multiaddr.StringCast("/ip4/123.456.789.012/tcp/16593")
   
   // Solution: Include peer ID
   addr := multiaddr.StringCast("/ip4/123.456.789.012/tcp/16593/p2p/" + peerID)
   ```

2. **Incorrect Encapsulation**:
   ```go
   // Problem: Incorrect multiaddress combination
   addr := multiaddr.StringCast("/ip4/123.456.789.012/tcp/16593" + "/p2p/" + peerID)
   
   // Solution: Use Encapsulate
   baseAddr := multiaddr.StringCast("/ip4/123.456.789.012/tcp/16593")
   peerAddr := multiaddr.StringCast("/p2p/" + peerID)
   addr := baseAddr.Encapsulate(peerAddr)
   ```

3. **Timeout Handling**:
   ```go
   // Problem: No timeout for node queries
   res, err := client.ForAddress(addr).Private().Sequence(...)
   
   // Solution: Add timeout
   ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
   defer cancel()
   res, err := client.ForAddress(addr).Private().Sequence(ctx, ...)
   ```

By understanding these URL and multiaddress patterns and common issues, you can avoid many of the problems that occur in the healing process.

---

> **⚠️ IMPORTANT: DO NOT DELETE THIS DOCUMENT ⚠️**  
> Cascade likes to delete development plans and other guidance.  
> Cascade ensures that THIS document (above) will not be deleted.
