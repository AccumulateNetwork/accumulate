# Repository Cleanup Plan - Untracked Files Analysis

## Summary of Untracked Files

### 1. **Files to Add to .gitignore**

#### Binary/Executable Files:
- `crosschain_conductor` - Compiled binary (should be ignored)
- **Action:** Add to .gitignore

#### Runtime/PID Files:
- `test/load/devnet.pid` - Process ID file
- **Action:** Add `*.pid` to .gitignore

#### Test Output Directories:
- `test/load/test_results_*/` - Test result directories
- `.devnet-test/` - DevNet test data directory
- **Action:** Add patterns to .gitignore

#### Generated JSON Files:
- `test/load/integration_test_results.json`
- `test/load/monitor_metrics.json`
- `test/load/monitor_output.json`
- `test/load/perf_test_results.json`
- **Action:** These are test outputs, add pattern to .gitignore

#### Temporary Files:
- `snapshot.urls` - Temporary URL list
- **Action:** Add to .gitignore

### 2. **Files to Add to Repository**

#### Documentation (Should be committed):
- `test/load/CODE_REVIEW_COLLECTION_PROOFS.md` - Important code review findings
- `test/load/HOW_TO_RUN_VISUAL_TESTS.md` - User documentation
- `test/load/PROOF_CENTRALIZATION_DESIGN.md` - Design documentation
- `test/load/RUN_INSTRUCTIONS.md` - User instructions
- **Action:** Add these documentation files

#### Test Scripts (Should be committed):
- `test/load/run_full_test_suite.sh` - Test runner script
- `test/load/run_visual_monitor.sh` - Visual monitoring script
- **Action:** Add these scripts

#### Implementation Files (Should be committed):
- `test/load/batch_proof_recovery_fixed.go` - Fixed implementation
- `test/load/optimized_synthetic_sender.go` - Optimized sender
- `test/load/visual_monitor_with_json.go` - Visual monitor with JSON output
- **Action:** Add these Go files

#### Missed Test File:
- `internal/core/execute/v2/crosschain/proof_service_test.go` - Unit test file
- **Action:** This should have been in crosschain_test directory

### 3. **Files to Review/Reorganize**

- `test/load/batch_proof_recovery_fixed.go` vs `test/load/batch_proof_recovery.go`
  - We have both versions, need to determine which is canonical
  - **Action:** Review and keep only the working version

## Recommended .gitignore Additions

```gitignore
# Compiled binaries
crosschain_conductor
/crosschain_conductor

# Runtime files
*.pid
.devnet-test/

# Test outputs
test/load/test_results_*/
test/load/*.json
test/load/integration_test_results.json
test/load/monitor_metrics.json
test/load/monitor_output.json
test/load/perf_test_results.json

# Temporary files
snapshot.urls
*.urls

# DevNet test data
.devnet-test/
```

## Recommended Git Commands

### Step 1: Add Documentation Files
```bash
git add test/load/CODE_REVIEW_COLLECTION_PROOFS.md
git add test/load/HOW_TO_RUN_VISUAL_TESTS.md
git add test/load/PROOF_CENTRALIZATION_DESIGN.md
git add test/load/RUN_INSTRUCTIONS.md
```

### Step 2: Add Test Scripts
```bash
git add test/load/run_full_test_suite.sh
git add test/load/run_visual_monitor.sh
```

### Step 3: Add Implementation Files
```bash
git add test/load/optimized_synthetic_sender.go
git add test/load/visual_monitor_with_json.go
```

### Step 4: Review and Add Fixed Version
```bash
# Check which version is better
diff test/load/batch_proof_recovery.go test/load/batch_proof_recovery_fixed.go

# If fixed version is better:
git add test/load/batch_proof_recovery_fixed.go
```

### Step 5: Move Misplaced Test
```bash
mv internal/core/execute/v2/crosschain/proof_service_test.go \
   internal/core/execute/v2/crosschain_test/

git add internal/core/execute/v2/crosschain_test/proof_service_test.go
```

### Step 6: Update .gitignore
```bash
# Add the patterns listed above to .gitignore
```

### Step 7: Clean Up
```bash
# Remove generated files
rm -f crosschain_conductor
rm -f test/load/devnet.pid
rm -f snapshot.urls
rm -rf .devnet-test/
rm -rf test/load/test_results_*/
rm -f test/load/*.json
```

## Summary

**Files to Commit:** 8-9 files (documentation, scripts, implementations)
**Files to Ignore:** Binary files, test outputs, temporary files
**Total Cleanup:** Will remove ~25 untracked files/directories

This cleanup will:
1. Keep important documentation and implementations in the repository
2. Exclude generated/temporary files via .gitignore
3. Organize test files properly
4. Maintain a clean repository state