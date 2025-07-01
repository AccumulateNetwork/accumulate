# Snapshot Extraction Refactoring Design

## Overview

This document outlines the refactoring of the snapshot extraction process in the Accumulate Network codebase. The goal is to move from a global state approach to a more structured, object-oriented design while preserving existing functionality.

## File Structure

1. `a_extract_struct.go` - Contains the core data structures for extraction
2. `a_extract_cmd.go` - Contains the command-line interface and cobra integration
3. `a_extract.go` - Contains the implementation of extraction methods

## Core Data Structure

The central component of the refactoring is the `ExtractState` struct, which will encapsulate all state previously held in global variables or local function variables.

### ExtractState Struct

```go
type ExtractState struct {
    // Input parameters
    SnapshotFile string
    NetworkFile  string
    MaxAccounts  int
    
    // Routing information
    Router       routing.Router
    
    // Collection data structures
    Transactions         []TransactionRecord
    Messages            []MessageRecord
    TransactionHashToIndex map[[32]byte]int
    MessageHashToIndex  map[[32]byte]int
    
    // Counters
    AccountCount        int
    ChainCount          int
    TransactionCount    int
    MessageCount        int
    PartitionCounts     map[string]int
    
    // Merkle tree analysis
    TotalChainEntries    int64
    TotalSnapshotEntries int64
    ChainsWithMerkleData int
    
    // Merkle tree detailed analysis
    AccountsWithMainChain    int
    AccountsWithoutMainChain int
    TotalChainsExamined      int
    TotalExpectedEntries     int64
    TotalFoundEntries        int64
}
```

## Design Principles

1. **Direct Field Access** - We will NOT use accessors/mutators. The variables will be exported (all capitalized) for direct access.

2. **Minimal Code Changes** - The refactoring aims to preserve existing functionality while moving state into the struct.

3. **Method-Based Operations** - Core operations will be methods on the `ExtractState` struct.

4. **Consolidated Reporting** - The struct will include methods for generating reports based on the collected data.

## Core Methods

```go
func NewExtractState() *ExtractState
func (e *ExtractState) ProcessSnapshot() error
func (e *ExtractState) ExamineAccountMerkleTree(accountURL *url.URL, accountData []byte, accountIndex int64) error
func (e *ExtractState) AnalyzeChainMerkleTree(accountURL *url.URL, chainType string, chainData []byte, chainIndex int) error
func (e *ExtractState) PrintReport()
```

## Command Structure

The `a_extract_cmd.go` file will define a cobra command that creates an `ExtractState` instance and calls its methods:

```go
var extractCmd = &cobra.Command{
    Use:   "extract [snapshot-file] [network-file]",
    Short: "Extract and analyze data from a snapshot file",
    Run: func(cmd *cobra.Command, args []string) {
        state := NewExtractState()
        state.SnapshotFile = args[0]
        state.NetworkFile = args[1]
        
        // Parse flags
        state.MaxAccounts, _ = cmd.Flags().GetInt("max-accounts")
        
        // Run extraction
        if err := state.Run(); err != nil {
            fmt.Printf("Error: %v\n", err)
            os.Exit(1)
        }
    },
}
```

## Implementation Strategy

1. Create the `ExtractState` struct with all necessary fields
2. Convert existing functions to methods on the struct
3. Replace global variables and local function variables with struct fields
4. Update the command-line interface to use the new structure
5. Ensure all tests pass with the refactored code

## Data Collection Review

The extraction process collects the following types of data:

1. **Accounts** - Basic account information and routing
2. **Chains** - Chain sub-records associated with accounts
3. **Transactions** - Transaction records from the snapshot
4. **Messages** - Message records from the snapshot
5. **Merkle Trees** - Analysis of Merkle trees for chains

Each of these data types has associated counters and collections in the `ExtractState` struct.

## Reporting

The `PrintReport` method will generate a consolidated report including:

1. Summary counts (accounts, chains, transactions, messages)
2. Merkle tree analysis results
3. Account distribution by partition
4. Examples of collected records

## Next Steps

1. Implement the `ExtractState` struct in `a_extract_struct.go`
2. Convert the existing extraction logic to methods on the struct
3. Update the command-line interface in `a_extract_cmd.go`
4. Update tests to use the new structure
5. Consider further refactoring to improve modularity and testability
