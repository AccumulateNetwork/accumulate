# Mining Fields Schema (Issue #3666)

This document describes the mining fields added to the KeyPage schema as foundation for LXR mining implementation.

## KeySpec Schema Changes

Two new optional fields have been added to the `KeySpec` type in `protocol/general.yml`:

### MiningDifficulty
- **Type**: `bytes` (optional)
- **Purpose**: Mining difficulty target hash for this key entry
- **Usage**: Used by third-party apps to set custom mining difficulty per miner
- **Values**:
  - `nil`: Mining disabled for this key
  - `32-byte hash`: Target difficulty for mining operations

### MiningExpiry  
- **Type**: `uvarint` (optional)
- **Purpose**: Block height when mining permission expires for this key entry
- **Usage**: Used for time-limited mining access control
- **Values**:
  - `0`: No expiry (permanent mining access if enabled)
  - `> 0`: Block height when mining expires

## Schema Definition

```yaml
KeySpec:
  fields:
    - name: PublicKeyHash
      type: bytes
      alternative: PublicKey
    - name: LastUsedOn
      type: uvarint
    - name: Delegate
      type: url
      pointer: true
    - name: MiningDifficulty
      description: is the mining difficulty target hash for this key entry. Used by third-party apps to set custom mining difficulty per miner. nil means mining disabled for this key.
      type: bytes
      optional: true
    - name: MiningExpiry
      description: is the block height when mining permission expires for this key entry. Zero means no expiry. Used for time-limited mining access control.
      type: uvarint
      optional: true
```

## Generated Go Types

When `go generate ./protocol` is run, these fields appear in the generated `KeySpec` struct:

```go
type KeySpec struct {
    PublicKeyHash    []byte   `json:"publicKeyHash"`
    LastUsedOn       uint64   `json:"lastUsedOn"`
    Delegate         *url.URL `json:"delegate,omitempty"`
    
    // Mining Fields (NEW in Issue #3666)
    MiningDifficulty []byte   `json:"miningDifficulty,omitempty"`
    MiningExpiry     uint64   `json:"miningExpiry,omitempty"`
}
```

## Backward Compatibility

- These fields are **optional** and default to their zero values
- Existing KeyPage accounts continue to work unchanged
- Applications can check for mining field presence using nil/zero checks

## Foundation for Future Issues

This schema foundation enables implementation of:
- **Mining Transaction Types** (#3668)
- **Mining Account Types** (#3669) 
- **Miner Registration Systems** (#3674)
- **Mining Validation Logic** (#3670)

See the [LXR Mining Implementation Order](../LXR_MINING_IMPLEMENTATION_ORDER.md) for the complete roadmap.

## Testing

Basic schema validation is covered in the existing protocol test suite. The fields are tested for:
- Proper serialization/deserialization
- JSON marshaling compatibility  
- Copy operations
- Field presence/absence handling

More comprehensive mining functionality tests will be added in later issues as the features are implemented.