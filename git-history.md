# Git History Issues

This document describes structural issues in the git history discovered on 2025-12-21.

## Branch Structure

The repository has two main lineages that have diverged:

```
main ─────────────────────────────────────────────────────────────────►
         \
          └── release-1.4 ──► v1.4.0 ──► v1.4.1 ──► ... ──► v1.4.3
                    │
                    └── (current: release-1.4.4-update, 107 commits ahead)
```

### Key Observations

1. **`release-1.4` branch** contains the v1.4.x release tags (v1.4.0 through v1.4.3-fix-the-fix)

2. **`main` branch** has diverged from `release-1.4` and does not contain the v1.4.x tags as ancestors

3. **`release-1.4.4-update` branch** is based on `release-1.4` (good), with 107 additional commits

4. **No v1.4.x tags are ancestors of `main`** - this means `git describe` fails on main-based branches

## Deleted Tag: v1.4.4-beta.1

The tag `v1.4.4-beta.1` was deleted on 2025-12-21 for the following reasons:

1. **Divergent history**: The tag pointed to commit `c981bd1b9` which was on a separate branch that diverged from `release-1.4`, not a direct descendant

2. **Accidentally committed binaries**: The tagged commit included compiled binaries:
   - `tools/deploy-follower/deploy-follower` (6.2 MB)
   - `tools/follower-monitor/follower-monitor` (12.9 MB)

   These binaries bloated the repository by ~19 MB

3. **Content preserved**: All source code from that commit exists in the current `release-1.4.4-update` branch with improvements:
   - Bug fixes (added `--partition` flag to restore-genesis)
   - Security improvements (localhost-only binding by default)
   - Additional features merged (#3695, #3697)

## Impact on Versioning

Because no v1.4.x tags are ancestors of branches based on `main`, the Makefile's version detection fails:

```makefile
GIT_DESCRIBE = $(shell git fetch --tags -q ; git describe --dirty)
```

This results in binaries built from `main` showing "version unknown".

### Workarounds

1. **Create a new tag** on the current branch before building
2. **Modify Makefile** to use `git describe --always --dirty` as fallback
3. **Ensure release branches** are based on `release-1.4` rather than `main`

## Recommendations

1. **For v1.4.4 release**: Tag the `release-1.4.4-update` branch (which is correctly based on `release-1.4`)

2. **For future releases**: Ensure feature branches targeting 1.4.x are based on `release-1.4`, not `main`

3. **Consider merging**: If `main` and `release-1.4` should converge, a merge strategy should be planned

## Commands Used for Analysis

```bash
# Check if a tag is ancestor of current branch
git merge-base --is-ancestor v1.4.0 HEAD

# Find commits unique to a branch
git log --oneline HEAD ^origin/release-1.4

# Find common ancestor
git merge-base origin/release-1.4 HEAD

# Check which branches contain a commit
git branch -a --contains <commit>
```
