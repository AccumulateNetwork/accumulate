# CrossChain Conductor Documentation

Documentation for the CrossChain Conductor system that handles cross-partition transactions.

## Key Files

See `internal/core/execute/v2/crosschain/` for implementation:
- `conductor.go` - Main conductor logic
- `types.go` - Data structures
- `recovery.go` - Transaction recovery
- `proof_service.go` - Proof construction/validation

## Features

- Async transaction processing
- Automatic retry with exponential backoff
- Collection proof batching
- Missing transaction recovery
