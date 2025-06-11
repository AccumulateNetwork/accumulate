# Accumulate Snapshot Documentation

## Overview of BPT Storage and Snapshot Collection

This document explains what data is stored in the Binary Patricia Tree (BPT) and how it's written to snapshot files in the Accumulate Network.

## What's Stored in the BPT

The BPT (Binary Patricia Tree) in Accumulate stores entries for all account types in the system. Each entry consists of a key (typically an account URL) and a value (a hash representing the account's state).

### Account Types Stored in BPT

All account types that implement the `protocol.Account` interface are stored in the BPT, including:

- ADI (Accumulate Digital Identities)
- AnchorLedger
- BlockLedger
- DataAccount
- KeyBook
- KeyPage
- LiteDataAccount
- LiteIdentity
- LiteTokenAccount
- SyntheticLedger
- SystemLedger
- TokenAccount
- TokenIssuer
- UnknownAccount
- UnknownSigner

### Account Data Stored in BPT

For each account, the BPT stores a hash that includes:

1. **Main State**: A hash of the account's main state (via `a.Main().Get()`)
2. **Secondary State**: 
   - Directory list (accounts contained by the ADI)
   - Scheduled events (for partition accounts)
3. **Chains**: A hash of all the account's chains, including:
   - Chain metadata
   - Current state anchors
4. **Pending Transactions**: A hash of all pending transactions, including:
   - Transaction hashes
   - Transaction status

### Key Structure

Each account is stored in the BPT with a key that is either:
- A full account URL (if the key can be resolved)
- A key hash (if the key cannot be resolved to an account URL)

## Snapshot BPT Section Format

When a snapshot is collected, the `collectBPT` function writes BPT entries to a dedicated section in the snapshot file:

1. Opens a new section in the snapshot file of type `SectionTypeBPT`
2. Iterates through all entries in the BPT
3. For each entry:
   - Attempts to resolve the key to a full account URL using `resolveAccountKey`
   - Creates a `RecordEntry` struct containing:
     - The resolved key (preferably a full account URL)
     - The BPT entry value hash (32 bytes)
   - Writes this `RecordEntry` to the snapshot using `WriteValue`
4. Logs progress every 100,000 entries with human-readable formatting

The `RecordEntry` struct is a simple container that holds:
```go
type RecordEntry struct {
    Key   *record.Key
    Value []byte
}
```

Where:
- `Key` is the resolved account key (preferably a full account URL)
- `Value` is the hash of the account state (32 bytes)

## Key Resolution Process

The key resolution process is particularly important for snapshot readability:

1. The `resolveAccountKey` function attempts to convert `KeyHash` keys to full account URLs
2. If resolution fails, it logs an error and keeps the original key
3. This resolution ensures that most keys in the snapshot BPT section are human-readable account URLs rather than opaque hashes

## Importance for Healing

The BPT snapshot section is critical for the healing process because:

1. It provides a complete inventory of all accounts in the system
2. The resolved keys make it easier to identify which accounts need healing
3. The value hashes allow for quick comparison to determine if an account's state has changed

When healing from a snapshot, the system can efficiently determine which accounts need to be synchronized by comparing the BPT entries in the snapshot to the current state of the network.
