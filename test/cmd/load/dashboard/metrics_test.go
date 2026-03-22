// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dashboard

import (
	"bytes"
	"context"
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

func TestSystemMetrics_Update(t *testing.T) {
	sm := NewSystemMetrics()

	// Update should not error
	if err := sm.Update(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	snap := sm.GetSnapshot()

	// Basic sanity checks (values depend on system)
	if snap.CPUPercent < 0 || snap.CPUPercent > 100 {
		t.Errorf("invalid CPU percent: %.2f", snap.CPUPercent)
	}
	if snap.MemoryTotalMB <= 0 {
		t.Errorf("invalid total memory: %.2f MB", snap.MemoryTotalMB)
	}
	if snap.MemoryUsedMB < 0 {
		t.Errorf("invalid used memory: %.2f MB", snap.MemoryUsedMB)
	}
}

func TestDashboard_Lifecycle(t *testing.T) {
	var buf bytes.Buffer
	d := NewWithWriter(100, &buf)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start dashboard in background
	go d.Start(ctx, 100*time.Millisecond)

	// Wait for a few updates
	time.Sleep(350 * time.Millisecond)

	// Record some transactions
	d.LoadMetrics().RecordTransaction(true, 10*time.Millisecond)
	d.LoadMetrics().RecordTransaction(false, 20*time.Millisecond)

	// Wait for update
	time.Sleep(150 * time.Millisecond)

	// Stop dashboard
	cancel()
	time.Sleep(50 * time.Millisecond)

	// Check that we got some output
	if buf.Len() == 0 {
		t.Error("expected dashboard output, got none")
	}

	// Check that dashboard stopped
	if !d.IsDone() {
		t.Error("dashboard should be done")
	}
}

func TestDashboard_Stop_Idempotent(t *testing.T) {
	d := New(100)

	// Stop should be safe to call multiple times
	d.Stop()
	d.Stop()
	d.Stop()

	if !d.IsDone() {
		t.Error("dashboard should be done after Stop()")
	}
}

func TestDisplay_ProgressBar(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(&buf)

	tests := []struct {
		percent float64
		width   int
	}{
		{0, 10},
		{50, 10},
		{100, 10},
		{75, 20},
		{-10, 10}, // Should clamp to 0
		{150, 10}, // Should clamp to 100
	}

	for _, tt := range tests {
		bar := d.progressBar(tt.percent, tt.width)
		if bar == "" {
			t.Errorf("progressBar(%.1f, %d) returned empty string", tt.percent, tt.width)
		}
	}
}

func TestDisplay_Render(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(&buf)

	load := NewLoadMetrics(1000)
	load.RecordTransaction(true, 10*time.Millisecond)
	load.RecordTransaction(true, 20*time.Millisecond)

	sys := NewSystemMetrics()
	sys.Update()

	// Should not panic
	d.Render(load, sys)

	// Should have written something
	if buf.Len() == 0 {
		t.Error("expected render output, got none")
	}

	output := buf.String()

	// Check for expected sections
	expectedSections := []string{
		"Accumulate Load Test Dashboard",
		"Load Test Metrics:",
		"System Metrics:",
		"Last update:",
	}

	for _, section := range expectedSections {
		if !bytes.Contains([]byte(output), []byte(section)) {
			t.Errorf("output missing expected section: %s", section)
		}
	}
}

func TestLoadMetrics_ConcurrentAccess(t *testing.T) {
	m := NewLoadMetrics(1000)

	// Simulate concurrent access
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			m.RecordTransaction(true, 10*time.Millisecond)
			time.Sleep(time.Millisecond)
		}
		close(done)
	}()

	// Read snapshots concurrently
	for i := 0; i < 100; i++ {
		_ = m.GetSnapshot()
		time.Sleep(time.Millisecond)
	}

	<-done

	snap := m.GetSnapshot()
	if snap.TotalTxs != 100 {
		t.Errorf("expected 100 total txs, got %d", snap.TotalTxs)
	}
}

func TestSystemMetrics_ConcurrentAccess(t *testing.T) {
	sm := NewSystemMetrics()

	// Simulate concurrent access
	done := make(chan struct{})
	go func() {
		for i := 0; i < 10; i++ {
			sm.Update()
			time.Sleep(10 * time.Millisecond)
		}
		close(done)
	}()

	// Read snapshots concurrently
	for i := 0; i < 10; i++ {
		_ = sm.GetSnapshot()
		time.Sleep(10 * time.Millisecond)
	}

	<-done
}
