# Accumulate Technical Documentation

## BPT (Binary Patricia Tree) Documentation

### Core Documentation

1. **[BPT Complete Guide](bpt-complete-guide.md)** - Comprehensive guide to understanding the BPT
   - What problem it solves
   - Dual-purpose design (validation + indexing)
   - How data flows from URL to transactions
   - Common misconceptions addressed

2. **[BPT Implementation Deep Dive](bpt-implementation-deep-dive.md)** - Technical implementation details
   - Node structures and navigation
   - Account representation
   - Hash computation
   - Integration with database layer

3. **[BPT Account Values Reference](bpt-account-values-reference.md)** - Reference for each account type
   - Exact YAML definitions
   - Go struct implementations
   - BPT hash components
   - Code examples for each type

### Analysis & Reviews

4. **[BPT Architectural Review](bpt-architectural-review.md)** - Critical design issues identified
   - Directory storage in state vs chains
   - Performance implications
   - Scalability limits
   - Recommended fixes

5. **[BPT Flaws Verification](bpt-flaws-verification.md)** - Verification of architectural flaws
   - Concrete evidence of issues
   - Performance measurements
   - Comparison of state vs chain storage

6. **[BPT Documentation Review](bpt-documentation-review.md)** - Analysis of documentation issues
   - Problems with current structure
   - Missing concepts
   - Recommendations for improvement

### Migration Strategy

7. **[BPT Automatic Migration Design](bpt-automatic-migration-design.md)** - Strategy for fixing architectural flaws
   - Automatic migration on modification
   - Touch transactions for proactive migration
   - Zero user intervention required
   - Complete migration in 8-10 weeks

## Key Findings

### Critical Issue: State vs Chain Storage

The BPT implementation has a fundamental architectural flaw where large collections (directories, pending transactions, authorities) are stored in state rather than chains. This causes:

- **Hard limits**: 100-1000 accounts per ADI
- **Performance degradation**: O(n) operations for n sub-accounts
- **Memory pressure**: Entire collections loaded at once
- **DOS vulnerabilities**: Spam attacks can degrade performance

### Solution: Automatic Migration

The proposed solution uses automatic migration to chains:
- Deploys without network downtime
- Migrates accounts transparently when modified
- Uses "touch" transactions for complete coverage
- Delivers 15,000x performance improvement for large accounts

## Examples

The [examples](../../examples/) directory contains working code demonstrating:
- Direct BPT lookups
- Token account queries
- Transaction retrieval

## Reading Order

For understanding the BPT:
1. Start with [BPT Complete Guide](bpt-complete-guide.md)
2. Review [BPT Account Values Reference](bpt-account-values-reference.md) for specifics
3. Deep dive with [BPT Implementation Deep Dive](bpt-implementation-deep-dive.md)

For understanding the issues and fixes:
1. Read [BPT Architectural Review](bpt-architectural-review.md)
2. Verify with [BPT Flaws Verification](bpt-flaws-verification.md)
3. Understand solution in [BPT Automatic Migration Design](bpt-automatic-migration-design.md)

---

*Last Updated: 2025-01-12*
*Status: Complete technical review with migration strategy*