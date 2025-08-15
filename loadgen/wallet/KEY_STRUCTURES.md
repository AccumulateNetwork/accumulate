# Accumulate Key Structures & Authority Hierarchy

## Core Concepts

### 1. Identity Architecture
- **ADI (Accumulate Digital Identifier)**: Primary identity namespace in Accumulate
- **Lite Identity**: Simplified identity derived from public key hash
- **Authority Chain**: Hierarchical signing control mechanism

### 2. Key Hierarchy Levels

```
ADI (Root Identity)
├── Authorities[] (Ordered list of signing authorities)
│   ├── [0] Primary KeyBook (default signer)
│   ├── [1] Manager KeyBook (optional, for key management)
│   └── [N] Additional KeyBooks (custom authorities)
│
└── KeyBook (Authority Container)
    ├── BookType: Normal | Validator
    ├── PageCount: Number of key pages
    └── KeyPage[0..N] (Individual Key Collections)
        ├── Keys[] (KeySpec array)
        ├── AcceptThreshold (signatures needed to accept)
        ├── RejectThreshold (signatures needed to reject)  
        ├── ResponseThreshold (responses before processing)
        └── CreditBalance (for transaction fees)
```

## Key Components

### AccountAuth Structure
```go
type AccountAuth struct {
    Authorities []AuthorityEntry // Ordered by priority
}

type AuthorityEntry struct {
    Url      *url.URL // Reference to KeyBook
    Disabled bool     // Bypasses auth checks if true
}
```

### KeyBook (Authority Container)
- **Purpose**: Groups related key pages under single authority
- **URL Format**: `acc://[adi-name]/[book-name]`
- **Types**:
  - `Normal`: Standard user keybook
  - `Validator`: Network validator keybook
- **Contains**: Multiple KeyPages (indexed 0 to PageCount-1)

### KeyPage (Signature Collection)
- **Purpose**: Holds actual signing keys with threshold logic
- **URL Format**: `acc://[adi-name]/[book-name]/[page-index]`
- **Components**:
  ```go
  type KeyPage struct {
      Keys              []KeySpec    // Public key hashes
      AcceptThreshold   uint64      // M-of-N for acceptance
      RejectThreshold   uint64      // K-of-N for rejection
      ResponseThreshold uint64      // Min responses needed
      BlockThreshold    uint64      // Blocks before timeout
      CreditBalance     uint64      // Transaction fee credits
  }
  ```

### KeySpec (Individual Key)
```go
type KeySpec struct {
    PublicKeyHash []byte   // SHA256 of public key
    LastUsedOn    uint64   // Nonce for replay protection
    Delegate      *url.URL // Optional delegation target
}
```

## Authority Resolution

### 1. Transaction Authorization Flow
```
Transaction → Principal Account → Authorities[0] (Primary KeyBook)
                                → KeyBook → KeyPage[i]
                                          → Verify Signatures vs Keys[]
                                          → Check Thresholds
```

### 2. Priority System
- Authorities are checked in array order (0 = highest priority)
- First authority to meet requirements authorizes transaction
- Manager KeyBook (index 1) typically handles key updates

### 3. Multi-Signature Logic
- **M-of-N**: AcceptThreshold of Keys must sign to approve
- **Rejection**: RejectThreshold can veto transaction
- **Response**: ResponseThreshold ensures minimum participation
- **Timeout**: BlockThreshold sets expiration in blocks

## Security Features

### 1. Replay Protection
- **Nonce Tracking**: Each KeySpec maintains LastUsedOn counter
- **Transaction Nonce**: Must exceed key's LastUsedOn value
- **Auto-increment**: Successful use updates LastUsedOn

### 2. Delegation
- Keys can delegate authority to other accounts
- Enables proxy signing and managed accounts
- Delegate URL points to authorized signer

### 3. Credit System
- KeyPages hold credits for transaction fees
- Prevents spam by requiring fee payment
- Credits deducted per signature validation

## Common Patterns

### 1. Simple Single-Sig Account
```
ADI → Authorities[0] → KeyBook → KeyPage[0]
                                → Keys[0] (single key)
                                → AcceptThreshold: 1
```

### 2. Multi-Sig Treasury
```
ADI → Authorities[0] → KeyBook → KeyPage[0]
                                → Keys[0..4] (5 signers)
                                → AcceptThreshold: 3 (3-of-5)
                                → RejectThreshold: 2 (veto)
```

### 3. Tiered Authority
```
ADI → Authorities[0] → DailyOpsBook   → LowValuePage (1-of-3)
    → Authorities[1] → AdminBook      → HighValuePage (3-of-5)
    → Authorities[2] → RecoveryBook   → EmergencyPage (5-of-7)
```

## Transaction Signing

### 1. Signature Composition
- **Initiator**: First signature includes transaction + nonce
- **Additional**: Co-signers reference initiator's signature
- **Authority**: Final signature from authorized KeyPage

### 2. Signature Types
- **Key Signature**: Direct signing with private key
- **Authority Signature**: Proof of KeyPage authorization
- **Delegated**: Signed on behalf of delegator

### 3. Validation Steps
1. Verify signature cryptographically valid
2. Check signer in KeyPage's Keys[]
3. Validate nonce > LastUsedOn
4. Count towards threshold requirements
5. Update LastUsedOn after success

## Best Practices

### 1. Key Management
- Separate operational and administrative keys
- Use multiple KeyPages for different security levels
- Regular key rotation via Manager KeyBook

### 2. Threshold Settings
- AcceptThreshold ≤ Total Keys (avoid lockout)
- RejectThreshold < AcceptThreshold (veto minority)
- ResponseThreshold ≤ AcceptThreshold

### 3. Authority Design
- Primary authority for routine operations
- Manager authority for key/threshold updates
- Recovery authority with high threshold

## Network Integration

### 1. Partition Routing
- Accounts exist on specific network partitions
- Signatures route to account's partition
- Cross-partition via synthetic transactions

### 2. Authority Caching
- Validators cache authority lookups
- Changes propagate via authority updates
- Eventual consistency model

### 3. Fee Distribution
- Credits purchased with ACME tokens
- Fees paid to validators
- Excess credits refundable

## Summary

The Accumulate key structure provides:
- **Hierarchical Control**: ADI → KeyBook → KeyPage → Keys
- **Flexible Security**: M-of-N signatures with thresholds
- **Replay Protection**: Nonce-based transaction uniqueness
- **Delegation Support**: Proxy signing capabilities
- **Credit System**: Anti-spam fee mechanism

This architecture enables everything from simple single-signature accounts to complex multi-party governance structures while maintaining security and preventing replay attacks.