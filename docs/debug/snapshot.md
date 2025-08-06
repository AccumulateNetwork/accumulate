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

## Snapshot File Format

### File Structure Overview

A snapshot file consists of multiple sections, each containing specific types of data:

1. **Header Section**: Contains metadata about the snapshot
2. **BPT Section**: Contains all Binary Patricia Tree entries
3. **Records Section**: Contains account and message records
4. **Index Section** (optional): Contains lookup indices for fast access

### File Header Format

The snapshot begins with a header section containing:

- **Magic Number**: Identifies the file as an Accumulate snapshot
- **Version**: The snapshot format version number
- **Root Hash**: The BPT root hash at the time of snapshot
- **System Ledger**: The partition's system ledger state (if applicable)
- **Creation Time**: When the snapshot was created

### Section Format

Each section in the snapshot file follows this structure:

1. **Section Type**: Identifier for the section type (BPT, Records, Index)
2. **Section Length**: Size of the section in bytes
3. **Section Data**: The actual section content
4. **Section Checksum**: Verification hash for the section

### BPT Section Format

The BPT section contains all entries from the Binary Patricia Tree:

1. Each entry is stored as a `RecordEntry` struct:
   ```go
   type RecordEntry struct {
       Key   *record.Key
       Value []byte
   }
   ```

2. Keys are stored in one of two formats:
   - Human-readable account URLs (preferred)
   - Key hashes (when URL resolution fails)

3. Values are stored as 32-byte hashes representing the account state

### Records Section Format

The Records section contains the actual account and message data:

1. **Account Records**: For each account:
   - Main state (account properties)
   - Directory entries (for ADIs)
   - Chain records (for all account chains)
   - Pending transactions

2. **Message Records**: For each message:
   - Message body
   - Message status
   - Related transaction status

### Account Records vs Message Records

Account records and message records serve different purposes in the network's data model:

#### Account Records

Account records represent the state of accounts in the Accumulate network:

1. **Content**: 
   - Store the current state of an account (balances, properties, settings)
   - Include directory entries for ADIs (Accumulate Digital Identities)
   - Contain chain data (transaction history, anchors)
   - Track pending transactions

2. **Structure**:
   - Keyed by account URLs (e.g., `acc://example.acme/account`)
   - Organized hierarchically in the BPT (Binary Patricia Tree)
   - Include main state and secondary state information

3. **Types**:
   - ADIs (Accumulate Digital Identities)
   - TokenAccounts (for holding tokens)
   - KeyBooks and KeyPages (for identity management)
   - DataAccounts (for storing arbitrary data)
   - Various ledger accounts (System, Block, Anchor, Synthetic)

4. **Purpose**:
   - Define the current state of the network
   - Represent ownership and balances
   - Track identity and authorization information

#### Message Records

Message records represent the transactions and signatures that flow through the network:

1. **Content**:
   - Transaction data (operations to be performed)
   - Signature data (authorizations for transactions)
   - Transaction status information
   - Message metadata

2. **Structure**:
   - Keyed by message hash (32-byte hash value)
   - Stored separately from account data
   - Include both the message itself and its status

3. **Types**:
   - TransactionMessages (operations like token transfers, account creation)
   - SignatureMessages (authorizations for transactions)
   - Various transaction subtypes (SendTokens, CreateDataAccount, etc.)

4. **Purpose**:
   - Represent the operations that modify account states
   - Track the history of changes to the network
   - Provide authorization proofs

#### Key Differences

1. **Persistence**:
   - Account records represent the current state and persist indefinitely
   - Message records represent historical operations and may be pruned over time

2. **Relationship**:
   - Messages modify accounts (cause state transitions)
   - Accounts maintain references to relevant messages (in chains)

3. **Storage**:
   - In the snapshot file, account records are collected from account state
   - Message records are collected by following references from accounts' chains

4. **Filtering**:
   - The snapshot collection allows selective filtering of messages (e.g., skipping signatures or token transactions)
   - Account filtering is typically based on account type (e.g., skipping system accounts)

### Index Section Format (Optional)

When the `--indexed` flag is used, an index section is created:

1. **Record Index**: Maps record keys to their position in the file
2. **Hash Index**: Maps hash values to their corresponding records

The index enables fast random access to records without scanning the entire file.

## Snapshot URLs File

During snapshot collection, a separate `.urls` file is created containing:

1. A header with column titles
2. One line per account with:
   - Account type (aligned in a 25-character column)
   - Full account URL

This file provides a human-readable inventory of all accounts in the snapshot.

## Snapshot Collection Process

### Collection Command

Snapshots are collected using the `debug snap collect` command:

```
debug snap collect [database] [snapshot]
```

Options:
- `--skip-bpt`: Skip the BPT section
- `--skip-system`: Skip system accounts
- `--skip-signatures`: Skip signature messages
- `--skip-token-txns`: Skip token transactions
- `--indexed`: Create an indexed snapshot
- `--partition`: Specify the partition (e.g., "bvn-Apollo.acme")

### Collection Workflow

1. **Database Connection**: Opens the specified database
2. **Partition Detection**: Determines the partition to snapshot
3. **File Creation**: Creates the output snapshot file
4. **Header Writing**: Writes the snapshot header with BPT root hash
5. **BPT Collection**: Collects all BPT entries
6. **Account Collection**: Collects all account records
7. **Message Collection**: Collects all message records
8. **Index Building** (optional): Builds a fast-lookup index

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

## Implementation Notes

### Memory Management

To prevent excessive memory usage during snapshot collection:

1. Records are collected in batches (default batch size: 1000)
2. Temporary files are used for index building
3. Root database batches are preferred for large snapshots
4. Progress is reported periodically to monitor memory usage

### Performance Considerations

- BPT collection reports progress every 1,000 entries
- Account type distribution is tracked and reported
- Estimated completion time is calculated based on progress
- A separate URLs file is generated for human readability

## Snapshot File Versioning

The snapshot format includes version information to ensure compatibility:

1. **Version 1**: Initial snapshot format
2. **Version 2**: Added support for indexed snapshots
3. **Version 3**: Enhanced BPT section with resolved keys

Future versions may include additional sections or optimizations while maintaining backward compatibility.

## Snapshot Validation Strategy

Validating that a snapshot accurately reflects the accounts and their transactions is critical, especially when transaction-to-block indexing is pruned. The following strategy provides a framework for snapshot validation.

### Account State Validation

1. **BPT Hash Comparison**:
   - Compare the BPT root hash in the snapshot header with the network's current BPT root hash
   - Any difference indicates state divergence that requires further investigation

2. **URL Inventory Validation**:
   - Use the collected URLs file (`snapshot.urls`) as a complete inventory of accounts
   - Compare URL inventories between the snapshot and the live network
   - Identify missing or extra accounts in either source
   - Group validation by account type for more targeted verification

3. **Account Sampling**:
   - If full validation is too resource-intensive, use the URL inventory to perform stratified sampling
   - Ensure the sample covers different account types (ADIs, TokenAccounts, etc.)
   - For large networks, use statistical sampling methods to achieve a confidence level of at least 95%

3. **Key Field Verification**:
   - For each sampled account, verify:
     - Account balances and properties
     - Chain anchors and heights
     - Pending transaction lists
     - Directory entries (for ADIs)

4. **Directory Completeness**:
   - For each ADI, verify that all directory entries in the network are present in the snapshot
   - Check that no extra accounts appear in the snapshot

### Transaction Validation

1. **Chain Integrity**:
   - Verify that transaction chains are complete from genesis to the latest entry
   - Check that chain anchors match between snapshot and network
   - Validate chain heights and entry counts

2. **Transaction Sampling**:
   - Sample transactions from various chains and account types
   - Verify transaction contents, including body, signatures, and status
   - Focus on transactions near chain boundaries to detect truncation issues

3. **Signature Verification**:
   - Ensure all required signatures for transactions are present
   - Verify signature validity and authority

4. **Status Consistency**:
   - Confirm transaction statuses match between snapshot and network
   - Pay special attention to pending transactions

### Handling Pruned Transaction-to-Block Indexing

With transaction-to-block indexing pruned and delegated to a data server, the validation strategy must adapt:

1. **Metadata Preservation**:
   - Ensure essential transaction metadata is preserved in the snapshot
   - Include transaction hashes, chain positions, and execution status

2. **Chain-Based Ordering**:
   - Use chain positions to establish relative transaction ordering
   - Verify that the relative ordering of transactions is maintained

3. **External Reference System**:
   - Implement a reference system to query the data server for timestamp information when needed
   - Document the relationship between snapshot data and external timestamp services

4. **Consistency Checks**:
   - Perform cross-chain consistency checks to validate transaction ordering
   - Use synthetic transactions and anchors to verify cross-partition consistency

### Validation Tooling

1. **Automated Validator**:
   - Develop a `debug snap validate` command that performs automated validation
   - Include options for validation depth and sampling rate

2. **Reporting**:
   - Generate detailed reports of validation results
   - Include statistics on accounts and transactions checked
   - Highlight any discrepancies found

3. **Continuous Validation**:
   - Implement periodic validation as part of network maintenance
   - Compare snapshots taken at different times to track state evolution

### Implementation Considerations

1. **Performance Optimization**:
   - Use parallel processing for validation when possible
   - Implement progressive validation to provide early feedback

2. **Resource Management**:
   - Control memory usage during validation
   - Use streaming approaches for large snapshots

3. **Error Handling**:
   - Classify validation errors by severity
   - Provide remediation suggestions for common issues

By implementing this validation strategy, operators can ensure that snapshots accurately reflect the network state, even with pruned transaction-to-block indexing.

## Leveraging the URLs File for Validation

The `snapshot.urls` file generated during snapshot collection provides significant advantages for validation:

### URL File Structure

The URLs file contains:

```
# ACCOUNT TYPE            URL
---------------------------------------------------------------------------
ADI                      acc://example.acme
TokenAccount             acc://example.acme/tokens
KeyBook                  acc://example.acme/book
...
Unresolved               [hash representation for unresolved keys]
```

### Validation Applications

1. **Complete Account Inventory**:
   - The URLs file provides a complete inventory of all accounts in the BPT
   - This enables exhaustive validation rather than sampling
   - Accounts can be grouped by type for targeted validation strategies

2. **Account Type Distribution Analysis**:
   - The file includes account type information
   - This allows validation of account type distributions between snapshots
   - Anomalies in type distribution can indicate issues

3. **Unresolved Key Analysis**:
   - Unresolved keys in the URLs file represent potential data integrity issues
   - These should be investigated during validation
   - A high number of unresolved keys may indicate database corruption

4. **Efficient Diff Operations**:
   - Simple text-based diff tools can quickly compare URL files from different snapshots
   - This provides a fast way to identify added or removed accounts

5. **Hierarchical Validation**:
   - URLs reveal the hierarchical structure of accounts
   - This enables validation of parent-child relationships between accounts

### Implementation in the Validation Command

A `debug snap validate` command could leverage the URLs file as follows:

```go
// Pseudocode for URL-based validation
func validateWithURLs(snapshot, network string) error {
    // Load URLs from snapshot
    snapshotURLs := loadURLsFile(snapshot + ".urls")
    
    // Generate URLs from network (or load from another snapshot)
    networkURLs := generateURLsFromNetwork(network)
    
    // Compare URL inventories
    missing, extra := compareURLs(snapshotURLs, networkURLs)
    
    // Report differences
    reportMissingAccounts(missing)
    reportExtraAccounts(extra)
    
    // Validate account contents for matching URLs
    validateMatchingAccounts(snapshotURLs, networkURLs)
    
    return nil
}
```

This approach provides a solid foundation for snapshot validation while respecting the planned changes to transaction-to-block indexing.
