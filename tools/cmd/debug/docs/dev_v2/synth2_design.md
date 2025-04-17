# Heal Synth2 Design Document

## Overview

This document outlines the design for `heal synth2`, a step-wise implementation of synthetic transaction healing that builds on the existing `heal synth` functionality. The goal is to create a plug-in replacement that breaks down the healing process into discrete phases for better debugging, testing, and understanding.

## Key Principles

1. **Use Existing Functionality**: We will only use existing v1 functionality and will not implement enhancements not found in v1.
2. **No Communication Layer Changes**: We will not modify the communication layer.
3. **No New Interfaces or Structs**: We will use the existing interfaces and structs.
4. **Step-by-Step Approach**: We will implement the healing process in discrete phases.
5. **Comprehensive Logging**: Each phase will include detailed logging to aid in debugging.
6. **Always Initialize Light Client**: We will always initialize the light client with a MemDB to prevent nil pointer dereferences.

## Light Client Initialization

The light client is a critical component for the healing process, particularly for retrieving synthetic ledger information. We've identified that the original implementation made the light client initialization optional, which could lead to nil pointer dereferences when attempting to use `h.light.OpenDB()`.

**Implementation**:
- Always initialize the light client, even if no persistent database is specified
- Use an in-memory database (MemDB) as a fallback when no persistent database is specified
- This approach ensures that `h.light` is never nil, preventing runtime errors

**Key Functions**:
- Use `light.NewClient()` with appropriate options for initialization
- Use `memdb.New()` for creating an in-memory database when needed
- Modify the `setup()` function to ensure the light client is always initialized

## Phased Implementation

The `heal synth2` command will implement a phased approach to healing:

### Phase 1: Partition Discovery

**Objective**: Identify all partitions in the network and categorize them by type.

**Implementation**:
- Use the existing network status to identify partitions
- Categorize partitions by type (DN vs BVN)
- Log detailed information about each partition
- Store the results for later phases

**Key Functions**:
- Use `h.net.Status.Network.Partitions` to access partitions
- No new API calls or interfaces required

### Phase 2: URL Construction

**Objective**: Build source and destination URLs for each partition pair.

**Implementation**:
- Create partition pairs based on the partitions discovered in Phase 1
- Build source and destination URLs using existing URL construction methods
- Validate the URLs to ensure they are properly formed
- Store the results for later phases

**Key Functions**:
- Use `protocol.PartitionUrl(partitionID)` for URL construction
- Use `JoinPath()` for appending paths to URLs
- No new API calls or interfaces required

### Phase 3: Chain Height Analysis

**Objective**: Query and compare chain heights for each partition pair.

**Implementation**:
- Query chain information for each partition pair
- Compare chain heights to identify what needs healing
- Log detailed information about chain heights
- Store the results for later phases

**Key Functions**:
- Use `h.tryEach().QueryAccountChains()` for querying chain information
- No new API calls or interfaces required

### Phase 4: Chain Loading

**Objective**: Load chains using existing v1 methods.

**Implementation**:
- Use the existing `pullDifferentialChains` function from v1
- Load chains for each partition pair
- Log detailed information about chain loading
- Store the results for later phases

**Key Functions**:
- Use `pullDifferentialChains()` for loading chains
- Use `h.light.IndexAccountChains()` for indexing chains
- No new API calls or interfaces required

### Phase 5: Healing Transaction Creation

**Objective**: Create and submit healing transactions.

**Implementation**:
- Use the existing `healSingleSynth` function from v1
- Create and submit healing transactions for each partition pair
- Log detailed information about transaction creation and submission
- Implement proper error handling and retry logic

**Key Functions**:
- Use `healSingleSynth()` for creating and submitting transactions
- Use `pullSynthLedger()` for retrieving synthetic ledger information
- No new API calls or interfaces required

## Logging Strategy

Each phase will include comprehensive structured logging:

```go
// Example logging
slog.Info("Phase 1: Partition Discovery")
slog.Info("Found directory partition", 
    "id", part.ID, 
    "type", part.Type)
slog.Info("Partition discovery complete", 
    "total", len(partitions),
    "directory", len(dnPartitions),
    "bvn", len(bvnPartitions),
    "duration", time.Since(startTime))
```

This structured logging will aid in debugging and understanding the healing process.

## Error Handling

We will implement proper error handling throughout the codebase:

```go
// Example error handling
if err != nil {
    slog.Error("Failed to query chain information", 
        "url", url, 
        "chain", chainName, 
        "error", err)
    return nil, errors.UnknownError.WithFormat("query chain info: %w", err)
}
```

This error handling will help identify and fix issues in the healing process.

## Implementation Plan

1. Create the basic command structure with the phased approach
2. Implement Phase 1: Partition Discovery
3. Implement Phase 2: URL Construction
4. Implement Phase 3: Chain Height Analysis
5. Implement Phase 4: Chain Loading
6. Implement Phase 5: Healing Transaction Creation
7. Test each phase individually and as a whole
8. Refine and optimize based on testing results

## Conclusion

The `heal synth2` command will provide a step-wise implementation of synthetic transaction healing that builds on the existing `heal synth` functionality. By breaking down the healing process into discrete phases, we can better debug, test, and understand the process.
