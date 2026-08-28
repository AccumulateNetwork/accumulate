# Branch Analysis by Category

## Summary Statistics

| Category | Count | Action |
|----------|-------|--------|
| Essential (keep) | 10 | KEEP |
| Active development (last 30 days) | 31 | KEEP (but delete merged ones after verification) |
| Recent work (30-90 days) | 4 | REVIEW |
| Stale (90-365 days) | 45 | REVIEW - likely delete most |
| Very old (1+ years) | 272 | DELETE - obsolete |
| Release branches | 35 | KEEP recent (1.0+), DELETE old (0.x) |
| **TOTAL** | **393** | **Target: ~113 remaining** |

## Category Breakdown

### 1. Essential Branches (KEEP - 10 branches)

#### Actually Essential (4):
- `main` - Production
- `dagbft-integration` - Active development
- `aip-53-lxr-mining-base` - Feature baseline
- `develop` - Legacy reference

#### Questionable "Essential" (6 - should DELETE):
- `hotfix-main` (1079 days old)
- `master` (1260 days old) - duplicate of main
- `not-develop` (1637 days old) - test branch
- `old-master` (1660 days old) - obsolete
- `stepapp1-develop-patch-61548` (1540 days old) - personal branch
- `test-mainnet` (1189 days old) - test branch

### 2. Active Development - Last 30 Days (31 branches)

#### Merged to dagbft-integration (30 branches - CAN DELETE):
All of these show [MERGED] in the report and are 0-9 days old:
- feature/issue-cleanup-64-issues
- issue-3742-bullshark-ordering-fix
- issue/dagbft-3815
- issue-3871-key-rotation
- issue-3875-per-peer-vote-rate-limiting
- feature/issue-3874
- issue-3872-timestamp-replay-protection
- feature/issue-3863
- feature/issue-3857
- feature/issue-3858
- feature/issue-3854
- feature/issue-3855
- feature/issue-3850
- feature/issue-3853
- feature/issue-3851
- feature/issue-3852
- feature/issue-3849
- feature/issue-3844
- feature/issue-3845
- feature/issue-3843
- feature/issue-3847
- feature/issue-3846
- issue/dagbft-3821
- issue/dagbft-3819
- issue/dagbft-3818
- issue/dagbft-3820
- issue/dagbft-3822
- fix/lint-cleanup
- 3718-cometbft-analysis
- issue-3742-fix-leader-chain-traversal

#### Unmerged Active Work (1 branch - KEEP):
- `3714-sdk-signature-docs` (25 days old) - Active documentation work

### 3. Recent Work 30-90 Days (4 branches)

#### Current release work (REVIEW):
- `3702-release-1.4.4-beta.3` (14 days old) - Current release
- `3701-release-1.4.4-update` (93 days old) - Release update
- `release-1.4.4-update` (95 days old) - Duplicate?

### 4. Stale Unmerged 90-365 Days (45 branches - REVIEW/DELETE)

Key branches that might have value:
- `3691-mcp-server-for-accumulate` (113 days) - MCP server work
- `3695-eliminate-need-for-observer` (95 days) - Observer work
- `3700-halt-at-major-block` (95 days) - Admin API
- `3704-api-stale-block-heights` (92 days) - API enhancement
- `3713-add-version-commands` (73 days) - Version commands

Mining-related branches (likely superseded):
- Multiple 3666/3669/3675/3676/3680 mining branches (158 days)

Cross-chain conductor branches (likely abandoned):
- 3652, 3653, 3656, 3659, 3660, 3661, 3662 series (200-250 days)

Healing/sync branches:
- healing_update, healing-hack, healing-anchor-synth-repair (200-362 days)

### 5. Release Branches (35 total)

#### Keep (9 branches):
- `release-1.4` (411 days) - Current major release
- `release-1.3` (751 days) - Previous major
- `release-1.2` (778 days) - Reference
- `release-1.1` (984 days) - Reference
- `release-1.0` (1076 days) - Reference
- `3702-release-1.4.4-beta.3` (14 days) - Current
- `3701-release-1.4.4-update` (93 days) - Current
- `release-1.4.4-update` (95 days) - Current

#### Delete (26 branches):
All cli-* and release-v0.* branches:
- cli-v1.0.0-rc1, cli-v1.0.0-rc1.1, cli-v1.0.0-rc1.2, cli-v1.0.0-rc1.2-debug, cli-v1.0.0-rc2
- cli-0.7.0-beta, cli-0.8.0-beta, cli-0.8.1-beta, cli-0.8.2-beta, cli-0.9.0-beta, cli-0.9.1-beta
- cli-v0.4, qa-0.7.0-beta
- release-v0.2, release-v0.3, release-v0.4, release-v0.5.1
- release-v0.6.0-rc0, release-v0.6.0-rc1, release-v0.8.0-beta
- dev-1.0.2, hotfix-1.0.4, regenesis-1.0, merge-release-v0.2
- tendermint-0.35.0-rc1

### 6. Very Old Branches 1+ Years (272 branches - DELETE)

#### By Age Range:
- **3+ years old** (2021-2022): ~200 branches
- **2-3 years old** (2022-2023): ~50 branches
- **1-2 years old** (2023-2024): ~22 branches

#### By Pattern:

**AC-* issue branches** (~150 branches):
- AC-125 through AC-3283
- Most related to closed or completed issues
- Examples: AC-1031, AC-1080, AC-1146, AC-2072, AC-3089, etc.

**DO-* devops branches** (~10 branches):
- DO-23, DO-30, DO-57, DO-59, DO-73, DO-76, DO-92

**Numbered issue branches** (~40 branches):
- 3148, 3151, 3199, 3266, 3267, 3322, 3379, 3384, etc.
- 3495, 3539, 3565, 3570, 3588, 3589, 3600, etc.
- 3616, 3617, 3621, 3623, 3626, 3640, 3644

**Feature/experimental branches** (~70 branches):
- bpt2, bpt3, light, ethan-work, ethan-tx-payload
- database, database-benchmark-index, database-interface, database-no-mutex
- factom-import, factom-import-nonce, factom-import-ps
- Various experimental: proxy-data-entries, eip712-mask, hashed-time-locks
- Test branches: test-2, test-target, test-AC-489, foo

## Deletion Priority Ranking

### Priority 1 - SAFE IMMEDIATE DELETION (~160 branches)
1. Test/debug branches: test-*, debug-*, foo, ethan, not-develop, old-master
2. Old AC-* issues from 2021-2022 (AC-125 through AC-2000 series)
3. Old cli-* and release-v0.* branches
4. Reverted branches: revert-*, cherry-pick-*
5. Experimental POCs: poc-*, wip-*, saving-work branches

### Priority 2 - REVIEW THEN DELETE (~100 branches)
1. 2023 AC-* issues (AC-3000 series)
2. 2024 numbered issues (3400-3640 range)
3. Abandoned features: sphereon/kotlin, java-sdk, terraform-*
4. Old refactoring: refactor-*, work-on-*

### Priority 3 - NEEDS CAREFUL REVIEW (~45 branches)
1. Recent stale branches (90-365 days)
2. Mining-related work that might be resumed
3. Cross-chain conductor branches
4. Recent feature work that might have value

### Priority 4 - KEEP (~88 branches)
1. Essential branches (4 actual)
2. Release branches 1.0+ (9)
3. Active work (1 unmerged)
4. Recent merged branches until verified (30)
5. Stale branches under review (44)

## Recommended Execution Order

1. **Phase 1** - Delete merged branches (30) - verify in dagbft-integration first
2. **Phase 2** - Delete obvious garbage (test-*, debug-*, foo, etc.) - ~30 branches
3. **Phase 3** - Delete old releases (cli-*, release-v0.*) - ~26 branches
4. **Phase 4** - Delete very old AC-* issues - ~150 branches
5. **Phase 5** - Manual review of stale branches - delete ~40 more

**Total reduction**: 393 → ~113 branches (71% cleanup)

