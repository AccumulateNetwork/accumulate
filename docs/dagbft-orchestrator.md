# DAG-BFT Integration Orchestrator

This document describes how to use the orchestrator to manage parallel work on DAG-BFT integration issues.

## Prerequisites

- Orchestrator installed: `/home/paul/go/src/github.com/PaulSnow/orchestrator`
- tmux installed
- glab CLI configured for GitLab access

## Configuration

The orchestrator config is at `config/dagbft-issues.json`. It defines:

- **Project**: `dagbft-integration`
- **Base branch**: `dagbft-integration`
- **Pipeline stages**: research → implement → validate → review
- **Workers**: Up to 5 parallel Claude Code instances

## Issues Being Tracked

| Issue | Title | Priority |
|-------|-------|----------|
| #3813 | Wire GossipSub into accumulated integration | 1 (blocking) |
| #3815 | Implement BPT sync recovery | 2 |
| #3816 | Backpressure improvements for high load | 2 |
| #3817 | Complete accumulated integration (tracking) | 3 |

## Usage

### Launch Workers

```bash
# From accumulate repo root
orch=/home/paul/go/src/github.com/PaulSnow/orchestrator/scripts/orch
$orch launch --config config/dagbft-issues.json
```

### Check Status

```bash
$orch status --config config/dagbft-issues.json
```

### Monitor in tmux

```bash
tmux attach -t orchestrator
# Ctrl-b then select 'dashboard' window for live view
```

### Worker Logs

```bash
tail -f /tmp/dagbft-integration-worker-1.log
```

### Cleanup

```bash
tmux kill-session -t orchestrator
rm -f /tmp/dagbft-integration-*
```

## Worktrees

Each issue gets its own worktree at:
```
/home/paul/worktrees/accumulate-dagbft/issue-{number}
```

Branches are named `issue/dagbft-{number}`.

## Pipeline Stages

1. **research**: Explore codebase, document findings in `docs-dev/research/`
2. **implement**: Write code, tests, commit to issue branch
3. **validate**: Run tests, verify build
4. **review**: Final review, create MR

## Advancing Stages Manually

If the monitor fails, manually update `config/dagbft-issues.json`:
- Set `pipeline_stage` to next stage number (0=research, 1=implement, etc.)
- Set `status` to `"pending"`
- Set `assigned_worker` to `null`
- Relaunch orchestrator
