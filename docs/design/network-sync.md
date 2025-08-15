# Network Anchor Synchronization Documentation

## Overview

This document provides a comprehensive technical explanation of how the Accumulate Directory Network (DN) and Block Validator Networks (BVNs) exchange anchor information to synchronize their states. The anchor exchange process is critical for maintaining network consistency and enabling healing mechanisms when partitions become out of sync.

## Table of Contents

1. [Core Concepts](#core-concepts)
2. [Validator Roles and Selection](#validator-roles-and-selection)
3. [Anchor Types and Structure](#anchor-types-and-structure)
4. [Ledger Structures for Tracking](#ledger-structures-for-tracking)
5. [Anchor Construction and Flow](#anchor-construction-and-flow)
6. [Sequence Tracking and Validation](#sequence-tracking-and-validation)
7. [Healing Process](#healing-process)
8. [Version Differences (V1 vs V2)](#version-differences-v1-vs-v2)
9. [Debug Commands](#debug-commands)
10. [Network Topology and Flow](#network-topology-and-flow)
11. [Troubleshooting](#troubleshooting)

## Core Concepts

### Anchors
Anchors are cryptographic commitments that represent the state of a partition at a specific point in time. They serve as checkpoints that allow partitions to verify and synchronize their states with each other.

**Key Properties:**
- **Root Chain Anchor**: Hash of the partition's root chain at a specific height
- **State Tree Anchor**: Root hash of the partition's Binary Patricia Tree (BPT) 
- **Block Indices**: Minor and major block indices for temporal ordering
- **Source Partition**: The partition that produced the anchor

### Synthetic Transactions
Synthetic transactions are system-generated transactions that facilitate cross-partition communication and state synchronization. They are produced as a result of user transactions that affect multiple partitions.

**Characteristics:**
- Generated automatically by the protocol
- Tracked alongside anchors in ledger structures
- Sequenced and delivered in order
- Critical for maintaining cross-partition consistency

### Network Partitions
- **Directory Network (DN)**: Central coordinator that manages network-wide state and routing
- **Block Validator Networks (BVNs)**: Partition-specific validators that process user transactions
- **Anchor Exchange Pattern**: DN ↔ BVN bidirectional anchor exchange

## Validator Roles and Selection

### Cross-Network Transaction Sending

Only specific validator nodes are responsible for sending anchors and synthetic transactions between network partitions. The selection process is handled automatically by the consensus mechanism.

### Block Leader Selection (CometBFT Consensus)

The validator that sends transactions to other networks is determined by **CometBFT's consensus mechanism**:

```go
// From accumulator.go - Leader determination
isLeader := bytes.Equal(app.Address.Bytes(), req.Header.GetProposerAddress())
```

**Key Points:**
- **CometBFT** (the consensus engine) selects a **block proposer** for each block
- The proposer becomes the **block leader** for that specific block
- Only the block leader sends synthetic transactions to other networks
- Leadership rotates automatically among validators

### Selection Process

1. **CometBFT Consensus Algorithm:**
   - Uses a **deterministic selection** based on validator stake/power
   - Each validator has a "voting power" (typically set to 1 for equal weight)
   - The proposer rotates among validators based on their power and the consensus algorithm

2. **Leader Identification:**
   - Each node compares its own address with the `ProposerAddress` in the block header
   - If they match, that node becomes the leader for this block
   - Only the leader executes cross-network message sending

### Transaction Sending Rules

```go
// Anchors - sent by any validator (but typically coordinated by the leader)
if x.isValidator {
    err = x.mainDispatcher.Submit(context.Background(), destPartUrl, env)
}

// Synthetic Transactions - ONLY sent by the block leader
if isLeader {
    env := &messaging.Envelope{Messages: messages}
    err = x.mainDispatcher.Submit(context.Background(), seq.Destination, env)
}
```

**Transaction Types:**
- **Anchors**: Can be sent by any validator node in the partition
- **Synthetic Transactions**: Only sent by the designated block leader
- **Non-validators**: Cannot send either anchors or synthetic transactions

### Validator Configuration

Validators are configured in the consensus section of partition snapshots:
- Validator sets are defined per partition in consensus JSON files
- Each validator has an address and voting power
- CometBFT handles the automatic rotation of leadership
- No manual intervention required for leader selection

### Summary

- **Who:** The validator selected as **block proposer** by CometBFT consensus
- **When:** Once per block (leadership rotates each block)
- **How:** CometBFT's built-in proposer selection algorithm
- **What:** Only the leader sends synthetic transactions; any validator can send anchors
- **Selection:** Automatic and deterministic - no manual configuration needed

## Anchor Types and Structure

### Base Anchor Structure (`PartitionAnchor`)
```go
type PartitionAnchor struct {
    Source          *url.URL  // Source partition URL
    MajorBlockIndex uint64    // Major block index (0 for minor blocks)
    MinorBlockIndex uint64    // Minor block index
    RootChainIndex  uint64    // Index of last root chain entry
    RootChainAnchor [32]byte  // Hash of root chain at RootChainIndex
    StateTreeAnchor [32]byte  // Root hash of partition's BPT
}
```

### Directory Network Anchors (`DirectoryAnchor`)
Extends `PartitionAnchor` with additional DN-specific information:

```go
type DirectoryAnchor struct {
    PartitionAnchor
    Updates        []NetworkAccountUpdate    // Network account synchronization updates
    Receipts       []*PartitionAnchorReceipt // Receipts for BVN anchors included in block
    MakeMajorBlock uint64                    // Signals BVNs to open major block
    MakeMajorBlockTime time.Time             // Timestamp for major block coordination
}
```

**Key Features:**
- **Network Updates**: Synchronizes network-wide account changes
- **Anchor Receipts**: Includes receipts for BVN anchors processed in the block
- **Major Block Coordination**: Signals when BVNs should create major blocks

### BVN Anchors (`BlockValidatorAnchor`)
Extends `PartitionAnchor` with BVN-specific information:

```go
type BlockValidatorAnchor struct {
    PartitionAnchor
    AcmeBurnt big.Int  // Amount of ACME tokens burned in transactions
}
```

## Ledger Structures for Tracking

### Anchor Ledger (`AnchorLedger`)
Tracks the overall anchor synchronization state for a partition:

```go
type AnchorLedger struct {
    Url                      *url.URL                    // Partition URL
    MinorBlockSequenceNumber uint64                      // Current minor block sequence
    MajorBlockIndex          uint64                      // Current major block index
    MajorBlockTime           time.Time                   // Last major block timestamp
    PendingMajorBlockAnchors []*url.URL                  // Pending major block anchors
    Sequence                 []*PartitionSyntheticLedger // Per-partition sync state
}
```

### Partition Synthetic Ledger (`PartitionSyntheticLedger`)
Tracks synchronization state between two specific partitions:

```go
type PartitionSyntheticLedger struct {
    Url       *url.URL     // Remote partition URL
    Produced  uint64       // Highest sequence number produced to this partition
    Received  uint64       // Highest sequence number received from this partition  
    Delivered uint64       // Highest sequence number successfully delivered
    Pending   []*url.TxID  // Transaction IDs of pending messages
}
```

**Sequence Number Meanings:**
- **Produced**: Last anchor/synthetic transaction sent TO the remote partition
- **Received**: Last anchor/synthetic transaction received FROM the remote partition
- **Delivered**: Last anchor/synthetic transaction successfully processed
- **Pending**: Queue of unprocessed transactions awaiting delivery

## Anchor Construction and Flow

### Anchor Construction Process

#### 1. Block Processing (`BeginBlock`)
During block processing, the executor determines if an anchor should be created based on:
- Major block creation requirements
- Synthetic transaction production
- Account state changes
- Received anchors from other partitions

#### 2. Anchor Preparation (`prepareAnchor`)
```go
func (x *Executor) prepareAnchor(block *Block) error {
    // Update anchor ledger sequence numbers
    anchorLedger.MinorBlockSequenceNumber++
    
    // Handle major block coordination
    if block.State.Anchor.ShouldOpenMajorBlock {
        ledger.MajorBlockIndex++
        ledger.MajorBlockTime = block.State.Anchor.OpenMajorBlockTime
    }
    
    // Build partition-specific anchor
    switch x.Describe.NetworkType {
    case protocol.PartitionTypeDirectory:
        ledger.Anchor = x.buildDirectoryAnchor(block, ledger, anchorLedger)
    case protocol.PartitionTypeBlockValidator:
        ledger.Anchor = x.buildPartitionAnchor(block, ledger)
    }
}
```

#### 3. Anchor Finalization (`finalizeBlock`)
The anchor is populated with final state information and sent to destination partitions:

```go
func (x *Executor) finalizeBlock(block *Block) error {
    // Complete anchor with final root chain and state tree anchors
    anchor.RootChainIndex = rootChain.Height() - 1
    anchor.RootChainAnchor = rootChain.Anchor()
    anchor.StateTreeAnchor = batch.BptRootHash()
    
    // Send anchor to destination partitions
    switch x.Describe.NetworkType {
    case protocol.PartitionTypeDirectory:
        // DN sends anchors to all BVNs
        for _, bvn := range x.globals.Active.BvnNames() {
            x.sendBlockAnchor(batch, anchor, sequenceNumber, bvn)
        }
    case protocol.PartitionTypeBlockValidator:
        // BVN sends anchor to DN
        x.sendBlockAnchor(batch, anchor, sequenceNumber, protocol.Directory)
    }
}
```

## Messaging Envelopes and Transmission

### Envelope Structure

All inter-partition communication in Accumulate uses `messaging.Envelope` structs as the fundamental unit of transmission. Each envelope contains:

```go
type Envelope struct {
    Signatures  []protocol.Signature    // Array of signatures (synthetic origin + key signatures)
    TxHash      []byte                  // Optional transaction hash for validation
    Transaction []*protocol.Transaction // Array of transactions being transmitted
    Messages    []Message               // Array of messaging protocol messages
}
```

**Key Properties:**
- Envelopes are the atomic unit of transmission between partitions
- Each envelope can contain multiple transactions and their associated signatures
- Envelopes are normalized and validated before transmission
- The envelope structure supports both transactions and pure messaging protocols

### Anchor Construction Process

Anchors are constructed by the `PrepareBlockAnchor` function during block finalization:

```go
func PrepareBlockAnchor(network *url.URL, netdef *protocol.NetworkDefinition, 
                       nodeKey []byte, anchor protocol.TransactionBody, 
                       sequenceNumber uint64, destPartUrl *url.URL) (*messaging.Envelope, error) {
    // 1. Create synthetic transaction
    txn := new(protocol.Transaction)
    txn.Header.Principal = destPartUrl.JoinPath(protocol.AnchorPool)
    txn.Body = anchor

    // 2. Create synthetic origin signature
    initSig, err := new(signing.Builder).
        SetUrl(network).
        SetVersion(sequenceNumber).
        InitiateSynthetic(txn, destPartUrl)

    // 3. Create key signature using node's Ed25519 key
    keySig, err := SignTransaction(netdef, nodeKey, txn, initSig.DestinationNetwork)

    // 4. Wrap in envelope
    return &messaging.Envelope{
        Transaction: []*protocol.Transaction{txn}, 
        Signatures: []protocol.Signature{initSig, keySig}
    }, nil
}
```

**Construction Steps:**
1. **Transaction Creation**: Creates a synthetic transaction with principal URL set to the destination partition's anchor pool (`destPartUrl/anchor-pool`)
2. **Synthetic Origin Signature**: Creates a synthetic origin signature using the source partition URL and sequence number for routing and ordering
3. **Key Signature**: Signs the transaction hash with the node's Ed25519 private key for authentication
4. **Envelope Wrapping**: Combines the transaction and both signatures into a messaging envelope

### Key Signature Process

The `SignTransaction` function creates the Ed25519 key signature:

```go
func SignTransaction(network *protocol.NetworkDefinition, nodeKey []byte, 
                    txn *protocol.Transaction, destination *url.URL) (protocol.Signature, error) {
    bld := new(signing.Builder).
        SetType(protocol.SignatureTypeED25519).
        SetPrivateKey(nodeKey).
        SetUrl(protocol.DnUrl().JoinPath(protocol.Network)).
        SetVersion(network.Version).
        SetTimestamp(1)

    return bld.Sign(txn.GetHash())
}
```

### Dispatcher and Transmission Mechanism

The messaging system uses a dispatcher to route and send envelopes between partitions:

```go
type dispatcher struct {
    sim       *Simulator
    envelopes map[string][]*messaging.Envelope  // Queued envelopes per partition
}
```

**Transmission Flow:**

1. **Submit Phase** (`dispatcher.Submit`):
   ```go
   func (d *dispatcher) Submit(ctx context.Context, u *url.URL, env *messaging.Envelope) error {
       // Route envelope to destination partition
       partition, err := d.sim.router.RouteAccount(u)
       if err != nil {
           return err
       }
       // Queue envelope for the partition
       d.envelopes[partition] = append(d.envelopes[partition], env)
       return nil
   }
   ```

2. **Send Phase** (`dispatcher.Send`):
   ```go
   func (d *dispatcher) Send(ctx context.Context) <-chan error {
       // Process all queued envelopes asynchronously
       for part, envelopes := range d.envelopes {
           for _, envelope := range envelopes {
               err := d.sim.SubmitTo(part, envelope)
               // Handle delivery confirmation or errors
           }
       }
   }
   ```

3. **Delivery Phase** (`simulator.SubmitTo`):
   ```go
   func (s *Simulator) SubmitTo(partition string, envelope *messaging.Envelope) error {
       x := s.Partition(partition)
       envelope = envelope.Copy()  // Use copy to avoid modification issues
       
       // Validate envelope before processing
       results, err := (*execute.ExecutorV1)(x.Executor).Validate(envelope, true)
       if err != nil {
           return errors.UnknownError.Wrap(err)
       }
       
       // Enqueue for execution
       x.Submit(false, envelope)
       return nil
   }
   ```

**Key Transmission Properties:**
- **Asynchronous Processing**: Envelopes are queued and sent asynchronously to avoid blocking block production
- **Partition Routing**: Router determines destination partition based on URL
- **Batch Processing**: Multiple envelopes can be queued and sent together for efficiency
- **Validation**: Destination partition validates envelopes before processing
- **Error Handling**: Delivery errors are reported through error channels

### Block Leader Role in Synthetic Transaction Transmission

The block leader has special responsibilities for sending synthetic transactions and anchors during block finalization:

```go
func (x *Executor) finalizeBlock(batch *database.Batch) error {
    // Send synthetic transactions (block leader only)
    if x.isLeader {
        err := x.sendSyntheticTransactionsForBlock(batch)
        if err != nil {
            return errors.UnknownError.WithFormat("send synthetic transactions: %w", err)
        }
    }

    // Send block anchor (block leader only)
    if x.isLeader {
        err := x.sendBlockAnchor(batch)
        if err != nil {
            return errors.UnknownError.WithFormat("send block anchor: %w", err)
        }
    }

    return nil
}
```

**Block Leader Responsibilities:**

1. **Synthetic Transaction Dispatch**: Only the block leader sends synthetic transactions to other partitions to avoid duplicate submissions

2. **Anchor Transmission**: Block leader constructs and sends anchors to destination partitions after block completion

3. **Sequence Management**: Block leader increments sequence numbers for outbound messages to maintain ordering

4. **Envelope Construction**: Block leader creates properly signed envelopes with both synthetic origin and key signatures

**Synthetic Transaction Sending Process:**

```go
func (x *Executor) sendSyntheticTransactionsForBlock(batch *database.Batch) error {
    // Get synthetic transactions produced in this block
    synthTxns := batch.GetSyntheticTransactions()
    
    // Group by destination partition
    byPartition := make(map[string][]*protocol.Transaction)
    for _, txn := range synthTxns {
        partition := x.router.RouteAccount(txn.Header.Principal)
        byPartition[partition] = append(byPartition[partition], txn)
    }
    
    // Send to each partition
    for partition, txns := range byPartition {
        envelope := x.prepareSyntheticEnvelope(txns, partition)
        err := x.dispatcher.Submit(ctx, partition, envelope)
        if err != nil {
            return err
        }
    }
    
    // Send all queued envelopes
    return x.dispatcher.Send(ctx)
}
```

**Key Design Principles:**
- **Single Point of Transmission**: Only block leader sends to prevent duplicate messages
- **Batch Efficiency**: Multiple synthetic transactions are batched per destination partition
- **Ordered Delivery**: Sequence numbers ensure proper ordering across block boundaries
- **Error Isolation**: Transmission errors don't block local block production

## How Partitions Get Out of Sync

Partitions in the Accumulate network can become out of sync due to various failure scenarios that affect the transmission and delivery of anchors and synthetic transactions. Understanding these failure modes is critical for implementing effective healing mechanisms.

### Root Causes of Synchronization Failures

#### 1. Network Transmission Failures

**Anchor Transmission Failures:**
- **Block Leader Failure**: If the block leader crashes or becomes unavailable during anchor transmission, anchors may not be sent to destination partitions
- **Network Connectivity Issues**: Intermittent network failures between partitions can cause anchor messages to be lost in transit
- **Dispatcher Errors**: The dispatcher component may fail to route anchors to the correct destination partition due to routing table inconsistencies

**Synthetic Transaction Transmission Failures:**
- **Cross-Chain Message Loss**: Synthetic transactions generated by user actions may fail to reach their destination partition
- **Sequence Number Gaps**: Network failures can create gaps in the sequence number chain, causing subsequent messages to be rejected
- **Envelope Validation Failures**: Malformed or corrupted message envelopes may be rejected by the destination partition

#### 2. Partition-Specific Failures

**Validator Node Failures:**
```go
// From conductor.go - Error handling during dispatch
errs := c.Dispatcher.Send(context.Background())
for err := range errs {
    switch err := err.(type) {
    case protocol.TransactionStatusError:
        slog.Error("Failed to dispatch transactions", "error", err, 
                  "stack", err.TransactionStatus.Error.PrintFullCallstack())
    default:
        slog.Error("Failed to dispatch transactions", "error", err)
    }
}
```

**Database Corruption or Inconsistency:**
- Ledger state corruption can cause sequence number tracking to become inconsistent
- Atomic write failures during ledger updates may leave partitions in an inconsistent state

#### 3. Timing and Concurrency Issues

**Out-of-Order Message Processing:**
```go
// From msg_sequenced.go - Out-of-order validation
if pending && seq.Number <= partLedger.Delivered {
    return nil, errors.FatalError.WithFormat("synthetic messages processed out of order")
}
```

**Race Conditions:**
- Multiple validators attempting to process the same sequence number simultaneously
- Block finalization occurring before all synthetic transactions are properly sequenced

### Specific Failure Scenarios

#### Anchor Synchronization Failures

**Missing Directory Network Anchors:**
- BVNs expect regular anchors from the Directory Network to validate cross-chain transactions
- If DN anchors are missing, BVNs cannot validate synthetic transactions that reference those anchors
- This creates a cascading effect where synthetic transaction processing is blocked

**Missing BVN Anchors:**
- The Directory Network requires anchors from all BVNs to maintain the global state
- Missing BVN anchors prevent the DN from processing cross-chain transactions involving those partitions

#### Synthetic Transaction Synchronization Failures

**Sequence Number Gaps:**
```go
// From ledger.go - Gap detection in pending queue
if n := s.Received - s.Delivered - uint64(len(s.Pending)); n > 0 {
    s.Pending = append(s.Pending, make([]*url.TxID, n)...)
}
```

**Delivery Confirmation Failures:**
- Synthetic transactions may be sent but never confirmed as delivered
- This causes the sending partition to continue retransmitting, potentially creating duplicates
- The receiving partition may reject duplicates, leading to permanent synchronization gaps

### Detection Mechanisms

#### Sequence Number Monitoring

Partitions continuously monitor sequence numbers to detect gaps:

```go
// From PartitionSyntheticLedger structure
type PartitionSyntheticLedger struct {
    Url       *url.URL     // Remote partition URL
    Produced  uint64       // Highest sequence number produced to partition
    Received  uint64       // Highest sequence number received from partition
    Delivered uint64       // Highest sequence number successfully delivered
    Pending   []*url.TxID  // Queue of pending transaction IDs
}
```

**Gap Detection Logic:**
- If `Received > Delivered + len(Pending)`, there are gaps in the sequence
- Missing sequence numbers are identified by `nil` entries in the `Pending` queue
- Healing mechanisms are triggered when gaps persist beyond a threshold time

#### Anchor Validation Monitoring

**Missing Anchor Detection:**
```go
// From healing/anchors.go - Anchor resolution
func ResolveSequenced[T messaging.Message](ctx context.Context, 
    client message.AddressedClient, net *NetworkInfo, 
    srcId, dstId string, seqNum uint64, anchor bool) (*api.MessageRecord[T], error) {
    
    // Query each node until one succeeds
    for peer := range net.Peers[strings.ToLower(srcId)] {
        res, err := client.ForPeer(peer).Private().Sequence(ctx, 
            srcUrl.JoinPath(account), dstUrl, seqNum, private.SequenceOptions{})
        if err != nil {
            continue // Try next peer
        }
        return api.MessageRecordAs[T](res)
    }
    
    return nil, errors.UnknownError.WithFormat("cannot resolve %s→%s #%d", 
                                               srcId, dstId, seqNum)
}
```

#### Healing Trigger Conditions

**Automatic Healing Triggers:**
```go
// From conductor.go - Healing during block processing
if c.Partition.Type != protocol.PartitionTypeDirectory {
    c.runTask(func() {
        err := c.healAnchors(context.Background(), batch, protocol.DnUrl(), e.Index)
        if err != nil {
            slog.Error("Error while healing anchors", "error", err)
        }
    })
}
```

**Manual Healing Tools:**
- Debug tools can manually trigger healing for specific sequence numbers
- Network operators can identify and resolve synchronization issues using diagnostic commands

### Impact on Network Operations

#### Transaction Processing Delays
- Missing anchors can block cross-chain transaction validation
- Sequence gaps prevent synthetic transaction delivery
- Users may experience delayed transaction confirmations

#### Partition Isolation
- Severely out-of-sync partitions may become effectively isolated from the network
- Cross-chain operations involving isolated partitions will fail
- Network healing mechanisms must restore synchronization before normal operations can resume

#### Data Consistency Issues
- Prolonged synchronization failures can lead to state divergence between partitions
- Healing mechanisms must carefully validate and reconcile state differences
- In extreme cases, manual intervention may be required to restore network consistency

## Critical Healing System Limitations

**⚠️ PRODUCTION SCALABILITY ISSUE**: The current healing system has fundamental limitations that make it impractical for production-scale synchronization failures.

### No Built-in Protocol Healing

The Accumulate protocol lacks automatic healing mechanisms:

- **Manual Intervention Required**: All synchronization failures require manual healing using debug tools
- **No Self-Recovery**: Partitions cannot automatically recover from sync failures
- **Single Point of Failure**: Network reliability depends entirely on manual operator intervention

### Healing Tool Scale Limitations

Current healing tools cannot handle moderate-scale synchronization issues:

```go
// From heal_anchor.go - Sequential processing approach
for i, txid := range all {
    select {
    case <-h.ctx.Done():
        return
    default:
    }
    if h.healSingleAnchor(src.ID, dst.ID, src2dst.Delivered+1+uint64(i), txid, txns) {
        goto pullAgain  // Restart entire process on any change
    }
}
```

**Scale Limitations**:
- **20k anchors**: Considered "tiny" for modern applications, yet too large for current healing tools
- **Single-threaded processing**: No parallel or batch processing capabilities
- **Memory constraints**: Tools fail or become impractical at production scales
- **No incremental healing**: Must process entire sync gap sequentially

### Operational Reality

**Manual Healing Becomes Impossible**:
- Production networks may accumulate hundreds of thousands of missing anchors
- Current tools would take days or weeks to process large sync gaps
- Memory and performance constraints prevent large-scale healing operations
- Network partitions can become permanently out of sync

**No Viable Recovery Path**:
- Networks experiencing large-scale sync failures have no practical recovery mechanism
- Manual healing is the only option, but doesn't scale to production needs
- Permanent data inconsistency becomes inevitable at scale

### Architectural Implications

This represents a **fundamental scalability bottleneck**:

1. **Network Reliability**: Production networks cannot reliably recover from sync failures
2. **Operational Overhead**: Manual healing creates unsustainable operational burden
3. **Data Integrity**: No guarantee of maintaining consistency at scale
4. **Growth Limitations**: Network scalability is constrained by healing system limitations

**Critical Need**: The healing system requires complete redesign with:
- Automatic healing mechanisms built into the core protocol
- Batch processing and parallel healing capabilities
- Incremental healing to avoid reprocessing entire sync gaps
- Performance optimization for production-scale recovery operations

## Sequence Tracking and Validation

### Sequence Number Management
Each partition maintains sequence numbers for anchor and synthetic transaction exchanges through the `PartitionSyntheticLedger` structure:

```go
type PartitionSyntheticLedger struct {
    Url       *url.URL     // Remote partition URL
    Produced  uint64       // Highest sequence number produced to this partition
    Received  uint64       // Highest sequence number received from this partition  
    Delivered uint64       // Highest sequence number successfully delivered
    Pending   []*url.TxID  // Queue of pending transaction IDs awaiting delivery
}
```

### Sequenced Message Structure
All network messages are wrapped in sequenced envelopes for ordering:

```go
type SequencedMessage struct {
    Message     Message   // The actual message content (anchor, synthetic transaction, etc.)
    Source      *url.URL  // Source partition that produced the message
    Destination *url.URL  // Destination partition for the message
    Number      uint64    // Sequence number from source partition
}

// Generate unique transaction ID from destination and message hash
func (m *SequencedMessage) ID() *url.TxID {
    return m.Destination.WithTxID(m.Hash())
}
```

### Ledger Update Process
When processing sequenced messages, ledgers are updated atomically with comprehensive validation:

```go
func (x SequencedMessage) updateLedger(batch *database.Batch, ctx *MessageContext,
                                      seq *messaging.SequencedMessage, pending bool) (*protocol.PartitionSyntheticLedger, error) {
    // Load the ledger for this partition
    isAnchor, ledger, err := x.loadLedger(batch, ctx, seq)
    if err != nil {
        return nil, errors.UnknownError.Wrap(err)
    }
    partLedger := ledger.Partition(seq.Source)

    // Prevent out-of-order processing that could corrupt state
    if pending && seq.Number <= partLedger.Delivered {
        msg := "synthetic messages"
        if isAnchor {
            msg = "anchors"
        }
        return nil, errors.FatalError.WithFormat("%s processed out of order: delivered %d, processed %d", 
                                                 msg, partLedger.Delivered, seq.Number)
    }

    // Update ledger state - Add returns true if ledger was modified
    if partLedger.Add(!pending, seq.Number, seq.ID()) {
        err = batch.Account(ledger.GetUrl()).Main().Put(ledger)
        if err != nil {
            return nil, errors.UnknownError.WithFormat("store synthetic transaction ledger: %w", err)
        }
    }

    return partLedger, nil
}
```

### Sequence Number Addition Logic
The `Add` method handles both received and delivered message tracking:

```go
// Add records a received or delivered synthetic transaction
func (s *PartitionSyntheticLedger) Add(delivered bool, sequenceNumber uint64, txid *url.TxID) (dirty bool) {
    // Always update received counter to highest seen sequence number
    if sequenceNumber > s.Received {
        s.Received, dirty = sequenceNumber, true
    }

    if delivered {
        // Message was successfully processed - update delivered counter
        if sequenceNumber > s.Delivered {
            s.Delivered, dirty = sequenceNumber, true
        }
        // Remove from pending queue (FIFO processing)
        if len(s.Pending) > 0 {
            s.Pending, dirty = s.Pending[1:], true
        }
        return dirty
    }

    // Message received but not yet delivered - add to pending queue
    // Grow pending queue if necessary to accommodate gaps
    if n := s.Received - s.Delivered - uint64(len(s.Pending)); n > 0 {
        s.Pending, dirty = append(s.Pending, make([]*url.TxID, n)...), true
    }

    // Prevent duplicate delivery
    if sequenceNumber <= s.Delivered {
        panic("already delivered")
    }

    // Insert transaction ID at correct position in pending queue
    i := sequenceNumber - s.Delivered - 1
    if s.Pending[i] == nil {
        s.Pending[i], dirty = txid, true
    }

    return dirty
}
```

### Out-of-Order Message Handling
The system maintains a pending queue to handle messages that arrive out of sequence:

1. **Gap Detection**: When `sequenceNumber > partLedger.Delivered + 1`, a gap is detected
2. **Pending Queue**: Messages are stored in the pending array at position `sequenceNumber - delivered - 1`
3. **Queue Growth**: The pending array automatically grows to accommodate sequence gaps
4. **FIFO Processing**: Messages are processed in order, removing the first item when delivered
5. **Duplicate Prevention**: Attempting to deliver an already-delivered message causes a panic

## Healing Process

The healing process detects and resolves synchronization gaps between partitions.

### Detection Phase
Healing is triggered when:
1. Sequence gaps are detected in ledger comparisons
2. Missing anchors are identified through sequence chain analysis
3. Periodic health checks reveal inconsistencies

### Query Phase
The healing system queries remote partitions for missing information:

```go
func (h *healer) findPendingAnchors(src, dest *url.URL) ([]*url.TxID, error) {
    // Query destination's anchor ledger
    destLedger := h.queryAnchorLedger(dest)
    partLedger := destLedger.Partition(src)
    
    // Identify gaps in sequence
    for seq := partLedger.Delivered + 1; seq <= partLedger.Received; seq++ {
        // Query source partition for missing anchor
        anchor := h.queryAnchorBySequence(src, dest, seq)
        pendingAnchors = append(pendingAnchors, anchor.ID())
    }
    
    return pendingAnchors, nil
}
```

### Submission Phase
Missing anchors are retrieved and submitted with proper signatures:

```go
func (h *healer) healSingleAnchor(txid *url.TxID, src, dest *url.URL) error {
    // Retrieve anchor transaction and signatures
    anchor, signatures := h.queryAnchorWithSignatures(src, txid)
    
    // Submit to healing system
    return healing.HealAnchor(h.ctx, h.client, anchor, signatures, src, dest)
}
```

### Healing Implementation
The core healing logic handles version-specific processing:

```go
func HealAnchor(ctx context.Context, client Client, anchor *messaging.SequencedMessage,
               signatures []protocol.Signature, src, dest *url.URL) error {
    // Determine healing version
    if isV2VandenbergEnabled && isDnToBvn(src, dest) {
        return healDnAnchorV2(ctx, client, anchor, signatures, dest)
    }
    
    return healAnchorV1(ctx, client, anchor, signatures, dest)
}
```

## Version Differences (V1 vs V2)

### V1 Healing (Legacy)
- **Scope**: All anchor types and directions
- **Process**: Direct anchor submission with signature verification
- **Limitations**: Less efficient for DN→BVN anchors

### V2 Healing (Vandenberg+)
- **Scope**: Optimized for DN→BVN anchors when `V2VandenbergEnabled` is true
- **Process**: Unified anchor format with improved validation
- **Benefits**: Better performance and consistency for directory anchors

```go
func healDnAnchorV2(ctx context.Context, client Client, anchor *messaging.SequencedMessage,
                   signatures []protocol.Signature, dest *url.URL) error {
    // V2-specific healing logic for DN anchors
    // Improved validation and submission process
    return submitAnchorV2(ctx, client, anchor, signatures, dest)
}
```

### Version Detection
```go
func (h *healer) shouldUseV2Healing(src, dest *url.URL) bool {
    return h.globals.Active.ExecutorVersion.V2VandenbergEnabled() &&
           protocol.AreEqual(src, protocol.DnUrl()) &&
           !protocol.AreEqual(dest, protocol.DnUrl())
}
```

## Debug Commands

### Sequence Inspection (`debug sequence`)
Tests network synchronization by comparing anchor and synthetic sequences:

```bash
# Check synchronization status for a network
./accumulated debug sequence <network-name>

# Example output:
# Partition: acc://bvn-cyclops.acme
# Anchor Ledger Sequences:
#   acc://directory.acme: produced=1234, received=1235, delivered=1235
# Synthetic Ledger Sequences:  
#   acc://directory.acme: produced=5678, received=5679, delivered=5679
```

**Implementation:**
```go
func debugSequence(network string) error {
    // Query anchor and synthetic ledgers
    anchorLedger := queryAnchorLedger(partition)
    synthLedger := querySyntheticLedger(partition)
    
    // Compare sequence numbers for each remote partition
    for _, partLedger := range anchorLedger.Sequence {
        fmt.Printf("Anchor: %s: produced=%d, received=%d, delivered=%d\n",
                  partLedger.Url, partLedger.Produced, partLedger.Received, partLedger.Delivered)
    }
}
```

### Anchor Healing (`debug heal-anchor`)
Manually triggers anchor healing between partitions:

```bash
# Heal anchors from source to destination
./accumulated debug heal-anchor <source-partition> <dest-partition>
```

### Synthetic Healing (`debug heal-synth`) 
Heals synthetic transactions between partitions:

```bash
# Heal synthetic transactions
./accumulated debug heal-synth <source-partition> <dest-partition>
```

## Network Topology and Flow

### Anchor Exchange Patterns

#### Directory Network → BVNs
```
DN Block Processing:
1. Process user transactions and BVN anchors
2. Build DirectoryAnchor with network updates and receipts
3. Send anchor to all BVNs simultaneously
4. BVNs receive and validate DN anchor
5. BVNs update their anchor ledgers
```

#### BVNs → Directory Network  
```
BVN Block Processing:
1. Process user transactions for partition
2. Build BlockValidatorAnchor with ACME burn info
3. Send anchor to DN only
4. DN receives and validates BVN anchor
5. DN includes BVN anchor receipt in next DirectoryAnchor
```

### Sequence Flow Example
```
Initial State:
DN Anchor Ledger for BVN-A: produced=0, received=0, delivered=0
BVN-A Anchor Ledger for DN: produced=0, received=0, delivered=0

After DN Block 1:
DN → BVN-A: DirectoryAnchor (seq=1)
DN Anchor Ledger for BVN-A: produced=1, received=0, delivered=0

After BVN-A Receives:
BVN-A Anchor Ledger for DN: produced=0, received=1, delivered=1

After BVN-A Block 1:
BVN-A → DN: BlockValidatorAnchor (seq=1)  
BVN-A Anchor Ledger for DN: produced=1, received=1, delivered=1

After DN Receives:
DN Anchor Ledger for BVN-A: produced=1, received=1, delivered=1
```

## Troubleshooting

### Common Issues

#### 1. Sequence Gaps
**Symptoms**: `debug sequence` shows gaps between received and delivered
**Causes**: Network partitions, message loss, processing errors
**Resolution**: Run `debug heal-anchor` to fill gaps

#### 2. Stale Anchors
**Symptoms**: Partitions report different major block indices
**Causes**: Major block coordination failures
**Resolution**: Check DN major block signals and BVN processing

#### 3. Signature Verification Failures
**Symptoms**: Healing fails with signature errors
**Causes**: Key mismatches, corrupted signatures
**Resolution**: Verify validator keys and re-query signatures

### Diagnostic Queries

#### Check Anchor Ledger State
```bash
# Query anchor ledger for partition
curl -X POST -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"query","params":{"url":"acc://partition.acme/ACME#anchor"},"id":1}' \
  http://node:16695/v3
```

#### Check Synthetic Ledger State  
```bash
# Query synthetic ledger for partition
curl -X POST -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"query","params":{"url":"acc://partition.acme/ACME#synthetic"},"id":1}' \
  http://node:16695/v3
```

#### Verify Sequence Chains
```bash
# Query anchor sequence chain
curl -X POST -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"query-chain","params":{"url":"acc://partition.acme/ACME#anchor","name":"anchor-sequence-chain/acc://remote.acme"},"id":1}' \
  http://node:16695/v3
```

### Performance Considerations

#### Healing Frequency
- Healing runs continuously in background
- Triggered by sequence gap detection
- Can be manually initiated for urgent issues

#### Network Load
- Anchor size typically 100-500 bytes
- Frequency depends on block production rate
- BVN anchors sent only to DN
- DN anchors broadcast to all BVNs

#### Storage Requirements
- Anchor ledgers grow with number of partitions
- Sequence chains store historical anchor references
- Pruning may be implemented for old entries

## References

### Key Source Files
- `internal/core/healing/anchors.go` - Core healing logic
- `internal/core/execute/v*/block/block_end.go` - Anchor construction
- `internal/core/execute/v*/block/msg_sequenced.go` - Sequence processing
- `protocol/types_gen.go` - Anchor and ledger type definitions
- `tools/cmd/debug/sequence.go` - Debug sequence command
- `tools/cmd/debug/heal_anchor.go` - Debug healing commands

### Protocol References
- Anchor types: `DirectoryAnchor`, `BlockValidatorAnchor`, `PartitionAnchor`
- Ledger types: `AnchorLedger`, `PartitionSyntheticLedger`, `SyntheticLedger`
- Message types: `SequencedMessage`, `BlockAnchor`
- Healing functions: `HealAnchor`, `healDnAnchorV2`, `healAnchorV1`

---

*This documentation is additive and does not disrupt existing Cyclops validator deployments or scripts. All described functionality is part of the standard Accumulate protocol implementation.*
