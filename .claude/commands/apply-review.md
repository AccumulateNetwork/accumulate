---
name: apply-review
description: "Where to get the review: 'mr' (latest MR comment), MR number, or 'last' (last review in conversation) (project)"
arguments:
  - name: source
    description: "Where to get the review: 'mr' (latest MR comment), MR number, or 'last' (last review in conversation)"
    required: false
---

## Apply Code Review Recommendations

**Source**: {{source}}

### Ground Rules

1. **FIXES ONLY** - Only apply recommended fixes, do NOT add extra improvements
2. **PROTOCOL COMPLIANCE** - All fixes must align with Accumulate protocol specifications
3. **NO SCOPE CREEP** - If a fix requires new features, create a GitLab issue instead
4. **MINIMAL CHANGES** - Change only what's necessary, avoid refactoring beyond the recommendation
5. **TEST AFTER EACH FIX** - Verify fixes don't introduce regressions

### Phase 1: Gather Review

Determine source of code review:
- **mr** or empty: Get latest comment from current branch's MR
- **[number]**: Get comments from specific MR
- **last**: Use the last code review from this conversation

```bash
# Get current branch and find associated MR
BRANCH=$(git branch --show-current)
glab mr list --source-branch="$BRANCH" --repo accumulatenetwork/accumulate
# Then fetch comments from the MR
```

### Phase 2: Extract Recommendations

Parse the code review and categorize recommendations:

1. **Must Fix (Critical)** - Apply these first, required before merge
2. **Should Fix (High)** - Apply these second, strongly recommended
3. **Consider (Low)** - Optional, apply only if explicitly requested

Use TodoWrite to track each recommendation:
```
Example todos:
- [Must Fix] Add error handling in ExecuteTransaction (file:line)
- [Must Fix] Fix memory leak in context cleanup
- [Should Fix] Improve error message clarity
- [Consider] Add performance optimization for hot path
```

### Phase 3: Apply Fixes

For EACH recommendation (in priority order):

#### Step 1: Understand the Issue
- Read the exact location mentioned in the review
- Understand the current code and the problem
- Review the suggested fix

#### Step 2: Verify Against Protocol
```bash
# Check if fix aligns with protocol
# Read relevant documentation if needed
```

- If fix requires spec change → SKIP, note as blocked
- If fix requires new feature → SKIP, create GitLab issue
- If fix is straightforward → Proceed

#### Step 3: Apply the Fix
- Make minimal changes as recommended
- Follow existing code patterns
- Preserve code style and formatting

#### Step 4: Verify the Fix
```bash
# Run targeted test for the changed code
go test -v -run <relevant_test_pattern> ./...

# Check for compilation errors
go build ./...
```

- If test fails → Investigate and fix
- If new errors introduced → Revert and reassess
- If passes → Mark todo as complete, move to next

### Phase 4: Final Verification

After all fixes applied:

```bash
# Full test suite
go test ./...

# Static analysis
go vet ./...

# Format check (if gofmt is used)
gofmt -l .
```

All must pass before proceeding.

### Phase 5: Commit Changes

Group related fixes into logical commits:

```bash
git add -A
git commit -m "$(cat <<'EOF'
Address code review feedback

- [Fix 1 description]
- [Fix 2 description]
- [Fix 3 description]

Generated with [Claude Code](https://claude.ai/code)

Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)"
```

### Phase 6: Report

Generate summary of applied fixes:

```markdown
## Code Review Fixes Applied

### Applied Successfully
| Priority | Issue | Location | Status |
|----------|-------|----------|--------|
| Must Fix | [description] | file:line | Fixed |
| Should Fix | [description] | file:line | Fixed |

### Skipped (Requires Further Action)
| Priority | Issue | Reason | Action |
|----------|-------|--------|--------|
| Consider | [description] | Requires new feature | Created issue #XXX |

### Verification Results
- Go build: PASS
- Go vet: PASS
- Go test: PASS (X tests)

### Next Steps
1. Push changes: `git push`
2. Request re-review if significant changes made
3. Address any skipped items in follow-up issues
```

### Phase 7: Push (if requested)

```bash
git push
```

Optionally request re-review:
```bash
glab mr note [MR_NUMBER] --message "Applied review feedback. Ready for re-review." --repo accumulatenetwork/accumulate
```

---

**Applying code review recommendations now...**

Begin by gathering the review source and extracting recommendations. Use TodoWrite to track each fix.
