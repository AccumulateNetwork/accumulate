// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dashboard

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestDisplay_New(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(&buf)

	if d == nil {
		t.Fatal("expected non-nil display")
	}

	if d.writer == nil {
		t.Error("expected writer to be set")
	}

	if d.width <= 0 {
		t.Error("expected positive width")
	}

	if d.height <= 0 {
		t.Error("expected positive height")
	}
}

func TestDisplay_Clear(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(&buf)

	d.Clear()

	output := buf.String()
	if len(output) == 0 {
		t.Error("expected clear output")
	}

	// Should contain ANSI escape codes
	if !strings.Contains(output, "\033[") {
		t.Error("expected ANSI escape codes in clear output")
	}
}

func TestDisplay_Show(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(&buf)

	d.Show()

	output := buf.String()
	if len(output) == 0 {
		t.Error("expected show cursor output")
	}
}

func TestDisplay_ProgressBar_Boundaries(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(&buf)

	tests := []struct {
		name    string
		percent float64
		width   int
		wantLen int
	}{
		{"zero percent", 0, 10, 10},
		{"fifty percent", 50, 20, 20},
		{"full", 100, 10, 10},
		{"negative clamped", -10, 10, 10},
		{"over 100 clamped", 150, 10, 10},
		{"small width", 50, 5, 5},
		{"large width", 50, 50, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bar := d.progressBar(tt.percent, tt.width)
			if bar == "" {
				t.Error("progressBar returned empty string")
			}

			// Should contain opening and closing brackets
			if !strings.Contains(bar, "[") || !strings.Contains(bar, "]") {
				t.Error("progress bar should contain brackets")
			}

			// Check color codes are present
			if !strings.Contains(bar, "\033[") {
				t.Error("progress bar should contain ANSI color codes")
			}
		})
	}
}

func TestDisplay_ProgressBar_ColorCoding(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(&buf)

	tests := []struct {
		percent     float64
		expectColor string
		description string
	}{
		{10, colorGreen, "low percent should be green"},
		{50, colorGreen, "medium percent should be green"},
		{75, colorYellow, "75% should be yellow"},
		{85, colorYellow, "85% should be yellow"},
		{95, colorRed, "95% should be red"},
		{100, colorRed, "100% should be red"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			bar := d.progressBar(tt.percent, 20)
			if !strings.Contains(bar, tt.expectColor) {
				t.Errorf("expected color %s in progress bar for %.0f%%", tt.expectColor, tt.percent)
			}
		})
	}
}

func TestDisplay_Render_Complete(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(&buf)

	load := NewLoadMetrics(1000)
	load.RecordTransaction(true, 10*time.Millisecond)
	load.RecordTransaction(true, 20*time.Millisecond)
	load.RecordTransaction(false, 30*time.Millisecond)

	sys := NewSystemMetrics()
	_ = sys.Update()

	d.Render(load, sys)

	output := buf.String()

	if buf.Len() == 0 {
		t.Fatal("expected render output")
	}

	// Check for all major sections
	expectedSections := []string{
		"Accumulate Load Test Dashboard",
		"Load Test Metrics:",
		"System Metrics:",
		"Status:",
		"Duration:",
		"Transactions:",
		"Performance:",
		"Latency:",
		"Error Rate:",
		"CPU:",
		"Memory:",
		"Disk:",
		"Network:",
		"Last update:",
	}

	for _, section := range expectedSections {
		if !strings.Contains(output, section) {
			t.Errorf("output missing expected section: %s", section)
		}
	}
}

func TestDisplay_RenderHeader(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(&buf)

	var sb strings.Builder
	d.renderHeader(&sb)

	output := sb.String()

	if !strings.Contains(output, "Accumulate Load Test Dashboard") {
		t.Error("header missing title")
	}

	// Should have border characters
	if !strings.Contains(output, "=") {
		t.Error("header missing border")
	}

	// Should have color codes
	if !strings.Contains(output, "\033[") {
		t.Error("header missing color codes")
	}
}

func TestDisplay_RenderLoadMetrics_StatusColors(t *testing.T) {
	tests := []struct {
		name           string
		successTxs     int
		failedTxs      int
		expectedStatus string
	}{
		{"healthy", 100, 0, "Running"},
		{"degraded", 85, 15, "Degraded"},
		{"critical", 40, 60, "Critical"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			d := NewDisplay(&buf)
			load := NewLoadMetrics(1000)

			// Record transactions
			for i := 0; i < tt.successTxs; i++ {
				load.RecordTransaction(true, 10*time.Millisecond)
			}
			for i := 0; i < tt.failedTxs; i++ {
				load.RecordTransaction(false, 10*time.Millisecond)
			}

			var sb strings.Builder
			d.renderLoadMetrics(&sb, load)
			output := sb.String()

			if !strings.Contains(output, tt.expectedStatus) {
				t.Errorf("expected status %s in output", tt.expectedStatus)
			}
		})
	}
}

func TestDisplay_RenderSystemMetrics_ColorThresholds(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(&buf)
	sys := NewSystemMetrics()

	// Force update to get some metrics
	_ = sys.Update()

	var sb strings.Builder
	d.renderSystemMetrics(&sb, sys)
	output := sb.String()

	// Should contain system metric labels
	expectedLabels := []string{"CPU:", "Memory:", "Disk:", "Network:"}
	for _, label := range expectedLabels {
		if !strings.Contains(output, label) {
			t.Errorf("output missing label: %s", label)
		}
	}

	// Should contain units
	expectedUnits := []string{"%", "GB", "MB/s"}
	foundUnit := false
	for _, unit := range expectedUnits {
		if strings.Contains(output, unit) {
			foundUnit = true
			break
		}
	}
	if !foundUnit {
		t.Error("output missing expected units")
	}
}

func TestDisplay_RenderFooter(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(&buf)

	var sb strings.Builder
	d.renderFooter(&sb)
	output := sb.String()

	if !strings.Contains(output, "Last update:") {
		t.Error("footer missing last update")
	}

	if !strings.Contains(output, "Ctrl+C") {
		t.Error("footer missing stop instruction")
	}
}

func TestDisplay_Separator(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(&buf)

	sep := d.separator()

	if sep == "" {
		t.Error("separator should not be empty")
	}

	if !strings.Contains(sep, "-") {
		t.Error("separator should contain dashes")
	}

	// Should be as wide as display width
	// (accounting for ANSI codes, count actual dashes)
	dashCount := strings.Count(sep, "-")
	if dashCount != d.width {
		t.Errorf("expected %d dashes, got %d", d.width, dashCount)
	}
}

func TestDisplay_RenderWithNoTransactions(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(&buf)

	load := NewLoadMetrics(100)
	sys := NewSystemMetrics()

	// Render with empty metrics
	d.Render(load, sys)

	output := buf.String()

	if buf.Len() == 0 {
		t.Error("expected output even with no transactions")
	}

	// Should still show structure
	if !strings.Contains(output, "Accumulate Load Test Dashboard") {
		t.Error("output missing header")
	}
}

func TestDisplay_RenderWithProgressBar(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(&buf)

	load := NewLoadMetrics(100) // Target 100 txs

	// Record 50 transactions (50% progress)
	for i := 0; i < 50; i++ {
		load.RecordTransaction(true, 10*time.Millisecond)
	}

	var sb strings.Builder
	d.renderLoadMetrics(&sb, load)
	output := sb.String()

	// Should show progress bar
	if !strings.Contains(output, "Progress:") {
		t.Error("output missing progress bar")
	}

	if !strings.Contains(output, "[") || !strings.Contains(output, "]") {
		t.Error("progress bar should have brackets")
	}
}

func TestDisplay_ANSI_Codes(t *testing.T) {
	// Test that ANSI constants are defined
	codes := []string{
		clearScreen,
		moveCursor,
		hideCursor,
		showCursor,
		resetStyle,
		bold,
		dim,
		colorReset,
		colorRed,
		colorGreen,
		colorYellow,
		colorBlue,
		colorCyan,
		colorWhite,
		colorGray,
	}

	for _, code := range codes {
		if code == "" {
			t.Error("ANSI code constant is empty")
		}
		if !strings.HasPrefix(code, "\033") {
			t.Errorf("ANSI code should start with escape character: %q", code)
		}
	}
}

func TestDisplay_RenderLatencyMetrics(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(&buf)

	load := NewLoadMetrics(1000)

	// Add varying latencies
	latencies := []time.Duration{
		5 * time.Millisecond,
		10 * time.Millisecond,
		15 * time.Millisecond,
		20 * time.Millisecond,
		100 * time.Millisecond,
	}

	for _, lat := range latencies {
		load.RecordTransaction(true, lat)
	}

	var sb strings.Builder
	d.renderLoadMetrics(&sb, load)
	output := sb.String()

	// Should show latency percentiles
	if !strings.Contains(output, "Latency:") {
		t.Error("output missing latency section")
	}

	// Should mention percentiles
	if !strings.Contains(output, "avg") || !strings.Contains(output, "p95") || !strings.Contains(output, "p99") {
		t.Error("output missing latency percentile labels")
	}
}
