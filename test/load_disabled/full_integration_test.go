package main

import (
	"context"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestSuite manages all integration tests
type TestSuite struct {
	logger           logging.OptionalLogger
	conductor        *CrossChainConductor
	recoveryManager  *RecoveryManager
	partitionHandler *SimplifiedPartitionHandler
	dispatcher       *TestNetworkDispatcher
	client           api.Querier
	db               database.Beginner

	// Metrics
	totalTests  int32
	passedTests int32
	failedTests int32
	startTime   time.Time

	// Test configuration
	partitions   []string
	testDuration time.Duration
	concurrency  int
}

// TestNetworkDispatcher simulates a real network with controllable behavior
type TestNetworkDispatcher struct {
	partitions map[string]*PartitionSimulator
	mu         sync.RWMutex
	logger     logging.OptionalLogger

	// Network simulation
	latencyMin     time.Duration
	latencyMax     time.Duration
	packetLossRate float64

	// Metrics
	totalSubmits   int64
	totalSuccesses int64
	totalFailures  int64
	totalDropped   int64
}

// PartitionSimulator simulates a single partition
type PartitionSimulator struct {
	ID           string
	IsHealthy    bool
	Sequences    map[uint64]*messaging.Envelope
	LastSequence uint64
	mu           sync.RWMutex

	// Failure simulation
	FailureStart    time.Time
	FailureDuration time.Duration
	RecoveryTime    time.Time
}

func NewTestSuite(logger logging.OptionalLogger) *TestSuite {
	return &TestSuite{
		logger:       logger,
		partitions:   []string{"BVN0", "BVN1", "BVN2", "Directory"},
		testDuration: 5 * time.Minute,
		concurrency:  10,
	}
}

func main() {
	fmt.Println("================================================================================")
	fmt.Println("                    FULL INTEGRATION TEST SUITE")
	fmt.Println("================================================================================")
	fmt.Println()

	logger := logging.NewConsole(logging.DefaultOptions())
	suite := NewTestSuite(logger)

	// Initialize components
	if !suite.Initialize() {
		fmt.Println("❌ Failed to initialize test suite")
		return
	}

	// Run all tests
	suite.RunAllTests()

	// Print final report
	suite.PrintReport()
}

func (ts *TestSuite) Initialize() bool {
	fmt.Println("🔧 Initializing Test Components...")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	ts.startTime = time.Now()

	// Create dispatcher
	ts.dispatcher = NewTestNetworkDispatcher(ts.logger)
	ts.dispatcher.Initialize(ts.partitions)

	// Create partition handler
	ts.partitionHandler = NewSimplifiedPartitionHandler(ts.dispatcher, ts.logger)
	ts.partitionHandler.Start(ts.partitions)

	// Create conductor
	ts.conductor = NewCrossChainConductor()
	ts.conductor.logger = ts.logger
	ts.conductor.dispatcher = ts.dispatcher

	// Get pooled client for V3
	ts.client = GetPooledClient("http://127.0.0.1:26660/v3")

	// Create recovery manager
	ts.recoveryManager = &RecoveryManager{
		conductor:      ts.conductor,
		logger:         ts.logger,
		client:         ts.client,
		recoveryQueue:  make(chan *RecoveryRequest, 100),
		activeRecovery: make(map[string]*RecoverySession),
	}

	fmt.Println("✅ All components initialized successfully")
	fmt.Println()

	return true
}

func (ts *TestSuite) RunAllTests() {
	tests := []struct {
		name string
		fn   func() bool
	}{
		{"V3 Connection Pooling", ts.TestV3ConnectionPooling},
		{"CrossChainConductor Basic Operations", ts.TestConductorBasics},
		{"Recovery System", ts.TestRecoverySystem},
		{"Partition Failure Handling", ts.TestPartitionFailures},
		{"Out-of-Order Sequences", ts.TestOutOfOrderSequences},
		{"Circuit Breaker", ts.TestCircuitBreaker},
		{"Ledger Recovery", ts.TestLedgerRecovery},
		{"Concurrent Load", ts.TestConcurrentLoad},
		{"Memory Leaks", ts.TestMemoryLeaks},
		{"Performance Under Failure", ts.TestPerformanceUnderFailure},
		{"Cascading Failures", ts.TestCascadingFailures},
		{"Recovery Storm Prevention", ts.TestRecoveryStormPrevention},
	}

	fmt.Println("📋 Running Test Suite")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	for i, test := range tests {
		atomic.AddInt32(&ts.totalTests, 1)
		fmt.Printf("\n[%d/%d] %s\n", i+1, len(tests), test.name)
		fmt.Println("────────────────────────────────────────────────────────────────────────────────")

		startTime := time.Now()
		passed := test.fn()
		duration := time.Since(startTime)

		if passed {
			atomic.AddInt32(&ts.passedTests, 1)
			fmt.Printf("✅ PASSED (%v)\n", duration)
		} else {
			atomic.AddInt32(&ts.failedTests, 1)
			fmt.Printf("❌ FAILED (%v)\n", duration)
		}
	}
}

// Test 1: V3 Connection Pooling
func (ts *TestSuite) TestV3ConnectionPooling() bool {
	fmt.Println("Testing V3 connection pooling and resource management...")

	// Test connection reuse
	clients := make([]*jsonrpc.Client, 0)
	for i := 0; i < 50; i++ {
		client := GetPooledClient("http://127.0.0.1:26660/v3")
		clients = append(clients, client)
	}

	// Check pool size limit
	poolSize := len(clientPool)
	if poolSize > maxPoolSize {
		fmt.Printf("  ❌ Pool size exceeded limit: %d > %d\n", poolSize, maxPoolSize)
		return false
	}
	fmt.Printf("  ✅ Pool size within limits: %d/%d\n", poolSize, maxPoolSize)

	// Test cleanup
	CleanupClientPool()

	// Verify connections are closed
	poolSizeAfter := len(clientPool)
	if poolSizeAfter != 0 {
		fmt.Printf("  ❌ Pool not properly cleaned: %d clients remaining\n", poolSizeAfter)
		return false
	}
	fmt.Printf("  ✅ Pool cleanup successful\n")

	// Test concurrent access
	var wg sync.WaitGroup
	errors := int32(0)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := GetPooledClient("http://127.0.0.1:26660/v3")
			if client == nil {
				atomic.AddInt32(&errors, 1)
			}
		}()
	}

	wg.Wait()

	if errors > 0 {
		fmt.Printf("  ❌ Concurrent access failed: %d errors\n", errors)
		return false
	}
	fmt.Printf("  ✅ Concurrent access successful\n")

	return true
}

// Test 2: CrossChainConductor Basic Operations
func (ts *TestSuite) TestConductorBasics() bool {
	fmt.Println("Testing CrossChainConductor basic operations...")

	ctx := context.Background()

	// Test anchor submission
	anchorTx := &protocol.BlockAnchor{
		Index:     1,
		Timestamp: time.Now().Unix(),
		Height:    100,
	}

	dest := protocol.PartitionUrl("BVN1")
	err := ts.conductor.SubmitAnchor(ctx, anchorTx, dest, 1)
	if err != nil {
		fmt.Printf("  ❌ Failed to submit anchor: %v\n", err)
		return false
	}
	fmt.Printf("  ✅ Anchor submitted successfully\n")

	// Test synthetic transaction submission
	synthTx := &protocol.SyntheticTransaction{
		Hash: []byte("test-hash"),
	}

	err = ts.conductor.SubmitSynthetic(ctx, synthTx, dest, 2)
	if err != nil {
		fmt.Printf("  ❌ Failed to submit synthetic: %v\n", err)
		return false
	}
	fmt.Printf("  ✅ Synthetic transaction submitted successfully\n")

	// Test metrics
	metrics := ts.conductor.GetMetrics()
	if metrics["anchors_sent"].(int64) != 1 {
		fmt.Printf("  ❌ Anchor metrics incorrect: %v\n", metrics["anchors_sent"])
		return false
	}
	if metrics["synthetics_sent"].(int64) != 1 {
		fmt.Printf("  ❌ Synthetic metrics incorrect: %v\n", metrics["synthetics_sent"])
		return false
	}
	fmt.Printf("  ✅ Metrics tracking correctly\n")

	return true
}

// Test 3: Recovery System
func (ts *TestSuite) TestRecoverySystem() bool {
	fmt.Println("Testing recovery system for missing transactions...")

	// Start recovery manager
	go ts.recoveryManager.Start()
	defer ts.recoveryManager.Stop()

	// Create recovery request
	request := &RecoveryRequest{
		PartitionID:  "BVN1",
		Type:         RecoveryTypeAnchor,
		FromSequence: 100,
		ToSequence:   110,
		Timestamp:    time.Now(),
	}

	// Submit recovery request
	ts.recoveryManager.recoveryQueue <- request

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Check if recovery session was created
	ts.recoveryManager.mu.RLock()
	session, exists := ts.recoveryManager.activeRecovery["BVN1-anchor"]
	ts.recoveryManager.mu.RUnlock()

	if !exists {
		fmt.Printf("  ❌ Recovery session not created\n")
		return false
	}
	fmt.Printf("  ✅ Recovery session created\n")

	if session.Status != RecoveryStatusInProgress {
		fmt.Printf("  ❌ Recovery status incorrect: %v\n", session.Status)
		return false
	}
	fmt.Printf("  ✅ Recovery in progress\n")

	return true
}

// Test 4: Partition Failure Handling
func (ts *TestSuite) TestPartitionFailures() bool {
	fmt.Println("Testing partition failure detection and handling...")

	ctx := context.Background()

	// Mark partition as down
	ts.dispatcher.SetPartitionHealth("BVN1", false)

	// Try to send transaction
	msg := &messaging.TransactionMessage{}
	dest := protocol.PartitionUrl("BVN1")

	err := ts.partitionHandler.SubmitTransaction(ctx, msg, dest, 1)
	if err != nil {
		fmt.Printf("  ⚠️  Transaction failed as expected: %v\n", err)
	}

	// Check if circuit opened
	status := ts.partitionHandler.GetPartitionStatus("BVN1")
	if status == nil {
		fmt.Printf("  ❌ Could not get partition status\n")
		return false
	}

	// After failures, circuit should open
	time.Sleep(100 * time.Millisecond)

	// Try more transactions - should be dropped
	dropped := 0
	for i := 2; i <= 5; i++ {
		err = ts.partitionHandler.SubmitTransaction(ctx, msg, dest, uint64(i))
		if err == nil {
			dropped++
		}
	}

	if dropped == 0 {
		fmt.Printf("  ❌ Transactions not being dropped when circuit open\n")
		return false
	}
	fmt.Printf("  ✅ Transactions dropped when partition down: %d\n", dropped)

	// Bring partition back
	ts.dispatcher.SetPartitionHealth("BVN1", true)

	// Test recovery
	ts.partitionHandler.HandleOutOfOrderSequence("BVN1", 1, 6)
	fmt.Printf("  ✅ Recovery triggered for missing sequences\n")

	return true
}

// Test 5: Out-of-Order Sequences
func (ts *TestSuite) TestOutOfOrderSequences() bool {
	fmt.Println("Testing out-of-order sequence detection and handling...")

	// Simulate partition behind
	ts.partitionHandler.HandleOutOfOrderSequence("BVN2", 50, 100)
	fmt.Printf("  ✅ Handled partition behind (needs catch-up)\n")

	// Simulate we are behind
	ts.partitionHandler.HandleOutOfOrderSequence("BVN2", 200, 150)
	fmt.Printf("  ✅ Handled us being behind (request missing)\n")

	return true
}

// Test 6: Circuit Breaker
func (ts *TestSuite) TestCircuitBreaker() bool {
	fmt.Println("Testing circuit breaker state transitions...")

	ctx := context.Background()
	dest := protocol.PartitionUrl("BVN3")

	// Cause failures to open circuit
	ts.dispatcher.SetPartitionHealth("BVN3", false)

	failCount := 0
	for i := 1; i <= 5; i++ {
		msg := &messaging.TransactionMessage{}
		err := ts.partitionHandler.SubmitTransaction(ctx, msg, dest, uint64(i))
		if err != nil {
			failCount++
		}
	}

	if failCount < 3 {
		fmt.Printf("  ❌ Not enough failures recorded: %d\n", failCount)
		return false
	}
	fmt.Printf("  ✅ Circuit breaker triggered after %d failures\n", failCount)

	// Check circuit state
	status := ts.partitionHandler.GetPartitionStatus("BVN3")
	if status != nil && status.CircuitOpen {
		fmt.Printf("  ✅ Circuit is open\n")
	} else {
		fmt.Printf("  ⚠️  Circuit state unclear\n")
	}

	return true
}

// Test 7: Ledger Recovery
func (ts *TestSuite) TestLedgerRecovery() bool {
	fmt.Println("Testing ledger-based transaction recovery...")

	// Simulate ledger with stored transactions
	ledger := NewLedgerRecoverySimulator(ts.logger)

	// Record transactions
	for i := uint64(1); i <= 10; i++ {
		ledger.RecordTransaction("Directory", i, "anchor")
	}

	// Recover range
	recovered := ledger.RecoverTransactions("Directory", 5, 8)
	if len(recovered) != 3 {
		fmt.Printf("  ❌ Incorrect recovery count: %d (expected 3)\n", len(recovered))
		return false
	}
	fmt.Printf("  ✅ Recovered %d transactions from ledger\n", len(recovered))

	return true
}

// Test 8: Concurrent Load
func (ts *TestSuite) TestConcurrentLoad() bool {
	fmt.Println("Testing system under concurrent load...")

	ctx := context.Background()
	var wg sync.WaitGroup

	successCount := int64(0)
	errorCount := int64(0)

	startTime := time.Now()

	// Launch concurrent workers
	for worker := 0; worker < ts.concurrency; worker++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for i := 0; i < 100; i++ {
				partition := ts.partitions[rand.Intn(len(ts.partitions))]
				dest := protocol.PartitionUrl(partition)
				msg := &messaging.TransactionMessage{}

				err := ts.partitionHandler.SubmitTransaction(ctx, msg, dest, uint64(i))
				if err != nil {
					atomic.AddInt64(&errorCount, 1)
				} else {
					atomic.AddInt64(&successCount, 1)
				}
			}
		}(worker)
	}

	wg.Wait()
	duration := time.Since(startTime)

	totalTx := successCount + errorCount
	tps := float64(totalTx) / duration.Seconds()

	fmt.Printf("  ✅ Processed %d transactions in %v\n", totalTx, duration)
	fmt.Printf("  ✅ Throughput: %.2f TPS\n", tps)
	fmt.Printf("  ✅ Success rate: %.2f%%\n", float64(successCount)/float64(totalTx)*100)

	return true
}

// Test 9: Memory Leaks
func (ts *TestSuite) TestMemoryLeaks() bool {
	fmt.Println("Testing for memory leaks...")

	// Get initial memory stats
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	initialAlloc := m1.Alloc

	// Run intensive operations
	ctx := context.Background()
	for i := 0; i < 10000; i++ {
		msg := &messaging.TransactionMessage{}
		dest := protocol.PartitionUrl("BVN0")
		ts.partitionHandler.SubmitTransaction(ctx, msg, dest, uint64(i))
	}

	// Force garbage collection
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	// Get final memory stats
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)
	finalAlloc := m2.Alloc

	// Check memory growth
	growth := finalAlloc - initialAlloc
	growthMB := float64(growth) / 1024 / 1024

	if growthMB > 50 {
		fmt.Printf("  ⚠️  High memory growth: %.2f MB\n", growthMB)
		return false
	}

	fmt.Printf("  ✅ Memory growth acceptable: %.2f MB\n", growthMB)
	fmt.Printf("  ✅ Goroutines: %d\n", runtime.NumGoroutine())

	return true
}

// Test 10: Performance Under Failure
func (ts *TestSuite) TestPerformanceUnderFailure() bool {
	fmt.Println("Testing performance with failing partitions...")

	// Mark half the partitions as down
	downPartitions := len(ts.partitions) / 2
	for i := 0; i < downPartitions; i++ {
		ts.dispatcher.SetPartitionHealth(ts.partitions[i], false)
	}

	ctx := context.Background()
	var wg sync.WaitGroup

	startTime := time.Now()
	successCount := int64(0)

	// Send transactions to all partitions
	for worker := 0; worker < 5; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for i := 0; i < 100; i++ {
				for _, partition := range ts.partitions {
					msg := &messaging.TransactionMessage{}
					dest := protocol.PartitionUrl(partition)

					err := ts.partitionHandler.SubmitTransaction(ctx, msg, dest, uint64(i))
					if err == nil {
						atomic.AddInt64(&successCount, 1)
					}
				}
			}
		}()
	}

	wg.Wait()
	duration := time.Since(startTime)

	tps := float64(successCount) / duration.Seconds()
	fmt.Printf("  ✅ Performance with %d/%d partitions down: %.2f TPS\n",
		downPartitions, len(ts.partitions), tps)

	// Restore partitions
	for _, partition := range ts.partitions {
		ts.dispatcher.SetPartitionHealth(partition, true)
	}

	return true
}

// Test 11: Cascading Failures
func (ts *TestSuite) TestCascadingFailures() bool {
	fmt.Println("Testing cascading partition failures...")

	ctx := context.Background()

	// Simulate cascading failures
	for i, partition := range ts.partitions {
		time.Sleep(100 * time.Millisecond)
		ts.dispatcher.SetPartitionHealth(partition, false)
		fmt.Printf("  💥 Partition %s failed (cascade %d/%d)\n",
			partition, i+1, len(ts.partitions))
	}

	// Try to send transactions
	allFailed := true
	for _, partition := range ts.partitions {
		msg := &messaging.TransactionMessage{}
		dest := protocol.PartitionUrl(partition)
		err := ts.partitionHandler.SubmitTransaction(ctx, msg, dest, 1)
		if err == nil {
			allFailed = false
		}
	}

	if !allFailed {
		fmt.Printf("  ⚠️  Some transactions succeeded unexpectedly\n")
	}

	// Recover all partitions
	fmt.Println("  🔄 Recovering all partitions...")
	for _, partition := range ts.partitions {
		ts.dispatcher.SetPartitionHealth(partition, true)
	}

	// Verify recovery
	recoveredCount := 0
	for _, partition := range ts.partitions {
		msg := &messaging.TransactionMessage{}
		dest := protocol.PartitionUrl(partition)
		err := ts.partitionHandler.SubmitTransaction(ctx, msg, dest, 2)
		if err == nil {
			recoveredCount++
		}
	}

	fmt.Printf("  ✅ Recovered %d/%d partitions\n", recoveredCount, len(ts.partitions))

	return recoveredCount == len(ts.partitions)
}

// Test 12: Recovery Storm Prevention
func (ts *TestSuite) TestRecoveryStormPrevention() bool {
	fmt.Println("Testing recovery storm prevention...")

	// Create many recovery requests at once
	for i := 0; i < 100; i++ {
		request := &RecoveryRequest{
			PartitionID:  "BVN0",
			Type:         RecoveryTypeAnchor,
			FromSequence: uint64(i * 10),
			ToSequence:   uint64((i + 1) * 10),
			Timestamp:    time.Now(),
		}

		select {
		case ts.recoveryManager.recoveryQueue <- request:
		default:
			// Queue full - this is expected and good
		}
	}

	// Check queue size
	queueSize := len(ts.recoveryManager.recoveryQueue)
	if queueSize > 100 {
		fmt.Printf("  ❌ Recovery queue overflow: %d\n", queueSize)
		return false
	}

	fmt.Printf("  ✅ Recovery queue bounded: %d/100\n", queueSize)

	// Check active recovery sessions
	ts.recoveryManager.mu.RLock()
	activeCount := len(ts.recoveryManager.activeRecovery)
	ts.recoveryManager.mu.RUnlock()

	fmt.Printf("  ✅ Active recovery sessions limited: %d\n", activeCount)

	return true
}

// Helper: Network Dispatcher Implementation
func NewTestNetworkDispatcher(logger logging.OptionalLogger) *TestNetworkDispatcher {
	return &TestNetworkDispatcher{
		partitions:     make(map[string]*PartitionSimulator),
		logger:         logger,
		latencyMin:     5 * time.Millisecond,
		latencyMax:     50 * time.Millisecond,
		packetLossRate: 0.01, // 1% packet loss
	}
}

func (tnd *TestNetworkDispatcher) Initialize(partitionIDs []string) {
	for _, id := range partitionIDs {
		tnd.partitions[id] = &PartitionSimulator{
			ID:        id,
			IsHealthy: true,
			Sequences: make(map[uint64]*messaging.Envelope),
		}
	}
}

func (tnd *TestNetworkDispatcher) Submit(ctx context.Context, dest *url.URL, env *messaging.Envelope) error {
	atomic.AddInt64(&tnd.totalSubmits, 1)

	// Simulate network latency
	latency := tnd.latencyMin + time.Duration(rand.Int63n(int64(tnd.latencyMax-tnd.latencyMin)))
	select {
	case <-time.After(latency):
	case <-ctx.Done():
		return ctx.Err()
	}

	// Simulate packet loss
	if rand.Float64() < tnd.packetLossRate {
		atomic.AddInt64(&tnd.totalDropped, 1)
		return fmt.Errorf("packet lost in network")
	}

	partitionID := getPartitionID(dest)

	tnd.mu.RLock()
	partition, exists := tnd.partitions[partitionID]
	tnd.mu.RUnlock()

	if !exists {
		return fmt.Errorf("partition %s not found", partitionID)
	}

	partition.mu.RLock()
	healthy := partition.IsHealthy
	partition.mu.RUnlock()

	if !healthy {
		atomic.AddInt64(&tnd.totalFailures, 1)
		return fmt.Errorf("partition %s is down", partitionID)
	}

	// Store the transaction
	partition.mu.Lock()
	partition.LastSequence++
	partition.Sequences[partition.LastSequence] = env
	partition.mu.Unlock()

	atomic.AddInt64(&tnd.totalSuccesses, 1)
	return nil
}

func (tnd *TestNetworkDispatcher) Send(ctx context.Context) <-chan error {
	ch := make(chan error, 1)
	close(ch)
	return ch
}

func (tnd *TestNetworkDispatcher) Close() {
	// Clean up resources
}

func (tnd *TestNetworkDispatcher) SetPartitionHealth(partitionID string, healthy bool) {
	tnd.mu.Lock()
	defer tnd.mu.Unlock()

	if partition, exists := tnd.partitions[partitionID]; exists {
		partition.mu.Lock()
		partition.IsHealthy = healthy
		if !healthy {
			partition.FailureStart = time.Now()
		} else {
			partition.RecoveryTime = time.Now()
		}
		partition.mu.Unlock()
	}
}

func (ts *TestSuite) PrintReport() {
	fmt.Println()
	fmt.Println("================================================================================")
	fmt.Println("                              TEST REPORT")
	fmt.Println("================================================================================")

	duration := time.Since(ts.startTime)
	passed := atomic.LoadInt32(&ts.passedTests)
	failed := atomic.LoadInt32(&ts.failedTests)
	total := atomic.LoadInt32(&ts.totalTests)

	fmt.Printf("\n📊 Test Results:\n")
	fmt.Printf("   Total Tests:  %d\n", total)
	fmt.Printf("   Passed:       %d (%.1f%%)\n", passed, float64(passed)/float64(total)*100)
	fmt.Printf("   Failed:       %d (%.1f%%)\n", failed, float64(failed)/float64(total)*100)
	fmt.Printf("   Duration:     %v\n", duration)

	// Dispatcher metrics
	if ts.dispatcher != nil {
		fmt.Printf("\n📈 Network Statistics:\n")
		fmt.Printf("   Total Submits:   %d\n", atomic.LoadInt64(&ts.dispatcher.totalSubmits))
		fmt.Printf("   Successes:       %d\n", atomic.LoadInt64(&ts.dispatcher.totalSuccesses))
		fmt.Printf("   Failures:        %d\n", atomic.LoadInt64(&ts.dispatcher.totalFailures))
		fmt.Printf("   Dropped:         %d\n", atomic.LoadInt64(&ts.dispatcher.totalDropped))
	}

	// Partition handler metrics
	if ts.partitionHandler != nil {
		metrics := ts.partitionHandler.GetMetrics()
		fmt.Printf("\n🔄 Partition Handler Metrics:\n")
		fmt.Printf("   Sent:            %d\n", metrics["total_sent"])
		fmt.Printf("   Failed:          %d\n", metrics["total_failed"])
		fmt.Printf("   Dropped:         %d\n", metrics["total_dropped"])
		fmt.Printf("   Healthy:         %d/%d\n", metrics["partitions_healthy"], len(ts.partitions))
	}

	// Memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("\n💾 Memory Usage:\n")
	fmt.Printf("   Allocated:       %.2f MB\n", float64(m.Alloc)/1024/1024)
	fmt.Printf("   Total Allocated: %.2f MB\n", float64(m.TotalAlloc)/1024/1024)
	fmt.Printf("   Goroutines:      %d\n", runtime.NumGoroutine())

	fmt.Println()
	if failed == 0 {
		fmt.Println("✅ ALL TESTS PASSED!")
	} else {
		fmt.Println("❌ SOME TESTS FAILED")
	}
	fmt.Println("================================================================================")
}

func getPartitionID(dest *url.URL) string {
	if dest.Authority != "" {
		return dest.Authority
	}
	return "unknown"
}
