---
title: Anchor Proofs and Receipts in Accumulate
description: Detailed explanation of how Accumulate creates proofs and generates receipts between networks based on anchors
tags: [accumulate, anchoring, proofs, receipts, merkle, cross-network, validation]
created: 2025-05-16
version: 1.0
---

# Anchor Proofs and Receipts in Accumulate

## Introduction

Accumulate's cross-network validation system relies on a sophisticated anchoring process that creates cryptographic proofs and receipts between different networks. This document explains the implementation details of how Accumulate creates these proofs and receipts, focusing on the underlying mechanisms and data structures.

## Anchoring Architecture Overview

Accumulate's anchoring system follows a hierarchical structure:

1. **Block Validator Networks (BVNs)** anchor their state to the **Directory Network (DN)**
2. **Directory Network** processes these anchors and returns receipts to the BVNs
3. **Receipts** contain Merkle proofs that cryptographically validate the anchoring process

This bidirectional anchoring process creates a verifiable chain of trust between different networks within the Accumulate ecosystem.

## Anchor Types and Data Structures

Accumulate implements two primary types of anchors:

### 1. Partition (BVN) Anchors

Partition anchors are sent from BVNs to the DN and contain the state of the BVN:

```go
// BlockValidatorAnchor represents an anchor from a BVN to the DN
type BlockValidatorAnchor struct {
    // Common fields
    PartitionAnchor
    
    // BVN-specific fields
    StateTreeAnchor []byte
    AcmeBurnt       *big.Int
}

// PartitionAnchor contains common anchor fields
type PartitionAnchor struct {
    Source          *url.URL
    RootChainIndex  uint64
    RootChainAnchor [32]byte
    MinorBlockIndex uint64
    MajorBlockIndex uint64
    Timestamp       time.Time
}
```

### 2. Directory Anchors

Directory anchors are sent from the DN to BVNs and contain receipts proving the inclusion of BVN anchors:

```go
// DirectoryAnchor represents an anchor from the DN to a BVN
type DirectoryAnchor struct {
    // Common fields
    PartitionAnchor
    
    // DN-specific fields
    Receipts        []*PartitionAnchorReceipt
    Updates         []NetworkAccountUpdate
    MakeMajorBlock  uint64
    MakeMajorBlockTime time.Time
}

// PartitionAnchorReceipt contains the proof that a BVN's anchor was included in the DN
type PartitionAnchorReceipt struct {
    Anchor            *PartitionAnchor
    RootChainReceipt  *merkle.Receipt
}
```

## Proof Generation Process

The process of generating proofs and receipts involves several key steps:

### 1. BVN Anchor Generation

When a BVN creates an anchor to send to the DN, it includes:

- The root hash of its state tree (`StateTreeAnchor`)
- The latest entry in its root chain (`RootChainAnchor`)
- Block indices and timestamps

```go
// generateAnchor generates an anchor for the current block
func (x *Executor) generateAnchor(batch *database.Batch) (*protocol.PartitionAnchor, error) {
    // Get the current block index
    blockIndex, err := batch.GetBptRootIndex()
    if err != nil {
        return nil, err
    }
    
    // Get the root hash
    rootHash, err := batch.GetBptRootHash(blockIndex)
    if err != nil {
        return nil, err
    }
    
    // Create the anchor
    anchor := &protocol.PartitionAnchor{
        Source:          x.Describe.PartitionId,
        RootChainIndex:  blockIndex,
        RootChainAnchor: rootHash,
        MinorBlockIndex: x.blockIndex,
        MajorBlockIndex: x.majorBlockIndex,
        Timestamp:       time.Now().UTC(),
    }
    
    return anchor, nil
}
```

### 2. DN Anchor Processing

When the DN receives a BVN anchor, it:

1. Validates the anchor
2. Adds the anchor to its chain
3. Generates a receipt proving inclusion

```go
// Execute processes a partition anchor in the DN
func (x PartitionAnchor) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
    body, err := x.check(st, tx)
    if err != nil {
        return nil, err
    }

    // Add the anchor to the chain - use the partition name as the chain name
    record := st.batch.Account(st.OriginUrl).AnchorChain(name)
    index, err := st.State.ChainUpdates.AddChainEntry2(st.batch, record.Root(), body.RootChainAnchor[:], body.RootChainIndex, body.MinorBlockIndex, false)
    if err != nil {
        return nil, err
    }
    st.State.DidReceiveAnchor(name, body, index)

    // And the BPT root
    _, err = st.State.ChainUpdates.AddChainEntry2(st.batch, record.BPT(), body.StateTreeAnchor[:], 0, 0, false)
    if err != nil {
        return nil, err
    }

    // Additional processing...
    
    return nil, nil
}
```

### 3. Receipt Generation

The DN generates receipts for each BVN anchor it processes. These receipts contain Merkle proofs that verify the inclusion of the BVN's anchor in the DN's state:

```go
// generateReceipt generates a receipt for a partition anchor
func (x *Executor) generateReceipt(ctx context.Context, anchor *protocol.PartitionAnchor, batch *database.Batch) (*protocol.PartitionAnchorReceipt, error) {
    // Get the root chain receipt
    rootChainReceipt, err := batch.Account(x.Describe.PartitionId).RootChain().Receipt(anchor.RootChainAnchor[:], x.rootChainIndex)
    if err != nil {
        return nil, err
    }
    
    // Create the receipt
    receipt := &protocol.PartitionAnchorReceipt{
        Anchor:           anchor,
        RootChainReceipt: rootChainReceipt,
    }
    
    return receipt, nil
}
```

### 4. Directory Anchor Creation

The DN periodically creates directory anchors that include receipts for all BVN anchors it has processed:

```go
// createDirectoryAnchor creates a directory anchor with receipts
func (x *Executor) createDirectoryAnchor(ctx context.Context, batch *database.Batch) (*protocol.DirectoryAnchor, error) {
    // Generate the base anchor
    partAnchor, err := x.generateAnchor(batch)
    if err != nil {
        return nil, err
    }
    
    // Create the directory anchor
    dirAnchor := &protocol.DirectoryAnchor{
        PartitionAnchor: *partAnchor,
        Receipts:        make([]*protocol.PartitionAnchorReceipt, 0, len(x.pendingReceipts)),
        Updates:         x.pendingUpdates,
    }
    
    // Add all pending receipts
    for _, receipt := range x.pendingReceipts {
        dirAnchor.Receipts = append(dirAnchor.Receipts, receipt)
    }
    
    // Clear pending receipts and updates
    x.pendingReceipts = nil
    x.pendingUpdates = nil
    
    return dirAnchor, nil
}
```

### 5. BVN Receipt Processing

When a BVN receives a directory anchor, it processes the receipts to verify its anchors were included in the DN:

```go
// Execute processes a directory anchor in a BVN
func (x DirectoryAnchor) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
    body, err := x.check(st, tx)
    if err != nil {
        return nil, err
    }

    // Add the anchor to the chain
    record := st.batch.Account(st.OriginUrl).AnchorChain(protocol.Directory)
    index, err := st.State.ChainUpdates.AddChainEntry2(st.batch, record.Root(), body.RootChainAnchor[:], body.RootChainIndex, body.MinorBlockIndex, false)
    if err != nil {
        return nil, err
    }
    st.State.DidReceiveAnchor(protocol.Directory, body, index)

    // Process receipts
    for _, receipt := range body.Receipts {
        st.logger.Info("Received receipt", "module", "anchoring", "for", receipt.Anchor.Source, "from", logging.AsHex(receipt.RootChainReceipt.Start).Slice(0, 4), "to", logging.AsHex(body.RootChainAnchor).Slice(0, 4), "source-block", body.MinorBlockIndex, "source", body.Source)
    }

    return nil, nil
}
```

## Merkle Receipt Implementation

The core of Accumulate's proof system is the Merkle receipt, which provides cryptographic proof of inclusion:

### Receipt Structure

```go
// Receipt represents a Merkle proof
type Receipt struct {
    Start      []byte         // The starting hash (typically a leaf)
    StartIndex int64          // The index of the starting hash
    Anchor     []byte         // The ending hash (typically a root)
    EndIndex   int64          // The index of the ending hash
    Entries    []*ReceiptEntry // The proof path
}

// ReceiptEntry represents a single node in the proof path
type ReceiptEntry struct {
    Hash  []byte // The hash value
    Right bool   // Whether this hash is applied from the right
}
```

### Receipt Validation

Receipts are validated by applying each entry in the proof path to the starting hash:

```go
// Validate takes a receipt and validates that the element hash progresses to the
// Merkle Dag Root hash (MDRoot) in the receipt
func (r *Receipt) Validate(opts *ValidateOptions) bool {
    MDRoot := r.Start // To begin with, we start with the object as the MDRoot
    // Now apply all the path hashes to the MDRoot
    for _, node := range r.Entries {
        if !opts.allowEntry(node) {
            return false
        }
        MDRoot = node.apply(MDRoot)
    }
    // In the end, MDRoot should be the same hash the receipt expects.
    return bytes.Equal(MDRoot, r.Anchor)
}

// apply applies a hash to the current state
func (n *ReceiptEntry) apply(hash []byte) []byte {
    if n.Right {
        // If this hash comes from the right, apply it that way
        return combineHashes(hash, n.Hash)
    }
    // If this hash comes from the left, apply it that way
    return combineHashes(n.Hash, hash)
}
```

### Receipt Combination

A key feature of Accumulate's receipt system is the ability to combine receipts, creating a chain of proofs:

```go
// Combine takes a 2nd receipt, attaches it to a root receipt, and returns the resulting
// receipt. The idea is that if this receipt is anchored into another chain,
// then we can create a receipt that proves the element in this receipt all
// the way down to an anchor in the root receipt.
func (r *Receipt) Combine(receipts ...*Receipt) (*Receipt, error) {
    for _, s := range receipts {
        if !bytes.Equal(r.Anchor, s.Start) {
            return nil, fmt.Errorf("receipts cannot be combined: anchor %x doesn't match root merkle tree %x", r.Anchor, s.Start)
        }
        nr := r.Copy()                // Make a copy of the first Receipt
        nr.Anchor = s.Anchor          // The MDRoot will be the one from the appended receipt
        for _, n := range s.Entries { // Make a copy and append the Nodes of the appended receipt
            nr.Entries = append(nr.Entries, n.Copy())
        }
        r = nr
    }
    return r, nil
}
```

## Validator Coordination and Transmission APIs

The anchor transmission process in Accumulate involves specific validator coordination and uses dedicated APIs for cross-network communication.

### Validator Coordination

Not every validator submits anchors to other networks. Anchor submission is coordinated through a designated process:

1. **Leader-Based Submission**:
   - Only the leader validator (determined by CometBFT's consensus mechanism) is responsible for producing the block
   - The leader's `Conductor` component triggers the anchor submission process during the `willBeginBlock` hook

```go
// From conductor.go
func (c *Conductor) sendAnchorForLastBlock(e execute.WillBeginBlock, batch *database.Batch) error {
    // Only one validator (the leader) will actually send the anchor
    // Others will skip this due to the ledger index check
    if ledger.Index < e.Index-1 {
        slog.DebugContext(e.Context, "Skipping anchor", "module", "conductor", "index", ledger.Index)
        return nil
    }
    // ...
}
```

2. **Single Submitter Per Partition**:
   - The `Conductor` component in each partition is responsible for sending anchors
   - Only one instance actively submits anchors at a time (the leader's instance)

### APIs and Protocols Used

Anchor transmission uses specific APIs and protocols:

1. **Anchor Preparation API**:
   - `PrepareAnchorSubmission` method prepares an anchor for transmission:

```go
func (x ValidatorContext) PrepareAnchorSubmission(ctx context.Context, anchor protocol.AnchorBody, 
    sequenceNumber uint64, destination *url.URL) (*messaging.Envelope, *protocol.Transaction, error) {
    // Create the transaction
    txn := new(protocol.Transaction)
    txn.Header.Principal = destination.JoinPath(protocol.AnchorPool)
    txn.Body = anchor

    // Create a sequenced message
    seq := &messaging.SequencedMessage{
        Message:     &messaging.TransactionMessage{Transaction: txn},
        Source:      x.Url(),
        Destination: destination,
        Number:      sequenceNumber,
    }

    // Sign the message
    h := seq.Hash()
    keySig, err := x.signTransaction(h[:])
    
    // Create the envelope
    return &messaging.Envelope{
        Messages: []messaging.Message{
            &messaging.BlockAnchor{
                Anchor:    seq,
                Signature: keySig,
            },
        },
    }, txn, nil
}
```

2. **Transmission Protocol**:
   - Anchors are transmitted using CometBFT's RPC interface, not the P2P interface directly
   - The `Submitter` component handles the actual transmission:

```go
func (s *Submitter) Submit(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
    // Serialize the envelope
    b, err := envelope.MarshalBinary()
    if err != nil {
        return nil, errors.EncodingError.WithFormat("marshal: %w", err)
    }

    // Submit via RPC
    var res *coretypes.ResultBroadcastTx
    if opts.Wait == nil || *opts.Wait {
        res, err = s.local.BroadcastTxSync(ctx, b)  // Synchronous RPC call
    } else {
        res, err = s.local.BroadcastTxAsync(ctx, b) // Asynchronous RPC call
    }
    // ...
}
```

### Client Discovery and Connection Management

The system uses a sophisticated peer discovery mechanism to find and connect to validators in other networks:

```go
// getClients returns a map of Tendermint RPC clients for the given partitions
func (d *dispatcher) getClients(ctx context.Context, want map[string]bool) map[string]peerClient {
    // First try local clients (same node)
    clients := make(map[string]peerClient, len(want))
    for part, client := range d.self {
        clients[part] = peerClient{DispatcherClient: client}
        delete(want, part)
    }
    
    // For partitions without local clients, discover remote peers
    if len(want) > 0 {
        // Walk the directory network to find peers for other partitions
        WalkPeers(ctx, d.self["directory"], func(ctx context.Context, peer coretypes.Peer) (WalkClient, bool) {
            // Create a client for the BVN
            bvn, err := NewHTTPClientForPeer(peer, config.PortOffsetBlockValidator-config.PortOffsetDirectory)
            
            // Check which BVN it's on
            st, err := bvn.Status(ctx)
            part := st.NodeInfo.Network
            
            // If we want this partition, add it to our clients
            if want[part] {
                delete(want, part)
                clients[part] = peerClient{bvn, &peer}
            }
            // ...
        })
    }
    
    return clients
}
```

## Cross-Network Proof Flow

The complete flow of proofs between networks follows these steps:

### 1. BVN to DN Anchoring

1. BVN creates a block and updates its state
2. BVN generates an anchor containing its state root
3. BVN sends the anchor to the DN via a `BlockValidatorAnchor` transaction

### 2. DN Processing

1. DN validates the BVN anchor
2. DN adds the anchor to its chain
3. DN generates a receipt proving inclusion
4. DN stores the receipt for later inclusion in a directory anchor

### 3. DN to BVN Anchoring

1. DN creates a directory anchor containing receipts for BVN anchors
2. DN leader validator prepares the directory anchor for transmission:
   - Wraps the anchor in a transaction
   - Signs the transaction with the validator key
   - Wraps the transaction in a messaging envelope
3. DN leader sends the directory anchor to each BVN using CometBFT's RPC interface
4. BVNs receive the directory anchor through their RPC endpoints
5. BVNs validate and store the directory anchor
6. BVNs process the receipts to verify their anchors were included in the DN

## Reliability Challenges and Healing Mechanisms

Cross-network communication in Accumulate faces several reliability challenges that necessitate healing mechanisms for both anchors and synthetic transactions.

### Reliability Challenges

The transmission of anchors and synthetic transactions between networks faces several inherent challenges:

1. **Single Point of Failure**:
   - Only the leader validator submits anchors and synthetic transactions
   - If the leader fails during submission, the transaction won't be sent
   - Leader changes during the submission process can cause missed transmissions

2. **Network Partition Issues**:
   - CometBFT's RPC interface relies on HTTP/HTTPS
   - Network partitions or temporary connectivity issues can cause transmission failures
   - No built-in retry mechanism in the base transmission protocol

3. **Peer Discovery Limitations**:
   - The peer discovery mechanism might not always find an available validator in the target network
   - The `getClients` function might return an empty map for some partitions

### Error Handling and Recovery

Accumulate implements sophisticated error handling for cross-network communication:

1. **Error Classification**:
   - The system distinguishes between transient and permanent errors
   - Certain errors are deliberately ignored to prevent unnecessary healing

```go
// CheckDispatchError ignores errors we don't care about.
func CheckDispatchError(err error) error {
    if err == nil {
        return nil
    }

    // Is the error "tx already exists in cache"?
    if err.Error() == mempool.ErrTxInCache.Error() {
        return nil
    }

    // Or RPC error "tx already exists in cache"?
    var rpcErr1 *jrpc.RPCError
    if errors.As(err, &rpcErr1) && *rpcErr1 == *errTxInCache1 {
        return nil
    }

    var rpcErr2 jsonrpc2.Error
    if errors.As(err, &rpcErr2) && (rpcErr2 == errTxInCache2 || rpcErr2 == errTxInCacheAcc) {
        return nil
    }

    var errorsErr *errors.Error
    if errors.As(err, &errorsErr) {
        // This probably should not be necessary
        if errorsErr.Code == errors.Delivered {
            return nil
        }
    }

    // It's a real error
    return err
}
```

2. **Panic Recovery**:
   - The dispatcher implements panic recovery to prevent cascading failures
   - Panics are logged and tracked via metrics

```go
func (d *dispatcher) send(ctx context.Context, queue map[string][]*messaging.Envelope, errs chan<- error) {
    defer func() {
        if r := recover(); r != nil {
            slog.ErrorContext(ctx, "Panicked while dispatching", "error", r)
            mDispatchPanics.Inc()
        }
    }()
    // ...
}
```

3. **Delivery Verification**:
   - Before resubmitting transactions, the system verifies if they've already been delivered
   - This prevents duplicate processing and unnecessary network traffic

```go
// Query the status
Q := api.Querier2{Querier: args.Querier}
if s, err := Q.QueryMessage(ctx, r.ID, nil); err == nil &&
    // Has it already been delivered?
    s.Status.Delivered() &&
    // Does the sequence info match?
    s.Sequence != nil &&
    s.Sequence.Source.Equal(protocol.PartitionUrl(si.Source)) &&
    s.Sequence.Destination.Equal(protocol.PartitionUrl(si.Destination)) &&
    s.Sequence.Number == si.Number {
    // If it's been delivered, skip it
    return errors.Delivered
}
```

```go
// getClients returns a map of Tendermint RPC clients for the given partitions
func (d *dispatcher) getClients(ctx context.Context, want map[string]bool) map[string]peerClient {
    // First try local clients (same node)
    clients := make(map[string]peerClient, len(want))
    for part, client := range d.self {
        clients[part] = peerClient{DispatcherClient: client}
        delete(want, part)
    }
    
    // For partitions without local clients, discover remote peers
    if len(want) > 0 {
        // Walk the directory network to find peers for other partitions
        // This may not always succeed for all wanted partitions
    }
    
    return clients
}
```

4. **Timing and Race Conditions**:
   - New validators joining the network might miss historical anchors
   - Concurrent operations can lead to inconsistent state

### Anchor Healing Mechanism

To address these reliability issues, Accumulate implements an anchor healing mechanism:

```go
func (c *Conductor) healAnchors(ctx context.Context, batch *database.Batch, destination *url.URL, currentBlock uint64) error {
    // Load the source sequence chain
    sequence := batch.Account(c.Url(protocol.AnchorPool)).AnchorSequenceChain()
    head, err := sequence.Head().Get()
    
    // Load the destination anchor ledger
    var ledger1 *protocol.AnchorLedger
    _, err = c.Querier.QueryAccountAs(ctx, destination.JoinPath(protocol.AnchorPool), nil, &ledger1)
    ledger2 := ledger1.Partition(c.Url())
    
    // For each not-yet delivered anchor
    for i := ledger2.Delivered + 1; i <= uint64(head.Count); i++ {
        // Load the anchor
        // ...
        
        // Ignore anchors from the last 10 blocks
        if currentBlock-anchor.GetPartitionAnchor().MinorBlockIndex < 10 {
            continue
        }
        
        // Check if we've already signed this one
        ok, err = c.didSign(ctx, txn.ID())
        if ok {
            continue
        }
        
        // Submit it
        err = c.submit(ctx, destination, env)
    }
    
    return nil
}
```

This healing process:
1. Identifies anchors that haven't been delivered to the destination network
2. Reconstructs the anchor envelopes
3. Verifies that the current validator hasn't already signed them
4. Resubmits the anchors to ensure eventual delivery

### Synthetic Transaction Healing

Similarly, synthetic transactions (cross-network messages) may fail to be delivered. Accumulate implements a healing process to recover them:

```go
// buildSynthReceiptV1 builds a synthetic receipt for healing
func (h *Healer) buildSynthReceiptV1(_ context.Context, args HealSyntheticArgs, si SequencedInfo) (*merkle.Receipt, error) {
    batch := args.Light.OpenDB(false)
    defer batch.Discard()
    uSrc := protocol.PartitionUrl(si.Source)
    
    // Complex receipt building logic to reconstruct proofs
    // ...
}
```

The synthetic transaction healing process works by:

1. **Detecting Missing Transactions**:
   - Tracking sequence numbers for cross-network messages
   - Identifying gaps in the sequence that indicate missing transactions

2. **Reconstructing Proofs**:
   - Building Merkle receipts that prove the transaction's inclusion in the source chain
   - Connecting these proofs to the destination chain's anchors

3. **Resubmitting with Valid Proofs**:
   - Creating a new synthetic message with the reconstructed proof
   - Submitting it directly to the destination network

```go
func (h *Healer) HealSynthetic(ctx context.Context, args HealSyntheticArgs, si SequencedInfo) error {
    // Query the synthetic transaction
    r, err := ResolveSequenced[messaging.Message](ctx, args.Client, args.NetInfo, si.Source, si.Destination, si.Number, false)
    
    // Check if it's already been delivered
    Q := api.Querier2{Querier: args.Querier}
    if s, err := Q.QueryMessage(ctx, r.ID, nil); err == nil &&
        s.Status.Delivered() &&
        s.Sequence != nil &&
        s.Sequence.Source.Equal(protocol.PartitionUrl(si.Source)) &&
        s.Sequence.Destination.Equal(protocol.PartitionUrl(si.Destination)) &&
        s.Sequence.Number == si.Number {
        // If it's been delivered, skip it
        return errors.Delivered
    }
    
    // Build the receipt
    var receipt *merkle.Receipt
    if args.NetInfo.Status.ExecutorVersion.V2VandenbergEnabled() {
        receipt, err = h.buildSynthReceiptV2(ctx, args, si)
    } else {
        receipt, err = h.buildSynthReceiptV1(ctx, args, si)
    }
    
    // Submit the synthetic transaction with the new proof
    msg := &messaging.SyntheticMessage{
        Message: r.Sequence,
        Proof: &protocol.AnnotatedReceipt{
            Receipt: receipt,
            Anchor: &protocol.AnchorMetadata{
                Account: protocol.DnUrl(),
            },
        },
        Signature: msg.Signature,
    }
    
    // Create and submit the envelope
    env := new(messaging.Envelope)
    env.Messages = []messaging.Message{msg}
    // ...
}
```

### Performance Considerations and Metrics

The healing process introduces performance overhead that must be carefully managed:

1. **Metrics Collection**:
   - Accumulate tracks various metrics related to cross-network communication
   - These metrics help identify bottlenecks and optimize the healing process

```go
// Metrics for monitoring dispatch performance and reliability
var (
    mDispatchPanics = promauto.NewCounter(prometheus.CounterOpts{
        Name: "accumulate_dispatch_panics_total",
        Help: "Total number of panics during dispatch",
    })
    
    mDispatchCalls = promauto.NewCounter(prometheus.CounterOpts{
        Name: "accumulate_dispatch_calls_total",
        Help: "Total number of dispatch calls",
    })
    
    mDispatchEnvelopes = promauto.NewCounter(prometheus.CounterOpts{
        Name: "accumulate_dispatch_envelopes_total",
        Help: "Total number of envelopes dispatched",
    })
    
    mDispatchErrors = promauto.NewCounter(prometheus.CounterOpts{
        Name: "accumulate_dispatch_errors_total",
        Help: "Total number of dispatch errors",
    })
)
```

2. **Throttling and Backoff**:
   - The healing process includes throttling to prevent overwhelming the network
   - Anchors from the last 10 blocks are ignored to allow normal delivery to succeed

```go
// Ignore anchors from the last 10 blocks
if currentBlock-anchor.GetPartitionAnchor().MinorBlockIndex < 10 {
    continue
}
```

3. **Batching Optimizations**:
   - Multiple anchors can be healed in a single batch
   - The dispatcher groups messages by destination to minimize connection overhead

```go
// Process the queue
for _, env := range queue {
    subs, err := submitter.Submit(ctx, env, api.SubmitOptions{})
    // ...
}
```

### Implementation Evolution

The healing mechanism has evolved across different versions of Accumulate:

1. **V1 vs V2 Receipt Building**:
   - V1 and V2 use different approaches to reconstruct receipts for healing
   - V2 (Vandenberg) introduced more efficient receipt building

```go
// Build the receipt using the appropriate version
var receipt *merkle.Receipt
if args.NetInfo.Status.ExecutorVersion.V2VandenbergEnabled() {
    receipt, err = h.buildSynthReceiptV2(ctx, args, si)
} else {
    receipt, err = h.buildSynthReceiptV1(ctx, args, si)
}
```

2. **V2 Receipt Building Improvements**:
   - More efficient chain traversal
   - Better handling of source and destination chains
   - Improved error reporting

```go
// V2 receipt building (simplified)
func (h *Healer) buildSynthReceiptV2(_ context.Context, args HealSyntheticArgs, si SequencedInfo) (*merkle.Receipt, error) {
    // Load the synthetic sequence chain entry
    b, err := batch.Account(uSrcSynth).SyntheticSequenceChain(si.Destination).Entry(int64(si.Number) - 1)
    
    // Locate the synthetic ledger main chain index entry
    mainIndex, err := batch.Index().Account(uSrcSynth).Chain("main").SourceIndex().FindIndexEntryAfter(seqEntry.Source)
    
    // Build the synthetic ledger part of the receipt
    receipt, err := batch.Account(uSrcSynth).MainChain().Receipt(seqEntry.Source, mainIndex.Source)
    
    // Connect to the BVN root chain
    bvnRootIndex, err := batch.Index().Account(uSrcSys).Chain("root").SourceIndex().FindIndexEntryAfter(mainIndex.Anchor)
    
    // Build the BVN part of the receipt
    bvnReceipt, err := batch.Account(uSrcSys).RootChain().Receipt(mainIndex.Anchor, bvnRootIndex.Source)
    receipt, err = receipt.Combine(bvnReceipt)
    
    // Connect to the DN if needed
    // ...
    
    return receipt, nil
}
```

3. **Baikonur Improvements**:
   - Enhanced synthetic message handling
   - Better compatibility with different protocol versions

```go
// Version-specific envelope creation
if args.NetInfo.Status.ExecutorVersion.V2BaikonurEnabled() {
    env.Messages = []messaging.Message{msg}
} else {
    env.Messages = []messaging.Message{&messaging.BadSyntheticMessage{
        Message:   msg.Message,
        Signature: msg.Signature,
        Proof:     msg.Proof,
    }}
}
```

### Implications for IBC Implementation

These reliability challenges and healing mechanisms have important implications for Accumulate's IBC implementation:

1. **Eventual Consistency Model**:
   - Accumulate's cross-network communication follows an eventual consistency model
   - Anchors and synthetic transactions may not be delivered immediately
   - The healing process ensures they will eventually be delivered
   - IBC implementations must account for this eventual consistency

2. **Proof Generation Complexity**:
   - The need to regenerate proofs for healing adds complexity to the IBC implementation
   - IBC relayers must be able to handle reconstructed proofs
   - Verification of these proofs requires access to historical state

3. **Performance Considerations**:
   - The healing process adds overhead to the system
   - IBC implementations should be designed to minimize the need for healing
   - Optimizing the primary transmission path improves overall performance

4. **State Verification Requirements**:
   - IBC implementations must maintain sufficient historical state to verify reconstructed proofs
   - Light clients need access to anchor points that can be used for verification

### Lost Transaction Detection Challenges

One of the most challenging aspects of Accumulate's cross-network communication is detecting lost transactions. Several factors make this particularly difficult:

1. **Sequence Tracking Limitations**:
   - Accumulate tracks sequence numbers using the `PartitionSyntheticLedger` structure
   - This structure maintains three key counters:
     - `Received`: The highest sequence number received
     - `Delivered`: The highest sequence number delivered
     - `Pending`: A slice of transaction IDs for transactions received but not yet delivered

```go
type PartitionSyntheticLedger struct {
    Url       *url.URL   // The partition URL
    Received  uint64     // The highest sequence number received
    Delivered uint64     // The highest sequence number delivered
    Pending   []*url.TxID // Transaction IDs for transactions received but not delivered
}
```

2. **Gap Detection Blind Spots**:
   - The system can only detect gaps between `Delivered` and `Received`
   - If a transaction is never received (e.g., due to network partition), its absence won't be detected
   - There's no way to know if sequence numbers beyond `Received` exist

```go
// Get gets the hash for a synthetic transaction.
func (s *PartitionSyntheticLedger) Get(sequenceNumber uint64) (*url.TxID, bool) {
    if sequenceNumber <= s.Delivered || sequenceNumber > s.Received {
        return nil, false
    }

    txid := s.Pending[sequenceNumber-s.Delivered-1]
    return txid, txid != nil
}
```

3. **Leader-Based Submission Model**:
   - Only the leader validator submits transactions
   - If the leader fails to submit a transaction, other validators won't know about it
   - There's no consensus mechanism for transaction submission itself

4. **Asynchronous Healing Triggers**:
   - Healing is triggered during block processing, not by detecting missing sequences
   - The healing process relies on comparing local and remote ledger states
   - There's no proactive mechanism to detect missing transactions

```go
// healAnchors compares local and remote ledger states
func (c *Conductor) healAnchors(ctx context.Context, batch *database.Batch, destination *url.URL, currentBlock uint64) error {
    // Load the source sequence chain
    sequence := batch.Account(c.Url(protocol.AnchorPool)).AnchorSequenceChain()
    head, err := sequence.Head().Get()
    
    // Load the destination anchor ledger
    var ledger1 *protocol.AnchorLedger
    _, err = c.Querier.QueryAccountAs(ctx, destination.JoinPath(protocol.AnchorPool), nil, &ledger1)
    ledger2 := ledger1.Partition(c.Url())
    
    // For each not-yet delivered anchor
    for i := ledger2.Delivered + 1; i <= uint64(head.Count); i++ {
        // Healing logic...
    }
}
```

5. **No End-to-End Acknowledgment**:
   - The system lacks an end-to-end acknowledgment protocol
   - The source network doesn't receive confirmation that a transaction was processed
   - This creates an asymmetric view of transaction state between networks

These challenges make it difficult to guarantee that all transactions are eventually delivered, despite the healing mechanism. The system relies on eventual consistency rather than strong consistency, which is a fundamental consideration for IBC implementations.

```go
// buildSynthReceiptV1 builds a synthetic receipt for healing
func (h *Healer) buildSynthReceiptV1(_ context.Context, args HealSyntheticArgs, si SequencedInfo) (*merkle.Receipt, error) {
    batch := args.Light.OpenDB(false)
    defer batch.Discard()
    uSrc := protocol.PartitionUrl(si.Source)
    uSys := uSrc.JoinPath(protocol.Ledger)
    uSynth := uSrc.JoinPath(protocol.Synthetic)

    // Load the synthetic sequence chain entry
    b, err := batch.Account(uSynth).SyntheticSequenceChain(si.Destination).Entry(int64(si.Number) - 1)
    if err != nil {
        return nil, err
    }

    // Build the synthetic receipt
    // ...

    // Combine the receipts
    return mainReceipt.Combine(rootReceipt, dnSourceRootReceipt, dnRootReceipt)
}
```

## Verification and Security

The anchoring and receipt system provides several security guarantees:

### 1. Tamper Resistance

Any attempt to tamper with a BVN's state would require changing:
- The BVN's state root
- The BVN's anchor in the DN
- The DN's state root
- All subsequent anchors and receipts

This creates a cryptographically secure chain that is extremely difficult to compromise.

### 2. Independent Verification

Any node can verify the validity of an anchor or receipt by:
1. Checking the cryptographic proofs
2. Validating the Merkle paths
3. Verifying the signatures on the transactions

### 3. Cross-Chain Validation

The bidirectional anchoring process ensures that:
- BVNs can verify their state is correctly represented in the DN
- The DN can verify the state of all BVNs
- External systems can verify Accumulate's state through the anchoring process

## Performance Optimizations

Accumulate implements several optimizations to make the anchoring and receipt process efficient:

### 1. Batched Anchoring

Anchors are generated periodically rather than for every transaction, reducing overhead:

```go
// shouldGenerateAnchor determines if an anchor should be generated
func (x *Executor) shouldGenerateAnchor() bool {
    // Generate an anchor if enough blocks have passed
    if x.blockIndex - x.lastAnchorBlock >= x.anchorInterval {
        return true
    }
    
    // Generate an anchor if enough time has passed
    if time.Since(x.lastAnchorTime) >= x.anchorTimeInterval {
        return true
    }
    
    return false
}
```

### 2. Efficient Proof Construction

Merkle proofs are constructed efficiently to minimize computational overhead:

```go
// build takes the values collected by GetReceipt and flushes out the data structures
// in the receipt to represent a fully populated version.
func (r *Receipt) build(getIntermediate getIntermediateFunc, anchorState *State) error {
    height := int64(1) // Start the height at 1, because the element isn't part
    r.Anchor = r.Start // of the nodes collected. To begin with, the element is the Merkle Dag Root
    stay := true       // stay represents the fact that the proof is already in this column

    // Efficient proof construction algorithm
    // ...
    
    return nil
}
```

### 3. Receipt Caching

Frequently used receipts are cached to avoid regenerating them:

```go
// getReceipt gets a receipt from cache or generates it
func (x *Executor) getReceipt(batch *database.Batch, start, anchor []byte) (*merkle.Receipt, error) {
    // Check cache
    cacheKey := fmt.Sprintf("%x:%x", start, anchor)
    if receipt, ok := x.receiptCache[cacheKey]; ok {
        return receipt, nil
    }
    
    // Generate receipt
    receipt, err := batch.Account(x.Describe.PartitionId).RootChain().Receipt(start, anchor)
    if err != nil {
        return nil, err
    }
    
    // Cache receipt
    x.receiptCache[cacheKey] = receipt
    
    return receipt, nil
}
```

## Practical Applications

The anchoring and receipt system enables several important features in Accumulate:

### 1. Transaction Verification

Clients can verify that their transactions have been included in the blockchain by:
1. Obtaining a receipt proving the transaction's inclusion in a BVN
2. Verifying the BVN's anchor in the DN
3. Potentially verifying the DN's anchor in an external blockchain

### 2. Cross-Partition Consistency

The anchoring system ensures consistency across different partitions by:
1. Anchoring each partition's state to the DN
2. Providing receipts that prove the inclusion of each partition's state
3. Enabling verification of cross-partition transactions

### 3. External Verification

External systems can verify Accumulate's state by:
1. Obtaining the latest DN anchor
2. Verifying the DN anchor against an external blockchain (if applicable)
3. Using receipts to verify specific transactions or states within Accumulate

## Conclusion

Accumulate's proof and receipt system provides a robust mechanism for cross-network validation. By using Merkle proofs and a hierarchical anchoring structure, Accumulate ensures that the state of each network can be cryptographically verified by other networks, creating a secure and trustless ecosystem.

The bidirectional anchoring process, combined with the ability to combine receipts into chains of proofs, enables powerful verification capabilities that extend beyond the boundaries of individual networks. This architecture is fundamental to Accumulate's security model and its ability to scale across multiple partitions while maintaining consistency and trust.

## References

1. [Anchoring Process](../03_anchoring_process.md)
2. [Cross-Network Communication](02_cross_network_communication.md)
3. [Routing Architecture](01_routing_architecture.md)
4. [Merkle Trees and Proofs](https://en.wikipedia.org/wiki/Merkle_tree)
