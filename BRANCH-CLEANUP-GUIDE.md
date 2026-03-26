# Branch Cleanup Quick Reference Guide

## Overview

After deleting 27 obsolete branches, **393 branches remain**. This guide provides a systematic approach to reducing this to approximately **113 essential branches** (71% reduction).

## Files Generated

1. **branch-preservation-review.md** - Comprehensive analysis with all branch details
2. **branch-categories.md** - Categorized breakdown with specific recommendations
3. **delete-old-branches.sh** - Executable script to delete 272 very old branches
4. **BRANCH-CLEANUP-GUIDE.md** - This quick reference (you are here)

## Current State

| Category | Count | Recommendation |
|----------|-------|----------------|
| Essential branches | 10 | Keep 4, delete 6 obsolete "essentials" |
| Active (last 30 days) | 31 | Keep 1 active, delete 30 already merged |
| Recent (30-90 days) | 4 | Review release branches |
| Stale (90-365 days) | 45 | Review, likely delete 30-40 |
| Very old (1+ years) | 272 | Delete all |
| Release branches | 35 | Keep 9 recent, delete 26 old |
| **TOTAL** | **393** | **Target: ~113** |

## Quick Actions

### Phase 1: Delete Recently Merged Branches (Safe - 30 branches)

These are all merged to dagbft-integration in the last week:

```bash
git push origin --delete \
  feature/issue-cleanup-64-issues \
  issue-3742-bullshark-ordering-fix \
  issue/dagbft-3815 \
  issue-3871-key-rotation \
  issue-3875-per-peer-vote-rate-limiting \
  feature/issue-3874 \
  issue-3872-timestamp-replay-protection \
  feature/issue-3863 \
  feature/issue-3857 \
  feature/issue-3858 \
  feature/issue-3854 \
  feature/issue-3855 \
  feature/issue-3850 \
  feature/issue-3853 \
  feature/issue-3851 \
  feature/issue-3852 \
  feature/issue-3849 \
  feature/issue-3844 \
  feature/issue-3845 \
  feature/issue-3843 \
  feature/issue-3847 \
  feature/issue-3846 \
  issue/dagbft-3821 \
  issue/dagbft-3819 \
  issue/dagbft-3818 \
  issue/dagbft-3820 \
  issue/dagbft-3822 \
  fix/lint-cleanup \
  3718-cometbft-analysis \
  issue-3742-fix-leader-chain-traversal
```

### Phase 2: Delete Obsolete "Essential" Branches (Safe - 6 branches)

```bash
git push origin --delete \
  hotfix-main \
  master \
  not-develop \
  old-master \
  stepapp1-develop-patch-61548 \
  test-mainnet
```

### Phase 3: Delete Old Release Branches (Safe - 26 branches)

Pre-1.0 releases that are no longer needed:

```bash
git push origin --delete \
  cli-v1.0.0-rc1 cli-v1.0.0-rc1.1 cli-v1.0.0-rc1.2 \
  cli-v1.0.0-rc1.2-debug cli-v1.0.0-rc2 \
  cli-0.7.0-beta cli-0.8.0-beta cli-0.8.1-beta cli-0.8.2-beta \
  cli-0.9.0-beta cli-0.9.1-beta cli-v0.4 qa-0.7.0-beta \
  release-v0.2 release-v0.3 release-v0.4 release-v0.5.1 \
  release-v0.6.0-rc0 release-v0.6.0-rc1 release-v0.8.0-beta \
  dev-1.0.2 hotfix-1.0.4 regenesis-1.0 \
  merge-release-v0.2 tendermint-0.35.0-rc1
```

### Phase 4: Delete Very Old Branches (Review First - 272 branches)

**IMPORTANT**: Review the script before executing!

```bash
# Review the script
less delete-old-branches.sh

# If satisfied, execute
bash delete-old-branches.sh
```

This will delete all branches that are:
- Over 1 year old
- Never merged to dagbft-integration or main
- Include: test branches, old AC-* issues, experimental branches, abandoned features

### Phase 5: Review Stale Branches (Manual - 45 branches)

Review these individually to determine value:

**Potentially valuable:**
- `3691-mcp-server-for-accumulate` - MCP server work
- `3695-eliminate-need-for-observer` - Observer improvements
- `3700-halt-at-major-block` - Admin API
- `3704-api-stale-block-heights` - API enhancement
- `3713-add-version-commands` - Version commands

**Likely obsolete:**
- Mining branches (3666, 3669, 3675, 3676, 3680 series)
- Cross-chain conductor branches (3652, 3653, 3656, 3659, 3660, 3661, 3662)
- Healing branches (healing_update, healing-hack, healing-anchor-synth-repair)

## Branches to Keep

### Protected (4 branches)
- `main` - Production branch
- `dagbft-integration` - Active development
- `aip-53-lxr-mining-base` - Feature baseline
- `develop` - Legacy reference

### Recent Releases (9 branches)
- `release-1.4`, `release-1.3`, `release-1.2`, `release-1.1`, `release-1.0`
- `3702-release-1.4.4-beta.3`
- `3701-release-1.4.4-update`
- `release-1.4.4-update`

### Active Work (1 branch)
- `3714-sdk-signature-docs` - Recent documentation work

## Expected Results

| Phase | Branches Deleted | Remaining |
|-------|-----------------|-----------|
| Starting point | - | 393 |
| Phase 1 (merged) | 30 | 363 |
| Phase 2 (obsolete essentials) | 6 | 357 |
| Phase 3 (old releases) | 26 | 331 |
| Phase 4 (very old) | 272 | 59 |
| Phase 5 (manual review) | ~40 | ~113 |
| **Final Target** | **~280** | **~113** |

## Verification Commands

```bash
# Count current branches
git branch -r | wc -l

# Check merged branches
git branch -r --merged origin/dagbft-integration | wc -l

# List branches by age
git for-each-ref --sort=-committerdate refs/remotes/origin \
  --format='%(committerdate:short) %(refname:short)' | head -50

# Check if a branch is merged
git branch -r --merged origin/dagbft-integration | grep "branch-name"
```

## Safety Tips

1. **Always verify merged status** before deleting recent branches
2. **Review the deletion script** before running Phase 4
3. **Keep a backup** of branch references: `git branch -r > branches-backup.txt`
4. **Test with dry-run** if unsure: replace `--delete` with `--dry-run`
5. **Can recover** within 30 days using GitLab's deleted branch recovery

## Recovery (if needed)

If you accidentally delete a branch:
1. Go to GitLab UI → Repository → Branches → Deleted branches
2. Find the branch and click "Restore"
3. Available for 30 days after deletion

## Questions to Ask

Before deleting any branch, ask:
1. Is it merged to dagbft-integration or main?
2. Is it referenced in any open issues or MRs?
3. Does it contain unique work not captured elsewhere?
4. Is it older than the latest release?

## Next Steps

1. ✅ Review this guide
2. ⬜ Execute Phase 1 (recently merged - safest)
3. ⬜ Execute Phase 2 (obsolete branches)
4. ⬜ Execute Phase 3 (old releases)
5. ⬜ Review and execute Phase 4 (very old branches)
6. ⬜ Manual review Phase 5 (stale branches)
7. ⬜ Verify final count and document decisions

---

**Last Updated**: 2026-03-25
**Generated by**: Claude Code branch analysis
