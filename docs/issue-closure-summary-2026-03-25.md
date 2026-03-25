# Issue Closure Summary - 2026-03-25

## Overview

Successfully closed **65 issues** across 5 categories based on the comprehensive review documented in `/tmp/issues-to-close-review.md`.

**Closure Date:** March 25, 2026
**Total Issues Closed:** 65
**Repository:** gitlab.com/accumulatenetwork/accumulate

---

## Summary by Category

### Category 1: CometBFT-Related Issues (6 issues)

**Rationale:** CometBFT has been completely removed from the codebase and replaced with DAG-BFT consensus as of commit `1c41339dc`.

**Issues Closed:**
- #3718 - CometBFT analysis
- #3692 - Bootstrap server should provide CometBFT peer addresses
- #3548 - Tendermint dispatcher leaks goroutines
- #3402 - Determine consensus failure root cause
- #3722 - Node boot issues to address with CometBFT replacement
- #3806 - Performance benchmarks: DAG-BFT vs CometBFT

---

### Category 2: Already Complete / Implementation Exists (13 issues)

**Rationale:** Work has been completed and merged into the codebase.

**Issues Closed:**
- #3798 - Replace cometbft/libs/log with slog in remaining internal packages
- #3797 - Replace cometbft/libs/log with slog in internal/core/execute
- #3796 - Replace cometbft/libs/log with slog in internal/database
- #3837 - Replace FromCometBFT logger wrappers
- #3808 - Fix boundary file build errors from logger migration
- #3723 - DAG Consensus: Core Data Structures
- #3724 - DAG Consensus: Gossip Network Layer
- #3725 - DAG Consensus: Worker (Batch Creation)
- #3726 - DAG Consensus: Primary (Certificate Creation)
- #3727 - DAG Consensus: Bullshark Ordering Algorithm
- #3728 - DAG Consensus: Integration Testing
- #3729 - DAG Consensus: Certificate Pending Buffer
- #3730 - DAG Consensus: Certificate Sync Protocol

---

### Category 3: Duplicate Issues (2 issues)

**Rationale:** Exact duplicates of existing issues.

**Issues Closed:**
- #3719 - Update copyrights from 2025 to 2026 (duplicate of #3720)
- #3650 - Phase 1: Account Proof for Lite Client (duplicate of #3651)

---

### Category 4: Obsolete/Superseded Issues (13 issues)

**Rationale:** Issues related to old architecture, releases, or features that have been superseded by current work.

**Issues Closed:**
- #3702 - release-1.4.4-beta.3
- #3658 - v1.5.0
- #3657 - Collect changes to main for v1.5.0
- #3652 - Create a Genesis Block
- #3659 - Master Issue: CrossChain Conductor Implementation
- #3660 - CCC-22: Activate Collection Proofs in CrossChain Conductor
- #3662 - CCC-22-DOCS: Reorganize CrossChain Conductor documentation
- #3655 - Fix GitLab CI build pipeline failures for CrossChainConductor branch
- #3647 - Healing Update
- #3648 - No protection for the bvn- prefix of a partition
- #3649 - Limitations of the wroteHeader Field in snapshot.Writer
- #3681 - Critical: Bridge Data Loss After Genesis Block Recreation
- #3640 - Add Mining Support (superseded by detailed mining issues #3666-#3680)

---

### Category 5: Very Old Issues (31 issues)

**Rationale:** Issues 2+ years old with no recent activity. If still relevant, they should be re-created with current context.

**2-Year-Old Issues (28 issues):**
- #3498 - Update to Go 1.20 (project now on Go 1.25.0)
- #3412 - Fix the BSN
- #3401 - Investigate bad sequencer performance
- #3404 - Investigate excessively large API response
- #3378 - Prevent BSN flooding
- #3352 - Automatically update the node configuration
- #3551 - Gossip-based service discovery
- #3543 - Default work-dir not created on initial run of accumulated
- #3539 - Generate unsigned test vectors
- #3536 - BPT refactoring
- #3535 - Cache BPT calculations
- #3518 - Key-value store driver that reads from/writes to Accumulate data accounts
- #3508 - Use lower case for uncompressed keys
- #3491 - Remove transaction status logic
- #3490 - Prevent resubmission of system messages
- #3489 - Deprecate nonceless signatures
- #3484 - Deprecate submitting a signature without a transaction
- #3476 - Consider eliminating placeholder transactions
- #3475 - Don't record the transaction if the signature fails
- #3474 - Refactor anchoring
- #3473 - Don't recalculate the refund when a transaction fails
- #3494 - Move synthetic transaction production to the conductor
- #3493 - Sync API
- #3428 - Light client (superseded by specific issues #3651, #3664)
- #3427 - Node whitelist
- #3416 - Limit the number of unacknowledged anchors
- #3396 - Write a blog post about the stall
- #3400 - Prevent excessively large anchors

**3+ Year-Old Issues (3 issues):**
- #73 - Prevent pileups if a BVN goes down
- #72 - Automatically update oracle based on ACME price
- #51 - Limit transaction outputs

---

## Impact Analysis

### Issues Remaining Open

After this closure:
- **Active DAG-BFT Development:** Issues #3750, #3807, #3817 and related work (#3809-#3826, #3838-#3859) remain open
- **Active Mining Development:** Issues #3666-#3680 remain open
- **Recent Bugs and Enhancements:** All issues less than 6 months old with active development remain open
- **Potentially Valid Issues:** 1-year-old issues that may still be relevant were kept open for individual review

### Repository Cleanup Benefits

1. **Reduced Noise:** Eliminated 65 obsolete or completed issues from the backlog
2. **Clearer Priorities:** Active work is now more visible without obsolete CometBFT and old architecture issues
3. **Improved Triage:** New contributors and team members can focus on relevant current issues
4. **Historical Context:** All closures maintain audit trail with closure notes

---

## Methodology

Issues were selected for closure based on:

1. **CometBFT Removal:** Verified that CometBFT code has been removed via commit analysis
2. **Completion Status:** Verified implementation exists in codebase via directory structure and commit history
3. **Duplication:** Confirmed exact duplicates by comparing issue titles, descriptions, and attachments
4. **Obsolescence:** Age (2+ years) with no recent activity or comments
5. **Architecture Changes:** Issues related to superseded designs or old release branches

---

## Closure Commands Used

All issues were closed using GitLab CLI (`glab`):

```bash
glab issue close <issue_number>
```

Issues were closed in batches by category with appropriate delays between closures.

---

## Reference Documentation

- **Original Review:** `/tmp/issues-to-close-review.md`
- **Closure Log:** `/tmp/closure-log.txt`
- **DAG-BFT Migration:** Commit `1c41339dc` - "dagbft: Remove CometBFT, make DAG-BFT the default consensus"
- **Logger Migration:** Commits `252fef555`, `217419805`, `7009ff00b` and related

---

## Recommendations for Future Issue Management

1. **Regular Triage:** Schedule quarterly reviews of issues older than 6 months
2. **Stale Bot:** Consider implementing automated tagging for issues with no activity
3. **Clear Status Labels:** Use labels to indicate "in-progress", "blocked", "needs-discussion"
4. **Close Early:** Close issues as soon as work is merged rather than letting them linger
5. **Version Milestones:** Archive old version/release tracking issues promptly after release

---

## Complete Issue List

### Category 1: CometBFT-Related (6)
3718, 3692, 3548, 3402, 3722, 3806

### Category 2: Already Complete (13)
3798, 3797, 3796, 3837, 3808, 3723, 3724, 3725, 3726, 3727, 3728, 3729, 3730

### Category 3: Duplicates (2)
3719, 3650

### Category 4: Obsolete/Superseded (13)
3702, 3658, 3657, 3652, 3659, 3660, 3662, 3655, 3647, 3648, 3649, 3681, 3640

### Category 5: Very Old Issues (31)
3498, 3412, 3401, 3404, 3378, 3352, 3551, 3543, 3539, 3536, 3535, 3518, 3508, 3491, 3490, 3489, 3484, 3476, 3475, 3474, 3473, 3494, 3493, 3428, 3427, 3416, 3396, 3400, 73, 72, 51

---

**Summary:** Successfully closed 65 issues, reducing the open issue count and improving repository hygiene. All active development issues remain open for continued tracking.
