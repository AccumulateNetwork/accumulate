# KeyPage Mining Fields Usage Guide

## Overview

The KeyPage schema has been extended with optional mining fields to support LXR mining functionality. These fields enable third-party applications to configure mining permissions and difficulty on a per-key basis.

## New Fields

### `MiningDifficulty` (optional bytes)

**Purpose**: Stores the mining difficulty target hash for a specific key entry.

**Usage**:
- `nil` or empty: Mining disabled for this key
- Non-empty bytes: Mining enabled with specified difficulty target
- Typically 32 bytes but flexible length supported

**Example**:
```go
keySpec := &KeySpec{
    PublicKeyHash:    publicKey,
    LastUsedOn:       blockHeight,
    MiningDifficulty: []byte{0x00, 0x00, 0x01, ...}, // 32-byte difficulty target
}
```

### `MiningExpiry` (optional uint64)

**Purpose**: Block height when mining permission expires for this key.

**Usage**:
- `0`: No expiry (mining allowed indefinitely)
- `> 0`: Mining disabled after this block height
- Used for time-limited mining access control

**Example**:
```go
keySpec := &KeySpec{
    PublicKeyHash: publicKey,
    LastUsedOn:    currentBlock,
    MiningExpiry:  currentBlock + 100000, // Expires in ~100k blocks
}
```

## Use Cases

### 1. Third-Party App Mining

Applications can set custom mining difficulty for registered miners:

```go
// Enable mining for a user with medium difficulty
userKey := &KeySpec{
    PublicKeyHash:    userPublicKey,
    MiningDifficulty: mediumDifficultyTarget,
    MiningExpiry:     0, // No expiry
}
keyPage.AddKeySpec(userKey)
```

### 2. Temporary Mining Access

Grant time-limited mining permissions:

```go
// Grant 30-day mining access
expiryBlock := currentBlock + (30 * 24 * 60 * 10) // ~30 days
tempMiner := &KeySpec{
    PublicKeyHash:    tempPublicKey,
    MiningDifficulty: standardDifficulty,
    MiningExpiry:     expiryBlock,
}
```

### 3. Mining Difficulty Tiers

Implement different mining tiers:

```go
// Premium tier - easier difficulty
premiumKey := &KeySpec{
    MiningDifficulty: easyDifficultyTarget,
}

// Standard tier - normal difficulty  
standardKey := &KeySpec{
    MiningDifficulty: normalDifficultyTarget,
}

// Disabled mining
disabledKey := &KeySpec{
    MiningDifficulty: nil, // Mining disabled
}
```

## Integration Points

### For Mining Validation

```go
func (executor *MiningExecutor) ValidateProof(keySpec *KeySpec, proof []byte) error {
    // Check if mining is enabled
    if len(keySpec.MiningDifficulty) == 0 {
        return errors.New("mining disabled for this key")
    }
    
    // Check if mining permission has expired
    if keySpec.MiningExpiry > 0 && currentBlock > keySpec.MiningExpiry {
        return errors.New("mining permission expired")
    }
    
    // Validate proof against difficulty target
    return validateProofOfWork(proof, keySpec.MiningDifficulty)
}
```

### For App Registration

```go
func (app *MiningApp) RegisterMiner(publicKey []byte, tier string) error {
    difficulty := app.getDifficultyForTier(tier)
    expiry := app.getExpiryForTier(tier)
    
    keySpec := &KeySpec{
        PublicKeyHash:    publicKey,
        MiningDifficulty: difficulty,
        MiningExpiry:     expiry,
    }
    
    return app.keyPage.AddKeySpec(keySpec)
}
```

## Backward Compatibility

- **Existing KeyPages**: Continue to work unchanged
- **Legacy keys**: Missing mining fields default to zero values (mining disabled)
- **Migration**: No migration required - new fields are optional

## Security Considerations

1. **Access Control**: Only authorized entities should modify mining fields
2. **Difficulty Validation**: Validate difficulty targets to prevent abuse
3. **Expiry Management**: Implement proper cleanup of expired mining permissions
4. **Rate Limiting**: Consider rate limits for mining field updates

## Next Steps

This foundation enables implementation of:
- **Mining Transaction Types** (#3668)
- **Mining Account Types** (#3669) 
- **Miner Registration Systems** (#3674)
- **Mining Validation Logic** (#3670)

See the [LXR Mining Implementation Order](./LXR_MINING_IMPLEMENTATION_ORDER.md) for the complete roadmap.