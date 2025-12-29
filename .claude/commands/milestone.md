---
name: milestone
description: "Issue number to resume from, or 'continue' to auto-detect last completed (project)"
arguments:
  - name: milestone_id
    description: The GitLab milestone ID or title to work on
    required: true
  - name: resume
    description: "Issue number to resume from, or 'continue' to auto-detect last completed"
    required: false
---

## Milestone Power Mode: {{milestone_id}}

{{#if resume}}
**Resume Mode**: Starting from issue #{{resume}}
{{else}}
**Fresh Start**: Beginning from first issue
{{/if}}

---

### Phase 0: Milestone Discovery

```bash
# Fetch milestone details
glab api "projects/accumulatenetwork%2Faccumulate/milestones?search={{milestone_id}}" --repo accumulatenetwork/accumulate

# Get all issues in the milestone
glab issue list --milestone "{{milestone_id}}" --repo accumulatenetwork/accumulate
```

Extract:
- Milestone title and description
- All associated issues
- Any ordering hints in the description

---

### Phase 1: Triage & Planning

**Analyze each issue to determine execution order:**

For each issue in the milestone:

1. **Read the issue**
   ```bash
   glab issue view <issue_number> --repo accumulatenetwork/accumulate --comments
   ```

2. **Classify the issue**
   | Factor | Weight | Notes |
   |--------|--------|-------|
   | Dependencies | High | Issues that others depend on go first |
   | Complexity | Medium | Simpler issues first (build momentum) |
   | Labels | Medium | `blocker`, `critical` = higher priority |
   | Issue number | Low | Lower numbers often represent foundational work |

3. **Check for blockers**
   - External dependencies?
   - Missing spec clarification?
   - Requires other issues first?

4. **Build execution order**

**Create a triage report:**

```markdown
## Milestone Triage: {{milestone_id}}

### Execution Order
| Order | Issue | Title | Complexity | Dependencies | Status |
|-------|-------|-------|------------|--------------|--------|
| 1 | #101 | Foundation work | Low | None | Ready |
| 2 | #102 | Build on #101 | Medium | #101 | Ready |
| 3 | #103 | Complex feature | High | #101, #102 | Ready |

### Blocked Issues (will skip)
| Issue | Title | Blocker | Action |
|-------|-------|---------|--------|
| #105 | Needs spec | Missing spec | Create spec issue |

### Estimated Scope
- Total issues: X
- Ready to work: Y
- Blocked: Z
```

**Present triage to user and ask for approval before proceeding.**

---

### Phase 2: Git Setup

```bash
# Ensure clean state
git status

# Start from main with latest
git checkout main
git pull origin main

# Create milestone tracking branch (optional, for reference)
git checkout -b milestone-{{milestone_id}}-base
git push -u origin milestone-{{milestone_id}}-base
git checkout main
```

---

### Phase 3: Stacked Execution Loop

**CRITICAL: STACKED MR WORKFLOW**

**Each branch MUST be created from the PREVIOUS issue's branch, NOT from main!**

This prevents merge conflicts when MRs are merged in sequence.

```
WRONG (causes conflicts):          CORRECT (no conflicts):
main                               main
 ├── issue-1-branch                 └── issue-1-branch
 ├── issue-2-branch (from main!)         └── issue-2-branch (from issue-1!)
 └── issue-3-branch (from main!)              └── issue-3-branch (from issue-2!)
```

**Track state for resume capability:**

```
MILESTONE_STATE:
  current_issue: null
  completed_issues: []
  created_mrs: []
  base_branch: main
  previous_branch: main  # ← UPDATE THIS AFTER EACH ISSUE!
```

**For each issue in execution order:**

#### Step 1: Setup Branch (Stacked) - MUST FOLLOW EXACTLY

```bash
# CRITICAL: Branch from PREVIOUS issue's branch, NOT main!
# For first issue: PREVIOUS_BRANCH="main"
# For subsequent issues: PREVIOUS_BRANCH="<previous_issue_number>-<description>"

PREVIOUS_BRANCH="<previous_issue_branch>"  # e.g., "855-delegation-support"
git checkout $PREVIOUS_BRANCH
git pull origin $PREVIOUS_BRANCH 2>/dev/null || true

# Create new branch FROM the previous branch (this is what makes it stacked!)
git checkout -b <issue_number>-<description>

# Verify: The new branch now contains ALL changes from previous issues
git log --oneline -5  # Should show commits from previous issues
```

#### Step 2: Execute /start Workflow

Perform the full `/start` workflow for this issue:

1. **Planning & Compliance** (Phase 1 of /start)
   - Read issue details
   - Develop work plan with TodoWrite
   - Check spec compliance

2. **Development** (Phase 2 of /start)
   - Implement the feature/fix
   - Build and verify: `go build ./...`

3. **Testing** (Phase 3 of /start)
   - Run tests: `/test branch` for targeted tests
   - Fix any failures: `/test fix` if needed
   - Commit changes

4. **Code Review** (Phase 4 of /start)
   - Self-review with `/review branch`
   - Address critical issues with `/apply-review`

#### Step 3: Create Stacked MR

```bash
# Push the branch
git push -u origin <branch_name>

# Create MR targeting PREVIOUS branch (stacked MR)
glab mr create \
  --title "<Description> [#<issue_number>]" \
  --description "$(cat <<'EOF'
## Summary
<implementation summary>

## Stack Position
- **Base**: <previous_branch>
- **This MR**: <current_branch>
- **Milestone**: {{milestone_id}}

## Dependencies
- Depends on: !<previous_mr_number> (if not main)

Closes #<issue_number>

Generated with [Claude Code](https://claude.ai/code)
EOF
)" \
  --target-branch "$PREVIOUS_BRANCH" \
  --repo accumulatenetwork/accumulate
```

#### Step 4: Update State (CRITICAL!)

```
completed_issues.append(issue_number)
created_mrs.append(mr_number)

# CRITICAL: Update previous_branch for the NEXT issue!
previous_branch = current_branch  # e.g., "856-queue-system"
# The NEXT issue will branch from THIS branch, not from main!
```

**Before starting the next issue, verify:**
- [ ] `previous_branch` is set to the branch you just completed
- [ ] Next `git checkout -b` will be from this branch, not main

#### Step 5: Decision Point

After each issue, evaluate:

| Condition | Action |
|-----------|--------|
| Tests pass, review OK | Continue to next issue |
| Tests fail, fixable | Run `/test fix`, then continue |
| Blocked by bug | Create GitLab issue, STOP or skip |
| Spec unclear | Ask user, STOP |
| All issues done | Proceed to Phase 4 |

---

### Phase 4: Stack Summary

After all issues processed:

```markdown
## Milestone {{milestone_id}} Complete

### MR Stack (merge in order)
| Order | MR | Issue | Target Branch | Status |
|-------|-----|-------|---------------|--------|
| 1 | !201 | #101 | main | Ready |
| 2 | !202 | #102 | 101-foundation | Ready |
| 3 | !203 | #103 | 102-feature | Ready |

### Merge Instructions
Merge MRs in order (1 → 2 → 3). Each MR will need rebase after previous merges:

```bash
# After merging !201 to main
git checkout 102-feature
git rebase main
git push --force-with-lease

# Continue for each MR in stack
```

### Skipped Issues
| Issue | Reason | Follow-up |
|-------|--------|-----------|
| #105 | Missing spec | Issue #110 created |

### Statistics
- Issues completed: X/Y
- MRs created: X
- Tests passing: PASS/FAIL
```

---

### Termination Conditions

**STOP milestone execution when:**

1. A blocking bug is discovered that affects multiple issues
2. Spec clarification needed that affects remaining issues
3. 3 consecutive issues fail (likely systemic problem)
4. User requests stop
5. All issues completed successfully

**On STOP, save state for resume:**

```bash
# Save state to temp file for resume
echo "RESUME_FROM=<next_issue>" > /tmp/milestone_{{milestone_id}}_state
echo "PREVIOUS_BRANCH=<branch>" >> /tmp/milestone_{{milestone_id}}_state
echo "COMPLETED=<issue1,issue2,...>" >> /tmp/milestone_{{milestone_id}}_state
```

---

### Resume Capability

If `resume` argument is provided:

1. Skip all issues before the specified issue number
2. Set `previous_branch` to the branch of the last completed issue
3. Continue execution from that issue

Special value `continue` auto-detects the last completed issue from saved state.

---

### Quick Reference

| Command | Description |
|---------|-------------|
| `/milestone 15` | Start milestone 15 from beginning |
| `/milestone 15 103` | Resume from issue #103 |
| `/milestone 15 continue` | Auto-resume from last completed |
| `/milestone "Q1 Release"` | Use milestone title |

---

**Starting Milestone Power Mode now...**

Begin with Phase 0: Milestone Discovery. Use TodoWrite extensively to track progress across all issues.
