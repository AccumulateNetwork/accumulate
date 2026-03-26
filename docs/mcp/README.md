# Accumulate MCP Server Documentation

## 🎯 Quick Links

**Looking for the working MCP server?**
- **Production Implementation:** `~/go/src/gitlab.com/AccumulateNetwork/mcp-accumulate/`
- **Version:** v0.2.0 (40 tools, fully functional)
- **Analysis:** [Existing Implementation Analysis](./existing-implementation-analysis.md)

**This directory contains:**
- Comprehensive design specifications
- API documentation and mapping
- Implementation guidelines
- Integration recommendations

---

## Overview

This directory contains comprehensive documentation for the Model Context Protocol (MCP) server for the Accumulate blockchain network. The MCP server enables AI assistants like Claude to interact with Accumulate networks through a standardized interface.

**Note:** A production-ready implementation already exists in the `mcp-accumulate` repository. This documentation provides the design foundation, API reference, and integration guidance.

## What is MCP?

The Model Context Protocol (MCP) is an open standard that enables AI assistants to integrate with external tools and data sources. It provides a unified way to expose functionality through:

- **Tools**: Invocable functions (like API calls)
- **Resources**: Readable data sources (like blockchain accounts)
- **Prompts**: Pre-configured workflows

## Documentation Structure

### Core MCP Server Documents

#### 1. [MCP API Server Design](./mcp-server-design.md) ⭐
**For live blockchain interaction via API**

Complete design specification covering:
- 28 MCP tools mapped to Accumulate APIs
- 6 resource types for blockchain data access
- 4 workflow prompts for common tasks
- Architecture diagrams and data flow
- Security considerations (keyless design)
- Future enhancements roadmap

**Key Sections:**
- Tool specifications (network, query, transaction, events)
- Resource URI templates
- Configuration management
- Implementation recommendations

**Use Case:** Live node interaction, transaction submission, real-time queries

---

#### 2. [MCP Database Server Design](./mcp-database-server-design.md) 🆕
**For direct database queries and historical analysis**

Database-focused MCP server specification:
- 24 MCP tools for direct database access
- Read-only database queries
- BPT and Merkle proof tools
- Snapshot analysis
- Bulk data operations
- Historical data access

**Key Sections:**
- Database management tools
- Account/chain/transaction queries
- BPT (Binary Patricia Tree) tools
- Snapshot import/export
- Analytics and statistics
- Raw key-value access

**Use Case:** Historical analysis, debugging, Merkle proof generation, bulk exports

---

### Supporting Documentation

#### 3. [API Mapping Reference](./api-mapping-reference.md)
**Detailed endpoint mappings with examples**

Comprehensive mapping documentation:
- MCP tool → Accumulate API endpoint mappings
- Request/response examples for all tools
- Code reference locations
- Error handling patterns
- V2 API compatibility layer
- Testing endpoints

**Includes:**
- 28 detailed tool mappings
- JSON-RPC request/response samples
- REST endpoint equivalents
- WebSocket subscription patterns
- Transaction type reference (60+ types)

#### 4. [Implementation Guide](./implementation-guide.md)
**Step-by-step development instructions**

Practical implementation guide:
- Project structure
- Step-by-step implementation (10 steps)
- Code examples in Go
- Testing strategy (unit + integration)
- Deployment instructions
- Troubleshooting guide

**Covers:**
- MCP server initialization
- Configuration management
- Accumulate API client wrapper
- Tool/resource/prompt handlers
- Docker deployment
- Claude Desktop integration

### 4. Accumulate API Documentation
**Complete API reference (auto-generated)**

Three comprehensive API documentation files:
- **[accumulate_api_summary.md](./accumulate_api_summary.md)** - Complete V3/V2 API specification
- **[accumulate_api_quick_reference.md](./accumulate_api_quick_reference.md)** - Quick lookup and examples
- **[accumulate_api_file_reference.md](./accumulate_api_file_reference.md)** - Source code mapping
- **[API_DOCUMENTATION_INDEX.md](./API_DOCUMENTATION_INDEX.md)** - Cross-reference guide

### Database Documentation

#### 5. [Database Implementation Guide](./accumulate_db_guide.md)
**Complete database architecture reference**

Comprehensive guide to Accumulate's database layer:
- Batch-based transaction model
- Binary Patricia Tree (BPT) structure
- Merkle chain implementation
- Key-value storage backends
- Snapshot formats
- Code examples and integration points

#### 6. [Database Resources Reference](./database_resources.md)
**Quick reference for database development**

Critical paths and structures:
- File locations (absolute paths)
- Package structure
- Interface definitions
- Common operations

#### 7. [Database Summary](./database-summary.md)
**Executive overview of database layer**

High-level concepts:
- Storage architecture
- Access patterns
- Performance characteristics
- MCP integration recommendations

## Quick Start

### For Project Managers & Stakeholders

1. Read: [MCP Server Design Specification](./mcp-server-design.md)
   - Understand the value proposition
   - Review feature list (28 tools)
   - See architecture overview

2. Review: Tool capabilities in **Appendix A** of design spec
   - Network information tools (5)
   - Query tools (11)
   - Transaction tools (9)
   - Event subscriptions (1)
   - Snapshots (1)

3. Understand: Security model
   - Read-only vs. transaction submission
   - Key management (external only)
   - Rate limiting and caching

### For Developers

1. Read: [Implementation Guide](./implementation-guide.md)
   - Review project structure
   - Follow step-by-step implementation
   - Set up development environment

2. Reference: [API Mapping Reference](./api-mapping-reference.md)
   - Understand endpoint mappings
   - Copy request/response examples
   - Locate source code references

3. Explore: Accumulate API docs
   - Study V3 API services
   - Review query types
   - Understand transaction types

### For API Users

1. Browse: [accumulate_api_quick_reference.md](./accumulate_api_quick_reference.md)
   - Quick lookup tables
   - Copy-paste examples
   - Common use cases

2. Deep Dive: [accumulate_api_summary.md](./accumulate_api_summary.md)
   - Complete API specification
   - All parameters and return types
   - Transport protocols

## Key Features

### Comprehensive API Coverage

✅ **Network & Node Information**
- Node status and service discovery
- Consensus and validator information
- Network health metrics
- P2P peer management

✅ **Blockchain Queries**
- Account lookups (40+ account types)
- Transaction queries and history
- Chain traversal and anchors
- Block and directory browsing
- Public key searches

✅ **Transaction Operations**
- Transaction validation
- Transaction submission (signed envelopes)
- Testnet faucet access
- Transaction builders (helper tools)

✅ **Real-Time Events**
- Block event subscriptions
- Transaction status updates
- WebSocket streaming support

### Security-First Design

🔒 **Key Management**
- MCP server NEVER handles private keys
- Signing must be done externally
- Only accepts pre-signed envelopes
- Supports hardware wallet integration

🔒 **Read-Only Mode**
- Optional read-only operation
- Disable transaction submission
- Safe for exploration and analysis

🔒 **Rate Limiting & Caching**
- Client-side rate limiting
- Intelligent caching (30s node info, 5s blockchain)
- Configurable cache TTLs

### Developer Experience

🚀 **Easy Integration**
- Standard MCP protocol
- Works with Claude Desktop
- Compatible with MCP ecosystem
- Comprehensive error messages

🚀 **Flexible Configuration**
- Multi-network support (MainNet, TestNet, DevNet)
- Feature flags for capabilities
- Multiple endpoint fallback
- JSON configuration files

🚀 **Excellent Documentation**
- 5 comprehensive documents
- Code examples in Go
- Request/response samples
- Troubleshooting guides

## Implementation Status

### ⚠️ IMPORTANT: Existing Implementation Available

**A production-ready MCP server already exists!**

**Repository:** `~/go/src/gitlab.com/AccumulateNetwork/mcp-accumulate/`
**Version:** v0.2.0
**Status:** ✅ Production-ready with 40 tools implemented

See [Existing Implementation Analysis](./existing-implementation-analysis.md) for complete details.

### Current Phase: Documentation & Integration ✅

**Completed:**
- [x] Complete API exploration and analysis
- [x] MCP server architecture design
- [x] Tool specifications (28 tools in design, 40 in existing impl)
- [x] Resource specifications (6 types)
- [x] Prompt specifications (4 workflows)
- [x] API mapping documentation
- [x] Implementation guide
- [x] Security design
- [x] Analysis of existing implementation

**Existing Implementation Status:**
- [x] ✅ **40 MCP tools** implemented and tested
- [x] ✅ **Wallet integration** (7 wallet tools)
- [x] ✅ **All query operations** (11 tools)
- [x] ✅ **All transaction types** (15 tools)
- [x] ✅ **Network status** (4 tools)
- [x] ✅ **Search operations** (3 tools)
- [x] ✅ **MCP resources** (wallet:// URIs)
- [x] ✅ **Full SDK integration** (Accumulate v1.4.2)
- [x] ✅ **Production deployment** (Claude Desktop compatible)

### Next Phase: Integration & Enhancement 🚧

**Integration Tasks:**
- [ ] Merge `mcp-accumulate` into `accumulate/tools/mcp-server`
- [ ] Add `accumulate://` resource URIs (6 types)
- [ ] Implement missing tools from design spec:
  - [ ] `accumulate_find_service`
  - [ ] `accumulate_query_delegate`
- [ ] Add secure mode (keyless operation)
- [ ] Enhance configuration (caching, read-only mode)
- [ ] Cross-link documentation
- [ ] Unified testing suite

### Future Phase: Advanced Features 📋

- [ ] Event subscriptions (WebSocket streaming)
- [ ] Snapshot management tools
- [ ] Advanced caching with Redis
- [ ] Connection pooling and failover
- [ ] Prometheus metrics
- [ ] GraphQL-style queries
- [ ] Batch operations

## Tool Catalog

### Network & Node Information (5 tools)

| Tool | Description | API |
|------|-------------|-----|
| `accumulate_node_info` | Get node information | V3 node-info |
| `accumulate_find_service` | Find service providers | V3 find-service |
| `accumulate_network_status` | Network health | V3 network-status |
| `accumulate_consensus_status` | Consensus info | V3 consensus-status |
| `accumulate_metrics` | Node metrics | V3 metrics |

### Query Tools (11 tools)

| Tool | Description | Query Type |
|------|-------------|------------|
| `accumulate_query_account` | Query account | DefaultQuery |
| `accumulate_query_transaction` | Query transaction | DefaultQuery |
| `accumulate_query_chain` | Query chain entries | ChainQuery |
| `accumulate_query_data` | Query data entries | DataQuery |
| `accumulate_query_directory` | List directory | DirectoryQuery |
| `accumulate_search_accounts` | Search accounts | SearchQuery |
| `accumulate_query_block` | Query block | BlockQuery |
| `accumulate_query_anchors` | Query anchors | AnchorSearchQuery |
| `accumulate_query_key_index` | Find key page | PublicKeySearchQuery |
| `accumulate_query_public_key_hash` | Search by key hash | PublicKeyHashSearchQuery |
| `accumulate_query_delegate` | Find delegates | DelegateSearchQuery |
| `accumulate_query_pending` | Pending transactions | PendingQuery |

### Transaction Tools (9 tools)

| Tool | Description | Type |
|------|-------------|------|
| `accumulate_submit_transaction` | Submit signed tx | Submit |
| `accumulate_validate_transaction` | Validate tx | Validate |
| `accumulate_faucet` | Testnet faucet | Faucet |
| `accumulate_build_send_tokens` | Build SendTokens | Builder |
| `accumulate_build_create_account` | Build CreateAccount | Builder |
| `accumulate_build_update_account` | Build UpdateAccount | Builder |
| `accumulate_build_write_data` | Build WriteData | Builder |
| `accumulate_build_token_issuance` | Build IssueTokens | Builder |
| `accumulate_build_burn_tokens` | Build BurnTokens | Builder |

### Event & Other Tools (3 tools)

| Tool | Description | Type |
|------|-------------|------|
| `accumulate_subscribe_events` | Event subscriptions | Events |
| `accumulate_list_snapshots` | List snapshots | Snapshots |

**Total: 28 Tools**

## Resource Catalog

| Resource URI Template | Description |
|-----------------------|-------------|
| `accumulate://account/{url}` | Account information |
| `accumulate://transaction/{txid}` | Transaction details |
| `accumulate://chain/{url}/{chain}` | Chain entries |
| `accumulate://directory/{url}` | Directory listing |
| `accumulate://block/{partition}/{height}` | Block data |
| `accumulate://network/{network}` | Network status |

## Configuration Example

```json
{
  "network": "MainNet",
  "endpoints": [
    "https://mainnet.accumulatenetwork.io/v3",
    "https://api.accumulate.defidevs.io/v3"
  ],
  "timeout": "30s",

  "enable_v2_compat": true,
  "enable_transaction_building": true,
  "enable_faucet": false,
  "enable_snapshots": false,
  "enable_events": true,

  "read_only": false,
  "allow_transaction_submit": true,
  "require_confirmation": true,
  "max_query_results": 1000,

  "cache_node_info": "30s",
  "cache_blockchain": "5s"
}
```

## Usage Examples

### Query Account Balance

```
User: "What is the balance of acc://alice.acme/tokens?"

AI uses: accumulate_query_account
  - url: "acc://alice.acme/tokens"

Response:
  - balance: "1000000000" (10 ACME)
  - tokenUrl: "acc://ACME"
  - type: "tokenAccount"
```

### Browse Directory

```
User: "Show me all accounts under alice.acme"

AI uses: accumulate_query_directory
  - url: "acc://alice.acme"
  - count: 20

Response:
  - acc://alice.acme/tokens (tokenAccount)
  - acc://alice.acme/data (dataAccount)
  - acc://alice.acme/book (keyBook)
```

### Check Transaction Status

```
User: "What's the status of transaction abc123...?"

AI uses: accumulate_query_transaction
  - txid: "abc123..."

Response:
  - status: "delivered"
  - type: "sendTokens"
  - result: success
```

### Build Transaction (Not Sign)

```
User: "Build a transaction to send 10 ACME from alice to bob"

AI uses: accumulate_build_send_tokens
  - from: "acc://alice.acme/tokens"
  - to: [{"url": "acc://bob.acme/tokens", "amount": "10000000000"}]

Response:
  - unsigned transaction payload
  - required signers: ["acc://alice.acme/book/1"]
  - estimated fee: "0.01 ACME"

Note: User must sign externally before submission
```

## Architecture Highlights

### Layered Design

```
┌─────────────────────────────────────┐
│   AI Assistant (Claude, etc.)       │  User interaction
└───────────────┬─────────────────────┘
                │ MCP Protocol (JSON-RPC over stdio/HTTP)
┌───────────────▼─────────────────────┐
│   Accumulate MCP Server             │  Tool/Resource handlers
│   - Tool routing                    │  Parameter validation
│   - Resource resolution             │  Response formatting
│   - Caching layer                   │
└───────────────┬─────────────────────┘
                │ Accumulate API Client
┌───────────────▼─────────────────────┐
│   Accumulate Network Node(s)        │  Blockchain operations
│   - JSON-RPC / REST / WebSocket     │  State queries
│   - P2P network                     │  Transaction submission
└─────────────────────────────────────┘
```

### Key Design Principles

1. **Separation of Concerns**
   - MCP server handles protocol translation
   - Accumulate client handles API communication
   - Key management stays external

2. **Fail-Safe Defaults**
   - Read-only mode available
   - Transaction confirmation required
   - Rate limiting enabled
   - Timeouts enforced

3. **Extensibility**
   - Modular tool registration
   - Pluggable caching
   - Multi-endpoint support
   - Feature flags for capabilities

## Testing Strategy

### Unit Tests
- Tool parameter validation
- Response parsing
- Error handling
- Cache behavior

### Integration Tests
- Live API calls to testnet
- End-to-end tool invocation
- Resource URI resolution
- Event subscription handling

### Manual Testing
- MCP Inspector tool
- Claude Desktop integration
- Performance benchmarks
- Security audit

## Deployment Options

### Standalone Binary
```bash
./accumulate-mcp
```

### Docker Container
```bash
docker run -v /path/to/config.json:/config.json accumulate-mcp
```

### Claude Desktop Integration
```json
{
  "mcpServers": {
    "accumulate": {
      "command": "/path/to/accumulate-mcp"
    }
  }
}
```

### Systemd Service
```bash
systemctl start accumulate-mcp
systemctl enable accumulate-mcp
```

## Security Considerations

### Critical Security Requirements

🔴 **NEVER Handle Private Keys**
- MCP server must not accept, store, or use private keys
- All signing must be done externally
- Only accept pre-signed transaction envelopes

🔴 **Input Validation**
- Validate all URLs and parameters
- Sanitize user inputs
- Prevent injection attacks

🔴 **Rate Limiting**
- Protect against DoS
- Respect node API limits
- Implement client-side throttling

🔴 **Error Handling**
- Never expose sensitive information in errors
- Provide helpful but safe error messages
- Log security-relevant events

### Recommended Practices

✅ Use HTTPS for all API calls
✅ Implement request timeouts
✅ Enable read-only mode for untrusted environments
✅ Require confirmation for transaction submission
✅ Log all transaction attempts
✅ Monitor for unusual activity

## Contributing

### Development Workflow

1. Read design documentation
2. Set up development environment
3. Implement feature or fix
4. Write tests (unit + integration)
5. Run test suite
6. Submit merge request
7. Code review
8. Merge to main

### Code Standards

- Follow Go best practices
- Write comprehensive tests
- Document all public APIs
- Use meaningful commit messages
- Keep functions focused and small

## Support & Resources

### Documentation
- [MCP Specification](https://spec.modelcontextprotocol.io/)
- [MCP Go SDK](https://github.com/mark3labs/mcp-go)
- [Accumulate Documentation](https://docs.accumulatenetwork.io/)
- [Accumulate Protocol](https://accumulatenetwork.io/)

### Community
- GitLab Issues: [Report bugs and request features](https://gitlab.com/AccumulateNetwork/accumulate/-/issues)
- Discord: [Accumulate Community](https://discord.gg/accumulate)

### Related Projects
- Accumulate CLI: `accumulated` command-line tool
- Accumulate SDK: Client libraries for multiple languages
- Accumulate Explorer: Web-based blockchain explorer

## Roadmap

### Phase 1: Core Implementation (Current)
- [x] Design specification
- [x] API documentation
- [x] Implementation guide
- [ ] Core MCP server
- [ ] Network tools
- [ ] Query tools
- [ ] Transaction tools
- [ ] Basic tests

### Phase 2: Advanced Features
- [ ] Event subscriptions
- [ ] Enhanced caching
- [ ] Connection pooling
- [ ] Comprehensive tests
- [ ] Performance optimization
- [ ] Docker deployment

### Phase 3: Ecosystem Integration
- [ ] Multi-network support
- [ ] Advanced analytics
- [ ] GraphQL queries
- [ ] Batch operations
- [ ] Development tools
- [ ] Production hardening

## FAQ

**Q: Does the MCP server handle private keys?**
A: No. The MCP server NEVER handles private keys. All transaction signing must be done externally.

**Q: Can I use this on MainNet?**
A: Yes, but configure carefully. Enable `require_confirmation` for transaction submission.

**Q: What's the difference between V2 and V3 APIs?**
A: V3 is the modern API with better structure. V2 is maintained for backward compatibility.

**Q: How do I sign transactions?**
A: Use the Accumulate CLI (`accumulated tx sign`) or SDK with your private keys, then submit the signed envelope via `accumulate_submit_transaction`.

**Q: Is rate limiting necessary?**
A: Yes, to protect both the MCP server and the Accumulate nodes from overload.

**Q: Can I run this in read-only mode?**
A: Yes, set `read_only: true` in config to disable transaction submission.

## License

MIT License - See LICENSE file for details

## Version History

- **v1.0** (2025-10-20): Initial documentation
  - Complete MCP server design
  - 28 tools specified
  - 6 resources defined
  - 4 prompts designed
  - Full API documentation
  - Implementation guide
  - Security model

## Acknowledgments

- Accumulate Network team for the robust API
- MCP community for the protocol standard
- mark3labs for the Go SDK
- Contributors to this documentation

---

**For questions or issues, please open an issue on GitLab: https://gitlab.com/AccumulateNetwork/accumulate/-/issues**
