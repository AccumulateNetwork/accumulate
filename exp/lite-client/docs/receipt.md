# Querying Account State Receipts with Merkle Proofs

This guide explains how to use the Accumulate APIs to query for account state receipts that contain Merkle roots and siblings, enabling you to construct Merkle paths to prove current account states.

## Overview

Account state receipts in Accumulate are cryptographic proofs that link an account's current state to the blockchain's Merkle root. These receipts contain:

- **Merkle Root (Anchor)**: The root hash of the Merkle tree at a specific block
- **Start Hash**: The hash of the account state being proven
- **Merkle Siblings (Entries)**: The sibling hashes needed to reconstruct the path from the start hash to the root
- **Block Information**: The block index and timestamp when the state was anchored

## Receipt Structure

The core receipt structure is defined in `pkg/database/merkle/receipt.go`:

```go
type Receipt struct {
    Start      []byte         // Hash of the element being proven
    StartIndex int64          // Index of the start element
    End        []byte         // Hash of the end element (if range)
    EndIndex   int64          // Index of the end element
    Anchor     []byte         // Merkle root hash
    Entries    []*ReceiptEntry // Merkle sibling hashes for proof construction
}
```

Each `ReceiptEntry` contains:
- `Hash`: The sibling hash
- `Right`: Boolean indicating if this hash goes on the right side of the proof

## Querying Account State Receipts

### Using Client API v2

The simplest way to query for account state receipts is using the client API:

```go
package main

import (
    "context"
    "fmt"
    
    "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
    "gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func queryAccountWithReceipt(client *api.Client, accountURL string) error {
    // Parse the account URL
    u, err := url.Parse(accountURL)
    if err != nil {
        return fmt.Errorf("parse URL: %w", err)
    }
    
    // Create query with receipt options
    query := &api.GeneralQuery{
        Url:            u,
        IncludeReceipt: &api.ReceiptOptions{ForAny: true}, // Request receipt
    }
    
    // Execute the query
    resp, err := client.Query(context.Background(), query)
    if err != nil {
        return fmt.Errorf("query account: %w", err)
    }
    
    // Access the account record and receipt
    if accountRecord := resp.GetAccount(); accountRecord != nil {
        fmt.Printf("Account: %v\n", accountRecord.Account.GetUrl())
        
        if receipt := accountRecord.Receipt; receipt != nil {
            fmt.Printf("Receipt Anchor (Merkle Root): %x\n", receipt.Anchor)
            fmt.Printf("Start Hash: %x\n", receipt.Start)
            fmt.Printf("Block Index: %d\n", receipt.LocalBlock)
            fmt.Printf("Number of Merkle Siblings: %d\n", len(receipt.Entries))
            
            // Print each sibling hash
            for i, entry := range receipt.Entries {
                side := "left"
                if entry.Right {
                    side = "right"
                }
                fmt.Printf("  Sibling %d (%s): %x\n", i, side, entry.Hash)
            }
        }
    }
    
    return nil
}
```

### Using Internal API v3

For more advanced use cases, you can use the internal API v3 querier:

```go
// From internal/api/v3/querier.go
func (s *Querier) queryAccount(ctx context.Context, batch *database.Batch, record *database.Account, wantReceipt *api.ReceiptOptions) (*api.AccountRecord, error) {
    r := new(api.AccountRecord)
    
    // Load account state
    state, err := record.Main().Get()
    if err != nil {
        return nil, errors.UnknownError.WithFormat("load state: %w", err)
    }
    r.Account = state
    
    // Skip receipt if not requested
    if !wantReceipt.Yes() {
        return r, nil
    }
    
    // Get receipt for account state
    block, receipt, err := indexing.ReceiptForAccountState(s.partition, batch, record)
    if err != nil {
        return nil, errors.UnknownError.WithFormat("get state receipt: %w", err)
    }
    
    // Populate receipt information
    r.Receipt = new(api.Receipt)
    r.Receipt.Receipt = *receipt
    r.Receipt.LocalBlock = block.BlockIndex
    if block.BlockTime != nil {
        r.Receipt.LocalBlockTime = *block.BlockTime
    }
    
    return r, nil
}
```

## How Account State Receipts Are Generated

The receipt generation process involves several steps, as implemented in `internal/database/indexing/receipts.go`:

```go
func ReceiptForAccountState(partition config.NetworkUrl, batch *database.Batch, account *database.Account) (block *protocol.IndexEntry, receipt *merkle.Receipt, err error) {
    // Get a receipt from the BPT (Blockchain Partition Tree)
    r, err := account.StateReceipt()
    if err != nil {
        return nil, nil, errors.UnknownError.WithFormat("get account state receipt: %w", err)
    }
    
    // Load the latest root index entry (for block information)
    ledger := batch.Account(partition.Ledger())
    rootEntry, err := LoadIndexEntryFromEnd(ledger.RootChain().Index(), 1)
    if err != nil {
        return nil, nil, errors.UnknownError.Wrap(err)
    }
    
    return rootEntry, r, nil
}
```

The `Account.StateReceipt()` method combines two receipts:

```go
// From internal/database/bpt_account.go
func (a *Account) StateReceipt() (*merkle.Receipt, error) {
    // Get the account state hasher
    hasher, err := a.parent.observer.DidChangeAccount(a.parent, a)
    if err != nil {
        return nil, err
    }
    
    // Get BPT receipt for this account
    rBPT, err := a.BptReceipt()
    if err != nil {
        return nil, err
    }
    
    // Generate receipt for the account state
    rState := hasher.Receipt(0, len(hasher)-1)
    
    // Verify consistency between state and BPT
    if !bytes.Equal(rState.Anchor, rBPT.Start) {
        return nil, errors.InternalError.With("bpt entry does not match account state")
    }
    
    // Combine the receipts
    receipt, err := rState.Combine(rBPT)
    if err != nil {
        return nil, fmt.Errorf("combine receipt: %w", err)
    }
    
    return receipt, nil
}
```

## Validating Merkle Proofs

Once you have a receipt, you can validate the Merkle proof to verify the account state:

```go
func validateReceipt(receipt *merkle.Receipt) bool {
    // The Validate method reconstructs the Merkle root from the start hash
    // by applying each sibling hash in the entries
    return receipt.Validate(nil)
}

// Example validation with error handling
func validateReceiptWithDetails(receipt *merkle.Receipt) error {
    if receipt == nil {
        return fmt.Errorf("receipt is nil")
    }
    
    if len(receipt.Entries) == 0 {
        return fmt.Errorf("receipt has no merkle siblings")
    }
    
    if len(receipt.Anchor) == 0 {
        return fmt.Errorf("receipt has no anchor (merkle root)")
    }
    
    if !receipt.Validate(nil) {
        return fmt.Errorf("merkle proof validation failed")
    }
    
    fmt.Printf("✓ Merkle proof is valid!\n")
    fmt.Printf("  Start hash: %x\n", receipt.Start)
    fmt.Printf("  Reconstructed root: %x\n", receipt.Anchor)
    
    return nil
}
```

## Manual Merkle Path Construction

The validation process manually reconstructs the Merkle root by applying sibling hashes:

```go
// From pkg/database/merkle/receipt.go - simplified version of Validate()
func reconstructMerkleRoot(receipt *merkle.Receipt) []byte {
    // Start with the element hash
    currentHash := receipt.Start
    
    // Apply each sibling hash in the proof
    for _, entry := range receipt.Entries {
        if entry.Right {
            // Sibling goes on the right: hash(current, sibling)
            currentHash = hash(currentHash, entry.Hash)
        } else {
            // Sibling goes on the left: hash(sibling, current)
            currentHash = hash(entry.Hash, currentHash)
        }
    }
    
    // The final hash should match the anchor (Merkle root)
    return currentHash
}
```

## CLI Example

You can also query account receipts using the Accumulate CLI:

```bash
# Query account with receipt
accumulate account get acc://example.acme/account --receipt

# The CLI will validate and display the receipt automatically
# Output includes receipt validation status and merkle proof details
```

The CLI output is handled in `cmd/accumulate/cmd/output.go`:

```go
func (x *Context) PrintTransactionQueryResponseV2(res *client.TransactionQueryResponse) (string, error) {
    for _, receipt := range res.Receipts {
        out += fmt.Sprintf("Receipt from %v#chain/%s\n", receipt.Account, receipt.Chain)
        
        if receipt.Error != "" {
            out += fmt.Sprintf("  Error!! %s\n", receipt.Error)
        }
        
        // Validate the merkle proof
        if !receipt.Proof.Validate(nil) {
            out += fmt.Sprintf("  Invalid!!\n")
        } else {
            out += fmt.Sprintf("  ✓ Valid merkle proof\n")
        }
    }
    return out, nil
}
```

## Key Points

1. **Receipt Options**: Always set `IncludeReceipt: &api.ReceiptOptions{ForAny: true}` to request receipts
2. **Validation**: Use `receipt.Validate(nil)` to verify the Merkle proof
3. **Block Context**: Receipts include block index and timestamp for temporal verification
4. **BPT Integration**: Account state receipts are anchored to the Blockchain Partition Tree
5. **Error Handling**: Always check for receipt errors and validation failures

## Use Cases

Account state receipts enable:
- **State Verification**: Prove an account's state at a specific block
- **Audit Trails**: Verify historical account states
- **Cross-Chain Proofs**: Export proofs for use in other systems
- **Light Client Support**: Verify states without full blockchain data

This cryptographic proof system ensures the integrity and authenticity of account states in the Accumulate network.
