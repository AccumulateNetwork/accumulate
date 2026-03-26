# Accumulate MCP Server Development Plan

## Focus: Mainnet Follower Deployment

**Date:** 2025-11-24
**Priority:** Production-ready follower deployment against Accumulate mainnet
**Scope:** MCP server improvements, prompts, and sampling strategies
**Note:** Staking integration deferred to separate development phase

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Current State Analysis](#current-state-analysis)
3. [Phase 1: Critical Fixes for Follower Deployment](#phase-1-critical-fixes-for-follower-deployment)
4. [Phase 2: Improved Prompts and Workflows](#phase-2-improved-prompts-and-workflows)
5. [Phase 3: Sampling Strategies](#phase-3-sampling-strategies)
6. [Phase 4: Monitoring and Observability](#phase-4-monitoring-and-observability)
7. [Phase 5: Historical Database Integration](#phase-5-historical-database-integration)
8. [Phase 6: Light Client Support](#phase-6-light-client-support)
9. [Implementation Priorities](#implementation-priorities)
10. [Testing Strategy](#testing-strategy)

---

## Executive Summary

The Accumulate MCP server provides 60+ tools for network interaction, wallet management, and follower node deployment. This plan focuses on hardening the server for production mainnet follower deployment with emphasis on:

1. **Reliability** - Robust error handling and recovery
2. **Observability** - Comprehensive monitoring and diagnostics
3. **Usability** - Streamlined prompts and sampling patterns
4. **Completeness** - Fill gaps in current tooling

---

## Current State Analysis

### Existing Tool Categories

| Category | Tools | Status |
|----------|-------|--------|
| Wallet Management | 7 | Complete |
| Account/Transaction Queries | 12 | Complete |
| Network Status | 5 | Complete |
| Key Management | 7 | Complete |
| Token Operations | 6 | Complete |
| Historical Database | 10 | Needs tuning |
| Follower Deployment | 6 | Needs enhancement |
| Accman Artifacts | 5 | Complete |
| Bootstrap Monitoring | 1 | Needs enhancement |

### Existing Prompts

| Prompt | Purpose | Status |
|--------|---------|--------|
| `deploy-follower-node` | Complete deployment workflow | Good, needs updates |
| `monitor-follower-health` | Health monitoring | Good |
| `troubleshoot-follower-sync` | Sync diagnostics | Good |
| `setup-dev-wallet` | Development setup | Complete |
| `quick-node-status` | Fast status check | Good |
| `organize-documentation` | Doc management | Complete |

### Current Gaps

1. **Pre-deployment validation** - No tool to validate prerequisites
2. **Snapshot acquisition** - No automated snapshot download
3. **Peer discovery** - Limited to hardcoded peers
4. **Progress tracking** - No sync progress percentage
5. **Log analysis** - No structured log parsing
6. **Configuration validation** - No pre-flight config check
7. **Transaction monitoring** - No wait-for-confirmation tool
8. **Receipt/proof verification** - Infrastructure exists but not exposed

---

## Phase 1: Critical Fixes for Follower Deployment

### 1.1 New Tool: `accumulate_validate_prerequisites`

Validates all requirements before deployment attempt.

```go
// Tool definition
{
    Name: "accumulate_validate_prerequisites",
    Description: "Validate system prerequisites for follower deployment",
    InputSchema: {
        "work_dir": "string - Target working directory",
        "network": "string - mainnet or testnet (default: mainnet)"
    }
}

// Checks performed:
// - Disk space (minimum 100GB free)
// - Memory (minimum 8GB available)
// - Docker installed and running
// - Ports 16591-16593, 16691-16693 available
// - Network connectivity to bootstrap server
// - Directory permissions
```

**Implementation:** `mcp/server/tools_prerequisites.go`

### 1.2 New Tool: `accumulate_download_snapshots`

Automates snapshot acquisition from known sources.

```go
{
    Name: "accumulate_download_snapshots",
    Description: "Download database snapshots for follower initialization",
    InputSchema: {
        "network": "string - mainnet or testnet",
        "bvn": "string - BVN name (e.g., Cyclops, Apollo)",
        "output_dir": "string - Directory to save snapshots",
        "source": "string - snapshot source (bootstrap, archive, custom URL)"
    }
}

// Features:
// - Progress reporting
// - Checksum verification
// - Resume on failure
// - Multiple source fallback
```

**Implementation:** `mcp/server/tools_snapshot_download.go`

### 1.3 Enhanced Tool: `accumulate_init_follower`

Current issues:
- Hardcoded bootstrap peers may be stale
- No validation of snapshot integrity
- No pre-flight configuration check

Enhancements:
```go
// Add to existing tool:
// 1. Automatic peer discovery from bootstrap server
// 2. Snapshot integrity validation (check accumulate.db structure)
// 3. Configuration validation before copying
// 4. Estimated sync time based on snapshot age
```

### 1.4 New Tool: `accumulate_get_sync_progress`

Real-time sync progress with ETA.

```go
{
    Name: "accumulate_get_sync_progress",
    Description: "Get detailed sync progress for running follower",
    InputSchema: {
        "container_name": "string - Docker container name (default: accumulate-follower)",
        "include_rate": "bool - Include sync rate calculation"
    }
}

// Returns:
// - Current block height (local)
// - Network block height
// - Blocks remaining
// - Sync percentage
// - Estimated time to completion
// - Current sync rate (blocks/minute)
```

**Implementation:** `mcp/server/tools_sync_progress.go`

### 1.5 New Tool: `accumulate_analyze_logs`

Structured log analysis for diagnostics.

```go
{
    Name: "accumulate_analyze_logs",
    Description: "Analyze follower logs for errors and warnings",
    InputSchema: {
        "container_name": "string - Docker container name",
        "lines": "int - Number of recent lines to analyze (default: 500)",
        "filter": "string - Error level filter: all, error, warning, critical"
    }
}

// Returns:
// - Error summary by category
// - Most recent errors with context
// - Pattern detection (repeated errors)
// - Suggested remediation
```

**Implementation:** `mcp/server/tools_log_analysis.go`

### 1.6 Enhanced Tool: `accumulate_query_bootstrap_server`

Current tool queries bootstrap server info. Enhance with:

```go
// Additional capabilities:
// - Query all BVN peers (not just DN)
// - Compare peer lists across partitions
// - Detect stale/unreachable peers
// - Recommend best peers based on latency
```

---

## Phase 2: Improved Prompts and Workflows

### 2.1 New Prompt: `prepare-mainnet-follower`

Pre-deployment preparation workflow.

```yaml
name: prepare-mainnet-follower
description: Complete preparation for mainnet follower deployment
arguments:
  - name: work_dir
    description: Target working directory
    required: true
  - name: bvn
    description: BVN to follow (Cyclops, Apollo, Yutu, Chandrayaan)
    required: true
```

**Workflow Steps:**
1. Run `accumulate_validate_prerequisites`
2. Query bootstrap server for current network status
3. Download snapshots for DN and selected BVN
4. Validate snapshot integrity
5. Generate optimized configuration
6. Output deployment command

### 2.2 Enhanced Prompt: `deploy-follower-node`

Update existing prompt with:
- Prerequisites check integration
- Snapshot age warning (>7 days old)
- Dynamic peer selection from bootstrap server
- Post-deployment verification checklist
- Automatic first-hour monitoring schedule

### 2.3 New Prompt: `recovery-from-failure`

Guided recovery when follower fails.

```yaml
name: recovery-from-failure
description: Diagnose and recover from follower failure
arguments:
  - name: work_dir
    description: Follower working directory
    required: true
  - name: failure_type
    description: crashed, sync_stalled, no_peers, db_corruption, unknown
    required: false
```

**Workflow:**
1. Diagnose failure type if not provided
2. Collect relevant logs and state
3. Determine if recoverable or needs re-deploy
4. Execute recovery steps
5. Verify recovery successful

### 2.4 New Prompt: `upgrade-follower-version`

Safe version upgrade workflow.

```yaml
name: upgrade-follower-version
description: Upgrade follower to new version with rollback capability
arguments:
  - name: work_dir
    required: true
  - name: target_version
    required: true
  - name: backup_first
    default: true
```

**Workflow:**
1. Create backup of current state
2. Verify target version compatibility
3. Stop current follower
4. Pull new Docker image
5. Start with new version
6. Verify sync continues
7. Rollback instructions if failed

### 2.5 New Prompt: `mainnet-sync-status`

Quick mainnet comparison status.

```yaml
name: mainnet-sync-status
description: Compare local follower with mainnet status
arguments:
  - name: container_name
    default: accumulate-follower
```

**Output:**
```
Mainnet Status vs Local Follower
================================
Network Height: 15,234,567
Local Height:   15,234,123
Behind:         444 blocks (0.003%)
Sync Rate:      ~150 blocks/min
ETA to Sync:    ~3 minutes
Peers: 5 (DN: 3, BVN: 2)
Status: SYNCING ⏳
```

---

## Phase 3: Sampling Strategies

### 3.1 Recommended Sampling Patterns

#### Quick Status Check (Low Latency)
```
accumulate_follower_status → Done
```
Single tool, immediate response.

#### Full Health Assessment (Medium)
```
accumulate_validate_prerequisites
  ↓ (parallel)
accumulate_follower_status + accumulate_node_info + accumulate_network_status
  ↓
accumulate_get_sync_progress
```

#### Deployment Workflow (Complete)
```
accumulate_validate_prerequisites
  ↓
accumulate_query_bootstrap_server (get peers)
  ↓
accumulate_download_snapshots (if needed)
  ↓
accumulate_init_follower
  ↓
accumulate_run_follower
  ↓ (wait 30s)
accumulate_follower_status
  ↓
accumulate_get_sync_progress
```

#### Troubleshooting Workflow
```
accumulate_follower_status
  ↓
accumulate_analyze_logs
  ↓
accumulate_get_sync_progress (if running)
  ↓
accumulate_node_info (network connectivity)
  ↓
Prompt: troubleshoot-follower-sync
```

### 3.2 Prompt-to-Tool Mapping

| Prompt | Primary Tools | Optional Tools |
|--------|---------------|----------------|
| `prepare-mainnet-follower` | validate_prerequisites, download_snapshots | query_bootstrap_server |
| `deploy-follower-node` | init_follower, run_follower | follower_status, get_sync_progress |
| `monitor-follower-health` | follower_status, node_info, network_status | analyze_logs |
| `troubleshoot-follower-sync` | analyze_logs, follower_status | get_sync_progress |
| `recovery-from-failure` | analyze_logs, follower_status | stop_follower, remove_follower |
| `mainnet-sync-status` | get_sync_progress, network_status | node_info |

### 3.3 Parallel Execution Opportunities

**Safe to parallelize:**
- `accumulate_network_status` + `accumulate_node_info`
- `accumulate_follower_status` + `accumulate_analyze_logs`
- `accumulate_query_bootstrap_server` (multiple partitions)

**Must be sequential:**
- `accumulate_validate_prerequisites` → `accumulate_init_follower`
- `accumulate_init_follower` → `accumulate_run_follower`
- `accumulate_stop_follower` → `accumulate_remove_follower`

---

## Phase 4: Monitoring and Observability

### 4.1 New Tool: `accumulate_create_monitoring_config`

Generate monitoring configuration for external systems.

```go
{
    Name: "accumulate_create_monitoring_config",
    Description: "Generate monitoring configuration for Prometheus/Grafana",
    InputSchema: {
        "output_format": "prometheus, grafana-dashboard, alertmanager",
        "follower_endpoint": "string",
        "alert_destinations": "array of webhook URLs"
    }
}
```

### 4.2 New Tool: `accumulate_health_report`

Comprehensive health report suitable for logging.

```go
{
    Name: "accumulate_health_report",
    Description: "Generate structured health report",
    InputSchema: {
        "container_name": "string",
        "format": "json, markdown, text"
    }
}

// Output includes:
// - Timestamp
// - Node identity
// - Sync status (percentage, ETA)
// - Peer connections (count, quality)
// - Resource usage (CPU, memory, disk)
// - Recent errors (last 5)
// - Overall health score (0-100)
```

### 4.3 Metrics to Track

| Metric | Description | Alert Threshold |
|--------|-------------|-----------------|
| `blocks_behind` | Distance from network head | > 1000 |
| `peer_count` | Connected peers | < 3 |
| `sync_rate` | Blocks per minute | < 10 (when behind) |
| `error_rate` | Errors per hour | > 10 |
| `disk_usage` | Percentage used | > 90% |
| `memory_usage` | Percentage used | > 85% |
| `uptime` | Continuous uptime | N/A (track) |

---

## Phase 5: Historical Database Integration

### 5.1 Database Manager Tuning

Current issues identified:
- No connection limits
- No idle timeout
- No memory pressure handling

**Improvements:**

```go
// Enhanced DatabaseManager
type DatabaseManager struct {
    maxConnections int           // Default: 10
    idleTimeout    time.Duration // Default: 30min
    lastAccess     map[string]time.Time
    mu             sync.RWMutex
}

func (dm *DatabaseManager) GetDatabase(dbPath string) (database.Beginner, error) {
    dm.mu.Lock()
    defer dm.mu.Unlock()

    // Check connection limit
    if len(dm.connections) >= dm.maxConnections {
        dm.evictLRU()
    }

    // Update last access
    dm.lastAccess[dbPath] = time.Now()

    // ... existing logic
}

func (dm *DatabaseManager) CleanupIdle() {
    // Called periodically to close idle connections
    cutoff := time.Now().Add(-dm.idleTimeout)
    for path, lastAccess := range dm.lastAccess {
        if lastAccess.Before(cutoff) {
            dm.closeConnection(path)
        }
    }
}
```

### 5.2 Enhanced Tool: `accumulate_db_query_account`

Add timeout and retry logic:

```go
// Add to existing tool:
{
    "timeout_seconds": "int - Query timeout (default: 30)",
    "retry_count": "int - Retry on failure (default: 3)"
}
```

### 5.3 New Tool: `accumulate_db_compare_snapshots`

Compare two database snapshots.

```go
{
    Name: "accumulate_db_compare_snapshots",
    Description: "Compare account state between two database snapshots",
    InputSchema: {
        "database_a": "string - First database name or path",
        "database_b": "string - Second database name or path",
        "account": "string - Account URL to compare (optional, compares all if omitted)",
        "output": "summary, detailed, diff"
    }
}
```

---

## Phase 6: Light Client Support

### 6.1 Expose Receipt/Proof Options

The protocol infrastructure exists but is not exposed via MCP tools.

**New parameter for query tools:**

```go
// Add to accumulate_query_account, accumulate_query_tx:
{
    "include_receipt": "bool - Include Merkle receipt for verification",
    "include_proof": "bool - Include proof data for SPV verification"
}
```

### 6.2 New Tool: `accumulate_verify_receipt`

```go
{
    Name: "accumulate_verify_receipt",
    Description: "Verify a transaction receipt against anchor chain",
    InputSchema: {
        "txid": "string - Transaction ID",
        "receipt": "object - Receipt data from query",
        "anchor_height": "int - Optional anchor height to verify against"
    }
}
```

### 6.3 New Tool: `accumulate_get_anchor_proof`

```go
{
    Name: "accumulate_get_anchor_proof",
    Description: "Get anchor chain proof for cross-chain verification",
    InputSchema: {
        "partition": "string - Source partition",
        "block_height": "int - Block height to prove"
    }
}
```

---

## Implementation Priorities

### Priority 1: Immediate (Week 1)
1. `accumulate_validate_prerequisites` - Critical for deployment success
2. `accumulate_get_sync_progress` - Essential for monitoring
3. `accumulate_analyze_logs` - Critical for debugging
4. Enhanced `deploy-follower-node` prompt

### Priority 2: Near-term (Week 2)
1. `accumulate_download_snapshots` - Streamline deployment
2. `prepare-mainnet-follower` prompt
3. `recovery-from-failure` prompt
4. Enhanced bootstrap peer discovery

### Priority 3: Medium-term (Week 3-4)
1. `accumulate_health_report` - Production monitoring
2. `accumulate_create_monitoring_config` - Integration
3. Database manager tuning
4. `mainnet-sync-status` prompt

### Priority 4: Future
1. Light client tools (Phase 6)
2. Database comparison tools
3. Upgrade workflow prompt
4. Advanced sampling patterns

---

## Testing Strategy

### Unit Tests

Each new tool requires:
1. Input validation tests
2. Error handling tests
3. Mock response tests
4. Edge case tests

### Integration Tests

1. **Deployment flow test** - Full deployment on testnet
2. **Recovery test** - Simulate failures and recover
3. **Monitoring test** - Verify metrics collection
4. **Upgrade test** - Version upgrade workflow

### End-to-End Tests

1. **Fresh deployment** - Clean machine to running follower
2. **Snapshot restore** - Restore from old snapshot
3. **Network partition** - Handle network issues
4. **Resource exhaustion** - Handle disk/memory limits

### Prompt Tests

Each prompt should be tested with:
1. All required arguments
2. Optional arguments combinations
3. Invalid inputs
4. Workflow completion

---

## Appendix A: Tool Implementation Files

| Tool | File | Status |
|------|------|--------|
| validate_prerequisites | `tools_prerequisites.go` | New |
| download_snapshots | `tools_snapshot_download.go` | New |
| get_sync_progress | `tools_sync_progress.go` | New |
| analyze_logs | `tools_log_analysis.go` | New |
| health_report | `tools_health_report.go` | New |
| create_monitoring_config | `tools_monitoring.go` | New |
| db_compare_snapshots | `tools_db_compare.go` | New |
| verify_receipt | `tools_light_client.go` | New |
| get_anchor_proof | `tools_light_client.go` | New |

---

## Appendix B: Prompt Template Files

All prompt templates are in `mcp/server/prompts.go`:

| Prompt | Function |
|--------|----------|
| prepare-mainnet-follower | `generatePrepareMainnetFollowerTemplate` |
| recovery-from-failure | `generateRecoveryFromFailureTemplate` |
| upgrade-follower-version | `generateUpgradeFollowerVersionTemplate` |
| mainnet-sync-status | `generateMainnetSyncStatusTemplate` |

---

## Appendix C: Configuration Options

New configuration options for `mcp/server/config.go`:

```go
type Config struct {
    // Existing...

    // New options
    SnapshotSources    []string      // URLs for snapshot download
    DefaultBVN         string        // Default BVN for deployment
    SyncCheckInterval  time.Duration // Interval for sync progress checks
    LogAnalysisLines   int           // Default lines for log analysis
    HealthReportFormat string        // Default health report format

    // Database manager
    DBMaxConnections   int           // Max concurrent DB connections
    DBIdleTimeout      time.Duration // Idle connection timeout

    // Monitoring
    MetricsEnabled     bool          // Enable metrics collection
    MetricsPort        int           // Metrics HTTP port
}
```

---

## Appendix D: Error Codes

Standardized error codes for consistent handling:

| Code | Category | Description |
|------|----------|-------------|
| 1001 | Network | Bootstrap server unreachable |
| 1002 | Network | Peer connection failed |
| 1003 | Network | Network status unavailable |
| 2001 | Storage | Insufficient disk space |
| 2002 | Storage | Database corruption detected |
| 2003 | Storage | Snapshot integrity check failed |
| 3001 | Container | Docker not available |
| 3002 | Container | Container start failed |
| 3003 | Container | Container not running |
| 4001 | Sync | Sync stalled |
| 4002 | Sync | No peers available |
| 4003 | Sync | Block height regression |
| 5001 | Config | Invalid configuration |
| 5002 | Config | Missing required parameter |
| 5003 | Config | Incompatible version |

---

## Revision History

| Date | Version | Changes |
|------|---------|---------|
| 2025-11-24 | 1.0 | Initial plan focused on mainnet follower deployment |
