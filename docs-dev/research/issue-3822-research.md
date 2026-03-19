# Research: Fix ineffassign lint warnings

## Summary
Found two ineffassign lint warnings in the test files. The first is in `height_test.go:152` where `min` is assigned but immediately shadowed by a second assignment without being used. The second is in `snapshot_test.go:440` where `allCerts` is created and populated but never used in the test.

## Verified Facts

### Fact 1: height_test.go:152 - `min` variable not used
- **Source**: `pkg/consensus/adapter/height_test.go:152-163`
- **Content**:
  ```go
  // Empty tracker
  min, max, ok := tracker.HeightRange()
  assert.False(t, ok)

  // Add some blocks
  tracker.RecordBlock(types.Round(2))
  tracker.RecordBlock(types.Round(4))
  tracker.RecordBlock(types.Round(6))

  min, max, ok = tracker.HeightRange()
  assert.True(t, ok)
  assert.Equal(t, uint64(1), min)
  assert.Equal(t, uint64(3), max)
  ```
- **Confidence**: HIGH
- **Issue**: Line 152 assigns `min` and `max` but only `ok` is used (to assert it's false). The `min` and `max` values from the empty tracker case are never checked - they're immediately overwritten at line 160 after adding blocks.

### Fact 2: snapshot_test.go:440 - `allCerts` variable not used
- **Source**: `pkg/consensus/snapshot/snapshot_test.go:439-440`
- **Content**:
  ```go
  // Build up certificates for multiple rounds (simulating 100 blocks)
  // We use simplified certificates without proper parent references for testing
  allCerts := make([]*types.Certificate, 0)
  allCerts = append(allCerts, genesisCerts...)
  ```
- **Confidence**: HIGH
- **Issue**: The `allCerts` slice is created and `genesisCerts` is appended to it, but the slice is never used afterward. The comment suggests it was intended to accumulate certificates for a simulation but the implementation is incomplete or the variable became unnecessary.

## Code References

### Primary Implementation Files
- `pkg/consensus/adapter/height_test.go:148-164` - `TestHeightTracker_HeightRange` function
- `pkg/consensus/snapshot/snapshot_test.go:403-514` - `TestSnapshot_CreateAndRestore` function

### Related Code
- `pkg/consensus/adapter/height.go` - HeightTracker implementation (exports `HeightRange()` method)
- `pkg/consensus/snapshot/snapshot.go` - Snapshot implementation

## Recommended Fixes

### Fix 1: height_test.go:152
Replace line 152 with blank identifiers for `min` and `max` since only `ok` is checked:
```go
_, _, ok := tracker.HeightRange()
```

### Fix 2: snapshot_test.go:439-440
Remove the unused `allCerts` variable entirely (lines 437-440):
```go
// Build up certificates for multiple rounds (simulating 100 blocks)
// We use simplified certificates without proper parent references for testing
allCerts := make([]*types.Certificate, 0)
allCerts = append(allCerts, genesisCerts...)
```
These lines can be deleted as `allCerts` is never used.

## Open Questions
None - the fixes are straightforward.

## Contradictions
None found.
