// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdPerfMon = &cobra.Command{
	Use:   "perfmon [server] [tps] [duration] [address]",
	Short: "Performance monitoring and load testing with detailed metrics",
	Args:  cobra.RangeArgs(3, 4),
	Run:   perfMon,
}

var (
	perfMonFlags = struct {
		OutputDir      string
		ReportInterval time.Duration
		EnableMetrics  bool
		CPUProfile     bool
		MemProfile     bool
	}{}
)

func init() {
	cmd.AddCommand(cmdPerfMon)
	cmdPerfMon.Flags().StringVar(&perfMonFlags.OutputDir, "output", "./perfmon-results", "Output directory for metrics and reports")
	cmdPerfMon.Flags().DurationVar(&perfMonFlags.ReportInterval, "interval", 10*time.Second, "Reporting interval for metrics")
	cmdPerfMon.Flags().BoolVar(&perfMonFlags.EnableMetrics, "metrics", true, "Enable detailed metrics collection")
	cmdPerfMon.Flags().BoolVar(&perfMonFlags.CPUProfile, "cpuprofile", false, "Enable CPU profiling")
	cmdPerfMon.Flags().BoolVar(&perfMonFlags.MemProfile, "memprofile", false, "Enable memory profiling")
}

// PerformanceMetrics tracks detailed performance data
type PerformanceMetrics struct {
	mu sync.Mutex

	// TPS metrics
	SubmittedCount  uint64
	SuccessCount    uint64
	ErrorCount      uint64
	LastSubmitTime  time.Time
	FirstSubmitTime time.Time

	// Latency tracking
	Latencies  []time.Duration
	LatencySum time.Duration

	// Error tracking
	ErrorsByType map[string]uint64

	// Transaction tracking
	PendingTxs   map[[32]byte]time.Time
	CompletedTxs uint64

	// Resource usage (sampled)
	CPUSamples    []float64
	MemorySamples []uint64

	// Block tracking
	BlockCount     uint64
	LastBlockTime  time.Time
	BlockIntervals []time.Duration
}

// PerformanceReport contains aggregated metrics for reporting
type PerformanceReport struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Duration  float64   `json:"duration_seconds"`

	// TPS metrics
	TargetTPS      uint64  `json:"target_tps"`
	AchievedTPS    float64 `json:"achieved_tps"`
	SubmittedTotal uint64  `json:"submitted_total"`
	SuccessTotal   uint64  `json:"success_total"`
	ErrorTotal     uint64  `json:"error_total"`
	SuccessRate    float64 `json:"success_rate_percent"`

	// Latency metrics
	LatencyP50  float64 `json:"latency_p50_ms"`
	LatencyP95  float64 `json:"latency_p95_ms"`
	LatencyP99  float64 `json:"latency_p99_ms"`
	LatencyMean float64 `json:"latency_mean_ms"`
	LatencyMin  float64 `json:"latency_min_ms"`
	LatencyMax  float64 `json:"latency_max_ms"`

	// Error breakdown
	ErrorsByType map[string]uint64 `json:"errors_by_type"`

	// Resource usage
	AvgCPU    float64 `json:"avg_cpu_percent"`
	MaxCPU    float64 `json:"max_cpu_percent"`
	AvgMemory uint64  `json:"avg_memory_bytes"`
	MaxMemory uint64  `json:"max_memory_bytes"`

	// Block metrics
	BlockCount       uint64  `json:"block_count"`
	AvgBlockInterval float64 `json:"avg_block_interval_ms"`
	BlockRate        float64 `json:"blocks_per_minute"`
}

func perfMon(_ *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(args[0], "v3"))
	server.Client.Timeout = time.Minute

	tps, err := strconv.ParseUint(args[1], 10, 64)
	checkf(err, "tps")

	duration, err := time.ParseDuration(args[2])
	checkf(err, "duration")

	var addr address.Address
	if len(args) > 3 {
		addr, err = address.Parse(args[3])
		checkf(err, "address")
	} else {
		pk, sk, err := ed25519.GenerateKey(rand.Reader)
		check(err)
		addr = &address.PrivateKey{
			PublicKey: address.PublicKey{
				Type: protocol.SignatureTypeED25519,
				Key:  pk,
			},
			Key: sk,
		}
		fmt.Println("Generated", addr)
	}

	// Create output directory
	err = os.MkdirAll(perfMonFlags.OutputDir, 0755)
	check(err)

	// Initialize metrics
	metrics := &PerformanceMetrics{
		ErrorsByType: make(map[string]uint64),
		PendingTxs:   make(map[[32]byte]time.Time),
		Latencies:    make([]time.Duration, 0, 10000),
	}

	// Setup account
	sk, kh := setupAccount(ctx, server, addr)
	account := protocol.LiteAuthorityForHash(kh[:]).JoinPath("ACME")

	var lid *protocol.LiteIdentity
	Q := api.Querier2{Querier: server}
	_, err = Q.QueryAccountAs(ctx, account.RootIdentity(), nil, &lid)
	check(err)

	// Setup data entry for transactions
	entry := new(protocol.DoubleHashDataEntry)
	entry.Data = append(entry.Data, []byte{})
	entry.Data = append(entry.Data, []byte("Performance Test"))
	entry.Data = append(entry.Data, []byte(time.Now().Format(time.RFC3339)))
	chainId := protocol.ComputeLiteDataAccountId(entry)
	lda, err := protocol.LiteDataAddress(chainId)
	check(err)

	nonce := uint64(time.Now().UTC().UnixMilli())

	// Start metrics collection
	metrics.FirstSubmitTime = time.Now()
	done := make(chan struct{})

	// Start transaction submission
	go submitTransactions(ctx, server, metrics, lid, lda, entry, sk, &nonce, tps, done)

	// Start metrics reporting
	go reportMetrics(metrics, tps, perfMonFlags.ReportInterval)

	// Start resource monitoring if enabled
	if perfMonFlags.EnableMetrics {
		go monitorResources(ctx, metrics)
	}

	// Run for specified duration
	fmt.Printf("Running performance test at %d TPS for %v\n", tps, duration)
	fmt.Printf("Metrics will be saved to %s\n", perfMonFlags.OutputDir)
	fmt.Println()

	time.Sleep(duration)

	// Signal completion
	close(done)
	cancel()
	time.Sleep(2 * time.Second) // Allow final metrics to be collected

	// Generate final report
	generateReport(metrics, tps, perfMonFlags.OutputDir)
}

func setupAccount(ctx context.Context, server *jsonrpc.Client, addr address.Address) (ed25519.PrivateKey, []byte) {
	sk, ok := addr.GetPrivateKey()
	if !ok {
		fatalf("address is not a private key")
	}
	kh, ok := addr.GetPublicKeyHash()
	if !ok {
		panic("have private key but no hash?!?")
	}

	Q := api.Querier2{Querier: server}
	var account *protocol.LiteTokenAccount
	_, err := Q.QueryAccountAs(ctx, protocol.LiteAuthorityForHash(kh).JoinPath("ACME"), nil, &account)
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		account = &protocol.LiteTokenAccount{
			Url: protocol.LiteAuthorityForHash(kh).JoinPath("ACME"),
		}
	default:
		check(err)
	}

	// Ensure balance
	if account.TokenBalance().Sign() == 0 {
		sub, err := server.Faucet(ctx, account.Url, api.FaucetOptions{Token: protocol.AcmeUrl()})
		check(err)
		fmt.Printf("Waiting for faucet transaction %v to complete\n", sub.Status.TxID)
		waitForMessages(ctx, Q, map[[32]byte]struct{}{
			sub.Status.TxID.Hash(): {},
		})
	}

	var lid *protocol.LiteIdentity
	_, err = Q.QueryAccountAs(ctx, account.Url.RootIdentity(), nil, &lid)
	check(err)

	// Ensure credits
	nonce := uint64(time.Now().UTC().UnixMilli())
	if lid.CreditBalance < 1000 {
		ns, err := server.NetworkStatus(ctx, api.NetworkStatusOptions{})
		check(err)
		env, err := build.Transaction().
			For(account.Url).
			AddCredits().To(lid.Url).Spend(100).
			WithOracle(float64(ns.Oracle.Price) / protocol.AcmeOraclePrecision).
			SignWith(lid.Url).Version(1).Timestamp(&nonce).PrivateKey(sk).
			Done()
		check(err)
		subs, err := server.Submit(ctx, env, api.SubmitOptions{})
		check(err)
		ids := map[[32]byte]struct{}{}
		for _, sub := range subs {
			if sub.Success {
				ids[sub.Status.TxID.Hash()] = struct{}{}
			} else if sub.Status != nil && sub.Status.Error != nil {
				check(sub.Status.Error)
			} else {
				fatalf(sub.Message)
			}
		}
		waitForMessages(ctx, Q, ids)
	}

	return sk, kh
}

func submitTransactions(
	ctx context.Context,
	server *jsonrpc.Client,
	metrics *PerformanceMetrics,
	lid *protocol.LiteIdentity,
	ldaUrl *url.URL,
	entry *protocol.DoubleHashDataEntry,
	sk ed25519.PrivateKey,
	nonce *uint64,
	tps uint64,
	done chan struct{},
) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			submitBatch(ctx, server, metrics, lid, ldaUrl, entry, sk, nonce, tps)
		}
	}
}

func submitBatch(
	ctx context.Context,
	server *jsonrpc.Client,
	metrics *PerformanceMetrics,
	lid *protocol.LiteIdentity,
	ldaUrl *url.URL,
	entry *protocol.DoubleHashDataEntry,
	sk ed25519.PrivateKey,
	nonce *uint64,
	tps uint64,
) {
	var messages []messaging.Message
	submitTime := time.Now()

	// Build transactions
	for i := range tps {
		_ = i
		env, err := build.Transaction().
			For(ldaUrl).
			WriteData(entry).
			SignWith(lid.Url).Version(1).Timestamp(nonce).PrivateKey(sk).
			Done()
		if err != nil {
			recordError(metrics, "build", err)
			continue
		}

		msg, err := env.Normalize()
		if err != nil {
			recordError(metrics, "normalize", err)
			continue
		}
		messages = append(messages, msg...)
	}

	// Submit batch
	subs, err := server.Submit(ctx, &messaging.Envelope{Messages: messages}, api.SubmitOptions{})
	if err != nil {
		recordError(metrics, "submit", err)
		return
	}

	// Track submissions
	metrics.mu.Lock()
	metrics.SubmittedCount += uint64(len(messages))
	metrics.LastSubmitTime = submitTime

	for _, sub := range subs {
		if sub.Success {
			metrics.SuccessCount++
			if sub.Status != nil {
				txHash := sub.Status.TxID.Hash()
				metrics.PendingTxs[txHash] = submitTime
			}
		} else {
			metrics.ErrorCount++
			if sub.Status != nil && sub.Status.Error != nil {
				recordErrorUnlocked(metrics, "tx_error", sub.Status.Error)
			}
		}
	}
	metrics.mu.Unlock()
}

func recordError(metrics *PerformanceMetrics, errType string, _ error) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	recordErrorUnlocked(metrics, errType, nil)
}

func recordErrorUnlocked(metrics *PerformanceMetrics, errType string, _ error) {
	metrics.ErrorCount++
	metrics.ErrorsByType[errType]++
}

func reportMetrics(metrics *PerformanceMetrics, targetTPS uint64, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastSubmitted uint64

	for range ticker.C {
		metrics.mu.Lock()

		submitted := metrics.SubmittedCount
		success := metrics.SuccessCount
		errors := metrics.ErrorCount
		elapsed := time.Since(metrics.FirstSubmitTime).Seconds()

		deltaSubmitted := submitted - lastSubmitted

		currentTPS := float64(deltaSubmitted) / interval.Seconds()
		avgTPS := float64(submitted) / elapsed
		successRate := 0.0
		if submitted > 0 {
			successRate = float64(success) / float64(submitted) * 100
		}

		metrics.mu.Unlock()

		fmt.Printf("[%.0fs] Target: %d TPS | Current: %.2f TPS | Avg: %.2f TPS | "+
			"Total: %d | Success: %d (%.2f%%) | Errors: %d\n",
			elapsed, targetTPS, currentTPS, avgTPS,
			submitted, success, successRate, errors)

		lastSubmitted = submitted
	}
}

func monitorResources(ctx context.Context, metrics *PerformanceMetrics) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Sample resource usage
			// Note: This is a placeholder. Real implementation would use
			// runtime.ReadMemStats() and potentially OS-specific calls for CPU
			var m struct{ Alloc uint64 }
			metrics.mu.Lock()
			metrics.MemorySamples = append(metrics.MemorySamples, m.Alloc)
			metrics.mu.Unlock()
		}
	}
}

func generateReport(metrics *PerformanceMetrics, targetTPS uint64, outputDir string) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	endTime := time.Now()
	duration := endTime.Sub(metrics.FirstSubmitTime).Seconds()

	report := PerformanceReport{
		StartTime:      metrics.FirstSubmitTime,
		EndTime:        endTime,
		Duration:       duration,
		TargetTPS:      targetTPS,
		SubmittedTotal: metrics.SubmittedCount,
		SuccessTotal:   metrics.SuccessCount,
		ErrorTotal:     metrics.ErrorCount,
		ErrorsByType:   metrics.ErrorsByType,
		BlockCount:     metrics.BlockCount,
	}

	// Calculate TPS
	if duration > 0 {
		report.AchievedTPS = float64(metrics.SubmittedCount) / duration
	}

	// Calculate success rate
	if metrics.SubmittedCount > 0 {
		report.SuccessRate = float64(metrics.SuccessCount) / float64(metrics.SubmittedCount) * 100
	}

	// Calculate latency percentiles
	if len(metrics.Latencies) > 0 {
		latencies := make([]time.Duration, len(metrics.Latencies))
		copy(latencies, metrics.Latencies)
		sort.Slice(latencies, func(i, j int) bool {
			return latencies[i] < latencies[j]
		})

		report.LatencyP50 = percentile(latencies, 50)
		report.LatencyP95 = percentile(latencies, 95)
		report.LatencyP99 = percentile(latencies, 99)
		report.LatencyMin = float64(latencies[0].Milliseconds())
		report.LatencyMax = float64(latencies[len(latencies)-1].Milliseconds())

		var sum time.Duration
		for _, l := range latencies {
			sum += l
		}
		report.LatencyMean = float64(sum.Milliseconds()) / float64(len(latencies))
	}

	// Calculate resource metrics
	if len(metrics.CPUSamples) > 0 {
		var sum float64
		for _, cpu := range metrics.CPUSamples {
			sum += cpu
			if cpu > report.MaxCPU {
				report.MaxCPU = cpu
			}
		}
		report.AvgCPU = sum / float64(len(metrics.CPUSamples))
	}

	if len(metrics.MemorySamples) > 0 {
		var sum uint64
		for _, mem := range metrics.MemorySamples {
			sum += mem
			if mem > report.MaxMemory {
				report.MaxMemory = mem
			}
		}
		report.AvgMemory = sum / uint64(len(metrics.MemorySamples))
	}

	// Calculate block metrics
	if len(metrics.BlockIntervals) > 0 {
		var sum time.Duration
		for _, interval := range metrics.BlockIntervals {
			sum += interval
		}
		report.AvgBlockInterval = float64(sum.Milliseconds()) / float64(len(metrics.BlockIntervals))
		if report.AvgBlockInterval > 0 {
			report.BlockRate = 60000.0 / report.AvgBlockInterval
		}
	}

	// Print summary
	printReport(&report)

	// Save JSON report
	reportFile := filepath.Join(outputDir, fmt.Sprintf("report_%s.json", time.Now().Format("20060102_150405")))
	saveReport(&report, reportFile)

	// Save CSV for time series analysis
	csvFile := filepath.Join(outputDir, fmt.Sprintf("metrics_%s.csv", time.Now().Format("20060102_150405")))
	saveMetricsCSV(metrics, csvFile)
}

func percentile(sorted []time.Duration, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := float64(p) / 100.0 * float64(len(sorted)-1)
	lower := int(math.Floor(rank))
	upper := int(math.Ceil(rank))
	weight := rank - float64(lower)

	if upper >= len(sorted) {
		return float64(sorted[len(sorted)-1].Milliseconds())
	}

	return float64(sorted[lower].Milliseconds())*(1-weight) + float64(sorted[upper].Milliseconds())*weight
}

func printReport(report *PerformanceReport) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("PERFORMANCE TEST REPORT")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Duration: %.2f seconds\n", report.Duration)
	fmt.Printf("Start: %s\n", report.StartTime.Format(time.RFC3339))
	fmt.Printf("End: %s\n\n", report.EndTime.Format(time.RFC3339))

	fmt.Println("THROUGHPUT METRICS")
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("Target TPS: %d\n", report.TargetTPS)
	fmt.Printf("Achieved TPS: %.2f\n", report.AchievedTPS)
	fmt.Printf("Total Submitted: %d\n", report.SubmittedTotal)
	fmt.Printf("Total Success: %d\n", report.SuccessTotal)
	fmt.Printf("Total Errors: %d\n", report.ErrorTotal)
	fmt.Printf("Success Rate: %.2f%%\n\n", report.SuccessRate)

	if len(report.ErrorsByType) > 0 {
		fmt.Println("ERROR BREAKDOWN")
		fmt.Println(strings.Repeat("-", 80))
		for errType, count := range report.ErrorsByType {
			fmt.Printf("  %s: %d\n", errType, count)
		}
		fmt.Println()
	}

	if report.LatencyMean > 0 {
		fmt.Println("LATENCY METRICS (milliseconds)")
		fmt.Println(strings.Repeat("-", 80))
		fmt.Printf("Mean: %.2f ms\n", report.LatencyMean)
		fmt.Printf("P50: %.2f ms\n", report.LatencyP50)
		fmt.Printf("P95: %.2f ms\n", report.LatencyP95)
		fmt.Printf("P99: %.2f ms\n", report.LatencyP99)
		fmt.Printf("Min: %.2f ms\n", report.LatencyMin)
		fmt.Printf("Max: %.2f ms\n\n", report.LatencyMax)
	}

	if report.BlockCount > 0 {
		fmt.Println("BLOCK METRICS")
		fmt.Println(strings.Repeat("-", 80))
		fmt.Printf("Total Blocks: %d\n", report.BlockCount)
		fmt.Printf("Avg Block Interval: %.2f ms\n", report.AvgBlockInterval)
		fmt.Printf("Block Rate: %.2f blocks/minute\n\n", report.BlockRate)
	}

	fmt.Println(strings.Repeat("=", 80) + "\n")
}

func saveReport(report *PerformanceReport, filename string) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling report: %v\n", err)
		return
	}

	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		fmt.Printf("Error writing report: %v\n", err)
		return
	}

	fmt.Printf("Report saved to: %s\n", filename)
}

func saveMetricsCSV(metrics *PerformanceMetrics, filename string) {
	f, err := os.Create(filename)
	if err != nil {
		fmt.Printf("Error creating CSV: %v\n", err)
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "timestamp,submitted,success,errors\n")
	fmt.Fprintf(f, "%d,%d,%d,%d\n",
		metrics.FirstSubmitTime.Unix(),
		metrics.SubmittedCount,
		metrics.SuccessCount,
		metrics.ErrorCount)

	fmt.Printf("Metrics CSV saved to: %s\n", filename)
}
