---
name: start
description: "Set to 'here' to start from current branch instead of main (default: switch to main and pull) (project)"
arguments:
  - name: issue_number
    description: The GitLab issue number to work on
    required: true
  - name: from_current
    description: "Set to 'here' to start from current branch instead of main (default: switch to main and pull)"
    required: false
---

## Issue #{{issue_number}}

### Phase 0: Git Setup

{{#if from_current}}
**Mode: Starting from current branch**

```bash
# Stay on current branch, just fetch latest
git fetch origin
git status
```

Note: Starting from current branch. Ensure this is intentional (e.g., continuing work or building on existing changes).
{{else}}
**Mode: Fresh start from main (default)**

```bash
# Ensure working directory is clean
git status

# Switch to main and pull latest
git checkout main
git pull origin main

# Verify we're up to date
git log -1 --oneline
```

If there are uncommitted changes, either:
1. Stash them: `git stash`
2. Commit them: `git add . && git commit -m "WIP"`
3. Or ask me how to proceed
{{/if}}

### Phase 1: Planning & Compliance

1. Read the full issue description and all GitLab notes using the gitlab glab cli tool:
   ```bash
   glab issue view {{issue_number}} --repo accumulatenetwork/accumulate --comments
   ```

2. Check for existing branch for this issue:
   ```bash
   git branch -a | grep "{{issue_number}}"
   ```
   - If branch exists: Ask whether to continue on existing branch or start fresh

3. Develop a detailed work plan using TodoWrite

4. **Compliance Check**: Review relevant documentation
   - Check `docs/` for protocol specifications
   - Review existing implementation patterns in similar areas

5. If unclear about anything, stop and ask

### Phase 2: Development

1. Create branch from current position:
   ```bash
   git checkout -b {{issue_number}}-<brief-issue-title>
   ```
   - Format: `{{issue_number}}-<brief-description>`
   - Example: `{{issue_number}}-fix-validator-bug`

2. Implement the feature/fix

3. Build and verify:
   ```bash
   go build ./...
   go vet ./...
   ```

4. For protocol changes, ensure compatibility with existing tests

### Phase 3: Testing & Issues

1. Run all tests, fix any failures:
   ```bash
   go test ./...
   ```
   Or use `/test` command for more options

2. If blocked by major issues:
   - First, scan existing GitLab open issues to avoid duplicates:
     ```bash
     glab issue list --search "<error keyword>" --repo accumulatenetwork/accumulate
     ```
   - If duplicate found: Add note to existing issue with findings or to clarify/add relevant scope
   - If new blocker: Ask if you'd like me to fix immediately (small fixes) or create a GitLab issue (major blockers)

3. Commit and push changes:
   ```bash
   git add -A
   git commit -m "Description (#{{issue_number}})

   Generated with [Claude Code](https://claude.ai/code)

   Co-Authored-By: Claude <noreply@anthropic.com>"
   git push -u origin <branch-name>
   ```

### Phase 4: Code Review & MR

1. Create a MR using the glab tool:
   ```bash
   glab mr create --title "Description [#{{issue_number}}]" \
     --description "## Summary
   ...

   Closes #{{issue_number}}" \
     --repo accumulatenetwork/accumulate
   ```

2. Perform code review using `/review branch`, post as MR comment

3. Address recommended actions using `/apply-review`

4. Run tests again (`/test`), fix issues, commit and push. Iterate until all issues are resolved.

5. Perform final code review, post grade

---

**Starting work on issue #{{issue_number}} now.**

Begin with Phase 0: Git Setup, then proceed through each phase sequentially.
