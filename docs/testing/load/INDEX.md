# Load Testing Documentation Index

[← Back to Testing Index](../INDEX.md) | [← Back to Main Index](../../INDEX.md)

## Overview
Comprehensive load testing documentation for Accumulate, including performance testing, stress testing, and gap recovery testing.

## Quick Start
- [README](README.md) - Load testing overview
- [Complete Documentation](README_COMPLETE.md) - Detailed guide
- [Run Instructions](RUN_INSTRUCTIONS.md) - How to run load tests

## Test Guides

### Gap Recovery Testing
- [Gap Testing README](GAP_TESTING_README.md) - Gap recovery test guide
- [Gap Test Results](GAP_TEST_RESULTS.md) - Test results and analysis
- Related Scripts:
  - [`gap_recovery_demo.sh`](../../../scripts/devnet/gap_recovery_demo.sh)
  - [`gap_test.sh`](../../../scripts/devnet/gap_test.sh)
  - [`test_gap_recovery.sh`](../../../scripts/devnet/test_gap_recovery.sh)

### Partition Testing
- [Partition Failure Design](PARTITION_FAILURE_DESIGN.md) - Network partition testing
- [`partition_manager.sh`](../../../scripts/devnet/partition_manager.sh) - Partition management
- [`test_partition_control.sh`](../../../scripts/devnet/test_partition_control.sh) - Partition control

### Visual Testing
- [How to Run Visual Tests](HOW_TO_RUN_VISUAL_TESTS.md) - Visual monitoring guide
- [Dashboard Usage](DASHBOARD_USAGE.md) - Dashboard documentation
- [`run_visual_monitor.sh`](../../../scripts/devnet/run_visual_monitor.sh) - Visual monitor script

## CrossChain Conductor Testing

### Design Documentation
- [CrossChain Conductor Design](CrossChainConductor_Design_Document.md) - CCC design
- [CrossChain Conductor Code Reference](CrossChainConductor_Code_Reference.md) - Code guide
- Related Code: [`internal/core/execute/v2/crosschain/`](../../../internal/core/execute/v2/crosschain/)

### Proof Testing
- [Proof Centralization Design](PROOF_CENTRALIZATION_DESIGN.md) - Proof system design
- [Proof Centralization No Cache](PROOF_CENTRALIZATION_DESIGN_NO_CACHE.md) - Non-cached approach
- [Collection Proofs Review](CODE_REVIEW_COLLECTION_PROOFS.md) - Code review

## Test Scripts

### Load Test Runners
Located in [`scripts/devnet/`](../../../scripts/devnet/):
- [`load_test_runner.sh`](../../../scripts/devnet/load_test_runner.sh) - Main test runner
- [`devnet_load_test.sh`](../../../scripts/devnet/devnet_load_test.sh) - DevNet load test
- [`devnet_load_test_enhanced.sh`](../../../scripts/devnet/devnet_load_test_enhanced.sh) - Enhanced version
- [`comprehensive_load_test.sh`](../../../scripts/devnet/comprehensive_load_test.sh) - Full test suite

### Interactive Testing
- [`interactive_load_test.sh`](../../../scripts/devnet/interactive_load_test.sh) - Interactive load testing
- [`interactive_pause_test.sh`](../../../scripts/devnet/interactive_pause_test.sh) - Pause/resume testing

### Test Suites
- [`run_full_test_suite.sh`](../../../scripts/devnet/run_full_test_suite.sh) - Complete suite
- [`run_complete_test_suite.sh`](../../../scripts/devnet/run_complete_test_suite.sh) - All tests
- [`quick_test.sh`](../../../scripts/devnet/quick_test.sh) - Quick validation

### Specialized Tests
- [`run_crosschain_test.sh`](../../../scripts/devnet/run_crosschain_test.sh) - CrossChain testing
- [`run_faucet_test.sh`](../../../scripts/devnet/run_faucet_test.sh) - Faucet testing
- [`test_synthetic_deposits.sh`](../../../scripts/devnet/test_synthetic_deposits.sh) - Synthetic testing

## Test Reports

### Summary Reports
- [Final Test Report](FINAL_TEST_REPORT.md) - Comprehensive test results
- [Final Review Summary](FINAL_REVIEW_SUMMARY.md) - Test review summary
- [Debugging Summary](DEBUGGING_SUMMARY.md) - Debug findings

### Code Reviews
- [Code Review Findings](CODE_REVIEW_FINDINGS.md) - Review results
- [Changes Since Last Commit](CHANGES_SINCE_LAST_COMMIT.md) - Recent changes

## Configuration

### DevNet Configuration
- See [DevNet Configuration](../devnet/DEVNET_CONFIGURATION.md)
- [`devnet_config.sh`](../../../scripts/devnet/devnet_config.sh) - Configuration script
- [`devnet_manager.sh`](../../../scripts/devnet/devnet_manager.sh) - DevNet management

### Test Parameters
- Number of transactions
- Concurrent workers
- Transaction types
- Network topology

## Performance Metrics

### Key Metrics
- Transactions per second (TPS)
- Latency (p50, p95, p99)
- Success rate
- Resource usage

### Monitoring
- Real-time dashboards
- Log aggregation
- Metric collection
- Alert thresholds

## Test Scenarios

### 1. Basic Load Test
```bash
./scripts/devnet/devnet_load_test.sh
```

### 2. Gap Recovery Test
```bash
./scripts/devnet/gap_recovery_demo.sh
```

### 3. Partition Failure Test
```bash
./scripts/devnet/partition_manager.sh create
./scripts/devnet/test_partition_control.sh
```

### 4. Comprehensive Test
```bash
./scripts/devnet/run_full_test_suite.sh
```

## Development

### AI Assistance
- [AI Assistant Guide](AI_ASSISTANT_GUIDE.md) - Using AI for test development

### API Improvements
- [API Improvements TODO](API_Improvements_TODO.md) - Planned improvements

### Project Documentation
- [Complete Project Documentation](COMPLETE_PROJECT_DOCUMENTATION.md) - Full project docs
- [Repository Cleanup Plan](REPOSITORY_CLEANUP_PLAN.md) - Maintenance plan

## Troubleshooting

### V3 Connection Issues
- [V3 Connection Fixes](v3_connection_fixes.md) - Connection troubleshooting
- [Apply V3 Fixes](APPLY_V3_FIXES.md) - Fix application guide

### Common Issues
1. **Connection Timeouts**
   - Check network configuration
   - Verify port availability
   - Review firewall rules

2. **Performance Degradation**
   - Monitor resource usage
   - Check for memory leaks
   - Review log files

3. **Test Failures**
   - Verify DevNet status
   - Check test parameters
   - Review error logs

## Best Practices

### Test Design
1. Start with minimal load
2. Gradually increase complexity
3. Monitor system resources
4. Document anomalies
5. Repeat tests for consistency

### Test Execution
1. Clean environment before tests
2. Warm up the system
3. Run multiple iterations
4. Collect comprehensive metrics
5. Analyze results thoroughly

## Related Documentation

- [DevNet Documentation](../devnet/INDEX.md) - DevNet setup and configuration
- [CrossChain Design](../../design/crosschain/INDEX.md) - CCC architecture
- [Performance Testing](../performance-tests.md) - Performance guidelines
- [Testing Main Index](../INDEX.md) - All testing documentation