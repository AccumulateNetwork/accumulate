# Research: Fix rangevarref lint warnings

## Summary

The issue identifies 10 locations across 6 files where range variables are used with `[:]` slice conversion in for-range loops. These are NOT actually unsafe pointer captures but are flagged by the linter because `[:]` creates a slice header pointing to the range variable. Since Go 1.22, the range variable is re-created each iteration, making these patterns safe. However, for linter compliance and backwards compatibility with pre-1.22 code, the fix is to create a local copy of the range variable before using `[:]`.

## Verified Facts

### Fact 1: Digest types are 32-byte arrays
- **Source**: `pkg/consensus/types/batch.go:20`, `pkg/consensus/types/certificate.go:19`, `pkg/consensus/types/header.go:20`
- **Content**:
  - `type BatchDigest [32]byte`
  - `type CertificateDigest [32]byte`
  - `type HeaderDigest [32]byte`
- **Confidence**: HIGH

### Fact 2: authorKey is a 32-byte array
- **Source**: `pkg/consensus/bullshark/bullshark.go:24`
- **Content**: `type authorKey [32]byte`
- **Confidence**: HIGH

### Fact 3: gossip/cert_sync.go line 69 - digest[:] in Marshal
- **Source**: `pkg/consensus/gossip/cert_sync.go:68-70`
- **Content**:
  ```go
  for _, digest := range r.Digests {
      copy(data[offset:], digest[:])
      offset += 32
  }
  ```
- **Analysis**: `Digests` is `[]types.CertificateDigest`. Range variable `digest` is a value copy. Using `digest[:]` creates a slice pointing to this loop-local copy. Safe because `copy` reads immediately, but linter flags it.
- **Confidence**: HIGH

### Fact 4: gossip/cert_sync.go line 179 - digest[:] in Marshal
- **Source**: `pkg/consensus/gossip/cert_sync.go:178-180`
- **Content**:
  ```go
  for _, digest := range r.Missing {
      copy(data[offset:], digest[:])
      offset += 32
  }
  ```
- **Analysis**: Same pattern as Fact 3. `Missing` is `[]types.CertificateDigest`.
- **Confidence**: HIGH

### Fact 5: cmd/consensus-testnet/block.go line 119 - h[:] in ComputeTxnsHash
- **Source**: `cmd/consensus-testnet/block.go:118-120`
- **Content**:
  ```go
  for _, h := range txHashes {
      hasher.Write(h[:])
  }
  ```
- **Analysis**: `txHashes` is `[][32]byte`. Range variable `h` is a value copy. Using `h[:]` for `hasher.Write` reads immediately but is flagged.
- **Confidence**: HIGH

### Fact 6: cmd/consensus-testnet/block.go line 131 - h[:] in UpdateStateHash
- **Source**: `cmd/consensus-testnet/block.go:130-131`
- **Content**:
  ```go
  for _, h := range txHashes {
      hasher.Write(h[:])
  }
  ```
- **Analysis**: Same pattern as Fact 5.
- **Confidence**: HIGH

### Fact 7: cmd/consensus-testnet/integration_test.go line 535 - h[:] in state hash map
- **Source**: `cmd/consensus-testnet/integration_test.go:534-535`
- **Content**:
  ```go
  for _, h := range stateHashes {
      hashCounts[hex.EncodeToString(h[:])]++
  }
  ```
- **Analysis**: `stateHashes` is `[][32]byte`. Range variable `h` used with `[:]` passed to `hex.EncodeToString` which reads immediately.
- **Confidence**: HIGH

### Fact 8: cmd/consensus-testnet/stress_test.go line 501 - h[:] in state hash map
- **Source**: `cmd/consensus-testnet/stress_test.go:500-501`
- **Content**:
  ```go
  for _, h := range stateHashes {
      hashCounts[hex.EncodeToString(h[:])]++
  }
  ```
- **Analysis**: Same pattern as Fact 7.
- **Confidence**: HIGH

### Fact 9: pkg/consensus/bullshark/bullshark.go line 172 - k[:] in GetLastCommitted
- **Source**: `pkg/consensus/bullshark/bullshark.go:171-172`
- **Content**:
  ```go
  for k, v := range b.lastCommitted {
      result[hex.EncodeToString(k[:])] = v
  }
  ```
- **Analysis**: `lastCommitted` is `map[authorKey]types.Round`. Range variable `k` is a value copy. Using `k[:]` passed to `hex.EncodeToString` reads immediately.
- **Confidence**: HIGH

### Fact 10: pkg/consensus/types/header.go lines 147, 164, 240, 257 - payload/parent iteration
- **Source**: `pkg/consensus/types/header.go:146-148, 163-165, 239-241, 256-258`
- **Content**:
  ```go
  // Line 147 (in marshalForDigest)
  for _, k := range payloadKeys {
      copy(data[offset:], k[:])
      ...
  }

  // Line 164 (in marshalForDigest)
  for _, p := range sortedParents {
      copy(data[offset:], p[:])
      ...
  }

  // Line 240 (in Marshal)
  for _, k := range payloadKeys {
      copy(data[offset:], k[:])
      ...
  }

  // Line 257 (in Marshal)
  for _, p := range sortedParents {
      copy(data[offset:], p[:])
      ...
  }
  ```
- **Analysis**: `payloadKeys` is `[]BatchDigest`, `sortedParents` is `[]CertificateDigest`. Both are 32-byte array types. Range variables used with `[:]` for immediate copy operations.
- **Confidence**: HIGH

## Code References

| File | Lines | Function | Variable | Type |
|------|-------|----------|----------|------|
| `pkg/consensus/gossip/cert_sync.go` | 69 | `CertSyncRequest.Marshal` | `digest` | `CertificateDigest` |
| `pkg/consensus/gossip/cert_sync.go` | 179 | `CertSyncResponse.Marshal` | `digest` | `CertificateDigest` |
| `cmd/consensus-testnet/block.go` | 119 | `ComputeTxnsHash` | `h` | `[32]byte` |
| `cmd/consensus-testnet/block.go` | 131 | `UpdateStateHash` | `h` | `[32]byte` |
| `cmd/consensus-testnet/integration_test.go` | 535 | `TestIntegration*` | `h` | `[32]byte` |
| `cmd/consensus-testnet/stress_test.go` | 501 | `TestStress*` | `h` | `[32]byte` |
| `pkg/consensus/bullshark/bullshark.go` | 172 | `GetLastCommitted` | `k` | `authorKey` |
| `pkg/consensus/types/header.go` | 147 | `marshalForDigest` | `k` | `BatchDigest` |
| `pkg/consensus/types/header.go` | 164 | `marshalForDigest` | `p` | `CertificateDigest` |
| `pkg/consensus/types/header.go` | 240 | `Marshal` | `k` | `BatchDigest` |
| `pkg/consensus/types/header.go` | 257 | `Marshal` | `p` | `CertificateDigest` |

## Fix Pattern

The standard fix for rangevarref warnings is to create a local copy of the loop variable before taking its address or creating a slice from it:

**Before:**
```go
for _, h := range hashes {
    hasher.Write(h[:])
}
```

**After:**
```go
for _, h := range hashes {
    h := h  // Create local copy
    hasher.Write(h[:])
}
```

Note: Since Go 1.22, range variables are re-created each iteration, making this technically unnecessary. However, the fix ensures compatibility with older Go versions and satisfies the linter.

## Open Questions

1. What Go version does this project target? If Go 1.22+, these warnings may be false positives and could be ignored or suppressed.
2. Is there an existing lint configuration that could be updated to ignore these specific warnings?

## Contradictions

None found. All instances follow the same pattern of using `[:]` on range variables over fixed-size array types.
