# Accumulate Network API - Source Code File Reference

## Directory Structure Overview

```
accumulate/
├── pkg/
│   ├── api/
│   │   ├── v3/                          # V3 API (Current)
│   │   │   ├── api.go                   # Core API interface definitions
│   │   │   ├── query.go                 # Query implementation helpers
│   │   │   ├── querier.go               # Querier2 helper methods
│   │   │   ├── queries.yml              # Query type definitions
│   │   │   ├── responses.yml            # Response type definitions
│   │   │   ├── records.yml              # Record type definitions
│   │   │   ├── options.yml              # Options type definitions
│   │   │   ├── events.yml               # Event type definitions
│   │   │   ├── enums.yml                # Enumeration definitions
│   │   │   ├── types.yml                # General type definitions
│   │   │   ├── types_gen.go             # Generated type code
│   │   │   ├── enums_gen.go             # Generated enum code
│   │   │   ├── unions_gen.go            # Generated union code
│   │   │   ├── jsonrpc/
│   │   │   │   ├── handler.go           # JSON-RPC handler factory
│   │   │   │   ├── services.go          # JSON-RPC service wrappers
│   │   │   │   └── client.go            # JSON-RPC client
│   │   │   ├── rest/
│   │   │   │   ├── services.go          # REST endpoint handlers
│   │   │   │   └── query.go             # REST query handler
│   │   │   ├── websocket/
│   │   │   │   ├── handler.go           # WebSocket connection handler
│   │   │   │   ├── client.go            # WebSocket client
│   │   │   │   ├── types.go             # WebSocket message types
│   │   │   │   └── types.yml            # WebSocket type definitions
│   │   │   ├── message/
│   │   │   │   ├── services.go          # Binary message service wrappers
│   │   │   │   ├── handler.go           # Binary message handler
│   │   │   │   ├── client.go            # Binary message client
│   │   │   │   ├── messages.yml         # Message type definitions
│   │   │   │   ├── types.go             # Message types
│   │   │   │   ├── types_gen.go         # Generated message types
│   │   │   │   ├── enums_gen.go         # Generated message enums
│   │   │   │   └── unions_gen.go        # Generated message unions
│   │   │   ├── p2p/
│   │   │   │   ├── services.go          # P2P node service impl
│   │   │   │   ├── p2p.go               # P2P node core
│   │   │   │   ├── client.go            # P2P client
│   │   │   │   ├── types.go             # P2P types
│   │   │   │   ├── discovery.go         # Peer discovery
│   │   │   │   ├── peer_manager.go      # Peer management
│   │   │   │   ├── types.yml            # P2P type definitions
│   │   │   │   ├── enums.yml            # P2P enum definitions
│   │   │   │   └── peerdb/              # Peer database
│   │   │   │       ├── db.go
│   │   │   │       ├── types.go
│   │   │   │       ├── atomic.go
│   │   │   │       └── types.yml
│   │   │   └── openapi.yml              # OpenAPI specification
│   │   ├── ethereum/                    # Ethereum RPC API
│   │   │   ├── services.go              # Ethereum service interface
│   │   │   ├── jsonrpc.go               # Ethereum JSON-RPC impl
│   │   │   ├── types.go                 # Ethereum types
│   │   │   ├── schema.yml               # Ethereum schema
│   │   │   └── types_gen.go             # Generated types
│   │   └── v2/                          # Legacy types (moved to internal)
│   └── client/
│       └── api/
│           └── v2/
│               ├── client.go            # V2 client
│               └── api_types.go         # V2 types
│
├── internal/
│   ├── api/
│   │   ├── v2/                          # V2 API (Legacy)
│   │   │   ├── api.go                   # API core
│   │   │   ├── jrpc.go                  # JSON-RPC handler
│   │   │   ├── jrpc_execute.go          # Execute methods
│   │   │   ├── jrpc_metrics.go          # Metrics methods
│   │   │   ├── query_jrpc.go            # Query methods
│   │   │   ├── query_v3.go              # V3 query wrapper
│   │   │   ├── error.go                 # Error handling
│   │   │   ├── methods.yml              # Method definitions
│   │   │   ├── types.yml                # Type definitions
│   │   │   ├── responses.yml            # Response definitions
│   │   │   ├── enums.yml                # Enum definitions
│   │   │   └── types_gen.go             # Generated types
│   │   └── private/
│   │       ├── api.go                   # Private API interfaces
│   │       └── types_gen.go             # Generated types
│   └── interfaces/
│       └── api/
│           └── (interface definitions)
```

## Key Files by Component

### V3 API Definition Files (YAML)

**Location:** `/pkg/api/v3/`

| File | Purpose | Contents |
|------|---------|----------|
| `queries.yml` | Query type definitions | DefaultQuery, ChainQuery, DataQuery, BlockQuery, SearchQueries |
| `records.yml` | Response record types | AccountRecord, ChainRecord, MessageRecord, BlockRecords, etc |
| `responses.yml` | Response structures | NodeInfo, ConsensusStatus, NetworkStatus, Submission, etc |
| `options.yml` | Request options | RangeOptions, ReceiptOptions, SubmitOptions, etc |
| `events.yml` | Event types | ErrorEvent, BlockEvent, GlobalsEvent |
| `enums.yml` | Enumerations | ServiceType, QueryType, RecordType, EventType, KnownPeerStatus |
| `types.yml` | General types | Common structures and helpers |
| `openapi.yml` | OpenAPI spec | API documentation spec |

### V3 Service Implementations

**Location:** `/pkg/api/v3/`

| Transport | Files | Purpose |
|-----------|-------|---------|
| JSON-RPC | `jsonrpc/handler.go`, `jsonrpc/services.go` | HTTP JSON-RPC 2.0 |
| REST | `rest/services.go`, `rest/query.go` | HTTP REST endpoints |
| WebSocket | `websocket/handler.go`, `websocket/client.go` | WebSocket streaming |
| Binary Message | `message/services.go`, `message/handler.go` | Binary protocol |
| P2P Network | `p2p/services.go`, `p2p/p2p.go` | libp2p implementation |

### V3 Generated Code

**Location:** `/pkg/api/v3/`

| File | Generated From | Contains |
|------|-----------------|----------|
| `types_gen.go` | `types.yml` + others | Generated type structs |
| `enums_gen.go` | `enums.yml` | Generated enum types |
| `unions_gen.go` | `queries.yml`, etc | Generated union marshaling |

**Message-specific Generated Code:**

| File | Generated From | Contains |
|------|-----------------|----------|
| `message/types_gen.go` | `message/messages.yml` | Message type structs |
| `message/enums_gen.go` | `message/enums.yml` | Message enum types |
| `message/unions_gen.go` | Message types | Union marshaling |

### V2 API Definition Files (YAML)

**Location:** `/internal/api/v2/`

| File | Purpose |
|------|---------|
| `methods.yml` | All V2 JSON-RPC method definitions |
| `types.yml` | V2 type definitions |
| `responses.yml` | V2 response structures |
| `enums.yml` | V2 enumerations |

### V2 Implementation

**Location:** `/internal/api/v2/`

| File | Purpose |
|------|---------|
| `api.go` | Core API structure |
| `jrpc.go` | JSON-RPC handler registration |
| `jrpc_execute.go` | Transaction execution methods |
| `jrpc_metrics.go` | Metrics methods |
| `query_jrpc.go` | Query implementations |
| `query_v3.go` | V3 compatibility layer |

### Private API

**Location:** `/internal/api/private/`

| File | Purpose |
|------|---------|
| `api.go` | Private/internal API interfaces |
| `types_gen.go` | Generated types for private APIs |

### Ethereum API

**Location:** `/pkg/api/ethereum/`

| File | Purpose |
|------|---------|
| `services.go` | Ethereum service interface |
| `jsonrpc.go` | Ethereum JSON-RPC implementation |
| `types.go` | Ethereum types |
| `schema.yml` | Ethereum API schema |

## Code Generation Configuration

### V3 API Generation

**File:** `/pkg/api/v3/api.go` (generation directives)

```go
//go:generate go run ... gen-enum --package api enums.yml
//go:generate go run ... gen-types --long-union-discriminator --package api responses.yml options.yml records.yml events.yml types.yml queries.yml
//go:generate go run ... gen-types --language go-union --out unions_gen.go records.yml events.yml queries.yml
```

### Message Generation

**File:** `/pkg/api/v3/message/types.go` (generation directives)

```go
//go:generate go run ... gen-types --package message messages.yml
//go:generate go run ... gen-types --language go-union --out unions_gen.go messages.yml
```

### V2 API Generation

**File:** `/internal/api/v2/api.go` (generation directives)

```go
//go:generate go run ... gen-enum --package api --out enums_gen.go enums.yml
//go:generate go run ... gen-types --package api types.yml responses.yml
//go:generate go run ... gen-api --package api methods.yml
```

## Related Protocol/Types Packages

| Package | Location | Purpose |
|---------|----------|---------|
| messaging | `pkg/types/messaging/` | Message definitions |
| protocol | `protocol/` | Protocol types and transactions |
| errors | `pkg/errors/` | Error handling |
| url | `pkg/url/` | URL/Address types |
| merkle | `pkg/database/merkle/` | Merkle tree types |

## Testing and Mocks

**Location:** `/test/mocks/`

```
test/
└── mocks/
    ├── pkg/
    │   └── api/
    │       ├── mock_NodeService_test.go
    │       ├── mock_Querier_test.go
    │       └── ...
    └── internal/
        └── api/
            └── ...
```

## Documentation Files

| File | Purpose |
|------|---------|
| `/pkg/api/v3/openapi.yml` | OpenAPI/Swagger specification |
| `/pkg/api/v3/message/docs.go` | Message protocol documentation |

## Integration Points

### Server-Side Integration

1. **Initialize services** from `/pkg/api/v3/`
2. **Create handlers** using:
   - `jsonrpc.NewHandler()` for JSON-RPC
   - `rest.NewHandler()` for REST
   - `websocket.NewHandler()` for WebSocket
3. **Register services** with handlers
4. **Mount on HTTP server**

### Client-Side Integration

1. **Create client** from `/pkg/client/api/v2/` or via service types
2. **Call service methods** directly
3. **Handle responses** (Record union types)
4. **Subscribe to events** via EventService

## Build/Generation Requirements

To regenerate all code:

```bash
# V3 API
go generate ./pkg/api/v3/...

# Message protocol
go generate ./pkg/api/v3/message/...

# V2 API
go generate ./internal/api/v2/...

# P2P types
go generate ./pkg/api/v3/p2p/...

# Ethereum API
go generate ./pkg/api/ethereum/...
```

## Size Statistics

| Component | Files | Approx Lines |
|-----------|-------|--------------|
| V3 Core | 12 | 2,000+ |
| V3 JSON-RPC | 3 | 300+ |
| V3 REST | 2 | 350+ |
| V3 WebSocket | 3 | 200+ |
| V3 Message | 12 | 500+ |
| V3 P2P | 15 | 800+ |
| V2 API | 8 | 9,400+ |
| Ethereum API | 4 | 200+ |
| **Total Generated** | - | 20,000+ |

