# MCP Protocol Coverage Analysis

**Date:** 2025-10-17
**Current Status:** Phase 1.5 Complete (22 tools, 75% coverage)

---

## Current Coverage

### ✅ Implemented (22 tools)

#### Query Operations (15 tools)
1. `accumulate_query_account` - Query any account
2. `accumulate_query_tx` - Query transaction status
3. `accumulate_query_chain` - Query chain entries
4. `accumulate_query_data` - Query data entries
5. `accumulate_query_directory` - Query ADI directory
6. `accumulate_query_pending` - Query pending transactions
7. `accumulate_query_minor_block` - Query minor blocks
8. `accumulate_query_major_block` - Query major blocks
9. `accumulate_query_keybook` - Query KeyBook
10. `accumulate_query_keypage` - Query KeyPage
11. `accumulate_search_public_key` - Search by public key
12. `accumulate_search_public_key_hash` - Search by key hash
13. `accumulate_search_anchor` - Search anchors

#### Network Operations (4 tools)
14. `accumulate_node_info` - Node information
15. `accumulate_network_status` - Network status
16. `accumulate_consensus_status` - Consensus status
17. `accumulate_metrics` - Network metrics
18. `accumulate_faucet` - Request testnet tokens

#### Transaction Operations (4 tools)
19. `accumulate_generate_key` - Generate ED25519 keys
20. `accumulate_create_lite_account` - Create lite account URL
21. `accumulate_send_tokens` - Send ACME tokens
22. `accumulate_add_credits` - Add credits to accounts
23. `accumulate_create_adi` - Create ADI with KeyBook

---

## ❌ Missing Coverage

### Critical for Staking (High Priority)

#### 1. Data Account Operations
**Impact:** **BLOCKING** for staking applications

##### `accumulate_create_data_account`
- **Transaction:** `CreateDataAccount`
- **Purpose:** Create data account under ADI
- **Fields:**
  - `Url` - Data account URL (e.g., `acc://myadi.acme/stake-records`)
  - `Authorities` - Optional additional authorities
- **Required for:** Storing stake records, timestamps, participant data

##### `accumulate_write_data`
- **Transaction:** `WriteData`
- **Purpose:** Write data entries to data account
- **Fields:**
  - `Entry` - Data entry (arbitrary bytes)
  - `WriteToState` - Whether to write to state
  - `Scratch` - Scratch space flag
- **Required for:** Recording stake events, tracking rewards, storing metadata

**Staking Use Cases:**
- Store stake amount, timestamp, participant address
- Track reward distribution history
- Record validator participation
- Audit trail for stake operations

#### 2. Token Account Operations
**Impact:** **REQUIRED** for ADI-based staking pools

##### `accumulate_create_token_account`
- **Transaction:** `CreateTokenAccount`
- **Purpose:** Create token account under ADI
- **Fields:**
  - `Url` - Token account URL (e.g., `acc://myadi.acme/tokens`)
  - `TokenUrl` - Token type URL (e.g., `acc://ACME`)
  - `Authorities` - Authority URLs (KeyBooks)
- **Required for:** ADI staking pools, managed token accounts

**Why Critical:**
- Lite accounts cannot have sub-accounts
- ADI token accounts enable complex authority
- Required for pool-based staking

---

### KeyBook/KeyPage Management (Phase 2)

#### 3. Key Management
**Impact:** Required for **multisig** and **key rotation**

##### `accumulate_create_keypage`
- **Transaction:** `CreateKeyPage`
- **Purpose:** Add new KeyPage to existing KeyBook
- **Fields:**
  - `Keys` - Initial keys for new page
- **Use case:** Expand multisig authority, delegate signing

##### `accumulate_update_keypage`
- **Transaction:** `UpdateKeyPage`
- **Purpose:** Add or remove keys from KeyPage
- **Operations:**
  - `Add` - Add new key
  - `Remove` - Remove existing key
  - `SetThreshold` - Update m-of-n threshold
- **Use case:** Key rotation, multisig updates

##### `accumulate_create_keybook`
- **Transaction:** `CreateKeyBook`
- **Purpose:** Create additional KeyBook for ADI
- **Use case:** Separate authorities for different operations

##### `accumulate_update_account_auth`
- **Transaction:** `UpdateAccountAuth`
- **Purpose:** Manage account authorities
- **Operations:**
  - Add/remove authority URLs
  - Update disabled status
- **Use case:** Complex authority hierarchies

---

### Token Issuer Operations (Low Priority)

#### 4. Token Issuance
**Impact:** Only needed for **custom tokens**

##### `accumulate_create_token`
- **Transaction:** `CreateToken`
- **Purpose:** Create new token type
- **Use case:** Custom staking tokens, governance tokens

##### `accumulate_issue_tokens`
- **Transaction:** `IssueTokens`
- **Purpose:** Mint new tokens
- **Use case:** Token distribution, rewards

##### `accumulate_burn_tokens`
- **Transaction:** `BurnTokens`
- **Purpose:** Destroy tokens
- **Use case:** Deflationary mechanics

---

## Coverage Priorities

### Phase 2: Data & Token Accounts (CRITICAL)
**Priority:** 🔴 **BLOCKING STAKING**

**Tools to Add (2):**
1. `accumulate_create_data_account`
2. `accumulate_write_data`
3. `accumulate_create_token_account`

**Impact:** 75% → 85% coverage
**Enables:** Full staking support, data persistence

**Estimated Effort:** 4-6 hours
- New file: `client/data.go` (~150 lines)
- Tool definitions: 3 tools (~90 lines)
- Tool handlers: 3 handlers (~180 lines)
- Routing: 3 cases (~9 lines)
- **Total:** ~430 lines

---

### Phase 3: Key Management (IMPORTANT)
**Priority:** 🟡 **ENHANCES SECURITY**

**Tools to Add (4):**
1. `accumulate_create_keypage`
2. `accumulate_update_keypage`
3. `accumulate_create_keybook`
4. `accumulate_update_account_auth`

**Impact:** 85% → 95% coverage
**Enables:** Key rotation, multisig, complex authority

**Estimated Effort:** 6-8 hours
- New file: `client/authority.go` (~200 lines)
- Tool definitions: 4 tools (~120 lines)
- Tool handlers: 4 handlers (~240 lines)
- **Total:** ~560 lines

---

### Phase 4: Token Issuance (OPTIONAL)
**Priority:** 🟢 **NICE TO HAVE**

**Tools to Add (3):**
1. `accumulate_create_token`
2. `accumulate_issue_tokens`
3. `accumulate_burn_tokens`

**Impact:** 95% → 100% coverage
**Enables:** Custom token creation, issuance

**Estimated Effort:** 4-6 hours

---

## Detailed Phase 2 Specification

### CreateDataAccount Transaction

**Protocol Structure:**
```go
type CreateDataAccount struct {
    Url         *url.URL
    Authorities []*url.URL  // Optional
}
```

**MCP Tool Schema:**
```json
{
  "name": "accumulate_create_data_account",
  "description": "Create a data account under an ADI for storing arbitrary data",
  "inputSchema": {
    "type": "object",
    "properties": {
      "url": {
        "type": "string",
        "description": "Data account URL (e.g., acc://myadi.acme/data)"
      },
      "sponsor": {
        "type": "string",
        "description": "Sponsor account URL (must have credits)"
      },
      "sponsor_private_key": {
        "type": "string",
        "description": "Private key of sponsor account"
      },
      "authorities": {
        "type": "array",
        "items": {"type": "string"},
        "description": "Optional: Additional authority URLs"
      },
      "network": {
        "type": "string",
        "default": "mainnet"
      }
    },
    "required": ["url", "sponsor", "sponsor_private_key"]
  }
}
```

**Example Usage:**
```bash
echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"accumulate_create_data_account","arguments":{"url":"acc://myadi.acme/stake-records","sponsor":"acc://myadi.acme/book","sponsor_private_key":"...","network":"http://127.0.0.1:26660/v3"}}}' | ./mcp-accumulate
```

---

### WriteData Transaction

**Protocol Structure:**
```go
type WriteData struct {
    Entry        []byte  // Data to write
    WriteToState bool    // Write to account state
    Scratch      bool    // Use scratch space
}
```

**MCP Tool Schema:**
```json
{
  "name": "accumulate_write_data",
  "description": "Write data entry to a data account",
  "inputSchema": {
    "type": "object",
    "properties": {
      "account_url": {
        "type": "string",
        "description": "Data account URL"
      },
      "data": {
        "type": "string",
        "description": "Data to write (hex or base64 encoded)"
      },
      "encoding": {
        "type": "string",
        "enum": ["hex", "base64", "utf8"],
        "default": "utf8"
      },
      "signer": {
        "type": "string",
        "description": "Signing authority URL"
      },
      "signer_private_key": {
        "type": "string",
        "description": "Private key of signer"
      },
      "write_to_state": {
        "type": "boolean",
        "default": false
      },
      "network": {
        "type": "string",
        "default": "mainnet"
      }
    },
    "required": ["account_url", "data", "signer", "signer_private_key"]
  }
}
```

**Example Usage (Staking):**
```json
{
  "account_url": "acc://myadi.acme/stake-records",
  "data": "{\"staker\":\"acc://alice.acme\",\"amount\":1000000000,\"timestamp\":1697500000}",
  "encoding": "utf8",
  "signer": "acc://myadi.acme/book",
  "signer_private_key": "...",
  "network": "http://127.0.0.1:26660/v3"
}
```

---

### CreateTokenAccount Transaction

**Protocol Structure:**
```go
type CreateTokenAccount struct {
    Url         *url.URL
    TokenUrl    *url.URL
    Authorities []*url.URL  // Optional
    Scratch     bool
}
```

**MCP Tool Schema:**
```json
{
  "name": "accumulate_create_token_account",
  "description": "Create a token account under an ADI",
  "inputSchema": {
    "type": "object",
    "properties": {
      "url": {
        "type": "string",
        "description": "Token account URL (e.g., acc://myadi.acme/tokens)"
      },
      "token_url": {
        "type": "string",
        "description": "Token type URL (e.g., acc://ACME)",
        "default": "acc://ACME"
      },
      "sponsor": {
        "type": "string",
        "description": "Sponsor account URL (must have credits)"
      },
      "sponsor_private_key": {
        "type": "string",
        "description": "Private key of sponsor account"
      },
      "authorities": {
        "type": "array",
        "items": {"type": "string"},
        "description": "Optional: Authority URLs (defaults to ADI's KeyBook)"
      },
      "network": {
        "type": "string",
        "default": "mainnet"
      }
    },
    "required": ["url", "sponsor", "sponsor_private_key"]
  }
}
```

---

## Staking Application Requirements

### Minimum Viable Coverage (Phase 2)
With Phase 2 complete, staking applications can:
- ✅ Create ADIs for staking pools
- ✅ Create data accounts for stake records
- ✅ Write stake events to data accounts
- ✅ Create token accounts under ADIs
- ✅ Query all stake data
- ✅ Manage credits for operations

**Coverage:** 85%
**Status:** **SUFFICIENT** for basic staking

### Enhanced Coverage (Phase 3)
With Phase 3, staking applications gain:
- ✅ Multisig pool management
- ✅ Key rotation for security
- ✅ Delegated authority
- ✅ Complex governance

**Coverage:** 95%
**Status:** **PRODUCTION READY** for enterprise staking

---

## Recommendation

### Immediate Next Steps
**Implement Phase 2 (Data & Token Accounts):**

1. Create `client/data.go`:
   - `CreateDataAccount()`
   - `WriteData()`
   - `CreateTokenAccount()`

2. Add 3 MCP tools
3. Test with staking use case

**Timeline:** 4-6 hours
**Impact:** Unblocks staking applications

### Future Phases
- **Phase 3:** Key management (when needed for multisig)
- **Phase 4:** Token issuance (if custom tokens required)

---

## Summary

**Current Coverage:** 75% (22 tools)
**Blocking Gap:** Data account operations
**Next Phase:** Phase 2 - Data & Token Accounts
**Target Coverage:** 85% (25 tools)

**After Phase 2, the MCP will support:**
- ✅ Complete ADI lifecycle
- ✅ Data persistence
- ✅ Token account management
- ✅ **Full staking application support**
