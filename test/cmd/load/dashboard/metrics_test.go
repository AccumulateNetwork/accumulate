// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dashboard

import (
	"testing"
	"time"
)

func TestLoadMetrics_RecordTransaction(t *testing.T) {
	m := NewLoadMetrics(100)

	// Record successful transaction
	m.RecordTransaction(true, 50*time.Millisecond)

	snap := m.GetSnapshot()
	if snap.TotalTxs != 1 {
		t.Errorf("expected 1 total tx, got %d", snap.TotalTxs)
	}
	if snap.SuccessfulTxs != 1 {
		t.Errorf("expected 1 successful tx, got %d", snap.SuccessfulTxs)
	}
	if snap.FailedTxs != 0 {
		t.Errorf("expected 0 failed txs, got %d", snap.FailedTxs)
	}

	// Record failed transaction
	m.RecordTransaction(false, 100*time.Millisecond)

	snap = m.GetSnapshot()
	if snap.TotalTxs != 2 {
		t.Errorf("expected 2 total txs, got %d", snap.TotalTxs)
	}
	if snap.SuccessfulTxs != 1 {
		t.Errorf("expected 1 successful tx, got %d", snap.SuccessfulTxs)
	}
	if snap.FailedTxs != 1 {
		t.Errorf("expected 1 failed tx, got %d", snap.FailedTxs)
	}
}

func TestLoadMetrics_TPS_Calculation(t *testing.T) {
	m := NewLoadMetrics(1000)

	// Record 10 transactions over 1 second
	start := time.Now()
	for i := 0; i < 10; i++ {
		m.RecordTransaction(true, 10*time.Millisecond)
		time.Sleep(10 * time.Millisecond)
	}
	elapsed := time.Since(start)

	snap := m.GetSnapshot()

	expectedTPS := float64(10) / elapsed.Seconds()
	tolerance := 2.0 // Allow 2 TPS difference for timing variations

	if snap.CurrentTPS < expectedTPS-tolerance || snap.CurrentTPS > expectedTPS+tolerance {
		t.Errorf("expected TPS ~%.2f, got %.2f", expectedTPS, snap.CurrentTPS)
	}
}

func TestLoadMetrics_ErrorRate(t *testing.T) {
	m := NewLoadMetrics(100)

	// Record 7 successful, 3 failed = 30% error rate
	for i := 0; i < 7; i++ {
		m.RecordTransaction(true, 10*time.Millisecond)
	}
	for i := 0; i < 3; i++ {
		m.RecordTransaction(false, 10*time.Millisecond)
	}

	snap := m.GetSnapshot()
	expectedErrorRate := 30.0
	tolerance := 1.0

	if snap.ErrorRate < expectedErrorRate-tolerance || snap.ErrorRate > expectedErrorRate+tolerance {
		t.Errorf("expected error rate ~%.1f%%, got %.1f%%", expectedErrorRate, snap.ErrorRate)
	}
}

func TestLoadMetrics_Latency_Percentiles(t *testing.T) {
	m := NewLoadMetrics(1000)

	// Record transactions with varying latencies
	latencies := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
		60 * time.Millisecond,
		70 * time.Millisecond,
		80 * time.Millisecond,
		90 * time.Millisecond,
		100 * time.Millisecond,
	}

	for _, lat := range latencies {
		m.RecordTransaction(true, lat)
	}

	snap := m.GetSnapshot()

	// Check that p95 and p99 are calculated and make sense
	if snap.P95Latency == 0 {
		t.Error("expected non-zero p95 latency")
	}
	if snap.P99Latency == 0 {
		t.Error("expected non-zero p99 latency")
	}
	if snap.P95Latency > snap.P99Latency {
		t.Errorf("p95 (%v) should be less than p99 (%v)", snap.P95Latency, snap.P99Latency)
	}
	if snap.AvgLatency == 0 {
		t.Error("expected non-zero average latency")
	}
}

func TestLoadMetrics_PeakTPS(t *testing.T) {
	m := NewLoadMetrics(1000)

	// Record transactions at a steady rate
	for i := 0; i < 10; i++ {
		m.RecordTransaction(true, 10*time.Millisecond)
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for window to complete
	time.Sleep(1100 * time.Millisecond)

	snap := m.GetSnapshot()
	peakTPS := snap.PeakTPS

	// Peak should be non-zero
	if peakTPS == 0 {
		t.Error("expected non-zero peak TPS")
	}

	// Record more transactions slower (should lower current TPS but not peak)
	for i := 0; i < 5; i++ {
		m.RecordTransaction(true, 10*time.Millisecond)
		time.Sleep(300 * time.Millisecond)
	}

	snap = m.GetSnapshot()

	// Peak should be greater than or equal to current
	if snap.PeakTPS < snap.CurrentTPS {
		t.Errorf("peak TPS (%.2f) should be >= current TPS (%.2f)", snap.PeakTPS, snap.CurrentTPS)
	}

	// Peak should not have decreased (allowing small floating point tolerance)
	if snap.PeakTPS < peakTPS-0.01 {
		t.Errorf("peak TPS decreased from %.2f to %.2f", peakTPS, snap.PeakTPS)
	}
}

func TestLoadMetrics_LatencyWindowLimit(t *testing.T) {
	m := NewLoadMetrics(1000)

	// Record more than maxWindow latencies
	for i := 0; i < 1500; i++ {
		m.RecordTransaction(true, time.Duration(i)*time.Millisecond)
	}

	// Check that latency window is limited
	m.mu.RLock()
	latencyCount := len(m.latencies)
	m.mu.RUnlock()

	if latencyCount > m.maxWindow {
		t.Errorf("latency window should be limited to %d, got %d", m.maxWindow, latencyCount)
	}

	if latencyCount != m.maxWindow {
		t.Errorf("expected latency window size %d, got %d", m.maxWindow, latencyCount)
	}
}

func TestLoadMetrics_AvgTPS_Calculation(t *testing.T) {
	m := NewLoadMetrics(1000)

	// Record transactions over a known period
	for i := 0; i < 50; i++ {
		m.RecordTransaction(true, 10*time.Millisecond)
		time.Sleep(10 * time.Millisecond)
	}

	snap := m.GetSnapshot()

	// Average TPS should be calculated from total time elapsed
	elapsed := time.Since(snap.StartTime).Seconds()
	expectedAvgTPS := float64(snap.TotalTxs) / elapsed

	tolerance := 2.0
	if snap.AvgTPS < expectedAvgTPS-tolerance || snap.AvgTPS > expectedAvgTPS+tolerance {
		t.Errorf("expected avg TPS ~%.2f, got %.2f", expectedAvgTPS, snap.AvgTPS)
	}
}

func TestLoadMetrics_ZeroLatencies(t *testing.T) {
	m := NewLoadMetrics(100)

	// Don't record any transactions
	snap := m.GetSnapshot()

	if snap.AvgLatency != 0 {
		t.Errorf("expected zero average latency, got %v", snap.AvgLatency)
	}

	if snap.P95Latency != 0 {
		t.Errorf("expected zero P95 latency, got %v", snap.P95Latency)
	}

	if snap.P99Latency != 0 {
		t.Errorf("expected zero P99 latency, got %v", snap.P99Latency)
	}
}

func TestLoadMetrics_SingleLatency(t *testing.T) {
	m := NewLoadMetrics(100)

	latency := 42 * time.Millisecond
	m.RecordTransaction(true, latency)

	snap := m.GetSnapshot()

	// With only one latency, all percentiles should be the same
	if snap.AvgLatency != latency {
		t.Errorf("expected avg latency %v, got %v", latency, snap.AvgLatency)
	}

	if snap.P95Latency != latency {
		t.Errorf("expected P95 latency %v, got %v", latency, snap.P95Latency)
	}

	if snap.P99Latency != latency {
		t.Errorf("expected P99 latency %v, got %v", latency, snap.P99Latency)
	}
}

func TestLoadMetrics_TargetTxs(t *testing.T) {
	targetTxs := int64(500)
	m := NewLoadMetrics(targetTxs)

	snap := m.GetSnapshot()

	if snap.TargetTxs != targetTxs {
		t.Errorf("expected target txs %d, got %d", targetTxs, snap.TargetTxs)
	}
}

func TestLoadMetrics_WindowReset(t *testing.T) {
	m := NewLoadMetrics(1000)

	// Record transactions in first window
	for i := 0; i < 10; i++ {
		m.RecordTransaction(true, 10*time.Millisecond)
	}

	// Wait for window to expire
	time.Sleep(1100 * time.Millisecond)

	// Record more transactions in new window
	for i := 0; i < 5; i++ {
		m.RecordTransaction(true, 10*time.Millisecond)
	}

	// Current TPS should be based on new window
	snap := m.GetSnapshot()

	// Current TPS should be lower than peak
	if snap.CurrentTPS > snap.PeakTPS {
		t.Errorf("current TPS (%.2f) should not exceed peak TPS (%.2f)", snap.CurrentTPS, snap.PeakTPS)
	}
}

func TestLoadMetrics_ErrorRate_ZeroTransactions(t *testing.T) {
	m := NewLoadMetrics(100)

	snap := m.GetSnapshot()

	// Error rate should be 0 when no transactions recorded
	if snap.ErrorRate != 0 {
		t.Errorf("expected 0%% error rate with no transactions, got %.2f%%", snap.ErrorRate)
	}
}

func TestLoadMetrics_ErrorRate_AllSuccess(t *testing.T) {
	m := NewLoadMetrics(100)

	for i := 0; i < 50; i++ {
		m.RecordTransaction(true, 10*time.Millisecond)
	}

	snap := m.GetSnapshot()

	if snap.ErrorRate != 0 {
		t.Errorf("expected 0%% error rate with all success, got %.2f%%", snap.ErrorRate)
	}
}

func TestLoadMetrics_ErrorRate_AllFailed(t *testing.T) {
	m := NewLoadMetrics(100)

	for i := 0; i < 50; i++ {
		m.RecordTransaction(false, 10*time.Millisecond)
	}

	snap := m.GetSnapshot()

	if snap.ErrorRate != 100 {
		t.Errorf("expected 100%% error rate with all failures, got %.2f%%", snap.ErrorRate)
	}
}

