# CrossChain Conductor Documentation

[← Back to Design Index](../INDEX.md) | [← Back to Main Index](../../INDEX.md)

## Overview
The CrossChain Conductor (CCC) manages cross-partition message delivery in Accumulate, ensuring reliable and ordered message delivery even in the presence of network partitions.

## Core Design Documents

### Gap Recovery Mechanism
- [Gap Recovery Design](GAP_RECOVERY_DESIGN.md) - Original design for gap recovery
- [Gap Recovery Actual Implementation](GAP_RECOVERY_ACTUAL.md) - As-implemented documentation
- [Improved Gap Recovery](IMPROVED_GAP_RECOVERY.md) - Enhancement proposals
- [Gap Request Design](GAP_REQUEST_DESIGN.md) - Gap detection and request protocol

### Recovery Flows
- [Recovery Flows](RECOVERY_FLOWS.md) - Complete recovery workflow documentation
- [Test Summary](TEST_SUMMARY.md) - Test coverage and validation

### Implementation Details
- [CCC Review](CCC_REVIEW.md) - Code review and analysis
- [Refactoring Summary](REFACTORING_SUMMARY.md) - Code refactoring documentation

## Implementation Files

### Core Components
- [`conductor.go`](../../../internal/core/execute/v2/crosschain/conductor.go) - Main conductor implementation
- [`conductor_inbound.go`](../../../internal/core/execute/v2/crosschain/conductor_inbound.go) - Inbound message handling
- [`conductor_outbound.go`](../../../internal/core/execute/v2/crosschain/conductor_outbound.go) - Outbound message handling

### Gap Recovery Implementation
- [`conductor_gap_recovery.go`](../../../internal/core/execute/v2/crosschain/conductor_gap_recovery.go) - Gap recovery logic
- [`destination_state.go`](../../../internal/core/execute/v2/crosschain/destination_state.go) - Per-destination state tracking

### Recovery Components
- [`conductor_recovery.go`](../../../internal/core/execute/v2/crosschain/conductor_recovery.go) - Recovery mechanisms
- [`sequence_tracker.go`](../../../internal/core/execute/v2/crosschain/sequence_tracker.go) - Sequence number tracking

### Testing Infrastructure
- [`conductor_pause.go`](../../../internal/core/execute/v2/crosschain/conductor_pause.go) - Pause/resume for testing (testnet build only)
- [`conductor_http.go`](../../../internal/core/execute/v2/crosschain/conductor_http.go) - HTTP debug endpoints

### Test Files
- [`test_gap_recovery_test.go`](../../../internal/core/execute/v2/crosschain/test_gap_recovery_test.go) - Gap recovery tests
- [`test_collection_proof_test.go`](../../../internal/core/execute/v2/crosschain/test_collection_proof_test.go) - Collection proof tests
- [`test_recovery_flows_test.go`](../../../internal/core/execute/v2/crosschain/test_recovery_flows_test.go) - Recovery flow tests

## Key Concepts

### 1. Simple Index-Based Gap Recovery
The CCC uses a simple yet effective approach:
- **SentTxIndex**: Tracks last successfully sent sequence number
- **CurrentTxIndex**: Tracks latest available sequence number
- **Gap Detection**: Automatic detection when destination reports missing sequences
- **Recovery**: Simple index reset and batch resend

See [Gap Recovery Actual](GAP_RECOVERY_ACTUAL.md) for details.

### 2. Collection Proofs
Efficient batching of multiple messages with a single proof:
- Reduces network overhead
- Improves recovery performance
- Maintains message ordering

See [Collection Proof Implementation](../PHASE1_COLLECTION_PROOF_FIX.md).

### 3. State Tracking
Per-destination state management:
```go
type DestinationSendState struct {
    Destination    *url.URL
    SentTxIndex    uint64  // Last successfully sent
    CurrentTxIndex uint64  // Latest available
    Messages       map[uint64]messaging.Message
}
```
See [`destination_state.go`](../../../internal/core/execute/v2/crosschain/destination_state.go).

## Testing

### Test Scripts
- [`gap_recovery_demo.sh`](../../../scripts/devnet/gap_recovery_demo.sh) - Gap recovery demonstration
- [`gap_test.sh`](../../../scripts/devnet/gap_test.sh) - Automated gap testing
- [`interactive_pause_test.sh`](../../../scripts/devnet/interactive_pause_test.sh) - Interactive partition testing

### Test Documentation
- [Gap Testing README](../../testing/load/GAP_TESTING_README.md)
- [Gap Test Results](../../testing/load/GAP_TEST_RESULTS.md)

## Configuration

### Build Tags
- `testnet` - Enables pause/resume functionality for testing

### Environment Variables
- `ACC_CCC_BATCH_SIZE` - Maximum messages per batch (default: 100)
- `ACC_CCC_TIMEOUT` - Send timeout in seconds (default: 30)

## Metrics and Monitoring

### Debug Endpoints (testnet build only)
- `GET /debug/ccc/status` - CCC status
- `GET /debug/ccc/metrics` - Performance metrics
- `POST /debug/ccc/pause` - Pause message delivery
- `POST /debug/ccc/resume` - Resume message delivery

### Key Metrics
- `sent_tx_index` - Last sent sequence per destination
- `current_tx_index` - Latest sequence per destination
- `gap_size` - Current gap size
- `total_sent` - Total messages sent
- `total_failed` - Total send failures
- `gap_resets` - Number of gap recovery resets

## Related Documentation

### Design Documents
- [Parent CCC Design](../CCC_DESIGN_SUMMARY.md) - Overall CCC architecture
- [Implementation Plan](../CCC_IMPLEMENTATION_PLAN.md) - Implementation roadmap
- [Validation Design](../CCC_VALIDATION_DESIGN.md) - Message validation

### Testing
- [DevNet Testing Guide](../../testing/devnet/DEVNET_TESTING_GUIDE.md) - DevNet setup for testing
- [Load Testing](../../testing/load/INDEX.md) - Performance testing

### Deployment
- [Deployment Guides](../../deployment/INDEX.md) - Production deployment