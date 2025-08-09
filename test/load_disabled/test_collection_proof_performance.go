package main

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// CollectionProofPerformanceTest demonstrates the efficiency gains
func main() {
	fmt.Println("================================================================================")
	fmt.Println("               COLLECTION PROOF PERFORMANCE COMPARISON")
	fmt.Println("================================================================================")
	fmt.Println()

	// Run performance comparison tests
	runPerformanceComparison()
}

func runPerformanceComparison() {
	scenarios := []struct {
		name           string
		missingTxCount int
		iterations     int
	}{
		{"Small Gap", 5, 100},
		{"Medium Gap", 25, 50},
		{"Large Gap", 100, 20},
		{"Massive Gap", 500, 5},
	}

	fmt.Println("Performance Comparison: Individual Proofs vs Collection Proofs")
	fmt.Println("─────────────────────────────────────────────────────────────────────────────")
	fmt.Printf("%-15s %-8s %-12s %-12s %-12s %-15s\n",
		"Scenario", "Count", "Individual", "Collection", "Speedup", "Proof Savings")
	fmt.Println("─────────────────────────────────────────────────────────────────────────────")

	totalSpeedup := float64(0)
	totalSavings := int64(0)

	for _, scenario := range scenarios {
		result := runScenarioComparison(scenario.name, scenario.missingTxCount, scenario.iterations)

		speedup := float64(result.IndividualTime.Nanoseconds()) / float64(result.CollectionTime.Nanoseconds())
		proofSavings := int64(scenario.missingTxCount-1) * int64(scenario.iterations)

		fmt.Printf("%-15s %-8d %-12s %-12s %-8.1fx %-15d\n",
			scenario.name,
			scenario.missingTxCount,
			result.IndividualTime.Round(time.Millisecond),
			result.CollectionTime.Round(time.Millisecond),
			speedup,
			proofSavings)

		totalSpeedup += speedup
		totalSavings += proofSavings
	}

	avgSpeedup := totalSpeedup / float64(len(scenarios))

	fmt.Println("─────────────────────────────────────────────────────────────────────────────")
	fmt.Printf("Average Speedup: %.1fx | Total Proof Savings: %d\n", avgSpeedup, totalSavings)
	fmt.Println()

	// Run detailed analysis
	runDetailedAnalysis()
}

type PerformanceResult struct {
	IndividualTime time.Duration
	CollectionTime time.Duration
	ProofsSaved    int
}

func runScenarioComparison(name string, missingCount, iterations int) PerformanceResult {
	var result PerformanceResult

	// Simulate individual proof recovery
	start := time.Now()
	for i := 0; i < iterations; i++ {
		simulateIndividualProofRecovery(missingCount)
	}
	result.IndividualTime = time.Since(start)

	// Simulate collection proof recovery
	start = time.Now()
	for i := 0; i < iterations; i++ {
		simulateCollectionProofRecovery(missingCount)
	}
	result.CollectionTime = time.Since(start)

	result.ProofsSaved = (missingCount - 1) * iterations

	return result
}

func simulateIndividualProofRecovery(missingCount int) {
	// Simulate the time to generate individual Merkle proofs
	for i := 0; i < missingCount; i++ {
		// Each transaction needs:
		// 1. Individual Merkle proof generation (expensive)
		// 2. Transaction data retrieval
		// 3. Network transmission of proof + data

		simulateMerkleProofGeneration()   // ~1ms per proof
		simulateTransactionRetrieval()    // ~0.5ms per transaction
		simulateNetworkTransmission(true) // Individual proof is larger
	}
}

func simulateCollectionProofRecovery(missingCount int) {
	// Simulate the time to generate a single collection proof
	// 1. Single collection proof generation (one-time cost)
	// 2. Batch transaction data retrieval
	// 3. Network transmission of collection proof + all data

	simulateCollectionProofGeneration(missingCount) // ~2ms regardless of count
	simulateBatchTransactionRetrieval(missingCount) // Batch retrieval is faster
	simulateNetworkTransmission(false)              // Single proof transmission
}

func simulateMerkleProofGeneration() {
	// Simulate Merkle proof computation
	time.Sleep(time.Duration(rand.Intn(200)+800) * time.Microsecond) // 0.8-1.0ms
}

func simulateCollectionProofGeneration(count int) {
	// Collection proof generation is roughly constant time
	// regardless of the number of elements (within reason)
	baseTime := 1500 + rand.Intn(1000)          // 1.5-2.5ms base
	scalingTime := count * (10 + rand.Intn(20)) // Small scaling factor
	time.Sleep(time.Duration(baseTime+scalingTime) * time.Microsecond)
}

func simulateTransactionRetrieval() {
	// Simulate database/ledger access
	time.Sleep(time.Duration(rand.Intn(300)+200) * time.Microsecond) // 0.2-0.5ms
}

func simulateBatchTransactionRetrieval(count int) {
	// Batch retrieval has better locality and caching
	baseTime := 500 + rand.Intn(300)             // 0.5-0.8ms base
	perItemTime := count * (50 + rand.Intn(100)) // 0.05-0.15ms per item
	time.Sleep(time.Duration(baseTime+perItemTime) * time.Microsecond)
}

func simulateNetworkTransmission(isIndividualProof bool) {
	// Network transmission time (simulated)
	if isIndividualProof {
		time.Sleep(time.Duration(rand.Intn(200)+300) * time.Microsecond) // 0.3-0.5ms
	} else {
		time.Sleep(time.Duration(rand.Intn(100)+200) * time.Microsecond) // 0.2-0.3ms
	}
}

func runDetailedAnalysis() {
	fmt.Println("DETAILED ANALYSIS: Memory and Network Efficiency")
	fmt.Println("─────────────────────────────────────────────────────────────────────────────")

	scenarios := []int{1, 5, 10, 25, 50, 100, 200, 500}

	fmt.Printf("%-8s %-15s %-15s %-15s %-15s\n",
		"Count", "Individual Size", "Collection Size", "Size Savings", "Proof Savings")
	fmt.Println("─────────────────────────────────────────────────────────────────────────────")

	for _, count := range scenarios {
		individual := calculateIndividualProofSize(count)
		collection := calculateCollectionProofSize(count)
		sizeSavings := individual - collection
		proofSavings := count - 1

		fmt.Printf("%-8d %-15s %-15s %-15s %-15d\n",
			count,
			formatBytes(individual),
			formatBytes(collection),
			formatBytes(sizeSavings),
			proofSavings)
	}

	fmt.Println()
	fmt.Println("REAL-WORLD SCENARIO SIMULATION")
	fmt.Println("─────────────────────────────────────────────────────────────────────────────")

	// Simulate realistic recovery scenarios
	simulateRealWorldScenarios()
}

func calculateIndividualProofSize(count int) int {
	// Individual Merkle proof size estimation
	// Each proof contains log2(tree_size) hashes + metadata
	proofSize := 32*20 + 100 // ~20 hashes * 32 bytes + metadata
	return count * proofSize
}

func calculateCollectionProofSize(count int) int {
	// Collection proof size: single ReceiptList structure
	// Contains: merkle state + elements list + single receipt
	merkleStateSize := 1000    // State structure
	elementsSize := count * 32 // Hash per element
	receiptSize := 32*20 + 100 // Single receipt

	return merkleStateSize + elementsSize + receiptSize
}

func formatBytes(bytes int) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	} else {
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	}
}

func simulateRealWorldScenarios() {
	scenarios := []struct {
		name        string
		description string
		count       int
		frequency   time.Duration
	}{
		{
			"Partition Restart",
			"Partition was down for 1 minute at 50 TPS",
			3000,
			time.Minute,
		},
		{
			"Network Partition",
			"Network split for 30 seconds at 20 TPS",
			600,
			30 * time.Second,
		},
		{
			"Maintenance Window",
			"Planned maintenance for 5 minutes at 10 TPS",
			3000,
			5 * time.Minute,
		},
		{
			"Cascading Failure",
			"Multiple partitions down for 2 minutes at 100 TPS",
			12000,
			2 * time.Minute,
		},
	}

	fmt.Printf("%-20s %-12s %-12s %-12s %-15s\n",
		"Scenario", "Count", "Individual", "Collection", "Time Savings")
	fmt.Println("─────────────────────────────────────────────────────────────────────────────")

	totalSavings := time.Duration(0)

	for _, scenario := range scenarios {
		individualTime := simulateScenarioTime(scenario.count, true)
		collectionTime := simulateScenarioTime(scenario.count, false)
		savings := individualTime - collectionTime
		totalSavings += savings

		fmt.Printf("%-20s %-12d %-12s %-12s %-15s\n",
			scenario.name,
			scenario.count,
			individualTime.Round(time.Millisecond),
			collectionTime.Round(time.Millisecond),
			savings.Round(time.Millisecond))
	}

	fmt.Println("─────────────────────────────────────────────────────────────────────────────")
	fmt.Printf("Total Time Savings: %s\n", totalSavings.Round(time.Millisecond))

	// Show additional benefits
	fmt.Println()
	fmt.Println("ADDITIONAL BENEFITS OF COLLECTION PROOFS:")
	fmt.Println("─────────────────────────────────────────────────────────────────────────────")
	fmt.Println("• Reduced CPU usage on source partition (fewer proof generations)")
	fmt.Println("• Lower memory usage (no need to store individual proofs)")
	fmt.Println("• Better network utilization (fewer, larger requests)")
	fmt.Println("• Improved cache locality (batch processing)")
	fmt.Println("• Atomic verification (all-or-nothing proof validation)")
	fmt.Println("• Simplified error handling (single proof to validate)")
}

func simulateScenarioTime(count int, individual bool) time.Duration {
	if individual {
		return time.Duration(count) * (1*time.Millisecond + 500*time.Microsecond)
	} else {
		// Collection proof: constant base time + small per-item scaling
		base := 2 * time.Millisecond
		perItem := time.Duration(count) * 10 * time.Microsecond
		return base + perItem
	}
}

// Advanced test: Concurrent recovery simulation
func runConcurrentRecoveryTest() {
	fmt.Println("\nCONCURRENT RECOVERY SIMULATION")
	fmt.Println("─────────────────────────────────────────────────────────────────────────────")

	// Simulate multiple partitions requesting recovery simultaneously
	partitions := []string{"BVN0", "BVN1", "BVN2", "Directory"}
	var wg sync.WaitGroup

	// Metrics
	var totalRequests int64
	var totalCollectionProofs int64
	var totalIndividualProofs int64
	var totalTimeSaved int64

	start := time.Now()

	for _, partition := range partitions {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()

			// Simulate random recovery requests
			for i := 0; i < 10; i++ {
				missingCount := rand.Intn(100) + 5 // 5-105 missing transactions
				atomic.AddInt64(&totalRequests, 1)

				if missingCount >= 5 { // Use collection proof threshold
					atomic.AddInt64(&totalCollectionProofs, 1)
					simulateCollectionProofRecovery(missingCount)
					// Time saved: (missingCount - 1) individual proofs
					atomic.AddInt64(&totalTimeSaved, int64(missingCount-1))
				} else {
					atomic.AddInt64(&totalIndividualProofs, 1)
					simulateIndividualProofRecovery(missingCount)
				}

				// Random delay between requests
				time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
			}
		}(partition)
	}

	wg.Wait()
	duration := time.Since(start)

	fmt.Printf("Concurrent Recovery Results:\n")
	fmt.Printf("  Total Requests: %d\n", totalRequests)
	fmt.Printf("  Collection Proofs: %d\n", totalCollectionProofs)
	fmt.Printf("  Individual Proofs: %d\n", totalIndividualProofs)
	fmt.Printf("  Individual Proofs Saved: %d\n", totalTimeSaved)
	fmt.Printf("  Total Duration: %s\n", duration.Round(time.Millisecond))
	fmt.Printf("  Collection Proof Usage: %.1f%%\n",
		float64(totalCollectionProofs)/float64(totalRequests)*100)
}
