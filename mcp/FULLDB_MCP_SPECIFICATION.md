# Fulldb MCP Integration Specification

## Problem Statement

The fulldb project needs to extract historical data from Accumulate backup databases via the Accumulate MCP server. Current limitations:

1. **Stdio-only protocol**: MCP server runs on stdio, making it incompatible with standard HTTP/REST architectures
2. **Missing HTTP mode**: No way to run MCP server as HTTP endpoint for programmatic access
3. **Limited extraction tools**: Need comprehensive tools for bulk data extraction

## Requirements

### 1. HTTP Server Mode

Add ability to run Accumulate MCP server as HTTP endpoint in addition to stdio mode.

**Use case:** Fulldb extractor needs to connect to Accumulate MCP over HTTP to extract data from backup databases.

**Proposed Implementation:**
```go
// New flag in main.go
--http-port=3000    // Start HTTP server on port 3000 instead of stdio
--http-host=localhost // Bind to specific host (default: localhost)
```

**HTTP Protocol:**
```
POST /mcp HTTP/1.1
Content-Type: application/json

{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "accumulate_db_list_accounts",
    "arguments": {
      "database": "2024-03-31-dn-historical",
      "limit": 1000
    }
  }
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [{
      "type": "text",
      "text": "{\"accounts\": [...], \"count\": 1000}"
    }]
  }
}
```

### 2. Enhanced Database Tools

#### 2.1 Batch Account Extraction

**Tool Name:** `accumulate_db_extract_accounts_batch`

**Purpose:** Extract multiple accounts efficiently in a single call

**Arguments:**
- `database` (string): Source database name
- `accounts` ([]string): Array of account URLs to extract
- `include_chains` (bool): Include chain data (default: false)
- `include_transactions` (bool): Include transactions (default: false)

**Returns:**
```json
{
  "accounts": [
    {
      "url": "acc://dn.acme/ledger/1",
      "type": "tokenAccount",
      "data": {...},
      "chains": [...],  // if include_chains=true
      "transactions": [...] // if include_transactions=true
    }
  ],
  "extracted_count": 100,
  "failed": []
}
```

#### 2.2 Database Iterator

**Tool Name:** `accumulate_db_iterate_accounts`

**Purpose:** Paginated iteration over all accounts in a database

**Arguments:**
- `database` (string): Source database name
- `cursor` (string, optional): Pagination cursor from previous call
- `page_size` (int): Number of accounts per page (default: 100, max: 1000)

**Returns:**
```json
{
  "accounts": ["acc://...", "acc://..."],
  "next_cursor": "base64-encoded-cursor",
  "has_more": true,
  "total_processed": 1500
}
```

#### 2.3 BPT Hash Calculation

**Tool Name:** `accumulate_db_get_bpt_hash`

**Purpose:** Get the BPT (Binary Patricia Tree) root hash for verification

**Arguments:**
- `database` (string): Source database name

**Returns:**
```json
{
  "hash": "deadbeef1234567890abcdef...",
  "height": 1234567,
  "timestamp": "2024-03-31T23:59:59Z"
}
```

### 3. Chain and Transaction Extraction

#### 3.1 Chain Extraction

**Tool Name:** `accumulate_db_get_chains`

**Arguments:**
- `database` (string): Source database name
- `account` (string): Account URL
- `chain_names` ([]string, optional): Specific chains to fetch

**Returns:**
```json
{
  "account": "acc://dn.acme/ledger/1",
  "chains": [
    {
      "name": "main",
      "height": 1500,
      "entries": [
        {"index": 0, "hash": "...", "data": {...}},
        {"index": 1, "hash": "...", "data": {...}}
      ]
    }
  ]
}
```

#### 3.2 Transaction Extraction

**Tool Name:** `accumulate_db_get_transactions`

**Arguments:**
- `database` (string): Source database name
- `account` (string): Account URL
- `start_index` (int, optional): Start from this transaction index
- `limit` (int): Max transactions to return

**Returns:**
```json
{
  "account": "acc://dn.acme/ledger/1",
  "transactions": [
    {
      "hash": "tx-hash-123",
      "type": "acme",
      "timestamp": 1234567890,
      "data": {...}
    }
  ],
  "total": 5000,
  "has_more": true
}
```

### 4. Progress Tracking

#### 4.1 Extraction Progress

**Tool Name:** `accumulate_db_extraction_progress`

**Purpose:** Track progress of long-running extractions

**Arguments:**
- `session_id` (string): Extraction session identifier

**Returns:**
```json
{
  "session_id": "abc-123",
  "database": "2024-03-31-dn-historical",
  "status": "in_progress",
  "accounts_processed": 15000,
  "accounts_total": 50000,
  "bytes_extracted": 5368709120,
  "start_time": "2024-11-01T15:00:00Z",
  "estimated_completion": "2024-11-01T17:30:00Z"
}
```

## Implementation Priorities

### Phase 1: HTTP Mode (Critical)
1. Add HTTP server mode flag
2. Implement JSON-RPC handler
3. Test with existing tools
4. Document HTTP API

### Phase 2: Enhanced Extraction Tools
1. Implement `accumulate_db_iterate_accounts`
2. Implement `accumulate_db_get_bpt_hash`
3. Test against real backup databases

### Phase 3: Batch Operations
1. Implement `accumulate_db_extract_accounts_batch`
2. Implement `accumulate_db_get_chains`
3. Implement `accumulate_db_get_transactions`

### Phase 4: Progress Tracking
1. Implement `accumulate_db_extraction_progress`
2. Add session management
3. Add resume capability

## Testing Requirements

### Integration Tests

**File:** `mcp/server/tools_fulldb_integration_test.go`

```go
// +build integration

package server

import (
    "net/http/httptest"
    "testing"
)

func TestHTTPMode_ListAccounts(t *testing.T) {
    // Test HTTP server mode with real database
    server := NewServer()
    httpServer := server.StartHTTP(":0") // Random port
    defer httpServer.Close()

    // Make HTTP request
    resp := makeHTTPRequest(httpServer.URL, "accumulate_db_list_accounts", ...)

    // Verify response
    require.NoError(t, resp.Error)
    require.NotEmpty(t, resp.Result)
}

func TestBatchExtraction(t *testing.T) {
    // Test extracting multiple accounts in one call
}

func TestPagination(t *testing.T) {
    // Test paginated iteration over large databases
}
```

## Configuration

### Environment Variables

```bash
# HTTP mode configuration
ACCUMULATE_MCP_HTTP_PORT=3000
ACCUMULATE_MCP_HTTP_HOST=localhost
ACCUMULATE_MCP_HTTP_CORS=true

# Performance tuning
ACCUMULATE_MCP_MAX_BATCH_SIZE=1000
ACCUMULATE_MCP_PAGE_SIZE=100
ACCUMULATE_MCP_TIMEOUT=30s
```

### Config File

**File:** `~/.accumulate/mcp-config.yaml`

```yaml
server:
  mode: http  # or "stdio" (default)
  http:
    host: localhost
    port: 3000
    cors: true
    max_request_size: 10MB

database:
  paths:
    - /media/paul/Expansion/staking-dbs-backup/dn
    - /media/paul/Expansion/staking-dbs-backup/bvn1

extraction:
  batch_size: 500
  page_size: 100
  timeout: 300s

logging:
  level: info
  format: json
```

## Security Considerations

1. **Local-only by default**: HTTP mode binds to localhost only
2. **Authentication**: Optional API key for remote access
3. **Rate limiting**: Prevent abuse of bulk extraction
4. **Read-only**: All extraction tools are read-only operations

## Backward Compatibility

- Default behavior remains stdio mode
- Existing tools continue to work unchanged
- HTTP mode is opt-in via flag
- No breaking changes to existing API

## Success Criteria

1. ✅ HTTP mode works alongside stdio mode
2. ✅ Can extract full database via HTTP API
3. ✅ BPT hash verification passes
4. ✅ Pagination handles 50,000+ accounts
5. ✅ Integration tests pass against real backup databases
6. ✅ Documentation updated with HTTP examples

## Timeline

- **Week 1**: HTTP mode implementation + basic testing
- **Week 2**: Enhanced extraction tools + integration tests
- **Week 3**: Batch operations + performance optimization
- **Week 4**: Documentation + final testing

## Open Questions

1. Should HTTP mode support WebSocket for streaming large datasets?
2. Do we need authentication for local HTTP server?
3. Should we support parallel extraction from multiple databases?
4. What's the maximum safe batch size for account extraction?

## References

- MCP Specification: https://spec.modelcontextprotocol.io/
- Accumulate Protocol: https://docs.accumulatenetwork.io/
- BadgerDB Documentation: https://dgraph.io/docs/badger/
