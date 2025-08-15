# AI Context Document for Accumulate

## Project Overview
**Name**: Accumulate  
**Type**: Blockchain Protocol  
**Language**: Go  
**Architecture**: Multi-partition blockchain with cross-chain messaging  
**Repository**: gitlab.com/accumulatenetwork/accumulate

## Key Concepts

### Core Components
1. **Directory Network (DN)**: Central identity and routing registry
2. **Block Validator Networks (BVNs)**: Independent blockchain partitions
3. **CrossChain Conductor (CCC)**: Manages inter-partition messaging
4. **Gap Recovery**: Simple index-based message recovery mechanism

### Architecture Patterns
- **Modular Design**: Components are replaceable and testable
- **Message-Driven**: Asynchronous message passing between partitions
- **State Management**: Per-destination state tracking
- **Error Recovery**: Automatic gap detection and recovery

## Code Structure

### Primary Implementation Paths
```
internal/core/execute/v2/         # Core execution engine
├── block/                        # Block processing
├── chain/                        # Chain execution
└── crosschain/                   # CrossChain Conductor
    ├── conductor.go             # Main conductor
    ├── conductor_gap_recovery.go # Gap recovery
    └── destination_state.go     # State tracking

protocol/                         # Protocol definitions
├── system.md                    # System protocol
└── transactions.md              # Transaction types

pkg/api/v3/                      # Current API version
internal/api/v2/                 # Legacy API
```

### Key Files for Understanding
1. **Entry Points**:
   - `cmd/accumulated/main.go` - Main daemon
   - `cmd/accumulate/main.go` - CLI client

2. **Core Logic**:
   - `internal/core/execute/v2/crosschain/conductor.go` - Message routing
   - `internal/core/execute/v2/block/block_begin.go` - Block processing
   - `protocol/transaction.go` - Transaction definitions

3. **Configuration**:
   - `config/` - Network configurations
   - `.gitlab-ci.yml` - CI/CD pipeline

## Common Tasks

### Development Workflow
```bash
# Start local development network
./scripts/devnet/devnet_config.sh start 3 3 1

# Run tests
go test ./...

# Run load tests
./scripts/devnet/devnet_load_test.sh

# Debug gap recovery
./scripts/devnet/interactive_pause_test.sh
```

### Testing Commands
```bash
# Unit tests with coverage
go test -coverprofile=coverage.out ./...

# Integration tests
go test ./test/e2e_v2/...

# Benchmark tests
go test -bench=. ./...
```

## Key Algorithms

### Gap Recovery Mechanism
```go
// Simplified gap recovery logic
type DestinationSendState struct {
    SentTxIndex    uint64  // Last successfully sent
    CurrentTxIndex uint64  // Latest available
}

// On failure: SentTxIndex stays unchanged
// On success: SentTxIndex = CurrentTxIndex
// On gap request: SentTxIndex = LastKnownByDestination
```

### Message Batching
- Collection proofs batch multiple messages
- Single proof for multiple transactions
- Reduces network overhead

## API Endpoints

### JSON-RPC Methods
- `query` - Query accounts/transactions
- `submit` - Submit transactions
- `metrics` - Get node metrics
- `status` - Get node status

### Debug Endpoints (testnet build)
- `/debug/ccc/status` - CCC status
- `/debug/ccc/pause` - Pause partition
- `/debug/ccc/resume` - Resume partition

## Testing Infrastructure

### Test Categories
1. **Unit Tests**: `*_test.go` files
2. **Integration Tests**: `test/e2e_v2/`
3. **Load Tests**: `scripts/devnet/*_test.sh`
4. **Simulation Tests**: `test/simulator/`

### Key Test Files
- `internal/core/execute/v2/crosschain/test_gap_recovery_test.go`
- `test/e2e_v2/sig_test.go`
- `test/simulator/simulator_test.go`

## Configuration Options

### Environment Variables
- `ACC_DEBUG` - Enable debug logging
- `ACC_CCC_BATCH_SIZE` - Message batch size
- `ACC_API_PORT` - API server port

### Build Tags
- `testnet` - Enable test features (pause/resume)
- `debug` - Enable debug endpoints
- `race` - Enable race detection

## Common Patterns

### Error Handling
```go
if err != nil {
    return errors.BadRequest.With("description").Wrap(err)
}
```

### Logging
```go
logger.Info("Message", "field", value)
logger.Error("Error", "error", err)
```

### State Management
```go
state.Lock()
defer state.Unlock()
// Modify state
```

## Dependencies

### Major Libraries
- `github.com/tendermint/tendermint` - Consensus
- `github.com/stretchr/testify` - Testing
- `github.com/prometheus/client_golang` - Metrics
- `gitlab.com/accumulatenetwork/core` - Core libraries

## Performance Considerations

### Optimization Points
1. Message batching for network efficiency
2. Index-based gap recovery (O(1) reset)
3. Concurrent message processing
4. Efficient state storage

### Resource Requirements
- Memory: 200-300MB per validator
- CPU: 1 core per 2-3 validators
- Storage: Linear with transaction volume

## Troubleshooting Guide

### Common Issues
1. **Port conflicts**: Change BASE_PORT
2. **Memory issues**: Reduce validator count
3. **Consensus failures**: Check time sync
4. **Gap recovery stuck**: Check CCC status

### Debug Commands
```bash
# Check node status
curl http://localhost:26660/v2/status

# View logs
tail -f devnet_config.log

# Check metrics
curl http://localhost:26660/metrics
```

## Security Considerations

### Key Management
- Never commit private keys
- Use hardware security modules
- Implement key rotation

### Network Security
- Firewall configuration required
- TLS for production APIs
- Rate limiting enabled

## Documentation Structure

### Navigation
- `docs/INDEX.md` - Main documentation index
- `docs/design/` - Architecture decisions
- `docs/testing/` - Test documentation
- `docs/api/` - API references
- `docs/deployment/` - Deployment guides

### Quick References
- [Gap Recovery](docs/design/crosschain/GAP_RECOVERY_ACTUAL.md)
- [DevNet Setup](docs/testing/devnet/devnet-setup.md)
- [API Reference](docs/api/api-interfaces-reference.md)

## Version Information
- Current Branch: 3653-add-a-crosschainconductor-process-for-coordinating-partitions
- Go Version: 1.19+
- Protocol Version: v2

## Contact Points
- GitLab: https://gitlab.com/accumulatenetwork/accumulate
- Issues: GitLab Issues
- Documentation: This repository

## AI Assistant Notes

### When working with this codebase:
1. Always check existing patterns before implementing new features
2. Run tests after making changes
3. Use the DevNet for testing
4. Follow the established error handling patterns
5. Document significant changes

### Key areas requiring attention:
- Gap recovery mechanism is critical for reliability
- State management must be thread-safe
- Message ordering must be preserved
- Performance monitoring is essential

### Testing priorities:
1. Gap recovery scenarios
2. Network partition handling
3. High-load conditions
4. State consistency