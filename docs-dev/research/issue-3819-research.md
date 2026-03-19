# Research: Fix copylocks in Certificate/Header

## Summary

The `Header` struct contains a `sync.RWMutex` field (`mu`) for protecting a cached digest. The `NewCertificate` function currently takes `Header` by value, which copies the mutex - a violation of Go's copylock rules. The fix requires changing `NewCertificate` to take `*Header` instead. Additionally, several locations directly assign `*header` to `Certificate.Header`, which also copies the lock. These all need to be changed to copy field-by-field or use an alternative approach.

## Verified Facts

### Fact 1: Header contains sync.RWMutex
- **Source**: `pkg/consensus/types/header.go:54-59`
- **Content**:
  ```go
  // mu protects the cached digest.
  mu sync.RWMutex
  // digest is the cached header digest, computed lazily.
  digest HeaderDigest
  // digestComputed indicates whether the digest has been computed.
  digestComputed bool
  ```
- **Confidence**: HIGH

### Fact 2: NewCertificate takes Header by value
- **Source**: `pkg/consensus/types/certificate.go:63`
- **Content**:
  ```go
  func NewCertificate(header Header, signatures [][]byte, signedAuthorities []uint16) *Certificate {
  ```
- **Confidence**: HIGH

### Fact 3: Certificate embeds Header by value (not pointer)
- **Source**: `pkg/consensus/types/certificate.go:46-48`
- **Content**:
  ```go
  type Certificate struct {
      // Header is the underlying header being certified.
      Header Header
  ```
- **Confidence**: HIGH

### Fact 4: NewCertificate is called in 17 files
- **Source**: `go vet` output and grep search
- **Content**: 46 call sites across these files:
  - `pkg/consensus/types/certificate.go` (definition)
  - `pkg/consensus/snapshot/snapshot_test.go`
  - `pkg/consensus/primary/cert_sync_test.go` (4 calls)
  - `pkg/consensus/primary/certificate_handler_test.go` (8 calls)
  - `pkg/consensus/primary/certificate_handler.go` (2 calls signalNewCertificate)
  - `pkg/consensus/primary/header_builder_test.go` (4 calls)
  - `pkg/consensus/primary/pending_certs_test.go` (11 calls)
  - `pkg/consensus/primary/primary_test.go` (3 calls)
  - `pkg/consensus/primary/vote_handler.go` (2 calls)
  - `pkg/consensus/consensus.go`
  - `pkg/consensus/genesis/genesis.go`
  - `pkg/consensus/gossip/cert_sync_test.go` (2 calls)
  - `pkg/consensus/gossip/gossip_test.go`
  - `pkg/consensus/gossip/protocols_test.go` (2 calls)
  - `internal/node/dagbft/integration_test.go` (7 calls)
- **Confidence**: HIGH

### Fact 5: All current callers dereference a *Header when calling NewCertificate
- **Source**: grep output showing call patterns
- **Content**: All calls use pattern `NewCertificate(*header, ...)` or `NewCertificate(*clonedHeader, ...)`
- **Confidence**: HIGH - The fix will be straightforward since callers already have `*Header`

### Fact 6: Additional copylock warnings exist in Certificate code
- **Source**: `go vet ./pkg/consensus/types/...` output
- **Content**:
  - `certificate.go:65` - literal copies lock value in NewCertificate assignment
  - `certificate.go:363` - UnmarshalCertificate copies lock: `Header: *header`
  - `certificate.go:384` - Clone copies lock: `Header: *header`
- **Confidence**: HIGH

### Fact 7: Test files also have direct Header copying
- **Source**: `go vet` output
- **Content**: Multiple test files directly assign `*header` to `Certificate.Header`:
  - `certificate_test.go:48,66,80,160,174,191,297,323`
  - `dag_test.go:49`
  - `stress_test.go:73,99,403`
- **Confidence**: HIGH

### Fact 8: Header has a Clone() method that properly copies
- **Source**: `pkg/consensus/types/header.go:360-383`
- **Content**:
  ```go
  func (h *Header) Clone() *Header {
      payload := make(map[BatchDigest]WorkerID, len(h.Payload))
      for k, v := range h.Payload {
          payload[k] = v
      }
      // ... deep copy all fields
      return &Header{
          Author:    author,
          Round:     h.Round,
          Epoch:     h.Epoch,
          Payload:   payload,
          Parents:   parents,
          Signature: signature,
      }
  }
  ```
- **Confidence**: HIGH - Clone() creates fresh Header without copying mutex fields

## Code References

### Primary implementation files
- `pkg/consensus/types/certificate.go:63-69` - NewCertificate definition
- `pkg/consensus/types/header.go:40-60` - Header struct with mutex

### Production code callers
- `pkg/consensus/primary/vote_handler.go:193` - creates certificate from aggregated votes
- `pkg/consensus/consensus.go:431` - creates certificate in consensus loop
- `pkg/consensus/genesis/genesis.go:132` - creates genesis certificate

### Test file callers (partial list)
- `pkg/consensus/primary/certificate_handler_test.go` - 8 call sites
- `pkg/consensus/primary/pending_certs_test.go` - 11 call sites
- `internal/node/dagbft/integration_test.go` - 7 call sites

## Open Questions

None - the issue is well-defined and the fix is straightforward.

## Contradictions

None found.

## Recommended Fix

### Option A: Change NewCertificate signature (simpler)
1. Change `NewCertificate(header Header, ...)` to `NewCertificate(header *Header, ...)`
2. Update callers: remove the dereference `*` since they already pass `*Header`
   - Change `NewCertificate(*header, ...)` to `NewCertificate(header, ...)`
3. Inside NewCertificate, use header.Clone() or copy fields individually (not `*header`)

### Option B: Store Header as pointer in Certificate (more invasive)
This would require changing `Certificate.Header Header` to `Certificate.Header *Header` which affects many more call sites and accessors.

**Recommended: Option A** - It's simpler and matches the existing pattern where callers already have `*Header`.

### Additional fixes needed:
1. `UnmarshalCertificate` (line 362-367): Use the returned `*Header` directly, don't dereference
2. `Clone()` (line 383-388): Use the cloned `*Header` directly, don't dereference
3. Test files: Fix direct assignments `Header: *header` to use Clone() or field-by-field copy
