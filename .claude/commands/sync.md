---
name: sync
description: Sync with Remote Main (project)
arguments: []
---

# Sync with Remote Main

Fetch and merge the latest changes from the remote main branch into the current branch.

## Instructions

1. First, check current branch status:
   ```bash
   git status
   ```

2. If there are uncommitted changes, warn the user and ask if they want to:
   - Stash changes first
   - Commit changes first
   - Abort the sync

3. Fetch and merge remote main:
   ```bash
   git fetch origin main
   git merge origin/main --no-edit
   ```

4. If merge conflicts occur:
   - List the conflicting files
   - Ask the user how they want to resolve them
   - Do NOT automatically resolve conflicts

5. After successful merge, show:
   - Number of commits merged
   - Summary of files changed
   - Current branch status

6. If the merge was successful and there were changes, ask if the user wants to push.

## Example Output

```
Fetching origin/main...
Merging origin/main into current branch...

Merge successful:
- 5 commits merged
- 12 files changed
- Branch is now 3 commits ahead of origin

Would you like to push these changes?
```
