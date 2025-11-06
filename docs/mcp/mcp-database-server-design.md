# Accumulate Database MCP Server Design Specification

## Overview

This document specifies the design of a Model Context Protocol (MCP) server for **direct database access** to Accumulate blockchain databases. Unlike the API-based MCP server, this server operates on database files directly, enabling historical queries, debugging, analysis, and Merkle proof generation.

## Purpose & Use Cases

### Primary Use Cases

1. **Historical Data Analysis**
   - Query past databases without running a node
   - Analyze account state at specific heights
   - Extract transaction history

2. **Database Debugging**
   - Inspect database contents
   - Verify Merkle proofs
   - Check BPT integrity
   - Analyze chain health

3. **Bulk Data Extraction**
   - Export all accounts
   - Generate reports
   - Statistical analysis
   - Data migration

4. **Merkle Proof Generation**
   - Generate proofs for accounts
   - Verify chain entries
   - Audit trail creation

5. **Development & Testing**
   - Inspect test databases
   - Debug snapshot files
   - Validate database migrations

### Differences from API-Based MCP

| Feature | API MCP | Database MCP |
|---------|---------|--------------|
| **Data Source** | Live node via JSON-RPC | Database files directly |
| **Node Required** | ✅ Yes | ❌ No |
| **Historical Data** | Limited to node history | Full database history |
| **Performance** | Network latency | Direct file I/O |
| **Merkle Proofs** | Via API calls | Generated directly |
| **Write Access** | Submit transactions | Read-only |
| **Use Case** | Live blockchain interaction | Analysis & debugging |

## Architecture

```
┌─────────────────────────────────────────────────┐
│        AI Assistant (Claude, etc.)              │
└─────────────────┬───────────────────────────────┘
                  │ MCP Protocol
┌─────────────────▼───────────────────────────────┐
│      Accumulate Database MCP Server             │
│  ┌──────────────┐  ┌──────────────┐            │
│  │  MCP Tools   │  │ MCP Resources│            │
│  └──────┬───────┘  └──────┬───────┘            │
└─────────┼──────────────────┼────────────────────┘
          │                  │
┌─────────▼──────────────────▼────────────────────┐
│        Database Access Layer                    │
│  ┌────────────┐  ┌────────────┐  ┌──────────┐  │
│  │   Batch    │  │    BPT     │  │  Chains  │  │
│  │  Manager   │  │  Manager   │  │  Manager │  │
│  └────────────┘  └────────────┘  └──────────┘  │
└─────────┬──────────────────┬─────────────┬──────┘
          │                  │             │
┌─────────▼──────────────────▼─────────────▼──────┐
│         Database Backend (BadgerDB/LevelDB)     │
│  /path/to/database/  or  snapshot.snap          │
└─────────────────────────────────────────────────┘
```

## MCP Tools Specification

### 1. Database Management Tools

#### Tool: `db_open`
**Description:** Open an Accumulate database for querying

**Parameters:**
- `path` (string, required): Path to database directory or snapshot file
- `backend` (enum, optional): Backend type ("badger", "leveldb", "snapshot")
- `read_only` (boolean, optional): Open in read-only mode (default: true)

**Returns:**
- `session_id`: Session identifier for subsequent queries
- `database_type`: Type of database opened
- `partition`: Partition name
- `height`: Current block height

**Example:**
```json
{
  "path": "/var/accumulate/bvn0/database",
  "backend": "badger",
  "read_only": true
}
```

**API Mapping:** `pkg/database.Open()`, `pkg/database/snapshot.Open()`

---

#### Tool: `db_close`
**Description:** Close an open database session

**Parameters:**
- `session_id` (string, required): Session identifier from db_open

**Returns:**
- `status`: "closed"

---

#### Tool: `db_info`
**Description:** Get database information and statistics

**Parameters:**
- `session_id` (string, required): Database session ID

**Returns:**
- `partition`: Partition name
- `height`: Current block height
- `bpt_root_hash`: BPT root hash
- `account_count`: Total number of accounts
- `backend`: Storage backend type
- `size_bytes`: Database size
- `statistics`: Additional statistics

**API Mapping:** `batch.BPT().GetRootHash()`, `batch.Account().Count()`

---

### 2. Account Query Tools

#### Tool: `db_query_account`
**Description:** Query account state from database

**Parameters:**
- `session_id` (string, required): Database session ID
- `url` (string, required): Account URL
- `include_chains` (boolean, optional): Include chain information
- `include_transactions` (boolean, optional): Include recent transactions

**Returns:**
- `account`: Account state (varies by type)
- `account_hash`: Account hash in BPT
- `chains`: Chain information (if requested)
  - `main`: Main chain state
  - `signature`: Signature chain state
  - `scratch`: Scratch chain state
  - etc.
- `recent_transactions`: Recent transaction hashes (if requested)

**Example:**
```json
{
  "session_id": "abc123",
  "url": "acc://alice.acme/tokens",
  "include_chains": true
}
```

**API Mapping:** `batch.Account(url).Main().Get()`, `batch.Account(url).MainChain().Get()`

---

#### Tool: `db_list_accounts`
**Description:** List all accounts in the database (or filtered subset)

**Parameters:**
- `session_id` (string, required): Database session ID
- `prefix` (string, optional): URL prefix filter (e.g., "acc://alice.acme")
- `type` (string, optional): Account type filter
- `start` (integer, optional): Pagination start
- `count` (integer, optional): Number of accounts (max: 1000)

**Returns:**
- `accounts`: Array of account URLs and types
- `total`: Total matching accounts
- `has_more`: Boolean indicating more results available

**API Mapping:** `batch.ForEachAccount()` with filtering

---

#### Tool: `db_get_account_hash`
**Description:** Get the BPT hash for an account

**Parameters:**
- `session_id` (string, required): Database session ID
- `url` (string, required): Account URL

**Returns:**
- `hash`: Account hash (32 bytes hex)
- `exists`: Boolean indicating if account exists

**API Mapping:** `batch.BPT().Get(accountKey)`

---

### 3. Chain Query Tools

#### Tool: `db_query_chain`
**Description:** Query chain entries for an account

**Parameters:**
- `session_id` (string, required): Database session ID
- `url` (string, required): Account URL
- `chain_name` (string, required): Chain name ("main", "signature", "scratch", etc.)
- `start` (integer, optional): Start index
- `count` (integer, optional): Number of entries (max: 1000)
- `expand` (boolean, optional): Expand entry data

**Returns:**
- `chain_name`: Chain name
- `height`: Total chain height
- `anchor`: Current anchor (Merkle root)
- `entries`: Array of chain entries
  - `index`: Entry index
  - `hash`: Entry hash
  - `value`: Entry data (if expanded)

**API Mapping:** `batch.Account(url).Chain(name).Get()`, `chain.Entry(index)`

---

#### Tool: `db_query_chain_entry`
**Description:** Query specific chain entry by index or hash

**Parameters:**
- `session_id` (string, required): Database session ID
- `url` (string, required): Account URL
- `chain_name` (string, required): Chain name
- `index` (integer, optional): Entry index
- `hash` (string, optional): Entry hash (hex)

**Returns:**
- `entry`: Entry data
- `index`: Entry index in chain
- `hash`: Entry hash
- `merkle_proof`: Merkle proof from entry to anchor (optional)

**Note:** Exactly one of `index` or `hash` must be provided.

**API Mapping:** `chain.Entry(index)`, `chain.EntryByHash(hash)`

---

#### Tool: `db_get_chain_anchor`
**Description:** Get current anchor (Merkle root) for a chain

**Parameters:**
- `session_id` (string, required): Database session ID
- `url` (string, required): Account URL
- `chain_name` (string, required): Chain name

**Returns:**
- `anchor`: Merkle root hash
- `height`: Chain height
- `timestamp`: Last update timestamp (if available)

**API Mapping:** `chain.Anchor()`

---

### 4. Transaction Query Tools

#### Tool: `db_query_transaction`
**Description:** Query transaction from database

**Parameters:**
- `session_id` (string, required): Database session ID
- `txid` (string, required): Transaction ID/hash
- `principal` (string, optional): Principal account URL (speeds up lookup)

**Returns:**
- `transaction`: Transaction data
- `status`: Transaction status
- `signatures`: Associated signatures
- `principal`: Principal account
- `produced`: Produced transactions

**API Mapping:** `batch.Account(principal).Transaction(txid).Main().Get()`

---

#### Tool: `db_query_transaction_status`
**Description:** Query transaction status

**Parameters:**
- `session_id` (string, required): Database session ID
- `txid` (string, required): Transaction ID/hash
- `principal` (string, required): Principal account URL

**Returns:**
- `code`: Status code
- `delivered`: Boolean
- `pending`: Boolean
- `result`: Transaction result
- `error`: Error information (if failed)

**API Mapping:** `batch.Account(principal).Transaction(txid).Status().Get()`

---

#### Tool: `db_list_transactions`
**Description:** List transactions for an account

**Parameters:**
- `session_id` (string, required): Database session ID
- `url` (string, required): Account URL
- `start` (integer, optional): Start index
- `count` (integer, optional): Number of transactions (max: 1000)

**Returns:**
- `transactions`: Array of transaction hashes and basic info
- `total`: Total transactions for account

**API Mapping:** Iterate main chain for transaction entries

---

### 5. BPT (Binary Patricia Tree) Tools

#### Tool: `db_bpt_get_root`
**Description:** Get BPT root hash

**Parameters:**
- `session_id` (string, required): Database session ID

**Returns:**
- `root_hash`: BPT root hash (32 bytes hex)
- `account_count`: Number of accounts in BPT

**API Mapping:** `batch.BPT().GetRootHash()`

---

#### Tool: `db_bpt_get_proof`
**Description:** Generate Merkle proof for an account

**Parameters:**
- `session_id` (string, required): Database session ID
- `url` (string, required): Account URL

**Returns:**
- `account_hash`: Hash of account in BPT
- `proof`: Merkle proof (array of hashes)
- `path`: Path through BPT (binary string)
- `root_hash`: BPT root hash

**Notes:** Proof can be verified independently to confirm account inclusion

**API Mapping:** `batch.BPT().Get()` with proof generation

---

#### Tool: `db_bpt_verify_proof`
**Description:** Verify a Merkle proof

**Parameters:**
- `account_hash` (string, required): Account hash to verify
- `proof` (array, required): Merkle proof
- `root_hash` (string, required): Expected root hash

**Returns:**
- `valid`: Boolean indicating if proof is valid
- `computed_root`: Computed root from proof

**Notes:** Utility function, doesn't require session_id

---

#### Tool: `db_bpt_iterate`
**Description:** Iterate over BPT entries

**Parameters:**
- `session_id` (string, required): Database session ID
- `start` (integer, optional): Start position
- `count` (integer, optional): Number of entries (max: 1000)

**Returns:**
- `entries`: Array of BPT entries
  - `key`: Account key hash
  - `value`: Account hash
  - `url`: Account URL (if resolvable)

**API Mapping:** BPT iteration

---

### 6. Data Account Tools

#### Tool: `db_query_data_entries`
**Description:** Query data entries from a data account

**Parameters:**
- `session_id` (string, required): Database session ID
- `url` (string, required): Data account URL
- `start` (integer, optional): Start index
- `count` (integer, optional): Number of entries (max: 100)
- `expand` (boolean, optional): Expand entry data

**Returns:**
- `entries`: Array of data entries
  - `index`: Entry index
  - `hash`: Entry hash
  - `data`: Entry data (if expanded, base64)
- `total`: Total entries

**API Mapping:** `batch.Account(url).Data()` chain traversal

---

### 7. Snapshot Tools

#### Tool: `db_snapshot_info`
**Description:** Get information about a snapshot file

**Parameters:**
- `path` (string, required): Path to snapshot file

**Returns:**
- `partition`: Partition name
- `height`: Snapshot height
- `timestamp`: Snapshot timestamp
- `root_hash`: BPT root hash
- `record_count`: Number of records
- `size_bytes`: File size

**API Mapping:** `snapshot.Open()`, header parsing

---

#### Tool: `db_snapshot_export`
**Description:** Export database to snapshot file

**Parameters:**
- `session_id` (string, required): Database session ID
- `output_path` (string, required): Output snapshot file path
- `partition` (string, required): Partition to export

**Returns:**
- `path`: Output file path
- `size_bytes`: Snapshot file size
- `record_count`: Number of records exported

**Notes:** Requires write permissions, use with caution

**API Mapping:** `snapshot.Collect()`, `snapshot.Create()`

---

### 8. Analysis & Statistics Tools

#### Tool: `db_analyze_accounts`
**Description:** Analyze accounts in database

**Parameters:**
- `session_id` (string, required): Database session ID
- `type_filter` (string, optional): Filter by account type

**Returns:**
- `total_accounts`: Total accounts
- `by_type`: Account counts by type
  - `identity`: Count
  - `tokenAccount`: Count
  - `dataAccount`: Count
  - etc.
- `total_size_bytes`: Total account data size

**API Mapping:** Iterate all accounts with statistics collection

---

#### Tool: `db_analyze_chains`
**Description:** Analyze chain health and statistics

**Parameters:**
- `session_id` (string, required): Database session ID
- `url` (string, required): Account URL

**Returns:**
- `chains`: Analysis for each chain
  - `name`: Chain name
  - `height`: Chain height
  - `anchor`: Merkle root
  - `health`: "ok" or issues detected
  - `discontinuities`: Count of missing entries

**API Mapping:** Chain traversal with health checks

---

#### Tool: `db_get_statistics`
**Description:** Get comprehensive database statistics

**Parameters:**
- `session_id` (string, required): Database session ID

**Returns:**
- `partition`: Partition name
- `height`: Block height
- `accounts`: Account statistics
- `transactions`: Transaction statistics
- `chains`: Chain statistics
- `storage`: Storage statistics

**API Mapping:** Aggregate database queries

---

### 9. Key-Value Store Tools (Advanced)

#### Tool: `db_raw_get`
**Description:** Get raw value by key

**Parameters:**
- `session_id` (string, required): Database session ID
- `key` (string, required): Key (hex encoded)

**Returns:**
- `value`: Raw value (base64)
- `exists`: Boolean

**Notes:** Advanced tool for debugging. Key must be SHA256 hash.

**API Mapping:** `batch.GetValue(key)`

---

#### Tool: `db_raw_iterate`
**Description:** Iterate raw key-value pairs

**Parameters:**
- `session_id` (string, required): Database session ID
- `prefix` (string, optional): Key prefix (hex)
- `start` (integer, optional): Start position
- `count` (integer, optional): Number of pairs (max: 1000)

**Returns:**
- `pairs`: Array of key-value pairs
  - `key`: Key (hex)
  - `value`: Value (base64)

**Notes:** Advanced tool for debugging

**API Mapping:** Backend iteration

---

## MCP Resources Specification

### Resource: `database`
**URI Template:** `database://{session_id}/info`

**Description:** Get database session information

**Returns:** Database info and statistics

---

### Resource: `account`
**URI Template:** `database://{session_id}/account/{url}`

**Description:** Read account data

**Returns:** Account state

---

### Resource: `chain`
**URI Template:** `database://{session_id}/chain/{url}/{chain_name}`

**Description:** Read chain data

**Returns:** Chain entries and anchor

---

### Resource: `bpt`
**URI Template:** `database://{session_id}/bpt`

**Description:** Read BPT root and statistics

**Returns:** BPT root hash and account count

---

### Resource: `transaction`
**URI Template:** `database://{session_id}/transaction/{txid}?principal={url}`

**Description:** Read transaction data

**Returns:** Transaction and status

---

## Configuration

```json
{
  "max_sessions": 10,
  "session_timeout": "1h",
  "read_only": true,
  "allowed_paths": [
    "/var/accumulate/*/database",
    "/snapshots/*.snap"
  ],
  "max_results_per_query": 1000,
  "enable_raw_access": false,
  "enable_export": false
}
```

### Security Configuration

```json
{
  "read_only_mode": true,
  "sandbox_paths": true,
  "allow_export": false,
  "require_confirmation_for_large_queries": true,
  "max_concurrent_sessions": 5
}
```

## Implementation Notes

### Language Choice
**Recommended: Go** (required for database package compatibility)

### Dependencies
```go
import (
    "gitlab.com/AccumulateNetwork/accumulate/pkg/database"
    "gitlab.com/AccumulateNetwork/accumulate/pkg/database/snapshot"
    "gitlab.com/AccumulateNetwork/accumulate/pkg/database/bpt"
    "gitlab.com/AccumulateNetwork/accumulate/pkg/database/merkle"
    "gitlab.com/AccumulateNetwork/accumulate/protocol"
)
```

### Session Management

```go
type Session struct {
    ID       string
    DB       database.Beginner
    Opened   time.Time
    LastUsed time.Time
    ReadOnly bool
}

type SessionManager struct {
    sessions map[string]*Session
    mu       sync.RWMutex
}
```

### Error Handling

All errors must include:
- Error code
- Human-readable message
- Session ID (if applicable)
- Suggested resolution

### Performance Considerations

1. **Session Caching:** Keep database open between queries
2. **Batch Operations:** Use View/Update batches efficiently
3. **Pagination:** Limit result sets to prevent memory issues
4. **Lazy Loading:** Don't expand data unless requested

## Usage Examples

### Example 1: Open Database and Query Account

```python
# Step 1: Open database
session = use_mcp_tool("db_open", {
    "path": "/var/accumulate/bvn0/database",
    "backend": "badger"
})

# Step 2: Query account
account = use_mcp_tool("db_query_account", {
    "session_id": session["session_id"],
    "url": "acc://alice.acme/tokens",
    "include_chains": True
})

print(f"Balance: {account['account']['balance']}")
print(f"Main chain height: {account['chains']['main']['height']}")

# Step 3: Close session
use_mcp_tool("db_close", {
    "session_id": session["session_id"]
})
```

### Example 2: Generate Merkle Proof

```python
# Open database
session = use_mcp_tool("db_open", {
    "path": "/snapshots/mainnet-height-1000000.snap",
    "backend": "snapshot"
})

# Get BPT proof for account
proof = use_mcp_tool("db_bpt_get_proof", {
    "session_id": session["session_id"],
    "url": "acc://ACME"
})

# Verify proof (independent verification)
verified = use_mcp_tool("db_bpt_verify_proof", {
    "account_hash": proof["account_hash"],
    "proof": proof["proof"],
    "root_hash": proof["root_hash"]
})

print(f"Proof valid: {verified['valid']}")
```

### Example 3: Bulk Account Export

```python
# Open database
session = use_mcp_tool("db_open", {
    "path": "/var/accumulate/bvn0/database"
})

# List all token accounts
accounts = use_mcp_tool("db_list_accounts", {
    "session_id": session["session_id"],
    "type": "tokenAccount",
    "count": 1000
})

# Export to CSV (in AI's memory)
for account in accounts["accounts"]:
    details = use_mcp_tool("db_query_account", {
        "session_id": session["session_id"],
        "url": account["url"]
    })
    print(f"{account['url']},{details['account']['balance']}")
```

## Security Considerations

### Read-Only by Default
- All database opens default to read-only
- Write operations require explicit configuration
- Snapshot export disabled by default

### Path Sandboxing
- Restrict accessible database paths
- Validate all file paths
- Prevent directory traversal attacks

### Resource Limits
- Maximum concurrent sessions: 10
- Session timeout: 1 hour
- Max results per query: 1000
- Memory limits for large queries

### Audit Logging
- Log all database opens
- Log all export operations
- Log failed access attempts

## Complete Tool List

| Category | Tool Name | Description |
|----------|-----------|-------------|
| **Database** | db_open | Open database/snapshot |
| **Database** | db_close | Close session |
| **Database** | db_info | Get database info |
| **Accounts** | db_query_account | Query account state |
| **Accounts** | db_list_accounts | List all accounts |
| **Accounts** | db_get_account_hash | Get BPT hash |
| **Chains** | db_query_chain | Query chain entries |
| **Chains** | db_query_chain_entry | Query specific entry |
| **Chains** | db_get_chain_anchor | Get chain anchor |
| **Transactions** | db_query_transaction | Query transaction |
| **Transactions** | db_query_transaction_status | Query tx status |
| **Transactions** | db_list_transactions | List transactions |
| **BPT** | db_bpt_get_root | Get BPT root |
| **BPT** | db_bpt_get_proof | Generate Merkle proof |
| **BPT** | db_bpt_verify_proof | Verify proof |
| **BPT** | db_bpt_iterate | Iterate BPT entries |
| **Data** | db_query_data_entries | Query data entries |
| **Snapshots** | db_snapshot_info | Get snapshot info |
| **Snapshots** | db_snapshot_export | Export to snapshot |
| **Analysis** | db_analyze_accounts | Analyze accounts |
| **Analysis** | db_analyze_chains | Analyze chain health |
| **Analysis** | db_get_statistics | Get statistics |
| **Advanced** | db_raw_get | Raw key-value get |
| **Advanced** | db_raw_iterate | Raw key-value iterate |

**Total Tools:** 24

## Comparison with API MCP Server

| Feature | API MCP | Database MCP |
|---------|---------|--------------|
| **Total Tools** | 40 | 24 |
| **Data Source** | Live node | Database files |
| **Wallet Support** | ✅ Yes | ❌ No |
| **Transaction Submit** | ✅ Yes | ❌ Read-only |
| **Historical Data** | Limited | ✅ Full |
| **Merkle Proofs** | Limited | ✅ Full |
| **Bulk Operations** | Limited | ✅ Optimized |
| **Network Required** | ✅ Yes | ❌ No |
| **Use Case** | Live operations | Analysis & debugging |

## Integration with API MCP Server

Both servers can coexist:
- **API MCP:** For live blockchain interaction
- **Database MCP:** For historical analysis and debugging

**Recommended Deployment:**
```json
{
  "mcpServers": {
    "accumulate-api": {
      "command": "/path/to/mcp-accumulate"
    },
    "accumulate-db": {
      "command": "/path/to/mcp-accumulate-db",
      "env": {
        "DB_READ_ONLY": "true",
        "ALLOWED_PATHS": "/var/accumulate,/snapshots"
      }
    }
  }
}
```

## Future Enhancements

1. **Database Comparison Tools**
   - Compare two databases
   - Detect differences
   - Validate migrations

2. **Advanced Analytics**
   - Token flow analysis
   - Account activity heatmaps
   - Transaction pattern detection

3. **Data Export Formats**
   - CSV export
   - JSON export
   - SQL dump

4. **Database Repair Tools**
   - Integrity checks
   - Corruption detection
   - Repair suggestions

## References

- [Database Implementation Guide](./accumulate_db_guide.md)
- [Database Resources Reference](./database_resources.md)
- [Database Summary](./database-summary.md)
- Main database code: `pkg/database/`
- BPT implementation: `pkg/database/bpt/`
- Snapshot format: `pkg/database/snapshot/`

## Version History

- **v1.0** (2025-10-20): Initial database MCP design
  - 24 tools for database queries
  - 5 resource types
  - Read-only focus
  - Merkle proof support
  - Snapshot support
  - Bulk operations

---

**Status:** Design Complete - Ready for Implementation
