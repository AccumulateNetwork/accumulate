# Test Specification: CrossChain Healing Recovery System

**Created**: 2025-09-01  
**Feature Design**: crosschain_healing_recovery_complete.md  
**Package**: internal/core/execute/v2/crosschain

## Test Plan Overview

### Testing Approach
- Unit tests for all recovery methods with mocked dependencies
- Integration tests for cross-component interactions
- End-to-end tests for complete recovery workflows
- Performance tests for large gap scenarios

### Coverage Target
- Minimum 80% code coverage for recovery components
- 100% coverage for critical paths (gap detection, message transmission)

## Test Suites

### Unit Tests

#### TestRecoveryManager_ProvideRecoveredTransactions
```go
func TestRecoveryManager_ProvideRecoveredTransactions(t *testing.T) {
    tests := []struct {
        name           string
        destination    *url.URL
        recovered      []*RecoveredTransaction
        transportError error
        wantErr        bool
        wantMetrics    map[string]float64
    }{
        {
            name:        "successful recovery response",
            destination: parseURL("acc://partition1"),
            recovered: []*RecoveredTransaction{
                {Sequence: 100, Hash: []byte("hash1"), Data: []byte("data1")},
                {Sequence: 101, Hash: []byte("hash2"), Data: []byte("data2")},
            },
            wantErr: false,
            wantMetrics: map[string]float64{
                "recovery_responses_sent": 1,
                "transactions_recovered":  2,
            },
        },
        {
            name:        "empty recovery list",
            destination: parseURL("acc://partition1"),
            recovered:   []*RecoveredTransaction{},
            wantErr:     false,
        },
        {
            name:           "transport error",
            destination:    parseURL("acc://partition1"),
            recovered:      []*RecoveredTransaction{{Sequence: 100}},
            transportError: errors.New("network error"),
            wantErr:        true,
            wantMetrics: map[string]float64{
                "recovery_response_errors": 1,
            },
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup mocks
            mockTransport := &MockRecoveryTransport{}
            mockMetrics := &MockMetrics{}
            
            rm := &RecoveryManager{
                transport: mockTransport,
                metrics:   mockMetrics,
                logger:    testLogger,
            }
            
            // Setup expectations
            if tt.transportError != nil {
                mockTransport.On("SendRecoveryResponse", mock.Anything).Return(tt.transportError)
            } else {
                mockTransport.On("SendRecoveryResponse", mock.Anything).Return(nil)
            }
            
            // Execute
            err := rm.ProvideRecoveredTransactions(tt.destination, tt.recovered)
            
            // Assert
            if (err != nil) != tt.wantErr {
                t.Errorf("ProvideRecoveredTransactions() error = %v, wantErr %v", err, tt.wantErr)
            }
            
            // Verify metrics
            for metric, expectedValue := range tt.wantMetrics {
                assert.Equal(t, expectedValue, mockMetrics.GetValue(metric))
            }
            
            mockTransport.AssertExpectations(t)
        })
    }
}
```

#### TestRecoveryManager_RecoverAnchors
```go
func TestRecoveryManager_RecoverAnchors(t *testing.T) {
    tests := []struct {
        name            string
        request         *RecoveryRequest
        dbAnchors       map[uint64]*protocol.AnchorTransaction
        dbErrors        map[uint64]error
        wantRecovered   int
        wantErr         bool
    }{
        {
            name: "successful anchor recovery",
            request: &RecoveryRequest{
                Source:           parseURL("acc://source-partition"),
                MissingSequences: []uint64{100, 101, 102},
            },
            dbAnchors: map[uint64]*protocol.AnchorTransaction{
                100: createTestAnchor(100, "hash100"),
                101: createTestAnchor(101, "hash101"), 
                102: createTestAnchor(102, "hash102"),
            },
            wantRecovered: 3,
            wantErr:       false,
        },
        {
            name: "partial recovery - some anchors missing",
            request: &RecoveryRequest{
                Source:           parseURL("acc://source-partition"),
                MissingSequences: []uint64{100, 101, 102},
            },
            dbAnchors: map[uint64]*protocol.AnchorTransaction{
                100: createTestAnchor(100, "hash100"),
                102: createTestAnchor(102, "hash102"),
            },
            wantRecovered: 2,
            wantErr:       false,
        },
        {
            name: "database error for some sequences",
            request: &RecoveryRequest{
                Source:           parseURL("acc://source-partition"),
                MissingSequences: []uint64{100, 101},
            },
            dbErrors: map[uint64]error{
                100: errors.New("database error"),
            },
            dbAnchors: map[uint64]*protocol.AnchorTransaction{
                101: createTestAnchor(101, "hash101"),
            },
            wantRecovered: 1,
            wantErr:       false,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup mocks
            mockDB := &MockDatabase{}
            mockMetrics := &MockMetrics{}
            
            rm := &RecoveryManager{
                database: mockDB,
                metrics:  mockMetrics,
                logger:   testLogger,
            }
            
            // Setup database expectations
            for _, seq := range tt.request.MissingSequences {
                if err, hasError := tt.dbErrors[seq]; hasError {
                    mockDB.On("GetAnchorBySequence", tt.request.Source, seq).Return(nil, err)
                } else if anchor, hasAnchor := tt.dbAnchors[seq]; hasAnchor {
                    mockDB.On("GetAnchorBySequence", tt.request.Source, seq).Return(anchor, nil)
                } else {
                    mockDB.On("GetAnchorBySequence", tt.request.Source, seq).Return(nil, nil)
                }
            }
            
            // Execute
            recovered, err := rm.RecoverAnchors(tt.request)
            
            // Assert
            if (err != nil) != tt.wantErr {
                t.Errorf("RecoverAnchors() error = %v, wantErr %v", err, tt.wantErr)
            }
            
            assert.Len(t, recovered, tt.wantRecovered)
            
            // Verify recovered transactions have correct data
            for _, tx := range recovered {
                assert.NotEmpty(t, tx.Hash)
                assert.NotEmpty(t, tx.Data)
                assert.Equal(t, TransactionTypeAnchor, tx.Type)
                assert.Contains(t, tt.request.MissingSequences, tx.Sequence)
            }
            
            mockDB.AssertExpectations(t)
        })
    }
}
```

#### TestProofService_CreateCollectionProof
```go
func TestProofService_CreateCollectionProof(t *testing.T) {
    tests := []struct {
        name           string
        transactions   []*RecoveredTransaction
        chainError     error
        wantProofSize  int
        wantErr        bool
    }{
        {
            name: "successful collection proof creation",
            transactions: []*RecoveredTransaction{
                {Hash: []byte("hash1"), Data: []byte("data1")},
                {Hash: []byte("hash2"), Data: []byte("data2")},
                {Hash: []byte("hash3"), Data: []byte("data3")},
            },
            wantProofSize: 3,
            wantErr:       false,
        },
        {
            name:         "empty transaction list",
            transactions: []*RecoveredTransaction{},
            wantErr:      true,
        },
        {
            name: "chain access error",
            transactions: []*RecoveredTransaction{
                {Hash: []byte("hash1"), Data: []byte("data1")},
            },
            chainError: errors.New("chain not accessible"),
            wantErr:    true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup mocks
            mockChain := &MockChain{}
            mockMerkle := &MockMerkleManager{}
            
            ps := &ProofService{
                chains: map[string]*MockChain{
                    "test-partition": mockChain,
                },
            }
            
            if tt.chainError != nil {
                mockChain.On("Inner").Return(nil, tt.chainError)
            } else {
                mockChain.On("Inner").Return(mockMerkle, nil)
                mockMerkle.On("GetReceiptList", mock.Anything).Return(&merkle.ReceiptList{
                    Entries: make([]*merkle.Receipt, len(tt.transactions)),
                }, nil)
            }
            
            // Execute
            proof, err := ps.buildCollectionProof(tt.transactions)
            
            // Assert
            if (err != nil) != tt.wantErr {
                t.Errorf("buildCollectionProof() error = %v, wantErr %v", err, tt.wantErr)
            }
            
            if !tt.wantErr {
                assert.NotNil(t, proof)
                assert.Equal(t, tt.wantProofSize, len(proof.Transactions))
            }
        })
    }
}
```

### Integration Tests

#### TestConductor_AutomaticGapHealing
```go
func TestConductor_AutomaticGapHealing(t *testing.T) {
    // This is a complex integration test that verifies the complete flow
    // from gap detection to recovery request transmission
    
    // Setup test environment
    testDB := setupTestDatabase(t)
    defer testDB.Close()
    
    mockTransport := &MockTransport{}
    mockMetrics := &MockMetrics{}
    
    conductor := NewConductor(&ConductorConfig{
        Database:  testDB,
        Transport: mockTransport,
        Metrics:   mockMetrics,
        Logger:    testLogger,
    })
    
    // Prepare test scenario: messages with gap
    messages := []Message{
        {Sequence: 100, Source: parseURL("acc://source")},
        {Sequence: 102, Source: parseURL("acc://source")}, // Gap at 101
        {Sequence: 103, Source: parseURL("acc://source")},
    }
    
    // Set expectation for recovery request
    mockTransport.On("SendRecoveryRequest", mock.MatchedBy(func(req *RecoveryRequest) bool {
        return req.Source.String() == "acc://source" && 
               len(req.MissingSequences) == 1 &&
               req.MissingSequences[0] == 101
    })).Return(nil)
    
    // Execute
    processed := conductor.ProcessInbound(messages)
    
    // Wait for async recovery request
    time.Sleep(100 * time.Millisecond)
    
    // Verify
    assert.Len(t, processed, 3) // All messages processed
    mockTransport.AssertExpectations(t)
    
    // Verify metrics
    assert.Equal(t, 1.0, mockMetrics.GetValue("gaps_detected"))
    assert.Equal(t, 1.0, mockMetrics.GetValue("recovery_requests_sent"))
}
```

#### TestEndToEndRecoveryFlow
```go
func TestEndToEndRecoveryFlow(t *testing.T) {
    // This test simulates a complete recovery scenario between two partitions
    
    // Setup source partition with missing transactions
    sourceDB := setupTestDatabaseWithTransactions(t, map[uint64]*protocol.AnchorTransaction{
        100: createTestAnchor(100, "anchor100"),
        101: createTestAnchor(101, "anchor101"),
        102: createTestAnchor(102, "anchor102"),
    })
    defer sourceDB.Close()
    
    // Setup destination partition (requesting recovery)
    destDB := setupTestDatabase(t)
    defer destDB.Close()
    
    // Create transport channel for communication
    transport := setupTestTransportChannel()
    
    // Create source conductor (provides recovery)
    sourceConductor := NewConductor(&ConductorConfig{
        PartitionID: "source",
        Database:    sourceDB,
        Transport:   transport,
    })
    
    // Create destination conductor (requests recovery)  
    destConductor := NewConductor(&ConductorConfig{
        PartitionID: "destination",
        Database:    destDB,
        Transport:   transport,
    })
    
    // Start both conductors
    go sourceConductor.Start()
    go destConductor.Start()
    defer sourceConductor.Stop()
    defer destConductor.Stop()
    
    // Simulate recovery request from destination
    recoveryReq := &RecoveryRequest{
        Source:           parseURL("acc://source"),
        Destination:      parseURL("acc://destination"),
        MissingSequences: []uint64{100, 101, 102},
        RequestType:      RecoveryTypeManual,
    }
    
    // Send recovery request
    err := destConductor.RequestRecovery(recoveryReq)
    require.NoError(t, err)
    
    // Wait for recovery to complete
    timeout := time.After(5 * time.Second)
    success := make(chan bool, 1)
    
    go func() {
        for {
            recovered := destConductor.GetRecoveredTransactionCount()
            if recovered >= 3 {
                success <- true
                return
            }
            time.Sleep(100 * time.Millisecond)
        }
    }()
    
    select {
    case <-success:
        t.Log("Recovery completed successfully")
    case <-timeout:
        t.Fatal("Recovery timed out")
    }
    
    // Verify all transactions were recovered
    recoveredTransactions := destConductor.GetRecoveredTransactions()
    assert.Len(t, recoveredTransactions, 3)
    
    // Verify collection proof was created and has correct size reduction
    proof := destConductor.GetLastCollectionProof()
    assert.NotNil(t, proof)
    
    // Calculate size reduction (collection proof should be much smaller)
    individualProofSize := calculateIndividualProofSize(recoveredTransactions)
    collectionProofSize := len(proof.Data)
    reduction := float64(individualProofSize-collectionProofSize) / float64(individualProofSize)
    
    assert.Greater(t, reduction, 0.9, "Collection proof should provide >90% size reduction")
}
```

### Performance Tests

#### TestRecoveryPerformance_LargeGaps
```go
func TestRecoveryPerformance_LargeGaps(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping performance test in short mode")
    }
    
    // Create large dataset
    const transactionCount = 10000
    testDB := setupTestDatabaseWithLargeDataset(t, transactionCount)
    defer testDB.Close()
    
    conductor := setupTestConductor(testDB)
    
    // Create recovery request for large gap
    missingSequences := make([]uint64, 1000) // 1000 missing transactions
    for i := 0; i < 1000; i++ {
        missingSequences[i] = uint64(i + 5000)
    }
    
    recoveryReq := &RecoveryRequest{
        Source:           parseURL("acc://source"),
        MissingSequences: missingSequences,
    }
    
    // Measure recovery performance
    start := time.Now()
    
    recovered, err := conductor.RecoverAnchors(recoveryReq)
    require.NoError(t, err)
    
    duration := time.Since(start)
    
    // Performance assertions
    assert.Equal(t, 1000, len(recovered))
    assert.Less(t, duration, 5*time.Second, "Recovery should complete within 5 seconds")
    
    // Measure collection proof creation performance
    start = time.Now()
    proof, err := conductor.buildCollectionProof(recovered)
    require.NoError(t, err)
    proofDuration := time.Since(start)
    
    assert.Less(t, proofDuration, 1*time.Second, "Collection proof should be created within 1 second")
    
    t.Logf("Recovered %d transactions in %v", len(recovered), duration)
    t.Logf("Created collection proof in %v", proofDuration)
}
```

### Error Handling Tests

#### TestRecoveryManager_ErrorHandling
```go
func TestRecoveryManager_ErrorHandling(t *testing.T) {
    tests := []struct {
        name          string
        scenario      string
        setupMocks    func(*MockDatabase, *MockTransport)
        expectedError string
        expectedRetry bool
    }{
        {
            name:     "database connection lost during recovery",
            scenario: "database_error",
            setupMocks: func(db *MockDatabase, transport *MockTransport) {
                db.On("GetAnchorBySequence", mock.Anything, mock.Anything).
                   Return(nil, errors.New("connection lost"))
            },
            expectedRetry: true,
        },
        {
            name:     "transport failure during response",
            scenario: "transport_error", 
            setupMocks: func(db *MockDatabase, transport *MockTransport) {
                db.On("GetAnchorBySequence", mock.Anything, mock.Anything).
                   Return(createTestAnchor(100, "hash"), nil)
                transport.On("SendRecoveryResponse", mock.Anything).
                         Return(errors.New("network timeout"))
            },
            expectedError: "network timeout",
            expectedRetry: true,
        },
        {
            name:     "corrupted data in database",
            scenario: "data_corruption",
            setupMocks: func(db *MockDatabase, transport *MockTransport) {
                corruptedAnchor := &protocol.AnchorTransaction{} // Invalid data
                db.On("GetAnchorBySequence", mock.Anything, mock.Anything).
                   Return(corruptedAnchor, nil)
            },
            expectedError: "invalid transaction data",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mockDB := &MockDatabase{}
            mockTransport := &MockTransport{}
            
            rm := &RecoveryManager{
                database:  mockDB,
                transport: mockTransport,
                logger:    testLogger,
            }
            
            tt.setupMocks(mockDB, mockTransport)
            
            // Execute recovery request
            req := &RecoveryRequest{
                Source:           parseURL("acc://source"),
                MissingSequences: []uint64{100},
            }
            
            _, err := rm.RecoverAnchors(req)
            
            if tt.expectedError != "" {
                assert.Error(t, err)
                assert.Contains(t, err.Error(), tt.expectedError)
            }
            
            // Verify retry behavior if expected
            if tt.expectedRetry {
                // Should have retry logic implemented
                assert.True(t, rm.shouldRetryRecovery(err))
            }
        })
    }
}
```

## Test Data and Utilities

### Mock Implementations
```go
type MockDatabase struct {
    mock.Mock
}

func (m *MockDatabase) GetAnchorBySequence(partition *url.URL, sequence uint64) (*protocol.AnchorTransaction, error) {
    args := m.Called(partition, sequence)
    return args.Get(0).(*protocol.AnchorTransaction), args.Error(1)
}

type MockTransport struct {
    mock.Mock
}

func (m *MockTransport) SendRecoveryRequest(req *RecoveryRequest) error {
    args := m.Called(req)
    return args.Error(0)
}

func (m *MockTransport) SendRecoveryResponse(resp *RecoveryResponse) error {
    args := m.Called(resp)
    return args.Error(0)
}
```

### Test Utilities
```go
func createTestAnchor(sequence uint64, hash string) *protocol.AnchorTransaction {
    return &protocol.AnchorTransaction{
        Sequence:    sequence,
        Hash:        []byte(hash),
        Data:        []byte(fmt.Sprintf("anchor-data-%d", sequence)),
        BlockHeight: sequence * 10,
        Timestamp:   time.Now(),
    }
}

func setupTestDatabase(t *testing.T) *database.Database {
    db, err := database.NewMemoryDatabase()
    require.NoError(t, err)
    return db
}

func parseURL(urlStr string) *url.URL {
    u, _ := url.Parse(urlStr)
    return u
}
```

## Coverage Requirements

### Minimum Coverage Targets
- **RecoveryManager**: 85% statement coverage
- **ProofService**: 80% statement coverage  
- **Conductor.ProcessInbound**: 90% statement coverage
- **Transport layer**: 75% statement coverage

### Critical Path Coverage
- Gap detection logic: 100%
- Recovery request creation: 100%
- Message transmission: 100%
- Error handling paths: 80%

## Test Execution Strategy

### Development Phase
```bash
# Run unit tests during development
go test ./internal/core/execute/v2/crosschain -v

# Run with coverage
go test ./internal/core/execute/v2/crosschain -coverprofile=recovery_coverage.out
go tool cover -html=recovery_coverage.out
```

### Integration Testing
```bash
# Run integration tests
go test ./test/integration/crosschain_recovery -v

# Performance tests
go test ./test/performance/crosschain -run=TestRecoveryPerformance
```

### CI/CD Pipeline
```bash
# Full test suite with validation
make tdd-validate
make test-coverage
```

## Success Criteria

- [ ] All unit tests pass with ≥80% coverage
- [ ] Integration tests demonstrate end-to-end recovery flow
- [ ] Performance tests meet latency requirements (< 1s for 100 tx recovery)
- [ ] Error handling tests cover all failure scenarios
- [ ] No mocks used in production code paths
- [ ] Collection proof size reduction verified (>90%)
- [ ] Automatic gap detection and recovery demonstrated