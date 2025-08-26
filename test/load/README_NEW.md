# Load Testing Documentation

## 📚 Documentation Structure

We've consolidated 27 separate documentation files into a single, comprehensive guide for easier navigation and maintenance.

### Primary Documentation

| Document | Purpose | Size |
|----------|---------|------|
| **[CONSOLIDATED_DOCS.md](./CONSOLIDATED_DOCS.md)** | 📖 **Complete reference guide** - All documentation in one place | ~500 lines |
| [LOAD_TEST_GUIDE.md](./LOAD_TEST_GUIDE.md) | Quick reference for running tests | ~180 lines |
| [TPS_PERFORMANCE_REPORT.md](./TPS_PERFORMANCE_REPORT.md) | Performance test results and baselines | ~160 lines |

## 🚀 Quick Start

```bash
# Start DevNet
go run ./cmd/accumulated run devnet -w .devnet

# Run standard load test (50k transactions at 100 TPS)
go test -v -run TestSimple50K -timeout 20m

# Run with custom parameters
go test -v -run TestStreamlinedLoad -args -txs 100000 -tps 200 -k 40 -a 40
```

## 📊 Performance Summary

- **Maximum Tested TPS**: 3000 (100% success rate)
- **Recommended Production TPS**: 200-500
- **No failures detected** up to 3000 TPS

## 🔍 Finding Information

### By Topic

- **Load Testing** → CONSOLIDATED_DOCS.md § [Load Testing Guide](#load-testing-guide)
- **CrossChain Conductor** → CONSOLIDATED_DOCS.md § [CrossChain Conductor](#crosschain-conductor)
- **DevNet Setup** → CONSOLIDATED_DOCS.md § [DevNet Configuration](#devnet-configuration)
- **Troubleshooting** → CONSOLIDATED_DOCS.md § [Troubleshooting](#troubleshooting)
- **API Issues** → CONSOLIDATED_DOCS.md § [API & Connection Management](#api--connection-management)

### Quick Commands

```bash
# Search consolidated docs
grep -n "keyword" CONSOLIDATED_DOCS.md

# View section headers
grep "^##" CONSOLIDATED_DOCS.md

# Jump to specific section (example: Performance)
sed -n '/^## Performance Results/,/^## /p' CONSOLIDATED_DOCS.md
```

## 🗂️ Archive

Previous documentation (27 files) can be archived using:
```bash
./archive_old_docs.sh
```

This will:
- Create timestamped archive directory
- Move old docs while preserving essential files
- Generate an index of archived content

## 📝 Maintenance

When adding new documentation:
1. **Update CONSOLIDATED_DOCS.md** instead of creating new files
2. Keep sections organized and searchable
3. Update the Table of Contents
4. Maintain the Quick Reference section

## 🔗 Related Documentation

- **Project Root**: [/README.md](../../README.md)
- **AI Guide**: [/AI_OPTIMIZED_DOCS.md](../../AI_OPTIMIZED_DOCS.md)
- **CrossChain Code**: [internal/core/execute/v2/crosschain/](../../internal/core/execute/v2/crosschain/)

---

*Documentation consolidated on 2025-08-18 | 27 files → 1 comprehensive guide*