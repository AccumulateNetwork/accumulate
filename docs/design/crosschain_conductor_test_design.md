# CrossChain Conductor - Test Design

**Feature**: CrossChain Conductor  
**Created**: 2025-08-30

## Test Design Principles

### Test Scenarios
Each test validates specific interface interactions and behaviors based on our 3 interfaces:
- `ChainProvider` - Access chains and track "top of chain" indices
- `MessageSender` - Send list proofs to other partitions  
- `MessageReceiver` - Receive messages and handle gap healing

### Mock Strategy
All external dependencies will be mocked in `*_test.go` files only. Production code will use real implementations.

## Test Categories

### 1. ChainProvider Interface Tests

#### TestGetAnchorChain
- **Setup**: Mock ChainProvider with anchor chain
- **Action**: Call `GetAnchorChain(ctx)`
- **Expected**: Returns anchor chain interface
- **Mock Interactions**: ChainProvider.GetAnchorChain() called once
- **Assertions**: Returned chain is not nil, correct type

#### TestGetSyntheticChain  
- **Setup**: Mock ChainProvider with synthetic chain
- **Action**: Call `GetSyntheticChain(ctx)`
- **Expected**: Returns synthetic chain interface
- **Mock Interactions**: ChainProvider.GetSyntheticChain() called once
- **Assertions**: Returned chain is not nil, correct type

#### TestGetTopOfChainIndex
- **Setup**: Mock ChainProvider returns index 42 for anchor chain
- **Action**: Call `GetTopOfChainIndex(ctx, ChainTypeAnchor)`
- **Expected**: Returns 42
- **Mock Interactions**: ChainProvider.GetTopOfChainIndex(ChainTypeAnchor) → 42
- **Assertions**: Returned index equals 42, no error

#### TestSetTopOfChainIndex
- **Setup**: Mock ChainProvider accepts index update
- **Action**: Call `SetTopOfChainIndex(ctx, ChainTypeAnchor, 100)`
- **Expected**: Index updated successfully
- **Mock Interactions**: ChainProvider.SetTopOfChainIndex(ChainTypeAnchor, 100) → nil
- **Assertions**: No error returned

### 2. Chain Interface Tests

#### TestCollectTransactionsFrom
- **Setup**: Mock Chain with transactions at indices 10, 11, 12
- **Action**: Call `CollectTransactionsFrom(10)`
- **Expected**: Returns 3 transactions
- **Mock Interactions**: Chain.CollectTransactionsFrom(10) → [tx10, tx11, tx12]
- **Assertions**: 3 transactions returned, correct sequence numbers

#### TestCollectTransactionsFromEmpty
- **Setup**: Mock Chain with no new transactions  
- **Action**: Call `CollectTransactionsFrom(50)` when chain top is 45
- **Expected**: Returns empty list
- **Mock Interactions**: Chain.CollectTransactionsFrom(50) → []
- **Assertions**: Empty slice returned, no error

#### TestGetCurrentTop
- **Setup**: Mock Chain with current top at 99
- **Action**: Call `GetCurrentTop()`
- **Expected**: Returns 99
- **Mock Interactions**: Chain.GetCurrentTop() → 99
- **Assertions**: Returned value equals 99, no error

### 3. MessageSender Interface Tests

#### TestSendListProof
- **Setup**: Mock MessageSender, valid list proof
- **Action**: Call `SendListProof(ctx, "partition1", proof)`
- **Expected**: Proof sent successfully
- **Mock Interactions**: MessageSender.SendListProof("partition1", proof) → nil  
- **Assertions**: No error returned

#### TestSendListProofFailure
- **Setup**: Mock MessageSender returns network error
- **Action**: Call `SendListProof(ctx, "unreachable", proof)`
- **Expected**: Error returned
- **Mock Interactions**: MessageSender.SendListProof("unreachable", proof) → error
- **Assertions**: Error returned, specific error type

### 4. MessageReceiver Interface Tests

#### TestReceiveMessage
- **Setup**: Mock MessageReceiver with last received sequence 25
- **Action**: Call `ReceiveMessage(ctx, "source1", envelope)`  
- **Expected**: Returns sequence number 25
- **Mock Interactions**: MessageReceiver.ReceiveMessage("source1", envelope) → 25, nil
- **Assertions**: Returned sequence equals 25, no error

#### TestRequestMissingMessages
- **Setup**: Mock MessageReceiver accepts gap healing request
- **Action**: Call `RequestMissingMessages(ctx, "source1", 20)`
- **Expected**: Request sent successfully
- **Mock Interactions**: MessageReceiver.RequestMissingMessages("source1", 20) → nil
- **Assertions**: No error returned

### 5. Integration Tests

#### TestCompleteListProofWorkflow
- **Setup**: All mocks configured for full workflow
- **Test Flow**:
  1. Get anchor chain from ChainProvider
  2. Get current top of chain index (45)
  3. Collect transactions from index 40 to 45
  4. Generate list proof from collected transactions
  5. Send list proof via MessageSender
  6. Update top of chain index to 45
- **Mock Interactions**:
  - ChainProvider.GetAnchorChain() → mockChain
  - ChainProvider.GetTopOfChainIndex(ChainTypeAnchor) → 40
  - mockChain.GetCurrentTop() → 45  
  - mockChain.CollectTransactionsFrom(40) → [tx40, tx41, tx42, tx43, tx44, tx45]
  - MessageSender.SendListProof("dest", proof) → nil
  - ChainProvider.SetTopOfChainIndex(ChainTypeAnchor, 45) → nil
- **Assertions**: All steps complete without error, correct call sequence

#### TestGapHealingWorkflow  
- **Setup**: All mocks configured for gap healing
- **Test Flow**:
  1. Receive out-of-order message via MessageReceiver
  2. MessageReceiver returns last received sequence (10)
  3. CCC detects gap (received seq 15, last was 10)
  4. Request missing messages from sequence 11
- **Mock Interactions**:
  - MessageReceiver.ReceiveMessage("source", envelope) → 10, nil (last received)
  - MessageReceiver.RequestMissingMessages("source", 11) → nil
- **Assertions**: Gap detected correctly, missing messages requested

## Mock Implementations

### MockChainProvider
```go
type MockChainProvider struct {
    mock.Mock
}
func (m *MockChainProvider) GetAnchorChain(ctx context.Context) (Chain, error)
func (m *MockChainProvider) GetSyntheticChain(ctx context.Context) (Chain, error)  
func (m *MockChainProvider) GetTopOfChainIndex(ctx context.Context, chainType ChainType) (uint64, error)
func (m *MockChainProvider) SetTopOfChainIndex(ctx context.Context, chainType ChainType, index uint64) error
```

### MockChain
```go
type MockChain struct {
    mock.Mock
}
func (m *MockChain) CollectTransactionsFrom(startSeq uint64) ([]Transaction, error)
func (m *MockChain) GetCurrentTop() (uint64, error)
```

### MockMessageSender
```go  
type MockMessageSender struct {
    mock.Mock
}
func (m *MockMessageSender) SendListProof(ctx context.Context, destination string, proof *protocol.AnnotatedReceipt) error
```

### MockMessageReceiver
```go
type MockMessageReceiver struct {
    mock.Mock  
}
func (m *MockMessageReceiver) ReceiveMessage(ctx context.Context, source string, message *messaging.Envelope) (uint64, error)
func (m *MockMessageReceiver) RequestMissingMessages(ctx context.Context, source string, fromSequence uint64) error
```

## Test Data Requirements

### Chain Data
- Sample chains with transactions at known sequence numbers
- Empty chains for edge case testing
- Chains with gaps in sequence numbers

### List Proof Data
- Valid list proofs for validation testing
- Invalid/corrupted list proofs for rejection testing
- Empty list proofs for edge cases

### Message Data
- Sample message envelopes
- Out-of-order message sequences
- Gap healing request/response pairs