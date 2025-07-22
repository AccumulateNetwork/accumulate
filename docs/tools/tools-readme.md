# Accumulate Network Development Tools

## Overview

The `tools/` directory contains essential development utilities for the Accumulate Network project. These tools support code generation, debugging, database management, testing, and maintenance workflows.

## Quick Navigation

- [🔧 **Development Tools**](#development-tools) - Code generation and development utilities
- [🐛 **Debugging Tools**](#debugging-tools) - Debugging and analysis utilities  
- [🗄️ **Database Tools**](#database-tools) - Database management and repair
- [🌐 **Network Tools**](#network-tools) - Network simulation and testing
- [⚙️ **Maintenance Tools**](#maintenance-tools) - System maintenance and utilities

## Tool Categories

### 🔧 Development Tools

| Tool | Purpose | Documentation |
|------|---------|---------------|
| `gen-api` | Generate API client code | [gen-api.md](gen-api.md) |
| `gen-enum` | Generate enum types and methods | [gen-enum.md](gen-enum.md) |
| `gen-model` | Generate data model code | [gen-model.md](gen-model.md) |
| `gen-sdk` | Generate SDK components | [gen-sdk.md](gen-sdk.md) |
| `gen-types` | Generate protocol types | [gen-types.md](gen-types.md) |
| `golangci-lint` | Custom linting configuration | [golangci-lint.md](golangci-lint.md) |

### 🐛 Debugging Tools

| Tool | Purpose | Documentation |
|------|---------|---------------|
| `analyze` | Network and performance analysis | [analyze.md](analyze.md) |
| `debug` | Comprehensive debugging utility | [debug.md](debug.md) |

### 🗄️ Database Tools

| Tool | Purpose | Documentation |
|------|---------|---------------|
| `dbrepair` | Database repair and recovery | [dbrepair.md](dbrepair.md) |
| `repair-indices` | Index repair utility | [repair-indices.md](repair-indices.md) |
| `snapshot` | Snapshot management | [snapshot.md](snapshot.md) |

### 🌐 Network Tools

| Tool | Purpose | Documentation |
|------|---------|---------------|
| `_factom` | Factom blockchain migration tools | [factom.md](factom.md) |
| `genesis` | Genesis block utilities | [genesis.md](genesis.md) |
| `light-client` | Lightweight blockchain client | [light-client.md](light-client.md) |
| `simulator` | Standalone network simulator | [simulator.md](simulator.md) |

### ⚙️ Maintenance Tools

| Tool | Purpose | Documentation |
|------|---------|---------------|
| `recode` | Code transformation utility | [recode.md](recode.md) |
| `sendinterrupt` | Signal sending utility | [sendinterrupt.md](sendinterrupt.md) |

## Quick Start

### Building Tools

```bash
# Build all tools
make tools

# Build specific tool
go build -o bin/debug ./tools/cmd/debug
go build -o bin/simulator ./tools/cmd/simulator
```

### Common Usage Patterns

```bash
# Debug network issues
./bin/debug network --analyze

# Run standalone simulator
./bin/simulator --bvn-count 3 --port 8080

# Repair database
./bin/dbrepair --database ./data --check

# Generate API code
./bin/gen-api --input ./protocol --output ./pkg/api
```

## Installation

### Prerequisites

- Go 1.21+
- Make (optional, for build automation)

### Build Instructions

```bash
# Clone repository
git clone https://gitlab.com/AccumulateNetwork/accumulate.git
cd accumulate

# Build all tools
cd tools
go build ./cmd/...

# Or build individual tools
go build -o ../bin/debug ./cmd/debug
go build -o ../bin/simulator ./cmd/simulator
```

## Integration with Development Workflow

### Code Generation Workflow

```bash
# 1. Update protocol definitions
vim protocol/types.go

# 2. Regenerate types
./bin/gen-types --input ./protocol --output ./pkg/types

# 3. Regenerate API
./bin/gen-api --input ./protocol --output ./pkg/api

# 4. Update SDK
./bin/gen-sdk --input ./pkg/api --output ./sdk
```

### Testing Workflow

```bash
# 1. Start simulator for testing
./bin/simulator --background --port 8080

# 2. Run tests against simulator
go test ./test/e2e/... -simulator-url http://localhost:8080

# 3. Debug failing tests
./bin/debug test --test-name TestFactomAddresses --verbose
```

### Database Maintenance

```bash
# 1. Check database health
./bin/dbrepair --database ./data --check --verbose

# 2. Repair indices if needed
./bin/repair-indices --database ./data --rebuild

# 3. Create snapshot
./bin/snapshot --database ./data --output ./backups/snapshot-$(date +%Y%m%d).tar.gz
```

## Tool Development

### Adding New Tools

1. Create directory: `tools/cmd/newtool/`
2. Add `main.go` with cobra CLI structure
3. Add documentation: `tools/docs/newtool.md`
4. Update this README
5. Add to build scripts

### Tool Standards

- **CLI Framework**: Use `github.com/spf13/cobra`
- **Logging**: Use `gitlab.com/accumulatenetwork/accumulate/internal/logging`
- **Configuration**: Support both CLI flags and config files
- **Error Handling**: Consistent error messages and exit codes
- **Documentation**: Include usage examples and common patterns

## Troubleshooting

### Common Issues

| Issue | Solution |
|-------|----------|
| Tool not found | Run `make tools` to build all tools |
| Permission denied | Check file permissions: `chmod +x bin/*` |
| Database locked | Ensure no other processes are using the database |
| Port already in use | Use `--port` flag to specify different port |

### Getting Help

```bash
# Get help for any tool
./bin/debug --help
./bin/simulator --help

# Get help for specific command
./bin/debug network --help
./bin/simulator run --help
```

## Contributing

1. Follow existing tool patterns and CLI conventions
2. Add comprehensive documentation for new tools
3. Include usage examples and common workflows
4. Test tools with various scenarios
5. Update this README when adding new tools

## See Also

- [Test Documentation](../test/docs/readme.md) - Testing guides and workflows
- [Development Guide](../docs/development.md) - General development setup
- [API Documentation](../docs/api.md) - API reference and examples

---

**Last Updated**: 2025-01-17  
**Total Tools**: 15  
**Documentation Coverage**: 100%
