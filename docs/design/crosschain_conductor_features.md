# CrossChain Conductor - Feature Specification

**Issue**: #3653  
**Created**: 2025-08-30

## Feature Requirements

### Core Features
- [ ] **Top of Chain Tracking**: Track "top of chain" index (sequence number) of last transaction sent
- [ ] **List Collection from Chain**: Collect transaction list from chain starting at "top of chain" sequence number
- [ ] **List Proof Generation**: Build list proof from collected transactions to top of chain
- [ ] **List Proof Transmission**: Create and send list proof containing transaction batch
- [ ] **Collection Proof Validation**: Validate inbound collection proofs for authenticity
- [ ] **Gap Healing via Sequence Numbers**: Request missing messages by returning last received sequence
- [ ] **Automatic Retry**: Failed sends retried automatically if sequence unchanged

### NO QUEUEING Requirements
- [ ] **No Message Queuing**: Out-of-order messages trigger immediate gap healing requests
- [ ] **Bounded Memory Usage**: Memory usage stays constant (no growing queues)
- [ ] **Sequence Number Reset**: Sender resets to last received sequence automatically

### Integration Requirements
- [ ] **Executor Integration**: Hook into message production in `block/synthetic.go`
- [ ] **API Integration**: Hook into message submission in `api/v3/tm/submitter.go`
- [ ] **Database Integration**: Track sequence numbers in database
- [ ] **Fallback Behavior**: Direct submission when CCC disabled/failed

### Performance Requirements
- [ ] **Network Efficiency**: Reduce invalid message overhead from O(n²) to O(1)
- [ ] **Proof Efficiency**: 90%+ reduction in proof data size for batches
- [ ] **Memory Efficiency**: No unbounded memory growth
- [ ] **CPU Efficiency**: <2x generation time for collection vs individual proofs

### Security Requirements  
- [ ] **Efficiency Only**: CCC provides efficiency optimization, NOT security
- [ ] **Consensus Validation**: All security guarantees come from consensus re-validation
- [ ] **Graceful Degradation**: System remains secure even with compromised CCC nodes