# 🚀 Accumulate Protocol v1.5.0-experimental Released!

## Release Information
- **Version:** v1.5.0-experimental
- **Branch:** 3653-add-a-crosschainconductor-process-for-coordinating-partitions
- **Tag:** v1.5.0-experimental
- **Status:** Successfully pushed to GitLab

## GitLab Links
- **Branch:** https://gitlab.com/accumulatenetwork/accumulate/-/tree/3653-add-a-crosschainconductor-process-for-coordinating-partitions
- **Tag:** https://gitlab.com/accumulatenetwork/accumulate/-/tags/v1.5.0-experimental
- **Create MR:** https://gitlab.com/accumulatenetwork/accumulate/-/merge_requests/new?merge_request%5Bsource_branch%5D=3653-add-a-crosschainconductor-process-for-coordinating-partitions

## What's New

### CrossChainConductor System
A complete rewrite of cross-partition transaction handling with:
- **13.2x Performance Improvement** through collection proofs
- **95% Memory Reduction** for large transaction batches
- **100% Success Rate** in extended load testing
- **Automatic Failure Recovery** with circuit breaker pattern

### Key Components
1. **CrossChainConductor** - Orchestrates all cross-partition messaging
2. **ProofService** - Centralized proof construction and validation (NO CACHING)
3. **BatchProofRecoveryManager** - Efficient recovery using collection proofs
4. **Visual Monitoring** - Real-time partition lag visualization

### ⚠️ Breaking Changes
This is an **EXPERIMENTAL** release with breaking changes:
- New architecture requires all nodes to upgrade
- Proof format changes for collection proofs
- Not backward compatible with v1.4.x
- Client libraries may need updates

## Commits Included (8 commits)
```
0bc0efac9 docs: Add release notes and script for v1.5.0-experimental
6da21481f chore: Repository cleanup and documentation organization
482b1d24e feat: Add centralized ProofService with collection proof optimization
4f904a845 feat: Implement CrossChainConductor with partition failure handling
1f132dbe4 Remove old CrosschainCoordinator files and update testing
01289e4b2 Implement CrossChainConductor with comprehensive load testing
e49578634 Add faucet helper infrastructure for sustained testing
c0f3c6e7d Add faucet collection load test for multi-partition testing
```

## Performance Metrics

### Collection Proof Optimization
- **Small batches (5 txs):** 2.9x speedup
- **Medium batches (25 txs):** 10x speedup
- **Large batches (100 txs):** 17.9x speedup
- **Massive batches (500 txs):** 25x speedup

### Load Testing Results
- **Extended Test:** 100 requests @ 46.87 req/sec (100% success)
- **Sustained Test:** 2,135 requests over 2 minutes (0 failures)
- **Memory Usage:** 361KB → 17KB for 500 transactions

## Next Steps

### For Testing
1. Deploy to test network
2. Run integration tests with multiple validators
3. Test client library compatibility
4. Monitor performance metrics

### For Production
1. Wait for merge request approval
2. Complete code review process
3. Test on staging environment
4. Plan coordinated upgrade (all nodes must upgrade together)

## Documentation
- [Release Notes](RELEASE_NOTES_v1.5.0.md)
- [Design Document](test/load/PROOF_CENTRALIZATION_DESIGN_NO_CACHE.md)
- [Run Instructions](test/load/RUN_INSTRUCTIONS.md)
- [Visual Monitoring](test/load/HOW_TO_RUN_VISUAL_TESTS.md)

## Support
For issues or questions about this experimental release:
- GitLab Issues: https://gitlab.com/accumulatenetwork/accumulate/issues
- Tag issues with: `v1.5.0-experimental`, `crosschain-conductor`, `breaking-change`

---

**Remember:** This is an experimental release. Thoroughly test in non-production environments before considering for production use.