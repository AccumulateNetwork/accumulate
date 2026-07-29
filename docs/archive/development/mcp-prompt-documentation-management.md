# MCP Prompt: Documentation Management

This prompt template should be added to the tracking repository's MCP server for managing Accumulate project documentation.

## Prompt Specification

**Name:** `organize-project-documentation`
**Description:** Organize and manage project documentation following Accumulate standards
**Category:** Project Management

## Required Arguments

- `action`: Action to perform - `review`, `organize`, `archive`, or `cleanup`
- `scope`: Scope of operation - `all`, `new-files`, or specific directory path

## Optional Arguments

- `create_index`: Create or update documentation index (default: true)
- `dry_run`: Show what would be done without making changes (default: false)

## Workflow Template

```markdown
# Documentation Management: {action} ({scope})

## Step 1: Review Current State

Use file system tools to:
- List all markdown files in repository
- Identify untracked documentation files
- Check for files in root directory or inappropriate locations
- Review existing docs/ structure

## Step 2: Categorize Documentation

Classify each document by type:

**Active Documentation:**
- Guides → `docs/guides/`
- Architecture → `docs/architecture/`
- Design → `docs/design/`
- Network/Deployment → `docs/network/` or `docs/deployment/`
- API → `docs/api/`
- Tutorials → `docs/tutorials/`

**Archive Documentation:**
- Development sessions → `docs/archive/development/YYYY-MM-DD-description.md`
- Test results → `docs/archive/testing/YYYY-MM-DD-description.md`
- Meeting notes → `docs/archive/meetings/YYYY-MM-DD-description.md`
- Investigation reports → `docs/archive/investigations/YYYY-MM-DD-description.md`

**Delete:**
- Content fully integrated into other docs
- Completely obsolete information
- Duplicate files
- Temporary scratch files

## Step 3: Apply Filename Standards

Transform filenames to follow standards:

**Rules:**
1. Use lowercase with hyphens: `my-document.md`
2. NO all-caps names (except standard: README.md, CHANGELOG.md, CONTRIBUTING.md)
3. Archive files include dates: `YYYY-MM-DD-description.md`
4. Descriptive names that indicate content
5. Avoid abbreviations unless widely known
6. Remove version numbers from filenames (track in git instead)

**Examples:**
- ❌ `BADGER_VALIDATION_INVESTIGATION.md`
- ✅ `2025-10-27-badger-validation-investigation.md`

- ❌ `DEPLOYMENT_LOG_JULY13_20251119_114543.md`
- ✅ `2025-11-19-deployment-log-july13.md`

- ❌ `QUICK_FIX.md`
- ✅ `bootstrap-quick-fix.md`

- ❌ `CONFIG_VALIDATION.md`
- ✅ `configuration-validation.md`

## Step 4: Organize Directory Structure

Ensure proper structure exists:

```
docs/
├── index.md                    # Main documentation index
├── guides/                     # User guides
├── architecture/               # Architecture documentation
├── design/                     # Design documents
├── network/                    # Network topology and config
├── deployment/                 # Deployment guides
│   ├── bootstrap/             # Bootstrap-specific deployment
│   └── follower/              # Follower node deployment
├── api/                        # API documentation
├── tutorials/                  # Step-by-step tutorials
└── archive/                    # Historical documentation
    ├── development/           # Development sessions
    ├── testing/               # Test results
    ├── meetings/              # Meeting notes
    └── investigations/        # Investigation reports
```

## Step 5: Move Files

For each file:
1. Determine correct location based on classification
2. Rename according to filename standards
3. Move file to correct directory
4. Update any references in other documentation

**Commands:**
```bash
# Create directory structure
mkdir -p docs/{guides,architecture,design,network,deployment/bootstrap,deployment/follower,api,tutorials}
mkdir -p docs/archive/{development,testing,meetings,investigations}

# Move files (example)
mv BADGER_VALIDATION_INVESTIGATION.md docs/archive/investigations/2025-10-27-badger-validation-investigation.md
mv CONFIG_VALIDATION.md docs/guides/configuration-validation.md
```

## Step 6: Update Index

Create or update `docs/index.md` with:
- Organized links to all documentation
- Brief descriptions of each document
- Clear section headings by topic
- Archive section with chronological organization
- Documentation guidelines section

## Step 7: Verify Organization

Check that:
- [ ] No markdown files in root directory (except README, CHANGELOG, CONTRIBUTING)
- [ ] All documentation in `docs/` or appropriate subdirectories
- [ ] Filenames follow standards
- [ ] Index is up to date
- [ ] No broken links in documentation
- [ ] Archive files have dates in filenames
- [ ] Active documentation is organized by topic

## Step 8: Create/Update Guidelines

Ensure `docs/index.md` includes:

**Documentation Guidelines:**
- Filename rules
- Organization principles
- When to archive vs. delete
- How to maintain index
- Contribution workflow

## Validation Checklist

- [ ] All documentation files reviewed
- [ ] Files categorized correctly
- [ ] Filenames follow standards
- [ ] Directory structure created
- [ ] Files moved to correct locations
- [ ] Documentation index updated
- [ ] No broken links
- [ ] Guidelines documented
- [ ] Changes ready for commit

## Common Patterns

### Development Session Notes
**Pattern:** Session notes documenting a specific development task or investigation

**Location:** `docs/archive/development/YYYY-MM-DD-description.md`

**Criteria:**
- Describes a specific task or investigation
- Time-bound (completed work)
- May contain temporary decisions or approaches
- Useful for historical reference

### Test Results
**Pattern:** Results from test runs, verification, or validation

**Location:** `docs/archive/testing/YYYY-MM-DD-description.md`

**Criteria:**
- Test execution results
- Verification reports
- Performance benchmarks
- May become obsolete quickly

### Active Guides
**Pattern:** Step-by-step instructions for users

**Location:** `docs/guides/descriptive-name.md`

**Criteria:**
- Intended for current use
- Regularly updated
- Referenced by users
- Part of standard workflow

### Architecture Documentation
**Pattern:** System design and technical architecture

**Location:** `docs/architecture/component-name.md`

**Criteria:**
- Describes system design
- Technical specifications
- Component relationships
- Maintained as system evolves

## Integration with Tracking Repo

This prompt should be available in the tracking repository's MCP server so that any Accumulate repository can use it for documentation management.

**Implementation:**
1. Add to tracking-repo MCP prompts
2. Include in `.tracking-repo/mcp/prompts/`
3. Make available via `prompts/list` and `prompts/get`
4. Reference in tracking-repo guidelines

## Example Usage

```json
{
  "method": "prompts/get",
  "params": {
    "name": "organize-project-documentation",
    "arguments": {
      "action": "organize",
      "scope": "all",
      "create_index": "true",
      "dry_run": "false"
    }
  }
}
```

## Related Tracking Repo Rules

This prompt should incorporate tracking-repo filename standards:
- Lowercase with hyphens
- Descriptive names
- Date prefixes for archives
- No version numbers in names
- Clear topic organization

## Maintenance

This prompt template should be updated when:
- Documentation standards change
- New documentation categories are added
- Organization structure evolves
- Best practices are refined

---

*Template Version: 1.0*
*Last Updated: 2025-11-23*
