# Accumulate Network API Documentation - Complete Index

This comprehensive documentation provides complete coverage of all Accumulate Network API versions and their capabilities.

## Documentation Files

### 1. **Main Comprehensive Guide**
   - **File:** `accumulate_api_summary.md`
   - **Contents:** 
     - Executive summary
     - V3 and V2 API version comparison
     - Detailed endpoint reference for all 12 services
     - Service type enumeration (10 types)
     - Query types reference (11 types)
     - Record types reference (13 types)
     - Event types (3 types)
     - Ethereum RPC API methods
     - Transport protocol implementations
     - Shared data structures
     - Error handling and enumerations
     - Usage patterns and best practices
   - **Best For:** Complete API understanding, comprehensive reference

### 2. **Quick Reference Guide**
   - **File:** `accumulate_api_quick_reference.md`
   - **Contents:**
     - API endpoint summary
     - Quick JSON-RPC examples
     - Query types lookup table
     - Record types lookup table
     - Common options reference
     - HTTP status codes
     - Event types table
     - V2 API method list
     - Ethereum RPC methods
     - Service types table
     - Common patterns and error handling
     - Best practices checklist
   - **Best For:** Quick lookups, copy-paste examples, quick implementation

### 3. **Source Code File Reference**
   - **File:** `accumulate_api_file_reference.md`
   - **Contents:**
     - Complete directory structure
     - File purpose mappings
     - V3 definition files (YAML)
     - V3 service implementations
     - V3 generated code locations
     - V2 definition files (YAML)
     - V2 implementation files
     - Private API files
     - Ethereum API files
     - Code generation configuration
     - Related packages reference
     - Testing and mocks
     - Integration points
     - Build/generation commands
     - Size statistics
   - **Best For:** Code navigation, finding specific files, understanding architecture

---

## API Versions Coverage

### V3 API (Modern - Recommended)
**Status:** Primary, actively maintained

**Components Documented:**
1. Node Service - Service discovery and information
2. Consensus Service - Validator consensus status
3. Network Service - Network configuration
4. Snapshot Service - Snapshot management
5. Metrics Service - Performance metrics
6. Query Service - Account/transaction queries
7. Submit Service - Transaction submission
8. Validate Service - Transaction validation
9. Faucet Service - Token requests
10. Event Service - Event subscriptions
11. Private Service - Internal sequencing

**Transports Documented:**
- JSON-RPC 2.0 (HTTP)
- REST (HTTP)
- WebSocket (WS/WSS)
- Binary Message Protocol
- P2P Network (libp2p)
- Ethereum RPC Compatibility

### V2 API (Legacy - Compatibility)
**Status:** Maintained for backward compatibility

**Methods Documented:** 40+ JSON-RPC methods
- Status and metadata queries
- Transaction queries
- Account queries
- Transaction execution (generic and specific types)
- Data chain operations
- Key lookups
- Block queries

---

## API Capabilities Summary

### Query Capabilities
- **Query Types:** 11 distinct query types
- **Record Types:** 13 response record types
- **Search:** Full-text search by anchor, key, delegate, message hash
- **Pagination:** Range queries with offset/limit
- **Expansion:** Nested record expansion
- **Receipts:** Merkle proof inclusion

### Transaction Capabilities
- **Submission:** Full transaction envelope submission
- **Validation:** Pre-submission validation
- **Status Tracking:** Transaction state polling
- **Wait-for-acceptance:** Optional blocking submission
- **Faucet:** Token minting requests

### Network Capabilities
- **Service Discovery:** DHT-based peer discovery
- **Peer Information:** Node information and service registration
- **Network Status:** Global configuration and state
- **Metrics:** Real-time performance metrics (TPS)
- **Event Streaming:** Real-time block and global event notification

### Data Types
- **Account Types:** ADI, Data Account, Key Book, Token Account, etc.
- **Chain Types:** Main, Signature, Index, Data chains
- **Transaction Types:** 18+ transaction types
- **Message Types:** 50+ message types
- **Block Types:** Minor and Major blocks with merkle trees

---

## Usage Patterns

### Pattern 1: Query Account State
1. Use `query` service with `DefaultQuery`
2. Specify account URL in scope
3. Receive `AccountRecord` with account details

### Pattern 2: Retrieve Transaction History
1. Use `query` service with `ChainQuery`
2. Specify chain name (main, signature, index, data)
3. Use `RangeOptions` for pagination
4. Receive `RecordRange[ChainEntryRecord[...]]`

### Pattern 3: Submit Transaction
1. Create `messaging.Envelope` with transaction + signatures
2. Use `submit` service
3. Optional: wait for acceptance
4. Receive `Submission` array with status

### Pattern 4: Monitor Events
1. Use `subscribe` service with optional partition/account filter
2. Stream receives `Event` messages as they occur
3. Types: ErrorEvent, BlockEvent, GlobalsEvent

### Pattern 5: Discover Services
1. Use `find-service` with network name
2. Optional: filter by service type and partition
3. Receive array of `FindServiceResult` with peer addresses

---

## Key Data Structures

### Request Options
- **RangeOptions:** Pagination (start, count, expand, fromEnd)
- **ReceiptOptions:** Merkle proofs (forAny, forHeight)
- **SubmitOptions:** Submission control (verify, wait)
- **ValidateOptions:** Validation detail (full)

### Response Records (11 Types)
- **AccountRecord:** Account + directory + pending
- **ChainRecord:** Chain metadata
- **ChainEntryRecord[T]:** Entry + proof + value
- **MessageRecord[T]:** Transaction + status + result
- **BlockRecords:** Minor and Major block data
- **RecordRange[T]:** Paginated results wrapper
- **KeyRecord:** Key specification
- **UrlRecord, TxIDRecord, IndexEntryRecord, ErrorRecord:** Value wrappers

### Enumerations
- **ServiceType:** 10 service types (0-10, with private 0xF001)
- **QueryType:** 11 query types
- **RecordType:** 13 record types
- **EventType:** 3 event types
- **KnownPeerStatus:** 3 peer status values

---

## Transport Protocol Comparison

| Aspect | JSON-RPC | REST | WebSocket | P2P | Ethereum |
|--------|----------|------|-----------|-----|----------|
| **Protocol** | HTTP POST | HTTP GET/POST | WS/WSS | libp2p | HTTP POST |
| **Efficiency** | Good | Moderate | Excellent | Excellent | Good |
| **Real-time** | Polling | Polling | Native | Native | Polling |
| **Latency** | 100-500ms | 100-500ms | 10-100ms | 10-100ms | 100-500ms |
| **Services** | All except Event | All except Event | All | All | Limited |
| **Best For** | General queries | Simple calls | Events + streams | P2P networks | Ethereum tools |

---

## Integration Checklist

### For Implementing a Client
- [ ] Choose transport (JSON-RPC, REST, WebSocket, or P2P)
- [ ] Implement error handling for -33XXX error codes
- [ ] Handle optional fields and pointer types
- [ ] Implement retry logic with exponential backoff
- [ ] Support request timeouts (recommended 30s default)
- [ ] Parse union types for Record responses
- [ ] Handle event subscriptions if needed
- [ ] Implement service discovery (find-service)
- [ ] Cache service addresses for performance
- [ ] Monitor version/commit fields

### For Implementing a Service
- [ ] Choose which services to implement
- [ ] Create service struct implementing required interface(s)
- [ ] Register with appropriate handler(s)
- [ ] Validate all input parameters
- [ ] Handle context cancellation properly
- [ ] Return proper error responses
- [ ] Support concurrent requests
- [ ] Implement rate limiting if needed
- [ ] Log errors and metrics
- [ ] Document any custom behavior

---

## File Organization Summary

```
Documentation/
├── accumulate_api_summary.md           # Complete 700+ line reference
├── accumulate_api_quick_reference.md   # Quick lookup + examples
├── accumulate_api_file_reference.md    # Source code navigation
└── API_DOCUMENTATION_INDEX.md          # This file
```

---

## Cross-Reference Quick Links

### By Use Case

**Need to query an account?**
→ See Query Service section in summary
→ See "Query an Account" example in quick reference

**Need to submit a transaction?**
→ See Submit Service section in summary
→ See "Submit Transaction" example in quick reference

**Need to monitor events?**
→ See Event Service section in summary
→ See "Monitor Events" in usage patterns section

**Need to find a service?**
→ See Node Service (find-service) in summary
→ See discovery and bootstrap section in summary

**Need specific code files?**
→ See file reference document organized by transport/service

### By API Version

**Using V3 (Modern)?**
→ Most of summary, quick reference, and all file paths
→ Location: `/pkg/api/v3/`

**Using V2 (Legacy)?**
→ V2 API section in summary
→ File reference → V2 API subsection
→ Location: `/internal/api/v2/`

**Using Ethereum RPC?**
→ Ethereum RPC API section in summary
→ File reference → Ethereum API subsection
→ Location: `/pkg/api/ethereum/`

### By Error Type

**Getting errors?**
→ Error Handling section in summary
→ Common Patterns → Error Handling in quick reference
→ Error codes are -33000 base for JSON-RPC

### By Data Type

**Confused about record types?**
→ Record Types Reference in quick reference (13 types)
→ Records defined in `/pkg/api/v3/records.yml`

**Confused about query types?**
→ Query Types Reference in quick reference (11 types)
→ Queries defined in `/pkg/api/v3/queries.yml`

---

## Statistics

- **Total API Versions:** 2 (V3 primary, V2 legacy)
- **Total Services:** 12 (11 in V3 + 1 private)
- **Total Query Types:** 11
- **Total Record Types:** 13
- **Total Event Types:** 3
- **Total Service Types:** 11 (10 + private)
- **V3 Transport Protocols:** 5 (JSON-RPC, REST, WebSocket, Binary, P2P)
- **Ethereum RPC Methods:** 7
- **V2 JSON-RPC Methods:** 40+
- **Total Generated Code:** 20,000+ lines
- **Total Documentation:** 1,500+ lines

---

## Maintenance and Updates

### When to Reference Each Document

1. **Summary Document** - When you need:
   - Complete specification
   - Parameter details
   - Return type details
   - Protocol comparisons
   - Implementation requirements

2. **Quick Reference** - When you need:
   - Fast lookups
   - Copy-paste examples
   - Quick comparisons
   - Error codes
   - Best practices

3. **File Reference** - When you need:
   - Find source files
   - Understand architecture
   - Code generation info
   - Integration points
   - Build commands

4. **This Index** - When you need:
   - Navigation help
   - Overview of all docs
   - Usage pattern reference
   - Cross-references
   - Statistics

---

## Document Versions

- **Documentation Version:** 1.0
- **Based on Codebase:** Accumulate Network main branch
- **API Versions Covered:** V2 and V3
- **Last Updated:** 2025-01-20
- **Scope:** Complete API coverage

---

## How to Use These Documents

### Getting Started
1. Start with this index
2. Read the quick reference for your use case
3. Dive into the comprehensive guide for details
4. Use file reference to navigate source code

### For Specific Questions
- Query capabilities? → Search for "Query" in summary
- Service method? → Check service section in summary
- Error code? → Check error handling section
- File location? → Check file reference document
- Example? → Check quick reference examples

### For Implementation
1. Review transport protocol comparison
2. Check integration checklist
3. Reference file locations
4. Use quick reference for examples
5. Consult comprehensive guide for details

---

## Support and Clarification

If you need clarification on:
- **Specific endpoints** - See comprehensive guide section for that service
- **Parameter types** - See data structures section or file reference
- **Code examples** - See quick reference guide
- **File locations** - See file reference document
- **Architecture** - See transport/protocol implementations section
- **Best practices** - See best practices section in quick reference

---

END OF DOCUMENTATION INDEX
