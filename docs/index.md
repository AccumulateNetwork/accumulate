# Accumulate Documentation Index

This index provides organized access to all Accumulate project documentation.

## Network & Deployment

### Network Information
- [MainNet Topology](network/mainnet-topology.md) - Current network topology, nodes, and configuration

### Bootstrap Server Deployment
- [Bootstrap Deployment Guide](deployment/bootstrap/deployment-guide.md) - Complete guide for deploying bootstrap servers
- [Bootstrap Server Reference](deployment/bootstrap/server-reference.md) - Bootstrap server operations and configuration

## Guides

- [Configuration Validation](guides/configuration-validation.md) - Guide for validating Accumulate configuration files
- [MCP Prompts Usage](guides/prompts-usage.md) - How to use MCP server prompts for common workflows

## Architecture

- [Bootstrap Server Architecture](architecture/bootstrap-architecture.md) - Technical architecture of the bootstrap server system

## Design Documents

- [Prompts Design Specification](design/prompts-design.md) - Design specification for MCP server prompts

## Development Archives

Historical development notes and session logs are archived for reference:

### Development Sessions
Located in [archive/development/](archive/development/):

**Database & Validation**
- [2025-10-27: BadgerDB Validation Investigation](archive/development/2025-10-27-badger-validation-investigation.md)

**Deployment Sessions**
- [2025-11-16: Follower Deployment Session](archive/development/2025-11-16-follower-deployment-session.md)
- [2025-11-19: July13 Genesis Deployment Log](archive/development/2025-11-19-deployment-log-july13.md)

**Bootstrap Server Development**
- [Bootstrap Info Server Implementation](archive/development/bootstrap-info-server-implementation.md)
- [Bootstrap Metrics Enhancement](archive/development/bootstrap-metrics-enhancement.md)
- [Bootstrap Quick Fix](archive/development/bootstrap-quick-fix.md)

**MCP Server Development**
- [MCP Fixes Applied](archive/development/mcp-fixes-applied.md)
- [MCP Implementation Review](archive/development/mcp-implementation-review.md)
- [MCP Phase 4: Prompts Implementation](archive/development/mcp-phase4-prompts-implementation.md)
- [MCP Prompt Analysis](archive/development/mcp-prompt-analysis.md)
- [MCP Prompt Analysis Checklist](archive/development/mcp-prompt-analysis-checklist.md)
- [MCP Prompt Analysis Process](archive/development/mcp-prompt-analysis-process.md)
- [MCP Deployment Issues](archive/development/mcp-deployment-issues.md)
- [MCP Review Summary](archive/development/mcp-review-summary.md)

### Test Results
Located in [archive/testing/](archive/testing/):

- [MCP Final Verification](archive/testing/mcp-final-verification.md)
- [MCP Honest Test Status](archive/testing/mcp-honest-test-status.md)
- [MCP Test Results - Phase 4](archive/testing/mcp-test-results-phase4.md)

## Documentation Guidelines

When creating new documentation:

### Filename Rules
- Use lowercase with hyphens: `my-document.md`
- Include dates for session notes: `YYYY-MM-DD-description.md`
- Use descriptive names that indicate content
- Avoid all-caps filenames except for standard files (README.md, CHANGELOG.md)

### Organization
- **Active documentation** goes in `docs/<topic>/`
- **Development sessions** go in `docs/archive/development/`
- **Test results** go in `docs/archive/testing/`
- **Obsolete docs** with information already integrated elsewhere should be deleted

### When to Archive
Archive documentation when:
- It's a development session or meeting note
- It's a test result or verification report
- The information is historical but may be useful for reference
- The work described is complete and documented elsewhere

### When to Delete
Delete documentation when:
- Information is fully integrated into active documentation
- Content is outdated and no longer relevant
- File is a duplicate of existing documentation

## Additional Resources

For more Accumulate documentation:
- Main README: [../README.md](../README.md)
- MCP Server: [../mcp/readme.md](../mcp/readme.md)
- Tools: [../tools/](../tools/)

---

*Last updated: 2025-11-23*
