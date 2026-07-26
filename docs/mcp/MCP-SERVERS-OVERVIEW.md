# Accumulate MCP Servers - Complete Overview

## Executive Summary

This document provides a comprehensive overview of **two distinct MCP servers** designed for the Accumulate blockchain protocol:

1. **API MCP Server** - Live blockchain interaction via JSON-RPC APIs
2. **Database MCP Server** - Direct database queries for historical analysis

Both servers enable AI assistants like Claude to interact with Accumulate, but serve different purposes and use cases.

## Quick Comparison

| Feature | API MCP Server | Database MCP Server |
|---------|----------------|---------------------|
| **Purpose** | Live blockchain operations | Historical data analysis |
| **Data Source** | Running node via API | Database files directly |
| **Tools Count** | 40 (implemented)<br>28 (designed) | 24 (designed) |
| **Node Required** | ✅ Yes | ❌ No |
| **Transaction Submit** | ✅ Yes | ❌ Read-only |
| **Historical Data** | Limited to node | ✅ Full access |
| **Merkle Proofs** | Via API | ✅ Generated directly |
| **Wallet Integration** | ✅ Yes (existing impl) | ❌ No |
| **Performance** | Network latency | Direct file I/O |
| **Use Cases** | Trading, transactions, live queries | Debugging, analysis, audits |

## MCP Server #1: API Server

### Status: ✅ Production-Ready (Existing Implementation)

**Repository:** `~/go/src/gitlab.com/AccumulateNetwork/mcp-accumulate/`

**Version:** v0.2.0

**Implementation Status:**
- ✅ 40 tools implemented
- ✅ Wallet integration (7 tools)
- ✅ Full SDK integration (Accumulate v1.4.2)
- ✅ Claude Desktop compatible
- ✅ Production tested

### Tool Categories

#### Wallet Management (7 tools)
- wallet_init
- wallet_vault_open / wallet_vault_lock
- wallet_generate_key
- wallet_list_keys
- wallet_set_network
- wallet_get_status

#### Query Operations (11 tools)
- accumulate_query_account
- accumulate_query_tx
- accumulate_query_chain
- accumulate_query_data
- accumulate_query_directory
- accumulate_query_pending
- accumulate_query_keybook
- accumulate_query_keypage
- accumulate_query_minor_block
- accumulate_query_major_block
- accumulate_search_public_key

#### Transaction Operations (15 tools)
- accumulate_send_tokens
- accumulate_create_lite_account
- accumulate_create_adi
- accumulate_create_data_account
- accumulate_create_token_account
- accumulate_create_keypage
- accumulate_create_keybook
- accumulate_create_token
- accumulate_write_data
- accumulate_generate_key
- accumulate_add_credits
- accumulate_update_keypage
- accumulate_update_account_auth
- accumulate_issue_tokens
- accumulate_burn_tokens

#### Network & Status (4 tools)
- accumulate_node_info
- accumulate_network_status
- accumulate_consensus_status
- accumulate_metrics

#### Search & Faucet (3 tools)
- accumulate_search_public_key_hash
- accumulate_search_anchor
- accumulate_faucet (testnet)

### Design Documents

- [MCP API Server Design](./mcp-server-design.md) - Complete specification (28 tools)
- [API Mapping Reference](./api-mapping-reference.md) - Endpoint mappings
- [Implementation Guide](./implementation-guide.md) - Development guide
- [Existing Implementation Analysis](./existing-implementation-analysis.md) - Comparison

### Use Cases

✅ **Live Blockchain Interaction**
- Send tokens between accounts
- Create ADIs and accounts
- Query current state
- Submit transactions

✅ **Wallet Operations**
- Generate and manage keys
- Multiple vault support
- Network switching
- Secure key storage

✅ **Network Monitoring**
- Check node status
- Monitor consensus
- Track network metrics
- View faucet status

### Security Model

**Integrated Approach:**
- Tools accept private keys directly
- All-in-one transaction building + signing + submission
- Wallet-based key management
- Convenient for developers

## MCP Server #2: Database Server

### Status: 📋 Designed (Ready for Implementation)

**Purpose:** Direct database access for historical queries and analysis

**Implementation Status:**
- ✅ Complete design specification
- ✅ 24 tools designed
- ✅ Database architecture documented
- ⏳ Implementation pending

### Tool Categories

#### Database Management (3 tools)
- db_open
- db_close
- db_info

#### Account Queries (3 tools)
- db_query_account
- db_list_accounts
- db_get_account_hash

#### Chain Queries (3 tools)
- db_query_chain
- db_query_chain_entry
- db_get_chain_anchor

#### Transaction Queries (3 tools)
- db_query_transaction
- db_query_transaction_status
- db_list_transactions

#### BPT Operations (4 tools)
- db_bpt_get_root
- db_bpt_get_proof
- db_bpt_verify_proof
- db_bpt_iterate

#### Data Account (1 tool)
- db_query_data_entries

#### Snapshots (2 tools)
- db_snapshot_info
- db_snapshot_export

#### Analysis (3 tools)
- db_analyze_accounts
- db_analyze_chains
- db_get_statistics

#### Advanced/Raw (2 tools)
- db_raw_get
- db_raw_iterate

### Design Documents

- [MCP Database Server Design](./mcp-database-server-design.md) - Complete specification
- [Database Implementation Guide](./accumulate_db_guide.md) - Database architecture
- [Database Resources](./database_resources.md) - Quick reference
- [Database Summary](./database-summary.md) - Executive overview

### Use Cases

✅ **Historical Analysis**
- Query past database states
- Analyze account history
- Extract transaction patterns
- Generate reports

✅ **Database Debugging**
- Inspect database contents
- Verify Merkle proofs
- Check BPT integrity
- Analyze chain health

✅ **Bulk Operations**
- Export all accounts
- Statistical analysis
- Data migration
- Audit trail generation

✅ **Merkle Proof Generation**
- Generate BPT proofs
- Verify account inclusion
- Create audit trails
- Independent verification

### Security Model

**Read-Only Approach:**
- Database opened in read-only mode
- No transaction submission
- No key handling
- Safe for analysis

## Architecture Comparison

### API MCP Server Architecture

```
┌─────────────────────────┐
│    AI Assistant         │
└────────┬────────────────┘
         │ MCP Protocol (stdio)
┌────────▼────────────────┐
│  MCP API Server         │
│  - 40 tools             │
│  - Wallet integration   │
│  - Transaction signing  │
└────────┬────────────────┘
         │ V3 API (HTTPS)
┌────────▼────────────────┐
│  Accumulate Node        │
│  - JSON-RPC API         │
│  - Live blockchain      │
└─────────────────────────┘
```

### Database MCP Server Architecture

```
┌─────────────────────────┐
│    AI Assistant         │
└────────┬────────────────┘
         │ MCP Protocol (stdio)
┌────────▼────────────────┐
│  MCP Database Server    │
│  - 24 tools             │
│  - Session management   │
│  - Read-only access     │
└────────┬────────────────┘
         │ Direct I/O
┌────────▼────────────────┐
│  Database Files         │
│  - BadgerDB / LevelDB   │
│  - Snapshot files       │
│  - Historical data      │
└─────────────────────────┘
```

## Integration Scenarios

### Scenario 1: Dual Deployment

Run both servers simultaneously for comprehensive coverage:

```json
{
  "mcpServers": {
    "accumulate-api": {
      "command": "/path/to/mcp-accumulate",
      "env": {
        "ACCUMULATE_NETWORK": "mainnet",
        "ACCUMULATE_WALLET_DIR": "/home/user/.wallet"
      }
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

**Benefits:**
- Live operations via API server
- Historical analysis via database server
- Merkle proof verification
- Comprehensive debugging

### Scenario 2: API Server Only

For live blockchain interaction:

```json
{
  "mcpServers": {
    "accumulate": {
      "command": "/path/to/mcp-accumulate"
    }
  }
}
```

**Best for:**
- Trading and transactions
- Wallet management
- Live queries
- Development

### Scenario 3: Database Server Only

For analysis and debugging:

```json
{
  "mcpServers": {
    "accumulate-db": {
      "command": "/path/to/mcp-accumulate-db"
    }
  }
}
```

**Best for:**
- Historical research
- Database debugging
- Audit generation
- Bulk exports

## Development Roadmap

### Phase 1: API Server (✅ COMPLETE)
- [x] 40 tools implemented
- [x] Wallet integration
- [x] SDK integration (v1.4.2)
- [x] Production deployment
- [x] Claude Desktop integration

### Phase 2: API Server Enhancement (🚧 In Progress)
- [ ] Add accumulate:// resources
- [ ] Add missing tools (find_service, query_delegate)
- [ ] Secure mode (keyless operation)
- [ ] Enhanced configuration
- [ ] Event subscriptions

### Phase 3: Database Server Implementation (📋 Planned)
- [ ] Initialize project structure
- [ ] Implement database management tools
- [ ] Implement query tools
- [ ] Implement BPT tools
- [ ] Implement snapshot tools
- [ ] Session management
- [ ] Testing and validation

### Phase 4: Integration & Documentation (📋 Planned)
- [ ] Merge mcp-accumulate into accumulate repo
- [ ] Unified documentation
- [ ] Cross-server examples
- [ ] Deployment guides
- [ ] Performance optimization

## Documentation Index

### Core Specifications
1. **[mcp-server-design.md](./mcp-server-design.md)** - API MCP server design (28 tools)
2. **[mcp-database-server-design.md](./mcp-database-server-design.md)** - Database MCP server design (24 tools)

### Implementation Guides
3. **[implementation-guide.md](./implementation-guide.md)** - Step-by-step API server implementation
4. **[QUICKSTART.md](./QUICKSTART.md)** - 30-minute quick start
5. **[existing-implementation-analysis.md](./existing-implementation-analysis.md)** - Analysis of mcp-accumulate repo

### API Documentation
6. **[api-mapping-reference.md](./api-mapping-reference.md)** - API endpoint mappings
7. **[accumulate_api_summary.md](./accumulate_api_summary.md)** - Complete API reference
8. **[accumulate_api_quick_reference.md](./accumulate_api_quick_reference.md)** - Quick lookup
9. **[accumulate_api_file_reference.md](./accumulate_api_file_reference.md)** - Source code mapping
10. **[API_DOCUMENTATION_INDEX.md](./API_DOCUMENTATION_INDEX.md)** - API cross-reference

### Database Documentation
11. **[accumulate_db_guide.md](./accumulate_db_guide.md)** - Complete database guide
12. **[database_resources.md](./database_resources.md)** - Database quick reference
13. **[database-summary.md](./database-summary.md)** - Database overview

### This Document
14. **[MCP-SERVERS-OVERVIEW.md](./MCP-SERVERS-OVERVIEW.md)** - You are here!

**Total:** 14 documents, 9000+ lines

## Quick Start Guides

### Using the API MCP Server

```bash
# The server is already implemented!
cd ~/go/src/gitlab.com/AccumulateNetwork/mcp-accumulate
go build -o mcp-accumulate
./mcp-accumulate
```

Configure in Claude Desktop:
```json
{
  "mcpServers": {
    "accumulate": {
      "command": "/path/to/mcp-accumulate"
    }
  }
}
```

Ask Claude:
> "What is the balance of acc://alice.acme/tokens?"

### Using the Database MCP Server (Future)

```bash
# Implementation pending
cd tools/mcp-database-server
go build -o mcp-accumulate-db
./mcp-accumulate-db
```

Ask Claude:
> "Open database at /var/accumulate/bvn0/database and analyze all token accounts"

## Key Takeaways

1. **Two Distinct Servers**
   - API Server: Live blockchain (40 tools, production-ready)
   - Database Server: Historical analysis (24 tools, designed)

2. **Complementary, Not Competitive**
   - API Server for live operations
   - Database Server for analysis
   - Both can run simultaneously

3. **Production Status**
   - API Server: ✅ Ready to use now
   - Database Server: 📋 Design complete, implementation needed

4. **Security Models**
   - API Server: Integrated wallet, handles keys
   - Database Server: Read-only, no keys

5. **Comprehensive Documentation**
   - 14 documents covering all aspects
   - 9000+ lines of documentation
   - Design specs, guides, references

## Contributing

### To API MCP Server
Repository: `~/go/src/gitlab.com/AccumulateNetwork/mcp-accumulate/`

Tasks:
- Add missing tools
- Implement accumulate:// resources
- Add secure mode
- Enhance configuration

### To Database MCP Server
Repository: `accumulate/tools/mcp-database-server/` (to be created)

Tasks:
- Implement 24 designed tools
- Session management
- Database access layer
- Testing

## Support & Resources

### Documentation
- This directory (`docs/mcp/`)
- [MCP Specification](https://spec.modelcontextprotocol.io/)
- [Accumulate Documentation](https://docs.accumulatenetwork.io/)

### Community
- GitLab Issues: https://gitlab.com/AccumulateNetwork/accumulate/-/issues
- Discord: https://discord.gg/accumulate

## Version History

- **v1.0** (2025-10-20): Complete MCP documentation
  - API MCP server design (28 tools)
  - Database MCP server design (24 tools)
  - Full API documentation
  - Database architecture guide
  - Implementation guides
  - Existing implementation analysis
  - This overview document

---

**For questions or implementation support:**
- API Server: See existing implementation at `mcp-accumulate/`
- Database Server: Follow design spec in `mcp-database-server-design.md`
- General: Open issue on GitLab

**Both servers working together provide complete Accumulate blockchain access for AI assistants!**
