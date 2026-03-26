---
name: test
description: "Test mode: 'all', 'branch', 'cargo', 'ketor', 'fix' (iterate until pass), or test name pattern (project)"
arguments:
  - name: mode
    description: "Test mode: 'all', 'branch', 'fix' (iterate until pass), or test name pattern"
    required: false
---

## Test Runner: {{mode}}

### Configuration

**Timeout**: 600000ms (10 minutes) - Required for full test suite

### Mode Selection

Determine test scope based on mode argument:

| Mode | Description |
|------|-------------|
| `all` or empty | Run all tests |
| `branch` | Run tests related to current branch changes |
| `fix` | Iterate: test -> fix failures -> retest until all pass |
| `<pattern>` | Run tests matching the pattern |

---

## Mode: ALL (default)

Run complete test suite:

```bash
# Run all Go tests
go test ./...

# Run with verbose output
go test -v ./...

# Run with race detection
go test -race ./...
```

---

## Mode: BRANCH

Run tests related to files changed in the current branch:

```bash
# Get changed files
CHANGED_FILES=$(git diff main...HEAD --name-only)

# Find related test files
echo "=== Files Changed ==="
echo "$CHANGED_FILES"

# For each changed .go file, find corresponding tests
echo "=== Related Tests ==="
for f in $CHANGED_FILES; do
  if [[ "$f" == *.go ]] && [[ "$f" != *_test.go ]]; then
    # Find the package directory
    DIR=$(dirname "$f")
    # Run tests for this package
    echo "Testing package: ./$DIR"
    go test -v "./$DIR/..." 2>&1 || true
  fi
done

# For changed test files, run them directly
for f in $CHANGED_FILES; do
  if [[ "$f" == *_test.go ]]; then
    DIR=$(dirname "$f")
    echo "Running tests in: ./$DIR"
    go test -v "./$DIR/..." 2>&1 || true
  fi
done
```

---

## Mode: FIX (Iterate Until Pass)

**CRITICAL**: This mode will iterate test -> fix -> test until all tests pass or max iterations reached.

### Ground Rules
1. **Max 10 iterations** - Stop and report if not fixed
2. **Only fix test failures** - Don't refactor or improve
3. **Create issues for blockers** - If fix requires new features, create GitLab issue

### Fix Loop

```
ITERATION=0
MAX_ITERATIONS=10

while [ $ITERATION -lt $MAX_ITERATIONS ]; do
  ITERATION=$((ITERATION+1))
  echo "=== Iteration $ITERATION of $MAX_ITERATIONS ==="

  # Run tests and capture output
  go test ./... 2>&1 | tee /tmp/test_output.log

  # Check if all passed
  if grep -q "^ok" /tmp/test_output.log && ! grep -q "FAIL" /tmp/test_output.log; then
    echo "All tests passed!"
    break
  fi

  # Extract failures and fix them
  # ... analyze and fix ...
done
```

### For Each Failure:

1. **Identify the failure**
   - Test name and location
   - Error message
   - Expected vs actual

2. **Analyze root cause**
   - Read the test code
   - Read the implementation being tested
   - Understand what's wrong

3. **Apply minimal fix**
   - Fix only what's broken
   - Don't refactor surrounding code
   - Preserve existing behavior

4. **Verify fix**
   - Run the specific failing test
   - If passes, continue to next failure
   - If still fails, analyze again

5. **Track progress**
   - Use TodoWrite to track each failure
   - Mark as complete when fixed
   - Note any that require GitLab issues

### Termination Conditions

STOP the fix loop when:
- All tests pass
- Max iterations (10) reached
- A fix requires new features
- Same test fails 3 times in a row (likely deeper issue)

### Final Report

```markdown
## Test Fix Results

### Summary
- **Iterations**: X
- **Tests Fixed**: Y
- **Tests Remaining**: Z

### Fixed Tests
| Test | Issue | Fix Applied |
|------|-------|-------------|
| test_name | description | what was changed |

### Remaining Failures (if any)
| Test | Issue | Blocker |
|------|-------|---------|
| test_name | description | Requires issue #XXX |

### Verification
- Go test: PASS/FAIL
- Go vet: PASS/FAIL
```

---

## Pattern Matching Mode

If mode doesn't match known modes, treat as test pattern:

```bash
# Run tests matching the pattern
go test -v -run "{{mode}}" ./...

# Or run a specific package
go test -v "./{{mode}}/..."
```

---

## Quick Reference

| Command | Description |
|---------|-------------|
| `/test` | Run all tests |
| `/test all` | Run all tests (explicit) |
| `/test branch` | Tests for current branch changes |
| `/test fix` | Iterate: fix failures until all pass |
| `/test TestSendTokens` | Run tests matching "TestSendTokens" |
| `/test ./internal/core/...` | Run tests in specific package |

---

**Starting test run now...**

Use TodoWrite to track test progress and failures. Report summary when complete.
