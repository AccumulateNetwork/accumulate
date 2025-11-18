# Snapshot Restore Design - Accman Perspective Review

**Date**: 2025-11-18
**Reviewer**: Accman maintainer perspective
**Design Document**: `SNAPSHOT_RESTORE_DEPLOYMENT.md`
**Context**: Week-long sync times from July snapshots making deployment impractical

> **Note**: This document provides the accman team's review and support for the snapshot restore design. It addresses integration concerns, identifies critical issues requiring coordination, and proposes an implementation approach that maintains separation of concerns between accman (Docker orchestration) and accumulated (protocol).

---

## Executive Summary

**Verdict**: ✅ **Strongly support** with recommended integration approach

**Key Benefits for Accman**:
- 🚀 Deployment time: 5 days → 10 minutes (99.9% improvement)
- 🎯 Maintains separation of concerns (accman=docker, accumulated=protocol)
- 🔌 Clean integration via MCP protocol
- 📦 Backward compatible with existing accman workflows

**Critical Issues to Resolve**:
1. Port numbering conflict (16xxx vs 52xxx)
2. Integration pattern needs clarification
3. Snapshot distribution mechanism undefined

---

## Problem Statement Validation

### Current Pain Point: CRITICAL

**Measured Impact**:
- Sync from July 2024 snapshot: **5-7+ days**
- Sync from genesis (10.6M blocks): **Weeks**
- User experience: **Unacceptable**

**Business Impact**:
- Users cannot deploy quickly for testing
- High barrier to entry for node operators
- Increased infrastructure costs (compute time)
- Poor competitive positioning vs other chains

**Accman Perspective**: This is the #1 blocker for accman usability. The snapshot restore design directly addresses our biggest user complaint.

---

## Architecture Review

### Proposed Flow

```
User → Accman → Accumulate MCP → accumulated restore-snapshot → Node Dirs → Accman Docker Deploy
```

### Separation of Concerns: ✅ EXCELLENT

| Component | Responsibility | Status |
|-----------|---------------|--------|
| **Accumulate MCP** | - Snapshot restore<br>- Node directory creation<br>- Protocol-level config | ✅ Protocol layer |
| **Accman** | - Docker orchestration<br>- Port mapping<br>- Multi-follower management<br>- User interface | ✅ Deployment layer |
| **Accumulated** | - Binary execution<br>- Node operations | ✅ Core protocol |

**Assessment**: Perfect separation. Accman stays in its lane (Docker/automation), delegates protocol details to accumulated via MCP.

---

## Integration Analysis

### Option 1: Call Accumulate MCP Directly (RECOMMENDED)

**Flow**:
```
Accman → HTTP/stdio → Accumulate MCP → accumulate_restore_from_snapshots → Node dirs
      → Accman continues with existing Docker deploy logic
```

**Pros**:
- ✅ Clean separation of concerns
- ✅ No duplicate code
- ✅ Accumulate MCP owns protocol knowledge
- ✅ Accman stays focused on Docker orchestration
- ✅ Easy to update when protocol changes

**Cons**:
- ⚠️ Requires accumulate MCP server running
- ⚠️ Additional dependency to manage

**Accman Changes Required**:
```go
// New method in accman MCP or library
func PrepareFollowerFromSnapshot(cfg *SnapshotRestoreConfig) (*NodeDirectories, error) {
    // 1. Call accumulate MCP tool: accumulate_restore_from_snapshots
    // 2. Return node directory paths
    // 3. Existing deploy_follower logic continues
}
```

### Option 2: Wrap in Accman (NOT RECOMMENDED)

**Flow**:
```
Accman → Execute accumulated binary directly → Node dirs → Docker deploy
```

**Pros**:
- ✅ No external MCP dependency

**Cons**:
- ❌ Duplicates protocol knowledge in accman
- ❌ Tight coupling to accumulated internals
- ❌ Hard to maintain when accumulated changes
- ❌ Violates separation of concerns

**Assessment**: Avoid this approach. Goes against accman's design philosophy.

### Option 3: Hybrid (ALTERNATIVE)

**Flow**:
```
Accman → Detect snapshot type → Route to appropriate method
    .snap files → Call accumulate MCP
    .tar.gz databases → Existing accman logic
```

**Pros**:
- ✅ Supports both snapshot types
- ✅ Backward compatible
- ✅ Flexible for different use cases

**Cons**:
- ⚠️ More complexity in accman
- ⚠️ Need to maintain two paths

---

## Critical Issues

### 1. Port Numbering Conflict 🔴 HIGH PRIORITY

**Problem**:
- Design uses: DN=16591-16593, BVN=16691-16693
- Accman uses: DN=52000-52002, BVN=52100-52102

**Impact**:
- Breaks existing accman deployments
- Confuses users
- Different ports for different methods

**Recommended Solution**:

**Make ports configurable in accumulate_restore_from_snapshots**:

```json
{
  "dn_snapshot": "...",
  "bvn_snapshot": "...",
  "work_dir": "...",
  "ports": {
    "dn_listen": 52000,
    "dn_api": 52001,
    "dn_p2p": 52002,
    "bvn_listen": 52100,
    "bvn_api": 52101,
    "bvn_p2p": 52102
  }
}
```

**Rationale**:
- Accman maintains its port convention (52xxx)
- Other tools can use 16xxx if they want
- Port offset becomes port_base instead
- Eliminates confusion

**Alternative**:
- Standardize on 16xxx across all tools
- Accman migrates to match
- Document breaking change

**Recommendation**: Make ports configurable. Don't force one convention.

### 2. Snapshot Distribution 🟡 MEDIUM PRIORITY

**Problem**: Design doesn't specify how users get .snap files

**Questions**:
- Where do users download snapshots from?
- How often are they created?
- Who creates them?
- How are they verified/trusted?

**Accman Impact**:
- If snapshots aren't readily available, this feature is useless
- Need snapshot repository/CDN
- Need accman integration to download snapshots

**Recommended Additions**:

Add to accumulate MCP:
```json
{
  "method": "accumulate_download_snapshot",
  "params": {
    "network": "MainNet",
    "partition": "Directory|Cyclops",
    "output_dir": "/snapshots",
    "latest": true
  }
}
```

Add to accman:
```json
{
  "method": "deploy_follower_from_snapshots",
  "params": {
    "network": "MainNet",
    "auto_download_snapshots": true,  // Download if not present
    "snapshot_dir": "/var/snapshots",
    "container_name": "follower-1"
  }
}
```

### 3. Port Offset vs Port Configuration 🟡 MEDIUM PRIORITY

**Design uses port_offset**:
```
Follower 1: 16591 + 0 = 16591
Follower 2: 16591 + 100 = 16691
```

**Accman uses individual port params**:
```
dn_p2p_port: 52000
dn_rpc_port: 52001
```

**Problem**: Different approaches to multi-follower support

**Recommendation**:
Support BOTH in accumulate MCP:
- `port_offset` for simple increments
- Individual port params for full control
- Port params override port_offset if both provided

### 4. Docker Port Mapping Confusion 🟡 MEDIUM PRIORITY

**Design example (line 342)**:
```bash
docker run -d \
  --name accumulate-follower-2 \
  -p 16691-16693:16591-16593 \  # Maps host 16691-16693 to container 16591-16593
  -p 16791-16793:16691-16693 \  # Maps host 16791-16793 to container 16691-16693
```

**Problem**: This is confusing. Container ports should match configured ports.

**Accman Approach** (cleaner):
```bash
docker run -d \
  --name accumulate-follower-2 \
  -p 53000:52000 \  # Host port matches what user specified
  -p 53001:52001 \
```

**Recommendation**:
- accumulate_restore_from_snapshots configures node with final ports
- Docker maps those ports 1:1 to host
- No port translation confusion

---

## Accman Integration Proposal

### Phase 1: Basic Integration (Immediate)

**New accman MCP method**:
```json
{
  "method": "deploy_follower_from_snapshots",
  "params": {
    "dn_snapshot": "/snapshots/dn.snap",
    "bvn_snapshot": "/snapshots/bvn.snap",
    "container_name": "follower-1",
    "volume_name": "follower-1-data",
    "binary_path": "/usr/local/bin/accumulated",
    "use_volume": true,
    "dn_p2p_port": "52000",
    "dn_rpc_port": "52001",
    "bvn_p2p_port": "52100",
    "bvn_rpc_port": "52101"
  }
}
```

**Implementation**:
1. Call accumulate MCP `accumulate_restore_from_snapshots` with user params
2. Get back node directories (dnn/, bvnn/, accumulate.toml)
3. Use existing accman Docker deployment logic
4. Return standard accman response

**Code changes**:
- Add new method to `cmd/accman-mcp/main.go`
- Add `DeployFollowerFromSnapshots()` to `pkg/accman/deploy/`
- Call accumulate MCP via HTTP or stdio
- Reuse existing Docker orchestration code

### Phase 2: Enhanced Integration (Follow-up)

**Add snapshot download**:
```json
{
  "method": "deploy_follower_from_network",
  "params": {
    "network": "MainNet",
    "bvn": "Cyclops",
    "auto_download_snapshots": true,
    "container_name": "follower-1"
  }
}
```

**Implementation**:
1. Call accumulate MCP to download latest snapshots
2. Call accumulate MCP to restore from snapshots
3. Deploy with Docker
4. One-command deployment

### Phase 3: Migration Path (Future)

**Deprecate old methods gracefully**:
- `deploy_follower` (tar.gz databases) → Still works, marked as "slow path"
- `deploy_follower_from_snapshots` → Recommended, fast path
- `deploy_follower_from_network` → Ultimate convenience

**Documentation**:
```markdown
## Deployment Methods

### Recommended: Snapshot-Based (5-10 minutes)
Use when deploying new followers or upgrading existing ones.

### Legacy: Database Sync (5+ days)
Use only for special cases or if snapshots unavailable.
```

---

## Backward Compatibility

### Existing Accman Users

**Impact**: ✅ Zero breaking changes

**Reasoning**:
- New method doesn't affect existing `deploy_follower`
- Users can continue using .tar.gz databases
- Snapshot-based deployment is additive feature
- Port configuration already supports custom ports

### Migration Path

**Users can migrate incrementally**:
1. Continue using existing deployments
2. Try snapshot-based for new followers
3. Migrate existing followers when convenient
4. No forced migration timeline

---

## Performance Analysis

### Deployment Time Comparison

| Method | Time | User Experience |
|--------|------|-----------------|
| **Genesis sync** | 5+ days | ❌ Unacceptable |
| **July snapshot sync** | 5-7 days | ❌ Still too long |
| **Database tar.gz** | Varies | ⚠️ Depends on DB age |
| **Snapshot restore** | 5-10 min | ✅ Excellent |

**Impact on Accman Users**:
- Testing deployments: Immediate feedback instead of days
- Production deployments: Same-day instead of same-week
- Disaster recovery: Minutes instead of days
- Development cycles: Massively accelerated

### Resource Requirements

**Snapshot restore**:
- CPU: Low (mostly I/O)
- RAM: 4-8 GB (same as current)
- Disk: Same as current (~15 GB per follower)
- Network: Snapshot download size (DN 21MB + BVN 1.5GB)

**Accman Impact**: ✅ No additional resource requirements

---

## Security Considerations

### Snapshot Trust Model

**Critical Question**: How do users verify snapshots are legitimate?

**Concerns**:
1. Malicious snapshots could contain incorrect state
2. Network split if followers restore from different snapshots
3. No verification mechanism in design

**Recommended Additions**:

**Snapshot Signing**:
```json
{
  "snapshot_file": "dn-20251118.snap",
  "signature_file": "dn-20251118.snap.sig",
  "public_key": "validator-pubkey.pem"
}
```

**Accman Integration**:
```go
// Before restoring, verify signature
if cfg.VerifySnapshots {
    if err := verifySnapshotSignature(snapshot, signature, pubkey); err != nil {
        return err
    }
}
```

**BPT Hash Verification**:
- Snapshots include BPT root hash
- Compare against known-good hash from multiple sources
- Warn if mismatch

---

## Operational Considerations

### Snapshot Management

**Questions for Accumulate Team**:
1. How often are snapshots created? (Daily? Weekly?)
2. Where are they hosted? (S3? CDN? GitLab releases?)
3. Retention policy? (Keep last 30 days? 90 days?)
4. Size growth? (1.5 GB now, what in 6 months?)

**Accman Impact**:
- Need to document snapshot locations
- May need snapshot caching mechanism
- Storage requirements for snapshot cache

### Monitoring

**What Accman Should Monitor**:
- Snapshot restore success/failure rate
- Time to restore (track degradation)
- Post-restore sync time (should be minimal)
- Disk space for snapshot cache

**Metrics to Add**:
```go
type SnapshotRestoreMetrics struct {
    RestoreDurationSeconds float64
    DNSnapshotSizeBytes    int64
    BVNSnapshotSizeBytes   int64
    PostRestoreSyncBlocks  int64
    SuccessRate            float64
}
```

---

## Documentation Requirements

### For Accman Users

**New guides needed**:
1. `snapshot_based_deployment.md`
   - What are .snap files?
   - Where to get them?
   - How to verify them?
   - Step-by-step deployment

2. Update `multiple_followers_guide.md`
   - Add snapshot-based examples
   - Compare to database-based approach
   - Performance benefits

3. Update `README-MCP.md`
   - Add `deploy_follower_from_snapshots` method
   - Show example requests/responses
   - Link to snapshot guide

### For Accumulate Team

**Requests for accumulate MCP docs**:
1. Snapshot creation guide for validators
2. Snapshot format specification
3. BPT hash verification procedure
4. Snapshot repository/hosting details

---

## Implementation Roadmap

### Phase 1: Core Integration (Week 1)

**Accumulate Team**:
- [ ] Implement `accumulate_restore_from_snapshots` in accumulate MCP
- [ ] Make ports configurable (not just offset)
- [ ] Add snapshot verification support
- [ ] Document snapshot hosting location

**Accman Team**:
- [ ] Add `deploy_follower_from_snapshots` method
- [ ] Integrate with accumulate MCP (HTTP client)
- [ ] Test with provided snapshots
- [ ] Document new method

### Phase 2: Enhanced Features (Week 2-3)

**Accumulate Team**:
- [ ] Implement `accumulate_download_snapshot`
- [ ] Set up snapshot repository/CDN
- [ ] Automated snapshot creation pipeline
- [ ] Snapshot verification tooling

**Accman Team**:
- [ ] Add `deploy_follower_from_network` method
- [ ] Implement snapshot caching
- [ ] Add monitoring metrics
- [ ] Create comprehensive guide

### Phase 3: Migration & Optimization (Week 4+)

**Both Teams**:
- [ ] User feedback collection
- [ ] Performance optimization
- [ ] Documentation improvements
- [ ] Deprecation planning for old methods

---

## Questions for Accumulate Team

### Critical Questions (Need answers before implementation)

1. **Port Configuration**:
   - Can we make ports fully configurable instead of just offset?
   - Should we standardize on 16xxx or 52xxx, or support both?

2. **Snapshot Distribution**:
   - Where will snapshots be hosted?
   - What's the download URL pattern?
   - How are they verified/signed?

3. **MCP Communication**:
   - Should accman call accumulate MCP via HTTP or stdio?
   - What's the expected address/port for accumulate MCP?
   - Will accumulate MCP be a separate process or embedded?

4. **Configuration Handoff**:
   - After restore, does accumulate.toml include all necessary config?
   - Or does accman need to modify it?
   - Who owns the final config file?

### Nice-to-Have Questions (Can answer later)

5. **Snapshot Metadata**:
   - Can snapshots include metadata file (JSON)?
   - Version, block height, hash, creation time?

6. **Incremental Snapshots**:
   - Timeline for incremental/delta snapshots?
   - Would reduce download size after initial deployment

7. **Multi-BVN Support**:
   - Can one follower track multiple BVNs?
   - Or separate deployment per BVN?

---

## Recommendations Summary

### For Accumulate Team

1. ✅ **Implement as designed** - Architecture is sound
2. 🔧 **Make ports configurable** - Not just offset
3. 📦 **Define snapshot distribution** - CDN, verification, automation
4. 🔐 **Add snapshot verification** - Signatures, BPT hashes
5. 📝 **Document snapshot creation** - For validator operators

### For Accman Team

1. ✅ **Integrate via MCP** - Don't duplicate protocol logic
2. 🔌 **Add new methods** - `deploy_follower_from_snapshots`, etc.
3. 📖 **Update documentation** - New guides for snapshot-based deployment
4. 🔄 **Maintain backward compat** - Don't break existing deployments
5. 📊 **Add monitoring** - Track snapshot restore metrics

### For Both Teams

1. 🤝 **Coordinate on ports** - Agree on standard or make flexible
2. 📚 **Cross-reference docs** - Link between accumulate and accman guides
3. 🧪 **Test integration** - End-to-end scenarios
4. 👥 **User feedback** - Beta test with real users

---

## Conclusion

### Overall Assessment: ✅ STRONGLY SUPPORT

**This design is exactly what accman needs:**
- Solves the #1 user pain point (deployment time)
- Maintains clean separation of concerns
- Integrates naturally with accman's architecture
- Backward compatible
- Positions accman as modern, fast deployment tool

**Critical Dependencies**:
1. Port configuration flexibility
2. Snapshot distribution infrastructure
3. Clear MCP communication protocol

**Timeline Impact**:
- With proper coordination: **2-3 weeks to production**
- Without coordination: Could drag on indefinitely

**Recommendation**:
- ✅ **Proceed with implementation**
- 🤝 **Schedule coordination meeting** (Accumulate + Accman teams)
- 📋 **Answer critical questions** (see Questions section)
- 🚀 **Target: Production-ready in 3 weeks**

---

**Status**: Ready for implementation pending coordination on:
1. Port configuration approach
2. Snapshot distribution details
3. MCP communication protocol

**Next Step**: Schedule technical coordination meeting between teams.
