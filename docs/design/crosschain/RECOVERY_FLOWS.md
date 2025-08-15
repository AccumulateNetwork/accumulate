# CrossChain Conductor Recovery Flows

## Overview

The CrossChain Conductor (CCC) implements several recovery mechanisms to handle missing messages and network disruptions in cross-partition communication.

## Recovery Flow Types

### 1. Gap Detection and Recovery

**Purpose**: Detect and recover missing messages in a sequence.

**Flow**:
1. **Detection**: When a message arrives out of sequence (e.g., receiving message #5 when expecting #3)
2. **Gap Identification**: CCC identifies the gap range (messages #3 and #4 are missing)
3. **Recovery Request**: Sends a RecoveryRequest to the source partition
4. **Response Processing**: Source partition sends the missing messages
5. **Gap Filling**: Missing messages are inserted and processed in order

**Key Components**:
- `SimpleSequenceTracker`: Detects gaps in message sequences
- `RecoveryManager`: Coordinates recovery requests and responses
- `RequestMissingMessages()`: Initiates recovery for a gap

### 2. Batch Recovery with Collection Proofs

**Purpose**: Efficiently recover multiple messages using collection proofs.

**Flow**:
1. **Batch Detection**: Multiple missing messages identified (e.g., messages #10-#50)
2. **Collection Request**: Request all missing messages as a batch
3. **Collection Proof Creation**: Source creates a single collection proof for all messages
4. **Batch Transmission**: All messages sent with one proof instead of 40 individual proofs
5. **Batch Validation**: Collection proof validates all messages at once

**Key Components**:
- `BatchProofRecoveryManager`: Manages batch recovery operations
- `CollectionProof`: Contains proof for multiple messages
- `ProofService`: Creates and validates collection proofs

### 3. Proactive Health Monitoring

**Purpose**: Detect potential issues before they cause failures.

**Flow**:
1. **Periodic Check**: Every 30 seconds, check partition health
2. **Sequence Analysis**: Analyze sequence numbers for each partition
3. **Gap Detection**: Identify any missing sequences
4. **Proactive Recovery**: Request missing messages before they're needed
5. **Health Reporting**: Update partition health status

**Key Components**:
- `periodicHealthCheck()`: Runs health checks
- `checkPartitionHealth()`: Analyzes partition status
- `GetHealthStatus()`: Returns current health metrics

### 4. Recovery Session Management

**Purpose**: Track and manage ongoing recovery operations.

**Flow**:
1. **Session Creation**: Create a recovery session for a gap
2. **Request Tracking**: Track which messages are being recovered
3. **Timeout Management**: Retry if recovery takes too long
4. **Session Completion**: Mark session complete when all messages received
5. **Cleanup**: Remove completed sessions after timeout

**Key Components**:
- `RecoverySession`: Tracks a single recovery operation
- `processRecoveryRequests()`: Handles incoming recovery requests
- `CleanupStaleSessions()`: Removes old sessions

## Recovery Request Format

```go
type RecoveryRequest struct {
    SourcePartition      *url.URL  // Requesting partition
    DestinationPartition *url.URL  // Partition with missing messages
    MessageType          string    // "synthetic" or "anchor"
    LastKnownSequence    uint64    // Last sequence received
    MissingSequences     []uint64  // Specific missing sequences
    RequestTime          time.Time // When request was made
}
```

## Recovery Response Format

```go
type RecoveryResponse struct {
    Success          bool
    RecoveredCount   int
    Messages         []messaging.Message
    CollectionProof  *CollectionProof  // Optional: for batch recovery
    Error            string
}
```

## Error Handling

### Timeout Handling
- Recovery requests timeout after 30 seconds
- Automatic retry up to 3 times
- Exponential backoff between retries

### Network Partition Handling
- Detect when partition is unreachable
- Queue recovery requests until partition returns
- Batch queued requests for efficiency

### Message Validation
- Validate recovered messages match requested sequences
- Verify proofs for recovered messages
- Reject invalid or tampered messages

## Performance Optimizations

### Collection Proofs
- Batch multiple messages with single proof
- Reduces proof overhead by up to 95% for large batches
- Automatic batching for sequential messages

### Caching
- Cache recently recovered messages
- Avoid duplicate recovery requests
- Share recovered messages across waiting goroutines

### Prioritization
- Prioritize recovery of older messages first
- Critical messages (anchors) get priority
- Load-based throttling to prevent overload

## Monitoring and Metrics

### Key Metrics
- `recovery_requests_total`: Total recovery requests sent
- `recovery_success_rate`: Percentage of successful recoveries
- `recovery_latency_ms`: Time to complete recovery
- `gaps_detected_total`: Total sequence gaps detected
- `collection_proofs_used`: Number of collection proofs created

### Health Indicators
- Partition connectivity status
- Average gap size
- Recovery queue depth
- Session timeout rate

## Testing Scenarios

### Unit Tests
1. **Gap Detection**: Test sequence gap identification
2. **Recovery Request**: Test request generation and sending
3. **Message Validation**: Test recovered message validation
4. **Collection Proof**: Test batch recovery with collection proofs

### Integration Tests
1. **Network Partition**: Simulate partition disconnect/reconnect
2. **Message Loss**: Simulate random message drops
3. **High Load**: Test recovery under heavy message load
4. **Concurrent Recovery**: Multiple simultaneous recovery sessions

### Failure Scenarios
1. **Recovery Timeout**: Source doesn't respond to recovery request
2. **Invalid Response**: Source sends wrong messages
3. **Proof Failure**: Collection proof validation fails
4. **Cascade Failure**: Recovery causes more gaps

## Best Practices

1. **Early Detection**: Detect gaps as soon as possible
2. **Batch When Possible**: Use collection proofs for multiple messages
3. **Monitor Health**: Track partition health continuously
4. **Clean Sessions**: Remove stale recovery sessions
5. **Validate Everything**: Always validate recovered messages and proofs
6. **Log Important Events**: Log recovery requests, responses, and failures
7. **Metrics First**: Track all recovery operations with metrics