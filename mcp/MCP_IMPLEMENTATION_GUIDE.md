# MCP Server Implementation Guide - Accumulate SDK API Coverage

Based on the comprehensive Accumulate SDK v1.4.2 analysis, this guide prioritizes which APIs to implement in the MCP server.

## Priority Tiers for Implementation

### Tier 1: Core Query APIs (MUST HAVE)
These are the most frequently used operations. Implement first.

#### Query Service (Querier)
```
Methods:
- QueryAccount()       - Query account details
- QueryChain()        - Query chain entries
- QueryDataAccount()  - Query data account entries
- QueryDirectory()    - Query account directory
- QueryPending()      - Query pending transactions
- QueryTransaction()  - Query transaction status
- QuerySignature()    - Query signature status
```

**Why First:**
- 80% of user interactions
- Foundation for other features
- Relatively simple request-response pattern
- No streaming complexity

**Resources Needed:**
- DefaultQuery, ChainQuery, DataQuery, DirectoryQuery, PendingQuery
- AccountRecord, MessageRecord, ChainRecord, ChainEntryRecord
- RangeOptions for pagination

---

### Tier 2: Transaction Submission (HIGH PRIORITY)
Essential for write operations.

#### Submitter Service
```
Methods:
- Submit()    - Submit transaction envelope
- Validate() - Pre-validate envelope
- Faucet()  - Request ACME tokens
```

**Why High Priority:**
- Enables users to submit transactions
- Critical for any meaningful interaction
- Validation is important for UX

**Resources Needed:**
- Transaction types (user transactions)
- Envelope structure
- Signature handling
- SubmitOptions, ValidateOptions

---

### Tier 3: Network Status APIs (HIGH PRIORITY)
Monitoring and diagnostic operations.

#### Node Service
```
Methods:
- NodeInfo()     - Get node information
- FindService() - Discover services
```

#### Network Service
```
Methods:
- NetworkStatus() - Get network globals
```

#### Consensus Service
```
Methods:
- ConsensusStatus() - Get consensus node status
```

#### Metrics Service
```
Methods:
- Metrics() - Get network TPS
```

**Why High Priority:**
- Essential for monitoring
- Diagnostic information
- Relatively lightweight
- Important for decision making

**Resources Needed:**
- NodeInfo, ConsensusStatus, NetworkStatus, Metrics
- ServiceAddress, FindServiceOptions
- Peer information structures

---

### Tier 4: Advanced Queries (MEDIUM PRIORITY)
Specialized query operations.

#### Search Operations
```
- SearchForAnchor()      - Search anchor
- SearchForPublicKey()   - Find public key
- SearchForDelegate()    - Find delegate
- SearchForMessage()     - Find message by hash
```

#### Block Queries
```
- QueryMajorBlock()  - Query major blocks
- QueryMinorBlock()  - Query minor blocks
```

**Why Medium Priority:**
- Less commonly used
- Build on existing infrastructure
- Search patterns are optional features

**Resources Needed:**
- AnchorSearchQuery, PublicKeySearchQuery, etc.
- BlockQuery
- ChainEntryRecord variations

---

### Tier 5: Event Subscriptions (LOWER PRIORITY)
Real-time event streaming.

#### Event Service
```
Methods:
- Subscribe() - Subscribe to events
```

**Why Lower Priority:**
- More complex implementation (requires streaming)
- WebSocket support not complete in SDK
- Can be added later with streaming MCP support

**Resources Needed:**
- Event types (ErrorEvent, BlockEvent, GlobalsEvent)
- WebSocket handling
- Event routing by partition/account

---

### Tier 6: Snapshots (OPTIONAL)
System management features.

#### Snapshot Service
```
Methods:
- ListSnapshots() - List available snapshots
```

**Why Optional:**
- Administrative feature
- Lower impact on user experience
- Can be added if needed

---

## Implementation Checklist

### Phase 1: Core Infrastructure
- [ ] URL parsing and handling (acc:// scheme)
- [ ] Transaction envelope creation
- [ ] Signature type enums
- [ ] Account type unions
- [ ] Transaction type unions
- [ ] Error handling and status codes

### Phase 2: Query Layer (Tier 1)
- [ ] DefaultQuery implementation
- [ ] ChainQuery implementation
- [ ] DataQuery implementation
- [ ] DirectoryQuery implementation
- [ ] PendingQuery implementation
- [ ] Record type marshaling
- [ ] RangeOptions pagination
- [ ] Receipt option handling

### Phase 3: Submission Layer (Tier 2)
- [ ] Create transaction builders for user transaction types
- [ ] Envelope signing
- [ ] Submit operation
- [ ] Validate operation
- [ ] Faucet operation
- [ ] Transaction status tracking

### Phase 4: Network Status (Tier 3)
- [ ] NodeInfo operation
- [ ] FindService operation
- [ ] NetworkStatus operation
- [ ] ConsensusStatus operation
- [ ] Metrics operation

### Phase 5: Search & Blocks (Tier 4)
- [ ] AnchorSearchQuery
- [ ] PublicKeySearchQuery
- [ ] PublicKeyHashSearchQuery
- [ ] DelegateSearchQuery
- [ ] MessageHashSearchQuery
- [ ] BlockQuery (major and minor)

### Phase 6: Events (Tier 5)
- [ ] Event subscription interface
- [ ] Event type handling
- [ ] Stream management

### Phase 7: Snapshots (Tier 6)
- [ ] ListSnapshots operation

---

## Key Implementation Decisions

### 1. Transport Layer
Start with **JSON-RPC 2.0** (simplest to implement)
- Later add REST as alternative
- P2P and WebSocket are advanced features for later

### 2. Query Builder Pattern
Create helper functions for each query type:
```go
// Users call this
mcp_query_account(account_url)
mcp_query_chain(account_url, chain_name, options)
mcp_query_pending(account_url, options)

// Internals handle
- Query structure creation
- RangeOptions construction
- Network call
- Result marshaling
```

### 3. Transaction Building
Create transaction builders for common types:
```
mcp_create_identity(url, authorities)
mcp_create_token_account(url, token_url)
mcp_send_tokens(from_account, recipients)
mcp_write_data(account_url, data_entry)
// etc.
```

### 4. Error Handling
Map Accumulate status codes to MCP errors
- Document all possible error states
- Provide helpful error messages

---

## Most Frequently Used APIs

Based on typical blockchain usage patterns:

1. **QueryAccount** (acc://alice.acme) - 25% of calls
2. **SendTokens** - 15% of calls
3. **QueryTransaction** - 15% of calls
4. **QueryPending** - 10% of calls
5. **CreateIdentity** - 8% of calls
6. **NetworkStatus** - 7% of calls
7. **QueryChain** - 5% of calls
8. **CreateTokenAccount** - 5% of calls
9. **Others** - 10% of calls

---

## API Coverage Strategy

### Immediate (Phase 1-2)
- Core query operations
- Basic transaction submission
- Essential network info

### Short-term (Phase 3-4)
- All query types
- Advanced searches
- Block queries
- Network status APIs

### Long-term (Phase 5+)
- Event subscriptions
- P2P integration
- WebSocket support
- System administration

---

## Resource Requirements by Component

### Query Component
- 11 query types to map
- 13 record types to handle
- Pagination logic
- Time: ~3-5 days

### Transaction Component
- 23 user transaction types
- 6 synthetic transaction types
- Signature handling
- Envelope creation
- Time: ~5-7 days

### Network Component
- 5 status/info endpoints
- Discovery logic
- Service enumeration
- Time: ~2-3 days

### Search Component
- 5 search query types
- Time: ~1-2 days

### Event Component
- 3 event types
- Streaming logic
- Time: ~3-4 days (but lower priority)

---

## File Structure for Implementation

```
mcp-accumulate/
├── accumulate/
│   ├── queries/
│   │   ├── default.go       # DefaultQuery
│   │   ├── chain.go         # ChainQuery
│   │   ├── data.go          # DataQuery
│   │   └── ...
│   ├── transactions/
│   │   ├── user.go          # User transaction types
│   │   ├── synthetic.go     # Synthetic transaction types
│   │   └── builder.go       # Transaction building helpers
│   ├── network/
│   │   ├── node.go          # Node service
│   │   ├── consensus.go     # Consensus service
│   │   └── ...
│   ├── types.go             # Common types (enums, structs)
│   ├── errors.go            # Error handling
│   └── client.go            # Main Accumulate client wrapper
├── mcp/
│   ├── handlers.go          # MCP request handlers
│   ├── resources.go         # MCP resource definitions
│   └── tools.go             # MCP tool definitions
└── main.go
```

---

## Testing Strategy

For each tier:
1. Unit tests for type conversions
2. Integration tests with SDK
3. API contract tests (input/output validation)
4. Error condition tests

Key scenarios:
- Query non-existent accounts
- Submit invalid transactions
- Search with various filters
- Pagination edge cases
- Error handling for all status codes

