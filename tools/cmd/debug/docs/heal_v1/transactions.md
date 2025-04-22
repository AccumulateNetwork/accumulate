# Transaction Creation for Network Healing

This document details how the `heal_anchor` and `heal_synth` approaches create transactions to heal the Accumulate network. Understanding these differences is crucial for implementing or modifying healing code.

## Shared Code Components {#shared-code-components}

Both healing approaches share several core components for transaction resolution and submission:

### 1. SequencedInfo Structure {#sequencedinfo-structure}

Both approaches use the same structure to identify transactions:

```go
// In healing/sequenced.go
type SequencedInfo struct {
    Source      string    // Source partition ID
    Destination string    // Destination partition ID
    Number      uint64    // Sequence number
    ID          *url.TxID // Transaction ID (optional)
}
```

### 2. ResolveSequenced Function {#resolvesequenced-function}

Both approaches use this shared function to resolve transaction IDs:

```go
// In healing/sequenced.go
func ResolveSequenced[T messaging.Message](ctx context.Context, client message.AddressedClient, 
    net *NetworkInfo, srcId, dstId string, seqNum uint64, anchor bool) (*api.MessageRecord[T], error) {
    // Determines the correct account path based on whether it's an anchor or synthetic transaction
    // Queries nodes until it finds one that can provide the transaction
    // Returns the message record with transaction details
}
```

- This function handles both anchor and synthetic transactions by setting the `anchor` parameter
- It constructs the appropriate URL path based on transaction type (anchor vs. synthetic)
- It queries nodes until it finds one that can provide the transaction details

**Related Documentation**:
- [URL Construction Considerations](./implementation.md#url-construction)
- [Common Pitfalls](./implementation.md#common-pitfalls)

### 3. waitFor Function {#waitfor-function}

Both approaches use this function to wait for transaction delivery:

```go
// In healing/anchors.go and healing/synthetic.go
func waitFor(ctx context.Context, querier api.Querier, txid *url.TxID) error {
    // Polls the transaction status until it's delivered or times out
    // Returns an error if the transaction fails to be delivered
}
```

### 4. Submission Process {#submission-process}

Both approaches use the same channel-based submission process:

```go
// In heal_common.go
func (h *healer) processSubmissions() {
    for msgs := range h.submit {
        // Process and submit messages to the network
        // Handles errors and retries
    }
}
```

**Related Documentation**:
- [Implementation Guidelines](./implementation.md#shared-components)

### 5. NetworkInfo Structure {#networkinfo-structure}

Both approaches use the same network information structure:

```go
// In healing/scan.go
type NetworkInfo struct {
    Status api.NetworkStatus
    Peers  map[string]map[peer.ID]PeerInfo
}
```

- This provides information about the network topology and validator status
- Used to determine which nodes to query for signatures or transaction data

**Related Documentation**:
- [Common Components](./overview.md#common-components)

## heal_anchor Transaction Creation {#heal_anchor-transaction-creation}

The anchor healing process creates transactions as follows:

```go
// In heal_anchor.go
func (h *healer) healSingleAnchor(srcId, dstId string, seqNum uint64, txid *url.TxID, txns map[[32]byte]*protocol.Transaction) bool {
    var count int
retry:
    err := healing.HealAnchor(h.ctx, healing.HealAnchorArgs{
        Client:  h.C2.ForAddress(nil),
        Querier: h.tryEach(),
        NetInfo: h.net,
        Known:   txns,
        Pretend: pretend,
        Wait:    waitForTxn,
        Submit: func(m ...messaging.Message) error {
            select {
            case h.submit <- m:
                return nil
            case <-h.ctx.Done():
                return errors.NotReady.With("canceled")
            }
        },
    }, healing.SequencedInfo{
        Source:      srcId,
        Destination: dstId,
        Number:      seqNum,
        ID:          txid,
    })
    // ... error handling
}
```

**Key Files**:
- `heal_anchor.go`: Main implementation
- `healing/anchors.go`: Core functionality for anchor healing

**Related Documentation**:
- [heal_anchor Approach](./overview.md#heal-anchor)
- [Caching System](./implementation.md#caching)

### Unique aspects of anchor healing {#unique-aspects-anchor}

#### 1. No Database Requirement {#no-database-requirement}

The `heal_anchor` approach is explicitly designed to work without a database:

```go
// In heal_anchor.go
func healAnchor(_ *cobra.Command, args []string) {
    lightDb = ""  // Explicitly disable the Light client
    h := &healer{
        // ... implementation
    }
    h.heal(args)
}
```

- Sets `lightDb = ""` to disable the Light client
- Queries validator nodes directly for signatures
- Does not require local storage of chain data

#### 2. Version-specific Healing Logic {#version-specific-healing}

```go
// In healing/anchors.go
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

- Automatically selects between V1 and V2 healing based on network version
- Special handling for Directory Network (DN) to Blockchain Validation Network (BVN) anchors
- V2 healing for DN→BVN anchors uses signatures from the DN→DN anchor

#### 3. Signature Collection from Validators {#signature-collection}

```go
// In healing/anchors.go
// Get a signature from each node that hasn't signed
var gotPartSig bool
var signatures []protocol.Signature
for peer, info := range args.NetInfo.Peers[strings.ToLower(si.Source)] {
    if signed[info.Key] {
        continue
    }
    
    // Query the node for its signature
    res, err := args.Client.ForAddress(addr).Private().Sequence(ctx, srcUrl.JoinPath(protocol.AnchorPool), dstUrl, si.Number, private.SequenceOptions{})
    // ... process signature
}
```

- Queries individual validator nodes directly for their signatures
- Tracks which validators have already signed to avoid duplicate requests
- Collects both partition signatures and user signatures
- Verifies signatures before including them

##### Why Signatures Are Distributed

Signatures in the Accumulate network are intentionally distributed across validator nodes for several important reasons:

1. **Consensus Security Model**: Each validator independently validates and signs transactions using its own private key. This distributed approach is fundamental to the network's security model, ensuring no single entity can compromise transaction integrity.

2. **Independent Validation**: Every validator must independently verify a transaction according to the network's rules before signing it. This ensures multiple parties have confirmed the transaction's validity.

3. **No Automatic Sharing**: Signatures are not automatically propagated to all nodes in the network. Each validator primarily stores its own signatures, which is why the healing process must actively query each validator.

4. **Threshold-Based Approach**: The system requires signatures from a threshold number of validators (not all validators) to consider a transaction valid. This design allows the network to function even if some validators are offline.

##### Implementation Implications

- The healing process must query each validator individually to collect signatures
- Any peer can be used to submit healing transactions once enough signatures are collected
- The caching system tracks problematic nodes to avoid querying them repeatedly
- The code is designed to be resilient to node failures during signature collection

#### 4. Threshold Verification {#threshold-verification}

```go
// In healing/anchors.go
threshold := g.ValidatorThreshold(si.Source)

if len(signed) >= int(threshold) {
    slog.InfoContext(ctx, "Sufficient signatures have been received")
    return errors.Delivered
}
```

- Checks if enough validators have signed to meet the threshold
- Returns early if the threshold is already met
- Uses the network's validator threshold for the source partition

#### 5. Version-specific Message Construction {#version-specific-message}

```go
// In healing/anchors.go
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

- V2 networks: Creates and submits individual `BlockAnchor` messages
- V1 networks: Creates a batch with the transaction and all signatures
- Adapts message format based on network version

#### 6. URL Construction for Anchors {#url-construction-anchors}

```go
// In healing/anchors.go
srcUrl := protocol.PartitionUrl(si.Source)
dstUrl := protocol.PartitionUrl(si.Destination)

// ... later in the code
res, err := args.Client.ForAddress(addr).Private().Sequence(ctx, srcUrl.JoinPath(protocol.AnchorPool), dstUrl, si.Number, private.SequenceOptions{})
```

- Uses `protocol.PartitionUrl()` to create the base URL for each partition
- Appends `protocol.AnchorPool` to the source URL to access the anchor pool
- This creates URLs like `acc://bvn-Apollo.acme/anchors`
- Passes the raw destination URL (e.g., `acc://bvn-Mocha.acme`)

**Related Documentation**:
- [URL Construction Considerations](./implementation.md#url-construction)
- [Implementation Guidelines](./implementation.md#bright-line-guidelines)

#### 7. Retry Logic {#retry-logic}

```go
// In heal_anchor.go
func (h *healer) healSingleAnchor(srcId, dstId string, seqNum uint64, txid *url.TxID, txns map[[32]byte]*protocol.Transaction) bool {
    var count int
retry:
    err := healing.HealAnchor(h.ctx, healing.HealAnchorArgs{
        // ... args
    }, healing.SequencedInfo{
        // ... info
    })
    
    // ... error handling
    
    count++
    if count >= 10 {
        slog.Error("Anchor still pending, skipping", "attempts", count)
        return false
    }
    slog.Error("Anchor still pending, retrying", "attempts", count)
    goto retry
}
```

- Implements retry logic for pending anchors
- Limits retries to 10 attempts
- Uses `goto` for efficient retry implementation

## heal_synth Transaction Creation {#heal_synth-transaction-creation}

The synthetic transaction healing process creates transactions as follows:

```go
// In heal_synth.go
func healSingleSynth(h *healer, source, destination string, number uint64, id *url.TxID) bool {
    var count int
retry:
    err := h.HealSynthetic(h.ctx, healing.HealSyntheticArgs{
        Client:    h.C2.ForAddress(nil),
        Querier:   h.C2,
        Submitter: h.C2,
        NetInfo:   h.net,
        Light:     h.light,
        Pretend:   pretend,
        Wait:      waitForTxn,
        
        // If an attempt fails, use the next anchor
        SkipAnchors: count,
    }, healing.SequencedInfo{
        Source:      source,
        Destination: destination,
        Number:      number,
        ID:          id,
    })
    // ... error handling
    
    count++
    if count >= 3 {
        slog.Error("Message still pending, skipping", "attempts", count)
        return false
    }
    slog.Error("Message still pending, trying next anchor", "attempts", count)
    goto retry
}
```

**Key Files**:
- `heal_synth.go`: Main implementation
- `healing/synthetic.go`: Core functionality for synthetic transaction healing

**Related Documentation**:
- [heal_synth Approach](./overview.md#heal-synth)
- [Light Client Usage](./light_client.md#light-client-usage-in-synthetic-healing)
- [Database Requirements](./database.md#database-usage-in-healing-approaches)

### Unique aspects of synthetic healing {#unique-aspects-synth}

#### 1. Database Requirement {#database-requirement}

The `heal_synth` approach requires a database to build Merkle proofs:

```go
// In heal_common.go
func (h *healer) initDB() error {
    if lightDb != "" {
        // ... initialize database
        h.light, err = light.NewClient(
            light.Store(db, ""),
            light.ClientV2(cv2),
            light.Querier(h.C2),
            light.Router(h.router),
        )
        // ... error handling
    }
    return nil
}
```

- Requires a database to store chain data
- Uses the Light client to access and manipulate the database
- Cannot function without a properly initialized database

#### 2. Light Client Usage {#light-client-usage}

The `heal_synth` approach uses the Light client extensively to build Merkle proofs:

```go
// In heal_synth.go
func pullSynthDirChains(h *healer) {
    ctx, cancel, _ := api.ContextWithBatchData(h.ctx)
    defer cancel()

    check(h.light.PullAccount(ctx, protocol.DnUrl().JoinPath(protocol.Network)))

    check(h.light.PullAccountWithChains(ctx, protocol.DnUrl().JoinPath(protocol.Ledger), includeRootChain))
    check(h.light.IndexAccountChains(ctx, protocol.DnUrl().JoinPath(protocol.Ledger)))

    check(h.light.PullAccountWithChains(ctx, protocol.DnUrl().JoinPath(protocol.AnchorPool), func(c *api.ChainRecord) bool {
        return c.Type == merkle.ChainTypeAnchor || c.IndexOf != nil && c.IndexOf.Type == merkle.ChainTypeAnchor
    }))
    check(h.light.IndexAccountChains(ctx, protocol.DnUrl().JoinPath(protocol.AnchorPool)))
}
```

- Uses the Light client to pull account data
- Uses the Light client to pull chain data
- Uses the Light client to index account chains
- These operations are essential for building Merkle proofs

#### 3. Version-specific Receipt Building {#version-specific-receipt-building}

The `heal_synth` approach includes version-specific logic for building Merkle receipts:

```go
// In healing/synthetic.go
// Build the receipt
var receipt *merkle.Receipt
if args.NetInfo.Status.ExecutorVersion.V2VandenbergEnabled() {
    receipt, err = h.buildSynthReceiptV2(ctx, args, si)
} else {
    receipt, err = h.buildSynthReceiptV1(ctx, args, si)
}
if err != nil {
    return err
}
```

- V1 networks: Uses `buildSynthReceiptV1` to build the receipt
- V2 networks: Uses `buildSynthReceiptV2` to build the receipt
- The receipt building process is significantly different between versions

#### 4. V2 Receipt Building Process {#v2-receipt-building}

For V2 networks, the receipt building process is more complex:

```go
// In healing/synthetic.go
func (h *Healer) buildSynthReceiptV2(_ context.Context, args HealSyntheticArgs, si SequencedInfo) (*merkle.Receipt, error) {
    batch := args.Light.OpenDB(false)
    defer batch.Discard()
    uSrc := protocol.PartitionUrl(si.Source)
    uSrcSys := uSrc.JoinPath(protocol.Ledger)
    uSrcSynth := uSrc.JoinPath(protocol.Synthetic)
    uDn := protocol.DnUrl()
    uDnSys := uDn.JoinPath(protocol.Ledger)
    uDnAnchor := uDn.JoinPath(protocol.AnchorPool)

    // Load the synthetic sequence chain entry
    b, err := batch.Account(uSrcSynth).SyntheticSequenceChain(si.Destination).Entry(int64(si.Number) - 1)
    // ... error handling
    
    // Locate the synthetic ledger main chain index entry
    mainIndex, err := batch.Index().Account(uSrcSynth).Chain("main").SourceIndex().FindIndexEntryAfter(seqEntry.Source)
    // ... error handling
    
    // Build the synthetic ledger part of the receipt
    receipt, err := batch.Account(uSrcSynth).MainChain().Receipt(seqEntry.Source, mainIndex.Source)
    // ... error handling
    
    // Locate the BVN root index entry
    bvnRootIndex, err := batch.Index().Account(uSrcSys).Chain("root").SourceIndex().FindIndexEntryAfter(mainIndex.Anchor)
    // ... error handling
    
    // Build the BVN part of the receipt
    bvnReceipt, err := batch.Account(uSrcSys).RootChain().Receipt(mainIndex.Anchor, bvnRootIndex.Source)
    // ... error handling
    receipt, err = receipt.Combine(bvnReceipt)
    // ... error handling
    
    // If the source is the DN we don't need to do anything else
    if strings.EqualFold(si.Source, protocol.Directory) {
        return receipt, nil
    }
    
    // Locate the DN-BVN anchor entry
    dnBvnAnchorChain := batch.Account(uDnAnchor).AnchorChain(si.Source).Root()
    // ... more receipt building steps
    
    return receipt, nil
}
```

- Builds a multi-part receipt that includes:
  1. The synthetic ledger part
  2. The BVN part
  3. The DN-BVN part (if source is not the DN)
  4. The DN part (if source is not the DN)
- Each part requires locating specific chain entries and building receipts
- The parts are combined to form a complete receipt

#### 5. Synthetic Message Construction {#synthetic-message-construction}

```go
// In healing/synthetic.go
// Submit the synthetic transaction directly to the destination partition
msg := &messaging.SyntheticMessage{
    Message: r.Sequence,
    Proof: &protocol.AnnotatedReceipt{
        Receipt: receipt,
        Anchor: &protocol.AnchorMetadata{
            Account: protocol.DnUrl(),
        },
    },
}
for _, sigs := range r.Signatures.Records {
    for _, sig := range sigs.Signatures.Records {
        sig, ok := sig.Message.(*messaging.SignatureMessage)
        if !ok {
            continue
        }
        ks, ok := sig.Signature.(protocol.KeySignature)
        if !ok {
            continue
        }
        msg.Signature = ks
    }
}
if msg.Signature == nil {
    return fmt.Errorf("synthetic message is not signed")
}

if !msg.Signature.Verify(nil, msg.Message) {
    return fmt.Errorf("signature is not valid")
}
```

- Creates a synthetic message with the original transaction and the Merkle proof
- Extracts and includes the signature from the original transaction
- Verifies the signature before submission
- The synthetic message includes all the information needed to prove the transaction was included in the source partition

#### 6. Version-specific Message Envelope {#version-specific-message-envelope}

```go
// In healing/synthetic.go
env := new(messaging.Envelope)

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

- V2 Baikonur networks: Uses `SyntheticMessage`
- Earlier networks: Uses `BadSyntheticMessage`
- Adapts message format based on network version

#### 7. Retry Logic {#synth-retry-logic}

```go
// In heal_synth.go
func healSingleSynth(h *healer, source, destination string, number uint64, id *url.TxID) bool {
    var count int
retry:
    err := h.HealSynthetic(h.ctx, healing.HealSyntheticArgs{
        // ... args
        
        // If an attempt fails, use the next anchor
        SkipAnchors: count,
    }, healing.SequencedInfo{
        // ... info
    })
    // ... error handling
    
    count++
    if count >= 3 {
        slog.Error("Message still pending, skipping", "attempts", count)
        return false
    }
    slog.Error("Message still pending, trying next anchor", "attempts", count)
    goto retry
}
```

- Implements retry logic for pending synthetic transactions
- Limits retries to 3 attempts (compared to 10 for anchor healing)
- Uses `SkipAnchors` to try different anchors on each attempt
- Uses `goto` for efficient retry implementation

## Key Differences Between Approaches {#key-differences-between-approaches}

| Feature | heal_anchor | heal_synth |
|---------|------------|------------|
| Database Requirement | No database required | Requires database for Merkle proofs |
| Light Client | Explicitly disabled | Required for chain access |
| Transaction Type | Anchor transactions | Synthetic transactions |
| Signature Collection | Queries validators directly | N/A (uses Merkle proofs) |
| URL Construction | Uses partition URLs with anchor pool | Uses partition URLs with synthetic |
| Version Handling | Different logic for V1 and V2 | Different receipt building for V1 and V2 |
| Retry Logic | Up to 10 retries | Up to 3 retries with different anchors |
| Proof Construction | N/A | Builds complex multi-part Merkle proofs |

**Related Documentation**:
- [Choosing an Approach](./overview.md#choosing-an-approach)
- [Implementation Guidelines](./implementation.md#bright-line-guidelines)
