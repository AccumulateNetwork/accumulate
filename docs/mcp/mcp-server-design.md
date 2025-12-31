# Accumulate MCP Server Design Specification

## Overview

This document specifies the design of a Model Context Protocol (MCP) server for the Accumulate blockchain network. The MCP server exposes Accumulate's full API surface through standardized MCP primitives (Tools, Resources, and Prompts) to enable AI assistants to interact with Accumulate networks.

## MCP Fundamentals

The Model Context Protocol defines three primary primitives:

1. **Tools**: Functions that can be invoked by the AI to perform actions
2. **Resources**: Data sources that can be read by the AI
3. **Prompts**: Pre-configured workflows and templates (optional)

## Architecture Overview

```
┌─────────────────────────────────────────────────────┐
│              AI Assistant (Claude, etc.)            │
└─────────────────┬───────────────────────────────────┘
                  │ MCP Protocol
┌─────────────────▼───────────────────────────────────┐
│           Accumulate MCP Server                     │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────┐ │
│  │  MCP Tools   │  │ MCP Resources│  │   Prompts │ │
│  └──────┬───────┘  └──────┬───────┘  └─────┬─────┘ │
└─────────┼──────────────────┼─────────────────┼──────┘
          │                  │                 │
┌─────────▼──────────────────▼─────────────────▼──────┐
│         Accumulate API Client Layer                 │
│  ┌─────────────────┐      ┌──────────────────────┐ │
│  │  V3 API Client  │      │  V2 API Client       │ │
│  │  (Primary)      │      │  (Legacy Compat)     │ │
│  └────────┬────────┘      └──────────┬───────────┘ │
└───────────┼──────────────────────────┼──────────────┘
            │                          │
┌───────────▼──────────────────────────▼──────────────┐
│         Accumulate Network Node(s)                  │
│  JSON-RPC | REST | WebSocket | P2P                  │
└─────────────────────────────────────────────────────┘
```

## MCP Tools Specification

Tools represent invokable functions. Each tool maps to one or more Accumulate API endpoints.

### 1. Network & Node Information Tools

#### Tool: `accumulate_node_info`
**Description:** Get information about an Accumulate network node

**Parameters:**
- `peer_id` (string, optional): Specific peer ID to query

**Returns:**
- `peer_id`: Node's peer ID
- `network`: Network name (e.g., "MainNet", "TestNet")
- `services`: List of available services
- `version`: Software version
- `commit`: Git commit hash

**API Mapping:** V3 `node-info`

---

#### Tool: `accumulate_find_service`
**Description:** Find nodes providing a specific service

**Parameters:**
- `network` (string, optional): Network name
- `service_type` (enum, optional): Service type to find (Node, Consensus, Network, Metrics, Query, Event, Submit, Validate, Faucet, Snapshot)
- `known_only` (boolean, optional): Only return known peers
- `timeout` (duration, optional): DHT query timeout

**Returns:**
- Array of service locations with peer IDs, status, and addresses

**API Mapping:** V3 `find-service`

---

#### Tool: `accumulate_network_status`
**Description:** Get overall network health and partition information

**Parameters:**
- `partition` (string, optional): Specific partition to query

**Returns:**
- Network health metrics
- Partition statuses
- Oracle prices
- Routing tables

**API Mapping:** V3 `network-status`

---

#### Tool: `accumulate_consensus_status`
**Description:** Get consensus information and validator status

**Parameters:**
- `partition` (string, optional): Partition ID
- `node_id` (string, optional): Specific node ID

**Returns:**
- Consensus height
- Block hash
- Validator set
- Voting power distribution
- Node health status

**API Mapping:** V3 `consensus-status`

---

#### Tool: `accumulate_metrics`
**Description:** Get node performance metrics

**Parameters:**
- `metric_type` (enum, optional): Specific metric category

**Returns:**
- TPS (transactions per second)
- Block production rate
- API latency
- P2P connection stats
- Memory/CPU usage

**API Mapping:** V3 `metrics`

---

### 2. Query Tools (Account & Blockchain Data)

#### Tool: `accumulate_query_account`
**Description:** Query account information by URL

**Parameters:**
- `url` (string, required): Account URL (e.g., "acc://alice/tokens")
- `include_receipt` (boolean, optional): Include Merkle receipt
- `prove` (boolean, optional): Include cryptographic proof

**Returns:**
- Account data (varies by account type: ADI, Token Account, Data Account, Key Page, etc.)
- Account metadata
- Optional: Merkle receipt and proof

**API Mapping:** V3 `query` with `DefaultQuery`

**Supported Account Types:**
- Unknown Account
- Lite Identity
- Lite Token Account
- Lite Data Account
- ADI (Accumulate Digital Identifier)
- Token Account
- Token Issuer
- Data Account
- Key Book
- Key Page
- And 30+ other protocol account types

---

#### Tool: `accumulate_query_transaction`
**Description:** Query transaction by ID or hash

**Parameters:**
- `txid` (string, required): Transaction ID (hash or "acc://account/hash" format)
- `include_receipt` (boolean, optional): Include Merkle receipt
- `prove` (boolean, optional): Include cryptographic proof
- `wait` (duration, optional): Wait for transaction to complete

**Returns:**
- Transaction data
- Status (pending, delivered, failed, etc.)
- Result and error information
- Optional: Merkle receipt and proof

**API Mapping:** V3 `query` with `DefaultQuery` on txid:// scope

---

#### Tool: `accumulate_query_chain`
**Description:** Query entries from an account's chain

**Parameters:**
- `url` (string, required): Account URL
- `chain_name` (string, required): Chain name (e.g., "main", "signature", "pending")
- `start` (integer, optional): Start index (default: 0)
- `count` (integer, optional): Number of entries (default: 10, max: 100)
- `expand` (boolean, optional): Expand entry data

**Returns:**
- Array of chain entries
- Total chain length
- Entry data (hashes or full objects if expanded)

**API Mapping:** V3 `query` with `ChainQuery`

---

#### Tool: `accumulate_query_data`
**Description:** Query data entries from a data account

**Parameters:**
- `url` (string, required): Data account URL
- `entry_hash` (string, optional): Specific entry hash
- `start` (integer, optional): Start index
- `count` (integer, optional): Number of entries
- `expand` (boolean, optional): Expand entry data

**Returns:**
- Data entries
- Entry metadata

**API Mapping:** V3 `query` with `DataQuery`

---

#### Tool: `accumulate_query_directory`
**Description:** List accounts under an ADI or directory

**Parameters:**
- `url` (string, required): Directory URL
- `start` (integer, optional): Start index
- `count` (integer, optional): Number of entries (max: 100)
- `expand` (boolean, optional): Include full account data

**Returns:**
- Array of account URLs or full account objects
- Total count

**API Mapping:** V3 `query` with `DirectoryQuery`

---

#### Tool: `accumulate_search_accounts`
**Description:** Search for accounts by URL pattern or type

**Parameters:**
- `url` (string, required): URL pattern (supports wildcards)
- `type` (string, optional): Account type filter
- `count` (integer, optional): Max results (default: 10)
- `start` (integer, optional): Pagination offset

**Returns:**
- Matching account URLs and basic info

**API Mapping:** V3 `query` with `SearchQuery`

---

#### Tool: `accumulate_query_block`
**Description:** Query block information by height or hash

**Parameters:**
- `partition` (string, optional): Partition ID
- `height` (integer, optional): Block height
- `block_hash` (string, optional): Block hash
- `include_entries` (boolean, optional): Include block entries

**Returns:**
- Block data
- Block hash
- Timestamp
- Transaction count
- Optional: Full entries

**API Mapping:** V3 `query` with `BlockQuery`

---

#### Tool: `accumulate_query_anchors`
**Description:** Query anchor chains and cross-chain receipts

**Parameters:**
- `url` (string, required): Anchor chain URL
- `source_partition` (string, optional): Source partition
- `start` (integer, optional): Start index
- `count` (integer, optional): Number of anchors

**Returns:**
- Anchor chain entries
- Cross-partition receipts
- Root chain hashes

**API Mapping:** V3 `query` with `AnchorSearchQuery`

---

#### Tool: `accumulate_query_key_index`
**Description:** Find which key page contains a specific public key

**Parameters:**
- `url` (string, required): Key book URL
- `public_key` (string, required): Public key (hex or base64)
- `key_type` (enum, optional): Key type (ED25519, SECP256K1, RCD1, etc.)

**Returns:**
- Key page URL
- Key index within page
- Key metadata

**API Mapping:** V3 `query` with `PublicKeySearchQuery`

---

#### Tool: `accumulate_query_public_key_hash`
**Description:** Search for accounts by public key hash

**Parameters:**
- `public_key_hash` (string, required): Hash of public key

**Returns:**
- Account URLs associated with the key
- Key authorities

**API Mapping:** V3 `query` with `PublicKeyHashSearchQuery`

---

#### Tool: `accumulate_query_delegate`
**Description:** Find delegated authority relationships

**Parameters:**
- `url` (string, required): Account URL

**Returns:**
- Delegated authorities
- Authority relationships
- Delegation chain

**API Mapping:** V3 `query` with `DelegateSearchQuery`

---

#### Tool: `accumulate_query_pending`
**Description:** Query pending transactions for an account

**Parameters:**
- `url` (string, required): Account URL
- `start` (integer, optional): Start index
- `count` (integer, optional): Number of entries

**Returns:**
- Pending transactions
- Transaction statuses

**API Mapping:** V3 `query` with `PendingQuery`

---

### 3. Transaction Tools

#### Tool: `accumulate_submit_transaction`
**Description:** Submit a signed transaction envelope to the network

**Parameters:**
- `envelope` (object, required): Signed transaction envelope
  - Can be provided as:
    - JSON object
    - Hex-encoded bytes
    - Base64-encoded bytes
- `check_only` (boolean, optional): Validate without submitting

**Returns:**
- Transaction ID
- Transaction hash
- Status
- Error information (if any)

**API Mapping:** V3 `submit`

**Note:** This tool does NOT sign transactions. Signing must be done externally with private keys.

---

#### Tool: `accumulate_validate_transaction`
**Description:** Validate a transaction envelope without submitting

**Parameters:**
- `envelope` (object, required): Transaction envelope to validate

**Returns:**
- Validation result
- Error details (if invalid)
- Estimated fees

**API Mapping:** V3 `validate`

---

#### Tool: `accumulate_faucet`
**Description:** Request testnet tokens from faucet (testnet only)

**Parameters:**
- `url` (string, required): Token account URL to fund

**Returns:**
- Faucet transaction ID
- Amount dispensed

**API Mapping:** V3 `faucet`

---

### 4. Transaction Building Helpers

These tools help construct transaction payloads (but do NOT sign them):

#### Tool: `accumulate_build_send_tokens`
**Description:** Build a SendTokens transaction payload

**Parameters:**
- `from` (string, required): Source token account URL
- `to` (array, required): Array of recipients
  - `url` (string): Destination account URL
  - `amount` (string): Amount to send (in base units)
- `metadata` (object, optional): Transaction metadata

**Returns:**
- Unsigned transaction payload (JSON)
- Required signers
- Estimated fee

**API Mapping:** Helper using protocol types from `protocol/send_tokens.go`

---

#### Tool: `accumulate_build_create_account`
**Description:** Build transaction to create a new account

**Parameters:**
- `url` (string, required): New account URL
- `type` (string, required): Account type (TokenAccount, DataAccount, KeyPage, etc.)
- `authorities` (array, required): Authority URLs for the account
- `options` (object, optional): Type-specific options

**Returns:**
- Unsigned transaction payload

**API Mapping:** Uses protocol types (CreateIdentity, CreateTokenAccount, CreateDataAccount, etc.)

---

#### Tool: `accumulate_build_update_account`
**Description:** Build transaction to update account authorities or settings

**Parameters:**
- `url` (string, required): Account URL to update
- `operations` (array, required): Update operations
  - `add_authority`, `remove_authority`, `update_key`, etc.

**Returns:**
- Unsigned transaction payload

**API Mapping:** Uses UpdateAccountAuth, UpdateKeyPage, etc.

---

#### Tool: `accumulate_build_write_data`
**Description:** Build transaction to write data entries

**Parameters:**
- `url` (string, required): Data account URL
- `entries` (array, required): Data entries to write
  - `data` (string): Entry data (bytes, hex, or string)

**Returns:**
- Unsigned transaction payload

**API Mapping:** Uses protocol WriteData type

---

#### Tool: `accumulate_build_token_issuance`
**Description:** Build transaction to issue new tokens

**Parameters:**
- `url` (string, required): Token issuer URL
- `to` (string, required): Recipient account URL
- `amount` (string, required): Amount to issue

**Returns:**
- Unsigned transaction payload

**API Mapping:** Uses protocol IssueTokens type

---

#### Tool: `accumulate_build_burn_tokens`
**Description:** Build transaction to burn tokens

**Parameters:**
- `url` (string, required): Token account URL
- `amount` (string, required): Amount to burn

**Returns:**
- Unsigned transaction payload

**API Mapping:** Uses protocol BurnTokens type

---

### 5. Event Subscription Tools

#### Tool: `accumulate_subscribe_events`
**Description:** Subscribe to real-time blockchain events

**Parameters:**
- `account` (string, optional): Filter by account URL
- `event_types` (array, optional): Event types to subscribe to
  - `Block`, `Transaction`, `Error`
- `partition` (string, optional): Filter by partition

**Returns:**
- Subscription ID
- Event stream endpoint

**API Mapping:** V3 Event Service `subscribe`

**Note:** Requires WebSocket or streaming connection

---

### 6. Snapshot Tools (Private/Admin)

#### Tool: `accumulate_list_snapshots`
**Description:** List available network snapshots

**Parameters:**
- `partition` (string, optional): Partition ID
- `height` (integer, optional): Block height

**Returns:**
- Available snapshots
- Snapshot metadata

**API Mapping:** V3 Snapshot Service (if exposed)

---

## MCP Resources Specification

Resources represent data sources that can be read. They use URI templates for addressing.

### Resource: `account`
**URI Template:** `accumulate://account/{url}`

**Description:** Read account information

**Parameters:**
- `url`: Account URL (e.g., "alice/tokens")

**Returns:** Account data in JSON format

**Example URIs:**
- `accumulate://account/alice.acme/tokens`
- `accumulate://account/ACME`
- `accumulate://account/acc://alice.acme/book/1`

---

### Resource: `transaction`
**URI Template:** `accumulate://transaction/{txid}`

**Description:** Read transaction information

**Parameters:**
- `txid`: Transaction ID or hash

**Returns:** Transaction data

**Example URIs:**
- `accumulate://transaction/{hash}`

---

### Resource: `chain`
**URI Template:** `accumulate://chain/{url}/{chain_name}?start={start}&count={count}`

**Description:** Read chain entries

**Parameters:**
- `url`: Account URL
- `chain_name`: Chain name
- `start`: Start index (optional)
- `count`: Entry count (optional)

**Example URIs:**
- `accumulate://chain/alice.acme/tokens/main?start=0&count=10`
- `accumulate://chain/alice.acme/book/1/signature`

---

### Resource: `directory`
**URI Template:** `accumulate://directory/{url}?start={start}&count={count}`

**Description:** List directory contents

**Parameters:**
- `url`: Directory/ADI URL
- `start`, `count`: Pagination

**Example URIs:**
- `accumulate://directory/alice.acme`
- `accumulate://directory/acme`

---

### Resource: `block`
**URI Template:** `accumulate://block/{partition}/{height}`

**Description:** Read block data

**Parameters:**
- `partition`: Partition ID
- `height`: Block height

**Example URIs:**
- `accumulate://block/BVN0/1000000`
- `accumulate://block/Directory/7500000`

---

### Resource: `network`
**URI Template:** `accumulate://network/{network_name}`

**Description:** Read network status

**Parameters:**
- `network_name`: Network name (MainNet, TestNet, etc.)

**Returns:** Network health, partitions, oracle data

---

## MCP Prompts Specification

Prompts provide pre-configured workflows for common tasks.

### Prompt: `create_identity`
**Description:** Guide user through creating an Accumulate Digital Identifier (ADI)

**Steps:**
1. Choose ADI name
2. Choose initial key
3. Build CreateIdentity transaction
4. Guide signing process
5. Submit transaction
6. Monitor for confirmation

---

### Prompt: `send_tokens`
**Description:** Guide user through sending tokens

**Steps:**
1. Select source account
2. Specify recipients and amounts
3. Build SendTokens transaction
4. Guide signing
5. Submit and track

---

### Prompt: `inspect_account`
**Description:** Comprehensive account inspection workflow

**Steps:**
1. Query account data
2. Check recent transactions
3. View authorities
4. Display token balances
5. Show pending transactions

---

### Prompt: `debug_transaction`
**Description:** Debug a failed or pending transaction

**Steps:**
1. Query transaction status
2. Check error messages
3. Inspect signatures
4. Verify authorities
5. Suggest fixes

---

## Configuration

The MCP server requires configuration for:

### Connection Settings
```json
{
  "network": "MainNet",
  "endpoints": {
    "primary": "https://mainnet.accumulatenetwork.io/v3",
    "fallback": ["https://api.accumulate.defidevs.io/v3"]
  },
  "timeout": "30s",
  "retry": {
    "max_attempts": 3,
    "backoff": "exponential"
  }
}
```

### Feature Flags
```json
{
  "enable_v2_compat": true,
  "enable_transaction_building": true,
  "enable_faucet": false,
  "enable_snapshot_tools": false,
  "enable_event_subscriptions": true
}
```

### Security
```json
{
  "read_only": false,
  "allow_transaction_submission": true,
  "require_confirmation": true,
  "max_query_results": 1000
}
```

## Implementation Notes

### Language Choice
Recommended: **Go**
- Native Accumulate client libraries available
- Strong typing for protocol messages
- Excellent P2P support (libp2p)
- Can reuse existing codebase

Alternative: **TypeScript/JavaScript**
- Broader MCP ecosystem
- Easier web integration
- Would need to implement protocol types

### Dependencies
- **MCP SDK:** github.com/mark3labs/mcp-go (for Go implementation)
- **Accumulate SDK:** Internal packages from this repo
  - `pkg/api/v3` - V3 API client
  - `pkg/types/messaging` - Message types
  - `protocol` - Protocol types
  - `pkg/url` - URL handling

### Error Handling
All tools must return structured errors:
```json
{
  "code": "ACCOUNT_NOT_FOUND",
  "message": "Account acc://alice.acme/tokens not found",
  "details": {
    "url": "acc://alice.acme/tokens",
    "suggestion": "Check URL spelling or verify account exists"
  }
}
```

### Rate Limiting
- Implement client-side rate limiting
- Respect node API limits
- Cache frequent queries (node-info, network-status)
- TTL: 30s for node info, 5s for blockchain data

### Authentication
- No authentication required for public data queries
- Transaction submission requires valid signatures (but NOT handled by MCP server)
- Optional: API key support for private node endpoints

### Testing Strategy
1. Unit tests for each tool
2. Integration tests against devnet
3. Mock Accumulate API for offline testing
4. Tool validation tests (parameter validation)
5. Resource URI parsing tests

## Usage Examples

### Example 1: Check Account Balance
```python
# AI assistant uses MCP tool
result = use_mcp_tool("accumulate_query_account", {
    "url": "acc://alice.acme/tokens"
})

print(f"Balance: {result.balance} ACME")
print(f"Token URL: {result.tokenUrl}")
```

### Example 2: Browse Directory
```python
# AI assistant uses MCP resource
accounts = read_mcp_resource("accumulate://directory/alice.acme")

for account in accounts:
    print(f"- {account.url}")
```

### Example 3: Build and Submit Transaction
```python
# Step 1: Build transaction (using tool)
tx_payload = use_mcp_tool("accumulate_build_send_tokens", {
    "from": "acc://alice.acme/tokens",
    "to": [{"url": "acc://bob.acme/tokens", "amount": "1000000000"}]
})

# Step 2: Sign externally (NOT in MCP)
signed = sign_with_private_key(tx_payload, private_key)

# Step 3: Submit (using tool)
result = use_mcp_tool("accumulate_submit_transaction", {
    "envelope": signed
})

print(f"Transaction submitted: {result.txid}")
```

## Security Considerations

### Key Management
- **CRITICAL:** MCP server MUST NOT handle private keys
- Transaction signing must be done externally
- MCP server only builds unsigned payloads and submits signed envelopes
- Recommend integration with hardware wallets or secure key management systems

### Read-Only Mode
For maximum security, MCP server can run in read-only mode:
- Disable all transaction submission tools
- Only expose query and informational tools
- Suitable for blockchain exploration and analysis

### Transaction Confirmation
Before submitting transactions:
1. Display transaction details to user
2. Show estimated fees
3. Require explicit confirmation
4. Log all submitted transactions

## Future Enhancements

### Phase 2 Features
1. **Advanced Analytics Tools**
   - Historical data queries
   - Token holder analysis
   - Transaction flow visualization

2. **Multi-Network Support**
   - Automatic network detection
   - Cross-network queries
   - Network comparison tools

3. **Smart Caching**
   - Local account cache
   - Block cache for historical queries
   - Intelligent cache invalidation

4. **Batch Operations**
   - Batch account queries
   - Multi-transaction submission
   - Bulk data exports

5. **Enhanced Event Subscriptions**
   - Custom event filters
   - Event aggregation
   - Alert triggers

### Phase 3 Features
1. **GraphQL-style Queries**
   - Nested data fetching
   - Field selection
   - Query optimization

2. **Development Tools**
   - Local devnet management
   - Test data generation
   - Contract simulation (if/when supported)

3. **Documentation Generation**
   - Auto-generate API docs from tools
   - Interactive examples
   - Code snippets in multiple languages

## Appendix A: Complete Tool List

| Category | Tool Name | Primary API |
|----------|-----------|-------------|
| **Network** | accumulate_node_info | V3 node-info |
| **Network** | accumulate_find_service | V3 find-service |
| **Network** | accumulate_network_status | V3 network-status |
| **Network** | accumulate_consensus_status | V3 consensus-status |
| **Network** | accumulate_metrics | V3 metrics |
| **Query** | accumulate_query_account | V3 query/DefaultQuery |
| **Query** | accumulate_query_transaction | V3 query/DefaultQuery |
| **Query** | accumulate_query_chain | V3 query/ChainQuery |
| **Query** | accumulate_query_data | V3 query/DataQuery |
| **Query** | accumulate_query_directory | V3 query/DirectoryQuery |
| **Query** | accumulate_search_accounts | V3 query/SearchQuery |
| **Query** | accumulate_query_block | V3 query/BlockQuery |
| **Query** | accumulate_query_anchors | V3 query/AnchorSearchQuery |
| **Query** | accumulate_query_key_index | V3 query/PublicKeySearchQuery |
| **Query** | accumulate_query_public_key_hash | V3 query/PublicKeyHashSearchQuery |
| **Query** | accumulate_query_delegate | V3 query/DelegateSearchQuery |
| **Query** | accumulate_query_pending | V3 query/PendingQuery |
| **Transaction** | accumulate_submit_transaction | V3 submit |
| **Transaction** | accumulate_validate_transaction | V3 validate |
| **Transaction** | accumulate_faucet | V3 faucet |
| **Builder** | accumulate_build_send_tokens | Protocol types |
| **Builder** | accumulate_build_create_account | Protocol types |
| **Builder** | accumulate_build_update_account | Protocol types |
| **Builder** | accumulate_build_write_data | Protocol types |
| **Builder** | accumulate_build_token_issuance | Protocol types |
| **Builder** | accumulate_build_burn_tokens | Protocol types |
| **Events** | accumulate_subscribe_events | V3 events |
| **Snapshots** | accumulate_list_snapshots | V3 snapshots |

**Total Tools:** 28

## Appendix B: V2 API Compatibility

For backward compatibility, the MCP server can optionally expose V2 API methods as tools:

- `accumulate_v2_query` - Generic V2 query
- `accumulate_v2_query_tx` - V2 transaction query
- `accumulate_v2_query_chain` - V2 chain query
- `accumulate_v2_query_data` - V2 data query
- And 35+ other V2 methods

These tools provide a migration path for existing V2 API users.

## Appendix C: Ethereum RPC Compatibility

The MCP server can expose Ethereum-compatible RPC methods for cross-chain tooling:

- `eth_blockNumber`
- `eth_getBalance`
- `eth_getTransactionByHash`
- `eth_getTransactionReceipt`
- `eth_sendRawTransaction`
- `eth_call`
- `eth_estimateGas`

These map to Accumulate equivalents where possible.

## References

- [MCP Specification](https://spec.modelcontextprotocol.io/)
- [MCP Go SDK](https://github.com/mark3labs/mcp-go)
- Accumulate API Documentation (see: `accumulate_api_summary.md`)
- Accumulate Protocol Documentation
- Accumulate Network: https://accumulatenetwork.io

## Version History

- **v1.0** (2025-10-20): Initial design specification
  - 28 core tools defined
  - 6 resource types
  - 4 workflow prompts
  - Full V3 API coverage
  - Optional V2 compatibility layer
