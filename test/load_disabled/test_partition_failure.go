package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// MockDispatcher simulates the network dispatcher with controllable failures
type MockDispatcher struct {
	partitionStates map[string]bool // true = healthy, false = down
	mu              sync.RWMutex
	submitCount     int64
	failCount       int64
	successCount    int64
}

func NewMockDispatcher() *MockDispatcher {
	return &MockDispatcher{
		partitionStates: make(map[string]bool),
	}
}

func (md *MockDispatcher) Submit(ctx context.Context, dest *url.URL, env *messaging.Envelope) error {
	atomic.AddInt64(&md.submitCount, 1)
	
	partitionID := getPartitionID(dest)
	
	md.mu.RLock()
	healthy, exists := md.partitionStates[partitionID]
	md.mu.RUnlock()
	
	if !exists {
		// Default to healthy
		healthy = true
	}
	
	if !healthy {
		atomic.AddInt64(&md.failCount, 1)
		return fmt.Errorf("connection refused: partition %s is down", partitionID)
	}
	
	// Simulate network delay
	time.Sleep(10 * time.Millisecond)
	
	atomic.AddInt64(&md.successCount, 1)
	return nil
}

func (md *MockDispatcher) SetPartitionState(partitionID string, healthy bool) {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.partitionStates[partitionID] = healthy
}

func main() {
	fmt.Println("========================================")
	fmt.Println("    PARTITION FAILURE HANDLING TEST")
	fmt.Println("========================================")
	fmt.Println()
	
	// Run test scenarios
	testBasicFailureHandling()
	fmt.Println()
	testCircuitBreaker()
	fmt.Println()
	testPartitionRecovery()
	fmt.Println()
	testOutOfOrderHandling()
	fmt.Println()
	testQueueLimits()
	
	fmt.Println("\n========================================")
	fmt.Println("         ALL TESTS COMPLETED")
	fmt.Println("========================================")
}

func testBasicFailureHandling() {
	fmt.Println("Test 1: Basic Failure Handling")
	fmt.Println("------------------------------")
	
	dispatcher := NewMockDispatcher()
	logger := logging.NewTestLogger(nil, "error", false)
	conductor := NewEnhancedCrossChainConductor(dispatcher, logger)
	
	partitions := []string{"BVN0", "BVN1", "BVN2", "Directory"}
	conductor.Start(partitions)
	defer conductor.Stop()
	
	// Set BVN1 as healthy initially
	dispatcher.SetPartitionState("BVN1", true)
	
	// Send successful transaction
	ctx := context.Background()
	dest := protocol.PartitionUrl("BVN1")
	msg := &messaging.TransactionMessage{}
	
	err := conductor.SubmitTransaction(ctx, msg, dest, 1)
	if err != nil {
		fmt.Printf("  ❌ Failed to send to healthy partition: %v\n", err)
	} else {
		fmt.Printf("  ✅ Successfully sent to healthy partition\n")
	}
	
	// Mark BVN1 as down
	dispatcher.SetPartitionState("BVN1", false)
	time.Sleep(100 * time.Millisecond)
	
	// Try to send - should fail and queue
	for i := 0; i < 5; i++ {
		err = conductor.SubmitTransaction(ctx, msg, dest, uint64(i+2))
		if err != nil {
			fmt.Printf("  ❌ Transaction %d failed: %v\n", i+2, err)
		} else {
			fmt.Printf("  ✅ Transaction %d queued (partition down)\n", i+2)
		}
	}
	
	// Check metrics
	metrics := conductor.GetMetrics()
	fmt.Printf("\nMetrics:\n")
	fmt.Printf("  Sent: %d\n", metrics["total_sent"])
	fmt.Printf("  Failed: %d\n", metrics["total_failed"])
	fmt.Printf("  Queued: %d\n", metrics["total_queued"])
	fmt.Printf("  Pending: %d\n", metrics["total_pending"])
}

func testCircuitBreaker() {
	fmt.Println("Test 2: Circuit Breaker")
	fmt.Println("-----------------------")
	
	// dispatcher := NewMockDispatcher() // Not used in this test
	logger := logging.NewTestLogger(nil, "error", false)
	monitor := NewPartitionHealthMonitor(logger)
	
	partitions := []string{"BVN0", "BVN1", "BVN2"}
	monitor.Start(partitions)
	
	// Simulate failures to trigger circuit breaker
	fmt.Println("  Simulating failures to open circuit...")
	for i := 0; i < 4; i++ {
		monitor.RecordFailure("BVN1", fmt.Errorf("connection refused"))
	}
	
	// Check if circuit is open
	canSend, _ := monitor.CanSendToPartition("BVN1")
	if !canSend {
		fmt.Printf("  ✅ Circuit breaker opened after failures\n")
	} else {
		fmt.Printf("  ❌ Circuit breaker didn't open\n")
	}
	
	// Get status
	status, _ := monitor.GetPartitionStatus("BVN1")
	fmt.Printf("  Partition State: %s\n", status.State)
	fmt.Printf("  Circuit State: %s\n", status.CircuitState)
	fmt.Printf("  Consecutive Fails: %d\n", status.ConsecutiveFails)
	
	// Wait and check half-open transition
	fmt.Println("\n  Waiting for half-open transition...")
	time.Sleep(31 * time.Second)
	
	canSend, _ = monitor.CanSendToPartition("BVN1")
	status, _ = monitor.GetPartitionStatus("BVN1")
	
	if status.CircuitState == CircuitHalfOpen {
		fmt.Printf("  ✅ Circuit transitioned to half-open\n")
	} else {
		fmt.Printf("  ❌ Circuit didn't transition to half-open\n")
	}
}

func testPartitionRecovery() {
	fmt.Println("Test 3: Partition Recovery")
	fmt.Println("--------------------------")
	
	dispatcher := NewMockDispatcher()
	logger := logging.NewTestLogger(nil, "error", false)
	conductor := NewEnhancedCrossChainConductor(dispatcher, logger)
	
	partitions := []string{"BVN0", "BVN1", "BVN2"}
	conductor.Start(partitions)
	defer conductor.Stop()
	
	// Mark BVN1 as down
	dispatcher.SetPartitionState("BVN1", false)
	
	// Queue some transactions
	ctx := context.Background()
	dest := protocol.PartitionUrl("BVN1")
	msg := &messaging.TransactionMessage{}
	
	fmt.Println("  Queueing transactions while partition is down...")
	for i := 1; i <= 5; i++ {
		conductor.SubmitTransaction(ctx, msg, dest, uint64(i))
	}
	
	metrics := conductor.GetMetrics()
	fmt.Printf("  Queued: %d transactions\n", metrics["total_queued"])
	
	// Bring partition back up
	fmt.Println("\n  Bringing partition back online...")
	dispatcher.SetPartitionState("BVN1", true)
	
	// Simulate recovery by recording success
	conductor.healthMonitor.RecordSuccess("BVN1", 5)
	
	// Wait for queue to drain
	time.Sleep(2 * time.Second)
	
	// Check if queue drained
	metrics = conductor.GetMetrics()
	fmt.Printf("  After recovery:\n")
	fmt.Printf("    Sent: %d\n", metrics["total_sent"])
	fmt.Printf("    Pending: %d\n", metrics["total_pending"])
	
	if metrics["total_pending"].(int) == 0 {
		fmt.Printf("  ✅ Queue successfully drained after recovery\n")
	} else {
		fmt.Printf("  ⚠️ Queue not fully drained\n")
	}
}

func testOutOfOrderHandling() {
	fmt.Println("Test 4: Out-of-Order Sequence Handling")
	fmt.Println("--------------------------------------")
	
	dispatcher := NewMockDispatcher()
	logger := logging.NewTestLogger(nil, "error", false)
	conductor := NewEnhancedCrossChainConductor(dispatcher, logger)
	
	partitions := []string{"BVN0", "BVN1", "BVN2"}
	conductor.Start(partitions)
	defer conductor.Stop()
	
	// Simulate out-of-order sequence
	fmt.Println("  Simulating out-of-order sequence...")
	
	// Partition sends sequence 150 when we expect 100
	err := conductor.HandleOutOfOrderSequence("BVN1", 150, 100)
	if err != nil {
		fmt.Printf("  ❌ Failed to handle out-of-order: %v\n", err)
	} else {
		fmt.Printf("  ✅ Out-of-order handled, catch-up initiated\n")
	}
	
	// Partition is behind - needs sequences 50-99
	fmt.Println("\n  Partition requesting missing sequences...")
	err = conductor.HandleOutOfOrderSequence("BVN2", 50, 100)
	if err != nil {
		fmt.Printf("  ❌ Failed to handle catch-up request: %v\n", err)
	} else {
		fmt.Printf("  ✅ Catch-up transactions being sent\n")
	}
}

func testQueueLimits() {
	fmt.Println("Test 5: Queue Limits")
	fmt.Println("--------------------")
	
	logger := logging.NewTestLogger(nil, "error", false)
	monitor := NewPartitionHealthMonitor(logger)
	monitor.maxQueueSize = 10 // Set small limit for testing
	
	partitions := []string{"BVN0"}
	monitor.Start(partitions)
	
	// Mark partition as down
	monitor.RegisterPartition("BVN0")
	monitor.RecordFailure("BVN0", fmt.Errorf("down"))
	monitor.RecordFailure("BVN0", fmt.Errorf("down"))
	monitor.RecordFailure("BVN0", fmt.Errorf("down"))
	
	// Try to queue more than limit
	fmt.Printf("  Queueing transactions (limit: %d)...\n", monitor.maxQueueSize)
	
	successCount := 0
	failCount := 0
	
	for i := 1; i <= 15; i++ {
		tx := &PendingTransaction{
			ID:          fmt.Sprintf("tx-%d", i),
			SequenceNum: uint64(i),
			Timestamp:   time.Now(),
		}
		
		err := monitor.QueueTransaction("BVN0", tx)
		if err != nil {
			failCount++
		} else {
			successCount++
		}
	}
	
	fmt.Printf("  Queued: %d\n", successCount)
	fmt.Printf("  Rejected: %d\n", failCount)
	
	if failCount > 0 {
		fmt.Printf("  ✅ Queue limit enforced correctly\n")
	} else {
		fmt.Printf("  ❌ Queue limit not enforced\n")
	}
	
	// Check queue size
	status, _ := monitor.GetPartitionStatus("BVN0")
	fmt.Printf("  Final queue size: %d\n", len(status.PendingQueue))
}

// Performance test
func testPerformanceUnderFailure() {
	fmt.Println("\nTest 6: Performance Under Failure")
	fmt.Println("---------------------------------")
	
	dispatcher := NewMockDispatcher()
	logger := logging.NewTestLogger(nil, "error", false)
	conductor := NewEnhancedCrossChainConductor(dispatcher, logger)
	
	partitions := []string{"BVN0", "BVN1", "BVN2", "Directory"}
	conductor.Start(partitions)
	defer conductor.Stop()
	
	// Set mixed partition states
	dispatcher.SetPartitionState("BVN0", true)
	dispatcher.SetPartitionState("BVN1", false) // Down
	dispatcher.SetPartitionState("BVN2", true)
	dispatcher.SetPartitionState("Directory", true)
	
	// Send many transactions concurrently
	var wg sync.WaitGroup
	startTime := time.Now()
	totalTx := int64(0)
	
	for _, partition := range partitions {
		for i := 0; i < 25; i++ {
			wg.Add(1)
			go func(part string, seq int) {
				defer wg.Done()
				
				ctx := context.Background()
				dest := protocol.PartitionUrl(part)
				msg := &messaging.TransactionMessage{}
				
				err := conductor.SubmitTransaction(ctx, msg, dest, uint64(seq))
				if err == nil {
					atomic.AddInt64(&totalTx, 1)
				}
			}(partition, i)
		}
	}
	
	wg.Wait()
	duration := time.Since(startTime)
	
	metrics := conductor.GetMetrics()
	
	fmt.Printf("  Duration: %v\n", duration)
	fmt.Printf("  Total Attempts: 100\n")
	fmt.Printf("  Successful: %d\n", totalTx)
	fmt.Printf("  Sent: %d\n", metrics["total_sent"])
	fmt.Printf("  Queued: %d\n", metrics["total_queued"])
	fmt.Printf("  Failed: %d\n", metrics["total_failed"])
	fmt.Printf("  TPS: %.2f\n", float64(totalTx)/duration.Seconds())
	
	if metrics["partitions_down"].(int) == 1 {
		fmt.Printf("  ✅ Correctly identified 1 down partition\n")
	}
}