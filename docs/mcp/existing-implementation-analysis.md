# Accumulate MCP Implementation - Analysis & Integration

## Overview

This document analyzes the existing MCP server implementation in the `mcp-accumulate` repository and shows how it relates to the comprehensive design specification created in this repository.

## Existing Implementation Status

### Repository Location
`~/go/src/gitlab.com/AccumulateNetwork/mcp-accumulate/`

### Current Version
**v0.2.0** - Production-ready MCP server with wallet integration

### Architecture

The existing implementation uses a **custom MCP protocol implementation** (not the mark3labs/mcp-go SDK):

```
mcp-accumulate/
├── main.go                 # Stdio JSON-RPC handler
├── server/
│   ├── server.go          # MCP protocol implementation
│   ├── tools.go           # 40 tool implementations
│   ├── tools_wallet.go    # Wallet-specific tools
│   ├── tool_definitions.go # MCP tool schemas
│   ├── resources.go       # MCP resources (wallet://)
│   ├── config.go          # Configuration management
│   └── state.go           # Runtime state management
├── client/
│   └── client.go          # Accumulate SDK v1.4.2 wrapper
└── wallet/
    └── client.go          # Wallet integration (ccli wrapper)
```

## Tools Implemented (40 Total)

### Wallet Management Tools (7)
| Tool | Status | Description |
|------|--------|-------------|
| `wallet_init` | ✅ Implemented | Initialize new wallet |
| `wallet_vault_open` | ✅ Implemented | Unlock vault |
| `wallet_vault_lock` | ✅ Implemented | Lock vault |
| `wallet_generate_key` | ✅ Implemented | Generate key pair |
| `wallet_list_keys` | ✅ Implemented | List wallet keys |
| `wallet_set_network` | ✅ Implemented | Set active network |
| `wallet_get_status` | ✅ Implemented | Get wallet status |

### Query Tools (11)
| Tool | Status | Design Doc Equivalent |
|------|--------|----------------------|
| `accumulate_query_account` | ✅ Implemented | accumulate_query_account |
| `accumulate_query_tx` | ✅ Implemented | accumulate_query_transaction |
| `accumulate_query_chain` | ✅ Implemented | accumulate_query_chain |
| `accumulate_query_data` | ✅ Implemented | accumulate_query_data |
| `accumulate_query_directory` | ✅ Implemented | accumulate_query_directory |
| `accumulate_query_pending` | ✅ Implemented | accumulate_query_pending |
| `accumulate_query_keybook` | ✅ Implemented | (additional - not in design) |
| `accumulate_query_keypage` | ✅ Implemented | (additional - not in design) |
| `accumulate_query_minor_block` | ✅ Implemented | accumulate_query_block |
| `accumulate_query_major_block` | ✅ Implemented | accumulate_query_block |
| `accumulate_search_public_key` | ✅ Implemented | accumulate_query_key_index |

### Network & Status Tools (4)
| Tool | Status | Design Doc Equivalent |
|------|--------|----------------------|
| `accumulate_node_info` | ✅ Implemented | accumulate_node_info |
| `accumulate_network_status` | ✅ Implemented | accumulate_network_status |
| `accumulate_consensus_status` | ✅ Implemented | accumulate_consensus_status |
| `accumulate_metrics` | ✅ Implemented | accumulate_metrics |

### Transaction Tools (15)
| Tool | Status | Design Doc Equivalent |
|------|--------|----------------------|
| `accumulate_send_tokens` | ✅ Implemented | (combined build+submit) |
| `accumulate_create_lite_account` | ✅ Implemented | (helper utility) |
| `accumulate_create_adi` | ✅ Implemented | accumulate_build_create_account |
| `accumulate_create_data_account` | ✅ Implemented | accumulate_build_create_account |
| `accumulate_create_token_account` | ✅ Implemented | accumulate_build_create_account |
| `accumulate_create_keypage` | ✅ Implemented | accumulate_build_create_account |
| `accumulate_create_keybook` | ✅ Implemented | accumulate_build_create_account |
| `accumulate_create_token` | ✅ Implemented | accumulate_build_create_account |
| `accumulate_write_data` | ✅ Implemented | accumulate_build_write_data |
| `accumulate_generate_key` | ✅ Implemented | (helper utility) |
| `accumulate_add_credits` | ✅ Implemented | (not in design) |
| `accumulate_update_keypage` | ✅ Implemented | accumulate_build_update_account |
| `accumulate_update_account_auth` | ✅ Implemented | accumulate_build_update_account |
| `accumulate_issue_tokens` | ✅ Implemented | accumulate_build_token_issuance |
| `accumulate_burn_tokens` | ✅ Implemented | accumulate_build_burn_tokens |

### Search Tools (3)
| Tool | Status | Design Doc Equivalent |
|------|--------|----------------------|
| `accumulate_search_public_key` | ✅ Implemented | accumulate_query_key_index |
| `accumulate_search_public_key_hash` | ✅ Implemented | accumulate_query_public_key_hash |
| `accumulate_search_anchor` | ✅ Implemented | accumulate_query_anchors |

### Testnet Tools (1)
| Tool | Status | Design Doc Equivalent |
|------|--------|----------------------|
| `accumulate_faucet` | ✅ Implemented | accumulate_faucet |

## Key Differences from Design Spec

### 1. Tool Design Philosophy

**Existing Implementation (Practical):**
- **All-in-one transaction tools**: Tools like `accumulate_send_tokens` handle building, signing, AND submitting
- **Wallet integration**: Tools accept `private_key` parameter and handle signing internally
- **Immediate execution**: Transactions are submitted immediately upon creation

**Design Spec (Separation of Concerns):**
- **Separate build/submit**: `accumulate_build_send_tokens` (build only) vs `accumulate_submit_transaction` (submit only)
- **No key handling**: MCP server NEVER touches private keys
- **External signing**: User must sign transactions externally

### 2. Wallet Integration

**Existing Implementation:**
- ✅ Full wallet integration via `ccli` wrapper
- ✅ Stateful wallet management (vaults, keys)
- ✅ MCP Resources: `wallet://config`, `wallet://state`, `wallet://keys`
- ✅ Environment-based configuration

**Design Spec:**
- ❌ No wallet integration specified
- ❌ No stateful key management
- ✅ Resource URIs: `accumulate://account/{url}` pattern

### 3. MCP SDK

**Existing Implementation:**
- Custom JSON-RPC MCP protocol handler
- Manual implementation of `initialize`, `tools/list`, `tools/call`, etc.
- Direct stdio communication

**Design Spec:**
- Recommends `github.com/mark3labs/mcp-go` SDK
- Higher-level abstractions
- Better type safety

### 4. Security Model

**Existing Implementation:**
```go
// Tools accept private keys directly
{
    "private_key": "0x1234...",
    "from": "acc://alice.acme/tokens",
    "to": "acc://bob.acme/tokens"
}
```

**Design Spec:**
```go
// Tools never see private keys
// Step 1: Build transaction
tx := mcp_build_send_tokens(...)

// Step 2: Sign externally (NOT in MCP)
signed := external_signer.Sign(tx)

// Step 3: Submit signed envelope
mcp_submit_transaction(signed)
```

## Coverage Comparison

### Implemented vs Designed

| Category | Existing Impl | Design Spec | Gap |
|----------|---------------|-------------|-----|
| **Query Tools** | 11 | 11 | ✅ Full coverage |
| **Network Tools** | 4 | 5 | Missing: find_service |
| **Transaction Tools** | 15 | 9 | ✅ More than designed |
| **Search Tools** | 3 | 4 | Missing: query_delegate |
| **Event Tools** | 0 | 1 | Missing: subscribe_events |
| **Snapshot Tools** | 0 | 1 | Missing: list_snapshots |
| **Wallet Tools** | 7 | 0 | ✅ Beyond design spec |
| **Total** | **40** | **28** | +12 tools |

## MCP Resources

### Existing Implementation

```
wallet://config  - Wallet and network configuration
wallet://state   - Runtime wallet state (vault status)
wallet://keys    - List of keys in wallet
```

### Design Spec

```
accumulate://account/{url}              - Account information
accumulate://transaction/{txid}         - Transaction details
accumulate://chain/{url}/{chain}        - Chain entries
accumulate://directory/{url}            - Directory listing
accumulate://block/{partition}/{height} - Block data
accumulate://network/{network}          - Network status
```

**Integration Opportunity:** Implement both resource patterns

## Configuration Comparison

### Existing Implementation
```json
{
  "mcpServers": {
    "accumulate": {
      "command": "/path/to/mcp-accumulate",
      "env": {
        "ACCUMULATE_NETWORK": "devnet",
        "ACCUMULATE_WALLET_DIR": "/home/user/.accumulate/devnet-wallet",
        "ACCUMULATE_SERVER": "http://localhost:8080/v3"
      }
    }
  }
}
```

**Features:**
- Environment variable configuration
- Runtime wallet path
- Network selection (mainnet/testnet/devnet/custom)
- Stateful operation

### Design Spec
```json
{
  "network": "MainNet",
  "endpoints": ["https://mainnet.accumulatenetwork.io/v3"],
  "timeout": "30s",
  "enable_transaction_building": true,
  "read_only": false,
  "max_query_results": 1000,
  "cache_node_info": "30s"
}
```

**Features:**
- File-based configuration
- Feature flags
- Caching settings
- Read-only mode
- Multiple endpoint failover

**Integration Opportunity:** Combine both approaches

## SDK Integration

### Existing Implementation ✅

**Excellent SDK integration:**
```go
import (
    "gitlab.com/AccumulateNetwork/accumulate/pkg/api/v3"
    "gitlab.com/AccumulateNetwork/accumulate/pkg/api/v3/jsonrpc"
    "gitlab.com/AccumulateNetwork/accumulate/protocol"
    urlpkg "gitlab.com/AccumulateNetwork/accumulate/pkg/url"
)

// Proper SDK usage
client := jsonrpc.NewClient(endpoint)
query := &api.DefaultQuery{Url: accountURL}
resp, err := client.Query(ctx, query)
```

**Achievements:**
- ✅ Uses official SDK v1.4.2
- ✅ Typed queries (no `map[string]interface{}`)
- ✅ Protocol types for transactions
- ✅ Proper URL handling
- ✅ ED25519 signature creation
- ✅ Lite account derivation

### Design Spec

**Similar approach recommended:**
```go
import (
    "gitlab.com/AccumulateNetwork/accumulate/pkg/api/v3"
    "gitlab.com/AccumulateNetwork/accumulate/pkg/client"
)

v3Client := client.NewV3Client(endpoint)
result := v3Client.Query(ctx, &api.DefaultQuery{...})
```

**Alignment:** Existing implementation already follows best practices

## Integration Strategy

### Option 1: Merge Implementations ⭐ RECOMMENDED

**Approach:** Bring existing implementation into accumulate repo

```
accumulate/
└── tools/
    └── mcp-server/           # Merge mcp-accumulate here
        ├── main.go
        ├── server/
        ├── client/
        ├── wallet/
        └── docs/              # Use docs/mcp/ design specs
```

**Benefits:**
- ✅ Keep battle-tested implementation
- ✅ Leverage existing wallet integration
- ✅ Add design spec enhancements
- ✅ Single source of truth

**Tasks:**
1. Move `mcp-accumulate` to `accumulate/tools/mcp-server`
2. Add missing tools from design spec:
   - `accumulate_find_service`
   - `accumulate_query_delegate`
   - `accumulate_subscribe_events` (future)
   - `accumulate_list_snapshots` (future)
3. Implement `accumulate://` resources (in addition to `wallet://`)
4. Add configuration enhancements (caching, read-only mode, etc.)
5. Separate build/submit tools for security-conscious users

### Option 2: Parallel Development

**Approach:** Maintain both implementations

**mcp-accumulate (practical, integrated):**
- Wallet-integrated
- All-in-one transaction tools
- Stateful operation
- For developers who want convenience

**accumulate/tools/mcp-server (secure, modular):**
- No key handling
- Separate build/submit
- Stateless operation
- For security-conscious users

**Challenges:**
- Duplicated maintenance
- Confusing for users
- Divergent features

### Option 3: Refactor Existing

**Approach:** Enhance `mcp-accumulate` with design spec features

**Add to existing:**
1. **Dual-mode operation:**
   ```go
   if config.SecureMode {
       // Design spec mode: separate build/submit, no keys
   } else {
       // Current mode: integrated wallet, all-in-one
   }
   ```

2. **Accumulate resources:**
   - Keep: `wallet://` resources
   - Add: `accumulate://` resources

3. **Optional features:**
   - Caching layer
   - Read-only mode
   - Rate limiting

4. **New tools:**
   - `accumulate_build_*` (unsigned transaction builders)
   - `accumulate_submit_envelope` (submit pre-signed)
   - `accumulate_validate_envelope`

## Recommendations

### Immediate Actions

1. **Document Existing Implementation** ✅
   - Update README.md in this repo to reference `mcp-accumulate`
   - Cross-link documentation
   - Show relationship between repos

2. **Create Migration Path**
   - Move `mcp-accumulate` into `accumulate/tools/mcp-server`
   - Preserve git history
   - Update import paths

3. **Add Missing Tools** (from design spec)
   - `accumulate_find_service` - Service discovery
   - `accumulate_query_delegate` - Delegation search
   - (Future) `accumulate_subscribe_events`
   - (Future) `accumulate_list_snapshots`

4. **Implement Resource Extensions**
   - Add `accumulate://account/{url}` resource
   - Add `accumulate://transaction/{txid}` resource
   - Add `accumulate://chain/{url}/{name}` resource
   - Keep existing `wallet://` resources

5. **Security Enhancement**
   - Add `--secure-mode` flag
   - In secure mode: disable private_key parameters
   - Add separate build/submit tools
   - Document security model differences

6. **Configuration Unification**
   - Support both env vars AND config file
   - Add caching configuration
   - Add read-only mode
   - Add feature flags

### Long-term Enhancements

1. **Event Subscriptions** (Phase 2)
   - WebSocket support
   - Event streaming
   - Subscription management

2. **Advanced Caching** (Phase 2)
   - Redis support
   - Configurable TTLs
   - Cache invalidation

3. **Connection Pooling** (Phase 2)
   - Multiple endpoint support
   - Load balancing
   - Failover handling

4. **Metrics & Monitoring** (Phase 3)
   - Prometheus metrics
   - Request tracking
   - Performance monitoring

## Conclusion

The existing `mcp-accumulate` implementation is **production-ready and feature-complete** with 40 tools covering all major use cases. It includes innovative wallet integration not present in the original design spec.

The design specification in `docs/mcp/` provides:
- Comprehensive documentation
- Security-focused architecture
- Additional resource types
- Implementation best practices

**Best Path Forward:**
1. Merge `mcp-accumulate` into `accumulate/tools/mcp-server`
2. Add missing tools from design spec (4 tools)
3. Implement `accumulate://` resources (6 resource types)
4. Add security mode for build/submit separation
5. Enhance configuration with caching and feature flags
6. Maintain both practical (wallet-integrated) and secure (keyless) modes

This gives users the best of both worlds:
- **Developers:** Integrated wallet, convenience tools, rapid development
- **Security-conscious:** Keyless operation, separate build/submit, external signing
- **All users:** Comprehensive documentation, battle-tested code, full API coverage

## Cross-Reference

| Design Doc | Existing Impl | Status |
|------------|---------------|--------|
| [mcp-server-design.md](./mcp-server-design.md) | mcp-accumulate/readme.md | ✅ Alignment needed |
| [api-mapping-reference.md](./api-mapping-reference.md) | mcp-accumulate/server/tools.go | ✅ Implemented |
| [implementation-guide.md](./implementation-guide.md) | mcp-accumulate/ | ✅ Already done! |
| [QUICKSTART.md](./QUICKSTART.md) | mcp-accumulate/readme.md | ✅ Working code |

## Version History

- **v1.0** (2025-10-20): Initial analysis
  - Analyzed existing mcp-accumulate implementation
  - Compared with design specification
  - Identified gaps and opportunities
  - Recommended integration strategy
