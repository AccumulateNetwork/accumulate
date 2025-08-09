package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// MetricsSnapshot represents a point-in-time snapshot of all metrics
type MetricsSnapshot struct {
	Timestamp   time.Time                   `json:"timestamp"`
	Sequence    int64                       `json:"sequence"`
	Partitions  map[string]*PartitionMetric `json:"partitions"`
	Network     *NetworkMetric              `json:"network"`
	Performance *PerformanceMetric          `json:"performance"`
	Event       string                      `json:"event,omitempty"`
}

// PartitionMetric represents metrics for a single partition
type PartitionMetric struct {
	Name         string  `json:"name"`
	Healthy      bool    `json:"healthy"`
	Sent         int64   `json:"sent"`
	Acknowledged int64   `json:"acknowledged"`
	Lag          int64   `json:"lag"`
	CatchUpRate  float64 `json:"catch_up_rate"`
	ProofSavings int64   `json:"proof_savings"`
}

// NetworkMetric represents network-wide metrics
type NetworkMetric struct {
	TotalSubmits     int64   `json:"total_submits"`
	TotalSuccesses   int64   `json:"total_successes"`
	TotalFailures    int64   `json:"total_failures"`
	SuccessRate      float64 `json:"success_rate"`
	CollectionProofs int64   `json:"collection_proofs"`
	IndividualProofs int64   `json:"individual_proofs"`
	ProofSavings     int64   `json:"proof_savings"`
}

// PerformanceMetric represents performance measurements
type PerformanceMetric struct {
	AverageLatency  float64 `json:"average_latency_ms"`
	P95Latency      float64 `json:"p95_latency_ms"`
	P99Latency      float64 `json:"p99_latency_ms"`
	ThroughputTPS   float64 `json:"throughput_tps"`
	ProofGenTimeAvg float64 `json:"proof_gen_time_avg_ms"`
}

// VisualMonitorWithJSON combines visual display with JSON logging
type VisualMonitorWithJSON struct {
	partitions       map[string]*PartitionInfo
	globalSequence   int64
	totalSubmits     int64
	totalSuccesses   int64
	totalFailures    int64
	collectionProofs int64
	individualProofs int64
	proofSavings     int64
	mu               sync.RWMutex
	jsonFile         *os.File
	jsonEncoder      *json.Encoder
	startTime        time.Time
	latencies        []float64
}

// PartitionInfo tracks partition state
type PartitionInfo struct {
	Name             string
	Healthy          bool
	LastSent         int64
	LastAcknowledged int64
	Lag              int64
	CatchUpRate      float64
	ProofSavings     int64
	LastUpdate       time.Time
}

func NewVisualMonitorWithJSON() (*VisualMonitorWithJSON, error) {
	// Open JSON log file
	jsonFile, err := os.Create("monitor_metrics.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create JSON log file: %w", err)
	}

	monitor := &VisualMonitorWithJSON{
		partitions:  make(map[string]*PartitionInfo),
		jsonFile:    jsonFile,
		jsonEncoder: json.NewEncoder(jsonFile),
		startTime:   time.Now(),
		latencies:   make([]float64, 0, 1000),
	}

	// Initialize partitions
	partitionNames := []string{"BVN0", "BVN1", "BVN2", "Directory"}
	for _, name := range partitionNames {
		monitor.partitions[name] = &PartitionInfo{
			Name:       name,
			Healthy:    true,
			LastUpdate: time.Now(),
		}
	}

	return monitor, nil
}

func (m *VisualMonitorWithJSON) Start() {
	fmt.Println("================================================================================")
	fmt.Println("         VISUAL PARTITION MONITOR WITH JSON LOGGING")
	fmt.Println("================================================================================")
	fmt.Println()
	fmt.Println("📊 JSON metrics are being saved to: monitor_metrics.json")
	fmt.Println("🖥️  Visual display will update every second")
	fmt.Println()
	fmt.Println("TO RUN THIS MONITOR:")
	fmt.Println("  go run visual_monitor_with_json.go")
	fmt.Println()
	fmt.Println("TO VIEW JSON LOGS IN REAL-TIME:")
	fmt.Println("  tail -f monitor_metrics.json | jq .")
	fmt.Println()
	fmt.Println("INTERACTIVE CONTROLS:")
	fmt.Println("  Press 1-4: Toggle partition health")
	fmt.Println("  Press 'c': Cause cascading failure")
	fmt.Println("  Press 'r': Recover all partitions")
	fmt.Println("  Press 'b': Simulate batch proof optimization")
	fmt.Println("  Press 'q': Quit")
	fmt.Println()
	fmt.Println("Starting simulation...")
	fmt.Println()
	time.Sleep(3 * time.Second)

	// Start background workers
	go m.simulateTransactions()
	go m.logMetrics()

	// Start visual display
	m.displayLoop()
}

func (m *VisualMonitorWithJSON) simulateTransactions() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		sequence := atomic.AddInt64(&m.globalSequence, 1)

		// Simulate sending to all healthy partitions
		m.mu.Lock()
		for _, partition := range m.partitions {
			if partition.Healthy {
				// Decide whether to use collection proof
				batchSize := rand.Intn(10) + 1
				useCollection := batchSize >= 2

				if useCollection {
					atomic.AddInt64(&m.collectionProofs, 1)
					savings := int64(batchSize - 1)
					atomic.AddInt64(&m.proofSavings, savings)
					partition.ProofSavings += savings
				} else {
					atomic.AddInt64(&m.individualProofs, int64(batchSize))
				}

				partition.LastSent = sequence
				atomic.AddInt64(&m.totalSubmits, int64(batchSize))

				// Simulate acknowledgment with slight delay
				if rand.Float64() > 0.1 { // 90% success rate
					partition.LastAcknowledged = sequence - rand.Int63n(3)
					atomic.AddInt64(&m.totalSuccesses, int64(batchSize))

					// Record latency
					latency := float64(rand.Intn(50) + 10)
					m.latencies = append(m.latencies, latency)
					if len(m.latencies) > 1000 {
						m.latencies = m.latencies[len(m.latencies)-1000:]
					}
				} else {
					atomic.AddInt64(&m.totalFailures, int64(batchSize))
				}

				// Calculate lag and catch-up rate
				partition.Lag = partition.LastSent - partition.LastAcknowledged
				if partition.Lag > 0 {
					partition.CatchUpRate = float64(partition.LastAcknowledged) / float64(time.Since(partition.LastUpdate).Seconds())
				}
			} else {
				// Accumulate lag for unhealthy partitions
				partition.Lag++
			}
		}
		m.mu.Unlock()
	}
}

func (m *VisualMonitorWithJSON) logMetrics() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		snapshot := m.captureSnapshot()

		// Write to JSON file
		if err := m.jsonEncoder.Encode(snapshot); err != nil {
			log.Printf("Failed to write JSON metrics: %v", err)
		}

		// Flush to ensure data is written
		m.jsonFile.Sync()
	}
}

func (m *VisualMonitorWithJSON) captureSnapshot() *MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	partitionMetrics := make(map[string]*PartitionMetric)
	for name, info := range m.partitions {
		partitionMetrics[name] = &PartitionMetric{
			Name:         name,
			Healthy:      info.Healthy,
			Sent:         info.LastSent,
			Acknowledged: info.LastAcknowledged,
			Lag:          info.Lag,
			CatchUpRate:  info.CatchUpRate,
			ProofSavings: info.ProofSavings,
		}
	}

	// Calculate performance metrics
	var avgLatency, p95Latency, p99Latency float64
	if len(m.latencies) > 0 {
		sum := float64(0)
		for _, l := range m.latencies {
			sum += l
		}
		avgLatency = sum / float64(len(m.latencies))

		// Simple percentile calculation (would use proper algorithm in production)
		p95Latency = m.latencies[int(float64(len(m.latencies))*0.95)]
		p99Latency = m.latencies[int(float64(len(m.latencies))*0.99)]
	}

	elapsed := time.Since(m.startTime).Seconds()
	throughput := float64(m.totalSubmits) / elapsed

	successRate := float64(0)
	if m.totalSubmits > 0 {
		successRate = float64(m.totalSuccesses) / float64(m.totalSubmits) * 100
	}

	return &MetricsSnapshot{
		Timestamp:  time.Now(),
		Sequence:   m.globalSequence,
		Partitions: partitionMetrics,
		Network: &NetworkMetric{
			TotalSubmits:     m.totalSubmits,
			TotalSuccesses:   m.totalSuccesses,
			TotalFailures:    m.totalFailures,
			SuccessRate:      successRate,
			CollectionProofs: m.collectionProofs,
			IndividualProofs: m.individualProofs,
			ProofSavings:     m.proofSavings,
		},
		Performance: &PerformanceMetric{
			AverageLatency:  avgLatency,
			P95Latency:      p95Latency,
			P99Latency:      p99Latency,
			ThroughputTPS:   throughput,
			ProofGenTimeAvg: avgLatency * 0.3, // Simulate proof gen being 30% of latency
		},
	}
}

func (m *VisualMonitorWithJSON) displayLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		m.displayStatus()
	}
}

func (m *VisualMonitorWithJSON) displayStatus() {
	// Clear screen
	fmt.Print("\033[H\033[2J")

	elapsed := time.Since(m.startTime)
	fmt.Println("================================================================================")
	fmt.Println("                    PARTITION LAG AND CATCH-UP MONITOR")
	fmt.Println("================================================================================")
	fmt.Printf("Running: %s | Global Sequence: %d | JSON Log: monitor_metrics.json\n\n", elapsed.Round(time.Second), m.globalSequence)

	// Display partition status table
	fmt.Println("┌─────────────┬──────────┬──────────┬──────────┬──────────┬─────────────────────┐")
	fmt.Println("│ Partition   │ Status   │ Sent     │ Acked    │ Lag      │ Proof Savings       │")
	fmt.Println("├─────────────┼──────────┼──────────┼──────────┼──────────┼─────────────────────┤")

	m.mu.RLock()
	for _, name := range []string{"BVN0", "BVN1", "BVN2", "Directory"} {
		partition := m.partitions[name]
		status := "🟢 HEALTHY"
		if !partition.Healthy {
			status = "🔴 DOWN"
		}

		fmt.Printf("│ %-11s │ %-8s │ %-8d │ %-8d │ %-8d │ %-19d │\n",
			partition.Name, status, partition.LastSent, partition.LastAcknowledged,
			partition.Lag, partition.ProofSavings)
	}
	m.mu.RUnlock()

	fmt.Println("└─────────────┴──────────┴──────────┴──────────┴──────────┴─────────────────────┘")

	// Display proof optimization metrics
	fmt.Println("\n📊 COLLECTION PROOF METRICS")
	fmt.Println("────────────────────────────────────────────────────────────────────────────────")
	fmt.Printf("  Collection Proofs: %d | Individual Proofs: %d | Total Savings: %d\n",
		m.collectionProofs, m.individualProofs, m.proofSavings)

	if m.collectionProofs > 0 || m.individualProofs > 0 {
		collectionPercent := float64(m.collectionProofs) / float64(m.collectionProofs+m.individualProofs) * 100
		efficiencyGain := float64(m.proofSavings) / float64(m.totalSubmits) * 100
		fmt.Printf("  Collection Usage: %.1f%% | Efficiency Gain: %.1f%%\n",
			collectionPercent, efficiencyGain)
	}

	// Display network statistics
	fmt.Println("\n🌐 NETWORK STATISTICS")
	fmt.Println("────────────────────────────────────────────────────────────────────────────────")
	successRate := float64(0)
	if m.totalSubmits > 0 {
		successRate = float64(m.totalSuccesses) / float64(m.totalSubmits) * 100
	}
	fmt.Printf("  Submits: %d | Successes: %d | Failures: %d | Success Rate: %.1f%%\n",
		m.totalSubmits, m.totalSuccesses, m.totalFailures, successRate)

	throughput := float64(m.totalSubmits) / elapsed.Seconds()
	fmt.Printf("  Throughput: %.1f TPS | Runtime: %s\n", throughput, elapsed.Round(time.Second))

	// Display controls
	fmt.Println("\n🎮 CONTROLS")
	fmt.Println("────────────────────────────────────────────────────────────────────────────────")
	fmt.Println("  1-4: Toggle partition  | c: Cascade fail | r: Recover all | b: Batch test | q: Quit")
}

func (m *VisualMonitorWithJSON) Close() {
	// Write final snapshot
	finalSnapshot := m.captureSnapshot()
	finalSnapshot.Event = "monitor_stopped"
	m.jsonEncoder.Encode(finalSnapshot)

	// Close JSON file
	m.jsonFile.Close()

	fmt.Println("\n✅ Monitor stopped. JSON metrics saved to: monitor_metrics.json")
	fmt.Println("   To analyze the metrics, run: cat monitor_metrics.json | jq .")
}

func main() {
	monitor, err := NewVisualMonitorWithJSON()
	if err != nil {
		log.Fatalf("Failed to create monitor: %v", err)
	}
	defer monitor.Close()

	monitor.Start()
}
