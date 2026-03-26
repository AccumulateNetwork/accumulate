---
name: beastmode
description: "Optional scope limiter: 'compile' (compilation only), 'test' (test failures), 'ketor' (ketor build/test only), 'cargo' (cargo build/test only), 'all' (default - everything) (project)"
arguments:
  - name: scope
    description: "Optional scope limiter: 'compile' (compilation only), 'test' (test failures), 'all' (default - everything)"
    required: false
---

## BEASTMODE: Aggressive Blocker Resolution

**Scope**: {{scope}}

### Ground Rules (CRITICAL - DO NOT VIOLATE)

1. **FIXES ONLY** - Only fix bugs in existing code. Do NOT implement new features.
2. **PROTOCOL COMPLIANCE** - All fixes must align with Accumulate protocol specifications
3. **NO SCOPE CREEP** - If a fix requires new features or spec changes, STOP and create a GitLab issue instead
4. **EXISTING ISSUES** - Before fixing, search GitLab for existing issues. If found, link to it and skip (unless fix is trivial)
5. **DOCUMENT EVERYTHING** - Create GitLab issues for any blockers that require significant work or are out of scope

### Phase 1: Discovery - Compilation

**Skip this phase if scope is 'test'**

1. **Build the project** and capture all compilation errors:
   ```bash
   go build ./... 2>&1 | tee /tmp/beastmode_build.log
   ```

2. **Run Go vet** for static analysis:
   ```bash
   go vet ./... 2>&1 | tee /tmp/beastmode_vet.log
   ```

### Phase 2: Discovery - Tests

**Skip this phase if scope is 'compile'**

1. **Run all tests** and capture failures:
   ```bash
   go test ./... 2>&1 | tee /tmp/beastmode_test.log
   ```

2. **Run with race detection** (for concurrency issues):
   ```bash
   go test -race ./... 2>&1 | tee /tmp/beastmode_race.log
   ```

### Phase 3: Categorize All Blockers

Categorize all blockers from Phases 1-2 into:

| Category | Source | Priority |
|----------|--------|----------|
| Compilation errors | go build | 1 (Critical) |
| Go vet warnings | go vet | 2 (High) |
| Test failures | go test | 3 (High) |
| Race conditions | go test -race | 4 (Medium) |

### Phase 4: Triage

For EACH blocker identified:

1. **Search GitLab** for existing issues:
   ```bash
   glab issue list --repo accumulatenetwork/accumulate --search "<error keyword>" | head -20
   ```

2. **Classify the blocker**:
   - If existing issue found → Note issue number, skip fix (unless trivial)
   - If fix requires new feature → Create GitLab issue, skip fix
   - If fix requires spec change → Create GitLab issue, skip fix
   - If fix is within existing code → Proceed to fix

3. **Check protocol compliance**:
   - Read relevant sections of protocol documentation
   - Ensure fix aligns with documented behavior

### Phase 5: Fix Loop

**IMPORTANT**: Use TodoWrite to track all blockers and their status!

For each fixable blocker (in priority order from Phase 3):

1. **Understand the error** - Read the full error message and context
2. **Locate the source** - Find the exact file and line causing the issue
3. **Implement minimal fix** - Change only what's necessary, avoid refactoring
4. **Verify fix** - Rebuild/retest to confirm the specific error is resolved
5. **Check for regressions** - Ensure fix didn't break other things
6. **Commit incrementally** - Small, focused commits for each fix

### Phase 6: Issue Creation

For blockers that CANNOT be fixed (require new features or spec changes):

1. **Create detailed GitLab issue** with:
   - Clear title describing the blocker
   - Error message and reproduction steps
   - Root cause analysis
   - Why it can't be fixed without new features
   - Suggested approach (if known)
   - Appropriate labels (bug, blocker, etc.)

2. **Link related issues** if any exist

### Phase 7: Summary

After all passes complete:

1. **Report fixes made** - List all errors fixed with commit hashes
2. **Report issues created** - List all GitLab issues created for unfixable blockers
3. **Report skipped items** - List items with existing GitLab issues
4. **Health assessment** using these metrics:

   | Metric | Target | Status |
   |--------|--------|--------|
   | Go Build | PASS | |
   | Go Vet | PASS | |
   | Go Test | 100% | |
   | Race Detection | PASS | |

   **Current status must include:**
   - Go build: PASS/FAIL
   - Go vet: PASS/FAIL
   - Go tests: X/Y passed (Z%)
   - List of failing items with error categories

### Scope Reference

| Scope Value | Phases Run |
|-------------|------------|
| `all` (default) | All phases |
| `compile` | Phase 1 only (build + vet) |
| `test` | Phase 2 only (tests) |

### Termination Conditions

STOP beastmode when ANY of these are true:
- All blockers are fixed or have GitLab issues
- A fix would require implementing a new protocol feature
- A fix would require changing the protocol specification
- More than 10 GitLab issues have been created (too many blockers - need human review)
- A single fix is taking more than 30 minutes (likely needs design discussion)

### Quick Commands Reference

```bash
# Build everything
go build ./...

# Run all tests
go test ./...

# Run specific package tests
go test -v ./internal/core/...

# Run with race detection
go test -race ./...

# Find specific error patterns in logs
grep -r "error:" /tmp/beastmode_*.log
```

---

**ENTERING BEASTMODE NOW...**

Begin with Phase 1: Discovery. Use TodoWrite extensively to track progress.
