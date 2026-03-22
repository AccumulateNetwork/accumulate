// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
)

// Configuration for performance monitoring
type Config struct {
	ServerURL        string
	OutputDir        string
	SamplingInterval time.Duration
	TestDuration     time.Duration
	InitialTPS       int
	MaxTPS           int
	TPSIncrement     int
	IterationMinutes int
}

// PerformanceMetrics captures all monitored data points
type PerformanceMetrics struct {
	Timestamp           time.Time                `json:"timestamp"`
	TPS                 float64                  `json:"tps"`
	TargetTPS           int                      `json:"target_tps"`
	SuccessfulTxs       int                      `json:"successful_txs"`
	FailedTxs           int                      `json:"failed_txs"`
	ErrorRate           float64                  `json:"error_rate"`
	LatencyP50          float64                  `json:"latency_p50_ms"`
	LatencyP95          float64                  `json:"latency_p95_ms"`
	LatencyP99          float64                  `json:"latency_p99_ms"`
	LatencyAvg          float64                      `json:"latency_avg_ms"`
	BlockHeight         uint64                       `json:"block_height"`
	BlockProductionRate float64                      `json:"blocks_per_second"`
	CPUUsage            float64                      `json:"cpu_usage_percent"`
	MemoryUsageMB       float64                      `json:"memory_usage_mb"`
	DiskIOReadMBs       float64                      `json:"disk_io_read_mbs"`
	DiskIOWriteMBs      float64                      `json:"disk_io_write_mbs"`
	NetworkRxMBs        float64                      `json:"network_rx_mbs"`
	NetworkTxMBs        float64                      `json:"network_tx_mbs"`
	Partitions          map[string]*PartitionMetrics `json:"partitions,omitempty"`
}

// PartitionMetrics tracks per-partition data
type PartitionMetrics struct {
	BlockHeight uint64  `json:"block_height"`
	TPS         float64 `json:"tps"`
	Validators  int     `json:"validators"`
}

// TestRun represents a complete test iteration
type TestRun struct {
	RunID           string                `json:"run_id"`
	StartTime       time.Time             `json:"start_time"`
	EndTime         time.Time             `json:"end_time"`
	TargetTPS       int                   `json:"target_tps"`
	Duration        time.Duration         `json:"duration"`
	Metrics         []*PerformanceMetrics `json:"metrics"`
	Summary         *RunSummary           `json:"summary"`
	Bottlenecks     []string              `json:"bottlenecks"`
	Recommendations []string              `json:"recommendations"`
}

// RunSummary aggregates metrics for a test run
type RunSummary struct {
	AverageTPS          float64 `json:"average_tps"`
	MaxTPS              float64 `json:"max_tps"`
	MinTPS              float64 `json:"min_tps"`
	TotalTransactions   int     `json:"total_transactions"`
	SuccessfulTxs       int     `json:"successful_txs"`
	FailedTxs           int     `json:"failed_txs"`
	OverallErrorRate    float64 `json:"overall_error_rate"`
	AvgLatencyP50       float64 `json:"avg_latency_p50_ms"`
	AvgLatencyP95       float64 `json:"avg_latency_p95_ms"`
	AvgLatencyP99       float64 `json:"avg_latency_p99_ms"`
	AvgCPU              float64 `json:"avg_cpu_percent"`
	MaxCPU              float64 `json:"max_cpu_percent"`
	AvgMemoryMB         float64 `json:"avg_memory_mb"`
	MaxMemoryMB         float64 `json:"max_memory_mb"`
	AvgBlockRate        float64 `json:"avg_block_rate"`
	TotalBlocks         uint64  `json:"total_blocks"`
}

// Monitor coordinates performance monitoring
type Monitor struct {
	config      Config
	client      *client.Client
	runs        []*TestRun
	currentRun  *TestRun
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	lastMetrics *ResourceMetrics
}

// ResourceMetrics for system monitoring
type ResourceMetrics struct {
	CPUPercent   float64
	MemoryMB     float64
	DiskReadMBs  float64
	DiskWriteMBs float64
	NetRxMBs     float64
	NetTxMBs     float64
	Timestamp    time.Time
}

func main() {
	config := parseFlags()

	// Create output directory
	if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	monitor, err := NewMonitor(config)
	if err != nil {
		log.Fatalf("Failed to create monitor: %v", err)
	}
	defer monitor.Close()

	log.Printf("Starting performance monitoring framework")
	log.Printf("Output directory: %s", config.OutputDir)
	log.Printf("Initial TPS: %d, Max TPS: %d, Increment: %d",
		config.InitialTPS, config.MaxTPS, config.TPSIncrement)

	// Run iterative load tests
	if err := monitor.RunIterativeTests(); err != nil {
		log.Fatalf("Test execution failed: %v", err)
	}

	// Generate final report
	if err := monitor.GenerateFinalReport(); err != nil {
		log.Fatalf("Failed to generate report: %v", err)
	}

	log.Printf("Performance monitoring complete. Reports in: %s", config.OutputDir)
}

func parseFlags() Config {
	config := Config{}

	flag.StringVar(&config.ServerURL, "url", "http://127.0.1.1:26660/v2",
		"Accumulate server URL")
	flag.StringVar(&config.OutputDir, "output", "./perfmon_results",
		"Output directory for results")
	flag.DurationVar(&config.SamplingInterval, "interval", 5*time.Second,
		"Metrics sampling interval")
	flag.DurationVar(&config.TestDuration, "duration", 2*time.Minute,
		"Duration per test iteration")
	flag.IntVar(&config.InitialTPS, "initial-tps", 100,
		"Initial TPS for baseline test")
	flag.IntVar(&config.MaxTPS, "max-tps", 1000,
		"Maximum TPS to test")
	flag.IntVar(&config.TPSIncrement, "tps-increment", 100,
		"TPS increment per iteration")
	flag.IntVar(&config.IterationMinutes, "iteration-minutes", 5,
		"Minutes to run each TPS level")

	flag.Parse()

	config.TestDuration = time.Duration(config.IterationMinutes) * time.Minute

	return config
}

func NewMonitor(config Config) (*Monitor, error) {
	cl, err := client.New(config.ServerURL)
	if err != nil {
		return nil, fmt.Errorf("creating API v2 client: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := &Monitor{
		config: config,
		client: cl,
		runs:   make([]*TestRun, 0),
		ctx:    ctx,
		cancel: cancel,
	}

	return m, nil
}

func (m *Monitor) Close() {
	if m.cancel != nil {
		m.cancel()
	}
}

// RunIterativeTests executes load tests with increasing TPS
func (m *Monitor) RunIterativeTests() error {
	log.Printf("=== Starting Iterative Load Testing ===")

	for tps := m.config.InitialTPS; tps <= m.config.MaxTPS; tps += m.config.TPSIncrement {
		log.Printf("\n--- Starting test iteration at %d TPS ---", tps)

		run := &TestRun{
			RunID:     fmt.Sprintf("run_%d_tps_%s", tps, time.Now().Format("20060102_150405")),
			StartTime: time.Now(),
			TargetTPS: tps,
			Duration:  m.config.TestDuration,
			Metrics:   make([]*PerformanceMetrics, 0),
		}

		m.mu.Lock()
		m.currentRun = run
		m.runs = append(m.runs, run)
		m.mu.Unlock()

		// Start metrics collection
		metricsDone := make(chan struct{})
		go m.collectMetrics(run, metricsDone)

		// Run load test
		if err := m.runLoadTest(tps); err != nil {
			log.Printf("Error running load test at %d TPS: %v", tps, err)
			// Continue to next iteration
		}

		// Wait for metrics collection to complete
		close(metricsDone)
		time.Sleep(2 * time.Second) // Allow final metrics to be collected

		run.EndTime = time.Now()

		// Analyze results
		m.analyzeRun(run)

		// Save run results
		if err := m.saveRunResults(run); err != nil {
			log.Printf("Error saving run results: %v", err)
		}

		// Check if we should continue
		if m.shouldStopTesting(run) {
			log.Printf("Stopping test iterations - performance degradation detected")
			break
		}

		log.Printf("Completed test at %d TPS. Summary: %.2f avg TPS, %.2f%% error rate",
			tps, run.Summary.AverageTPS, run.Summary.OverallErrorRate*100)

		// Brief pause between iterations
		time.Sleep(10 * time.Second)
	}

	return nil
}

// runLoadTest executes the load test at specified TPS
func (m *Monitor) runLoadTest(tps int) error {
	loadCmd := filepath.Join(m.config.OutputDir, "..", "..", "cmd", "load", "load")

	// Check if load tool exists, if not try to build it
	if _, err := os.Stat(loadCmd); os.IsNotExist(err) {
		log.Printf("Load test tool not found, attempting to build...")
		buildCmd := exec.Command("go", "build", "-o", loadCmd, "./test/cmd/load")
		buildCmd.Dir = filepath.Join(m.config.OutputDir, "..", "..", "..")
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("building load test tool: %w", err)
		}
	}

	// Calculate duration in seconds
	durationSec := int(m.config.TestDuration.Seconds())

	args := []string{
		"-s", m.config.ServerURL,
		"-t", strconv.Itoa(tps),
		"-d", strconv.Itoa(durationSec),
	}

	log.Printf("Executing: %s %s", loadCmd, strings.Join(args, " "))

	cmd := exec.CommandContext(m.ctx, loadCmd, args...)

	// Capture output to log file
	logFile := filepath.Join(m.config.OutputDir, fmt.Sprintf("load_%d_tps.log", tps))
	outFile, err := os.Create(logFile)
	if err != nil {
		return fmt.Errorf("creating log file: %w", err)
	}
	defer outFile.Close()

	cmd.Stdout = outFile
	cmd.Stderr = outFile

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running load test: %w", err)
	}

	return nil
}

// collectMetrics gathers performance metrics during test run
func (m *Monitor) collectMetrics(run *TestRun, done chan struct{}) {
	ticker := time.NewTicker(m.config.SamplingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			metrics := m.gatherMetrics(run.TargetTPS)
			if metrics != nil {
				m.mu.Lock()
				run.Metrics = append(run.Metrics, metrics)
				m.mu.Unlock()
			}
		}
	}
}

// gatherMetrics collects current system and network metrics
func (m *Monitor) gatherMetrics(targetTPS int) *PerformanceMetrics {
	metrics := &PerformanceMetrics{
		Timestamp:  time.Now(),
		TargetTPS:  targetTPS,
		Partitions: make(map[string]*PartitionMetrics),
	}

	// Get node status for block height
	if status, err := m.getNodeStatus(); err == nil {
		if status.BvnHeight > 0 {
			metrics.BlockHeight = uint64(status.BvnHeight)
		}
		if status.DnHeight > 0 {
			metrics.BlockHeight = uint64(status.DnHeight)
		}
	}

	// Calculate TPS from recent block data
	metrics.TPS = m.calculateCurrentTPS()

	// Get resource metrics
	if resMetrics, err := m.getResourceMetrics(); err == nil {
		metrics.CPUUsage = resMetrics.CPUPercent
		metrics.MemoryUsageMB = resMetrics.MemoryMB

		// Calculate rates if we have previous metrics
		if m.lastMetrics != nil {
			timeDelta := resMetrics.Timestamp.Sub(m.lastMetrics.Timestamp).Seconds()
			if timeDelta > 0 {
				metrics.DiskIOReadMBs = (resMetrics.DiskReadMBs - m.lastMetrics.DiskReadMBs) / timeDelta
				metrics.DiskIOWriteMBs = (resMetrics.DiskWriteMBs - m.lastMetrics.DiskWriteMBs) / timeDelta
				metrics.NetworkRxMBs = (resMetrics.NetRxMBs - m.lastMetrics.NetRxMBs) / timeDelta
				metrics.NetworkTxMBs = (resMetrics.NetTxMBs - m.lastMetrics.NetTxMBs) / timeDelta
			}
		}
	}

	return metrics
}

// getNodeStatus retrieves current node status
func (m *Monitor) getNodeStatus() (*api.StatusResponse, error) {
	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
	defer cancel()

	var resp api.StatusResponse
	err := m.client.RequestAPIv2(ctx, "status", nil, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

// calculateCurrentTPS estimates current TPS from recent blocks
func (m *Monitor) calculateCurrentTPS() float64 {
	// This is a simplified calculation
	// In production, this would query recent blocks and count transactions
	return 0.0
}

// getResourceMetrics collects system resource usage
func (m *Monitor) getResourceMetrics() (*ResourceMetrics, error) {
	metrics := &ResourceMetrics{
		Timestamp: time.Now(),
	}

	// Get CPU usage
	if runtime.GOOS == "linux" {
		metrics.CPUPercent = m.getCPUUsageLinux()
		metrics.MemoryMB = m.getMemoryUsageLinux()

		// Get disk I/O - store absolute values for rate calculation
		diskRead, diskWrite := m.getDiskIOLinux()

		// Get network I/O - store absolute values for rate calculation
		netRx, netTx := m.getNetworkIOLinux()

		// Store raw values in ResourceMetrics for next calculation
		metrics.DiskReadMBs = diskRead
		metrics.DiskWriteMBs = diskWrite
		metrics.NetRxMBs = netRx
		metrics.NetTxMBs = netTx
	}

	m.lastMetrics = metrics
	return metrics, nil
}

// Platform-specific resource monitoring (Linux)
func (m *Monitor) getCPUUsageLinux() float64 {
	// Use pidof and top to get CPU usage for accumulated processes
	cmd := exec.Command("sh", "-c", "pidof accumulated | xargs -I {} ps -p {} -o %cpu --no-headers | awk '{sum+=$1} END {print sum}'")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	cpu, _ := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	return cpu
}

func (m *Monitor) getMemoryUsageLinux() float64 {
	cmd := exec.Command("sh", "-c", "pidof accumulated | xargs -I {} ps -p {} -o rss --no-headers | awk '{sum+=$1} END {print sum/1024}'")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	mem, _ := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	return mem
}

func (m *Monitor) getDiskIOLinux() (read, write float64) {
	// This is a simplified version - would need to track accumulated process I/O
	return 0, 0
}

func (m *Monitor) getNetworkIOLinux() (rx, tx float64) {
	// This is a simplified version - would need to track network interface stats
	return 0, 0
}

// analyzeRun performs bottleneck analysis and generates recommendations
func (m *Monitor) analyzeRun(run *TestRun) {
	run.Summary = m.calculateSummary(run)
	run.Bottlenecks = m.identifyBottlenecks(run)
	run.Recommendations = m.generateRecommendations(run)
}

// calculateSummary aggregates metrics for the run
func (m *Monitor) calculateSummary(run *TestRun) *RunSummary {
	summary := &RunSummary{}

	if len(run.Metrics) == 0 {
		return summary
	}

	var tpsSum, cpuSum, memSum, blockRateSum float64
	var p50Sum, p95Sum, p99Sum float64
	maxTPS, maxCPU, maxMem := 0.0, 0.0, 0.0
	minTPS := run.Metrics[0].TPS

	for _, m := range run.Metrics {
		tpsSum += m.TPS
		if m.TPS > maxTPS {
			maxTPS = m.TPS
		}
		if m.TPS < minTPS {
			minTPS = m.TPS
		}

		summary.SuccessfulTxs += m.SuccessfulTxs
		summary.FailedTxs += m.FailedTxs

		cpuSum += m.CPUUsage
		if m.CPUUsage > maxCPU {
			maxCPU = m.CPUUsage
		}

		memSum += m.MemoryUsageMB
		if m.MemoryUsageMB > maxMem {
			maxMem = m.MemoryUsageMB
		}

		blockRateSum += m.BlockProductionRate
		p50Sum += m.LatencyP50
		p95Sum += m.LatencyP95
		p99Sum += m.LatencyP99
	}

	n := float64(len(run.Metrics))
	summary.AverageTPS = tpsSum / n
	summary.MaxTPS = maxTPS
	summary.MinTPS = minTPS
	summary.TotalTransactions = summary.SuccessfulTxs + summary.FailedTxs
	if summary.TotalTransactions > 0 {
		summary.OverallErrorRate = float64(summary.FailedTxs) / float64(summary.TotalTransactions)
	}
	summary.AvgCPU = cpuSum / n
	summary.MaxCPU = maxCPU
	summary.AvgMemoryMB = memSum / n
	summary.MaxMemoryMB = maxMem
	summary.AvgBlockRate = blockRateSum / n
	summary.AvgLatencyP50 = p50Sum / n
	summary.AvgLatencyP95 = p95Sum / n
	summary.AvgLatencyP99 = p99Sum / n

	return summary
}

// identifyBottlenecks analyzes metrics to find performance bottlenecks
func (m *Monitor) identifyBottlenecks(run *TestRun) []string {
	bottlenecks := make([]string, 0)
	summary := run.Summary

	// Check if target TPS was achieved
	targetAchieved := (summary.AverageTPS / float64(run.TargetTPS)) * 100
	if targetAchieved < 90 {
		bottlenecks = append(bottlenecks,
			fmt.Sprintf("Target TPS not achieved: %.1f%% of target (%d TPS)",
				targetAchieved, run.TargetTPS))
	}

	// Check error rate
	if summary.OverallErrorRate > 0.05 { // 5% threshold
		bottlenecks = append(bottlenecks,
			fmt.Sprintf("High error rate: %.2f%%", summary.OverallErrorRate*100))
	}

	// Check CPU usage
	if summary.MaxCPU > 90 {
		bottlenecks = append(bottlenecks,
			fmt.Sprintf("CPU saturation detected: %.1f%% max usage", summary.MaxCPU))
	}

	// Check latency
	if summary.AvgLatencyP99 > 5000 { // 5 second threshold
		bottlenecks = append(bottlenecks,
			fmt.Sprintf("High latency detected: P99=%.1fms", summary.AvgLatencyP99))
	}

	// Check memory growth
	if len(run.Metrics) > 2 {
		startMem := run.Metrics[0].MemoryUsageMB
		endMem := run.Metrics[len(run.Metrics)-1].MemoryUsageMB
		growth := ((endMem - startMem) / startMem) * 100
		if growth > 50 { // 50% growth threshold
			bottlenecks = append(bottlenecks,
				fmt.Sprintf("Memory growth during test: %.1f%% increase", growth))
		}
	}

	// Check block production rate
	if summary.AvgBlockRate < 0.5 { // Less than 1 block per 2 seconds
		bottlenecks = append(bottlenecks,
			fmt.Sprintf("Slow block production: %.3f blocks/sec", summary.AvgBlockRate))
	}

	return bottlenecks
}

// generateRecommendations provides AI-driven tuning suggestions
func (m *Monitor) generateRecommendations(run *TestRun) []string {
	recommendations := make([]string, 0)
	summary := run.Summary

	// CPU-based recommendations
	if summary.MaxCPU > 80 {
		recommendations = append(recommendations,
			"Consider horizontal scaling: add more validator nodes to distribute load")
		recommendations = append(recommendations,
			"Profile CPU hotspots and optimize critical paths in transaction processing")
	}

	// Latency-based recommendations
	if summary.AvgLatencyP99 > 3000 {
		recommendations = append(recommendations,
			"High P99 latency suggests consensus delays - review validator connectivity")
		recommendations = append(recommendations,
			"Consider tuning block production interval for lower latency")
	}

	// Error rate recommendations
	if summary.OverallErrorRate > 0.05 {
		recommendations = append(recommendations,
			"Investigate error patterns - may indicate resource exhaustion or configuration issues")
		recommendations = append(recommendations,
			"Review transaction validation timeouts and retry logic")
	}

	// Memory recommendations
	if summary.MaxMemoryMB > 8192 { // 8GB threshold
		recommendations = append(recommendations,
			"High memory usage - review cache sizes and memory pool configurations")
		recommendations = append(recommendations,
			"Consider implementing more aggressive garbage collection")
	}

	// Block production recommendations
	if summary.AvgBlockRate < 0.5 {
		recommendations = append(recommendations,
			"Slow block production - review consensus timeout parameters")
		recommendations = append(recommendations,
			"Check for network latency between validators")
	}

	// TPS scaling recommendations
	if len(run.Bottlenecks) == 0 && summary.AverageTPS >= float64(run.TargetTPS)*0.95 {
		recommendations = append(recommendations,
			fmt.Sprintf("Performance healthy at %d TPS - can attempt higher load", run.TargetTPS))
	}

	return recommendations
}

// shouldStopTesting determines if test iterations should halt
func (m *Monitor) shouldStopTesting(run *TestRun) bool {
	summary := run.Summary

	// Stop if error rate exceeds 20%
	if summary.OverallErrorRate > 0.20 {
		log.Printf("Stopping: Error rate exceeded 20%% (%.2f%%)", summary.OverallErrorRate*100)
		return true
	}

	// Stop if TPS achieved is less than 50% of target
	if summary.AverageTPS < float64(run.TargetTPS)*0.5 {
		log.Printf("Stopping: TPS achievement below 50%% of target")
		return true
	}

	// Stop if CPU constantly maxed out
	if summary.MaxCPU > 95 && summary.AvgCPU > 90 {
		log.Printf("Stopping: CPU saturation detected")
		return true
	}

	return false
}

// saveRunResults writes run data to disk
func (m *Monitor) saveRunResults(run *TestRun) error {
	// Save detailed metrics
	metricsFile := filepath.Join(m.config.OutputDir, fmt.Sprintf("%s_metrics.json", run.RunID))
	if err := m.saveJSON(metricsFile, run); err != nil {
		return fmt.Errorf("saving metrics: %w", err)
	}

	// Save summary report
	reportFile := filepath.Join(m.config.OutputDir, fmt.Sprintf("%s_report.txt", run.RunID))
	if err := m.saveTextReport(reportFile, run); err != nil {
		return fmt.Errorf("saving report: %w", err)
	}

	// Save CSV for easy analysis
	csvFile := filepath.Join(m.config.OutputDir, fmt.Sprintf("%s_metrics.csv", run.RunID))
	if err := m.saveCSV(csvFile, run); err != nil {
		return fmt.Errorf("saving CSV: %w", err)
	}

	return nil
}

func (m *Monitor) saveJSON(filename string, data interface{}) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func (m *Monitor) saveTextReport(filename string, run *TestRun) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	fmt.Fprintf(file, "Performance Test Report\n")
	fmt.Fprintf(file, "======================\n\n")
	fmt.Fprintf(file, "Run ID: %s\n", run.RunID)
	fmt.Fprintf(file, "Target TPS: %d\n", run.TargetTPS)
	fmt.Fprintf(file, "Duration: %s\n", run.Duration)
	fmt.Fprintf(file, "Start Time: %s\n", run.StartTime.Format(time.RFC3339))
	fmt.Fprintf(file, "End Time: %s\n\n", run.EndTime.Format(time.RFC3339))

	fmt.Fprintf(file, "Summary\n")
	fmt.Fprintf(file, "-------\n")
	fmt.Fprintf(file, "Average TPS: %.2f\n", run.Summary.AverageTPS)
	fmt.Fprintf(file, "Max TPS: %.2f\n", run.Summary.MaxTPS)
	fmt.Fprintf(file, "Min TPS: %.2f\n", run.Summary.MinTPS)
	fmt.Fprintf(file, "Total Transactions: %d\n", run.Summary.TotalTransactions)
	fmt.Fprintf(file, "Successful: %d\n", run.Summary.SuccessfulTxs)
	fmt.Fprintf(file, "Failed: %d\n", run.Summary.FailedTxs)
	fmt.Fprintf(file, "Error Rate: %.2f%%\n", run.Summary.OverallErrorRate*100)
	fmt.Fprintf(file, "Avg Latency P50: %.2f ms\n", run.Summary.AvgLatencyP50)
	fmt.Fprintf(file, "Avg Latency P95: %.2f ms\n", run.Summary.AvgLatencyP95)
	fmt.Fprintf(file, "Avg Latency P99: %.2f ms\n", run.Summary.AvgLatencyP99)
	fmt.Fprintf(file, "Avg CPU: %.2f%%\n", run.Summary.AvgCPU)
	fmt.Fprintf(file, "Max CPU: %.2f%%\n", run.Summary.MaxCPU)
	fmt.Fprintf(file, "Avg Memory: %.2f MB\n", run.Summary.AvgMemoryMB)
	fmt.Fprintf(file, "Max Memory: %.2f MB\n", run.Summary.MaxMemoryMB)
	fmt.Fprintf(file, "Avg Block Rate: %.3f blocks/sec\n\n", run.Summary.AvgBlockRate)

	if len(run.Bottlenecks) > 0 {
		fmt.Fprintf(file, "Bottlenecks Identified\n")
		fmt.Fprintf(file, "---------------------\n")
		for i, b := range run.Bottlenecks {
			fmt.Fprintf(file, "%d. %s\n", i+1, b)
		}
		fmt.Fprintf(file, "\n")
	}

	if len(run.Recommendations) > 0 {
		fmt.Fprintf(file, "Recommendations\n")
		fmt.Fprintf(file, "---------------\n")
		for i, r := range run.Recommendations {
			fmt.Fprintf(file, "%d. %s\n", i+1, r)
		}
		fmt.Fprintf(file, "\n")
	}

	return nil
}

func (m *Monitor) saveCSV(filename string, run *TestRun) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// CSV header
	fmt.Fprintf(file, "timestamp,target_tps,tps,successful_txs,failed_txs,error_rate,")
	fmt.Fprintf(file, "latency_p50,latency_p95,latency_p99,latency_avg,")
	fmt.Fprintf(file, "block_height,block_rate,cpu_percent,memory_mb,")
	fmt.Fprintf(file, "disk_read_mbs,disk_write_mbs,net_rx_mbs,net_tx_mbs\n")

	// Data rows
	for _, m := range run.Metrics {
		fmt.Fprintf(file, "%s,%d,%.2f,%d,%d,%.4f,",
			m.Timestamp.Format(time.RFC3339),
			m.TargetTPS, m.TPS, m.SuccessfulTxs, m.FailedTxs, m.ErrorRate)
		fmt.Fprintf(file, "%.2f,%.2f,%.2f,%.2f,",
			m.LatencyP50, m.LatencyP95, m.LatencyP99, m.LatencyAvg)
		fmt.Fprintf(file, "%d,%.3f,%.2f,%.2f,",
			m.BlockHeight, m.BlockProductionRate, m.CPUUsage, m.MemoryUsageMB)
		fmt.Fprintf(file, "%.2f,%.2f,%.2f,%.2f\n",
			m.DiskIOReadMBs, m.DiskIOWriteMBs, m.NetworkRxMBs, m.NetworkTxMBs)
	}

	return nil
}

// GenerateFinalReport creates comprehensive report across all runs
func (m *Monitor) GenerateFinalReport() error {
	reportFile := filepath.Join(m.config.OutputDir, "final_report.txt")
	file, err := os.Create(reportFile)
	if err != nil {
		return err
	}
	defer file.Close()

	fmt.Fprintf(file, "Performance Monitoring Final Report\n")
	fmt.Fprintf(file, "===================================\n\n")
	fmt.Fprintf(file, "Test Configuration\n")
	fmt.Fprintf(file, "------------------\n")
	fmt.Fprintf(file, "Server URL: %s\n", m.config.ServerURL)
	fmt.Fprintf(file, "Initial TPS: %d\n", m.config.InitialTPS)
	fmt.Fprintf(file, "Max TPS: %d\n", m.config.MaxTPS)
	fmt.Fprintf(file, "TPS Increment: %d\n", m.config.TPSIncrement)
	fmt.Fprintf(file, "Iteration Duration: %s\n", m.config.TestDuration)
	fmt.Fprintf(file, "Total Runs: %d\n\n", len(m.runs))

	fmt.Fprintf(file, "Performance Envelope\n")
	fmt.Fprintf(file, "--------------------\n")

	// Find maximum sustainable TPS (error rate < 5%, TPS > 90% of target)
	maxSustainableTPS := 0
	for _, run := range m.runs {
		if run.Summary.OverallErrorRate < 0.05 &&
		   run.Summary.AverageTPS >= float64(run.TargetTPS)*0.9 {
			if run.TargetTPS > maxSustainableTPS {
				maxSustainableTPS = run.TargetTPS
			}
		}
	}
	fmt.Fprintf(file, "Maximum Sustainable TPS: %d\n\n", maxSustainableTPS)

	fmt.Fprintf(file, "Run Results\n")
	fmt.Fprintf(file, "-----------\n")
	for _, run := range m.runs {
		fmt.Fprintf(file, "\n%s (Target: %d TPS)\n", run.RunID, run.TargetTPS)
		fmt.Fprintf(file, "  Avg TPS: %.2f (%.1f%% of target)\n",
			run.Summary.AverageTPS,
			(run.Summary.AverageTPS/float64(run.TargetTPS))*100)
		fmt.Fprintf(file, "  Error Rate: %.2f%%\n", run.Summary.OverallErrorRate*100)
		fmt.Fprintf(file, "  Latency P99: %.2f ms\n", run.Summary.AvgLatencyP99)
		fmt.Fprintf(file, "  CPU: %.1f%% avg, %.1f%% max\n",
			run.Summary.AvgCPU, run.Summary.MaxCPU)
		fmt.Fprintf(file, "  Memory: %.1f MB avg, %.1f MB max\n",
			run.Summary.AvgMemoryMB, run.Summary.MaxMemoryMB)

		if len(run.Bottlenecks) > 0 {
			fmt.Fprintf(file, "  Bottlenecks: %d identified\n", len(run.Bottlenecks))
		}
	}

	fmt.Fprintf(file, "\n\nKey Findings\n")
	fmt.Fprintf(file, "------------\n")
	fmt.Fprintf(file, "1. Maximum sustainable TPS: %d\n", maxSustainableTPS)

	if len(m.runs) > 0 {
		lastRun := m.runs[len(m.runs)-1]
		fmt.Fprintf(file, "2. Primary bottlenecks at maximum load:\n")
		for _, b := range lastRun.Bottlenecks {
			fmt.Fprintf(file, "   - %s\n", b)
		}

		fmt.Fprintf(file, "\n3. Recommendations for dagbft-integration:\n")
		for _, r := range lastRun.Recommendations {
			fmt.Fprintf(file, "   - %s\n", r)
		}
	}

	// Generate visualization data
	if err := m.generateVisualizationData(); err != nil {
		log.Printf("Warning: Failed to generate visualization data: %v", err)
	}

	return nil
}

// generateVisualizationData creates data files for plotting
func (m *Monitor) generateVisualizationData() error {
	// Create summary CSV across all runs
	summaryFile := filepath.Join(m.config.OutputDir, "summary.csv")
	file, err := os.Create(summaryFile)
	if err != nil {
		return err
	}
	defer file.Close()

	fmt.Fprintf(file, "run_id,target_tps,avg_tps,max_tps,error_rate,")
	fmt.Fprintf(file, "avg_latency_p50,avg_latency_p95,avg_latency_p99,")
	fmt.Fprintf(file, "avg_cpu,max_cpu,avg_memory,max_memory,avg_block_rate\n")

	for _, run := range m.runs {
		s := run.Summary
		fmt.Fprintf(file, "%s,%d,%.2f,%.2f,%.4f,",
			run.RunID, run.TargetTPS, s.AverageTPS, s.MaxTPS, s.OverallErrorRate)
		fmt.Fprintf(file, "%.2f,%.2f,%.2f,",
			s.AvgLatencyP50, s.AvgLatencyP95, s.AvgLatencyP99)
		fmt.Fprintf(file, "%.2f,%.2f,%.2f,%.2f,%.3f\n",
			s.AvgCPU, s.MaxCPU, s.AvgMemoryMB, s.MaxMemoryMB, s.AvgBlockRate)
	}

	// Create gnuplot script for visualization
	plotScript := filepath.Join(m.config.OutputDir, "plot.gnu")
	if err := m.createPlotScript(plotScript); err != nil {
		log.Printf("Warning: Failed to create plot script: %v", err)
	}

	return nil
}

func (m *Monitor) createPlotScript(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	script := `#!/usr/bin/gnuplot
# Performance visualization script
set terminal png size 1200,800
set datafile separator ","

# TPS comparison
set output 'tps_comparison.png'
set title "TPS: Target vs Achieved"
set xlabel "Target TPS"
set ylabel "TPS"
set grid
plot 'summary.csv' using 2:3 with linespoints title "Achieved TPS" lw 2, \
     x title "Target TPS" with lines lw 2 dt 2

# Error rate
set output 'error_rate.png'
set title "Error Rate by Load"
set xlabel "Target TPS"
set ylabel "Error Rate (%)"
set grid
plot 'summary.csv' using 2:($5*100) with linespoints title "Error Rate" lw 2

# Latency
set output 'latency.png'
set title "Latency Percentiles"
set xlabel "Target TPS"
set ylabel "Latency (ms)"
set grid
set logscale y
plot 'summary.csv' using 2:6 with linespoints title "P50" lw 2, \
     'summary.csv' using 2:7 with linespoints title "P95" lw 2, \
     'summary.csv' using 2:8 with linespoints title "P99" lw 2

# CPU usage
set output 'cpu_usage.png'
set title "CPU Usage"
set xlabel "Target TPS"
set ylabel "CPU (%)"
set grid
unset logscale y
plot 'summary.csv' using 2:9 with linespoints title "Avg CPU" lw 2, \
     'summary.csv' using 2:10 with linespoints title "Max CPU" lw 2

# Memory usage
set output 'memory_usage.png'
set title "Memory Usage"
set xlabel "Target TPS"
set ylabel "Memory (MB)"
set grid
plot 'summary.csv' using 2:11 with linespoints title "Avg Memory" lw 2, \
     'summary.csv' using 2:12 with linespoints title "Max Memory" lw 2
`

	_, err = file.WriteString(script)
	return err
}

// Latency calculation helpers
func calculateLatencyPercentiles(latencies []float64) (p50, p95, p99, avg float64) {
	if len(latencies) == 0 {
		return 0, 0, 0, 0
	}

	sorted := make([]float64, len(latencies))
	copy(sorted, latencies)
	sort.Float64s(sorted)

	sum := 0.0
	for _, l := range sorted {
		sum += l
	}
	avg = sum / float64(len(sorted))

	p50 = sorted[int(float64(len(sorted))*0.50)]
	p95 = sorted[int(float64(len(sorted))*0.95)]
	p99 = sorted[int(float64(len(sorted))*0.99)]

	return
}
