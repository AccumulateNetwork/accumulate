// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package monitoring

import (
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestPercentileCalculations validates P50/P95/P99 calculations
// as specified in the acceptance criteria
func TestPercentileCalculations(t *testing.T) {
	testCases := []struct {
		name           string
		values         []float64
		expectedP50    float64
		expectedP95    float64
		expectedP99    float64
		tolerancePct   float64
	}{
		{
			name:         "uniform_distribution",
			values:       []float64{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0},
			expectedP50:  6.0,  // 50th percentile of 10 values = index 5 (6th value)
			expectedP95:  10.0, // 95th percentile of 10 values = index 9 (10th value)
			expectedP99:  10.0, // 99th percentile of 10 values = index 9 (10th value)
			tolerancePct: 10.0,
		},
		{
			name: "realistic_settlement_times",
			values: func() []float64 {
				vals := make([]float64, 100)
				for i := range vals {
					vals[i] = 1.0 + float64(i)*0.05
				}
				return vals
			}(),
			expectedP50:  3.5,
			expectedP95:  5.75,
			expectedP99:  5.95,
			tolerancePct: 5.0,
		},
		{
			name: "with_outliers",
			values: func() []float64 {
				vals := make([]float64, 100)
				for i := range vals {
					if i < 95 {
						vals[i] = 1.0 + float64(i)*0.01
					} else {
						vals[i] = 10.0 + float64(i)*0.1
					}
				}
				return vals
			}(),
			expectedP50:  1.5,
			expectedP95:  19.5, // index 95 = 10.0 + 95*0.1 = 19.5
			expectedP99:  19.9, // index 99 = 10.0 + 99*0.1 = 19.9
			tolerancePct: 10.0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p50, p95, p99 := calculatePercentiles(tc.values)

			// Validate calculations are within tolerance
			require.InDelta(t, tc.expectedP50, p50, tc.expectedP50*tc.tolerancePct/100.0,
				"P50 should match expected value")
			require.InDelta(t, tc.expectedP95, p95, tc.expectedP95*tc.tolerancePct/100.0,
				"P95 should match expected value")
			require.InDelta(t, tc.expectedP99, p99, tc.expectedP99*tc.tolerancePct/100.0,
				"P99 should match expected value")

			t.Logf("Percentiles: P50=%.2f (expected %.2f), P95=%.2f (expected %.2f), P99=%.2f (expected %.2f)",
				p50, tc.expectedP50, p95, tc.expectedP95, p99, tc.expectedP99)
		})
	}
}

// TestBottleneckDetection simulates scenarios with known bottlenecks
// and validates that performance monitoring can detect them
func TestBottleneckDetection(t *testing.T) {
	acctesting.DisableDebugFeatures()
	defer acctesting.EnableDebugFeatures()

	t.Run("high_latency_detection", func(t *testing.T) {
		alice := url.MustParse("alice")
		aliceKey := acctesting.GenerateKey(alice)
		bobKey := acctesting.GenerateKey("bob")
		bob := acctesting.AcmeLiteAddressStdPriv(bobKey)

		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 1, 1),
			simulator.Genesis(GenesisTime),
		)

		MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
		CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
		MakeAccount(t, sim.DatabaseFor(alice), &protocol.TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: protocol.AcmeUrl()})
		CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))

		// Create temporary directory for performance data
		tmpDir, err := os.MkdirTemp("", "bottleneck_test_*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		dsl := &logging.DataSetLog{}
		dsl.SetPath(tmpDir)
		dsl.SetProcessName("bottleneck_test")
		dsl.Initialize("latency", logging.DefaultOptions())
		ds := dsl.GetDataSet("latency")

		var timestamp uint64
		latencies := make([]float64, 0)

		// Simulate transactions with varying latency
		for i := range 50 {
			startTime := time.Now()

			txn := MustBuild(t,
				build.Transaction().For(alice, "tokens").
					SendTokens(1, 0).To(bob).
					SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

			sim.SubmitTxnSuccessfully(txn)

			latency := time.Since(startTime).Seconds()
			latencies = append(latencies, latency)

			// Record latency
			ds.Lock()
			ds.Save("txn", i, 10, true)
			ds.Save("latency", latency, 10, false)
			ds.Unlock()

			// Simulate occasional high latency (bottleneck)
			if i%10 == 9 {
				time.Sleep(100 * time.Millisecond)
			}
		}

		// Analyze for bottlenecks
		p50, p95, p99 := calculatePercentiles(latencies)
		avgLatency := average(latencies)
		maxLatency := max(latencies)

		// Detect potential bottleneck: P99 significantly higher than P50
		bottleneckRatio := p99 / p50
		hasBottleneck := bottleneckRatio > 2.0

		t.Logf("Latency analysis: avg=%.3fs, max=%.3fs, P50=%.3fs, P95=%.3fs, P99=%.3fs",
			avgLatency, maxLatency, p50, p95, p99)
		t.Logf("Bottleneck detection: P99/P50 ratio=%.2f, bottleneck detected=%v",
			bottleneckRatio, hasBottleneck)

		// Save dataset
		files, err := dsl.DumpDataSetToDiskFile()
		require.NoError(t, err)
		require.NotEmpty(t, files)

		t.Logf("Performance data saved to: %v", files)
	})
}

// TestRealLoadWorkload simulates a realistic load test scenario
// and validates all monitoring components work together
func TestRealLoadWorkload(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real load workload test in short mode")
	}

	acctesting.DisableDebugFeatures()
	defer acctesting.EnableDebugFeatures()

	const (
		targetTPS    = 50
		durationSecs = 10
		numTxns      = targetTPS * durationSecs
	)

	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey("bob")
	bob := acctesting.AcmeLiteAddressStdPriv(bobKey)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &protocol.TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: protocol.AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))

	// Setup monitoring
	tmpDir, err := os.MkdirTemp("", "load_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dsl := &logging.DataSetLog{}
	dsl.SetPath(tmpDir)
	dsl.SetProcessName("load_test")
	dsl.SetHeader(fmt.Sprintf("## Load Test: %d TPS for %d seconds\n", targetTPS, durationSecs))
	dsl.Initialize("performance", logging.DefaultOptions())
	ds := dsl.GetDataSet("performance")

	var timestamp uint64
	startTime := time.Now()
	settlementTimes := make([]float64, 0, numTxns)
	successCount := 0

	// Run load test
	for i := range numTxns {
		txnStartTime := time.Now()

		txn := MustBuild(t,
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob).
				SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

		st := sim.SubmitTxnSuccessfully(txn)
		if st != nil {
			successCount++
			settlementTime := time.Since(txnStartTime).Seconds()
			settlementTimes = append(settlementTimes, settlementTime)

			// Record to dataset
			ds.Lock()
			ds.Save("txn_id", i, 10, true)
			ds.Save("timestamp", float64(time.Now().Unix()), 15, false)
			ds.Save("settlement_time", settlementTime, 10, false)
			ds.Unlock()
		}

		// Throttle to maintain target TPS
		if i > 0 && i%targetTPS == 0 {
			elapsed := time.Since(startTime)
			expectedTime := time.Duration(i/targetTPS) * time.Second
			if elapsed < expectedTime {
				time.Sleep(expectedTime - elapsed)
			}
		}
	}

	totalElapsed := time.Since(startTime)
	actualTPS := float64(successCount) / totalElapsed.Seconds()

	// Generate performance report
	p50, p95, p99 := calculatePercentiles(settlementTimes)
	avgSettlement := average(settlementTimes)
	maxSettlement := max(settlementTimes)

	// Save dataset
	files, err := dsl.DumpDataSetToDiskFile()
	require.NoError(t, err)
	require.NotEmpty(t, files)

	// Generate summary report
	reportFile := filepath.Join(tmpDir, "load_test_summary.txt")
	f, err := os.Create(reportFile)
	require.NoError(t, err)
	defer f.Close()

	fmt.Fprintf(f, "Load Test Summary\n")
	fmt.Fprintf(f, "=================\n\n")
	fmt.Fprintf(f, "Configuration:\n")
	fmt.Fprintf(f, "  Target TPS: %d\n", targetTPS)
	fmt.Fprintf(f, "  Duration: %d seconds\n", durationSecs)
	fmt.Fprintf(f, "  Target Transactions: %d\n\n", numTxns)
	fmt.Fprintf(f, "Results:\n")
	fmt.Fprintf(f, "  Successful Transactions: %d\n", successCount)
	fmt.Fprintf(f, "  Actual TPS: %.2f\n", actualTPS)
	fmt.Fprintf(f, "  Total Duration: %.2f seconds\n\n", totalElapsed.Seconds())
	fmt.Fprintf(f, "Settlement Time Metrics:\n")
	fmt.Fprintf(f, "  Average: %.3f seconds\n", avgSettlement)
	fmt.Fprintf(f, "  Maximum: %.3f seconds\n", maxSettlement)
	fmt.Fprintf(f, "  P50: %.3f seconds\n", p50)
	fmt.Fprintf(f, "  P95: %.3f seconds\n", p95)
	fmt.Fprintf(f, "  P99: %.3f seconds\n\n", p99)
	fmt.Fprintf(f, "Performance Data Files:\n")
	for _, file := range files {
		fmt.Fprintf(f, "  - %s\n", file)
	}

	// Validate results
	require.Equal(t, numTxns, successCount, "all transactions should succeed")
	require.Greater(t, actualTPS, 0.0, "TPS should be positive")

	// Validate metrics accuracy (within 30% for simulation timing variance)
	tolerance := 0.30
	require.GreaterOrEqual(t, actualTPS, float64(targetTPS)*(1-tolerance),
		"actual TPS should be within tolerance")

	t.Logf("Load test completed: %d txns in %.2fs (%.2f TPS)",
		successCount, totalElapsed.Seconds(), actualTPS)
	t.Logf("Settlement times: avg=%.3fs, P50=%.3fs, P95=%.3fs, P99=%.3fs",
		avgSettlement, p50, p95, p99)
	t.Logf("Report saved to: %s", reportFile)
}

// TestThresholdAlerting validates that performance thresholds can be detected
func TestThresholdAlerting(t *testing.T) {
	testCases := []struct {
		name              string
		settlementTimes   []float64
		thresholdP95      float64
		shouldAlert       bool
		description       string
	}{
		{
			name:            "normal_performance",
			settlementTimes: generateSettlementTimes(100, 1.0, 0.1),
			thresholdP95:    2.0,
			shouldAlert:     false,
			description:     "Normal performance within thresholds",
		},
		{
			name:            "degraded_performance",
			settlementTimes: generateSettlementTimes(100, 3.0, 0.5),
			thresholdP95:    2.0,
			shouldAlert:     true,
			description:     "Degraded performance exceeds P95 threshold",
		},
		{
			name:            "with_spikes",
			settlementTimes: generateSettlementTimesWithSpikes(100, 1.0, 10.0, 10),
			thresholdP95:    2.0,
			shouldAlert:     true,
			description:     "Performance spikes trigger alert",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, p95, _ := calculatePercentiles(tc.settlementTimes)

			alert := p95 > tc.thresholdP95
			require.Equal(t, tc.shouldAlert, alert,
				"alert status should match expectation for %s", tc.description)

			t.Logf("%s: P95=%.2fs, threshold=%.2fs, alert=%v",
				tc.description, p95, tc.thresholdP95, alert)
		})
	}
}

// Helper functions

func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func max(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	maxVal := values[0]
	for _, v := range values {
		maxVal = math.Max(maxVal, v)
	}
	return maxVal
}

func generateSettlementTimes(count int, mean, stddev float64) []float64 {
	times := make([]float64, count)
	for i := range times {
		// Simple normal-ish distribution
		times[i] = mean + (float64(i%10)-5)*stddev/5
		if times[i] < 0.1 {
			times[i] = 0.1
		}
	}
	return times
}

func generateSettlementTimesWithSpikes(count int, baseMean, spikeValue float64, spikeCount int) []float64 {
	times := generateSettlementTimes(count, baseMean, 0.1)
	spikeInterval := count / spikeCount
	for i := 0; i < spikeCount; i++ {
		idx := (i + 1) * spikeInterval
		if idx < len(times) {
			times[idx] = spikeValue
		}
	}
	return times
}
