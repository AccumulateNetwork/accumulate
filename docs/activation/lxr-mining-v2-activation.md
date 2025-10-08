# LXR Mining V2 Activation Procedure

## Overview

This document describes the process for activating the LXR memory-hard proof-of-work mining feature on the Accumulate network. The feature is guarded by the **ExecutorVersionV2LXRMining** launch site and requires network-wide coordination for activation.

## Launch Site Details

- **Name**: `ExecutorVersionV2LXRMining`
- **Value**: `9`
- **Label**: `v2-lxr-mining`
- **Description**: Enables LXR memory-hard proof-of-work mining for anti-spam protection

## Prerequisites

### Technical Requirements

1. **Node Software**:
   - All nodes must be running Accumulate v2.x or later
   - Nodes must have `ExecutorVersionV2LXRMining` support compiled in
   - Verify with: `accumulated version --verbose`

2. **Network Consensus**:
   - Majority (>66%) of validators must be ready for upgrade
   - Coordination through governance process
   - Announcement window: minimum 7 days before activation

3. **Testing Requirements**:
   - Feature must pass all tests on TestNet
   - Performance benchmarks validated
   - Security audit completed
   - No critical issues outstanding

### Governance Requirements

1. **Proposal Submission**:
   - Submit activation proposal to governance
   - Include rationale, timeline, and rollback plan
   - Minimum 7-day discussion period

2. **Voting**:
   - Requires >66% validator approval
   - Voting period: 7 days
   - Emergency veto: network council

## Activation Steps

### Phase 1: Preparation (T-7 days)

1. **Network Announcement**:
   ```bash
   # Post activation notice to all communication channels
   - Discord announcement
   - Twitter/X announcement
   - Email to validator operators
   - Update network status page
   ```

2. **Validator Coordination**:
   - Confirm all validators have upgraded
   - Schedule coordination call
   - Share activation timeline
   - Test activation on TestNet

3. **Monitoring Setup**:
   - Deploy metrics collection
   - Configure alerting thresholds
   - Prepare rollback scripts
   - Test emergency procedures

### Phase 2: Activation (T-Day)

1. **Pre-Activation Checks** (T-0 minus 1 hour):
   ```bash
   # Verify network health
   accumulated network status

   # Check validator readiness
   accumulated query validator-status --all

   # Verify current executor version
   accumulated query globals | grep ExecutorVersion
   ```

2. **Initiate Activation** (T-0):

   The network activation is performed through the `ActivateProtocolVersion` transaction:

   ```bash
   # Create activation transaction (Directory network only)
   accumulated tx create \
     --type activate-protocol-version \
     --version 9 \
     --wait

   # Or via API
   curl -X POST https://api.accumulate.network/v3 \
     -d '{
       "jsonrpc": "2.0",
       "id": 1,
       "method": "submit",
       "params": {
         "envelope": {
           "transaction": [{
             "header": {
               "principal": "dn.acme"
             },
             "body": {
               "type": "activateProtocolVersion",
               "version": 9
             }
           }],
           "signatures": [...]
         }
       }
     }'
   ```

3. **Monitor Activation** (T+0 to T+24 hours):
   ```bash
   # Watch for activation propagation
   watch 'accumulated query globals | grep ExecutorVersion'

   # Monitor for errors
   tail -f /var/log/accumulated/accumulated.log | grep -i "lxr\|mining\|version"

   # Check partition synchronization
   accumulated query partition-status --all
   ```

### Phase 3: Verification (T+24 to T+72 hours)

1. **Feature Verification**:
   ```bash
   # Test CreateMiningAuthority transaction
   accumulated tx create \
     --type create-mining-authority \
     --url "test.acme/mining" \
     --difficulty 100 \
     --table-size 20 \
     --table-seed 12345 \
     --passes 6 \
     --wait

   # Test LXR mining signature
   # (Will be rejected on networks < V2LXRMining)
   accumulated tx submit-with-mining \
     --url "test.acme" \
     --difficulty 100
   ```

2. **Performance Monitoring**:
   - Transaction throughput (should be stable)
   - Block production rate (should be unchanged)
   - Memory usage (monitor for LXR table caching)
   - Network latency (should be normal)

3. **Issue Tracking**:
   - Monitor issue tracker for reports
   - Triage any activation-related issues
   - Communicate status updates

## Guarded Code Paths

The following operations are blocked until ExecutorVersionV2LXRMining is active:

### Signature Validation
**File**: `internal/core/execute/v2/block/sig_user.go`
```go
// LXR Mining signatures are conditionally registered
registerConditionalExec[UserSignature](&signatureExecutors,
    func(ctx *SignatureContext) bool {
        return ctx.GetActiveGlobals().ExecutorVersion.V2LXRMiningEnabled()
    },
    protocol.SignatureTypeLXRMining,
)
```

**Behavior**:
- Before activation: LXR mining signatures rejected with "unsupported signature type"
- After activation: LXR mining signatures processed normally

### Transaction Execution
**File**: `internal/core/execute/v2/chain/create_mining_authority.go`
```go
func (x CreateMiningAuthority) Validate(st *StateManager, tx *Delivery) ... {
    if !st.Globals.ExecutorVersion.V2LXRMiningEnabled() {
        return nil, errors.NotAllowed.With(
            "LXR mining has not been activated on this network")
    }
    ...
}
```

**Behavior**:
- Before activation: CreateMiningAuthority transactions fail validation
- After activation: CreateMiningAuthority transactions process normally

## Rollback Procedure

### Emergency Rollback

If critical issues are discovered within 48 hours of activation:

1. **Assess Impact**:
   - Determine severity (P0: immediate rollback, P1: monitor, P2: fix forward)
   - Count affected transactions
   - Evaluate network stability

2. **Initiate Rollback** (P0 only):
   ```bash
   # This requires emergency consensus override
   # Contact network council immediately

   # Revert to previous executor version
   accumulated tx create \
     --type activate-protocol-version \
     --version 8 \
     --emergency \
     --wait
   ```

3. **Post-Rollback**:
   - Investigate root cause
   - Fix issues
   - Reschedule activation
   - Post-mortem report

### Forward Fix

For non-critical issues (P1/P2):

1. Deploy hotfix to next minor version
2. Schedule patch activation
3. Communicate issue and resolution

## Monitoring and Observability

### Key Metrics

1. **Version Status**:
   ```bash
   # Check current executor version on all partitions
   accumulated query globals --partition dn | grep ExecutorVersion
   accumulated query globals --partition BVN0 | grep ExecutorVersion
   # ... repeat for all BVNs
   ```

2. **Mining Activity**:
   ```bash
   # Count LXR mining signatures
   accumulated query signatures \
     --type lxr-mining \
     --since "2025-01-01" \
     --count

   # Monitor mining authority accounts
   accumulated query account "*.acme/mining" --type mining-authority
   ```

3. **Performance Metrics**:
   - Average block time
   - Transaction success rate
   - Signature validation time
   - Memory usage (LXR cache)

### Alerting Thresholds

Configure alerts for:
- Executor version mismatch across partitions (critical)
- Transaction failure rate > 5% (warning)
- Block production delay > 2x normal (warning)
- Memory usage > 80% (warning)
- LXR mining signature failures > 10% (investigate)

## Communication Plan

### Timeline

- **T-7 days**: Initial announcement
- **T-3 days**: Validator coordination call
- **T-1 day**: Final readiness check
- **T-0**: Activation
- **T+1 hour**: Initial status update
- **T+24 hours**: Full verification report
- **T+72 hours**: Final activation report

### Channels

1. **Official Announcements**:
   - Discord #announcements
   - Twitter/X @AccumulateNetwork
   - Blog post
   - Email to validator list

2. **Technical Updates**:
   - Discord #validators
   - GitHub Discussions
   - Status page

3. **Emergency Communication**:
   - Discord #network-emergencies (private)
   - Validator emergency contact list
   - PagerDuty/on-call rotation

## Success Criteria

Activation is considered successful when:

1. ✅ All partitions report ExecutorVersionV2LXRMining
2. ✅ CreateMiningAuthority transactions succeed
3. ✅ LXR mining signatures validate correctly
4. ✅ No critical issues reported within 72 hours
5. ✅ Network performance metrics within normal ranges
6. ✅ 100% partition synchronization maintained

## References

- [LXR Mining Specification](../specs/32-lxr-mining.md)
- [LXR Mining Milestone](../LXR_MINING_MILESTONE.md)
- [GitLab Issue #3665](https://gitlab.com/accumulatenetwork/accumulate/-/issues/3665)
- [Executor Version Management](../protocol/version.md)

## Appendix: Testing Checklist

Before production activation, verify on TestNet:

- [ ] CreateMiningAuthority transaction succeeds after activation
- [ ] CreateMiningAuthority transaction fails before activation
- [ ] LXR mining signatures validate after activation
- [ ] LXR mining signatures rejected before activation
- [ ] Version guard tests pass
- [ ] Network upgrades cleanly without transaction loss
- [ ] Rollback procedure tested successfully
- [ ] Performance benchmarks meet requirements
- [ ] Security audit findings addressed
- [ ] Documentation complete and accurate

---

**Document Version**: 1.0
**Last Updated**: October 7, 2025
**Author**: Accumulate Network Team
**Reviewers**: [To be added during review process]
