# Implementation Clarity Assessment
## Snapshot Restore MCP Tool

**Date**: 2025-11-18
**Status**: Ready for Implementation
**Documents Reviewed**:
- `SNAPSHOT_RESTORE_DEPLOYMENT.md` (accman repo)
- `accman_snapshot_restore_review.md` (MCP repo)

---

## Executive Summary

**Overall Readiness**: ✅ **CLEAR - Ready to implement**

The task is well-defined with clear requirements, scope, and implementation approach. Key decisions have been made based on accman feedback. The simplified scope (manual snapshots, no automation) makes this achievable in a short timeframe.

**Recommended Timeline**: 1-2 weeks for MCP tool implementation + testing

---

## What is CLEAR ✅

### 1. Core Functionality

**Crystal Clear**:
- ✅ Tool name: `accumulate_restore_from_snapshots`
- ✅ Input: Two snapshot files (DN + BVN), work directory, ports
- ✅ Output: Node directories ready for Docker deployment
- ✅ Process: Restore each partition separately, generate dual-node config

**Well-Documented**:
- Exact restore process (5 steps documented)
- Config format for restore vs runtime
- Port configuration methods (offset vs explicit)
- Directory structure requirements

**Examples Provided**:
- Accman integration examples (52xxx ports)
- Simple deployment examples (16xxx ports)
- Docker deployment commands
- Error handling scenarios

### 2. Port Configuration

**Fully Specified**:
- ✅ Support both `port_offset` (simple) and `ports` (explicit)
- ✅ `ports` overrides `port_offset` if both provided
- ✅ Default ports: DN=16591-16593, BVN=16691-16693
- ✅ Accman convention: DN=52000-52002, BVN=52100-52102

**Clear Priority**:
- Explicit `ports` is recommended for accman
- Port offset for simple use cases
- No ambiguity about precedence

### 3. Scope Boundaries

**Explicitly Excluded**:
- ❌ Automated snapshot download (future)
- ❌ Snapshot repository/CDN (future)
- ❌ Snapshot verification/signing (future)
- ❌ Incremental snapshots (future)

**Explicitly Included**:
- ✅ Manual snapshot creation (export-snapshot tool exists)
- ✅ Restore from user-provided snapshot files
- ✅ Port configuration flexibility
- ✅ Dual-node configuration generation

**No Feature Creep**: Scope is locked and minimal

### 4. Integration Pattern

**Unambiguous**:
- Accman calls Accumulate MCP via HTTP/stdio
- Accumulate MCP returns node directory paths
- Accman handles Docker deployment
- Clean separation of concerns maintained

**Interface Clear**:
- Request format documented (JSON)
- Response format documented (JSON)
- Example calls provided for both tools

### 5. Testing Strategy

**Testable Criteria**:
- Snapshot files exist and are readable
- Node directories created with correct structure
- Config files generated correctly
- Ports configured as specified
- Database restored successfully

**Success Metrics**:
- Follower starts at snapshot block height (~10.6M)
- Syncs forward from that point
- API endpoints responsive
- Total deployment time < 10 minutes

---

## What is UNCLEAR ⚠️

### 1. MCP Communication Protocol (MINOR)

**Question**: How does accman call the Accumulate MCP?

**Options**:
- HTTP client (what host/port?)
- Stdio (direct subprocess?)
- Embedded library?

**Impact**: Low - just needs decision on deployment pattern

**Recommendation**:
- Start with HTTP on configurable port (default 3000)
- Document in accman integration guide
- Can add stdio support later if needed

**Resolution Needed**: Document MCP server endpoint configuration

---

### 2. Error Handling Details (MINOR)

**Unclear Scenarios**:
- What if DN snapshot is from block 10.6M but BVN is from 10.5M?
- What if snapshot file is corrupted?
- What if work_dir already contains data?
- What if restore-snapshot command fails mid-way?

**Impact**: Low - these are implementation details

**Recommendation**:
- Validate snapshot files exist before starting
- Check work_dir is empty or prompt to overwrite
- Return detailed error messages
- No partial state (all-or-nothing restore)

**Resolution**: Handle during implementation, add to troubleshooting docs

---

### 3. Config File Ownership (MINOR)

**Question**: After restore, who modifies accumulate.toml?

**Scenarios**:
- MCP tool generates final config with ports → accman uses as-is
- OR: MCP tool generates base config → accman modifies for ports

**Current Assumption**: MCP generates complete config

**Impact**: Low - just affects who has authority over config

**Recommendation**:
- MCP owns protocol-level config (storage, network, partition)
- MCP applies port configuration from user params
- Accman does NOT modify accumulate.toml
- Accman only handles Docker port mapping

**Resolution**: Confirm this pattern in implementation

---

### 4. Snapshot Compatibility (LOW)

**Unclear**:
- Can DN and BVN snapshots be from different block heights?
- Maximum acceptable delta between partitions?
- What happens if snapshots are too old?

**Impact**: Low - network will sync forward anyway

**Current Behavior**:
- Restore whatever user provides
- Let node sync from there
- No validation of compatibility

**Recommendation**:
- Document "best practice" (snapshots from same time)
- Don't enforce strict validation initially
- Can add warnings if delta > 1000 blocks (future enhancement)

**Resolution**: Document as best practice, don't block on it

---

### 5. Tendermint Config (MEDIUM - Needs Resolution)

**Question**: What about config/tendermint.toml?

**From Documentation**: Restore process creates both `accumulate.toml` and `tendermint.toml`

**Unclear**:
- Does restore-snapshot generate tendermint.toml?
- Or do we copy from somewhere?
- What should be in it?
- Does it need port configuration too?

**Impact**: Medium - affects whether restore works

**Current Evidence**:
- Existing followers have tendermint.toml with:
  - Consensus parameters
  - P2P settings
  - Storage settings

**Investigation Needed**:
1. Check if `restore-snapshot` generates tendermint.toml
2. If not, determine what default/template to use
3. Check if tendermint ports need configuration

**Resolution**: Research `restore-snapshot` behavior, document findings

---

### 6. Network-Specific Differences (LOW)

**Question**: Are there MainNet vs TestNet differences?

**Assumptions**:
- Bootstrap peers differ (already documented)
- Network name in config differs
- Everything else same?

**Impact**: Low - mostly configuration data

**Recommendation**:
- Support network parameter (MainNet, TestNet)
- Look up default bootstrap peers by network
- Allow override via params

**Resolution**: Implement network-aware defaults

---

## Implementation Blockers

### CRITICAL (Must resolve before starting)

**None identified** ✅

All critical decisions made. Scope is clear. Requirements are documented.

### HIGH PRIORITY (Should resolve during implementation)

1. **Tendermint Config Handling**
   - Research how restore-snapshot handles config/tendermint.toml
   - Document what needs to be created/copied
   - Est. Time: 1-2 hours investigation

### MEDIUM PRIORITY (Can resolve during testing)

2. **MCP Communication Protocol**
   - Document HTTP endpoint (host:port)
   - Create accman HTTP client example
   - Est. Time: 1 hour documentation

3. **Error Handling Strategy**
   - Define validation checks
   - Write error messages
   - Est. Time: 2-4 hours during implementation

### LOW PRIORITY (Nice to have)

4. **Snapshot Compatibility Validation**
   - Add warnings for large block deltas
   - Future enhancement

5. **Network-Specific Defaults**
   - TestNet bootstrap peers
   - Future enhancement

---

## Recommended Implementation Plan

### Phase 1: Core MCP Tool (Week 1)

**Day 1-2: Research & Setup**
- ✅ Documentation done
- 🔍 Research restore-snapshot tendermint.toml handling
- 🔍 Test restore-snapshot with current snapshots
- 📝 Document findings

**Day 3-4: Implementation**
- Implement MCP tool in `/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/mcp/server/tools_snapshot_restore.go`
- Add tool definition to `tool_definitions.go`
- Implement port configuration logic (both methods)
- Generate accumulate.toml with correct ports

**Day 5: Testing**
- Test with DN + BVN snapshots
- Test port offset mode
- Test explicit ports mode
- Verify node directories created correctly
- Test with Docker deployment

### Phase 2: Accman Integration (Week 2)

**Day 1-2: MCP Communication**
- Document MCP HTTP endpoint
- Test accman calling accumulate MCP
- Verify response format

**Day 3-4: Accman Tool**
- Implement `deploy_follower_from_snapshots` in accman
- HTTP client to call accumulate MCP
- Docker deployment with returned paths
- Port mapping to host

**Day 5: End-to-End Testing**
- Full workflow: snapshots → MCP → accman → Docker
- Multiple followers with different ports
- Verify followers start at correct block height
- Performance testing (time to deploy)

---

## Risk Assessment

### Technical Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| restore-snapshot doesn't handle ports | Low | Medium | We control the config generation after restore |
| Tendermint config missing | Medium | Medium | Research during Phase 1 Day 1 |
| Port conflicts in Docker | Low | Low | Well-documented, testable |
| Snapshot incompatibility | Low | Low | Network syncs forward anyway |

### Schedule Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Tendermint config complexity | Medium | +1 day | Build in buffer time |
| MCP communication issues | Low | +0.5 day | Standard HTTP, well-documented |
| Accman integration bugs | Medium | +1 day | Iterative testing |

**Overall Risk**: LOW - Well-scoped, clear requirements, limited unknowns

---

## Decision Log

### Decisions Made ✅

1. **Port Configuration**: Support both offset and explicit (explicit wins)
2. **Scope**: Manual snapshots only, no automated distribution
3. **Integration**: Accman calls MCP via HTTP, MCP returns paths
4. **Separation**: MCP owns protocol, accman owns Docker
5. **Config Ownership**: MCP generates complete accumulate.toml

### Decisions Deferred 🔄

1. **Snapshot Repository**: Future enhancement, not blocking
2. **Verification**: Future enhancement, users verify manually
3. **Incremental Snapshots**: Future optimization

### Decisions Needed ⏳

1. **MCP HTTP Endpoint**: Default host:port for MCP server
2. **Tendermint Config**: How to handle config/tendermint.toml
3. **Error Messages**: Standard format for failure cases

---

## Recommendations

### For Implementation Team

1. ✅ **Start Implementation** - Requirements are clear enough
2. 🔍 **Investigate Tendermint Config First** - Only unknown with medium impact
3. 📝 **Document MCP Endpoint** - Quick decision, low impact
4. 🧪 **Test Early, Test Often** - Use existing snapshots from validator backup
5. 📊 **Track Metrics** - Deployment time, success rate, error types

### For Product/Planning

1. ✅ **Approve Simplified Scope** - Manual snapshots is pragmatic
2. 📋 **Future Roadmap** - Automated distribution can come later
3. 🎯 **Success Criteria** - <10 min deployment time, >95% success rate
4. 📣 **User Communication** - Set expectations about manual snapshot process

---

## Conclusion

### Readiness Score: 9/10

**Strengths**:
- ✅ Requirements crystal clear
- ✅ Scope well-defined and locked
- ✅ Examples and documentation complete
- ✅ Integration pattern decided
- ✅ Success criteria defined

**Gaps (minor)**:
- ⚠️ Tendermint config handling (research needed)
- ⚠️ MCP endpoint documentation (quick decision)
- ⚠️ Error message standards (implementation detail)

**Recommendation**: **PROCEED WITH IMPLEMENTATION**

The task is sufficiently clear to begin coding. Remaining unknowns are minor and can be resolved during Phase 1 Day 1 research. No blockers identified.

---

**Next Actions**:
1. Assign to developer
2. Research tendermint.toml handling (1-2 hours)
3. Document MCP HTTP endpoint
4. Begin implementation Phase 1

**Status**: ✅ **Ready for Development**
