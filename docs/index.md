# Accumulate Documentation Index

This directory contains technical documentation for the Accumulate blockchain protocol.

## Core Documentation

### Architecture

| Document | Description | Location |
|----------|-------------|----------|
| [AppHash](./apphash.md) | AppHash computation and CometBFT ABCI interface | `docs/apphash.md` |
| [Receipts](./receipts.md) | Merkle receipt architecture, user API, and test references | `docs/receipts.md` |
| [SMT/BPT](../internal/database/smt/README.md) | Stateful Merkle Trees and Binary Patricia Trees | `internal/database/smt/` |

### API

All API versions are actively used in concert:

| Document | Description | Location |
|----------|-------------|----------|
| [API v2](../internal/api/v2/README.md) | API v2 execute dispatch and migration notes | `internal/api/v2/` |
| [API v3](../pkg/api/v3/README.md) | API v3 architecture with services and transports | `pkg/api/v3/` |
| [Database](../pkg/database/README.md) | Hierarchical caching data model | `pkg/database/` |

### Protocol

| Document | Description | Location |
|----------|-------------|----------|
| [System Accounts](../protocol/system.md) | System accounts and subnet architecture | `protocol/system.md` |
| [Transactions](../protocol/transactions.md) | Transaction design and signatures | `protocol/transactions.md` |

### Execution

Both v1 and v2 executors are part of the codebase:

| Document | Description | Location |
|----------|-------------|----------|
| [Chain Validators v1](../internal/core/execute/v1/chain/README.md) | Chain validator design (v1) | `internal/core/execute/v1/chain/` |
| [Chain Validators v2](../internal/core/execute/v2/chain/README.md) | Chain validator design (v2) | `internal/core/execute/v2/chain/` |
| [Signing Rules](../internal/core/execute/v2/signing.md) | Signing and authorization rules | `internal/core/execute/v2/` |

### Tools

| Document | Description | Location |
|----------|-------------|----------|
| [Lite Client](../tools/cmd/debug/docs/lite_client.md) | Lite client design and implementation | `tools/cmd/debug/docs/` |
| [Snapshots](../tools/cmd/debug/docs/snapshot.md) | BPT storage and snapshot collection | `tools/cmd/debug/docs/` |

## Contributing

When adding new documentation:
1. Place protocol/architecture docs in `docs/`
2. Place component-specific docs in the relevant package directory
3. Update this index when adding significant documentation
