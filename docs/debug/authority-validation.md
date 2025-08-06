# Validating the Directory Network Authority Set

This document explains how to validate the authority set of the Directory Network (DN) from genesis to present, focusing on three key validation aspects:

1. Validating authority signatures with timestamped changes
2. Validating authority signatures of major blocks
3. Validating minor blocks and their relationship to major blocks

> **New**: This functionality is now implemented in the Accumulate debug tools. See the [Using the Authority Validation Tools](#using-the-authority-validation-tools) section for details on how to use these tools.

## Table of Contents

1. [Introduction to Accumulate Network Block Structure](#introduction-to-accumulate-network-block-structure)
2. [Major Block Structure](#major-block-structure)
3. [Minor Blocks and Transaction Healing](#minor-blocks-and-transaction-healing)
4. [Authority Set Validation](#authority-set-validation)
   - [Validating Authority Signatures with Timestamped Changes](#1-validating-authority-signatures-with-timestamped-changes)
   - [Validating Authority Signatures of Major Blocks](#2-validating-authority-signatures-of-major-blocks)
   - [Validating Minor Blocks and Their Relationship to Major Blocks](#3-validating-minor-blocks-and-their-relationship-to-major-blocks)
5. [Putting It All Together: Full Validation Process](#putting-it-all-together-full-validation-process)

## Introduction to Accumulate Network Block Structure

The Accumulate Network consists of multiple partitions: Business Validation Networks (BVNs) and a Directory Network (DN). Each partition maintains its own blockchain, and these blockchains are synchronized through an anchoring system.

The block structure in Accumulate consists of:

- **Major Blocks**: High-level blocks that group multiple minor blocks together
- **Minor Blocks**: Contain actual transactions and chain entries
- **Anchors**: Cryptographic links between different parts of the network

The Directory Network (DN) serves as the authoritative record keeper for the entire network. Its authority set (validators) is critical for the security and integrity of the entire system.

## Major Block Structure

Major blocks are stored in a chain called "major-block" within the anchor pool account of each partition. The `MajorBlockRecord` structure includes:

```go
type MajorBlockRecord struct {
    Index         uint64                          // The block number/height
    Time          time.Time                       // When the block was created
    MinorBlocks   *RecordRange[*MinorBlockRecord] // Minor blocks contained in this major block
    LastBlockTime *time.Time                      // Timestamp of the last block
}
```

Major blocks are queried through the API using partition URLs like `acc://dn.acme` or `acc://bvn0.acme`:

```go
// Using the Directory Network URL
partitionUrl, err := url.Parse("acc://dn.acme")
query := &client.MajorBlocksQuery{
    Count: count,
    Start: startIndex,
}
query.Url = partitionUrl
resp, err := cl.QueryMajorBlocks(ctx, query)
```

## Minor Blocks and Transaction Healing

Minor blocks contain the actual transactions and chain entries. The `MinorBlockRecord` structure includes:

```go
type MinorBlockRecord struct {
    Index         uint64                                  // The block number
    Time          *time.Time                              // When the block was created
    Source        *url.URL                                // URL of the partition that produced this block
    Entries       *RecordRange[*ChainEntryRecord[Record]] // Chain entries in this block
    Anchored      *RecordRange[*MinorBlockRecord]         // Other minor blocks anchored by this block
    LastBlockTime *time.Time                              // Timestamp of the last block
}
```

Minor blocks are grouped into major blocks for efficiency and security. They contain the actual transaction data and chain entries that modify the state of accounts.

## Authority Set Validation

A critical aspect of building a secure light client is validating the authority set of the Directory Network (DN) from genesis to the present. This ensures that all blocks and transactions are properly authorized by legitimate validators.

### 1. Validating Authority Signatures with Timestamped Changes

The authority set in the Directory Network can change over time due to:
- Key rotations (validators changing their signing keys)
- New validators joining the network
- Validators leaving the network

To validate these changes:

1. Start with the genesis block's authority set, which is the initial set of validators
2. For each major block:
   - Check for authority set update transactions
   - Verify that these transactions are properly signed by the required threshold of the previous authority set (typically 2/3)
   - Update your local copy of the authority set accordingly

```go
func validateAuthorityChanges(ctx context.Context, cl *client.Client) error {
    // Start with genesis block (index 0)
    currentBlock := uint64(0)
    
    // Initialize with genesis authorities
    authorities, err := getGenesisAuthorities(ctx, cl)
    if err != nil {
        return fmt.Errorf("failed to get genesis authorities: %v", err)
    }
    
    // Track the latest validated block
    var latestBlock uint64
    
    // Query blocks in batches
    const batchSize = 100
    for {
        // Query a batch of major blocks
        query := &client.MajorBlocksQuery{
            Count: batchSize,
            Start: currentBlock,
        }
        
        partitionUrl, err := url.Parse("acc://dn.acme")
        if err != nil {
            return fmt.Errorf("failed to parse DN URL: %v", err)
        }
        
        query.Url = partitionUrl
        
        resp, err := cl.QueryMajorBlocks(ctx, query)
        if err != nil {
            return fmt.Errorf("failed to query major blocks: %v", err)
        }
        
        if resp == nil || len(resp.Items) == 0 {
            break
        }
        
        // Process each block for authority changes
        for _, item := range resp.Items {
            block := convertToMajorBlockRecord(item)
            
            // Check for authority change transactions
            authorityChanges, err := getAuthorityChangesFromBlock(ctx, cl, block)
            if err != nil {
                return fmt.Errorf("failed to get authority changes from block %d: %v", block.Index, err)
            }
            
            // Validate each change is properly signed by the current authority set
            for _, change := range authorityChanges {
                if !validateAuthorityChangeSignatures(change, authorities) {
                    return fmt.Errorf("invalid authority change signatures in block %d", block.Index)
                }
                
                // Apply the change to our tracked authority set
                authorities = applyAuthorityChange(authorities, change)
            }
            
            latestBlock = block.Index
        }
        
        // Move to the next batch
        currentBlock += uint64(len(resp.Items))
        
        // If we received fewer items than requested, we've reached the end
        if uint64(len(resp.Items)) < batchSize {
            break
        }
    }
    
    fmt.Printf("Successfully validated authority changes up to block %d\n", latestBlock)
    return nil
}
```

### 2. Validating Authority Signatures of Major Blocks

Each major block in the Directory Network is signed by the active validator set. To validate these signatures:

1. For each major block, extract the signatures
2. Verify that the signatures come from validators in the current authority set
3. Ensure that the signatures meet the required threshold (typically 2/3 of voting power)

```go
func validateMajorBlockSignatures(ctx context.Context, cl *client.Client, blockIndex uint64, authorities *AuthoritySet) (bool, error) {
    // Query the specific major block
    query := &client.MajorBlocksQuery{
        Count: 1,
        Start: blockIndex,
    }
    
    partitionUrl, err := url.Parse("acc://dn.acme")
    if err != nil {
        return false, fmt.Errorf("failed to parse DN URL: %v", err)
    }
    
    query.Url = partitionUrl
    
    resp, err := cl.QueryMajorBlocks(ctx, query)
    if err != nil {
        return false, fmt.Errorf("failed to query major block: %v", err)
    }
    
    if resp == nil || len(resp.Items) == 0 {
        return false, fmt.Errorf("major block not found")
    }
    
    // Extract the block and its signatures
    block := convertToMajorBlockRecord(resp.Items[0])
    signatures, err := getMajorBlockSignatures(ctx, cl, block)
    if err != nil {
        return false, fmt.Errorf("failed to get signatures for block %d: %v", blockIndex, err)
    }
    
    // Verify signatures against the authority set
    validSignatures := 0
    totalVotingPower := authorities.TotalVotingPower()
    signedVotingPower := uint64(0)
    
    for _, sig := range signatures {
        if authorities.Contains(sig.ValidatorID) && verifySignature(sig, block) {
            validSignatures++
            signedVotingPower += authorities.GetVotingPower(sig.ValidatorID)
        }
    }
    
    // Check if we have enough voting power (typically 2/3)
    requiredVotingPower := totalVotingPower * 2 / 3
    if signedVotingPower < requiredVotingPower {
        return false, fmt.Errorf("insufficient voting power: got %d, need %d", signedVotingPower, requiredVotingPower)
    }
    
    return true, nil
}
```

### 3. Validating Minor Blocks and Their Relationship to Major Blocks

Minor blocks contain the actual transactions and are grouped into major blocks. To validate the last 24 hours of minor blocks:

1. Determine the major blocks that cover the last 24 hours
2. For each major block, retrieve its minor blocks
3. Validate the signatures on each minor block
4. Verify that the minor blocks are correctly referenced in their parent major block

```go
func validateRecentMinorBlocks(ctx context.Context, cl *client.Client, authorities *AuthoritySet) error {
    // Calculate the timestamp for 24 hours ago
    oneDayAgo := time.Now().Add(-24 * time.Hour)
    
    // Find the major block closest to 24 hours ago
    startBlock, err := findMajorBlockByTime(ctx, cl, oneDayAgo)
    if err != nil {
        return fmt.Errorf("failed to find starting block: %v", err)
    }
    
    // Query from that block to the latest
    query := &client.MajorBlocksQuery{
        Start: startBlock,
        // Count is omitted to get all blocks from start to latest
    }
    
    partitionUrl, err := url.Parse("acc://dn.acme")
    if err != nil {
        return fmt.Errorf("failed to parse DN URL: %v", err)
    }
    
    query.Url = partitionUrl
    
    resp, err := cl.QueryMajorBlocks(ctx, query)
    if err != nil {
        return fmt.Errorf("failed to query major blocks: %v", err)
    }
    
    if resp == nil || len(resp.Items) == 0 {
        return fmt.Errorf("no major blocks found")
    }
    
    // Process each major block and its minor blocks
    for _, item := range resp.Items {
        majorBlock := convertToMajorBlockRecord(item)
        
        // Validate the major block's signatures
        valid, err := validateMajorBlockSignatures(ctx, cl, majorBlock.Index, authorities)
        if err != nil {
            return fmt.Errorf("failed to validate signatures for major block %d: %v", majorBlock.Index, err)
        }
        
        if !valid {
            return fmt.Errorf("invalid signatures for major block %d", majorBlock.Index)
        }
        
        // Get and validate all minor blocks in this major block
        minorBlocks, err := getMinorBlocksForMajorBlock(ctx, cl, majorBlock)
        if err != nil {
            return fmt.Errorf("failed to get minor blocks for major block %d: %v", majorBlock.Index, err)
        }
        
        for _, minorBlock := range minorBlocks {
            // Validate minor block signatures
            valid, err := validateMinorBlockSignatures(ctx, cl, minorBlock, authorities)
            if err != nil {
                return fmt.Errorf("failed to validate signatures for minor block %d: %v", minorBlock.Index, err)
            }
            
            if !valid {
                return fmt.Errorf("invalid signatures for minor block %d", minorBlock.Index)
            }
            
            // Verify the minor block is correctly referenced in the major block
            if !verifyMinorBlockInMajor(minorBlock, majorBlock) {
                return fmt.Errorf("minor block %d not correctly referenced in major block %d", minorBlock.Index, majorBlock.Index)
            }
        }
    }
    
    return nil
}
```

## Putting It All Together: Full Validation Process

To fully validate the Directory Network's authority set and its blocks:

1. Start with the genesis block and its initial authority set
2. Validate all authority changes chronologically
3. For each major block, validate its signatures against the current authority set
4. For recent blocks (last 24 hours), validate the minor blocks and their relationship to major blocks

This comprehensive validation ensures that:
- All authority changes are legitimate and properly signed
- All major blocks are signed by the correct authority set
- Recent minor blocks (which contain actual transactions) are valid and properly included in major blocks

By following this process, a light client can establish trust in the Directory Network's state without having to store or process the entire blockchain history.

## Codebase References

### Key Data Structures

1. **Major Block Chain**
   - Implementation: `internal/database/model_gen.go`
   - Key methods: `MajorBlockChain()` and `newMajorBlockChain()`
   - Chain name: `"major-block"` (stored in anchor pool account)

2. **Block Records**
   - `MajorBlockRecord` and `MinorBlockRecord`: `pkg/api/v3/types_gen.go`
   - Query structures: `MajorBlocksQuery` and `MinorBlocksQuery` in `internal/api/v2/types_gen.go`

3. **Chain Structure**
   - `Chain2` implementation: `internal/database/account_chains.go`
   ```go
   type Chain2 struct {
       account *Account
       key     *record.Key
       inner   *MerkleManager
       index   *Chain2
   }
   ```

### API Implementations

1. **Major Block Queries**
   - Implementation: `internal/api/v2/query_v3.go`
   - Methods: `QueryMajorBlocks()` and `QueryMajorBlock()`

2. **Minor Block Queries**
   - Implementation: `internal/api/v2/query_v3.go`
   - Method: `QueryMinorBlocks()`

3. **Block Processing**
   - Package: `internal/core/execute/v2/block`
   - Key files:
     - `block_major.go`: Major block processing
     - `block_end.go`: Block finalization
     - `msg_make_major_block.go`: Major block creation

### Authority Validation

1. **Signature Verification**
   - Implementation: `internal/core/execute/v2/block/sig_authority.go`
   - Authority signatures: `protocol.AuthoritySignature`

2. **Chain Updates**
   - Implementation: `internal/core/execute/v2/block/block_end.go`
   - Structure: `chainUpdate` for tracking modified chains

3. **Block State Management**
   - Implementation: `internal/core/execute/v2/block/state.go`
   - Method: `MergeTransaction()` for building blocks

## Example: Full Validation Implementation

Here's a complete example of how to implement the full validation process:

```go
func validateDirectoryNetworkFromGenesis(ctx context.Context, cl *client.Client) error {
    // Step 1: Get the genesis authority set
    authorities, err := getGenesisAuthorities(ctx, cl)
    if err != nil {
        return fmt.Errorf("failed to get genesis authorities: %v", err)
    }
    
    // Step 2: Validate all authority changes chronologically
    err = validateAuthorityChanges(ctx, cl)
    if err != nil {
        return fmt.Errorf("failed to validate authority changes: %v", err)
    }
    
    // Step 3: Get the current authority set after all changes
    currentAuthorities, err := getCurrentAuthorities(ctx, cl)
    if err != nil {
        return fmt.Errorf("failed to get current authorities: %v", err)
    }
    
    // Step 4: Validate recent major blocks
    latestMajorBlock, err := getLatestMajorBlock(ctx, cl)
    if err != nil {
        return fmt.Errorf("failed to get latest major block: %v", err)
    }
    
    // Validate the last 100 major blocks or fewer if there aren't that many
    startBlock := uint64(0)
    if latestMajorBlock.Index > 100 {
        startBlock = latestMajorBlock.Index - 100
    }
    
    for i := startBlock; i <= latestMajorBlock.Index; i++ {
        valid, err := validateMajorBlockSignatures(ctx, cl, i, currentAuthorities)
        if err != nil {
            return fmt.Errorf("failed to validate major block %d: %v", i, err)
        }
        
        if !valid {
            return fmt.Errorf("invalid signatures for major block %d", i)
        }
    }
    
    // Step 5: Validate recent minor blocks
    err = validateRecentMinorBlocks(ctx, cl, currentAuthorities)
    if err != nil {
        return fmt.Errorf("failed to validate recent minor blocks: %v", err)
    }
    
    fmt.Println("Successfully validated the Directory Network from genesis to present")
    return nil
}
```

This implementation provides a comprehensive validation of the Directory Network's authority set from genesis to the present, ensuring the integrity and security of the entire Accumulate Network.

## Using the Authority Validation Tools

The authority validation functionality is now implemented in the Accumulate debug tools. You can use the following commands to validate the Directory Network authority set:

### Installation

The authority validation tools are part of the Accumulate debug tools. To build and install them:

```bash
cd /path/to/accumulate
go build -o accumulate ./cmd/accumulate
```

### Available Commands

The authority validation tools are available under the `accumulate debug authority` command:

```bash
accumulate debug authority [command]
```

Available commands:

1. **genesis** - Get the genesis authority set
   ```bash
   accumulate debug authority genesis
   ```

2. **changes** - Track authority changes up to a specific major block
   ```bash
   accumulate debug authority changes [major-block-index]
   ```
   If no major block index is specified, it will track changes up to the latest major block.

3. **validate-block** - Validate a specific major block's signatures
   ```bash
   accumulate debug authority validate-block [major-block-index]
   ```

4. **validate-all** - Validate the entire authority set chain from genesis to current state
   ```bash
   accumulate debug authority validate-all
   ```

### Example Usage

Here's an example workflow for validating the Directory Network authority set:

1. First, retrieve the genesis authority set:
   ```bash
   accumulate debug authority genesis
   ```

2. Track all authority changes from genesis to the latest major block:
   ```bash
   accumulate debug authority changes
   ```

3. Validate a specific major block's signatures:
   ```bash
   accumulate debug authority validate-block 100
   ```

4. Validate the entire authority set chain from genesis to current state:
   ```bash
   accumulate debug authority validate-all
   ```

## Implementation Details

The authority validation tools are implemented in the following files:

1. **authority_validation.go** - Core validation logic
   - `AuthorityValidator` - Main struct for validating authority sets
   - `GetGenesisAuthoritySet` - Retrieves the initial authority set from genesis
   - `ValidateAuthorityChange` - Validates authority changes
   - `TrackAuthorityChanges` - Tracks all authority changes from genesis
   - `ValidateMajorBlockSignature` - Validates signatures on major blocks
   - `ValidateMinorBlockInclusion` - Validates minor block inclusion in major blocks
   - `ValidateAuthoritySetFromGenesis` - Validates the entire authority set chain

2. **authority_cmd.go** - Command-line interface
   - `authorityGenesisCmd` - Command to get the genesis authority set
   - `authorityChangesCmd` - Command to track authority changes
   - `authorityValidateBlockCmd` - Command to validate a major block's signatures
   - `authorityValidateAllCmd` - Command to validate the entire authority set chain
