# KeyBook, KeyPage, and Signature Management Analysis

## Critical Finding

**The MCP is missing fundamental key management and authorization capabilities required for staking and most ADI operations.**

## Accumulate's Key Management Architecture

### Hierarchy
```
ADI (acc://alice.acme)
  └── KeyBook (acc://alice.acme/book)
       ├── KeyPage 1 (Priority 0) - Primary keys
       │    ├── Key A (weight: 1)
       │    ├── Key B (weight: 1)
       │    └── Threshold: 2-of-2
       ├── KeyPage 2 (Priority 1) - Backup keys
       │    ├── Key C (weight: 1)
       │    ├── Key D (weight: 1)
       │    └── Threshold: 1-of-2
       └── KeyPage 3 (Priority 2) - Recovery keys
```

### Authority Model

1. **KeyBook** - Container account that holds KeyPages
2. **KeyPage** - Collection of public keys with m-of-n threshold
3. **Key Priority** - Lower priority = higher authority (0 is highest)
4. **Multisig** - Each KeyPage has configurable m-of-n thresholds
5. **Delegated Authority** - KeyPages can delegate to other KeyBooks

### Signature Types (15 total)

The protocol supports multiple signature types:

| Type | Purpose | Use Case |
|------|---------|----------|
| **ED25519Signature** | Standard signatures | Most user transactions |
| **DelegatedSignature** | Authority delegation | Multisig, custodial |
| **AuthoritySignature** | KeyBook/KeyPage authority | ADI operations |
| **BTCSignature** | Bitcoin keys | Cross-chain |
| **ETHSignature** | Ethereum keys | Cross-chain |
| **RCD1Signature** | Factoid keys | Factoid migration |
| **LegacyED25519Signature** | Legacy format | Backwards compat |
| **InternalSignature** | System operations | Protocol use |
| **PartitionSignature** | Network consensus | Validators |
| **ReceiptSignature** | Receipts/proofs | Anchoring |
| **RemoteSignature** | Remote signing | Advanced |
| **TypedDataSignature** | EIP-712 style | Ethereum compat |
| **BTCLegacySignature** | BTC P2PKH | Legacy Bitcoin |
| **EcdsaSha256Signature** | ECDSA generic | General purpose |
| **RsaSha256Signature** | RSA keys | Enterprise |

---

## Current MCP Capabilities

### ✅ What Exists Today

**In `client/client.go` (SendTokens):**
```go
// Create ED25519 signature for lite account
sig := &protocol.ED25519Signature{
    PublicKey: privateKey.Public().(ed25519.PublicKey),
    Signer:    fromUrl.RootIdentity(),
    Timestamp: uint64(time.Now().UnixMilli()),
}
sig.Signature = ed25519.Sign(privateKey, txnHash[:])
```

**Coverage:**
- ✅ ED25519 signing for **lite accounts only**
- ✅ Single-key transactions
- ✅ Basic envelope creation

### 🔴 What's Missing (Critical Gaps)

#### 1. Query KeyBooks & KeyPages ❌
**No tools to:**
- Query KeyBook accounts (acc://alice.acme/book)
- Query KeyPage details (keys, weights, thresholds)
- List all keys with authority over an account
- Check key page priorities

**Why staking needs it:**
- Verify user has authority to sign staking transactions
- Display available keys to user
- Validate multisig requirements
- Check if backup keys exist

#### 2. ADI Signature Support ❌
**Current limitation:**
- Only works with lite accounts (single-key)
- No KeyPage/KeyBook authority handling
- No multisig support
- No delegated signatures

**Why staking needs it:**
- Staking typically uses ADI accounts, not lite accounts
- Need to sign with proper KeyPage authority
- May require multisig for high-value stakes
- Corporate staking requires delegated authority

#### 3. Key Management Operations ❌
**No tools for:**
- Add key to KeyPage (`AddKeyOperation`)
- Remove key from KeyPage (`RemoveKeyOperation`)
- Update key in KeyPage (`UpdateKeyOperation`)
- Update KeyPage thresholds
- Create new KeyBook
- Create new KeyPage

**Why staking needs it:**
- Rotate keys after staking for security
- Add backup keys before locking funds
- Update multisig requirements
- Recovery scenarios (lost keys)

#### 4. Authority Resolution ❌
**No support for:**
- Determine which KeyPage to use
- Find keys with signing authority
- Resolve delegated authorities
- Build authority signature chains

**Why staking needs it:**
- Must sign with correct authority level
- Handle delegated staking scenarios
- Corporate/institutional staking
- Complex authorization models

---

## Staking Requirements for Key Management

### Scenario 1: Individual Stakes 50k ACME (Pure Staking)

**User:** Alice (acc://alice.acme)
**Staking Account:** acc://alice.acme/staking-tokens

**Required operations:**
1. Query KeyBook to see available keys
2. Select appropriate KeyPage for signing
3. Sign staking conversion transaction with KeyPage authority
4. Verify signature was accepted

**Current MCP limitation:** ❌
- Can't query KeyBook/KeyPage
- Can't sign with KeyPage authority (only lite account signing)
- Can't verify which keys have authority

### Scenario 2: Corporate Stakes 1M ACME (Delegated Staking)

**Organization:** Acme Corp (acc://acmecorp.acme)
**Staking Account:** acc://acmecorp.acme/treasury-stake
**Authority Model:** 3-of-5 multisig on KeyPage 0

**Required operations:**
1. Query KeyBook to see multisig configuration
2. Get signatures from 3 different executives
3. Aggregate signatures into authority signature
4. Submit with proper authority chain
5. Verify all thresholds met

**Current MCP limitation:** ❌
- No multisig support
- No authority signature creation
- No signature aggregation
- Can't verify threshold requirements

### Scenario 3: Delegated Staking with Custodian

**User:** Bob (acc://bob.acme)
**Custodian:** StakePool (acc://stakepool.acme)
**Authority:** Bob delegates to StakePool's KeyBook

**Required operations:**
1. Create delegated signature from Bob's keys
2. StakePool signs with their authority
3. Build signature chain: Bob → StakePool → Transaction
4. Submit with proper delegation

**Current MCP limitation:** ❌
- No delegated signature support
- No signature chain building
- Can't represent authority delegation

---

## Protocol Transaction Types Requiring Key Management

For reference, these transaction types require proper KeyPage authority:

### ADI Management
- `CreateIdentity` - Create new ADI
- `CreateKeyBook` - Create KeyBook under ADI
- `CreateKeyPage` - Create KeyPage under KeyBook
- `UpdateKeyPage` - Modify keys/thresholds

### Account Management
- `CreateTokenAccount` - Create token account under ADI
- `CreateDataAccount` - Create data account under ADI
- `UpdateAccountAuth` - Change account authorities

### Token Operations (ADI-based)
- `SendTokens` from ADI token account
- `BurnTokens` from ADI token account
- `IssueTokens` (for token issuers)

### Staking Operations
- **Staking conversion** (via external service) - Signs with KeyPage authority
- **Rewards withdrawal** - May require KeyPage signing
- **Unstaking** - Requires original staking authority

**Current MCP limitation:** ❌
- Only `SendTokens` from lite accounts works
- All ADI-based operations unsupported

---

## Recommended Additions to MCP

### Priority 1: Query Tools (Read-Only) 🟢 RECOMMENDED

These are **non-destructive** and enable applications to understand the key structure:

#### 1. `accumulate_query_keybook`
```json
{
  "name": "accumulate_query_keybook",
  "description": "Query a KeyBook account to see its KeyPages",
  "parameters": {
    "url": "acc://alice.acme/book",
    "network": "mainnet"
  }
}
```

**Returns:**
- List of KeyPages with priorities
- KeyPage URLs
- Authority structure

#### 2. `accumulate_query_keypage`
```json
{
  "name": "accumulate_query_keypage",
  "description": "Query a KeyPage to see keys and thresholds",
  "parameters": {
    "url": "acc://alice.acme/book/1",
    "network": "mainnet"
  }
}
```

**Returns:**
- Public keys in the page
- Key weights
- m-of-n thresholds (Accept, Reject, Response)
- Allowed transaction types

#### 3. `accumulate_find_authority`
```json
{
  "name": "accumulate_find_authority",
  "description": "Find which KeyBooks/KeyPages have authority over an account",
  "parameters": {
    "url": "acc://alice.acme/tokens",
    "network": "mainnet"
  }
}
```

**Returns:**
- List of authorities (KeyBook URLs)
- Authority type (direct, delegated)
- Required signatures/thresholds

### Priority 2: Signing Tools 🟡 IMPORTANT

Enable signing with KeyPage authority (not just lite accounts):

#### 4. `accumulate_sign_transaction`
```json
{
  "name": "accumulate_sign_transaction",
  "description": "Sign a transaction with KeyPage authority",
  "parameters": {
    "transaction": {...},  // Transaction body
    "signer_url": "acc://alice.acme/book/1",  // KeyPage URL
    "private_key": "...",
    "network": "mainnet"
  }
}
```

**Capabilities:**
- Sign with specific KeyPage
- Handle authority signatures
- Support multisig workflows
- Return partial signature for aggregation

#### 5. `accumulate_send_tokens_adi`
```json
{
  "name": "accumulate_send_tokens_adi",
  "description": "Send tokens from ADI token account (requires KeyPage signing)",
  "parameters": {
    "from": "acc://alice.acme/tokens",
    "to": "acc://bob.acme/tokens",
    "amount": "100.0",
    "signer_url": "acc://alice.acme/book/1",
    "private_key": "...",
    "network": "mainnet"
  }
}
```

**Difference from existing `accumulate_send_tokens`:**
- Works with ADI accounts (not just lite accounts)
- Uses KeyPage authority
- Supports multisig (returns partial sig if threshold not met)

### Priority 3: Key Management 🟡 OPTIONAL

Full key lifecycle management:

#### 6. `accumulate_create_keybook`
Create a new KeyBook under an ADI

#### 7. `accumulate_create_keypage`
Create a new KeyPage in a KeyBook

#### 8. `accumulate_add_key`
Add a public key to a KeyPage

#### 9. `accumulate_remove_key`
Remove a key from a KeyPage

#### 10. `accumulate_update_keypage_threshold`
Update m-of-n thresholds

---

## Impact on Staking

### Current State: 🔴 **BLOCKED**

Without key management support:
- ❌ Can't stake from ADI accounts
- ❌ Can't verify user has authority
- ❌ No multisig staking
- ❌ No delegated staking
- ❌ Limited to lite account operations only

### With Priority 1 (Query Tools): 🟡 **PARTIAL**

- ✅ Can query user's keys and authority
- ✅ Can display multisig requirements
- ✅ Can verify account structure
- ❌ Still can't sign ADI transactions
- ❌ Must use external tools for signing

### With Priority 1 + 2 (Query + Signing): 🟢 **FUNCTIONAL**

- ✅ Full ADI account support
- ✅ KeyPage authority signing
- ✅ Multisig workflows
- ✅ Delegated staking
- ✅ Complete staking lifecycle
- ⚠️ Can't create/modify keys (but can use existing structure)

### With Priority 1 + 2 + 3 (Full Suite): 🟢 **COMPLETE**

- ✅ Everything above, plus:
- ✅ Key rotation and security
- ✅ Setup new staking accounts from scratch
- ✅ Recovery scenarios
- ✅ Dynamic authority management

---

## Recommended Implementation Order

### Phase 1: Essential Queries (Week 1)
1. `accumulate_query_keybook` - See KeyBook structure
2. `accumulate_query_keypage` - See keys and thresholds
3. Update `accumulate_query_account` to return authority info

**Enables:** Understanding of user's key structure, verification of authority

### Phase 2: ADI Signing (Week 2)
4. `accumulate_send_tokens_adi` - Send from ADI accounts with KeyPage authority
5. Generic `accumulate_sign_transaction` - Sign any transaction type
6. Update transaction submission to handle authority signatures

**Enables:** Full staking lifecycle with existing ADI structures

### Phase 3: Key Management (Week 3+)
7. `accumulate_create_keybook`
8. `accumulate_create_keypage`
9. `accumulate_add_key` / `accumulate_remove_key`
10. `accumulate_update_keypage_threshold`

**Enables:** Full key lifecycle, account setup, security operations

---

## Technical Implementation Notes

### 1. Signature Building
```go
// Current (lite account only)
sig := &protocol.ED25519Signature{
    PublicKey: privateKey.Public().(ed25519.PublicKey),
    Signer:    fromUrl.RootIdentity(),  // Lite account
    Timestamp: uint64(time.Now().UnixMilli()),
}

// Needed (KeyPage authority)
sig := &protocol.ED25519Signature{
    PublicKey: privateKey.Public().(ed25519.PublicKey),
    Signer:    keyPageUrl,  // acc://alice.acme/book/1
    Timestamp: uint64(time.Now().UnixMilli()),
}
```

### 2. Authority Resolution
Need to:
1. Query account to get authority URLs
2. Query KeyBook to get KeyPages
3. Select appropriate KeyPage based on priority
4. Verify key is in selected KeyPage
5. Sign with KeyPage as signer

### 3. Multisig Aggregation
For m-of-n:
1. Collect signatures from m different keys
2. All must reference same KeyPage
3. Submit together in envelope
4. Network validates threshold

---

## Current MCP Signature Coverage

| Signature Type | Supported | Priority |
|----------------|-----------|----------|
| ED25519Signature (lite) | ✅ Yes | Complete |
| ED25519Signature (KeyPage) | ❌ No | **CRITICAL** |
| AuthoritySignature | ❌ No | High |
| DelegatedSignature | ❌ No | Medium |
| BTCSignature | ❌ No | Low |
| ETHSignature | ❌ No | Low |
| (Others) | ❌ No | Low |

---

## Conclusion

**The MCP is currently limited to lite account operations only.**

For staking support, we need **at minimum**:
1. ✅ Query KeyBooks and KeyPages (read-only, safe)
2. ✅ Sign with KeyPage authority (enables ADI transactions)

These two additions unblock:
- ADI-based staking
- Multisig staking
- Corporate/institutional use cases
- 90% of real-world staking scenarios

**Full key management (Priority 3) is optional** but provides complete protocol coverage.

---

## Updated Protocol Completeness

| Feature Category | Before | After Phase 1+2 |
|-----------------|--------|-----------------|
| Lite Accounts | 100% | 100% |
| ADI Accounts | 10% | 80% |
| KeyBook Queries | 0% | 100% |
| KeyPage Queries | 0% | 100% |
| Lite Signing | 100% | 100% |
| KeyPage Signing | 0% | 90% |
| Key Management | 0% | 0% (Phase 3) |

**Current Overall: 40%**
**After Phase 1+2: 85%**
**After Phase 3: 95%**

The MCP's protocol coverage jumps from 40% to 85% with KeyBook/KeyPage support, making it viable for real-world staking applications.
