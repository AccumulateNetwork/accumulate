# Accumulate Network Test Suite - Master Index

## Overview

This comprehensive index provides structured access to all testing documentation, organized for both human developers and AI assistants to efficiently navigate and utilize the test suite.

## Quick Navigation

### 🎯 **By Role/Use Case**
- **New Developer**: [readme.md](readme.md) → [unit-tests.md](unit-tests.md) → [debugging.md](debugging.md)
- **CI/CD Engineer**: [ci-cd.md](ci-cd.md) → [test-maintenance.md](test-maintenance.md)
- **Performance Engineer**: [performance-tests.md](performance-tests.md) → [simulator-tests.md](simulator-tests.md)
- **QA Engineer**: [e2e-tests.md](e2e-tests.md) → [testing.md](testing.md)
- **AI Assistant**: This file → [ai-guidance.md](ai-guidance.md) → Specific guides

### 📚 **By Documentation Type**
- **Overview**: [readme.md](readme.md)
- **Complete Catalog**: [test-content.md](test-content.md)
- **Comprehensive Guide**: [testing.md](testing.md)
- **Specialized Guides**: [unit-tests.md](unit-tests.md), [e2e-tests.md](e2e-tests.md), [performance-tests.md](performance-tests.md), [simulator-tests.md](simulator-tests.md)
- **Development**: [debugging.md](debugging.md), [ci-cd.md](ci-cd.md), [test-maintenance.md](test-maintenance.md)

## Keyword Index

### 🔍 **Testing Concepts**
- **Unit Testing**: [unit-tests.md](unit-tests.md) - Lines 1-742
- **Integration Testing**: [e2e-tests.md](e2e-tests.md) - Lines 1-848
- **Performance Testing**: [performance-tests.md](performance-tests.md) - Lines 1-1031
- **Load Testing**: [performance-tests.md](performance-tests.md) - Lines 400-600
- **Benchmark Testing**: [performance-tests.md](performance-tests.md) - Lines 200-400
- **Simulation Testing**: [simulator-tests.md](simulator-tests.md) - Lines 1-411

### 🛠 **Tools & Commands**
- **Go Testing**: [testing.md](testing.md) - Lines 100-200
- **Delve Debugger**: [debugging.md](debugging.md) - Lines 200-300
- **VS Code Integration**: [debugging.md](debugging.md) - Lines 300-400
- **CI/CD Pipelines**: [ci-cd.md](ci-cd.md) - Lines 1-1005
- **GitLab CI**: [ci-cd.md](ci-cd.md) - Lines 100-300
- **GitHub Actions**: [ci-cd.md](ci-cd.md) - Lines 300-500

### 🐛 **Debugging & Troubleshooting**
- **Flaky Tests**: [debugging.md](debugging.md) - Lines 500-600, [test-maintenance.md](test-maintenance.md) - Lines 200-400
- **Performance Issues**: [debugging.md](debugging.md) - Lines 600-700
- **Memory Leaks**: [debugging.md](debugging.md) - Lines 400-500
- **Race Conditions**: [debugging.md](debugging.md) - Lines 300-400
- **Timeout Issues**: [debugging.md](debugging.md) - Lines 200-300

### 🔧 **Maintenance & Optimization**
- **Test Health Monitoring**: [test-maintenance.md](test-maintenance.md) - Lines 1-200
- **Performance Optimization**: [test-maintenance.md](test-maintenance.md) - Lines 400-600
- **Test Refactoring**: [test-maintenance.md](test-maintenance.md) - Lines 600-800
- **Dependency Management**: [test-maintenance.md](test-maintenance.md) - Lines 800-1000

## Function & Test Index

### 📋 **Test Categories by File Location**
```
test/e2e/
├── api_test.go - API functionality tests
├── txn_*_test.go - Transaction-specific tests
├── net_*_test.go - Network operation tests
├── query_*_test.go - Query operation tests
└── sim_*_test.go - Simulation tests

test/simulator/
├── consensus_test.go - Consensus mechanism tests
├── network_test.go - Network behavior tests
└── partition_test.go - Network partition tests

test/harness/
├── harness.go - Test harness utilities
└── simulator.go - Simulator framework
```

### 🎯 **Common Test Patterns**
- **Setup/Teardown**: [unit-tests.md](unit-tests.md) - Lines 200-300
- **Table-Driven Tests**: [unit-tests.md](unit-tests.md) - Lines 300-400
- **Mock Usage**: [unit-tests.md](unit-tests.md) - Lines 400-500
- **Parallel Testing**: [performance-tests.md](performance-tests.md) - Lines 100-200

## AI Assistant Guidance

### 🤖 **For Code Generation**
1. **Test Templates**: See [unit-tests.md](unit-tests.md) - Lines 500-600
2. **Common Patterns**: See [testing.md](testing.md) - Lines 300-400
3. **Best Practices**: See each guide's "Best Practices" section

### 🧠 **For Debugging Assistance**
1. **Error Patterns**: [debugging.md](debugging.md) - Lines 700-800
2. **Common Solutions**: [debugging.md](debugging.md) - Lines 800-900
3. **Diagnostic Commands**: [debugging.md](debugging.md) - Lines 100-200

### 📊 **For Performance Analysis**
1. **Benchmarking**: [performance-tests.md](performance-tests.md) - Lines 200-400
2. **Profiling**: [performance-tests.md](performance-tests.md) - Lines 600-800
3. **Optimization**: [performance-tests.md](performance-tests.md) - Lines 800-1000

## Cross-Reference Matrix

| Topic | Primary Doc | Secondary Docs | Related Tools |
|-------|-------------|----------------|---------------|
| Unit Testing | unit-tests.md | testing.md, debugging.md | go test, VS Code |
| E2E Testing | e2e-tests.md | simulator-tests.md, testing.md | simulator, harness |
| Performance | performance-tests.md | debugging.md, test-maintenance.md | pprof, benchstat |
| Debugging | debugging.md | All guides | dlv, VS Code, logging |
| CI/CD | ci-cd.md | test-maintenance.md | GitLab, GitHub Actions |
| Maintenance | test-maintenance.md | ci-cd.md, debugging.md | Scripts, monitoring |

## Command Quick Reference

### 🚀 **Essential Commands**
```bash
# Run all tests
make test

# Run specific categories
make test-unit
make test-e2e
make test-performance

# Debug specific test
go test -v -run TestName ./path/to/test

# Performance profiling
go test -cpuprofile=cpu.prof -memprofile=mem.prof ./...

# Coverage analysis
go test -coverprofile=coverage.out ./...
```

### 🔍 **Debugging Commands**
```bash
# Verbose output
go test -v ./...

# Race detection
go test -race ./...

# Timeout adjustment
go test -timeout=30m ./...

# Debug with Delve
dlv test ./path/to/test -- -test.run TestName
```

## File Size & Complexity Reference

| File | Size | Lines | Complexity | Update Frequency |
|------|------|-------|------------|------------------|
| readme.md | 7.5KB | 275 | Low | Stable |
| testing.md | 15.4KB | 702 | Medium | Occasional |
| test-content.md | 11.5KB | 335 | Low | Regular |
| unit-tests.md | 15.8KB | 742 | Medium | Occasional |
| e2e-tests.md | 20.7KB | 848 | High | Regular |
| performance-tests.md | 22.5KB | 1031 | High | Occasional |
| simulator-tests.md | 9.2KB | 411 | Medium | Stable |
| debugging.md | 18KB | 830 | High | Occasional |
| ci-cd.md | 22.4KB | 1005 | High | Regular |
| test-maintenance.md | 25KB | 1070 | High | Regular |

## Maintenance Notes

### 📅 **Update Schedule**
- **Weekly**: CI/CD configurations, test results
- **Monthly**: Performance benchmarks, maintenance scripts
- **Quarterly**: Comprehensive review of all documentation
- **As Needed**: New test types, tool updates, best practices

### 🔄 **Version Control**
- All documentation is version-controlled with the codebase
- Changes should be atomic with related code changes
- Documentation reviews are part of the code review process

---

*This index is automatically maintained. Last updated: 2025-01-17*
