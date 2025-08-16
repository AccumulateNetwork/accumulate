# Example Workflow: Issue #1234 - Optimize Transaction Processing

This example demonstrates using the design-reviewer agent for a real issue.

## Issue Description
**Issue #1234**: Transaction processing is slow during peak hours. Need to optimize the transaction validation and execution pipeline to handle 10x current throughput.

## Step 1: Create Initial Design

```bash
$ python3 tools/design-reviewer/design_reviewer.py create \
    --issue 1234 \
    --title "Optimize Transaction Processing" \
    --body "Transaction processing is slow during peak hours. Need to optimize validation and execution pipeline for 10x throughput."

✅ Design document created: docs/designs/issue-1234-design.md
📝 Next step: Edit the design document to add specifications
```

## Step 2: Complete Design Document

Edit `docs/designs/issue-1234-design.md`:

```markdown
# Issue Design Document: #1234

## Issue Summary
- **Issue ID**: #1234
- **Title**: Optimize Transaction Processing
- **Status**: Draft
- **Created**: 2025-08-16
- **Last Updated**: 2025-08-16

## Problem Statement
Transaction processing is slow during peak hours (>1000 TPS). Current implementation processes transactions sequentially, causing bottlenecks. Need to optimize validation and execution pipeline to handle 10,000 TPS.

## Design Specification

### Architecture Overview
Implement parallel transaction processing with:
1. Transaction batching
2. Parallel validation
3. Optimistic execution
4. Result caching

### Component Definitions

#### Affected Files
```
internal/core/execute/v2/block/executor.go
internal/core/execute/v2/block/transaction.go
internal/core/validation/validator.go
```

#### New Components
```
internal/core/execute/v2/block/batch_processor.go
internal/core/execute/v2/block/parallel_validator.go
internal/core/cache/tx_cache.go
```

### API Contracts

#### Functions/Methods
```go
// Batch processor for parallel execution
func NewBatchProcessor(size int) *BatchProcessor {
    // Initialize with batch size
}

func (bp *BatchProcessor) ProcessBatch(txs []*Transaction) ([]Result, error) {
    // Process transaction batch in parallel
}

// Parallel validator
func ValidateParallel(txs []*Transaction, workers int) ([]ValidationResult, error) {
    // Validate transactions using worker pool
}

// Transaction cache
func NewTxCache(capacity int) *TxCache {
    // Initialize cache with capacity
}
```

#### Data Structures
```go
type BatchProcessor struct {
    batchSize  int
    workers    int
    resultChan chan Result
}

type ValidationResult struct {
    TxID    string
    Valid   bool
    Error   error
    Cached  bool
}

type TxCache struct {
    capacity int
    entries  map[string]*CacheEntry
    mutex    sync.RWMutex
}
```

### Data Flow
1. Transactions arrive in blocks
2. Group into batches of 100
3. Parallel validation (10 workers)
4. Check cache for repeat transactions
5. Execute valid transactions in parallel
6. Aggregate results
7. Update state atomically

### Error Handling
- **Invalid Transaction**: Mark invalid, continue processing others
- **Worker Failure**: Retry with exponential backoff
- **Cache Miss**: Process normally, update cache
- **State Conflict**: Retry transaction with lock

### Testing Requirements
- [ ] Benchmark: 10,000 TPS sustained for 1 hour
- [ ] Unit tests for BatchProcessor
- [ ] Unit tests for ParallelValidator
- [ ] Integration test for full pipeline
- [ ] Stress test with 100,000 transactions
- [ ] Test cache hit ratio > 30%
- [ ] Test error recovery mechanisms

## Implementation Checklist
- [ ] Implement BatchProcessor
- [ ] Implement ParallelValidator
- [ ] Implement TxCache
- [ ] Integrate with existing executor
- [ ] Add metrics collection
- [ ] Performance benchmarks
- [ ] Documentation updated

## Acceptance Criteria
1. Process 10,000 TPS sustained
2. P99 latency < 100ms
3. Zero transaction loss
4. Cache hit ratio > 30%
5. All tests passing

## Change Log
- 2025-08-16: Initial design created
```

## Step 3: Implement Features

```bash
$ git checkout -b feature/issue-1234-optimize-tx

# Implement the batch processor
$ vim internal/core/execute/v2/block/batch_processor.go

# Implement parallel validator
$ vim internal/core/execute/v2/block/parallel_validator.go

# Implement transaction cache
$ vim internal/core/cache/tx_cache.go
```

## Step 4: First Review (Partial Implementation)

```bash
$ python3 tools/design-reviewer/design_reviewer.py review \
    --design docs/designs/issue-1234-design.md \
    --branch feature/issue-1234-optimize-tx

📊 Compliance Score: 45.0%
✅ Passed: 9/20 checks
🚨 Critical Issues: 2
📄 Report saved: docs/designs/issue-1234-review.md
```

Review report shows:

```markdown
# Code Review Report

## Design Compliance Summary
- **Overall Compliance**: 45.0%
- **Checks Passed**: 9/20
- **Critical Issues**: 2
- **Warnings**: 0

## Detailed Analysis

### ✅ Compliant Areas
- Struct 'BatchProcessor' implemented
- Struct 'TxCache' implemented
- Function 'NewBatchProcessor' implemented
- Function 'NewTxCache' implemented
- Test files added: 2

### ❌ Missing Implementation
- Function 'ProcessBatch' not found
- Function 'ValidateParallel' not found
- Struct 'ValidationResult' not found
- Missing integration tests

### 🚨 Critical Issues
- No test files found for parallel_validator
- Performance benchmarks not implemented

## Recommendations
- ❌ Code does not meet design requirements
- Significant work needed to match design specification

### Required Actions:
1. Fix: No test files found for parallel_validator
2. Fix: Performance benchmarks not implemented
```

## Step 5: Complete Implementation

```bash
# Add missing functions
$ vim internal/core/execute/v2/block/batch_processor.go

# Add validation result struct
$ vim internal/core/execute/v2/block/parallel_validator.go

# Add tests
$ vim internal/core/execute/v2/block/parallel_validator_test.go

# Add benchmarks
$ vim internal/core/execute/v2/block/benchmark_test.go
```

## Step 6: Final Review

```bash
$ python3 tools/design-reviewer/design_reviewer.py review \
    --design docs/designs/issue-1234-design.md \
    --branch feature/issue-1234-optimize-tx

📊 Compliance Score: 95.0%
✅ Passed: 19/20 checks
🚨 Critical Issues: 0
📄 Report saved: docs/designs/issue-1234-review.md
```

Final report:

```markdown
# Code Review Report

## Design Compliance Summary
- **Overall Compliance**: 95.0%
- **Checks Passed**: 19/20
- **Critical Issues**: 0
- **Warnings**: 1

## Detailed Analysis

### ✅ Compliant Areas
- Function 'NewBatchProcessor' implemented
- Function 'ProcessBatch' implemented
- Function 'ValidateParallel' implemented
- Function 'NewTxCache' implemented
- Struct 'BatchProcessor' implemented
- Struct 'ValidationResult' implemented
- Struct 'TxCache' implemented
- Test files added: 4
- Benchmark tests implemented

### ⚠️ Warnings
- Documentation not fully updated

## Recommendations
- ✅ Code meets design requirements
- Consider minor improvements listed above
```

## Step 7: Run Tests and Benchmarks

```bash
# Run unit tests
$ go test ./internal/core/execute/v2/block/...
PASS
ok  internal/core/execute/v2/block  2.345s

# Run benchmarks
$ go test -bench=. ./internal/core/execute/v2/block/...
BenchmarkBatchProcessor-8       1000     1045632 ns/op
BenchmarkParallelValidator-8    2000      652341 ns/op
BenchmarkTxCache-8              5000000      234 ns/op
PASS

# Verify 10,000 TPS
$ go run test/stress/tx_stress.go --tps 10000 --duration 60s
✅ Sustained 10,247 TPS for 60 seconds
✅ P99 latency: 87ms
✅ Zero transaction loss
```

## Step 8: Update Design with Performance Results

```bash
$ python3 tools/design-reviewer/design_reviewer.py update \
    --design docs/designs/issue-1234-design.md \
    --changes "Achieved 10,247 TPS with P99 latency of 87ms. Cache hit ratio: 34%"

✅ Design document updated
```

## Step 9: Commit and Create PR

```bash
# Stage all changes
$ git add .

# Commit with reference to design
$ git commit -m "feat: optimize transaction processing for 10x throughput

Implements parallel transaction processing per design doc #1234:
- Batch processing with configurable size
- Parallel validation using worker pools
- Transaction result caching
- Achieved 10,247 TPS (target: 10,000)
- P99 latency: 87ms (target: <100ms)

Design doc: docs/designs/issue-1234-design.md
Closes #1234"

# Push branch
$ git push origin feature/issue-1234-optimize-tx

# Create PR
$ gh pr create \
    --title "Optimize transaction processing (10x throughput)" \
    --body "Implementation per design doc issue-1234-design.md. Compliance: 95%" \
    --assignee @me
```

## Step 10: PR Review Process

The reviewer can verify:

```bash
# Check design compliance
$ python3 tools/design-reviewer/design_reviewer.py review \
    --design docs/designs/issue-1234-design.md \
    --pr 567

# View the generated report
$ cat docs/designs/issue-1234-review.md
```

## Summary

This workflow demonstrates:

1. **Design-First Approach**: Created detailed design before coding
2. **Iterative Development**: Multiple review cycles during implementation
3. **Compliance Tracking**: Measured progress against design (45% → 95%)
4. **Performance Validation**: Verified acceptance criteria with benchmarks
5. **Documentation**: Maintained design doc with actual results
6. **Traceability**: Clear link from issue → design → implementation → PR

## Key Benefits Observed

- **Early Detection**: Found missing components at 45% compliance
- **Clear Requirements**: Design doc prevented scope creep
- **Measurable Progress**: Compliance score showed completion status
- **Quality Assurance**: Ensured all design aspects implemented
- **Documentation**: Design doc serves as permanent reference

## Metrics from Example

- **Time to Design**: 30 minutes
- **Implementation Time**: 4 hours
- **Review Cycles**: 2
- **Final Compliance**: 95%
- **Performance Achievement**: 102% of target
- **Tests Written**: 4 test files, 23 test cases
- **Documentation**: Complete design + review reports