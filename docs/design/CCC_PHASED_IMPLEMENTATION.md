# CCC Phased Implementation Plan

## Overview
Break the CCC implementation into manageable phases, each building on the previous. Start with getting collection proofs working as the foundation.

---

## Phase 1: Get Collection Proofs Working (2-3 weeks)
**Goal:** Fix the non-functional collection proof implementation to enable efficient batch processing

### Critical Fix Required
- **File:** `internal/core/execute/v2/crosschain/proof_service.go`
- **Line 303:** Currently passes `nil` for merkle state
- **Fix:** Pass actual merkle state from the Chain object

### Tasks
1. **Fix merkle state access in ProofService** (3 days)
   ```go
   // Current (BROKEN):
   receiptList, err := merkle.GetReceiptList(nil, startIdx, endIdx)
   
   // Fixed:
   receiptList, err := merkle.GetReceiptList(req.SourceChain.GetMerkleState(), startIdx, endIdx)
   ```

2. **Add Chain method for merkle state access** (2 days)
   - Modify `database.Chain` interface to expose merkle state
   - Ensure thread-safe access to underlying merkle tree
   - Add proper error handling

3. **Test collection proof generation** (3 days)
   - Unit tests for single and batch proofs
   - Verify merkle proof validation
   - Test with various batch sizes (1, 10, 100, 1000 txs)
   - Compare proof sizes: individual vs collection

4. **Integration testing** (2 days)
   - Test with actual chain data
   - Verify proofs validate correctly
   - Performance benchmarking

5. **Add historical state support** (3 days)
   - Ability to create proofs from earlier chain states
   - Required for healing/catch-up scenarios
   - Test with different historical points

### Success Criteria
- [ ] Collection proofs generate successfully
- [ ] Proofs validate correctly
- [ ] 90%+ reduction in proof overhead for batches
- [ ] Tests pass for various batch sizes
- [ ] Historical state proofs work

### Dependencies
- Database team: Help with merkle state access
- No external blockers

### Risks
- **Low:** Well-understood problem with clear solution

---

## Phase 2: Chain Indexing by Destination/Type (3-4 weeks)
**Goal:** Enable efficient access to transaction chains for CCC routing

### Tasks
1. **Design index structure** (3 days)
   - Key: "destination/type" (e.g., "BVN1/synthetic")
   - Value: Chain of sequence numbers
   - Consider storage backend (BadgerDB vs separate index)

2. **Implement destination indexing** (5 days)
   - Hook into transaction production
   - Update index on new transactions
   - Handle partition ID changes

3. **Add query interfaces** (3 days)
   - Get transactions by destination/type
   - Get sequence ranges
   - Support pagination for large sets

4. **Implement source tracking for destinations** (3 days)
   - Track what sequences received from each source
   - Enable gap detection
   - Support healing queries

5. **Testing and optimization** (5 days)
   - Performance testing with many partitions
   - Concurrent access testing
   - Index rebuild capability

### Success Criteria
- [ ] Can query all transactions for a destination/type in <100ms
- [ ] Index updates don't slow transaction processing
- [ ] Supports 100+ partitions efficiently
- [ ] Gap detection works correctly

### Dependencies
- Phase 1 complete (need working proofs for testing)

### Risks
- **Medium:** Performance at scale needs validation

---

## Phase 3: CCC API Integration (2-3 weeks)
**Goal:** Route anchor/synthetic transactions through CCC at API layer

### Tasks
1. **Modify Submitter to detect transaction types** (2 days)
   ```go
   func (s *Submitter) Submit(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) {
       if isAnchorOrSynthetic(envelope) {
           return s.ccc.ProcessInbound(ctx, envelope, opts)
       }
       // Regular path...
   }
   ```

2. **Implement CCC inbound validation** (3 days)
   - Sequence number checking
   - Reject out-of-sequence messages
   - Trigger healing for gaps

3. **Add CCC to API initialization** (2 days)
   - Wire up CCC in node startup
   - Configure connection to database
   - Add configuration options

4. **Update error responses** (2 days)
   - Clear messages for sequence errors
   - Distinguish temporary vs permanent failures
   - Add retry guidance

5. **Testing** (3 days)
   - Test routing logic
   - Verify regular transactions unaffected
   - Test sequence validation
   - Load testing

### Success Criteria
- [ ] Anchor/synthetic transactions route through CCC
- [ ] Out-of-sequence messages rejected
- [ ] Regular transactions unaffected
- [ ] <10ms additional latency

### Dependencies
- Phase 2 complete (need chain indexing)

### Risks
- **Low:** Clean interfaces make integration straightforward

---

## Phase 4: Healing with Collection Proofs (3-4 weeks)
**Goal:** Implement automated recovery for missing transactions

### Tasks
1. **Gap detection mechanism** (3 days)
   - Detect missing sequences on rejection
   - Track gaps per source/type
   - Prioritize critical gaps

2. **Query missing transaction sets** (3 days)
   - Request specific sequence ranges
   - Use collection proofs for efficiency
   - Handle large gaps with batching

3. **Healing envelope processing** (4 days)
   - Create separate envelope type
   - Process asynchronously from normal flow
   - Submit to CometBFT when complete

4. **Recovery orchestration** (4 days)
   - Coordinate multiple healing requests
   - Prevent duplicate requests
   - Handle partial failures

5. **Testing and monitoring** (5 days)
   - Test with artificial gaps
   - Test with network partitions
   - Add metrics and alerting
   - Performance testing

### Success Criteria
- [ ] Automatic gap detection and healing
- [ ] Healing at 10,000+ TPS
- [ ] No manual intervention required
- [ ] Healing doesn't impact normal operations

### Dependencies
- Phases 1-3 complete

### Risks
- **Medium:** Complex coordination, potential for healing storms

---

## Phase 5: Production Hardening (4-5 weeks)
**Goal:** Make CCC production-ready with monitoring, reliability, and performance

### Tasks
1. **Comprehensive monitoring** (1 week)
   - Metrics dashboard
   - Alerting rules
   - Performance tracking
   - SLO definition

2. **Reliability improvements** (1 week)
   - Circuit breakers
   - Timeout handling
   - Graceful degradation
   - Partition tolerance

3. **Performance optimization** (1 week)
   - Connection pooling
   - Caching strategies
   - Batch optimization
   - Load balancing

4. **Operational tooling** (1 week)
   - Debugging tools
   - Manual intervention capabilities
   - State inspection
   - Recovery procedures

5. **Documentation and training** (1 week)
   - Operator documentation
   - Troubleshooting guide
   - Architecture documentation
   - Team training

### Success Criteria
- [ ] 99.9% uptime SLO
- [ ] <100ms p99 latency
- [ ] 1000+ TPS sustained
- [ ] Full monitoring coverage
- [ ] Team trained on operations

### Dependencies
- All previous phases complete

### Risks
- **Low:** Standard production hardening tasks

---

## Phase 6: Advanced Features (Optional, 3-4 weeks)
**Goal:** Add nice-to-have features once core is stable

### Potential Features
- Priority queuing for critical transactions
- Advanced load balancing strategies
- Cross-partition query optimization
- Predictive healing (pre-fetch likely needed transactions)
- Multi-version proof support
- Compression optimizations

---

## Timeline Summary

| Phase | Duration | Cumulative | Critical Path |
|-------|----------|------------|---------------|
| Phase 1: Collection Proofs | 2-3 weeks | 2-3 weeks | ✅ Yes |
| Phase 2: Chain Indexing | 3-4 weeks | 5-7 weeks | ✅ Yes |
| Phase 3: API Integration | 2-3 weeks | 7-10 weeks | ✅ Yes |
| Phase 4: Healing | 3-4 weeks | 10-14 weeks | ✅ Yes |
| Phase 5: Production | 4-5 weeks | 14-19 weeks | ✅ Yes |
| Phase 6: Advanced | 3-4 weeks | 17-23 weeks | ❌ No |

**Total: 14-19 weeks (3.5-5 months) for core functionality**

## Immediate Next Steps

1. **Week 1:** Fix collection proof merkle state issue
2. **Week 2:** Test and validate collection proofs
3. **Week 3:** Begin chain indexing design
4. **Decision Point:** Validate approach before proceeding

## Risk Mitigation

- **Start with Phase 1:** Highest value, lowest risk
- **Validate early:** Each phase has clear success criteria
- **Incremental deployment:** Test on testnet after each phase
- **Parallel work:** Some Phase 2 design can start during Phase 1
- **Regular checkpoints:** Weekly progress reviews

## Resource Requirements

- **Team Size:** 2-3 senior engineers
- **Dedicated:** Full-time commitment required
- **Support:** Database team consultation needed
- **Testing:** QA support for later phases
- **DevOps:** Support for Phase 5 production hardening