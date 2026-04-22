// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dashboard

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestDashboard_New(t *testing.T) {
	d := New(100)

	if d == nil {
		t.Fatal("expected non-nil dashboard")
	}

	if d.loadMetrics == nil {
		t.Error("expected load metrics to be initialized")
	}

	if d.systemMetrics == nil {
		t.Error("expected system metrics to be initialized")
	}

	if d.display == nil {
		t.Error("expected display to be initialized")
	}

	if d.done == nil {
		t.Error("expected done channel to be initialized")
	}
}

func TestDashboard_NewWithWriter(t *testing.T) {
	var buf bytes.Buffer
	d := NewWithWriter(200, &buf)

	if d == nil {
		t.Fatal("expected non-nil dashboard")
	}

	// Verify custom writer is used
	d.LoadMetrics().RecordTransaction(true, 10*time.Millisecond)
	d.render()

	if buf.Len() == 0 {
		t.Error("expected output to custom writer")
	}
}

func TestDashboard_LoadMetrics(t *testing.T) {
	d := New(100)

	lm := d.LoadMetrics()
	if lm == nil {
		t.Error("expected non-nil load metrics")
	}

	// Test that we can record transactions
	lm.RecordTransaction(true, 5*time.Millisecond)
	snap := lm.GetSnapshot()

	if snap.TotalTxs != 1 {
		t.Errorf("expected 1 transaction, got %d", snap.TotalTxs)
	}
}

func TestDashboard_Start_Stop(t *testing.T) {
	var buf bytes.Buffer
	d := NewWithWriter(100, &buf)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	// Start dashboard in background
	done := make(chan struct{})
	go func() {
		d.Start(ctx, 50*time.Millisecond)
		close(done)
	}()

	// Wait for a few updates
	time.Sleep(150 * time.Millisecond)

	// Should not be done yet
	if d.IsDone() {
		t.Error("dashboard should not be done while running")
	}

	// Cancel context to stop
	cancel()

	// Wait for shutdown
	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Error("dashboard did not stop in time")
	}

	// Should be done now
	if !d.IsDone() {
		t.Error("dashboard should be done after context cancelled")
	}
}

func TestDashboard_StopDirect(t *testing.T) {
	d := New(100)

	// Stop without starting
	d.Stop()

	if !d.IsDone() {
		t.Error("dashboard should be done after Stop()")
	}
}

func TestDashboard_UpdateLoop(t *testing.T) {
	var buf bytes.Buffer
	d := NewWithWriter(100, &buf)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start dashboard
	go d.Start(ctx, 50*time.Millisecond)

	// Wait for initial render
	time.Sleep(100 * time.Millisecond)
	initialLen := buf.Len()

	// Record some transactions
	d.LoadMetrics().RecordTransaction(true, 10*time.Millisecond)
	d.LoadMetrics().RecordTransaction(true, 20*time.Millisecond)
	d.LoadMetrics().RecordTransaction(false, 30*time.Millisecond)

	// Wait for more updates
	time.Sleep(150 * time.Millisecond)

	// Should have more output
	if buf.Len() <= initialLen {
		t.Error("expected more output after updates")
	}

	// Stop
	cancel()
	time.Sleep(100 * time.Millisecond)
}

func TestDashboard_ConcurrentAccess(t *testing.T) {
	var buf bytes.Buffer
	d := NewWithWriter(1000, &buf)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Start dashboard
	go d.Start(ctx, 50*time.Millisecond)

	// Concurrently record transactions
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			success := i%5 != 0 // 80% success rate
			d.LoadMetrics().RecordTransaction(success, time.Duration(i%50)*time.Millisecond)
			time.Sleep(time.Millisecond)
		}
		close(done)
	}()

	// Wait for transactions to complete
	<-done

	// Wait a bit more for final updates
	time.Sleep(100 * time.Millisecond)

	// Verify metrics
	snap := d.LoadMetrics().GetSnapshot()
	if snap.TotalTxs != 100 {
		t.Errorf("expected 100 total txs, got %d", snap.TotalTxs)
	}

	// Should have reasonable success/failure counts
	if snap.SuccessfulTxs < 70 || snap.SuccessfulTxs > 90 {
		t.Errorf("expected ~80 successful txs, got %d", snap.SuccessfulTxs)
	}
}

func TestDashboard_Render(t *testing.T) {
	var buf bytes.Buffer
	d := NewWithWriter(100, &buf)

	// Record some transactions
	d.LoadMetrics().RecordTransaction(true, 10*time.Millisecond)
	d.LoadMetrics().RecordTransaction(true, 20*time.Millisecond)
	d.LoadMetrics().RecordTransaction(false, 30*time.Millisecond)

	// Manually trigger render
	d.render()

	if buf.Len() == 0 {
		t.Error("expected output after render")
	}

	output := buf.String()

	// Check for key sections
	expectedSections := []string{
		"Accumulate Load Test Dashboard",
		"Load Test Metrics:",
		"System Metrics:",
		"Transactions:",
		"Performance:",
	}

	for _, section := range expectedSections {
		if !strings.Contains(output, section) {
			t.Errorf("output missing expected section: %s", section)
		}
	}
}

func TestDashboard_MultipleStops(t *testing.T) {
	d := New(100)

	// Multiple stops should be safe
	d.Stop()
	d.Stop()
	d.Stop()

	if !d.IsDone() {
		t.Error("dashboard should be done")
	}
}

func TestDashboard_SystemMetricsUpdate(t *testing.T) {
	var buf bytes.Buffer
	d := NewWithWriter(100, &buf)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Start dashboard
	go d.Start(ctx, 50*time.Millisecond)

	// Wait for system metrics to be collected
	time.Sleep(150 * time.Millisecond)

	// Check output contains system metrics
	output := buf.String()
	if !strings.Contains(output, "CPU:") && !strings.Contains(output, "Memory:") {
		t.Error("expected system metrics in output")
	}
}

func TestDashboard_EmptyMetrics(t *testing.T) {
	var buf bytes.Buffer
	d := NewWithWriter(100, &buf)

	// Render without any transactions
	d.render()

	if buf.Len() == 0 {
		t.Error("expected output even with no transactions")
	}

	snap := d.LoadMetrics().GetSnapshot()
	if snap.TotalTxs != 0 {
		t.Errorf("expected 0 transactions, got %d", snap.TotalTxs)
	}
}

func TestDashboard_HighErrorRate(t *testing.T) {
	var buf bytes.Buffer
	d := NewWithWriter(100, &buf)

	// Record mostly failed transactions
	for i := 0; i < 10; i++ {
		d.LoadMetrics().RecordTransaction(false, 10*time.Millisecond)
	}
	for i := 0; i < 2; i++ {
		d.LoadMetrics().RecordTransaction(true, 10*time.Millisecond)
	}

	d.render()

	snap := d.LoadMetrics().GetSnapshot()
	if snap.ErrorRate < 80 || snap.ErrorRate > 85 {
		t.Errorf("expected ~83%% error rate, got %.2f%%", snap.ErrorRate)
	}
}
