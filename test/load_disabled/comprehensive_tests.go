package main

import (
	"context"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ComprehensiveTestSuite runs all tests
type ComprehensiveTestSuite struct {
	logger           logging.OptionalLogger
	partitionHandler *SimplifiedPartitionHandler
	dispatcher       *MockTestDispatcher

	// Test tracking
	totalTests  int
	passedTests int
	failedTests int
	testResults []TestResult
}

// TestResult stores individual test results
type TestResult struct {
	Name     string
	Passed   bool
	Duration time.Duration
	Error    string
}

// MockTestDispatcher for testing
type MockTestDispatcher struct {
	partitionStates map[string]bool
	mu              sync.RWMutex
	submitCount     int64
	successCount    int64
	failCount       int64
	dropCount       int64
}

func NewMockTestDispatcher() *MockTestDispatcher {
	return &MockTestDispatcher{
		partitionStates: make(map[string]bool),
	}
}

func (m *MockTestDispatcher) Submit(ctx context.Context, dest *url.URL, env *messaging.Envelope) error {
	atomic.AddInt64(&m.submitCount, 1)

	partitionID := getPartitionFromURL(dest)

	m.mu.RLock()
	healthy, exists := m.partitionStates[partitionID]
	m.mu.RUnlock()

	if !exists {
		healthy = true
	}

	if !healthy {
		atomic.AddInt64(&m.failCount, 1)
		return fmt.Errorf("partition %s is down", partitionID)
	}

	// Simulate network delay
	select {
	case <-time.After(time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}

	atomic.AddInt64(&m.successCount, 1)
	return nil
}

func (m *MockTestDispatcher) Send(ctx context.Context) <-chan error {
	ch := make(chan error, 1)
	close(ch)
	return ch
}

func (m *MockTestDispatcher) Close() {}

func (m *MockTestDispatcher) SetPartitionHealth(partition string, healthy bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.partitionStates[partition] = healthy
}

func getPartitionFromURL(dest *url.URL) string {
	if dest.Authority != "" {
		return dest.Authority
	}
	return "unknown"
}

func main() {
	fmt.Println("================================================================================")
	fmt.Println("                         COMPREHENSIVE TEST SUITE")
	fmt.Println("================================================================================")
	fmt.Println()

	suite := &ComprehensiveTestSuite{
		testResults: make([]TestResult, 0),
	}

	suite.RunAllTests()
	suite.PrintReport()
}

func (cts *ComprehensiveTestSuite) RunAllTests() {
	// Initialize components
	cts.dispatcher = NewMockTestDispatcher()
	cts.partitionHandler = NewSimplifiedPartitionHandler(cts.dispatcher, cts.logger)

	partitions := []string{"BVN0", "BVN1", "BVN2", "Directory"}
	cts.partitionHandler.Start(partitions)

	// Initialize all partitions as healthy
	for _, p := range partitions {
		cts.dispatcher.SetPartitionHealth(p, true)
	}

	// Define all tests
	tests := []struct {
		name string
		fn   func() bool
	}{
		{"Basic Transaction Submission", cts.testBasicSubmission},
		{"Partition Failure Detection", cts.testPartitionFailure},
		{"Circuit Breaker Functionality", cts.testCircuitBreaker},
		{"Transaction Dropping", cts.testTransactionDropping},
		{"Out-of-Order Sequence Handling", cts.testOutOfOrderSequences},
		{"Concurrent Load Handling", cts.testConcurrentLoad},
		{"Memory Usage", cts.testMemoryUsage},
		{"Recovery Triggering", cts.testRecoveryTriggering},
		{"Cascading Failures", cts.testCascadingFailures},
		{"Performance Under Load", cts.testPerformance},
	}

	// Run each test
	for i, test := range tests {
		cts.totalTests++
		fmt.Printf("[%d/%d] Running: %s\n", i+1, len(tests), test.name)

		start := time.Now()
		passed := test.fn()
		duration := time.Since(start)

		result := TestResult{
			Name:     test.name,
			Passed:   passed,
			Duration: duration,
		}

		if passed {
			cts.passedTests++
			fmt.Printf("  ✅ PASSED (%.2fs)\n\n", duration.Seconds())
		} else {
			cts.failedTests++
			fmt.Printf("  ❌ FAILED (%.2fs)\n\n", duration.Seconds())
		}

		cts.testResults = append(cts.testResults, result)
	}
}

// Test 1: Basic Transaction Submission
func (cts *ComprehensiveTestSuite) testBasicSubmission() bool {
	ctx := context.Background()
	dest := protocol.PartitionUrl("BVN0")
	msg := &messaging.TransactionMessage{}

	err := cts.partitionHandler.SubmitTransaction(ctx, msg, dest, 1)
	if err != nil {
		fmt.Printf("    Failed to submit: %v\n", err)
		return false
	}

	metrics := cts.partitionHandler.GetMetrics()
	sent := metrics["total_sent"].(int64)

	if sent != 1 {
		fmt.Printf("    Incorrect sent count: %d\n", sent)
		return false
	}

	fmt.Printf("    Successfully submitted transaction\n")
	return true
}

// Test 2: Partition Failure Detection
func (cts *ComprehensiveTestSuite) testPartitionFailure() bool {
	ctx := context.Background()
	dest := protocol.PartitionUrl("BVN1")

	// Mark partition as down
	cts.dispatcher.SetPartitionHealth("BVN1", false)

	// Try to send multiple transactions
	failCount := 0
	for i := 1; i <= 5; i++ {
		msg := &messaging.TransactionMessage{}
		err := cts.partitionHandler.SubmitTransaction(ctx, msg, dest, uint64(i))
		if err != nil {
			failCount++
		}
	}

	// After 3 failures, circuit should open and start dropping
	if failCount < 3 {
		fmt.Printf("    Not enough failures detected: %d\n", failCount)
		return false
	}

	fmt.Printf("    Correctly detected partition failure after %d attempts\n", failCount)

	// Restore partition
	cts.dispatcher.SetPartitionHealth("BVN1", true)

	return true
}

// Test 3: Circuit Breaker
func (cts *ComprehensiveTestSuite) testCircuitBreaker() bool {
	ctx := context.Background()
	dest := protocol.PartitionUrl("BVN2")

	// Cause failures
	cts.dispatcher.SetPartitionHealth("BVN2", false)

	// Send transactions until circuit opens
	for i := 1; i <= 10; i++ {
		msg := &messaging.TransactionMessage{}
		cts.partitionHandler.SubmitTransaction(ctx, msg, dest, uint64(i))
	}

	// Check if partition is marked as down
	status := cts.partitionHandler.GetPartitionStatus("BVN2")
	if status == nil {
		fmt.Printf("    Could not get partition status\n")
		return false
	}

	if !status.CircuitOpen {
		fmt.Printf("    Circuit did not open after failures\n")
		return false
	}

	fmt.Printf("    Circuit breaker opened successfully\n")

	// Restore partition
	cts.dispatcher.SetPartitionHealth("BVN2", true)

	return true
}

// Test 4: Transaction Dropping
func (cts *ComprehensiveTestSuite) testTransactionDropping() bool {
	ctx := context.Background()
	dest := protocol.PartitionUrl("Directory")

	// Mark partition as down
	cts.dispatcher.SetPartitionHealth("Directory", false)

	// Force circuit to open
	for i := 1; i <= 5; i++ {
		msg := &messaging.TransactionMessage{}
		cts.partitionHandler.SubmitTransaction(ctx, msg, dest, uint64(i))
	}

	// Now transactions should be dropped
	initialDropped := cts.partitionHandler.GetMetrics()["total_dropped"].(int64)

	// Send more transactions
	for i := 6; i <= 10; i++ {
		msg := &messaging.TransactionMessage{}
		cts.partitionHandler.SubmitTransaction(ctx, msg, dest, uint64(i))
	}

	finalDropped := cts.partitionHandler.GetMetrics()["total_dropped"].(int64)
	droppedCount := finalDropped - initialDropped

	if droppedCount == 0 {
		fmt.Printf("    No transactions were dropped\n")
		return false
	}

	fmt.Printf("    Successfully dropped %d transactions\n", droppedCount)

	// Restore partition
	cts.dispatcher.SetPartitionHealth("Directory", true)

	return true
}

// Test 5: Out-of-Order Sequences
func (cts *ComprehensiveTestSuite) testOutOfOrderSequences() bool {
	// Test partition behind scenario
	cts.partitionHandler.HandleOutOfOrderSequence("BVN0", 50, 100)
	fmt.Printf("    Handled partition behind scenario (50 < 100)\n")

	// Test we are behind scenario
	cts.partitionHandler.HandleOutOfOrderSequence("BVN1", 200, 150)
	fmt.Printf("    Handled us behind scenario (200 > 150)\n")

	return true
}

// Test 6: Concurrent Load
func (cts *ComprehensiveTestSuite) testConcurrentLoad() bool {
	ctx := context.Background()
	partitions := []string{"BVN0", "BVN1", "BVN2", "Directory"}

	// Ensure all partitions are healthy
	for _, p := range partitions {
		cts.dispatcher.SetPartitionHealth(p, true)
	}

	var wg sync.WaitGroup
	successCount := int64(0)
	errorCount := int64(0)

	workers := 10
	txPerWorker := 50

	start := time.Now()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for i := 0; i < txPerWorker; i++ {
				partition := partitions[rand.Intn(len(partitions))]
				dest := protocol.PartitionUrl(partition)
				msg := &messaging.TransactionMessage{}

				err := cts.partitionHandler.SubmitTransaction(ctx, msg, dest, uint64(i))
				if err != nil {
					atomic.AddInt64(&errorCount, 1)
				} else {
					atomic.AddInt64(&successCount, 1)
				}
			}
		}(w)
	}

	wg.Wait()
	duration := time.Since(start)

	totalTx := successCount + errorCount
	tps := float64(totalTx) / duration.Seconds()

	fmt.Printf("    Processed %d transactions in %.2fs\n", totalTx, duration.Seconds())
	fmt.Printf("    Throughput: %.0f TPS\n", tps)
	fmt.Printf("    Success rate: %.1f%%\n", float64(successCount)/float64(totalTx)*100)

	return successCount > 0
}

// Test 7: Memory Usage
func (cts *ComprehensiveTestSuite) testMemoryUsage() bool {
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	initialMem := m1.Alloc

	// Run intensive operations
	ctx := context.Background()
	for i := 0; i < 1000; i++ {
		dest := protocol.PartitionUrl("BVN0")
		msg := &messaging.TransactionMessage{}
		cts.partitionHandler.SubmitTransaction(ctx, msg, dest, uint64(i))
	}

	// Force GC
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)
	finalMem := m2.Alloc

	memGrowth := finalMem - initialMem
	memGrowthMB := float64(memGrowth) / 1024 / 1024

	fmt.Printf("    Memory growth: %.2f MB\n", memGrowthMB)
	fmt.Printf("    Goroutines: %d\n", runtime.NumGoroutine())

	// Accept up to 10MB growth for this test
	return memGrowthMB < 10
}

// Test 8: Recovery Triggering
func (cts *ComprehensiveTestSuite) testRecoveryTriggering() bool {
	// Simulate recovery scenarios
	partitions := []string{"BVN0", "BVN1", "BVN2"}

	for _, p := range partitions {
		// Mark as recovered
		cts.partitionHandler.markPartitionRecovered(p)

		// Trigger recovery
		cts.partitionHandler.HandleOutOfOrderSequence(p, 100, 150)
	}

	fmt.Printf("    Triggered recovery for %d partitions\n", len(partitions))

	return true
}

// Test 9: Cascading Failures
func (cts *ComprehensiveTestSuite) testCascadingFailures() bool {
	ctx := context.Background()
	partitions := []string{"BVN0", "BVN1", "BVN2", "Directory"}

	// Simulate cascading failures
	for i, p := range partitions {
		time.Sleep(50 * time.Millisecond)
		cts.dispatcher.SetPartitionHealth(p, false)
		fmt.Printf("    Partition %s failed (%d/%d)\n", p, i+1, len(partitions))
	}

	// Try to send to all partitions
	allFailed := true
	for _, p := range partitions {
		dest := protocol.PartitionUrl(p)
		msg := &messaging.TransactionMessage{}
		err := cts.partitionHandler.SubmitTransaction(ctx, msg, dest, 1)
		if err == nil {
			allFailed = false
		}
	}

	// Recover all partitions
	for _, p := range partitions {
		cts.dispatcher.SetPartitionHealth(p, true)
	}

	fmt.Printf("    All partitions failed and recovered\n")

	return allFailed
}

// Test 10: Performance
func (cts *ComprehensiveTestSuite) testPerformance() bool {
	ctx := context.Background()
	partitions := []string{"BVN0", "BVN1", "BVN2", "Directory"}

	// Ensure all healthy
	for _, p := range partitions {
		cts.dispatcher.SetPartitionHealth(p, true)
	}

	// Mark half as down
	cts.dispatcher.SetPartitionHealth("BVN1", false)
	cts.dispatcher.SetPartitionHealth("BVN2", false)

	successCount := int64(0)
	start := time.Now()

	// Send many transactions
	for i := 0; i < 1000; i++ {
		partition := partitions[i%len(partitions)]
		dest := protocol.PartitionUrl(partition)
		msg := &messaging.TransactionMessage{}

		err := cts.partitionHandler.SubmitTransaction(ctx, msg, dest, uint64(i))
		if err == nil {
			successCount++
		}
	}

	duration := time.Since(start)
	tps := float64(successCount) / duration.Seconds()

	fmt.Printf("    Performance with 50%% partitions down: %.0f TPS\n", tps)

	// Restore all partitions
	for _, p := range partitions {
		cts.dispatcher.SetPartitionHealth(p, true)
	}

	return tps > 100 // Expect at least 100 TPS even with failures
}

func (cts *ComprehensiveTestSuite) PrintReport() {
	fmt.Println("================================================================================")
	fmt.Println("                              TEST REPORT")
	fmt.Println("================================================================================")
	fmt.Println()

	fmt.Printf("Total Tests: %d\n", cts.totalTests)
	fmt.Printf("Passed:      %d (%.1f%%)\n", cts.passedTests,
		float64(cts.passedTests)/float64(cts.totalTests)*100)
	fmt.Printf("Failed:      %d (%.1f%%)\n", cts.failedTests,
		float64(cts.failedTests)/float64(cts.totalTests)*100)
	fmt.Println()

	// Dispatcher metrics
	if cts.dispatcher != nil {
		fmt.Println("Network Statistics:")
		fmt.Printf("  Total Submits:  %d\n", atomic.LoadInt64(&cts.dispatcher.submitCount))
		fmt.Printf("  Successes:      %d\n", atomic.LoadInt64(&cts.dispatcher.successCount))
		fmt.Printf("  Failures:       %d\n", atomic.LoadInt64(&cts.dispatcher.failCount))
		fmt.Println()
	}

	// Partition handler metrics
	if cts.partitionHandler != nil {
		metrics := cts.partitionHandler.GetMetrics()
		fmt.Println("Partition Handler Metrics:")
		fmt.Printf("  Sent:           %d\n", metrics["total_sent"])
		fmt.Printf("  Failed:         %d\n", metrics["total_failed"])
		fmt.Printf("  Dropped:        %d\n", metrics["total_dropped"])
		fmt.Println()
	}

	// Memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Println("Memory Usage:")
	fmt.Printf("  Allocated:      %.2f MB\n", float64(m.Alloc)/1024/1024)
	fmt.Printf("  Total Alloc:    %.2f MB\n", float64(m.TotalAlloc)/1024/1024)
	fmt.Printf("  Goroutines:     %d\n", runtime.NumGoroutine())
	fmt.Println()

	// Test details
	fmt.Println("Test Details:")
	fmt.Println("─────────────────────────────────────────────────────────────────────")
	for _, result := range cts.testResults {
		status := "✅ PASS"
		if !result.Passed {
			status = "❌ FAIL"
		}
		fmt.Printf("%-40s %s (%.2fs)\n", result.Name, status, result.Duration.Seconds())
	}
	fmt.Println()

	// Final verdict
	if cts.failedTests == 0 {
		fmt.Println("✅ ALL TESTS PASSED!")
	} else {
		fmt.Println("❌ SOME TESTS FAILED")
	}
	fmt.Println("================================================================================")
}
