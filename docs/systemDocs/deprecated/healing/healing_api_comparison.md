# Accumulate Healing APIs: Implementation and Comparison

## Introduction

This document provides a comprehensive analysis of the healing systems in Accumulate, including:

1. A comparison between the healing APIs documented in the previous markdown file and their actual implementation in the Accumulate codebase
2. How these APIs relate to sequence and network status APIs
3. The reporting systems used to monitor healing progress
4. A detailed outline of the healing problem and how the implementation addresses it

## Table of Contents

1. [The Healing Problem](#healing-problem)
2. [Healing APIs Overview](#healing-apis-overview)
3. [API Implementation Analysis](#api-implementation-analysis)
4. [Sequence API Comparison](#sequence-api-comparison)
5. [Network Status API Comparison](#network-status-api-comparison)
6. [Accumulate API Layers](#api-layers)
7. [Reporting System Analysis](#reporting-system-analysis)
8. [Database and Caching Infrastructure](#database-caching)
9. [Receipt Creation and Signature Gathering](#receipt-signature)
10. [Integration Points](#integration-points)
11. [Key Differences and Similarities](#key-differences-and-similarities)

## The Healing Problem <a name="healing-problem"></a>

Before diving into the APIs, it's important to understand the fundamental problem that healing solves in Accumulate.

### Problem Definition

In Accumulate's multi-chain architecture with a Directory Network (DN) and multiple Blockchain Validation Networks (BVNs), two types of data must be consistently synchronized across partitions:

1. **Synthetic Transactions**: Protocol-generated transactions that need to be delivered across partitions
2. **Anchors**: Cryptographic commitments that link blocks across different partitions

Due to network conditions, node failures, or other issues, these transactions or anchors may sometimes fail to be delivered, creating inconsistencies in the distributed ledger.

### Challenges

1. **Identifying Missing Data**: Determining which transactions or anchors are missing between partitions
2. **Efficient Discovery**: Using algorithms that can efficiently find missing data without excessive network traffic
3. **Versioning**: Handling different network protocol versions with appropriate healing strategies
4. **Validation**: Ensuring that healed transactions or anchors are cryptographically valid
5. **Monitoring**: Tracking the progress and success rate of healing operations

### Solution Approach

Accumulate's healing system addresses these challenges through:

1. **Binary Search Algorithm**: Efficiently identifies missing transactions by recursively dividing the search space
2. **Version-Specific Logic**: Implements different healing strategies based on the network protocol version
3. **Signature Collection**: For anchors, collects signatures from network nodes to ensure validity
4. **Detailed Reporting**: Provides comprehensive metrics and statistics about the healing process
5. **Partition Pair Tracking**: Monitors the status of each valid partition pair separately

## Healing APIs Overview <a name="healing-apis-overview"></a>

The healing APIs in Accumulate are primarily implemented in the `internal/core/healing` package and consist of two main components:

1. **Synthetic Transaction Healing API**
   - Primary function: `HealSynthetic`
   - Arguments structure: `HealSyntheticArgs`

2. **Anchor Healing API**
   - Primary function: `HealAnchor`
   - Arguments structure: `HealAnchorArgs`

Both APIs share common infrastructure and helper functions for resolving sequences, fetching transactions, and submitting healing operations.

## API Implementation Analysis <a name="api-implementation-analysis"></a>

### Synthetic Transaction Healing

#### API Definition vs. Implementation

**API Definition (from documentation):**
```go
func healSingleSynth(h *healer, source, destination string, number uint64, id *url.TxID, txns map[[32]byte]*protocol.Transaction) bool
```

**Actual Implementation:**
```go
func (h *Healer) HealSynthetic(ctx context.Context, args HealSyntheticArgs, si SequencedInfo) error {
    // Implementation details
}
```

The actual implementation is more comprehensive than what was documented. The `HealSynthetic` function:

1. Takes a context, arguments structure, and sequence information
2. Resolves the synthetic transaction using the client
3. Checks if the transaction has already been delivered
4. Builds a cryptographic receipt (using either V1 or V2 depending on the network version)
5. Submits the synthetic transaction directly to the destination partition

The implementation includes version-specific logic to handle different network protocol versions (pre-Baikonur, Baikonur, and Vandenberg).

#### Database Interactions

The synthetic transaction healing process interacts with databases in several ways:

1. **Light Client Database**:
   ```go
   batch := args.Light.OpenDB(false)
   defer batch.Discard()
   ```
   The healing process uses a light client database to access chain data without loading the entire blockchain. This database is opened in read-only mode and discarded after use.

2. **Chain Access**:
   ```go
   receipt, err := batch.Account(uSrcSynth).MainChain().Receipt(seqEntry.Source, mainIndex.Source)
   ```
   The process directly accesses various chains (main chain, root chain, anchor chain) to build receipts.

3. **Index Lookups**:
   ```go
   mainIndex, err := batch.Index().Account(uSrcSynth).Chain("main").SourceIndex().FindIndexEntryAfter(seqEntry.Source)
   ```
   The process uses index lookups to efficiently find entries in the chains.

#### Caching Mechanisms

The healing process implements sophisticated caching to improve performance:

1. **Transaction Cache**:
   ```go
   type txFetcher struct {
       Client message.AddressedClient
       Cache  *lru.Cache[string, *protocol.Transaction]
   }
   ```
   A least-recently-used (LRU) cache stores transactions to avoid repeated network requests.

2. **Cache Statistics**:
   ```go
   slog.InfoContext(h.ctx, "Healed transaction", 
       "source", source, "destination", destination, "number", number, "id", id,
       "submitted", h.txSubmitted, "delivered", h.txDelivered, 
       "hitRate", h.fetcher.Cache.HitRate())
   ```
   The process tracks and logs cache hit rates to monitor efficiency.

3. **In-Memory Transaction Map**:
   ```go
   txns map[[32]byte]*protocol.Transaction
   ```
   An in-memory map stores transactions by hash for quick lookup during the healing process.

#### Receipt Creation

The healing process creates cryptographic receipts to prove transaction validity:

1. **Version-Specific Receipt Building**:
   ```go
   var receipt *merkle.Receipt
   if args.NetInfo.Status.ExecutorVersion.V2VandenbergEnabled() {
       receipt, err = h.buildSynthReceiptV2(ctx, args, si)
   } else {
       receipt, err = h.buildSynthReceiptV1(ctx, args, si)
   }
   ```
   Different receipt building strategies are used based on the network version.

2. **V2 Receipt Building**:
   ```go
   func (h *Healer) buildSynthReceiptV2(_ context.Context, args HealSyntheticArgs, si SequencedInfo) (*merkle.Receipt, error) {
       // Load the synthetic sequence chain entry
       b, err := batch.Account(uSrcSynth).SyntheticSequenceChain(si.Destination).Entry(int64(si.Number) - 1)
       
       // Build the synthetic ledger part of the receipt
       receipt, err := batch.Account(uSrcSynth).MainChain().Receipt(seqEntry.Source, mainIndex.Source)
       
       // Build the BVN part of the receipt
       bvnReceipt, err := batch.Account(uSrcSys).RootChain().Receipt(mainIndex.Anchor, bvnRootIndex.Source)
       receipt, err = receipt.Combine(bvnReceipt)
       
       // Build the DN-BVN part of the receipt
       bvnDnReceipt, err := dnBvnAnchorChain.Receipt(uint64(bvnAnchorHeight), bvnAnchorIndex.Source)
       receipt, err = receipt.Combine(bvnDnReceipt)
       
       // Build the DN part of the receipt
       dnReceipt, err := batch.Account(uDnSys).RootChain().Receipt(bvnAnchorIndex.Anchor, dnRootIndex.Source)
       receipt, err = receipt.Combine(dnReceipt)
       
       return receipt, nil
   }
   ```
   The V2 receipt building process combines multiple receipts from different chains to create a complete proof.

### Anchor Healing

#### API Definition vs. Implementation

**API Definition (from documentation):**
```go
func (h *healer) healSingleAnchor(srcId, dstId string, seqNum uint64, txid *url.TxID, txns map[[32]byte]*protocol.Transaction) bool
```

**Actual Implementation:**
```go
func HealAnchor(ctx context.Context, args HealAnchorArgs, si SequencedInfo) error {
    // If the network is running Vandenberg and the anchor is from the DN to a
    // BVN, use version 2
    if args.NetInfo.Status.ExecutorVersion.V2VandenbergEnabled() &&
        strings.EqualFold(si.Source, protocol.Directory) &&
        !strings.EqualFold(si.Destination, protocol.Directory) {
        return healDnAnchorV2(ctx, args, si)
    }
    return healAnchorV1(ctx, args, si)
}
```

The actual implementation is more sophisticated than documented:

1. It has version-specific logic to handle different network protocol versions
2. For Vandenberg-enabled networks, it uses a specialized function for DN-to-BVN anchors
3. It includes fallback mechanisms if the DN self-anchor hasn't been delivered
4. It handles signature collection from network peers

#### Signature Gathering Process

The anchor healing process includes a sophisticated signature gathering mechanism:

1. **Tracking Existing Signatures**:
   ```go
   // Keep track of which validators have already signed
   signed := map[string]bool{}
   for _, sigs := range sigSets {
       for _, sig := range sigs.Signatures.Records {
           msg, ok := sig.Message.(*messaging.SignatureMessage)
           if !ok {
               continue
           }
           
           ks, ok := msg.Signature.(protocol.KeySignature)
           if !ok {
               continue
           }
           
           signed[ks.GetSigner().String()] = true
       }
   }
   ```
   The process tracks which validators have already signed to avoid duplicate signatures.

2. **Peer Enumeration**:
   ```go
   // Get a signature from each node that hasn't signed
   var gotPartSig bool
   var signatures []protocol.Signature
   for peer, info := range args.NetInfo.Peers[strings.ToLower(si.Source)] {
       if signed[info.Key] {
           continue
       }
       
       addr := multiaddr.StringCast("/p2p/" + peer.String())
       if len(info.Addresses) > 0 {
           addr = info.Addresses[0].Encapsulate(addr)
       }
       
       // Query the node for its signature
       // ...
   }
   ```
   The process enumerates peers in the network to collect signatures from nodes that haven't signed yet.

3. **Direct Node Queries**:
   ```go
   res, err := args.Client.ForAddress(addr).Private().Sequence(ctx, srcUrl.JoinPath(protocol.AnchorPool), dstUrl, si.Number, private.SequenceOptions{})
   ```
   The process directly queries individual nodes for their signatures using the private API.

4. **Signature Validation**:
   ```go
   // Filter out bad signatures
   if !sig.Verify(nil, seq) {
       slog.ErrorContext(ctx, "Node gave us an invalid signature", "id", info)
       continue
   }
   ```
   Each signature is cryptographically verified before being accepted.

5. **Signature Submission**:
   ```go
   if args.NetInfo.Status.ExecutorVersion.V2Enabled() {
       for _, sig := range signatures {
           blk := &messaging.BlockAnchor{
               Signature: sig.(protocol.KeySignature),
               Anchor:    seq,
           }
           err = args.Submit(blk)
       }
   } else {
       msg := []messaging.Message{
           &messaging.TransactionMessage{Transaction: theAnchorTxn},
       }
       for _, sig := range signatures {
           msg = append(msg, &messaging.SignatureMessage{Signature: sig})
       }
       err = args.Submit(msg...)
   }
   ```
   The collected signatures are submitted using different formats based on the network version.

### Common Infrastructure

Both APIs rely on shared infrastructure:

1. **SequencedInfo Structure**:
   ```go
   type SequencedInfo struct {
       Source      string
       Destination string
       Number      uint64
       ID          *url.TxID
   }
   ```

2. **NetworkInfo Structure**:
   ```go
   type NetworkInfo struct {
       Status *protocol.NetworkStatus
       Peers  map[string]map[peer.ID]*network.PeerInfo
   }
   ```

3. **ResolveSequenced Function**:
   A generic function to resolve sequenced messages (transactions or anchors) between partitions.

## Sequence API Comparison <a name="sequence-api-comparison"></a>

The healing APIs heavily rely on the sequence APIs to identify and resolve missing transactions or anchors.

### Sequence API

The Sequence API in Accumulate is primarily used to:

1. Retrieve information about sequenced messages between partitions
2. Check the delivery status of messages
3. Obtain signatures for messages

### Integration with Healing

The healing process uses the Sequence API to:

1. **Resolve Sequenced Messages**:
   ```go
   r, err := ResolveSequenced[messaging.Message](ctx, args.Client, args.NetInfo, si.Source, si.Destination, si.Number, false)
   ```

2. **Check Delivery Status**:
   ```go
   s, err := Q.QueryMessage(ctx, r.ID, nil)
   if err == nil && s.Status.Delivered() {
       // Message already delivered
   }
   ```

3. **Obtain Signatures**:
   ```go
   res, err := args.Client.ForAddress(addr).Private().Sequence(ctx, srcUrl.JoinPath(protocol.AnchorPool), dstUrl, si.Number, private.SequenceOptions{})
   ```

### Key Differences

1. **Purpose**:
   - Sequence API: Provides information about sequenced messages
   - Healing API: Uses sequence information to repair missing messages

2. **Scope**:
   - Sequence API: Read-only operations to query sequence state
   - Healing API: Write operations to fix inconsistencies

## Network Status API Comparison <a name="network-status-api-comparison"></a>

The Network Status API provides information about the current state of the Accumulate network, which is essential for the healing process.

### Network Status API

The Network Status API in Accumulate is used to:

1. Retrieve information about network partitions
2. Get the current protocol version and features
3. Obtain routing information for messages

### Integration with Healing

The healing process uses the Network Status API to:

1. **Determine Valid Partition Pairs**:
   ```go
   // Skip BVN to BVN pairs (not valid for anchoring)
   if src.Type != protocol.PartitionTypeDirectory && dst.Type != protocol.PartitionTypeDirectory {
       continue
   }
   ```

2. **Check Protocol Version**:
   ```go
   if args.NetInfo.Status.ExecutorVersion.V2VandenbergEnabled() {
       // Use Vandenberg-specific logic
   }
   ```

3. **Obtain Peer Information**:
   ```go
   for peer, info := range args.NetInfo.Peers[strings.ToLower(si.Source)] {
       // Use peer information for signature collection
   }
   ```

### Key Differences

1. **Purpose**:
   - Network Status API: Provides information about the network state
   - Healing API: Uses network information to guide the healing process

2. **Scope**:
   - Network Status API: Network-wide information
   - Healing API: Partition-specific operations

## Accumulate API Layers <a name="api-layers"></a>

The healing processes interact with multiple API layers in the Accumulate stack. Understanding these layers is crucial for comprehending how the healing processes work.

### Tendermint APIs

At the lowest level, Accumulate uses Tendermint for consensus and blockchain management. The healing processes interact with Tendermint APIs in several ways:

1. **ABCI (Application Blockchain Interface)**:
   ```go
   // Example of how Accumulate interacts with Tendermint ABCI
   func (n *Node) initApp(genesis []byte) (*abci.Client, error) {
       app := accumulate.New(n.logger, n.database, n.executor, n.config.Accumulate)
       client, err := abci.NewClient(n.logger, app, n.config.Tendermint.SocketPath())
       if err != nil {
           return nil, errors.UnknownError.WithFormat("create ABCI client: %w", err)
       }
       return client, nil
   }
   ```
   The ABCI interface is used for transaction submission and validation.

2. **RPC (Remote Procedure Call)**:
   ```go
   // Example of using Tendermint RPC to check transaction status
   resp, err := client.Tx(ctx, txHash, false)
   ```
   The RPC interface is used to query transaction status and blockchain state.

3. **P2P (Peer-to-Peer)**:
   ```go
   // Example of P2P communication for node discovery
   addr := multiaddr.StringCast("/p2p/" + peer.String())
   ```
   The P2P interface is used for node discovery and communication.

### Original Accumulate APIs (v1)

The original Accumulate APIs provided basic functionality for interacting with the blockchain:

1. **Transaction Submission**:
   ```go
   // Example of v1 API transaction submission
   resp, err := client.Execute(ctx, tx)
   ```

2. **Query Operations**:
   ```go
   // Example of v1 API query
   resp, err := client.Query(ctx, query)
   ```

3. **Limitations**:
   - Limited support for complex queries
   - No built-in support for pagination
   - No standardized error handling

### Accumulate v2 APIs

The v2 APIs introduced significant improvements:

1. **Enhanced Query Capabilities**:
   ```go
   // Example of v2 API query with options
   resp, err := client.QueryTransaction(ctx, txid, api.QueryOptions{
       IncludeReceipt: true,
       IncludeStatus: true,
   })
   ```

2. **Improved Transaction Submission**:
   ```go
   // Example of v2 API transaction submission with wait
   resp, err := client.Submit(ctx, envelope, api.SubmitOptions{
       Wait: true,
   })
   ```

3. **Structured Error Handling**:
   ```go
   if errors.Is(err, api.ErrNotFound) {
       // Handle not found error
   }
   ```

### Accumulate v3 APIs

The v3 APIs represent the current generation and are used extensively by the healing processes:

1. **Message-Based Architecture**:
   ```go
   // Example of v3 API message-based request
   resp, err := client.Request(ctx, &api.MessageRequest{
       Type: api.MessageTypeQuery,
       Payload: &api.QueryRequest{
           URL: url,
       },
   })
   ```

2. **Scope Property**:
   The scope property is a critical feature of v3 APIs that determines the context of an API call:
   
   ```go
   // Example of using scope in v3 API
   resp, err := client.QueryMessage(ctx, txid, api.QueryOptions{
       Scope: api.ScopeGlobal, // Query across all partitions
   })
   ```
   
   Scope values include:
   - **ScopeLocal**: Limits the query to the local partition
   - **ScopeNetwork**: Extends the query to the entire network the partition belongs to
   - **ScopeGlobal**: Extends the query to all known networks
   
   In the healing context, scope is used to control where transactions and anchors are queried from:
   
   ```go
   // Example from healing code using scope
   Q := api.Querier2{Querier: args.Querier}
   s, err := Q.QueryMessage(ctx, r.ID, &api.QueryOptions{
       Scope: api.ScopeNetwork, // Look across the network for the message
   })
   ```

3. **Addressed Client**:
   ```go
   // Example of using addressed client in v3 API
   client := message.AddressedClient{
       Client: client,
       Address: addr,
   }
   resp, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
   ```
   The addressed client allows directing requests to specific nodes, which is essential for signature collection.

4. **Streaming Support**:
   ```go
   // Example of v3 API streaming
   stream, err := client.Subscribe(ctx, api.SubscribeOptions{
       Events: []api.EventType{api.EventTypeTransaction},
   })
   for event := range stream.Events() {
       // Process event
   }
   ```
   Streaming allows the healing process to monitor events in real-time.

### API Usage in Healing Processes

The healing processes primarily use v3 APIs, but interact with all layers:

1. **Tendermint Layer**:
   - Used for low-level transaction verification
   - P2P communication for node discovery

2. **v1/v2 Compatibility**:
   - Some legacy code paths still use v1/v2 APIs
   - Version detection to handle different API versions

3. **v3 Primary Usage**:
   - Network status queries
   - Transaction and anchor queries
   - Signature collection
   - Transaction submission

4. **Scope Usage**:
   - **ScopeLocal**: Used when querying known partition-specific data
   - **ScopeNetwork**: Used for most healing operations to ensure network-wide consistency
   - **ScopeGlobal**: Rarely used in healing, primarily for cross-network operations

## Integration Points <a name="integration-points"></a>

The healing APIs integrate with the sequence and network status APIs at several key points:

1. **Initialization**:
   - The healing process begins by querying the network status to identify valid partition pairs
   - It uses the sequence API to determine which transactions or anchors need healing

2. **Transaction/Anchor Resolution**:
   - The healing process uses the sequence API to resolve the details of missing transactions or anchors

3. **Version-Specific Logic**:
   - The healing process checks the network protocol version from the network status to determine which healing algorithm to use

4. **Signature Collection**:
   - For anchors, the healing process uses peer information from the network status to collect signatures from network nodes

5. **Delivery Verification**:
   - After submitting healing operations, the process uses the sequence API to verify successful delivery

## Key Differences and Similarities <a name="key-differences-and-similarities"></a>

### Similarities

1. **Data Structures**:
   - All three APIs use similar data structures for representing partitions, transactions, and network state

2. **Context Awareness**:
   - All APIs are context-aware and accept a context.Context parameter for cancellation and timeout

3. **Error Handling**:
   - All APIs use similar error handling patterns, with specific error types for common conditions

### Differences

1. **Purpose**:
   - Sequence API: Information retrieval about sequenced messages
   - Network Status API: Information about network state
   - Healing API: Active repair of network inconsistencies

2. **Direction of Data Flow**:
   - Sequence and Network Status APIs: Primarily read operations
   - Healing API: Primarily write operations

3. **Complexity**:
   - Sequence API: Relatively simple query operations
   - Network Status API: Network-wide queries
   - Healing API: Complex operations involving multiple steps and fallback mechanisms

4. **Version Sensitivity**:
   - Healing API: Highly sensitive to network protocol version
   - Sequence API: Less version-dependent
   - Network Status API: Version-aware but primarily for reporting

## Reporting System Analysis <a name="reporting-system-analysis"></a>

The reporting system is a critical component of the healing process, providing visibility into the progress and effectiveness of healing operations.

### Reporting Implementation

#### Command-Line Interface

The reporting system is exposed through the command-line interface:

```bash
./tools/cmd/debug/debug heal synth mainnet --report-interval=1
./tools/cmd/debug/debug heal anchor mainnet --report-interval=1
```

#### Report Types

1. **Detailed Reports**: Comprehensive statistics about the healing process
2. **Periodic Reports**: Regular updates during long-running healing operations

#### Report Format

The reports use ASCII characters for maximum terminal compatibility:

```
+-----------------------------------------------------------+
|                DETAILED HEALING REPORT                    |
+-----------------------------------------------------------+
* Total Runtime: 1h0m0s

* DELIVERY STATUS:
   Initial Delivered: 10000
   Current Delivered: 10500
   Transactions Healed: 500

* PARTITION PAIR STATUS:
   Total Valid Anchor Partition Pairs: 6
   Up to Date: 2
   Needs Healing: 4
```

### Reporting vs. API Implementation

#### Data Sources

The reporting system draws data from multiple sources:

1. **Healer Structure**: Transaction counters and statistics
2. **Partition Pair Statistics**: Status of each valid partition pair
3. **Network Status**: Information about the network topology

#### Integration with Healing APIs

The reporting system is tightly integrated with the healing APIs:

1. **Initialization**: The reporting system initializes statistics for all valid partition pairs
2. **Progress Tracking**: The healing APIs update statistics as they process transactions or anchors
3. **Status Determination**: The reporting system uses the statistics to determine the status of each partition pair

#### Key Metrics

The reporting system tracks several key metrics:

1. **Transaction Counts**: Submitted, delivered, failed
2. **Healing Progress**: Initial delivered, current delivered, transactions healed
3. **Partition Pair Status**: Up to date vs. needs healing
4. **Delivery Gaps**: Difference between produced and delivered transactions

### Comparison with Documentation

The actual implementation of the reporting system is more sophisticated than what was documented:

1. **Partition Pair Filtering**: The implementation correctly filters for valid anchor partition pairs (DN→BVN and BVN→DN only)
2. **Timestamp Formatting**: Uses human-readable timestamp formats
3. **ASCII Formatting**: Uses simple ASCII characters for better terminal compatibility
4. **Comprehensive Metrics**: Tracks a wider range of metrics than documented

## Database and Caching Infrastructure <a name="database-caching"></a>

The healing processes in Accumulate rely on sophisticated database and caching mechanisms to efficiently identify and repair missing transactions or anchors.

### Database Architecture

#### Light Client Database

The healing processes use a light client database to access chain data without loading the entire blockchain:

```go
batch := args.Light.OpenDB(false)
defer batch.Discard()
```

This approach provides several benefits:

1. **Reduced Memory Footprint**: Only the necessary data is loaded into memory
2. **Faster Startup**: The healing process can start without waiting for a full node to sync
3. **Read-Only Access**: The database is opened in read-only mode to prevent accidental modifications

#### Chain Access Patterns

The healing processes access various chains to build receipts and verify transactions:

1. **Main Chain**: Contains the primary transaction data
   ```go
   receipt, err := batch.Account(uSrcSynth).MainChain().Receipt(seqEntry.Source, mainIndex.Source)
   ```

2. **Root Chain**: Contains the root hashes of the blockchain
   ```go
   bvnReceipt, err := batch.Account(uSrcSys).RootChain().Receipt(mainIndex.Anchor, bvnRootIndex.Source)
   ```

3. **Anchor Chain**: Contains the anchor data between partitions
   ```go
   bvnDnReceipt, err := dnBvnAnchorChain.Receipt(uint64(bvnAnchorHeight), bvnAnchorIndex.Source)
   ```

4. **Synthetic Sequence Chain**: Contains the sequence information for synthetic transactions
   ```go
   b, err := batch.Account(uSrcSynth).SyntheticSequenceChain(si.Destination).Entry(int64(si.Number) - 1)
   ```

### Caching Infrastructure

#### Transaction Cache

The healing processes implement a least-recently-used (LRU) cache to store transactions and avoid repeated network requests:

```go
type txFetcher struct {
    Client message.AddressedClient
    Cache  *lru.Cache[string, *protocol.Transaction]
}
```

This cache is used throughout the healing process:

```go
// Try to get the transaction from the cache first
txn, found := f.Cache.Get(id.String())
if found {
    return txn, nil
}

// If not in cache, fetch it from the network
// ...

// Add to cache for future use
f.Cache.Add(id.String(), txn)
```

#### In-Memory Transaction Map

In addition to the LRU cache, the healing processes use an in-memory map to store transactions by hash for quick lookup during the healing process:

```go
txns map[[32]byte]*protocol.Transaction
```

This map is passed to the healing functions to provide fast access to transaction data without requiring additional network requests.

### Cache Performance Monitoring

The healing processes track and log cache performance metrics:

```go
slog.InfoContext(h.ctx, "Healed transaction", 
    "source", source, "destination", destination, "number", number, "id", id,
    "submitted", h.txSubmitted, "delivered", h.txDelivered, 
    "hitRate", h.fetcher.Cache.HitRate())
```

These metrics help identify potential performance bottlenecks and optimize the caching strategy.

## Receipt Creation and Signature Gathering <a name="receipt-signature"></a>

### Cryptographic Receipt Creation

The healing processes create cryptographic receipts to prove transaction validity. These receipts are essential for the destination partition to verify the authenticity of the transaction or anchor.

#### Version-Specific Receipt Building

The receipt building process varies based on the network protocol version:

```go
var receipt *merkle.Receipt
if args.NetInfo.Status.ExecutorVersion.V2VandenbergEnabled() {
    receipt, err = h.buildSynthReceiptV2(ctx, args, si)
} else {
    receipt, err = h.buildSynthReceiptV1(ctx, args, si)
}
```

#### V2 Receipt Building Process

The V2 receipt building process is particularly sophisticated, combining multiple receipts from different chains to create a complete proof:

1. **Load Sequence Entry**:
   ```go
   b, err := batch.Account(uSrcSynth).SyntheticSequenceChain(si.Destination).Entry(int64(si.Number) - 1)
   ```

2. **Build Synthetic Ledger Receipt**:
   ```go
   receipt, err := batch.Account(uSrcSynth).MainChain().Receipt(seqEntry.Source, mainIndex.Source)
   ```

3. **Build BVN Receipt**:
   ```go
   bvnReceipt, err := batch.Account(uSrcSys).RootChain().Receipt(mainIndex.Anchor, bvnRootIndex.Source)
   receipt, err = receipt.Combine(bvnReceipt)
   ```

4. **Build DN-BVN Receipt**:
   ```go
   bvnDnReceipt, err := dnBvnAnchorChain.Receipt(uint64(bvnAnchorHeight), bvnAnchorIndex.Source)
   receipt, err = receipt.Combine(bvnDnReceipt)
   ```

5. **Build DN Receipt**:
   ```go
   dnReceipt, err := batch.Account(uDnSys).RootChain().Receipt(bvnAnchorIndex.Anchor, dnRootIndex.Source)
   receipt, err = receipt.Combine(dnReceipt)
   ```

This multi-step process creates a complete cryptographic proof that can be verified by the destination partition.

### Signature Gathering Process

The anchor healing process includes a sophisticated signature gathering mechanism to ensure the validity of healed anchors.

#### Tracking Existing Signatures

The process first checks which validators have already signed the anchor:

```go
// Keep track of which validators have already signed
signed := map[string]bool{}
for _, sigs := range sigSets {
    for _, sig := range sigs.Signatures.Records {
        msg, ok := sig.Message.(*messaging.SignatureMessage)
        if !ok {
            continue
        }
        
        ks, ok := msg.Signature.(protocol.KeySignature)
        if !ok {
            continue
        }
        
        signed[ks.GetSigner().String()] = true
    }
}
```

#### Peer Enumeration and Direct Node Queries

The process then enumerates peers in the network to collect signatures from nodes that haven't signed yet:

```go
// Get a signature from each node that hasn't signed
var gotPartSig bool
var signatures []protocol.Signature
for peer, info := range args.NetInfo.Peers[strings.ToLower(si.Source)] {
    if signed[info.Key] {
        continue
    }
    
    addr := multiaddr.StringCast("/p2p/" + peer.String())
    if len(info.Addresses) > 0 {
        addr = info.Addresses[0].Encapsulate(addr)
    }
    
    // Query the node for its signature
    res, err := args.Client.ForAddress(addr).Private().Sequence(ctx, srcUrl.JoinPath(protocol.AnchorPool), dstUrl, si.Number, private.SequenceOptions{})
    // ...
}
```

#### Signature Validation and Submission

Each signature is cryptographically verified before being accepted:

```go
// Filter out bad signatures
if !sig.Verify(nil, seq) {
    slog.ErrorContext(ctx, "Node gave us an invalid signature", "id", info)
    continue
}
```

The collected signatures are then submitted using different formats based on the network version:

```go
if args.NetInfo.Status.ExecutorVersion.V2Enabled() {
    for _, sig := range signatures {
        blk := &messaging.BlockAnchor{
            Signature: sig.(protocol.KeySignature),
            Anchor:    seq,
        }
        err = args.Submit(blk)
    }
} else {
    msg := []messaging.Message{
        &messaging.TransactionMessage{Transaction: theAnchorTxn},
    }
    for _, sig := range signatures {
        msg = append(msg, &messaging.SignatureMessage{Signature: sig})
    }
    err = args.Submit(msg...)
}
```

## Conclusion

The healing systems in Accumulate are sophisticated mechanisms that address the critical problem of maintaining data consistency across a distributed network of partitions. They leverage multiple API layers, database systems, and caching mechanisms to identify and repair inconsistencies in the distributed ledger, while providing comprehensive reporting on the healing progress.

Key aspects of the implementation include:

1. **Version-Specific Logic**: Different healing strategies based on the network protocol version
2. **Efficient Discovery Algorithms**: Binary search to identify missing transactions
3. **Sophisticated Database Interactions**: Light client database access patterns for efficient data retrieval
4. **Advanced Caching Mechanisms**: LRU cache and in-memory transaction maps for performance optimization
5. **Cryptographic Receipt Creation**: Multi-step process to create verifiable proofs
6. **Distributed Signature Gathering**: Collection and validation of signatures from network peers
7. **Multi-Layer API Integration**: Interaction with Tendermint, v1, v2, and v3 APIs
8. **Scope-Aware API Calls**: Strategic use of scope properties in v3 API calls
9. **Comprehensive Reporting**: Detailed metrics and statistics about the healing process
10. **Valid Partition Pair Tracking**: Monitoring only the valid partition pairs (DN→BVN and BVN→DN)
11. **Fallback Mechanisms**: Handling edge cases and error conditions

The integration between the healing processes and the various layers of the Accumulate stack demonstrates the sophisticated architecture of the system, where higher-level operations build upon lower-level services to provide robust network maintenance capabilities.

Understanding these relationships and implementation details is crucial for AI systems working with the Accumulate codebase, as it highlights the dependencies, interactions, and optimizations across different components of the system.
