# Implementation Workflow for Issue #3660: Collection Proofs

## Overview
This guide shows how to use the design-reviewer agent while implementing collection proofs.

## Workflow Steps

### 1. Initial Setup (COMPLETED ✅)
```bash
# Create design document
python3 tools/design-reviewer/design_reviewer.py create --issue 3660

# Create feature branch
git checkout -b 3660-activate-collection-proofs

# Run initial review (baseline)
python3 tools/design-reviewer/design_reviewer.py review \
  --design docs/designs/issue-3660-design.md \
  --branch 3660-activate-collection-proofs
```

### 2. Implementation Phase

#### Step 2.1: Fix Race Conditions
```bash
# Edit proof_service.go to use atomic operations
vim internal/core/execute/v2/crosschain/proof_service.go

# Run review to check compliance
python3 tools/design-reviewer/design_reviewer.py review \
  --design docs/designs/issue-3660-design.md \
  --branch 3660-activate-collection-proofs

# Check the compliance score - should increase as you implement
```

#### Step 2.2: Fix Memory Leaks
```bash
# Edit batch_proof_recovery.go to add cleanup
vim internal/core/execute/v2/crosschain/batch_proof_recovery.go

# Review progress
python3 tools/design-reviewer/design_reviewer.py review \
  --design docs/designs/issue-3660-design.md \
  --branch 3660-activate-collection-proofs
```

#### Step 2.3: Add Configuration
```bash
# Add configuration structure
vim internal/core/execute/v2/crosschain/conductor.go

# Check compliance
python3 tools/design-reviewer/design_reviewer.py review \
  --design docs/designs/issue-3660-design.md \
  --branch 3660-activate-collection-proofs
```

### 3. Testing Phase

#### Step 3.1: Local Devnet Testing (Production Parity)
```bash
# Start local devnet for realistic testing
./scripts/devnet.sh start --validators 3 --partitions 2

# Wait for devnet to be ready
./scripts/devnet.sh wait

# Run integration tests against devnet
go test -tags=devnet ./internal/core/execute/v2/crosschain/... \
  -run TestCollectionProofs \
  -devnet.url=http://localhost:26657

# Run load test (10,000 TPS target)
go test -tags=devnet ./test/load/... \
  -run TestCollectionProofLoad \
  -devnet.url=http://localhost:26657 \
  -load.tps=10000 \
  -load.duration=10m

# Document results
python3 tools/design-reviewer/design_reviewer.py update \
  --design docs/designs/issue-3660-design.md \
  --changes "Devnet tests: achieved 10,247 TPS with collection proofs enabled"
```

#### Step 3.2: CI Testing (Fast Feedback)
```bash
# Unit tests with race detection (no network needed)
go test -race ./internal/core/execute/v2/crosschain/...

# Simulator-based integration tests (fast)
go test ./test/simulator/... -run TestCollectionProofs

# Mock-based tests
go test ./internal/core/execute/v2/crosschain/... \
  -run TestCollectionProofMock

# Document results
python3 tools/design-reviewer/design_reviewer.py update \
  --design docs/designs/issue-3660-design.md \
  --changes "CI tests: all unit and simulator tests passing"
```

#### Step 3.3: Extended Devnet Testing
```bash
# 24-hour memory leak test on devnet
go test -tags=devnet ./internal/core/execute/v2/crosschain/... \
  -run TestCollectionProofMemory \
  -timeout=24h \
  -memprofile=mem.prof \
  -devnet.url=http://localhost:26657

# Analyze memory profile
go tool pprof mem.prof

# Update design with results
python3 tools/design-reviewer/design_reviewer.py update \
  --design docs/designs/issue-3660-design.md \
  --changes "24-hour devnet test: memory stable, no leaks detected"
```

### 4. Review Checkpoints

The design-reviewer will check for these key implementations:

| Function | Purpose | Compliance Check |
|----------|---------|------------------|
| `batchTransactionsForProof` | Group transactions by destination | ✓ Function exists |
| `CreateCollectionProof` | Generate collection proof | ✓ Context timeout |
| `processBatchRecovery` | Handle recovery with cleanup | ✓ Defer cleanup |
| `ConductorConfig` | Configuration structure | ✓ All flags present |

### 5. Monitoring Compliance

```bash
# Check current compliance at any time
python3 tools/design-reviewer/design_reviewer.py review \
  --design docs/designs/issue-3660-design.md \
  --branch 3660-activate-collection-proofs

# View detailed report
cat docs/designs/issue-3660-review.md
```

### 6. Pre-Commit Validation

```bash
# Before committing, ensure high compliance
COMPLIANCE=$(python3 tools/design-reviewer/design_reviewer.py review \
  --design docs/designs/issue-3660-design.md \
  --branch 3660-activate-collection-proofs | grep "Compliance Score" | cut -d: -f2 | cut -d% -f1)

if (( $(echo "$COMPLIANCE < 90" | bc -l) )); then
  echo "❌ Compliance too low: ${COMPLIANCE}%"
  echo "Review the report: docs/designs/issue-3660-review.md"
  exit 1
fi

echo "✅ Ready to commit with ${COMPLIANCE}% compliance"
```

### 7. Commit with Design Reference

```bash
# Commit with reference to design compliance
git commit -m "fix: implement collection proofs with race condition fixes

Implements collection proof activation per design doc #3660:
- Fixed race conditions with atomic operations
- Added defer cleanup for memory leak prevention
- Implemented configuration flags for gradual rollout
- Added context timeout for proof generation

Design compliance: 95%
Fixes #3660"
```

## Expected Compliance Progression

| Stage | Expected Compliance | Key Indicators |
|-------|-------------------|----------------|
| Initial | 0% | No implementation |
| Race fixes | 25% | Atomic operations added |
| Memory fixes | 50% | Cleanup implemented |
| Configuration | 75% | Config struct complete |
| Full implementation | 95%+ | All functions present |

## Common Issues and Solutions

### Issue: Function Not Found
**Solution**: Ensure function signature matches design exactly
```go
// Design specifies:
func (cc *CrossChainConductor) batchTransactionsForProof(
    messages []messaging.Message,
) []ProofBatch

// Implementation must match exactly
```

### Issue: Missing Context
**Solution**: Add context parameter to functions
```go
// Before
func processBatchRecovery(req *BatchRecoveryRequest) error

// After (matches design)
func processBatchRecovery(ctx context.Context, req *BatchRecoveryRequest) error
```

### Issue: No Cleanup
**Solution**: Add defer statements
```go
func processBatchRecovery(ctx context.Context, req *BatchRecoveryRequest) error {
    brm.mu.Lock()
    session := brm.activeRecovery[sessionKey]
    brm.mu.Unlock()
    
    // ADD THIS:
    defer func() {
        brm.mu.Lock()
        delete(brm.activeRecovery, sessionKey)
        brm.mu.Unlock()
    }()
    
    // ... rest of function
}
```

## Testing Environment Setup

### Local Devnet Requirements
```bash
# Ensure devnet script is executable
chmod +x ./scripts/devnet.sh

# Start devnet with specific configuration for collection proof testing
./scripts/devnet.sh start \
  --validators 3 \
  --partitions 2 \
  --enable-collection-proofs \
  --collection-threshold 2

# Verify devnet is running
./scripts/devnet.sh status

# View logs for debugging
./scripts/devnet.sh logs --follow
```

### Test Data Generation
```bash
# Generate test transactions for collection proof testing
go run ./test/tools/gentx \
  --count 1000 \
  --destinations 10 \
  --output test_transactions.json

# Use test data in load testing
go test -tags=devnet ./test/load/... \
  -run TestCollectionProofLoad \
  -test.data=test_transactions.json
```

## Integration with CI/CD

### GitLab CI Configuration
```yaml
# Fast CI tests using simulators
unit-tests:
  stage: test
  script:
    - go test -race ./internal/core/execute/v2/crosschain/...
    - go test ./test/simulator/... -run TestCollectionProofs

# Design compliance check
design-review:
  stage: test
  script:
    - python3 tools/design-reviewer/design_reviewer.py review \
        --design docs/designs/issue-3660-design.md \
        --branch $CI_COMMIT_REF_NAME
    - COMPLIANCE=$(grep "Compliance Score" docs/designs/issue-3660-review.md | cut -d: -f2 | cut -d% -f1)
    - if [ "$COMPLIANCE" -lt "90" ]; then exit 1; fi
  artifacts:
    reports:
      - docs/designs/issue-3660-review.md

# Nightly devnet tests (separate pipeline)
devnet-integration:
  stage: integration
  when: scheduled
  script:
    - ./scripts/devnet.sh start --validators 3 --partitions 2
    - ./scripts/devnet.sh wait
    - go test -tags=devnet ./internal/core/execute/v2/crosschain/... \
        -devnet.url=http://localhost:26657
    - ./scripts/devnet.sh stop
```

## Next Steps After Implementation

1. **Update Design Status**
```bash
# Change status from Draft to Implemented
vim docs/designs/issue-3660-design.md
# Update Status: Implemented
```

2. **Generate Final Report**
```bash
python3 tools/design-reviewer/design_reviewer.py review \
  --design docs/designs/issue-3660-design.md \
  --branch 3660-activate-collection-proofs > final-report.txt
```

3. **Create PR with Report**
```bash
gh pr create \
  --title "feat: activate collection proofs (#3660)" \
  --body "$(cat final-report.txt)" \
  --assignee @me
```

## Support

For issues with the design-reviewer:
- Check `tools/design-reviewer/README.md`
- Review example in `tools/design-reviewer/example-workflow.md`
- Ensure Python 3.6+ is installed