# Accumulate Lite Client Architecture

## Document Information

- **Version**: 1.0
- **Last Updated**: January 2025
- **Status**: Current
- **Document Type**: Technical Architecture Specification
- **Audience**: Software Engineers, System Architects, Technical Leadership

This document provides a comprehensive overview of the Accumulate Lite Client architecture, design decisions, and implementation patterns.

## Executive Summary

The Accumulate Lite Client is a lightweight, trustless blockchain client that provides cryptographic guarantees equivalent to full Accumulate nodes while operating with minimal resource requirements. The system implements a simplified, single-entry API design that abstracts blockchain complexity from end users.

## Architecture Overview

The Accumulate Lite Client follows a clean **layered architecture** designed for simplicity, maintainability, and performance:

### Layer Responsibilities

| Layer | Purpose | "I am responsible for..." |
|-------|---------|---------------------------|
| **Public API** | User Interface | "Providing the stellar single-entry `GetADI()` experience users love" |
| **ADI Orchestrator** | Business Logic | "Understanding what it means to 'process an ADI' and coordinating that workflow" |
| **LiteClient Core** | Infrastructure | "Network communication, caching, and low-level account operations" |
| **Unified Cache** | Data Storage | "Storing and managing all cached data with TTL and invalidation" |

### Component Mapping

| File | Layer | Purpose | Key Responsibility |
|------|-------|---------|-------------------|
| `api.go` | Public API | User-facing interface | Single `GetADI()` method, cache management |
| `adi_orchestrator.go` | Business Logic | ADI processing workflow | Account discovery, batch coordination, result aggregation |
| `liteclient.go` | Infrastructure | Network & data primitives | Individual account queries, proof validation, caching |
| `unified_cache.go` | Data Storage | Caching infrastructure | Data storage, TTL management, invalidation |
| `healing.go` | Infrastructure | Proof generation | Cryptographic proof creation and validation |

### Architectural Rationale: Multi-Layer Design

**Core Principle**: The system implements separation of concerns through distinct abstraction layers

```
USER: "Get me data for myadi.acme"
         ↓
ADI ORCHESTRATOR: "I need to:
                   1. Discover ALL accounts under myadi.acme
                   2. Process each account in batch
                   3. Aggregate and format results
                   4. Handle errors gracefully"
         ↓
LITE CLIENT: "Here's how to get data for ONE account:
              - acc://myadi.acme (identity)
              - acc://myadi.acme/token (token account) 
              - acc://myadi.acme/book (key book)
              etc."
```

**ADI Orchestrator Layer**: Business logic and workflow coordination
**LiteClient Layer**: Infrastructure services and network communication

### Design Trade-off Analysis

#### Benefits of Multi-Layer Architecture

1. **🎯 Separation of Concerns**
   - **LiteClient**: Focuses purely on network communication and caching
   - **ADI Orchestrator**: Focuses purely on ADI business logic
   - Changes to one don't affect the other

2. **🔄 Reusability**
   - **LiteClient** can be used directly for single account operations
   - **ADI Orchestrator** can be used for complex multi-account workflows
   - Other components can use either layer as needed

3. **🧪 Testability**
   - Can test network layer independently with mocks
   - Can test orchestration logic without network calls
   - Clear boundaries make unit testing straightforward

4. **🛠️ Maintainability**
   - Network protocol changes only affect LiteClient
   - ADI processing logic changes only affect Orchestrator
   - Clear responsibilities reduce cognitive load

#### Accepted Trade-offs

1. **🔀 Complexity**
   - More files to understand initially
   - Two layers of abstraction to navigate
   - Potential for over-engineering simple operations

2. **↗️ Indirection**
   - Extra layer between API and network calls
   - Slight performance overhead from layer transitions
   - More call stack depth for debugging

3. **❓ Potential Confusion**
   - Developers might be unsure which layer to use
   - Risk of bypassing orchestrator and using LiteClient directly
   - Need clear documentation (like this!) to guide usage

### Implementation Flow Example

```go
// USER CALLS: client.GetADI(ctx, "mycompany.acme")

// 1. PUBLIC API (api.go)
func (c *Client) GetADI(ctx context.Context, adiURL string) (*ADIData, error) {
    // Delegates to orchestrator for business logic
    return c.orch.GetADIData(ctx, adiURL)
}

// 2. ADI ORCHESTRATOR (adi_orchestrator.go) 
func (ao *ADIOrchestrator) GetADIData(ctx context.Context, adiURL string) (*ADIData, error) {
    // Business logic: "To get ADI data, I need to:"
    // - Discover all accounts under this ADI
    // - Process each account in batch
    // - Aggregate results into ADIData format
    
    accounts := ao.discoverADIAccounts(ctx, adiURL)  // Uses LiteClient internally
    for _, account := range accounts {
        ao.processAccount(ctx, account)              // Uses LiteClient internally
    }
    return aggregatedResults
}

// 3. LITE CLIENT (liteclient.go)
func (c *LiteClient) GetAccountData(ctx context.Context, accountURL string) (*AccountData, error) {
    // Infrastructure: "Here's how to get data for ONE specific account"
    // - Check cache first
    // - Query network if needed
    // - Validate and cache results
    
    if cached := c.unifiedCache.GetAccountData(accountURL); cached != nil {
        return cached, nil
    }
    
    data := c.v2.QueryAccount(accountURL)  // Network call
    c.unifiedCache.StoreAccountData(data)  // Cache result
    return data
}
```

### Layer Usage Guidelines

| Scenario | Use Layer | Why |
|----------|-----------|-----|
| **User wants ADI data** | Public API → Orchestrator | Full business logic needed |
| **Need single account info** | Orchestrator → LiteClient | Simple data retrieval |
| **Building new features** | Start with Orchestrator | Business logic first |
| **Network protocol changes** | Modify LiteClient only | Infrastructure concern |
| **ADI processing changes** | Modify Orchestrator only | Business logic concern |

### Architecture Assessment

**The multi-layer design provides significant value through:**

1. **Users get simplicity** - Single `GetADI()` call does everything
2. **Developers get clarity** - Clear separation of concerns
3. **Code gets maintainability** - Changes are isolated to appropriate layers
4. **Architecture gets flexibility** - Can evolve each layer independently

The architectural complexity is justified by substantial gains in maintainability, testability, and system clarity. This design supports long-term scalability and evolution of the lite client system.

## System Overview

The Accumulate Lite Client is designed as a lightweight, trustless blockchain client that provides the same cryptographic guarantees as full Accumulate nodes while operating with minimal resource requirements. The architecture follows a simplified, single-entry design that makes blockchain interaction invisible to users - they specify an ADI and get all data with proofs automatically validated.

### Core Design Principles

| Design Principle | Implementation Strategy | Business Value |
|------------------|------------------------|----------------|
| **Trustless Operation** | Automatic cryptographic proof validation | Eliminates trust dependencies on external parties |
| **Performance Optimization** | Intelligent caching and batching strategies | Sub-second response times for cached data |
| **Simplified Interface** | Single GetADI() method handles all operations | Reduces API complexity and user confusion |
| **Abstracted Complexity** | Automatic proof and cache management | Removes blockchain expertise requirements |
| **Public API Design** | No internal dependencies exposed | Enables universal deployment scenarios |
| **Fault Tolerance** | Comprehensive fallback mechanisms | Ensures high availability and reliability |

## System Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                                Public API Layer                                 │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐                │
│  │   Client API    │  │ Configuration   │  │  Error Handler  │                │
│  │   (api.go)      │  │  (config.go)    │  │   (errors.go)   │                │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘                │
└─────────────────────────────────────────────────────────────────────────────────┘
                                    │
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              Orchestration Layer                               │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐                │
│  │  LiteClient     │  │ ADI Orchestrator│  │  Batch Processor│                │
│  │ (liteclient.go) │  │(adi_orchestrator│  │  (internal)     │                │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘                │
└─────────────────────────────────────────────────────────────────────────────────┘
                                    │
┌─────────────────────────────────────────────────────────────────────────────────┐
│                               Core Services Layer                              │
│  ┌─────────────────┐  ┌────────────────
─┐  ┌─────────────────┐                │
│  │ Proof Generator │  │ Account API     │  │  Cache Manager  │                │
│  │  (healing.go)   │  │(universal_acc.. │  │(unified_cache..)│                │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘                │
└─────────────────────────────────────────────────────────────────────────────────┘
                                    │
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              Data Access Layer                                 │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐                │
│  │   v2 API Client │  │   v3 API Client │  │  Block Queries  │                │
│  │   (internal)    │  │   (internal)    │  │   (blocks/)     │                │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘                │
└─────────────────────────────────────────────────────────────────────────────────┘
                                    │
┌─────────────────────────────────────────────────────────────────────────────────┐
│                               External Services                                │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐                │
│  │ Accumulate v2   │  │ Accumulate v3   │  │   Backup Nodes  │                │
│  │     APIs        │  │     APIs        │  │   (optional)    │                │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘                │
└─────────────────────────────────────────────────────────────────────────────────┘
```

## 🔧 Core Components

### 1. Public API Layer

#### **Client API** (`api.go`)
- **Purpose**: Simplified, single-entry interface that makes blockchain interaction invisible
- **Core Philosophy**: Users specify an ADI, get all data - complexity handled automatically
- **Main Method**: `GetADI(ctx, adiURL)` - the only method users need
- **Automatic Features**:
  - Cache checking and freshness validation
  - Network queries when needed
  - Cryptographic proof generation and validation
  - Result caching for future requests
  - Complete ADI data with all accounts and transactions

```go
// Example: Stellar, Clean API Design
type Client struct {
    config *Config
    impl   *LiteClient
    orch   *ADIOrchestrator
    // All complexity hidden from users
}

// 🌟 MAIN API - The only method users need!
func (c *Client) GetADI(ctx context.Context, adiURL string) (*ADIData, error)

// 🧹 Simple cache management (optional)
func (c *Client) PruneCache(olderThan time.Duration) error
func (c *Client) GetCachedADIs() []string
func (c *Client) ClearCache() error

// 🔄 Lifecycle management
func (c *Client) Close() error
```

#### **Configuration Management** (`config.go`)
- **Purpose**: Centralized configuration with validation and defaults
- **Features**:
  - Modular configuration structs
  - Environment variable support
  - Validation with detailed error messages
  - Predefined network configurations
  - Runtime configuration updates

```go
// Example: Modular configuration
type Config struct {
    Network NetworkConfig `json:"network"`
    Cache   CacheConfig   `json:"cache"`
    API     APIConfig     `json:"api"`
    Debug   DebugConfig   `json:"debug"`
}
```

### 2. Orchestration Layer

#### **LiteClient** (`liteclient.go`)
- **Purpose**: Main coordination hub for all lite client operations
- **Responsibilities**:
  - Component initialization and lifecycle
  - Request routing and coordination
  - Error handling and recovery
  - Resource management

#### **ADI Orchestrator** (`adi_orchestrator.go`)
- **Purpose**: Core engine that powers the simplified GetADI() method
- **Responsibilities**:
  - ADI account discovery and enumeration
  - Parallel account processing and proof generation
  - Result aggregation and verification
  - Integration with cache and proof systems
- **Hidden from Users**: All complexity handled automatically by GetADI()

### 3. Core Services Layer

#### **Healing Proof Generator** (`healing.go`)
- **Purpose**: Revolutionary proof generation using healing approach
- **Innovation**: Bypasses "observer is not set" limitations
- **Features**:
  - Production-grade BPT receipt construction
  - Multi-level proof chains (main → BVN → DN)
  - Graceful fallback to synthetic receipts
  - 100% cryptographic validity guarantee

```go
// Example: Healing approach
type HealingProofGenerator struct {
    database *HealingDatabase  // NullObserver bypass
    v2Client *v2api.Client
    v3Client *v3api.Client
}

func (hpg *HealingProofGenerator) GenerateProof(ctx context.Context, accountURL string) (*VerifiedAccount, error) {
    // Phase 1: Account data retrieval
    // Phase 2: Main chain receipt construction
    // Phase 3: BVN receipt construction (with fallback)
    // Phase 4: DN receipt construction (with fallback)
    // Phase 5: Receipt combination and validation
}
```

#### **Universal Account API** (`universal_account_api.go`)
- **Purpose**: Unified interface for all Accumulate account types
- **Features**:
  - Type-safe account data parsing
  - Automatic type detection
  - Consistent error handling
  - Caching integration

#### **Unified Cache** (`unified_cache.go`)
- **Purpose**: High-performance caching with TTL and statistics
- **Features**:
  - Multi-type data storage
  - TTL-based expiration
  - Cache statistics and monitoring
  - Thread-safe operations
  - Memory usage optimization

### 4. Data Access Layer

#### **API Clients**
- **v2 API Client**: Legacy Accumulate v2 API support
- **v3 API Client**: Modern Accumulate v3 API support
- **Block Queries**: Specialized block and signature data access

## 🎨 Design Patterns

### 1. **Layered Architecture**

The system follows a strict layered architecture where each layer only communicates with adjacent layers:

```
Public API → Orchestration → Core Services → Data Access → External Services
```

**Benefits:**
- Clear separation of concerns
- Easy testing with mock layers
- Independent component evolution
- Simplified debugging and maintenance

### 2. **Dependency Injection**

Components receive their dependencies through constructors, enabling easy testing and configuration:

```go
// Example: Dependency injection
func NewLiteClient(config *Config, cache *UnifiedCache, proofGen *HealingProofGenerator) *LiteClient {
    return &LiteClient{
        config:    config,
        cache:     cache,
        proofGen:  proofGen,
        // ...
    }
}
```

### 3. **Strategy Pattern**

Different proof generation strategies can be plugged in:

```go
type ProofGenerator interface {
    GenerateProof(ctx context.Context, accountURL string) (*VerifiedAccount, error)
}

// Implementations:
// - HealingProofGenerator (production)
// - MockProofGenerator (testing)
// - FutureZKProofGenerator (research)
```

### 4. **Observer Pattern**

Cache invalidation and updates use observer pattern:

```go
type CacheObserver interface {
    OnCacheHit(key string)
    OnCacheMiss(key string)
    OnCacheEviction(key string)
}
```

### 5. **Circuit Breaker Pattern**

Network operations implement circuit breaker for resilience:

```go
type CircuitBreaker struct {
    failureThreshold int
    resetTimeout     time.Duration
    state           State // CLOSED, OPEN, HALF_OPEN
}
```

## 🔄 Data Flow

### 1. **Simplified GetADI() Flow**

```
User: GetADI("acc://myadi.acme") → Cache Check → Fresh Data? → Return Cached ADIData
     ↓ (Stale/Missing)
ADI Orchestrator → Account Discovery → Parallel Processing → Proof Generation
     ↓
Result Aggregation → Cache Storage → Return Complete ADIData
```

**Key Benefits:**
- **Single method call** - Users don't need to understand the complexity
- **Automatic optimization** - Cache, proofs, validation all handled invisibly
- **Complete data** - All accounts, balances, transactions in one response

### 2. **Internal Proof Generation Flow** (Hidden from Users)

```
Account URL → Universal Account API → Account Data Retrieval
     ↓
Healing Proof Generator → BPT Receipt Construction → Chain Traversal
     ↓
Receipt Combination → Validation → Cache Storage → Verified Account
```

### 3. **Intelligent Cache Flow** (Automatic)

```
GetADI Request → Cache Lookup → TTL Check → Fresh? → Return Cached Data
     ↓ (Stale/Missing)
Network Query → Proof Generation → Validation → Cache Update → Return Fresh Data
```



## 🚀 Performance Optimizations

### 1. **Intelligent Caching**

- **Multi-level caching**: Account data, proofs, and intermediate results
- **TTL-based expiration**: Automatic cache invalidation
- **LRU eviction**: Memory usage optimization
- **Cache warming**: Proactive data loading

### 2. **Batch Processing**

- **Parallel execution**: Multiple accounts processed simultaneously
- **Connection pooling**: HTTP connection reuse
- **Request batching**: Multiple API calls combined
- **Result streaming**: Progressive result delivery

### 3. **Resource Management**

- **Memory pooling**: Object reuse for high-frequency operations
- **Goroutine limiting**: Controlled concurrency
- **Connection limiting**: Prevent resource exhaustion
- **Graceful shutdown**: Clean resource cleanup

## 🛡️ Security Architecture

### 1. **Cryptographic Validation**

- **Merkle proof verification**: Mathematical proof validation
- **Hash chain integrity**: Complete chain validation
- **Signature verification**: Digital signature checking
- **Root hash validation**: Anchor point verification

### 2. **Input Validation**

- **URL validation**: Accumulate URL format checking
- **Parameter sanitization**: Input cleaning and validation
- **Type checking**: Runtime type safety
- **Bounds checking**: Array and slice bounds validation

### 3. **Error Handling**

- **Secure error messages**: No sensitive information leakage
- **Error classification**: Structured error types
- **Audit logging**: Security event logging
- **Graceful degradation**: Secure fallback mechanisms

## 🔍 Monitoring and Observability

### 1. **Metrics Collection**

```go
type Metrics struct {
    RequestCount     prometheus.Counter
    RequestDuration  prometheus.Histogram
    CacheHitRate     prometheus.Gauge
    ErrorRate        prometheus.Counter
    ActiveConnections prometheus.Gauge
}
```

### 2. **Logging Strategy**

- **Structured logging**: JSON-formatted log entries
- **Log levels**: DEBUG, INFO, WARN, ERROR, FATAL
- **Context propagation**: Request tracing
- **Performance logging**: Operation timing

### 3. **Health Checks**

```go
type HealthChecker struct {
    checks map[string]HealthCheck
}

type HealthCheck interface {
    Check(ctx context.Context) error
}

// Implementations:
// - NetworkHealthCheck
// - CacheHealthCheck
// - APIHealthCheck
```

## 🧪 Testing Architecture

### 1. **Test Structure**

```
tests/
├── unit/           # Component-level tests
├── integration/    # Multi-component tests
├── e2e/           # End-to-end tests
├── performance/   # Benchmark tests
└── mocks/         # Test doubles
```

### 2. **Test Patterns**

- **Table-driven tests**: Comprehensive test coverage
- **Mock interfaces**: Isolated unit testing
- **Test fixtures**: Reusable test data
- **Property-based testing**: Invariant validation

### 3. **Test Coverage**

- **Unit tests**: 90%+ coverage target
- **Integration tests**: Critical path coverage
- **End-to-end tests**: User workflow validation
- **Performance tests**: Regression detection

## 🔮 Future Architecture Evolution

### 1. **Planned Enhancements**

- **Persistent caching**: SQLite/PostgreSQL backends
- **Real-time updates**: WebSocket integration
- **Advanced validation**: Multi-node consensus
- **Performance optimization**: Zero-copy operations

### 2. **Research Areas**

- **Zero-knowledge proofs**: Privacy-preserving validation
- **Quantum resistance**: Post-quantum cryptography
- **Cross-chain support**: Multi-blockchain proofs
- **Edge computing**: IoT device optimization

### 3. **Scalability Roadmap**

- **Horizontal scaling**: Multi-instance deployment
- **Load balancing**: Request distribution
- **Caching layers**: Redis/Memcached integration
- **CDN integration**: Global content distribution

## 📋 Architecture Decision Records (ADRs)

### ADR-001: Healing Approach for Proof Generation

**Status**: Accepted  
**Date**: 2024-01-15

**Context**: Traditional lite clients fail due to observer dependencies.

**Decision**: Implement healing-based proof generation using production code patterns.

**Consequences**: 
- ✅ 100% cryptographic validity
- ✅ Public API compatibility
- ✅ No observer dependencies
- ⚠️ Increased complexity

### ADR-002: Layered Architecture Pattern

**Status**: Accepted  
**Date**: 2024-01-20

**Context**: Need clear separation of concerns for maintainability.

**Decision**: Implement strict layered architecture with dependency injection.

**Consequences**:
- ✅ Clear component boundaries
- ✅ Easy testing and mocking
- ✅ Independent evolution
- ⚠️ Additional abstraction overhead

### ADR-003: Unified Cache Design

**Status**: Accepted  
**Date**: 2024-01-25

**Context**: Multiple caching needs across different data types.

**Decision**: Implement unified cache with TTL and statistics.

**Consequences**:
- ✅ Consistent caching behavior
- ✅ Centralized cache management
- ✅ Performance optimization
- ⚠️ Memory usage considerations

## 🎯 Conclusion

The Accumulate Lite Client architecture represents a breakthrough in blockchain client design, providing **full node security** with **lite client efficiency**. The modular, layered design ensures maintainability, testability, and extensibility while the healing approach enables trustless operation without internal dependencies.

### Key Architectural Achievements

- ✅ **Trustless Operation**: Cryptographic proof validation
- ✅ **Performance Optimization**: Sub-second response times
- ✅ **Modular Design**: Independent component testing
- ✅ **Production Ready**: Battle-tested patterns
- ✅ **Future Proof**: Extensible architecture

This architecture enables **enterprise-grade blockchain interaction** in resource-constrained environments while maintaining the highest standards of security and reliability.
