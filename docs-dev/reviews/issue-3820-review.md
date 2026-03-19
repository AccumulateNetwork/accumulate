# Review Report: Migrate exp/light logger

## Decision: APPROVED

## Summary

The implementation correctly addresses issue #3820. The research identified that the `exp/light` package's `Logger` function was deprecated and unused, and the CometBFT logger import was unnecessary since the package already uses `log/slog` directly. The implementation removed the dead code rather than migrating to the `logging.FromCometBFT()` wrapper, which was the correct decision.

## Fresh Eyes Test

### Points of Confusion

None significant. The documentation clearly explains:
- The original issue title suggested wrapper migration
- The research discovered removal was the correct approach
- The validation confirms the implementation followed the research

### Unstated Assumptions

| Assumption | Impact | Acceptable? |
|------------|--------|-------------|
| Reader knows what CometBFT is | Low | Yes - context evident from code |
| Reader understands slog vs CometBFT logger | Low | Yes - documented in research |
| No callers depend on the Logger function | Medium | Yes - explicitly addressed in research (deprecated, no-op) |

## Alternative Interpretations

| Step | Could Be Misread As | Clarification Needed |
|------|---------------------|---------------------|
| "Remove the deprecated Logger function" | Could remove wrong function | No - research includes exact code with line numbers |
| Issue title "Migrate to FromCometBFT wrapper" | Actually implement wrapper | No - research clearly explains why removal is correct |

## Code Verification

| File | Expected State | Verified |
|------|---------------|----------|
| `exp/light/client.go` | No CometBFT import | ✓ Lines 9-25 show no `cometbft/libs/log` |
| `exp/light/client.go` | No Logger function | ✓ No Logger option function present |
| `exp/light/sync.go` | Uses log/slog | ✓ Line 14 imports `log/slog` |
| `internal/logging/compat.go` | FromCometBFT exists | ✓ Lines 25-35 confirmed |
| Client struct | No logger field | ✓ Lines 28-34 confirmed |

## Known Pitfalls Coverage

No error log (`docs-dev/errors/error-log.md`) exists for this project. The following potential pitfalls were considered:

| Pitfall | Addressed? | Notes |
|---------|-----------|-------|
| Breaking API changes | Yes | Logger was deprecated no-op, safe to remove |
| Missing imports after removal | Yes | Build verified successful |
| Incomplete migration | N/A | No migration needed - removal was correct |

## Build Verification

```
go build ./exp/light/...  - PASSED
go build ./...            - PASSED
go test ./exp/light/... -v -short - PASSED
```

## Final Checklist

- [x] Self-contained (no external knowledge needed)
- [x] All examples verified against actual code
- [x] No high-risk ambiguities
- [x] Ready for human review

## Implementation Quality

### Commit Structure

1. `68d164382` - Research documented
2. `cd45a2ae5` - Implementation (removed Logger function and CometBFT import)
3. `43f8f9fef` - Validation completed

### Code Quality

- Clean removal with no orphaned code
- No breaking changes (function was deprecated and did nothing)
- Package continues to function correctly with slog

## Required Changes Before Approval

None. The implementation is complete and correct.

## Notes for Human Reviewer

The actual implementation differs from the issue title ("use logging.FromCometBFT() wrapper") because the research discovered:

1. The Logger function was already deprecated
2. The Logger function did nothing (returned nil, stored nothing)
3. The Client struct had no logger field
4. All actual logging used `log/slog` directly

Therefore, removing the dead code was the correct solution rather than wiring up a wrapper that would never be used.
