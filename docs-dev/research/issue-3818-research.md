# Research: Fix SA1026 json.Marshal in persist.go

## Summary

The staticcheck SA1026 error occurs at `persist.go:248` where `json.Marshal(cert)` is called on a `*types.Certificate`. The `Certificate` struct contains a `Header` field with `Payload map[BatchDigest]WorkerID`. Since `BatchDigest` is a `[32]byte` array (not a string), Go's standard json.Marshal cannot serialize this map as JSON map keys must be strings. The Certificate type already has a binary `Marshal()` method that correctly serializes the Payload map. The recommended fix is to use this existing binary serialization instead of JSON, storing it as base64-encoded data in the JSON checkpoint.

## Verified Facts

### Fact 1: The staticcheck SA1026 error location and cause
- **Source**: `pkg/consensus/persist/persist.go:248`
- **Content**: `data, err := json.Marshal(cert)` attempts to marshal `*types.Certificate`
- **Confidence**: HIGH

### Fact 2: Certificate contains Header with unsupported map type
- **Source**: `pkg/consensus/types/certificate.go:46-60`
- **Content**:
  ```go
  type Certificate struct {
      Header Header
      Signatures [][]byte
      SignedAuthorities []uint16
      StateHash StateHash
  }
  ```
- **Confidence**: HIGH

### Fact 3: Header.Payload has BatchDigest (non-string) key type
- **Source**: `pkg/consensus/types/header.go:48`
- **Content**: `Payload map[BatchDigest]WorkerID`
- **Confidence**: HIGH

### Fact 4: BatchDigest is a [32]byte array
- **Source**: `pkg/consensus/types/batch.go:20`
- **Content**: `type BatchDigest [32]byte`
- **Confidence**: HIGH

### Fact 5: Certificate already has a binary Marshal method
- **Source**: `pkg/consensus/types/certificate.go:213-263`
- **Content**: `func (c *Certificate) Marshal() ([]byte, error)` - serializes using binary encoding with proper handling of the Payload map by sorting keys and writing them as raw bytes
- **Confidence**: HIGH

### Fact 6: Header.Marshal handles Payload correctly via binary encoding
- **Source**: `pkg/consensus/types/header.go:207-267`
- **Content**: `Marshal()` sorts payload keys and writes each as `[digest (32 bytes)][worker (1 byte)]`
- **Confidence**: HIGH

### Fact 7: UnmarshalCertificate exists for deserialization
- **Source**: `pkg/consensus/types/certificate.go:265-368`
- **Content**: `func UnmarshalCertificate(data []byte) (*Certificate, error)` - properly deserializes the binary format
- **Confidence**: HIGH

### Fact 8: CertificateData.Data field is json.RawMessage
- **Source**: `pkg/consensus/persist/persist.go:74`
- **Content**: `Data json.RawMessage \`json:"data"\``
- **Confidence**: HIGH

### Fact 9: Similar hash types use hex encoding for JSON
- **Source**: `pkg/types/record/key_hash.go:46-48`
- **Content**:
  ```go
  func (k KeyHash) MarshalJSON() ([]byte, error) {
      return json.Marshal(k.String())
  }
  ```
- **Confidence**: HIGH

### Fact 10: Build passes, staticcheck fails
- **Source**: Command execution of `go build ./...` and `staticcheck ./pkg/consensus/persist/...`
- **Content**: Build exit code 0, staticcheck returns SA1026 error
- **Confidence**: HIGH

## Code References

| File | Line | Description |
|------|------|-------------|
| `pkg/consensus/persist/persist.go` | 248 | Location of the SA1026 error - `json.Marshal(cert)` |
| `pkg/consensus/persist/persist.go` | 62-75 | `CertificateData` struct with `Data json.RawMessage` |
| `pkg/consensus/types/certificate.go` | 46-60 | `Certificate` struct definition |
| `pkg/consensus/types/certificate.go` | 213-263 | `Certificate.Marshal()` binary serialization |
| `pkg/consensus/types/certificate.go` | 265-368 | `UnmarshalCertificate()` binary deserialization |
| `pkg/consensus/types/header.go` | 40-60 | `Header` struct with `Payload map[BatchDigest]WorkerID` |
| `pkg/consensus/types/header.go` | 207-267 | `Header.Marshal()` binary serialization |
| `pkg/consensus/types/batch.go` | 20 | `type BatchDigest [32]byte` |

## Recommended Solutions

### Option 1: Use Binary Serialization with Base64 Encoding (Recommended)

Replace `json.Marshal(cert)` with the existing `cert.Marshal()` binary serialization, then base64-encode the result for storage in the JSON checkpoint.

**Pros:**
- Uses existing, tested serialization code
- No changes to types package required
- Binary format is more compact
- Already has matching `UnmarshalCertificate()` function

**Implementation:**
```go
// In ToCheckpoint():
data, err := cert.Marshal()
if err != nil {
    continue
}
cp.Certificates = append(cp.Certificates, CertificateData{
    Digest: cert.Digest().String(),
    Round:  cert.Round(),
    Author: fmt.Sprintf("%x", cert.Author()),
    Data:   json.RawMessage(`"` + base64.StdEncoding.EncodeToString(data) + `"`),
})
```

**Restoration:**
```go
// When restoring:
var encoded string
if err := json.Unmarshal(certData.Data, &encoded); err != nil {
    return err
}
data, err := base64.StdEncoding.DecodeString(encoded)
if err != nil {
    return err
}
cert, err := types.UnmarshalCertificate(data)
```

### Option 2: Add Custom MarshalJSON/UnmarshalJSON to Certificate

Add JSON serialization methods to the Certificate type that convert the Payload map to a string-keyed format.

**Pros:**
- Cleaner JSON output (human-readable)
- Direct json.Marshal works

**Cons:**
- Requires modifying the types package
- Need to implement both MarshalJSON and UnmarshalJSON
- Need similar methods for Header
- More complex implementation

### Option 3: Convert to Serializable Format at Persist Layer

Create an intermediate struct with string-keyed maps at the persist layer.

**Pros:**
- No changes to types package
- Human-readable JSON

**Cons:**
- More code in persist package
- Duplicate data structure

## Open Questions

1. **Is there a restore function for CertificateData?** - The `CertificateData.Data` field is populated but no code was found that reads it back. This suggests the feature may be incomplete or the restore logic is in a different location not yet examined.

2. **Should the solution support migration from existing checkpoints?** - If checkpoints already exist with (broken) JSON data, migration may be needed.

## Contradictions

None found. The codebase consistently uses binary serialization for Certificate/Header types, making the use of json.Marshal at persist.go:248 an anomaly.
