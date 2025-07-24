# Lite Client Implementation Plan

## Overview

This document outlines the implementation plan for the Accumulate Lite Client, a trustless caching system for verified account states.

## Project Goals

### Primary Objectives

1. **Trustless Caching of Account State**
   - Maintain a local cache of verified account states (balances, transactions, proofs)
   - Serve requests from cache when data is fresh
   - Fetch, validate, and cache new data when needed

2. **Protocol Compliance**
   - Validate Merkle Receipts against operator-signed root hashes
   - Ensure every cached datum is backed by a valid proof

3. **Extensibility & Usability**
   - Support all account types (Lite Token Accounts, ADIs, KeyBooks, KeyPages)
   - Provide CLI and web interface for inspection and cache management

## Implementation Phases

### Phase 1: Receipt Retrieval & Validation ✅ COMPLETED
- Implement `GetAccountReceipt(accountURL)`
- Validate Merkle proof locally
- **Status**: Done

### Phase 2: Generic Account Retrieval & Caching 🔄 IN PROGRESS
- Extend retrieval from Lite Token Accounts to any account type
- Cache account data (URL, balance, transaction history)
- **Target**: August 2025

### Phase 3: Cache Pruning & Eviction ⏳ PLANNED
- Design and implement eviction policy (LRU or TTL)
- Expose pruning commands via CLI
- **Target**: August 2025

### Phase 4: Root Hash Authenticity 🚧 BLOCKED
- Integrate root-hash lookup and signature verification
- **Dependency**: Waiting for specification document

### Phase 5: CLI & Web Interface 
- Minimal CLI commands (`fetch`, `show`, `prune`)
- Basic web dashboard showing cache status

## Architecture Overview

The lite client follows a modular architecture with clear separation of concerns:

### Core Components

#### CacheManager
- **Purpose**: Local storage and retrieval of verified account data
- **Storage Backend**: Embedded key-value database (Badger or LevelDB)
- **API Methods**:
  - `Get(accountURL) → (AccountData, Proof)`
  - `Put(accountURL, AccountData, Proof)`
  - `Prune(...)`


#### RPC Client
- **Purpose**: Interface with Accumulate network
- **Capabilities**:
  - Fetch account receipts: `GetAccountReceipt(accountURL)`
  - Fetch account data: `GetAccountData(accountURL)`
- **Implementation**: Wraps Accumulate Go SDK or REST endpoints


#### ProofValidator
- **Purpose**: Validate Merkle receipt authenticity
- **Process**:
  1. Reconstruct Merkle path from receipt
  2. Verify inclusion in state root
  3. Pass root hash to RootVerifier


#### RootVerifier
- **Purpose**: Verify root hash signatures
- **Function**: Validate that root hash is signed by operator keybook
- **Status**: Pending specification document


#### Interface Layer
- **CLI**: Command-line interface (`fetch`, `show`, `prune` commands)
- **Web**: HTTP server with JSON API and HTML dashboard


---

## Implementation Details

- **Language & Tooling**  
  - **Go** (>= 1.19)  
  - **Modules**:  
    - `github.com/accumulatenetwork/accumulate/client`  
    - `github.com/accumulatenetwork/accumulate/protocol`  
    - `github.com/dgraph-io/badger/v3` (for embedded storage)

### Key Interfaces

```go
type LiteClient struct {
    rpc       *client.Client
    cache     CacheManager
    validator ProofValidator
    rootVer   RootVerifier
}

func (lc *LiteClient) GetAccount(ctx context.Context, url string) (*AccountData, error) {
    if data, ok := lc.cache.Get(url); ok {
        return data, nil
    }
    receipt, err := lc.rpc.GetAccountReceipt(ctx, url)
    if err != nil {
        return nil, err
    }
    if err := lc.validator.Validate(receipt); err != nil {
        return nil, err
    }
    // root verification deferred to Phase 4
    acctData, err := lc.rpc.GetAccountData(ctx, url)
    if err != nil {
        return nil, err
    }
    lc.cache.Put(url, acctData, receipt)
    return acctData, nil
}
```

### Testing Strategy

1. **Unit Tests** (Go `testing` + `testify`):
   - Feed known valid/invalid receipts to `ProofValidator`
   - Mock `rpc` to return edge-case payloads
   - Test individual component functionality

2. **Integration Tests**:
   - Use local testnet node or public Testnet
   - Create test accounts (LTA, ADI, KeyBook)
   - Verify end-to-end behavior of `LiteClient.GetAccount`

3. **Coverage Report**:
   - Generate `go test -coverprofile=coverage.out`
   - Target ≥ 80% coverage on core packages

### Cache Pruning Design

**Policy**: LRU with configurable maximum size or TTL-based expiration

**API Interface**:
```go
type CacheManager interface {
    Get(key string) (*AccountData, bool)
    Put(key string, data *AccountData, receipt *Receipt)
    Prune(maxEntries int) error
}
```

**CLI Commands**:
- `lite prune --max=1000`
- `lite prune --older-than=24h`

## Development Schedule

### Current Milestones

| Phase | Milestone | Target Date | Status | Notes |
|-------|-----------|-------------|--------|-------|
| 2 | Generic Account Caching MVP | August 2025 | 🔄 In Progress | ADI, KeyBook, KeyPage support |
| 3 | Cache Pruning Implementation | August 2025 | ⏳ Planned | LRU/TTL policy + CLI commands |
| 4 | Root Hash Spec Review | TBD | 🚧 Blocked | Waiting for specification |
| 5 | CLI MVP Release | September 2025 | 🔜 Planned | Core commands implementation |
| 5 | Web Dashboard Prototype | September 2025 | 🔜 Planned | Basic HTML + JSON API |

### Immediate Next Steps

1. **Complete Phase 2**: Expand retrieval logic for all account types
2. **Design Phase 3**: Implement cache pruning with LRU/TTL policies
3. **Coordinate with Paul**: Review root-hash specification to unblock Phase 4
4. **Prepare CLI**: Design command-line interface for cache management
5. **Testing**: Maintain comprehensive test coverage throughout development

