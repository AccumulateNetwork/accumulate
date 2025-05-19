# Receipt Creation and Signature Gathering

## Introduction

Receipt creation and signature gathering are fundamental processes in Accumulate's healing mechanisms. Receipts provide cryptographic proof of transaction inclusion, while signatures validate the authenticity of transactions and anchors. This document details how these processes work within the healing framework.

## Cryptographic Receipts

### What is a Receipt?

In Accumulate, a receipt is a cryptographic proof that demonstrates a transaction has been included in the blockchain. It consists of a series of hashes that form a path from the transaction to a known root hash.

```go
// From pkg/types/merkle/receipt.go
type Receipt struct {
    Start  []byte
    Anchor []byte
    Entries []ReceiptEntry
}

type ReceiptEntry struct {
    Hash []byte
    Right bool
}
```

A receipt contains:
- **Start**: The hash of the transaction or data being proven
- **Anchor**: The root hash that the proof leads to
- **Entries**: A series of hashes and directions (left/right) that form the Merkle path

### Receipt Creation in Synthetic Healing

The synthetic healing process creates receipts to prove the validity of synthetic transactions:

```go
// From internal/core/healing/synthetic.go
func (h *Healer) buildSynthReceiptV2(ctx context.Context, args HealSyntheticArgs, si SequencedInfo) (*merkle.Receipt, error) {
    batch := args.Light.OpenDB(false)
    defer batch.Discard()

    // Load the synthetic sequence chain entry
    uSrcSynth := protocol.PartitionUrl(si.Source).JoinPath(protocol.Synthetic)
    b, err := batch.Account(uSrcSynth).SyntheticSequenceChain(si.Destination).Entry(int64(si.Number) - 1)
    if err != nil {
        return nil, errors.UnknownError.WithFormat(
            "load synthetic sequence chain entry %d: %w", si.Number, err)
    }
    seqEntry := new(protocol.IndexEntry)
    err = seqEntry.UnmarshalBinary(b)
    if err != nil {
        return nil, err
    }

    // Locate the synthetic ledger main chain index entry
    mainIndex, err := batch.Index().Account(uSrcSynth).Chain("main").SourceIndex().FindIndexEntryAfter(seqEntry.Source)
    if err != nil {
        return nil, errors.UnknownError.WithFormat(
            "locate synthetic ledger main chain index entry after %d: %w", seqEntry.Source, err)
    }

    // Build the synthetic ledger part of the receipt
    receipt, err := batch.Account(uSrcSynth).MainChain().Receipt(seqEntry.Source, mainIndex.Source)
    if err != nil {
        return nil, errors.UnknownError.WithFormat(
            "build synthetic ledger receipt: %w", err)
    }

    // Additional receipt building steps...
    // ...

    return receipt, nil
}
```

This function builds a receipt that proves the synthetic transaction is included in the blockchain by:

1. Loading the synthetic sequence chain entry
2. Locating the main chain index entry
3. Building a receipt from the sequence entry to the main chain
4. Extending the receipt to include the BVN and DN parts

### Receipt Creation in Anchor Healing

The anchor healing process also creates receipts, but with a focus on proving the validity of anchors:

```go
// From internal/core/healing/anchors.go
func buildAnchorReceipt(ctx context.Context, batch *database.Batch, src, dst string, number uint64) (*merkle.Receipt, error) {
    // Load the anchor sequence chain entry
    uSrcAnchor := protocol.PartitionUrl(src).JoinPath(protocol.AnchorPool)
    b, err := batch.Account(uSrcAnchor).AnchorSequenceChain(dst).Entry(int64(number) - 1)
    if err != nil {
        return nil, errors.UnknownError.WithFormat(
            "load anchor sequence chain entry %d: %w", number, err)
    }
    seqEntry := new(protocol.IndexEntry)
    err = seqEntry.UnmarshalBinary(b)
    if err != nil {
        return nil, err
    }

    // Locate the anchor ledger main chain index entry
    mainIndex, err := batch.Index().Account(uSrcAnchor).Chain("main").SourceIndex().FindIndexEntryAfter(seqEntry.Source)
    if err != nil {
        return nil, errors.UnknownError.WithFormat(
            "locate anchor ledger main chain index entry after %d: %w", seqEntry.Source, err)
    }

    // Build the anchor ledger part of the receipt
    receipt, err := batch.Account(uSrcAnchor).MainChain().Receipt(seqEntry.Source, mainIndex.Source)
    if err != nil {
        return nil, errors.UnknownError.WithFormat(
            "build anchor ledger receipt: %w", err)
    }

    // Additional receipt building steps...
    // ...

    return receipt, nil
}
```

The anchor receipt proves that an anchor is included in the blockchain, which is essential for validating cross-partition references.

## Signature Gathering

### What are Signatures?

Signatures in Accumulate are cryptographic proofs that a transaction or anchor has been approved by a validator. They are essential for the consensus mechanism and for validating the authenticity of transactions and anchors.

```go
// From pkg/types/messaging/v1/signature.go
type SignatureMessage struct {
    Signature protocol.Signature
    Signer    *url.URL
    SignerVersion uint64
    TransactionHash [32]byte
    Timestamp time.Time
}
```

A signature message contains:
- **Signature**: The cryptographic signature
- **Signer**: The identity of the validator that created the signature
- **SignerVersion**: The version of the signer
- **TransactionHash**: The hash of the transaction being signed
- **Timestamp**: When the signature was created

### Signature Gathering in Synthetic Healing

The synthetic healing process gathers signatures to validate synthetic transactions:

```go
// From internal/core/healing/synthetic.go
// Fetch the transaction and signatures
var sigSets []*api.SignatureSetRecord
Q := api.Querier2{Querier: args.Querier}
res, err := Q.QueryMessage(ctx, si.ID, nil)
switch {
case err == nil:
    if res.Status.Delivered() {
        slog.InfoContext(ctx, "Synthetic message has been delivered", "id", si.ID, "source", si.Source, "destination", si.Destination, "number", si.Number)
        return errors.Delivered
    }
    sigSets = res.Signatures.Records
    txn = res.Message.(*messaging.TransactionMessage).Transaction

case !errors.Is(err, errors.NotFound):
    return err
}
```

This code fetches existing signatures for a synthetic transaction. If the transaction has been delivered, it returns an error to indicate that healing is not needed.

### Signature Gathering in Anchor Healing

The anchor healing process includes more complex signature gathering, as it needs to collect signatures from multiple validators:

```go
// From internal/core/healing/anchors.go
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
    
    ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()
    
    slog.InfoContext(ctx, "Querying node for its signature", "id", peer)
    res, err := args.Client.ForAddress(addr).Private().Sequence(ctx, srcUrl.JoinPath(protocol.AnchorPool), dstUrl, si.Number, private.SequenceOptions{})
    if err != nil {
        slog.ErrorContext(ctx, "Query failed", "error", err)
        continue
    }
    
    // Process the signature
    // ...
}
```

This code tracks which validators have already signed the anchor and queries each validator that hasn't signed yet to collect their signature.

### Signature Validation

Before using signatures, the healing processes validate them to ensure they are authentic:

```go
// From internal/core/healing/anchors.go
// Filter out bad signatures
if !sig.Verify(nil, seq) {
    slog.ErrorContext(ctx, "Node gave us an invalid signature", "id", info)
    continue
}
```

This validation ensures that only valid signatures are included in the healed transaction or anchor.

## Receipt and Signature Combination

The healing processes combine receipts and signatures to create a complete proof of transaction or anchor validity:

```go
// From internal/core/healing/synthetic.go
// Create a synthetic transaction with the receipt
synth := new(protocol.SyntheticTransaction)
synth.Transaction = txn
synth.Proof = receipt

// Create a transaction message with the synthetic transaction
msg := new(messaging.TransactionMessage)
msg.Transaction = synth

// Submit the transaction with signatures
err = args.Submit(msg)
if err != nil {
    return err
}
```

This combination ensures that the healed transaction or anchor includes both the transaction data and the proof of its validity.

## Receipt Verification

The healing processes verify receipts to ensure they are valid before using them:

```go
// From internal/core/healing/synthetic.go
// Verify the receipt
err = receipt.Verify()
if err != nil {
    return nil, errors.UnknownError.WithFormat(
        "verify receipt: %w", err)
}
```

This verification ensures that the receipt is cryptographically valid and provides a proper proof of transaction inclusion.

## Version-Specific Receipt Creation

The healing processes include version-specific receipt creation to handle different network protocol versions:

```go
// From internal/core/healing/synthetic.go
func (h *Healer) HealSynthetic(ctx context.Context, args HealSyntheticArgs, si SequencedInfo) error {
    // If the network is running Vandenberg, use version 2
    if args.NetInfo.Status.ExecutorVersion.V2VandenbergEnabled() {
        return h.healSyntheticV2(ctx, args, si)
    }
    return h.healSyntheticV1(ctx, args, si)
}
```

This version-specific approach ensures that the receipt creation process is compatible with the network protocol version.

## Receipt and Signature Storage

The healing processes store receipts and signatures in the transaction or anchor data:

```go
// From internal/core/healing/synthetic.go
// Create a synthetic transaction with the receipt
synth := new(protocol.SyntheticTransaction)
synth.Transaction = txn
synth.Proof = receipt

// Create a transaction message with the synthetic transaction
msg := new(messaging.TransactionMessage)
msg.Transaction = synth
```

This storage ensures that the receipt and signatures are available for verification when the transaction or anchor is processed.

## Recent Changes

Recent changes to the receipt creation and signature gathering processes include:

1. **Improved Error Handling**: Enhanced error handling to better handle receipt creation and signature gathering errors
2. **Version-Specific Receipt Creation**: Implemented version-specific receipt creation to handle different network protocol versions
3. **Signature Validation**: Enhanced signature validation to ensure only valid signatures are included

## Best Practices

When working with receipt creation and signature gathering in the healing processes, it's important to follow these best practices:

1. **Verify Receipts**: Always verify receipts to ensure they are cryptographically valid
2. **Validate Signatures**: Always validate signatures to ensure they are authentic
3. **Handle Version Differences**: Include version-specific logic to handle different network protocol versions
4. **Follow Data Integrity Rules**: Never fabricate or fake receipts or signatures that aren't available from the network

```go
// Example of following data integrity rules
// From internal/core/healing/synthetic.go
// Verify the receipt
err = receipt.Verify()
if err != nil {
    return nil, errors.UnknownError.WithFormat(
        "verify receipt: %w", err)
}
```

## Conclusion

Receipt creation and signature gathering are critical components of the healing processes in Accumulate. By creating valid receipts and collecting authentic signatures, the healing processes can ensure that healed transactions and anchors are properly validated and accepted by the network.

In the next document, we will explore the reporting system used by the healing processes to track and report on healing progress.
