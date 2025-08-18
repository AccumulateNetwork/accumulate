# AI Navigation Index for Accumulate Documentation

> **Purpose**: This index is optimized for AI assistants to quickly locate documentation and code.

## 🎯 Quick Topic Navigation

| Topic | Documentation | Code Location |
|-------|--------------|---------------|
| **Architecture** | `docs/architecture/` | `internal/core/` |
| **CrossChain Conductor** | `docs/crosschain/` | `internal/core/execute/v2/crosschain/` |
| **Load Testing** | `test/load/sl-load/README.md` | `test/load/sl-load/` |
| **Tools** | `docs/tools/` | `tools/cmd/` |
| **API** | `docs/api/` | `pkg/api/v3/` |
| **Protocol** | `docs/protocol/` | `protocol/` |
| **Debugging** | `docs/debugging/` | - |
| **Setup** | `docs/setup/` | `cmd/accumulated/` |

## 📋 Essential Documents by Purpose

### System Understanding
- **Architecture Overview**: `docs/architecture/README.md`
- **Network Design**: `docs/architecture/network-sync.md`
- **P2P Architecture**: `docs/architecture/P2P-ARCHITECTURE.md`
- **Light Client**: `docs/architecture/light-client-design.md`

### CrossChain System
- **Design Document**: `docs/crosschain/README.md`
- **Implementation**: `internal/core/execute/v2/crosschain/conductor.go`
- **Types**: `internal/core/execute/v2/crosschain/types.go`
- **Recovery**: `internal/core/execute/v2/crosschain/recovery.go`
- **Proof Service**: `internal/core/execute/v2/crosschain/proof_service.go`

### Load Testing
- **Streamlined Test**: `test/load/sl-load/README.md`
- **Design**: `test/load/sl-load/sl_design.md`
- **Entry Point**: `test/load/sl-load/sl_test.go`
- **Performance Report**: `test/load/TPS_PERFORMANCE_REPORT.md`

### Development Setup
- **DevNet Setup**: `docs/setup/devnet-setup.md`
- **Bootstrap Server**: `docs/setup/BOOTSTRAP-SERVER.md`
- **Configuration**: `docs/configuration/`

### Debugging & Troubleshooting
- **Troubleshooting Guide**: `docs/debugging/TROUBLESHOOTING.md`
- **Debug Tool**: `docs/tools/debug-tool.md`
- **Authority Validation**: `docs/debugging/debug-authority-validation.md`
- **Snapshot Debugging**: `docs/debugging/debug-snapshot.md`

### Tools & Utilities
- **Analyze Tool**: `docs/tools/analyze-tool.md`
- **Debug Tool**: `docs/tools/debug-tool.md`
- **Simulator**: `docs/tools/simulator-tool.md`
- **Factom Import**: `docs/tools/factom-tool.md`

## 🔍 Key Code Patterns & Locations

### Transaction Flow
```
1. Entry: internal/core/execute/v2/block/block_end.go:578
2. Conductor: internal/core/execute/v2/crosschain/conductor.go:192
3. Dispatcher: internal/core/execute/execute.go
4. Network: pkg/api/v3/jsonrpc/
```

### Configuration Points
```
- Daemon: internal/node/daemon/run.go:407
- DevNet: cmd/accumulated/run/devnet.go
- Load Test: test/load/sl-load/sl_test.go:20-27
```

### Testing Entry Points
```bash
# Streamlined load test
go test -v ./test/load/sl-load -run TestStreamlinedLoad

# Simple presets
go test -v ./test/load -run TestSimple50K
go test -v ./test/load -run TestSimple100K
```

## 🚀 Common Tasks

### Enable CrossChain Conductor
```go
// File: internal/node/daemon/run.go:407
EnableCrosschainCoordinator: true
```

### Run Load Test
```bash
cd test/load/sl-load
go test -v -run TestStreamlinedLoad -args -txs 50000 -tps 100
```

### Start DevNet
```bash
go run ./cmd/accumulated run devnet -w .devnet
```

### Debug Network Issues
```bash
# Check status
curl http://127.0.0.1:26660/metrics

# View logs
tail -f .devnet/*/node.log

# Test connectivity
go test -run TestDevnetDiscovery
```

## 📊 Performance Baselines

| Metric | Value | Location |
|--------|-------|----------|
| Max TPS Tested | 3000 | `test/load/TPS_PERFORMANCE_REPORT.md` |
| Success Rate | 100% | `test/load/sl-load/sl_design.md` |
| Memory/TX | 200B | `docs/crosschain/README.md` |
| Retry Success | 95% | `internal/core/execute/v2/crosschain/conductor.go` |

## 🔗 Quick Links

### Project Structure
- Main README: `README.md`
- Contributing: `CONTRIBUTING.md`
- License: `LICENSE`

### Test Documentation
- Load Test Guide: `test/load/LOAD_TEST_GUIDE.md`
- Consolidated Docs: `test/load/CONSOLIDATED_DOCS.md`
- SL Test Design: `test/load/sl-load/sl_design.md`

### Configuration
- DevNet Config: `cmd/accumulated/run/devnet.go`
- Test Config: `test/load/sl-load/sl_types.go`

## 🏷️ Search Tags for AI

### By Feature
- `#crosschain` - Cross-partition transactions
- `#loadtest` - Performance testing
- `#devnet` - Local development network
- `#debugging` - Troubleshooting guides
- `#architecture` - System design

### By Component
- `#conductor` - CrossChain Conductor
- `#dispatcher` - Message dispatcher
- `#recovery` - Transaction recovery
- `#proof` - Proof service
- `#metrics` - Performance metrics

### By Action
- `#setup` - Initial configuration
- `#run` - Execution commands
- `#test` - Testing procedures
- `#debug` - Debugging steps
- `#monitor` - Monitoring tools

---

*Last Updated: 2025-08-18 | Use this index for rapid navigation of Accumulate documentation*