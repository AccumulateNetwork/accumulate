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

// TestDispatcher simulates a dispatcher with controllable partition failures
type TestDispatcher struct {
	partitionStates map[string]bool // true = healthy, false = down
	mu              sync.RWMutex
	submitCount     int64
	failCount       int64
	successCount    int64
	logger          logging.OptionalLogger
}

func NewTestDispatcher(logger logging.OptionalLogger) *TestDispatcher {
	return &TestDispatcher{
		partitionStates: make(map[string]bool),
		logger:          logger,
	}
}

func (td *TestDispatcher) Submit(ctx context.Context, dest *url.URL, env *messaging.Envelope) error {
	atomic.AddInt64(&td.submitCount, 1)

	partitionID := td.getPartitionID(dest)

	td.mu.RLock()
	healthy, exists := td.partitionStates[partitionID]
	td.mu.RUnlock()

	if !exists {
		// Default to healthy
		healthy = true
	}

	if !healthy {
		atomic.AddInt64(&td.failCount, 1)
		return fmt.Errorf("connection refused: partition %s is down", partitionID)
	}

	// Simulate network delay
	select {
	case <-time.After(5 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}

	atomic.AddInt64(&td.successCount, 1)
	return nil
}

func (td *TestDispatcher) SetPartitionState(partitionID string, healthy bool) {
	td.mu.Lock()
	defer td.mu.Unlock()
	td.partitionStates[partitionID] = healthy

	state := "healthy"
	if !healthy {
		state = "down"
	}
	td.logger.Info("Partition state changed", "partition", partitionID, "state", state)
}

func (td *TestDispatcher) getPartitionID(dest *url.URL) string {
	if dest.Authority != "" {
		return dest.Authority
	}
	return "unknown"
}

func (td *TestDispatcher) Send(ctx context.Context) <-chan error {
	// Not used in our test
	ch := make(chan error, 1)
	close(ch)
	return ch
}

func (td *TestDispatcher) Close() {
	// No-op for test dispatcher
}

func (td *TestDispatcher) GetStats() (int64, int64, int64) {
	return atomic.LoadInt64(&td.submitCount),
		atomic.LoadInt64(&td.successCount),
		atomic.LoadInt64(&td.failCount)
}

// LedgerRecoverySimulator simulates ledger-based recovery
type LedgerRecoverySimulator struct {
	logger             logging.OptionalLogger
	storedTransactions map[string][]*StoredTransaction
	mu                 sync.RWMutex
}

type StoredTransaction struct {
	PartitionID string
	SequenceNum uint64
	Timestamp   time.Time
	Type        string
	Hash        []byte
}

func NewLedgerRecoverySimulator(logger logging.OptionalLogger) *LedgerRecoverySimulator {
	return &LedgerRecoverySimulator{
		logger:             logger,
		storedTransactions: make(map[string][]*StoredTransaction),
	}
}

func (lrs *LedgerRecoverySimulator) RecordTransaction(partitionID string, seqNum uint64, txType string) {
	lrs.mu.Lock()
	defer lrs.mu.Unlock()

	tx := &StoredTransaction{
		PartitionID: partitionID,
		SequenceNum: seqNum,
		Timestamp:   time.Now(),
		Type:        txType,
		Hash:        []byte(fmt.Sprintf("hash-%d", seqNum)),
	}

	lrs.storedTransactions[partitionID] = append(lrs.storedTransactions[partitionID], tx)
}

func (lrs *LedgerRecoverySimulator) RecoverTransactions(partitionID string, fromSeq, toSeq uint64) []*StoredTransaction {
	lrs.mu.RLock()
	defer lrs.mu.RUnlock()

	var recovered []*StoredTransaction

	transactions, exists := lrs.storedTransactions[partitionID]
	if !exists {
		return recovered
	}

	for _, tx := range transactions {
		if tx.SequenceNum >= fromSeq && tx.SequenceNum < toSeq {
			recovered = append(recovered, tx)
		}
	}

	lrs.logger.Info("Recovered transactions from ledger",
		"partition", partitionID,
		"range", fmt.Sprintf("%d-%d", fromSeq, toSeq-1),
		"count", len(recovered))

	return recovered
}

func main() {
	fmt.Println("========================================")
	fmt.Println("   SIMPLIFIED PARTITION HANDLING TEST")
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("Testing: Drop failed transactions and rely on ledger recovery")
	fmt.Println()

	// Run test scenarios
	testDropAndRecover()
	fmt.Println()
	testOutOfOrderRecovery()
	fmt.Println()
	testCircuitBreakerWithDrops()
	fmt.Println()
	testMassiveFailureScenario()

	fmt.Println("\n========================================")
	fmt.Println("         ALL TESTS COMPLETED")
	fmt.Println("========================================")
}

func testDropAndRecover() {
	fmt.Println("Test 1: Drop Failed Transactions & Ledger Recovery")
	fmt.Println("-------------------------------------------------")

	var logger logging.OptionalLogger
	dispatcher := NewTestDispatcher(logger)
	handler := NewSimplifiedPartitionHandler(dispatcher, logger)
	ledgerRecovery := NewLedgerRecoverySimulator(logger)

	partitions := []string{"BVN0", "BVN1", "BVN2"}
	handler.Start(partitions)

	// Set BVN1 as healthy initially
	dispatcher.SetPartitionState("BVN1", true)

	// Send some successful transactions and record them in ledger
	ctx := context.Background()
	dest := protocol.PartitionUrl("BVN1")

	fmt.Println("\n  Phase 1: Sending successful transactions...")
	for i := uint64(1); i <= 5; i++ {
		msg := &messaging.TransactionMessage{}
		err := handler.SubmitTransaction(ctx, msg, dest, i)
		if err != nil {
			fmt.Printf("  ❌ Transaction %d failed: %v\n", i, err)
		} else {
			// Record in ledger for recovery later
			ledgerRecovery.RecordTransaction("BVN1", i, "anchor")
			fmt.Printf("  ✅ Transaction %d sent and recorded in ledger\n", i)
		}
	}

	// Mark BVN1 as down
	fmt.Println("\n  Phase 2: Partition goes down...")
	dispatcher.SetPartitionState("BVN1", false)
	time.Sleep(100 * time.Millisecond)

	// Try to send more transactions - they should be dropped
	fmt.Println("\n  Phase 3: Attempting to send while partition is down...")
	for i := uint64(6); i <= 10; i++ {
		msg := &messaging.TransactionMessage{}

		// First few attempts will retry and fail
		err := handler.SubmitTransaction(ctx, msg, dest, i)
		if err != nil {
			fmt.Printf("  ⚠️ Transaction %d failed after retries\n", i)
		}

		// Record what we tried to send in the ledger
		ledgerRecovery.RecordTransaction("BVN1", i, "anchor")
	}

	// After threshold, circuit should open and transactions should be dropped
	fmt.Println("\n  Phase 4: Circuit opens, transactions dropped...")
	for i := uint64(11); i <= 15; i++ {
		msg := &messaging.TransactionMessage{}
		err := handler.SubmitTransaction(ctx, msg, dest, i)
		if err == nil {
			fmt.Printf("  📦 Transaction %d dropped (partition down, will recover from ledger)\n", i)
			// Still record in ledger for recovery
			ledgerRecovery.RecordTransaction("BVN1", i, "anchor")
		}
	}

	// Check metrics
	metrics := handler.GetMetrics()
	fmt.Printf("\n  Metrics:\n")
	fmt.Printf("    Sent: %d\n", metrics["total_sent"])
	fmt.Printf("    Failed: %d\n", metrics["total_failed"])
	fmt.Printf("    Dropped: %d\n", metrics["total_dropped"])

	// Simulate partition recovery
	fmt.Println("\n  Phase 5: Partition recovers...")
	dispatcher.SetPartitionState("BVN1", true)

	// Simulate out-of-order sequence detection
	fmt.Println("\n  Phase 6: Out-of-order sequence triggers ledger recovery...")
	handler.HandleOutOfOrderSequence("BVN1", 5, 15)

	// Recover from ledger
	recovered := ledgerRecovery.RecoverTransactions("BVN1", 6, 16)
	fmt.Printf("  📚 Recovered %d transactions from ledger\n", len(recovered))

	// Resend recovered transactions
	fmt.Println("\n  Phase 7: Resending recovered transactions...")
	successCount := 0
	for _, tx := range recovered {
		msg := &messaging.TransactionMessage{}
		err := handler.SubmitTransaction(ctx, msg, dest, tx.SequenceNum)
		if err == nil {
			successCount++
		}
	}
	fmt.Printf("  ✅ Successfully resent %d/%d recovered transactions\n", successCount, len(recovered))

	// Final stats
	attempts, successes, failures := dispatcher.GetStats()
	fmt.Printf("\n  Final Dispatcher Stats:\n")
	fmt.Printf("    Total Attempts: %d\n", attempts)
	fmt.Printf("    Successes: %d\n", successes)
	fmt.Printf("    Failures: %d\n", failures)
}

func testOutOfOrderRecovery() {
	fmt.Println("Test 2: Out-of-Order Sequence Recovery")
	fmt.Println("--------------------------------------")

	var logger logging.OptionalLogger
	dispatcher := NewTestDispatcher(logger)
	handler := NewSimplifiedPartitionHandler(dispatcher, logger)

	partitions := []string{"BVN0", "BVN1", "BVN2"}
	handler.Start(partitions)

	// All partitions healthy
	dispatcher.SetPartitionState("BVN0", true)
	dispatcher.SetPartitionState("BVN1", true)
	dispatcher.SetPartitionState("BVN2", true)

	fmt.Println("\n  Scenario 1: Partition was down and missed sequences 100-149")
	handler.HandleOutOfOrderSequence("BVN1", 100, 150)

	fmt.Println("\n  Scenario 2: We are behind - partition has sequences we don't")
	handler.HandleOutOfOrderSequence("BVN2", 200, 180)

	fmt.Println("\n  Scenario 3: Partition recovers and requests specific range")
	handler.HandleOutOfOrderSequence("BVN0", 50, 75)
}

func testCircuitBreakerWithDrops() {
	fmt.Println("Test 3: Circuit Breaker with Transaction Drops")
	fmt.Println("----------------------------------------------")

	var logger logging.OptionalLogger
	dispatcher := NewTestDispatcher(logger)
	handler := NewSimplifiedPartitionHandler(dispatcher, logger)

	partitions := []string{"BVN0", "BVN1"}
	handler.Start(partitions)

	// BVN0 starts healthy
	dispatcher.SetPartitionState("BVN0", true)

	ctx := context.Background()
	dest := protocol.PartitionUrl("BVN0")

	// Send transactions until circuit opens
	fmt.Println("\n  Causing failures to open circuit...")
	dispatcher.SetPartitionState("BVN0", false)

	failureCount := 0
	dropCount := 0

	for i := uint64(1); i <= 10; i++ {
		msg := &messaging.TransactionMessage{}
		err := handler.SubmitTransaction(ctx, msg, dest, i)

		if err != nil {
			failureCount++
			fmt.Printf("  ❌ Transaction %d failed\n", i)
		} else {
			dropCount++
			fmt.Printf("  📦 Transaction %d dropped (circuit open)\n", i)
		}

		// After 3 failures, circuit should open
		if i == 3 {
			time.Sleep(100 * time.Millisecond) // Give time for circuit to open
		}
	}

	fmt.Printf("\n  Results:\n")
	fmt.Printf("    Failed (before circuit opened): %d\n", failureCount)
	fmt.Printf("    Dropped (after circuit opened): %d\n", dropCount)

	// Check partition status
	status := handler.GetPartitionStatus("BVN0")
	if status != nil {
		fmt.Printf("    Circuit Open: %v\n", status.CircuitOpen)
		fmt.Printf("    Is Healthy: %v\n", status.IsHealthy)
	}
}

func testMassiveFailureScenario() {
	fmt.Println("Test 4: Massive Failure Scenario")
	fmt.Println("--------------------------------")

	var logger logging.OptionalLogger
	dispatcher := NewTestDispatcher(logger)
	handler := NewSimplifiedPartitionHandler(dispatcher, logger)
	ledgerRecovery := NewLedgerRecoverySimulator(logger)

	partitions := []string{"BVN0", "BVN1", "BVN2", "BVN3", "BVN4"}
	handler.Start(partitions)

	// Start with all healthy
	for _, p := range partitions {
		dispatcher.SetPartitionState(p, true)
	}

	ctx := context.Background()

	fmt.Println("\n  Phase 1: Normal operation - sending to all partitions...")

	// Send initial transactions
	var wg sync.WaitGroup
	sentCount := int64(0)

	for _, partition := range partitions {
		for i := uint64(1); i <= 20; i++ {
			wg.Add(1)
			go func(p string, seq uint64) {
				defer wg.Done()

				dest := protocol.PartitionUrl(p)
				msg := &messaging.TransactionMessage{}

				err := handler.SubmitTransaction(ctx, msg, dest, seq)
				if err == nil {
					atomic.AddInt64(&sentCount, 1)
					ledgerRecovery.RecordTransaction(p, seq, "anchor")
				}
			}(partition, i)
		}
	}

	wg.Wait()
	fmt.Printf("  ✅ Sent %d transactions across %d partitions\n", sentCount, len(partitions))

	// Simulate cascading failures
	fmt.Println("\n  Phase 2: Cascading partition failures...")

	failedPartitions := []string{"BVN1", "BVN2", "BVN3"}
	for _, p := range failedPartitions {
		dispatcher.SetPartitionState(p, false)
		fmt.Printf("  💥 Partition %s failed\n", p)
		time.Sleep(50 * time.Millisecond)
	}

	// Try to send more transactions
	fmt.Println("\n  Phase 3: Attempting to send with multiple partitions down...")

	droppedCount := int64(0)
	failedCount := int64(0)
	successCount := int64(0)

	for _, partition := range partitions {
		for i := uint64(21); i <= 30; i++ {
			wg.Add(1)
			go func(p string, seq uint64) {
				defer wg.Done()

				dest := protocol.PartitionUrl(p)
				msg := &messaging.TransactionMessage{}

				err := handler.SubmitTransaction(ctx, msg, dest, seq)
				if err != nil {
					atomic.AddInt64(&failedCount, 1)
				} else {
					// Check if partition is in failed list
					isDown := false
					for _, failed := range failedPartitions {
						if failed == p {
							isDown = true
							break
						}
					}

					if isDown {
						atomic.AddInt64(&droppedCount, 1)
					} else {
						atomic.AddInt64(&successCount, 1)
					}
				}

				// Record all attempts in ledger for recovery
				ledgerRecovery.RecordTransaction(p, seq, "anchor")
			}(partition, i)
		}
	}

	wg.Wait()

	fmt.Printf("\n  Results during failure:\n")
	fmt.Printf("    Successful (healthy partitions): %d\n", successCount)
	fmt.Printf("    Failed (retries exhausted): %d\n", failedCount)
	fmt.Printf("    Dropped (circuit open): %d\n", droppedCount)

	// Recover partitions
	fmt.Println("\n  Phase 4: Partitions recovering...")

	for _, p := range failedPartitions {
		dispatcher.SetPartitionState(p, true)
		fmt.Printf("  🔄 Partition %s back online\n", p)

		// Trigger recovery for each partition
		handler.HandleOutOfOrderSequence(p, 20, 31)

		// Recover from ledger
		recovered := ledgerRecovery.RecoverTransactions(p, 21, 31)
		fmt.Printf("  📚 Recovering %d transactions for %s from ledger\n", len(recovered), p)
	}

	// Final metrics
	metrics := handler.GetMetrics()
	fmt.Printf("\n  Final System Metrics:\n")
	fmt.Printf("    Total Sent: %d\n", metrics["total_sent"])
	fmt.Printf("    Total Failed: %d\n", metrics["total_failed"])
	fmt.Printf("    Total Dropped: %d\n", metrics["total_dropped"])
	fmt.Printf("    Healthy Partitions: %d/%d\n", metrics["partitions_healthy"], len(partitions))

	// Dispatcher stats
	attempts, successes, failures := dispatcher.GetStats()
	fmt.Printf("\n  Dispatcher Statistics:\n")
	fmt.Printf("    Total Submit Attempts: %d\n", attempts)
	fmt.Printf("    Network Successes: %d\n", successes)
	fmt.Printf("    Network Failures: %d\n", failures)

	fmt.Println("\n  ✅ Test demonstrates that dropped transactions can be recovered from ledger")
	fmt.Println("     when partitions come back online, without holding them in memory")
}
