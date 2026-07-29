# Accumulate Receipt Architecture

This document describes how Merkle receipts are constructed in Accumulate, mapping from individual account states and transactions through partition-level proofs to the global Directory Network (DN) AppHash.

## Overview

Accumulate uses a hierarchical proof structure:

1. **Account Level**: Account state hashes stored in the Binary Patricia Tree (BPT)
2. **Partition Level**: BPT root becomes the partition's AppHash (used in CometBFT consensus)
3. **Global Level**: BVN anchors are recorded in the DN, allowing global verification

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Directory Network (DN)                             │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                         DN BPT Root (DN AppHash)                     │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    ▲                                        │
│                    ┌───────────────┼───────────────┐                       │
│                    │               │               │                        │
│              ┌─────┴─────┐   ┌─────┴─────┐   ┌─────┴─────┐                 │
│              │DN Accounts│   │BVN0 Anchor│   │BVN1 Anchor│  ...            │
│              │  States   │   │  Chains   │   │  Chains   │                 │
│              └───────────┘   └─────┬─────┘   └─────┬─────┘                 │
└──────────────────────────────────────────────────────────────────────────────┘
                                     │               │
                    ┌────────────────┘               └────────────────┐
                    │                                                 │
                    ▼                                                 ▼
┌─────────────────────────────────────┐     ┌─────────────────────────────────────┐
│              BVN0                    │     │              BVN1                    │
│                                      │     │                                      │
│  ┌────────────────────────────────┐ │     │  ┌────────────────────────────────┐ │
│  │   BVN0 BPT Root (BVN0 AppHash) │ │     │  │   BVN1 BPT Root (BVN1 AppHash) │ │
│  └────────────────────────────────┘ │     │  └────────────────────────────────┘ │
│                  ▲                   │     │                  ▲                   │
│      ┌───────────┼───────────┐      │     │      ┌───────────┼───────────┐      │
│      │           │           │      │     │      │           │           │      │
│  ┌───┴───┐   ┌───┴───┐   ┌───┴───┐ │     │  ┌───┴───┐   ┌───┴───┐   ┌───┴───┐ │
│  │ Acct1 │   │ Acct2 │   │ Acct3 │ │     │  │ Acct4 │   │ Acct5 │   │ Acct6 │ │
│  │ State │   │ State │   │ State │ │     │  │ State │   │ State │   │ State │ │
│  └───────┘   └───────┘   └───────┘ │     │  └───────┘   └───────┘   └───────┘ │
└──────────────────────────────────────┘     └──────────────────────────────────────┘
```

## Account State Hash Components

Each account's state hash is computed from multiple components (see `observer_prod.go`):

```go
func (a *observedAccount) hashState() (hash.Hasher, error) {
    hashState(&err, &hasher, true, a.Main().Get)          // Main account state
    hashState(&err, &hasher, false, a.hashSecondaryState) // Directory list + events
    hashState(&err, &hasher, false, a.hashChains)         // All chain anchors
    hashState(&err, &hasher, false, a.hashPending)        // Pending transactions
    return hasher, err
}
```

| Component | Description | Affects Hash When |
|-----------|-------------|-------------------|
| Main State | Account struct (TokenAccount, DataAccount, etc.) | Balance changes, data writes to state |
| Secondary State | Directory list, scheduled events | Sub-accounts created |
| Chains | Anchor of each chain (main, data, scratch) | Any chain entry added |
| Pending | Pending transaction hashes and signatures | Transactions in flight |

## Receipt Types

### 1. Account State Receipt (`StateReceipt`)

Proves an account's current state is included in the partition's BPT.

**Path**: Account State Elements → Account State Hash → BPT Path → BPT Root (AppHash)

```go
// internal/database/bpt_account.go
func (a *Account) StateReceipt() (*merkle.Receipt, error) {
    hasher := observer.DidChangeAccount(batch, account)  // Compute state hash
    rBPT := a.BptReceipt()                               // Get BPT proof
    rState := hasher.Receipt(0, len(hasher)-1)           // State elements proof
    return rState.Combine(rBPT)                          // Combined receipt
}
```

**Receipt Structure**:
- `Start`: First state element hash
- `Entries`: Merkle path through state elements + BPT
- `Anchor`: BPT Root (partition AppHash)

### 2. BPT Receipt (`BptReceipt`)

Proves an account's state hash is in the BPT.

**Path**: Account State Hash → BPT Path → BPT Root (AppHash)

```go
func (a *Account) BptReceipt() (*merkle.Receipt, error) {
    return a.parent.BPT().GetReceipt(a.key)
}
```

### 3. Chain Entry Receipt

Proves a specific chain entry (transaction, data entry) is anchored in the root chain.

**Path**: Chain Entry → Chain Anchor → Root Chain → Root Chain Anchor

```go
// internal/database/indexing/receipts.go
func ReceiptForChainEntry(...) (*merkle.Receipt, error) {
    accountReceipt := chain.Receipt(entryIndex, indexEntry.Source)  // Entry → chain anchor
    rootReceipt := getRootReceipt(partition, batch, ...)            // Chain → root chain
    return accountReceipt.Combine(rootReceipt)
}
```

## Cross-Partition Anchoring

### BVN → DN Anchoring

Each BVN periodically sends a `BlockValidatorAnchor` transaction to the DN containing:

| Field | Description |
|-------|-------------|
| `RootChainAnchor` | Anchor of the BVN's root chain |
| `StateTreeAnchor` | BVN's BPT root (BVN AppHash) |
| `MinorBlockIndex` | Block number on the BVN |
| `RootChainIndex` | Index in the root chain |

The DN stores these in anchor chains per BVN:
- `acc://dn.acme/anchors` - AnchorLedger account
- `AnchorChain(bvnName).Root()` - Stores RootChainAnchors
- `AnchorChain(bvnName).BPT()` - Stores StateTreeAnchors (BPT roots)

### DN → BVN Anchoring

The DN sends `DirectoryAnchor` transactions to each BVN containing:
- DN's RootChainAnchor and StateTreeAnchor
- Receipts for other BVN anchors that were included

## Full Receipt Path: Account → DN AppHash

To prove an account state on BVN0 is committed globally:

```
1. Account State Receipt (on BVN0)
   Account State Elements → Account State Hash → BVN0 BPT → BVN0 AppHash

2. BVN Anchor Receipt (on DN)
   BVN0 AppHash → BVN0 Anchor Chain on DN → DN Root Chain → DN BPT → DN AppHash
```

### Receipt Chain by Transaction Type

| Transaction Type | Partition | Receipt Path |
|-----------------|-----------|--------------|
| SendTokens | BVN | Sender balance → BVN BPT → (via anchor) → DN BPT |
| WriteData | BVN | Data chain entry → Root chain → BVN AppHash → DN anchor → DN AppHash |
| CreateIdentity | BVN | Identity state → BVN BPT → DN anchor → DN AppHash |
| AddCredits | BVN | Credit balance → BVN BPT → DN anchor → DN AppHash |
| UpdateKeyPage | BVN | KeyPage state → BVN BPT → DN anchor → DN AppHash |
| DirectoryAnchor | BVN | Received anchor → BVN BPT → (local proof only) |
| BlockValidatorAnchor | DN | Anchor chain → DN BPT → DN AppHash |
| Synthetic (any) | Target BVN | Via SequencedMessage with source receipt |

### Synthetic Transaction Receipts

Synthetic transactions include a `SourceReceipt` proving their origin:

```yaml
# pkg/api/v3/records.yml
MessageRecord:
  fields:
    - name: SourceReceipt
      type: merkle.Receipt  # Receipt from originating partition
```

## Chain Structure Per Account

Each account can have multiple chains:

| Chain | Purpose | Indexed |
|-------|---------|---------|
| `main` | Transaction hashes | Yes |
| `scratch` | Temporary data (not anchored globally) | No |
| `data` | Data entries (DataAccount only) | Yes |
| `signature` | Signature hashes | Yes |

The Root Chain on the system ledger (`acc://bvn-X.acme/ledger`) anchors all account chains:

```
Account Chain Entry → Account Chain Anchor → Root Chain Entry → Root Chain Anchor
                                                                      ↓
                                                              BlockValidatorAnchor
                                                                      ↓
                                                              DN Anchor Chain
```

## API Receipt Options

When querying via API, receipts can be requested:

```yaml
# pkg/api/v3/queries.yml
ReceiptOptions:
  fields:
    - name: ForAny        # Receipt for any matching entry
    - name: ForHeight     # Receipt for specific chain height
    - name: ForBlock      # Receipt anchored in specific block
```

The returned `Receipt` includes:

```yaml
# pkg/api/v3/types.yml
Receipt:
  fields:
    - type: merkle.Receipt    # The actual Merkle proof
    - name: LocalBlock        # Block number where anchored
    - name: LocalBlockTime    # Block timestamp
    - name: MajorBlock        # Major block number (if applicable)
```

## Validation

To validate a receipt:

```go
receipt.Validate(nil)  // Returns true if valid, false otherwise
```

The receipt proves:
1. `Start` hash is included in the Merkle tree
2. Following the `Entries` path produces `Anchor`
3. `Anchor` matches the expected root (AppHash)

## Creating Receipts (User API)

### State Receipt

Proves an account's complete state is included in the partition's BPT:

```go
batch := db.Begin(false)
defer batch.Discard()

account := batch.Account(accountUrl)
receipt, err := account.StateReceipt()
if err != nil {
    return err
}

// receipt.Start = account state hash
// receipt.Anchor = BPT root hash
// receipt.Entries = Merkle path
```

### BPT Receipt

Proves an account's hash is included in the BPT (subset of state receipt):

```go
receipt, err := account.BptReceipt()
// receipt.Start = account hash (as stored in BPT)
// receipt.Anchor = BPT root hash
```

### Chain Receipt

Proves a chain entry exists and builds proof between indexes:

```go
mainChain, err := account.MainChain().Get()
if err != nil {
    return err
}

// Build receipt from index 5 to index 10
receipt, err := mainChain.Receipt(5, 10)
// receipt.Start = hash at index 5
// receipt.Anchor = Merkle root at index 10
```

### Combining Receipts

Receipts can be combined when one's anchor matches another's start:

```go
// BVN receipt + DN receipt = global receipt
globalReceipt, err := bvnReceipt.Combine(dnReceipt)
// globalReceipt.Start = bvnReceipt.Start
// globalReceipt.Anchor = dnReceipt.Anchor
```

## Test References

Comprehensive tests demonstrating receipt creation are in `internal/database/receipt_test.go`.

### State Receipt Tests by Account Type

| Test | Account Type | Line |
|------|--------------|------|
| `TestStateReceipt_Identity` | ADI (Identity) | 29 |
| `TestStateReceipt_TokenAccount` | ADI Token Account | 54 |
| `TestStateReceipt_LiteTokenAccount` | Lite Token Account | 87 |
| `TestStateReceipt_LiteIdentity` | Lite Identity | 112 |
| `TestStateReceipt_DataAccount` | ADI Data Account | 137 |
| `TestStateReceipt_LiteDataAccount` | Lite Data Account | 163 |
| `TestStateReceipt_KeyBook` | Key Book | 190 |
| `TestStateReceipt_KeyPage` | Key Page | 213 |
| `TestStateReceipt_TokenIssuer` | Token Issuer | 237 |

### State Receipt Tests After Transactions

| Test | Transaction Type | Line |
|------|-----------------|------|
| `TestStateReceipt_AfterSendTokens` | SendTokens | 265 |
| `TestStateReceipt_AfterWriteData` | WriteData (ToState) | 326 |
| `TestStateReceipt_AfterAddCredits` | AddCredits | 376 |
| `TestStateReceipt_AfterBurnTokens` | BurnTokens | 420 |
| `TestStateReceipt_AfterUpdateKeyPage` | UpdateKeyPage | 464 |
| `TestStateReceipt_AfterCreateTokenAccount` | CreateTokenAccount | 507 |
| `TestStateReceipt_AfterCreateDataAccount` | CreateDataAccount | 539 |
| `TestStateReceipt_AfterIssueTokens` | IssueTokens | 571 |

### BPT and Multi-Account Tests

| Test | Description | Line |
|------|-------------|------|
| `TestBptReceipt` | BPT receipt generation and validation | 631 |
| `TestStateReceipt_MultipleAccounts` | Multiple accounts share same BPT root | 664 |

### Global Receipt Tests (BVN → DN)

| Test | Description | Line |
|------|-------------|------|
| `TestGlobalReceipt_SendTokens` | Transaction hash → DN root chain | 709 |
| `TestGlobalStateReceipt_AllAccounts` | Account state → DN root chain (all types) | 930 |

### Running the Tests

```bash
# Run all receipt tests
go test -v ./internal/database/... -run "Receipt"

# Run state receipt tests only
go test -v ./internal/database/... -run "TestStateReceipt"

# Run global receipt tests
go test -v ./internal/database/... -run "TestGlobal"

# Run with verbose output
go test -v ./internal/database/... -run "TestGlobalReceipt_SendTokens" 2>&1 | tee /tmp/test.log
```

## Key Files

| File | Purpose |
|------|---------|
| `internal/database/bpt_account.go` | StateReceipt, BptReceipt |
| `internal/database/observer_prod.go` | Account state hash computation |
| `internal/database/indexing/receipts.go` | Chain entry receipts |
| `internal/database/receipt_test.go` | Comprehensive receipt tests |
| `internal/core/execute/v2/chain/partition_anchor.go` | BVN → DN anchoring |
| `internal/core/execute/v2/chain/directory_anchor.go` | DN → BVN anchoring |
| `pkg/types/merkle/receipt.go` | Receipt type and validation |
