# 32. LXR Mining Signature

## Summary

This specification defines the LXR (memory-hard proof-of-work) mining signature type for Accumulate, enabling anti-spam protection and optional proof-of-work requirements for certain transaction types. The design is inspired by Factom's PegNet oracle mining system.

## Motivation

As Accumulate grows, there is a need for optional anti-spam mechanisms that don't rely solely on credit costs. LXR mining signatures provide:

1. **Anti-Spam Protection**: Require computational work for certain operations (account creation, bulk transactions)
2. **Fair Access**: Memory-hard algorithm prevents ASIC dominance
3. **Optional Security Layer**: Applications can require mining proofs for sensitive operations
4. **Economic Incentives**: Mining can be tied to rewards for network services

## Technical Specification

### LXR Hash Algorithm

LXR (pronounced "elixir") is a memory-hard hashing algorithm that:
- Requires significant RAM (typically 1GB) making ASIC development expensive
- Uses a large lookup table with random access patterns
- Is approximately 90% memory-bound, 10% compute-bound
- Provides tunable difficulty through leading zero requirements

### Signature Structure

```go
type LXRMiningSignature struct {
    // Standard signature fields
    PublicKey     []byte      // Miner's public key
    Signer        *url.URL    // Miner's key page URL
    SignerVersion uint64      // Key page version
    Timestamp     uint64      // Mining timestamp
    Vote          VoteType    // How the miner votes
    
    // Mining-specific fields
    Nonce         uint64      // Mining nonce
    Difficulty    uint64      // Target difficulty
    WorkProof     [32]byte    // The resulting hash that meets difficulty
    
    // Memory table configuration
    TableSize     uint32      // Size of memory table (power of 2, e.g., 30 = 1GB)
    TableSeed     uint64      // Seed for table generation
    Passes        uint8       // Number of passes through the table
    
    // Optional fields
    Memo          string      // Mining pool identifier or message
    Data          []byte      // Additional mining metadata
    
    // Signature of the work proof
    Signature     []byte      // ED25519 signature of WorkProof
    TransactionHash [32]byte  // Hash of the transaction being signed
}
```

### Mining Process

1. **Input Construction**:
   ```
   input = transaction_hash || nonce || public_key || timestamp
   ```

2. **LXR Hash Computation**:
   - Generate memory table using TableSeed (if not cached)
   - Apply LXR hash algorithm with specified Passes
   - Result is 32-byte hash

3. **Difficulty Check**:
   - Count leading zeros in hash
   - Must meet or exceed target difficulty

4. **Signature Creation**:
   - Sign the WorkProof with miner's private key
   - Attach signature to complete the mining signature

### Difficulty Calculation

Difficulty is expressed as the number of leading zero bits required:
- Difficulty 10 = approximately 1 in 1,024 hashes
- Difficulty 20 = approximately 1 in 1,048,576 hashes
- Difficulty 30 = approximately 1 in 1,073,741,824 hashes

The actual probability is: `P = 1 / 2^difficulty`

### Verification Process

1. **Reconstruct Input**: Combine transaction hash, nonce, public key, and timestamp
2. **Compute LXR Hash**: Using provided table configuration
3. **Verify Difficulty**: Check if hash has sufficient leading zeros
4. **Verify Signature**: Validate ED25519 signature of WorkProof

## Use Cases

### 1. Anti-Spam for Account Creation

```go
// Require mining proof for creating new identities
transaction := &CreateIdentity{
    Url: "acc://newuser.acme",
}

miningSig := &LXRMiningSignature{
    PublicKey:  minerKey,
    Signer:     keyPageUrl,
    Difficulty: 20, // Moderate difficulty
}

miningSig.Mine(transaction)
miningSig.Sign(privateKey)
```

### 2. Priority Transaction Processing

Applications can offer faster processing for transactions with higher difficulty proofs:

```go
if signature.Difficulty >= 25 {
    processPriority(transaction)
} else {
    processNormal(transaction)
}
```

### 3. Mining Pools and Rewards

Mining pools can be created where multiple miners contribute work:

```go
type MiningPool struct {
    PoolID      string
    Members     []MinerInfo
    Difficulty  uint64
    RewardPool  *TokenAccount
}
```

## Network Configuration

### Difficulty Parameters

Networks can configure mining requirements:

```yaml
mining:
  enabled: true
  requirements:
    createIdentity:
      minDifficulty: 15
      maxDifficulty: 30
    sendTokens:
      minDifficulty: 0  # Optional
      maxDifficulty: 20
  tableConfig:
    defaultSize: 30  # 2^30 bytes = 1GB
    defaultSeed: 0xDEADBEEF
    defaultPasses: 5
```

### Epoch Management

Mining difficulty can be adjusted per epoch:

```go
type MiningEpoch struct {
    EpochNumber      uint64
    StartTime        time.Time
    BaseDifficulty   uint64
    TotalWork        *big.Int  // Cumulative work done
    ActiveMiners     uint64
}
```

## Security Considerations

1. **Memory Requirements**: 1GB default prevents most IoT/embedded attacks
2. **ASIC Resistance**: Memory-hard design makes ASIC development uneconomical
3. **Replay Protection**: Timestamp and nonce prevent replay attacks
4. **Signature Binding**: ED25519 signature binds proof to specific key

## Implementation Notes

### Memory Optimization

- Table can be generated deterministically from seed
- Tables can be cached and reused across mining operations
- Different table sizes for different security levels

### Performance Targets

- Single-threaded CPU: ~10-100 hashes/second (1GB table)
- GPU advantage: Limited to ~10x due to memory bandwidth
- ASIC advantage: Limited to ~100x due to memory costs

## Migration Path

1. **Phase 1**: Add LXRMiningSignature type to protocol
2. **Phase 2**: Enable optional mining for specific transaction types
3. **Phase 3**: Integrate with fee/credit system
4. **Phase 4**: Add mining rewards and pool support

## Related Work

- Factom PegNet: Oracle mining system
- Ethereum Ethash: Memory-hard PoW
- Monero RandomX: CPU-optimized PoW
- Zcash Equihash: Memory-oriented PoW

## Conclusion

LXR mining signatures provide a flexible, ASIC-resistant proof-of-work mechanism that can be used for anti-spam protection, priority processing, and economic incentives. The memory-hard design ensures fair access while the configurable difficulty allows for various use cases.

## Future Enhancements

1. **Adaptive Difficulty**: Automatic adjustment based on network conditions
2. **Mining Rewards**: Direct integration with token distribution
3. **Delegated Mining**: Allow third parties to mine on behalf of users
4. **Multi-Algorithm Support**: Support for different PoW algorithms
5. **Quantum Resistance**: Migration path to quantum-resistant algorithms