# LXR Mining - Third-Party App Integration (Simplified)

## Overview

This document describes how third-party applications can integrate LXR mining into their Accumulate-based systems using existing Accumulate primitives.

## Design Approach

Instead of creating new complex account types, we leverage Accumulate's existing infrastructure:
- **Mining Accounts** (DataAccounts) for configuration and state
- **KeyBooks** for miner registries
- **Token Accounts** for stake and rewards
- **Existing transaction types** for all operations

## Architecture

### 1. App Mining Account (Namespace)

Each app creates a mining account as their namespace:

```
acc://myapp.acme/mining          (DataAccount - configuration & state)
acc://myapp.acme/mining/miners   (KeyBook - registered miners)
acc://myapp.acme/mining/rewards  (TokenAccount - reward pool)
acc://myapp.acme/mining/stake    (TokenAccount - staking pool)
```

### 2. Configuration Storage

The mining configuration is stored as entries in the DataAccount:

```json
{
  "config": {
    "version": 1,
    "mining": {
      "algorithm": "lxr",
      "tableSize": 30,
      "tableSeed": "0xDEADBEEF",
      "passes": 5,
      "epochDuration": 3600,
      "baseDifficulty": 20,
      "maxDifficulty": 40
    },
    "requirements": {
      "minStake": 1000000000,
      "registrationFee": 100000000
    },
    "rewards": {
      "blockReward": 10000000,
      "distribution": "proportional"
    }
  }
}
```

### 3. Miner Registration

Miners register by adding their key to the KeyBook and staking tokens:

```go
// Step 1: Add key to the miners KeyBook
updateKeyPage := &UpdateKeyPage{
    Target: "acc://myapp.acme/mining/miners",
    Operation: AddKey,
    Key: minerPublicKey,
}

// Step 2: Stake tokens (regular token transfer)
stakeTokens := &SendTokens{
    From: minerAccount,
    To: "acc://myapp.acme/mining/stake",
    Amount: requiredStake,
    Memo: minerID, // Links stake to miner
}
```

### 4. Mining State Tracking

Current mining state stored in DataAccount entries:

```json
{
  "epoch": 42,
  "state": {
    "currentDifficulty": 25,
    "totalHashrate": "1.5 TH/s",
    "activeMiners": 127,
    "lastBlockTime": 1656789012,
    "totalWork": "0x1234567890abcdef"
  },
  "miners": {
    "acc://miner1.acme": {
      "lastSubmission": 1656789000,
      "blocksFound": 15,
      "totalWork": "0x123456"
    }
  }
}
```

### 5. Custom Validation Rules

Apps validate mining proofs in their transaction validation:

```go
func validateTransaction(txn Transaction, sigs []Signature) error {
    // Look for LXR mining signature
    for _, sig := range sigs {
        if lxrSig, ok := sig.(*LXRMiningSignature); ok {
            // Load app mining config
            config := loadMiningConfig("acc://myapp.acme/mining")
            
            // Verify miner is registered
            if !isRegisteredMiner(lxrSig.Signer, config) {
                return ErrUnregisteredMiner
            }
            
            // Verify difficulty meets requirements
            if lxrSig.Difficulty < config.MinDifficulty {
                return ErrInsufficientDifficulty
            }
            
            // Additional app-specific validation
            return validateAppSpecific(txn, lxrSig, config)
        }
    }
    
    return ErrMiningProofRequired
}
```

### 6. Difficulty Management in KeyPage

Per-miner difficulty can be stored in KeyPage fields:

```go
type KeyPageExtension struct {
    // Existing KeyPage fields...
    
    // Mining-specific fields (optional)
    MiningDifficulty uint64  `json:"miningDifficulty,omitempty"`
    MiningQuota      uint64  `json:"miningQuota,omitempty"`
    LastMiningBlock  uint64  `json:"lastMiningBlock,omitempty"`
}
```

## Implementation Example

### Setting Up Mining for an App

```go
// 1. Create mining namespace
createDataAccount := &CreateDataAccount{
    Url: "acc://myapp.acme/mining",
}

// 2. Create miners registry
createKeyBook := &CreateKeyBook{
    Url: "acc://myapp.acme/mining/miners",
    PublicKeyHash: appControlKey,
}

// 3. Create reward pool
createTokenAccount := &CreateTokenAccount{
    Url: "acc://myapp.acme/mining/rewards",
    TokenUrl: "acc://ACME",
}

// 4. Write initial configuration
writeConfig := &WriteData{
    Target: "acc://myapp.acme/mining",
    Entry: &AccumulateDataEntry{
        Data: [][]byte{
            []byte("config"),
            configJSON,
        },
    },
}
```

### Miner Registration Process

```go
// Miner side
func registerAsMiner(appMining, minerKey string) error {
    // 1. Check requirements
    config := getConfig(appMining)
    
    // 2. Add key to registry
    tx1 := &UpdateKeyPage{
        Target: appMining + "/miners",
        Operation: AddKey,
        Key: minerKey,
    }
    
    // 3. Stake required tokens
    tx2 := &SendTokens{
        To: appMining + "/stake",
        Amount: config.MinStake,
    }
    
    // 4. Wait for confirmation
    return waitForRegistration(appMining, minerKey)
}
```

### Mining and Submission

```go
func mineForApp(appMining string, txn Transaction) (*LXRMiningSignature, error) {
    // 1. Get current requirements
    config := getConfig(appMining)
    
    // 2. Create mining signature
    miningSig := &LXRMiningSignature{
        PublicKey:     minerPublicKey,
        Signer:        minerKeyPage,
        SignerVersion: keyPageVersion,
        Timestamp:     time.Now().Unix(),
        TableSize:     config.TableSize,
        TableSeed:     config.TableSeed,
        Passes:        config.Passes,
    }
    
    // 3. Mine until difficulty met
    err := miningSig.Mine(txn, config.CurrentDifficulty)
    if err != nil {
        return nil, err
    }
    
    // 4. Sign the proof
    miningSig.Sign(minerPrivateKey)
    
    return miningSig, nil
}
```

## Benefits of This Approach

1. **No Protocol Changes**: Uses only existing Accumulate account types
2. **Flexible**: Apps can customize their mining requirements
3. **Transparent**: All configuration and state visible on-chain
4. **Upgradeable**: Apps can modify configs via DataAccount updates
5. **Permissioned**: Apps control their miner registry via KeyBooks
6. **Economic**: Built-in staking and rewards via TokenAccounts

## Migration Path

1. **Phase 1**: Apps experiment with mining using DataAccounts
2. **Phase 2**: Standardize configuration format
3. **Phase 3**: Add native protocol support if patterns emerge
4. **Phase 4**: Optimize with dedicated mining account types if needed

## Conclusion

This simplified approach allows apps to integrate LXR mining today using existing Accumulate primitives, providing immediate value while allowing for future protocol optimizations based on real-world usage patterns.