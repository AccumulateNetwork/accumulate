# Performance Test Results - RC v1.5.1-breaking (Issue #3905)

**Test Date**: 2026-04-14 16:22:56
**Methodology**: Incremental TPS testing (1K → 15K) with pushback detection (error > 5%)
**Configurations**: 6 (3/4 validators × 1/2/3 BVNs)

---

## Executive Summary

- **Total Configurations Tested**: 9
- **Average Per-BVN Throughput**: 0 TPS
- **Test Duration**: ~135 minutes

## Per-Configuration Results

| Test ID | Config | Max Sustained TPS | Per-BVN Limit | Total Network | Status |
|---------|--------|------------------|---------------|----------------|--------|
| A1 | 2v,1b | 0 TPS | ~0 TPS | ~0 TPS | ✓ No pushback |
| A2 | 3v,1b | 0 TPS | ~0 TPS | ~0 TPS | ✓ No pushback |
| A3 | 4v,1b | 0 TPS | ~0 TPS | ~0 TPS | ✓ No pushback |
| B1 | 2v,2b | 0 TPS | ~0 TPS | ~0 TPS | ✓ No pushback |
| B2 | 3v,2b | 0 TPS | ~0 TPS | ~0 TPS | ✓ No pushback |
| B3 | 4v,2b | 0 TPS | ~0 TPS | ~0 TPS | ✓ No pushback |
| C1 | 2v,3b | 0 TPS | ~0 TPS | ~0 TPS | ✓ No pushback |
| C2 | 3v,3b | 0 TPS | ~0 TPS | ~0 TPS | ✓ No pushback |
| C3 | 4v,3b | 0 TPS | ~0 TPS | ~0 TPS | ✓ No pushback |

## Detailed Results

### Test A1: Single-BVN-2-Validators

**Configuration**: 2 validators, 1 BVN(s)
**Max Sustained TPS**: 0
**Stable Range**: 0-0 TPS (<1% error)

| TPS Target | Submitted | Success | Failed | Error % | Actual TPS | Status |
|-----------|-----------|---------|--------|---------|-----------|--------|

### Test A2: Single-BVN-3-Validators

**Configuration**: 3 validators, 1 BVN(s)
**Max Sustained TPS**: 0
**Stable Range**: 0-0 TPS (<1% error)

| TPS Target | Submitted | Success | Failed | Error % | Actual TPS | Status |
|-----------|-----------|---------|--------|---------|-----------|--------|

### Test A3: Single-BVN-4-Validators

**Configuration**: 4 validators, 1 BVN(s)
**Max Sustained TPS**: 0
**Stable Range**: 0-0 TPS (<1% error)

| TPS Target | Submitted | Success | Failed | Error % | Actual TPS | Status |
|-----------|-----------|---------|--------|---------|-----------|--------|

### Test B1: Dual-BVN-2-Validators

**Configuration**: 2 validators, 2 BVN(s)
**Max Sustained TPS**: 0
**Stable Range**: 0-0 TPS (<1% error)

| TPS Target | Submitted | Success | Failed | Error % | Actual TPS | Status |
|-----------|-----------|---------|--------|---------|-----------|--------|

### Test B2: Dual-BVN-3-Validators

**Configuration**: 3 validators, 2 BVN(s)
**Max Sustained TPS**: 0
**Stable Range**: 0-0 TPS (<1% error)

| TPS Target | Submitted | Success | Failed | Error % | Actual TPS | Status |
|-----------|-----------|---------|--------|---------|-----------|--------|

### Test B3: Dual-BVN-4-Validators

**Configuration**: 4 validators, 2 BVN(s)
**Max Sustained TPS**: 0
**Stable Range**: 0-0 TPS (<1% error)

| TPS Target | Submitted | Success | Failed | Error % | Actual TPS | Status |
|-----------|-----------|---------|--------|---------|-----------|--------|

### Test C1: Triple-BVN-2-Validators

**Configuration**: 2 validators, 3 BVN(s)
**Max Sustained TPS**: 0
**Stable Range**: 0-0 TPS (<1% error)

| TPS Target | Submitted | Success | Failed | Error % | Actual TPS | Status |
|-----------|-----------|---------|--------|---------|-----------|--------|

### Test C2: Triple-BVN-3-Validators

**Configuration**: 3 validators, 3 BVN(s)
**Max Sustained TPS**: 0
**Stable Range**: 0-0 TPS (<1% error)

| TPS Target | Submitted | Success | Failed | Error % | Actual TPS | Status |
|-----------|-----------|---------|--------|---------|-----------|--------|

### Test C3: Triple-BVN-4-Validators

**Configuration**: 4 validators, 3 BVN(s)
**Max Sustained TPS**: 0
**Stable Range**: 0-0 TPS (<1% error)

| TPS Target | Submitted | Success | Failed | Error % | Actual TPS | Status |
|-----------|-----------|---------|--------|---------|-----------|--------|

## Analysis

### Per-BVN Scaling

- **Best single-BVN**: A1 (2v) = 0 TPS
- **3-BVN configurations**:
  - C1: 0 total (0 per-BVN)
  - C2: 0 total (0 per-BVN)
  - C3: 0 total (0 per-BVN)

### RC v1.5.1-breaking Readiness

✗ **Single-BVN needs work**: <8K TPS
✗ **Multi-BVN needs optimization**: <3K TPS per-BVN

---
Generated: 2026-04-14T16:22:56.405836