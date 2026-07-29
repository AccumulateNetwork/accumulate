# MCP Knowledge Base System

This system helps AI assistants work more effectively on the Accumulate codebase by providing:

1. **Structured Documentation** - Authoritative sources for each topic
2. **Lessons Learned** - Past problems and their solutions/workarounds
3. **Fixed Decisions** - Configuration and architectural choices that must not be changed
4. **Current Status** - What's working, what's not, and blockers

## MCP Tools

### `accumulate_get_knowledge`

Get structured knowledge about a topic. **Use this first when starting work on any topic.**

```json
{
  "topic": "follower-deployment",
  "section": "lessons"  // Optional: "documentation", "lessons", "decisions", "status"
}
```

Returns documentation paths, lessons learned, decisions, and current status.

### `accumulate_check_decisions`

Before modifying a file, check for applicable decisions. **Prevents accidentally undoing fixes.**

```json
{
  "topic": "follower-deployment",
  "file_path": "tools/follower-monitor/main.go",
  "proposed_change": "Change port to 26657"
}
```

Returns violations if the change conflicts with fixed decisions.

### `accumulate_record_lesson`

Record a new lesson when you encounter and solve a problem.

```json
{
  "topic": "follower-deployment",
  "title": "CometBFT requires valid commit signatures",
  "problem": "Starting at non-genesis height fails with invalid signature error",
  "error_message": "panic: failed to verify vote",
  "root_cause": "Snapshot format doesn't include CometBFT commit signatures",
  "workaround": "Copy blockstore.db from original validator backup",
  "affected_files": ["internal/node/daemon/snapshots.go"]
}
```

## Knowledge Base Format

Knowledge bases are YAML files in `mcp/knowledge/`:

```yaml
version: "1.0"
topic: "topic-name"
last_updated: "2025-11-30"

documentation:
  primary:
    - path: "docs/path/to/authoritative.md"
      description: "Description"
      authority: "definitive"  # definitive, reference, implementation
      topics: ["topic1", "topic2"]

lessons_learned:
  - id: "LESSON-001"
    title: "Short title"
    problem: "What happened"
    error_message: "Exact error"
    root_cause: "Why it happened"
    solution_status: "RESOLVED|UNRESOLVED|WORKAROUND_AVAILABLE"
    required_fix: "How to fix permanently"
    workaround: "Temporary workaround"
    affected_files: ["path/to/file.go"]

decisions:
  - id: "DECISION-001"
    title: "Port configuration"
    decision: "Use ports 16592/16692"
    rationale: "Why this decision"
    values:
      dn_rpc_port: 16592
      bvn_rpc_port: 16692
    DO_NOT:
      - "Do not use ports 26657/26656"
    affected_files: ["tools/follower-monitor/main.go"]

current_status:
  working: ["List of working things"]
  not_working: ["List of broken things"]
  blockers:
    - description: "Blocker description"
      severity: "critical"
```

## How This Helps AI

### Problem 1: Hard to Find Documentation
**Before**: AI searches through scattered docs, may find outdated info
**After**: `accumulate_get_knowledge` returns authoritative sources ranked by importance

### Problem 2: No Memory of Lessons Learned
**Before**: Each session starts fresh, same mistakes repeated
**After**: Lessons are persisted in YAML, `get_knowledge` surfaces them proactively

### Problem 3: Regression of Fixes
**Before**: AI might change ports back to wrong values
**After**: `check_decisions` returns violations before changes are made

## Usage in AI Workflow

1. **Starting a task**: Call `accumulate_get_knowledge` first
2. **Before editing a file**: Call `accumulate_check_decisions`
3. **After solving a problem**: Call `accumulate_record_lesson`

## Example Session

```
AI: Starting work on follower deployment...
AI: [Calls accumulate_get_knowledge topic="follower-deployment"]

Response shows:
- Primary doc: docs/operations/snapshot-restore-complete-analysis.md
- LESSON-001: Snapshot missing commit signatures (UNRESOLVED)
- DECISION-001: Ports are 16592/16692

AI: I see there's an unresolved issue with commit signatures.
    The workaround is to copy blockstore.db from backup.
    I also see the port configuration is fixed at 16592/16692.

AI: [Before editing follower-monitor, calls accumulate_check_decisions]
    file_path="tools/follower-monitor/main.go"

Response:
- DECISION-001 applies to this file
- DO_NOT: Use ports 26657/26656

AI: I'll make sure to use the correct ports 16592/16692.
```

## Creating New Knowledge Bases

Create a new YAML file in `mcp/knowledge/` named `<topic>.yaml`.

Topics should be specific but not too narrow. Good examples:
- `follower-deployment` - Everything about deploying followers
- `snapshot-restore` - Snapshot creation and restoration
- `mcp-server` - MCP server development

## Maintenance

Knowledge bases should be updated when:
- A new lesson is learned (use `accumulate_record_lesson`)
- A decision is made that should be permanent (edit YAML manually)
- Documentation paths change (edit YAML manually)
- Status changes (blockers resolved, etc.)
