# AIP → Issue → Branch Mapping

**Complete Work Inventory**: Accumulate Improvement Proposals to Implementation

---

## AIP-53: Mining System

### Status: BLOCKED (5+ months)

### Issues Related
- **#3885** (URGENT) - AIP-53 Mining project status - 7 branches dormant 5 months
- **#3684** - ARM64 crypto implementation (related)
- **18 mining-labeled issues** total

### Branches (7)
| Branch | Commits | Last Activity | Status |
|--------|---------|---------------|--------|
| fix/dennis-mining-correct-crypto | ? | 2026-03-26 | Needs review |
| 3640-mining-support | ? | 2025-03-19 | Dormant 5+ mo |
| 3665-lxr-mining-clean | ? | 2025-08-29 | Stale |
| 3680-lxr-mining-baseline-clean | ? | 2025-08-29 | Stale |
| issue-3871-key-rotation | ? | 2025-03-25 | Related |
| +2 more | ? | 2025-2026 | ? |

### Required Action
- [ ] Review all 7 branches
- [ ] Decide: Continue, Pause, or Cancel
- [ ] If Continue: Consolidate into single PR
- [ ] If Cancel: Close branches and document decision

### Timeline Estimate
- **Decision**: 1-2 hours
- **Implementation** (if continue): 2-4 weeks
- **Archive** (if cancel): 2 hours

---

## AIP-54: Unknown (Mentioned in Issue #3886)

### Status: UNDEFINED

### Issues Related
- **#3886** - Merge 4 active 2026 branches ready for integration
- References AIP-54 but no clear definition

### Branches
- (Unknown - needs investigation)

### Required Action
- [ ] Find AIP-54 specification
- [ ] Define scope
- [ ] Link to appropriate branches
- [ ] Create issue if missing

### Timeline Estimate
- **Investigation**: 1-2 hours
- **Definition**: TBD based on scope

---

## Other AIPs Mentioned in Issues

### AIP-006
- Found in: `grep -r "AIP-006"` results
- Status: Unclear
- Issues: (Unknown)

### AIP-010
- Found in: `grep -r "AIP-010"` results
- Status: Unclear
- Issues: (Unknown)

---

## Major Issue → Branch Mappings

### Critical/Urgent Issues

#### #3885: AIP-53 Mining (URGENT)
- **Type**: Epic
- **Branches**: 7 (see above)
- **Status**: BLOCKED
- **Action**: Decision required

#### #3884: ARM64 Crypto (CRITICAL)
- **Type**: Bug
- **Branches**: (Unknown - needs triage)
- **Status**: New/triaging
- **Action**: Assess scope and fix

#### #3890: DAG-BFT Production (HIGH)
- **Type**: Feature
- **Branch**: dagbft-integration (40+ commits)
- **Status**: Tested at 7.3K TPS, needs stability test
- **Action**: Run 24-hour stability test

#### #3889: DAG-BFT Transaction Bug (HIGH)
- **Type**: Bug
- **Branches**: dagbft-integration
- **Status**: Needs fix
- **Action**: Reproduce and isolate bug

---

## Feature Branches with Real Work (50+ commits)

### Cryptographic Proof System

#### #3658: Cryptographic Proof API
- **Branch**: 3658-cryptographic-proof-api
- **Commits**: 95
- **Last Activity**: 2025-08-26
- **Status**: Ready for merge assessment
- **Action**: [ ] Create PR or merge

#### #3660: Activate Collection Proofs
- **Branch**: 3660-activate-collection-proofs
- **Commits**: 85
- **Last Activity**: 2025-08-18
- **Status**: Ready for merge assessment
- **Action**: [ ] Create PR or merge

#### #3664: Cryptographic Proof in Lite Client
- **Branch**: 3664-api-support-cryptographic-proof-system-in-lite-client
- **Commits**: 102
- **Last Activity**: 2025-08-27
- **Status**: Ready for merge assessment
- **Action**: [ ] Create PR or merge

---

### Release/Maintenance

#### #3702: Release 1.4.4-beta.3
- **Branch**: 3702-release-1.4.4-beta.3
- **Commits**: 91
- **Last Activity**: 2026-03-11
- **Status**: Beta release branch
- **Action**: [ ] Merge or supersede

#### #3706: Reduce Genesis Memory Usage
- **Branch**: 3706-reduce-genesis-memory-usage
- **Commits**: 73
- **Last Activity**: 2025-12-29
- **Status**: Optimization
- **Action**: [ ] Create PR or merge

---

### Enhancement/Features

#### #3705: Transaction Whitelist on Keypage
- **Branch**: 3705-add-transaction-whitelist-to-keypage
- **Commits**: 98
- **Last Activity**: 2026-01-09
- **Status**: Feature ready
- **Action**: [ ] Create PR or merge

#### #3713: Add Version Commands
- **Branch**: 3713-add-version-commands
- **Commits**: 88
- **Last Activity**: 2026-01-11
- **Status**: Enhancement ready
- **Action**: [ ] Create PR or merge

#### #3714: SDK Signature Documentation
- **Branch**: 3714-sdk-signature-docs
- **Commits**: 86
- **Last Activity**: 2026-02-28
- **Status**: Documentation
- **Action**: [ ] Create PR or merge

---

## Issue-by-Issue Branch Tracking

### Issues 3824-3868 (Integrated into main)
All merged or in-progress:
- #3824: Concurrent map fix ✅
- #3860: TestState fix ✅
- #3866: Snapshot fix ✅
- #3868: Test/validate fix ✅
- #3843: URL parsing ✅
- #3844: Performance monitoring ✅
- #3845: Dashboard ✅
- ... and more

### Issues 3869-3880 (Consensus/Security)
Status: Mostly dated 2026-03-25, awaiting decision
- #3869: Vote spam fix (dagbft-integration)
- #3870: Vote verification CPU fix
- #3871: Key rotation
- #3872: Timestamp replay protection
- #3873: LRU eviction locking
- #3875: Per-peer vote rate limiting
- #3876: Race conditions
- #3877: Replace math/rand
- #3878: Lock copying violations
- #3879: Request body size limits
- #3880: Certificate verification fallback

### Issues 3888+ (Recent Optimizations)
- #3888: BPT parallel updates (dagbft-integration + issue-3888-bpt-parallel-updates)
- #3889: DAG-BFT transaction bug
- #3890: DAG-BFT production readiness
- #3891: Production deployment config
- #3892: 10K TPS infrastructure & testing

---

## Dead/Stale Branches (Archive Candidates)

### 4000+ Commits (should archive)
| Branch | Commits | Last Activity | Reason |
|--------|---------|---------------|--------|
| 3652-create-a-genesis-block | 4000+ | 2025-08-29 | Dead |
| 3653-crosschainconductor | 4000+ | 2025-09-01 | Dead |
| 3661-sdk-connection | 4393 | 2025-08-18 | Dead |
| 3662-ccc-docs | 4297 | 2025-08-18 | Dead |

### 5+ Months Dormant (reassess)
| Branch | Last Activity | Reason |
|--------|---------------|--------|
| 3640-mining-support | 2025-03-19 | Blocked by AIP-53 |
| 3665-lxr-mining-clean | 2025-08-29 | Blocked by AIP-53 |
| 3680-lxr-mining-baseline | 2025-08-29 | Blocked by AIP-53 |

---

## Integration Strategy

### Phase 1: Cleanup (This Week)
1. Delete 4 dead branches (4000+ commits)
2. Archive 7 mining branches (pending AIP-53 decision)
3. Review [gone] branches (already pruned)

### Phase 2: Consolidation (2 Weeks)
1. Integrate 8 feature branches (3658-3714)
2. Merge consensus/security fixes to appropriate branch
3. Resolve conflicts

### Phase 3: Planning (3-4 Weeks)
1. DAG-BFT stability testing
2. Mine work (if AIP-53 continues)
3. Roadmap creation

### Phase 4: Production (1-3 Months)
1. Merge clean up work
2. Deploy v1.0.0-critical-fixes to followers
3. Plan DAG-BFT network upgrade

---

## Summary Statistics

| Category | Count | Status |
|----------|-------|--------|
| **AIPs with clear scope** | 0 | Need definition |
| **AIPs mentioned** | 4 | AIP-6, 10, 53, 54 |
| **High-priority AIPs** | 1 | AIP-53 (BLOCKED) |
| **Issues mapped** | 100 | Needs triage |
| **Branches with 50+ commits** | 8 | Ready to merge |
| **Branches 5+ months old** | 7 | Reassess/archive |
| **Dead branches** | 4 | Archive immediately |
| **Feature branches** | 20+ | Active/merged |

---

## Recommendations

### 1. Define Missing AIPs
- [ ] Create specifications for AIP-6, AIP-10, AIP-54
- [ ] Link to requirements and issues
- [ ] Estimate effort and timeline

### 2. Clear AIP-53 Decision
- [ ] Schedule meeting this week
- [ ] Options: Continue, Pause, Cancel
- [ ] Consolidate or archive branches

### 3. Integrate Feature Work
- [ ] Merge 8 branches with real work (3658-3714)
- [ ] Resolve conflicts
- [ ] Test merged code

### 4. Stabilize DAG-BFT
- [ ] Run 24-hour stability test
- [ ] Fix identified bugs (#3889)
- [ ] Document upgrade procedure

### 5. Create Process
- [ ] One issue → one branch policy
- [ ] Branch naming convention
- [ ] Automated cleanup of merged branches

