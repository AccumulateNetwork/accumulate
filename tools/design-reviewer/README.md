# Design-Driven Code Reviewer Agent

## Overview
The Design-Driven Code Reviewer is an automated agent that maintains design documents for issues and ensures code implementations match approved designs.

## Installation

```bash
# Make the script executable
chmod +x tools/design-reviewer/design_reviewer.py

# Create an alias for easy access (optional)
alias design-review='python3 tools/design-reviewer/design_reviewer.py'
```

## Quick Start: Working with a New Issue

### Step 1: Create Design Document
When you receive a new issue, immediately create a design document:

```bash
# Basic usage
python3 tools/design-reviewer/design_reviewer.py create --issue 123

# With issue details
python3 tools/design-reviewer/design_reviewer.py create \
  --issue 123 \
  --title "Implement caching layer" \
  --body "Add Redis caching to improve API response times"
```

This creates: `docs/designs/issue-123-design.md`

### Step 2: Complete the Design
Edit the generated design document to add:
1. Detailed problem statement
2. Architecture overview
3. Component specifications
4. API contracts (function signatures)
5. Data structures
6. Error handling approach
7. Testing requirements
8. Acceptance criteria

### Step 3: Implement Code
Create a feature branch and implement according to the design:

```bash
git checkout -b feature/issue-123-caching
# ... implement code according to design ...
```

### Step 4: Review Implementation
Before committing, review your code against the design:

```bash
# Review current branch against design
python3 tools/design-reviewer/design_reviewer.py review \
  --design docs/designs/issue-123-design.md \
  --branch feature/issue-123-caching

# Or review a specific PR
python3 tools/design-reviewer/design_reviewer.py review \
  --design docs/designs/issue-123-design.md \
  --pr 456
```

### Step 5: Address Compliance Issues
The review generates a report showing:
- Compliance score (percentage)
- Missing implementations
- Critical issues
- Recommendations

Fix any issues and re-run the review.

### Step 6: Update Design (if needed)
If implementation required design changes:

```bash
python3 tools/design-reviewer/design_reviewer.py update \
  --design docs/designs/issue-123-design.md \
  --changes "Modified API to use async pattern for better performance"
```

## Workflow Example

### Complete Issue Workflow

```bash
# 1. New issue arrives: "Add rate limiting to API endpoints"
ISSUE_ID=789
ISSUE_TITLE="Add rate limiting to API endpoints"

# 2. Create design document
python3 tools/design-reviewer/design_reviewer.py create \
  --issue $ISSUE_ID \
  --title "$ISSUE_TITLE"

# 3. Edit the design document
vim docs/designs/issue-789-design.md

# 4. Create feature branch
git checkout -b feature/issue-789-rate-limiting

# 5. Implement the feature
# ... write code according to design ...

# 6. Review implementation
python3 tools/design-reviewer/design_reviewer.py review \
  --design docs/designs/issue-789-design.md \
  --branch feature/issue-789-rate-limiting

# 7. Check the report
cat docs/designs/issue-789-review.md

# 8. If compliance < 90%, fix issues and re-review
# ... fix issues ...
python3 tools/design-reviewer/design_reviewer.py review \
  --design docs/designs/issue-789-design.md \
  --branch feature/issue-789-rate-limiting

# 9. Commit when compliance is satisfactory
git add .
git commit -m "feat: implement rate limiting per design doc #789"

# 10. Push and create PR
git push origin feature/issue-789-rate-limiting
```

## Design Document Structure

The generated design document includes:

```markdown
# Issue Design Document: #[ID]

## Issue Summary
- Issue metadata and status

## Problem Statement
- Clear problem description

## Design Specification
### Architecture Overview
- High-level design approach

### Component Definitions
- Affected and new files

### API Contracts
- Function signatures
- Data structures

### Data Flow
- Step-by-step process

### Error Handling
- Error conditions and responses

### Testing Requirements
- Required test coverage

## Implementation Checklist
- Trackable implementation items

## Acceptance Criteria
- Clear success metrics
```

## Review Report Format

The review report shows:

```markdown
# Code Review Report

## Design Compliance Summary
- Overall Compliance: 85.5%
- Checks Passed: 17/20
- Critical Issues: 1
- Warnings: 2

## Detailed Analysis
### ✅ Compliant Areas
- Functions implemented correctly
- Data structures match design

### ❌ Missing Implementation
- Missing error handlers
- Incomplete test coverage

### Recommendations
- Specific fixes needed
```

## Configuration

Create `tools/design-reviewer/config.json`:

```json
{
  "design_dir": "docs/designs",
  "template_dir": "tools/design-reviewer/templates",
  "strict_mode": true,
  "require_tests": true,
  "min_compliance_score": 90
}
```

## Best Practices

### 1. Design First
- Always create design before coding
- Get design approval from team
- Use design as implementation guide

### 2. Keep Designs Updated
- Update design when requirements change
- Document why changes were made
- Maintain change log

### 3. Review Frequently
- Review after each major component
- Don't wait until end to review
- Use compliance score as quality metric

### 4. Test Coverage
- Include test requirements in design
- Verify tests match design specs
- Ensure edge cases covered

## Integration with Development Process

### Git Hooks
Add pre-commit hook to check compliance:

```bash
#!/bin/bash
# .git/hooks/pre-commit

ISSUE_ID=$(git branch --show-current | grep -oP '\d+')
if [ -n "$ISSUE_ID" ]; then
  DESIGN_DOC="docs/designs/issue-${ISSUE_ID}-design.md"
  if [ -f "$DESIGN_DOC" ]; then
    python3 tools/design-reviewer/design_reviewer.py review \
      --design "$DESIGN_DOC" \
      --branch $(git branch --show-current)
  fi
fi
```

### CI/CD Pipeline
Add to CI pipeline:

```yaml
design-review:
  stage: test
  script:
    - python3 tools/design-reviewer/design_reviewer.py review \
        --design docs/designs/issue-$CI_MERGE_REQUEST_IID-design.md \
        --pr $CI_MERGE_REQUEST_IID
  artifacts:
    reports:
      - docs/designs/*-review.md
```

### VS Code Task
Add to `.vscode/tasks.json`:

```json
{
  "label": "Review Design Compliance",
  "type": "shell",
  "command": "python3",
  "args": [
    "tools/design-reviewer/design_reviewer.py",
    "review",
    "--design",
    "${input:designDoc}",
    "--branch",
    "${input:branch}"
  ]
}
```

## Troubleshooting

### Common Issues

1. **Low Compliance Score**
   - Ensure function names match exactly
   - Check data structure definitions
   - Verify test files included

2. **Missing Functions**
   - Review git diff scope
   - Ensure changes committed
   - Check function naming

3. **Design Parse Errors**
   - Use correct markdown format
   - Follow template structure
   - Validate code blocks

## Advanced Usage

### Custom Templates
Create custom design templates in `tools/design-reviewer/templates/`

### Automated Design Generation
Use with issue tracking APIs:

```python
import requests
from design_reviewer import DesignReviewer

# Fetch issue from GitHub/GitLab
issue = requests.get(f"https://api.github.com/repos/owner/repo/issues/{id}")
reviewer = DesignReviewer()
reviewer.create_design(
    issue_id=issue['number'],
    title=issue['title'],
    body=issue['body']
)
```

### Batch Review
Review multiple issues:

```bash
for design in docs/designs/*-design.md; do
  python3 tools/design-reviewer/design_reviewer.py review \
    --design "$design" \
    --branch main
done
```

## Support

For issues or improvements:
1. Check existing designs in `docs/designs/`
2. Review generated reports
3. Adjust compliance rules in config
4. Extend the agent for your needs