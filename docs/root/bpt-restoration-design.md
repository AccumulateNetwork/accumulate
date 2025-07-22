# Simplified BPT Restoration Design

## Core Problem
Snapshots without BPT sections cause restoration to fail, preventing mainnet node launch.

## Key Insight: "Sometimes Less is More"
The current complex validation approach has a fundamental flaw: it can't prove the rebuilt BPT doesn't contain extra entries not present in the original snapshot's BPT section. **Root hash comparison is the only reliable validation.**

## Simplified Strategy

1. **Always ignore missing BPT sections** - Log warning, don't fail
2. **Always rebuild BPT from all accounts** - Ensures consistency  
3. **Only validate root hash** - Simple, reliable, complete validation
4. **Skip individual account hash verification** - It's incomplete and unreliable

## Implementation

### 1. Modify `readBptSnapshot` to Never Fail
```go
func readBptSnapshot(snap *snapshot.Reader, opts *RestoreOptions) ([32]byte, error) {
    // Try to read the expected root hash from snapshot header
    // Don't try to read individual BPT entries - we'll rebuild from accounts
    return snap.Header.RootHash, nil
}
```

### 2. Simplify Restoration Logic
```go
func Restore(db database.Beginner, file ioutil2.SectionReader, opts *RestoreOptions) error {
    rd, err := snapshot.Open(file)
    if err != nil {
        return errors.UnknownError.WithFormat("open snapshot: %w", err)
    }

    expectedRootHash := rd.Header.RootHash
    
    // ... restore all accounts (existing logic) ...
    
    // Always rebuild BPT from accounts
    err = batch.UpdateBPT()
    if err != nil {
        return errors.UnknownError.WithFormat("rebuild BPT: %w", err)
    }
    
    err = batch.Commit()
    if err != nil {
        return errors.UnknownError.WithFormat("commit changes: %w", err)
    }
    
    // Simple root hash validation
    return validateRootHash(db, expectedRootHash)
}
```

### 3. Simple Root Hash Validation
```go
func validateRootHash(db database.Beginner, expectedRootHash [32]byte) error {
    batch := db.Begin(false)
    defer batch.Discard()
    
    actualRootHash, err := batch.GetBptRootHash()
    if err != nil {
        return errors.UnknownError.WithFormat("get rebuilt BPT root hash: %w", err)
    }
    
    zeroHash := [32]byte{}
    
    // Case 1: Expected root hash is zero (genesis or partition snapshot)
    if expectedRootHash == zeroHash {
        // Log the actual root hash for reference
        fmt.Printf("INFO: Snapshot had zero root hash, rebuilt BPT root hash: %x\n", actualRootHash)
        return nil
    }
    
    // Case 2: Expected root hash is non-zero, must match exactly
    if expectedRootHash != actualRootHash {
        return errors.InvalidRecord.WithFormat(
            "BPT root hash mismatch: expected %x, rebuilt %x", 
            expectedRootHash, actualRootHash)
    }
    
    fmt.Printf("INFO: BPT root hash validation successful: %x\n", actualRootHash)
    return nil
}
```

## Expected Behavior

- **Normal snapshots**: Root hash validation passes, restoration succeeds
- **Partition snapshots**: Zero root hash logged as info, restoration succeeds  
- **Corrupted snapshots**: Root hash mismatch, restoration fails with clear error
- **Missing BPT sections**: No impact, BPT rebuilt from accounts, validation by root hash

This approach is **simpler, more reliable, and actually validates what matters** - the complete integrity of the rebuilt BPT through its root hash.
