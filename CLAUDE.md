# Accumulate Protocol - Development Notes

## MANDATORY: Review Tracking Repository Before Development

**FIRST ACTION before any development work:**
1. **Review**: `~/go/src/gitlab.com/AccumulateNetwork/tracking_repo/CLAUDE.md`
2. **Check**: Latest development rules, file naming standards, TDD requirements
3. **Update**: Any changes in development process or organization
4. **Verify**: Understanding of pre-merge simplification requirements

**This ensures all development follows current standards and organization.**

## Tracking MCP Server

Use the tracking MCP for project management and standards enforcement.

### Before Starting Work
- Call `start-new-issue` prompt to verify issue/MR status
- Call `find-docs` to search relevant documentation

### Before Commits
- Call `pre-commit-check` prompt for validation
- Call `verify-test-coverage` to confirm 80% coverage

### Before Merge Requests
- Call `pre-mr-validation` prompt for complete validation
- Call `verify-issue-status` to confirm issue/MR states

### Available Tools
- `verify-issue-status` - Check GitLab issue and MR status
- `validate-filename` - Check filename naming rules
- `check-test-first` - Verify TDD compliance
- `verify-test-coverage` - Confirm coverage threshold
- `query-issues` - Search issues with filters
- `generate-report` - Create formatted reports
- `find-docs` - Search documentation

## Project Overview

This is the core Accumulate Protocol implementation including:
- `accumulated` - Main blockchain node binary
- `cmd/` - CLI tools and binaries
- `pkg/` - Core protocol packages
- `internal/` - Internal implementation
- `protocol/` - Protocol definitions
- `mcp/` - MCP server for AI integration

## Development Workflow

### Using the Orchestrator (MANDATORY)

All work MUST use the orchestrator (`orch`) for workflow enforcement:

```bash
# Start work on an issue
orch start 3715 accumulate

# Check status
orch status

# Commit changes (auto-formats with issue reference)
orch commit "Add new feature"

# Create draft merge request
orch mr

# After MR is merged, finish the session
orch finish
```

The orchestrator:
- Creates proper issue branches (`issue-{N}-{description}`)
- Enforces commit message format (`Issue #N: message`)
- Targets draft branch for MRs (not main)
- Logs activity to tracking repo

### TDD Development Rules
- Follow full TDD process during development
- **MANDATORY**: Simplify and remove AI artifacts before creating merge requests
- Remove TDD scaffolding, excessive interfaces, AI-guidance comments
- Focus final code on business clarity and maintainability
- Maintain test coverage after simplification

### File Naming Standards
- **NEVER** use ALL_CAPS_FILE_NAMES
- **ALWAYS** use lowercase-with-hyphens

## Key Commands

```bash
# Build
go build ./cmd/accumulated

# Run tests
go test ./...

# Run with devnet
accumulated run devnet --bvns 3
```

## Related Repositories

| Repository | Purpose |
|------------|---------|
| **devnet** | Devnet CLI and Kermit operations |
| **wallet** | CLI wallet and MCP wallet server |
| **staking** | Staking rewards system |
| **explorer** | Block explorer |
