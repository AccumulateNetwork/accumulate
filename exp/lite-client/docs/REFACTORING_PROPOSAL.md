# Architecture Refactoring Proposal

## Current Issues

### 1. Cross-Layer Dependencies
- `account_data.go` (Core Services) directly uses `LiteClient` struct (Orchestration Layer)
- Multiple v2 client instances scattered across layers
- Tight coupling between layers violates clean architecture principles

### 2. Scattered v2 Client Usage
```
liteclient.go:29    v2           *v2.Client
receipt.go:21       client *v2api.Client  
account_data.go:96  c.v2.Query(ctx, query)
```

### 3. Layer Violations
- Core Services Layer accessing Orchestration Layer directly
- Data Access Layer logic mixed with Core Services
- No clear interface boundaries between layers

## Proposed Refactoring

### 1. Centralized Network Client (Data Access Layer)

Create a single `NetworkClient` that encapsulates all v2/v3 API access:

```go
// network_client.go
type NetworkClient struct {
    v2 *v2.Client
    v3 *jsonrpc.Client
}

type NetworkInterface interface {
    QueryAccount(ctx context.Context, url string) (map[string]interface{}, error)
    QueryChain(ctx context.Context, url string, options ChainQueryOptions) (*ChainResponse, error)
    QueryStatus(ctx context.Context, partition string) (*StatusResponse, error)
}
```

### 2. Clean Core Services Layer

Remove LiteClient dependencies from core services:

```go
// account_service.go
type AccountService struct {
    network NetworkInterface
    cache   CacheInterface
}

// proof_service.go  
type ProofService struct {
    network NetworkInterface
    cache   CacheInterface
}

// cache_service.go
type CacheService struct {
    storage map[string]interface{}
    ttl     time.Duration
}
```

### 3. Interface-Based Orchestration Layer

Make orchestration depend only on interfaces:

```go
// liteclient.go
type LiteClient struct {
    accountService AccountServiceInterface
    proofService   ProofServiceInterface
    cacheService   CacheServiceInterface
}

// adi_orchestrator.go
type ADIOrchestrator struct {
    client LiteClientInterface
}
```

### 4. Dependency Injection

Use constructor injection to wire dependencies:

```go
func NewLiteClient(server string) (*LiteClient, error) {
    // Data Access Layer
    networkClient := NewNetworkClient(server)
    
    // Core Services Layer
    cacheService := NewCacheService(5 * time.Minute)
    accountService := NewAccountService(networkClient, cacheService)
    proofService := NewProofService(networkClient, cacheService)
    
    // Orchestration Layer
    return &LiteClient{
        accountService: accountService,
        proofService:   proofService,
        cacheService:   cacheService,
    }, nil
}
```

## Benefits

### 1. Clean Layer Separation
- Each layer depends only on interfaces from lower layers
- No circular dependencies
- Clear responsibility boundaries

### 2. Single Network Client
- One v2 client instance per LiteClient
- Centralized network configuration
- Easier testing and mocking

### 3. Testability
- Each service can be tested in isolation
- Mock interfaces for unit testing
- No need for real network calls in unit tests

### 4. Maintainability
- Changes to network layer don't affect core services
- Easy to swap implementations
- Clear code organization

## Implementation Plan

### Phase 1: Create Interfaces
1. Define `NetworkInterface`, `AccountServiceInterface`, etc.
2. Create interface files in each layer

### Phase 2: Implement Data Access Layer
1. Create `NetworkClient` with all v2/v3 logic
2. Move network-specific code from other files

### Phase 3: Refactor Core Services
1. Remove `LiteClient` dependencies from `account_data.go`
2. Create `AccountService`, `ProofService`, `CacheService`
3. Make them depend only on interfaces

### Phase 4: Update Orchestration Layer
1. Refactor `LiteClient` to use service interfaces
2. Update `ADIOrchestrator` to use `LiteClientInterface`

### Phase 5: Update Public API
1. Ensure `api.go` still works with refactored orchestration
2. Maintain backward compatibility

## File Structure After Refactoring

```
exp/lite-client/
├── interfaces/
│   ├── network.go          # NetworkInterface
│   ├── account_service.go  # AccountServiceInterface  
│   ├── proof_service.go    # ProofServiceInterface
│   └── cache_service.go    # CacheServiceInterface
├── data_access/
│   └── network_client.go   # NetworkClient implementation
├── core_services/
│   ├── account_service.go  # AccountService implementation
│   ├── proof_service.go    # ProofService implementation
│   └── cache_service.go    # CacheService implementation
├── orchestration/
│   ├── liteclient.go       # LiteClient (orchestrator)
│   └── adi_orchestrator.go # ADIOrchestrator
└── api/
    └── client.go           # Public API
```

This refactoring will create a clean, maintainable architecture with proper separation of concerns and minimal cross-layer dependencies.
