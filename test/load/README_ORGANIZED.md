# Accumulate Load Testing

## Directory Structure

This directory contains load testing tools for the Accumulate network. The tests are organized as follows:

### 📁 sl-load/ - Streamlined Load Test (PRIMARY)
**The main, fully-functional load testing framework**

```bash
# Run with default settings (1000 txs)
go test -v ./sl-load -run TestStreamlinedLoad

# Run with custom parameters
go test -v ./sl-load -run TestStreamlinedLoad -args -txs 50000 -tps 100 -k 20 -a 20
```

**Features:**
- Complete flag-based configuration
- Tested up to 3000 TPS with 100% success
- Smart endpoint discovery
- Comprehensive reporting

[Full Documentation →](./sl-load/README.md)

### 📁 simple/ - Simple Preset Tests
**Pre-configured tests for common scenarios**

```bash
# 50k transactions at 100 TPS
go test -v ./simple -run TestSimple50K

# 100k transactions at 200 TPS  
go test -v ./simple -run TestSimple100K
```

**Note:** Edit constants in test files to change settings (marked with `// DEFAULT:`)

### 📁 Other Files

- `devnet_endpoint.go` - Shared endpoint discovery
- `devnet_smart_discovery.go` - Advanced discovery system
- Other `.go` files - Various experimental/utility tests

## Quick Start

### 1. Start DevNet
```bash
go run ./cmd/accumulated run devnet -w .devnet
```

### 2. Run Load Test
```bash
# Use streamlined test (recommended)
cd sl-load
go test -v -run TestStreamlinedLoad -args -txs 10000 -tps 100

# Or use simple presets
cd ../simple
go test -v -run TestSimple50K
```

### 3. Monitor Results
```bash
# Check metrics
curl http://127.0.0.1:26660/metrics

# Watch logs
tail -f .devnet/*/node.log
```

## Performance Summary

| TPS | Success Rate | Status |
|-----|-------------|--------|
| 50-200 | 100% | ✅ Production Ready |
| 500 | 100% | ✅ Excellent |
| 1000 | 100% | ✅ Outstanding |
| 2000 | 100% | ✅ Exceptional |
| 3000 | 100% | ✅ Maximum Tested |

## Documentation

- [Consolidated Documentation](./CONSOLIDATED_DOCS.md) - All docs in one place
- [Load Test Guide](./LOAD_TEST_GUIDE.md) - Quick reference
- [Performance Report](./TPS_PERFORMANCE_REPORT.md) - Detailed results

## Choosing a Test

| Need | Use | Command |
|------|-----|---------|
| **Full control** | sl-load | `go test -v ./sl-load -run TestStreamlinedLoad -args ...` |
| **Quick 50k test** | simple | `go test -v ./simple -run TestSimple50K` |
| **Quick 100k test** | simple | `go test -v ./simple -run TestSimple100K` |
| **Custom scenario** | sl-load | Use flags to configure |

## Important Notes

1. **sl-load is the primary test** - Use it for serious testing
2. **Simple tests are presets** - Quick tests with fixed parameters
3. **Always start devnet first** - Tests will skip if devnet isn't running
4. **Monitor metrics** - Use curl to check real-time performance

---

*For detailed documentation, see [sl-load/README.md](./sl-load/README.md)*