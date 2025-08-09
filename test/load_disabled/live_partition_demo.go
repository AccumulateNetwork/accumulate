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

// LiveDemo runs a live demonstration of partition failure handling
func main() {
	fmt.Println("================================================================================")
	fmt.Println("              LIVE PARTITION FAILURE HANDLING DEMONSTRATION")
	fmt.Println("================================================================================")
	fmt.Println()
	fmt.Println("This demo will:")
	fmt.Println("  1. Send continuous transactions to all partitions")
	fmt.Println("  2. Show real-time metrics every 2 seconds")
	fmt.Println("  3. Demonstrate dropping when partitions fail")
	fmt.Println("  4. Show recovery when partitions come back")
	fmt.Println()
	fmt.Println("INSTRUCTIONS:")
	fmt.Println("  - Watch the metrics update in real-time")
	fmt.Println("  - In another terminal, manipulate partitions:")
	fmt.Println("    ./partition_manager.sh stop BVN1")
	fmt.Println("    ./partition_manager.sh start BVN1")
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop the demo")
	fmt.Println("================================================================================")
	fmt.Println()

	// Initialize components
	var logger logging.OptionalLogger
	dispatcher := NewLiveDispatcher()
	handler := NewSimplifiedPartitionHandler(dispatcher, logger)

	partitions := []string{"BVN0", "BVN1", "BVN2", "Directory"}
	handler.Start(partitions)

	// Start all partitions as healthy
	for _, p := range partitions {
		dispatcher.SetPartitionHealth(p, true)
	}

	// Metrics tracking
	var (
		totalSent   int64
		totalFailed int64
		startTime   = time.Now()
	)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C
	go func() {
		<-make(chan struct{})
		cancel()
	}()

	// Start transaction senders
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			seqNum := uint64(workerID * 1000)                // Avoid sequence conflicts
			ticker := time.NewTicker(250 * time.Millisecond) // 4 tx/sec per worker
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					// Send to a random partition
					partition := partitions[seqNum%uint64(len(partitions))]
					dest := protocol.PartitionUrl(partition)
					msg := &messaging.TransactionMessage{}

					err := handler.SubmitTransaction(ctx, msg, dest, seqNum)
					if err != nil {
						atomic.AddInt64(&totalFailed, 1)
					} else {
						atomic.AddInt64(&totalSent, 1)
					}

					seqNum++
				}
			}
		}(i)
	}

	// Metrics reporter
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		lastSent := int64(0)
		lastTime := time.Now()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Calculate rates
				now := time.Now()
				duration := now.Sub(lastTime).Seconds()
				currentSent := atomic.LoadInt64(&totalSent)

				sentDelta := currentSent - lastSent
				tps := float64(sentDelta) / duration

				// Get handler metrics
				metrics := handler.GetMetrics()
				handlerSent := metrics["total_sent"].(int64)
				handlerFailed := metrics["total_failed"].(int64)
				handlerDropped := metrics["total_dropped"].(int64)
				healthyPartitions := metrics["partitions_healthy"].(int)

				// Get dispatcher stats
				dispatcherStats := dispatcher.GetStats()

				// Clear screen and print header
				fmt.Print("\033[H\033[2J") // Clear screen
				fmt.Println("================================================================================")
				fmt.Println("                           LIVE METRICS DASHBOARD")
				fmt.Println("================================================================================")
				fmt.Printf("Running for: %s\n", time.Since(startTime).Round(time.Second))
				fmt.Println()

				// Partition health status
				fmt.Println("🔹 PARTITION STATUS")
				fmt.Println("────────────────────────────────────────────────────────────────────────────────")
				for _, p := range partitions {
					if dispatcher.IsHealthy(p) {
						fmt.Printf("  ✅ %-12s HEALTHY\n", p)
					} else {
						fmt.Printf("  ❌ %-12s DOWN (transactions being dropped)\n", p)
					}
				}
				fmt.Printf("\n  Healthy: %d/%d partitions\n", healthyPartitions, len(partitions))

				// Transaction metrics
				fmt.Println("\n🔹 TRANSACTION METRICS")
				fmt.Println("────────────────────────────────────────────────────────────────────────────────")
				fmt.Printf("  Current TPS:      %.1f transactions/sec\n", tps)
				fmt.Printf("  Total Sent:       %d\n", handlerSent)
				fmt.Printf("  Total Failed:     %d\n", handlerFailed)
				fmt.Printf("  Total Dropped:    %d (will be recovered from ledger)\n", handlerDropped)

				if total := handlerSent + handlerFailed + handlerDropped; total > 0 {
					successRate := float64(handlerSent) / float64(total) * 100
					fmt.Printf("  Success Rate:     %.1f%%\n", successRate)
				}

				// Network simulation stats
				fmt.Println("\n🔹 NETWORK SIMULATION")
				fmt.Println("────────────────────────────────────────────────────────────────────────────────")
				fmt.Printf("  Submit Attempts:  %d\n", dispatcherStats.Attempts)
				fmt.Printf("  Network Success:  %d\n", dispatcherStats.Successes)
				fmt.Printf("  Network Failures: %d\n", dispatcherStats.Failures)

				// Instructions
				fmt.Println("\n🔹 CONTROL INSTRUCTIONS")
				fmt.Println("────────────────────────────────────────────────────────────────────────────────")
				fmt.Println("  In another terminal, try:")
				fmt.Println("    ./partition_manager.sh stop BVN1    # Stop a partition")
				fmt.Println("    ./partition_manager.sh start BVN1   # Restart it")
				fmt.Println("    ./partition_manager.sh fail BVN2    # Simulate failure")
				fmt.Println()
				fmt.Println("  Press Ctrl+C to stop the demo")

				// Update for next iteration
				lastSent = currentSent
				lastTime = now
			}
		}
	}()

	// Simulate some automatic failures for demo
	go func() {
		time.Sleep(10 * time.Second)
		fmt.Println("\n🔥 AUTO-DEMO: Simulating BVN1 failure...")
		dispatcher.SetPartitionHealth("BVN1", false)

		time.Sleep(15 * time.Second)
		fmt.Println("\n🔧 AUTO-DEMO: Recovering BVN1...")
		dispatcher.SetPartitionHealth("BVN1", true)
		handler.HandleOutOfOrderSequence("BVN1", 100, 200) // Trigger recovery

		time.Sleep(10 * time.Second)
		fmt.Println("\n💥 AUTO-DEMO: Cascading failure - BVN2 and Directory down...")
		dispatcher.SetPartitionHealth("BVN2", false)
		dispatcher.SetPartitionHealth("Directory", false)

		time.Sleep(15 * time.Second)
		fmt.Println("\n🔄 AUTO-DEMO: Recovering all partitions...")
		dispatcher.SetPartitionHealth("BVN2", true)
		dispatcher.SetPartitionHealth("Directory", true)
	}()

	// Wait for shutdown
	wg.Wait()

	// Print final summary
	fmt.Println("\n================================================================================")
	fmt.Println("                              DEMO COMPLETE")
	fmt.Println("================================================================================")

	metrics := handler.GetMetrics()
	fmt.Printf("\nFinal Statistics:\n")
	fmt.Printf("  Duration:         %s\n", time.Since(startTime).Round(time.Second))
	fmt.Printf("  Total Sent:       %d\n", metrics["total_sent"])
	fmt.Printf("  Total Failed:     %d\n", metrics["total_failed"])
	fmt.Printf("  Total Dropped:    %d\n", metrics["total_dropped"])

	if dropped := metrics["total_dropped"].(int64); dropped > 0 {
		fmt.Printf("\n✅ Successfully demonstrated partition failure handling!\n")
		fmt.Printf("   %d transactions were dropped and would be recovered from ledger\n", dropped)
	}
}

// LiveDispatcher simulates network with controllable partitions
type LiveDispatcher struct {
	partitionHealth map[string]bool
	mu              sync.RWMutex
	attempts        int64
	successes       int64
	failures        int64
}

func NewLiveDispatcher() *LiveDispatcher {
	return &LiveDispatcher{
		partitionHealth: make(map[string]bool),
	}
}

func (ld *LiveDispatcher) Submit(ctx context.Context, dest *url.URL, env *messaging.Envelope) error {
	atomic.AddInt64(&ld.attempts, 1)

	partition := getPartition(dest)

	ld.mu.RLock()
	healthy := ld.partitionHealth[partition]
	ld.mu.RUnlock()

	// Simulate network delay
	select {
	case <-time.After(5 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}

	if !healthy {
		atomic.AddInt64(&ld.failures, 1)
		return fmt.Errorf("partition %s is down", partition)
	}

	atomic.AddInt64(&ld.successes, 1)
	return nil
}

func (ld *LiveDispatcher) Send(ctx context.Context) <-chan error {
	ch := make(chan error, 1)
	close(ch)
	return ch
}

func (ld *LiveDispatcher) Close() {}

func (ld *LiveDispatcher) SetPartitionHealth(partition string, healthy bool) {
	ld.mu.Lock()
	defer ld.mu.Unlock()
	ld.partitionHealth[partition] = healthy
}

func (ld *LiveDispatcher) IsHealthy(partition string) bool {
	ld.mu.RLock()
	defer ld.mu.RUnlock()
	return ld.partitionHealth[partition]
}

type DispatcherStats struct {
	Attempts  int64
	Successes int64
	Failures  int64
}

func (ld *LiveDispatcher) GetStats() DispatcherStats {
	return DispatcherStats{
		Attempts:  atomic.LoadInt64(&ld.attempts),
		Successes: atomic.LoadInt64(&ld.successes),
		Failures:  atomic.LoadInt64(&ld.failures),
	}
}

func getPartition(dest *url.URL) string {
	if dest.Authority != "" {
		return dest.Authority
	}
	return "unknown"
}
