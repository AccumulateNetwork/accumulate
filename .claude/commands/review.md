---
name: review
description: "What to review: 'branch' (current branch vs main), 'staged' (staged changes), 'mr' (merge request number), or file path (project)"
arguments:
  - name: target
    description: "What to review: 'branch' (current branch vs main), 'staged' (staged changes), 'mr' (merge request number), or file path"
    required: false
---

## Code Review: {{target}}

### Phase 1: Gather Changes

Determine what to review based on target:
- **branch** or empty: `git diff main...HEAD`
- **staged**: `git diff --staged`
- **mr**: Use `glab mr diff {{target}} --repo accumulatenetwork/accumulate`
- **file path**: Read the specified file

```bash
# Get changes to review
git diff main...HEAD --stat
git diff main...HEAD
```

### Phase 2: Review Criteria

Evaluate each category and assign a rating (A+, A, B+, B, C, D, F):

#### 1. Architecture & Design
- [ ] Clean separation of concerns
- [ ] Appropriate abstractions (not over-engineered)
- [ ] Follows existing patterns in codebase
- [ ] Minimal scope - only changes what's necessary
- [ ] No backwards-compatibility hacks for removed code

#### 2. Code Quality
- [ ] Follows Go naming conventions (CamelCase exports, camelCase private)
- [ ] Appropriate error handling with proper error wrapping
- [ ] No unnecessary panics in production paths
- [ ] Memory efficient (no unnecessary allocations)
- [ ] Thread-safe where applicable (proper use of mutexes, channels)

#### 3. Protocol Compliance
- [ ] Aligns with Accumulate protocol specification
- [ ] Transaction validation is correct
- [ ] Account state changes follow rules
- [ ] No undocumented behavior changes

#### 4. Testing
- [ ] New functionality has tests
- [ ] Edge cases covered
- [ ] Tests actually test the claimed behavior
- [ ] No skipped/disabled tests without justification
- [ ] Tests pass: `go test ./...`

#### 5. Security
- [ ] No command injection vulnerabilities
- [ ] Proper input validation at system boundaries
- [ ] No hardcoded secrets or credentials
- [ ] Signature verification correct
- [ ] No integer overflow in balance calculations

#### 6. Documentation
- [ ] Complex logic has inline comments
- [ ] Public APIs documented
- [ ] No excessive documentation for obvious code
- [ ] Comments explain "why" not "what"

#### 7. Performance
- [ ] No obvious performance regressions
- [ ] Appropriate use of caching
- [ ] No unnecessary allocations in hot paths
- [ ] Database queries efficient

### Phase 3: Generate Review Report

Use this template for the review output:

```markdown
# Code Review: [Title/Branch/MR]

## Executive Summary
**Overall Grade: [A+/A/B+/B/C/D/F]**

[2-3 sentence summary of the changes and overall quality]

## Quantitative Analysis

| Metric | Value |
|--------|-------|
| Files Changed | X |
| Lines Added | +X |
| Lines Removed | -X |
| Tests Added | X |

## Category Ratings

| Category | Grade | Notes |
|----------|-------|-------|
| Architecture | | |
| Code Quality | | |
| Protocol Compliance | | |
| Testing | | |
| Security | | |
| Documentation | | |
| Performance | | |

## Strengths

1. [Strength with specific example]
2. [Strength with specific example]

## Areas for Improvement

### 1. [Issue Title]

**Location**: `file:line`

**Issue**: [Description of the problem]

**Current Code**:
```go
// problematic code
```

**Recommendation**:
```go
// suggested fix
```

**Priority**: [Critical/High/Medium/Low]

## Security Considerations

- [Any security notes]

## Specific File Reviews

### `path/to/file.go`
- Good aspect
- Concern
- Issue that must be fixed

## Recommended Actions

### Must Fix (Before Merge)
1. [Critical item]

### Should Fix (Before Merge)
1. [High priority item]

### Consider (Future Improvement)
1. [Nice-to-have]

## Verdict

**[APPROVED / APPROVED WITH CHANGES / NEEDS WORK / REJECTED]**

[Final notes]
```

### Phase 4: Post Review

1. If reviewing an MR, post the review as a comment:
   ```bash
   glab mr note {{mr_number}} --message "[review content]" --repo accumulatenetwork/accumulate
   ```

2. Summarize critical findings for the developer

---

**Starting code review now...**
