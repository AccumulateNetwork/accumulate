// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dashboard

import (
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	// ANSI escape codes
	clearScreen = "\033[2J"
	moveCursor  = "\033[H"
	hideCursor  = "\033[?25l"
	showCursor  = "\033[?25h"
	resetStyle  = "\033[0m"
	bold        = "\033[1m"
	dim         = "\033[2m"

	// Colors
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"
	colorGray   = "\033[90m"
)

// Display handles terminal UI rendering
type Display struct {
	writer io.Writer
	width  int
	height int
}

// NewDisplay creates a new display renderer
func NewDisplay(w io.Writer) *Display {
	return &Display{
		writer: w,
		width:  120,
		height: 40,
	}
}

// Clear clears the screen and hides cursor
func (d *Display) Clear() {
	fmt.Fprint(d.writer, clearScreen+moveCursor+hideCursor)
}

// Show displays cursor
func (d *Display) Show() {
	fmt.Fprint(d.writer, showCursor)
}

// Render renders the complete dashboard
func (d *Display) Render(load *LoadMetrics, sys *SystemMetrics) {
	var sb strings.Builder

	// Move to top and clear
	sb.WriteString(moveCursor)

	// Header
	d.renderHeader(&sb)
	sb.WriteString("\n")

	// Load test metrics
	d.renderLoadMetrics(&sb, load)
	sb.WriteString("\n")

	// System metrics
	d.renderSystemMetrics(&sb, sys)
	sb.WriteString("\n")

	// Footer
	d.renderFooter(&sb)

	fmt.Fprint(d.writer, sb.String())
}

func (d *Display) renderHeader(sb *strings.Builder) {
	title := " Accumulate Load Test Dashboard "
	padding := (d.width - len(title)) / 2
	if padding < 0 {
		padding = 0
	}

	sb.WriteString(bold + colorCyan)
	sb.WriteString(strings.Repeat("=", d.width))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat(" ", padding))
	sb.WriteString(title)
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("=", d.width))
	sb.WriteString(resetStyle)
	sb.WriteString("\n")
}

func (d *Display) renderLoadMetrics(sb *strings.Builder, m *LoadMetrics) {
	snap := m.GetSnapshot()

	sb.WriteString(bold + "Load Test Metrics:" + resetStyle + "\n")
	sb.WriteString(d.separator())

	// Status and duration
	status := "Running"
	statusColor := colorGreen
	if snap.ErrorRate > 10 {
		status = "Degraded"
		statusColor = colorYellow
	}
	if snap.ErrorRate > 50 {
		status = "Critical"
		statusColor = colorRed
	}

	duration := time.Since(snap.StartTime).Round(time.Second)

	sb.WriteString(fmt.Sprintf("  Status:   %s%s%s\n", statusColor, status, colorReset))
	sb.WriteString(fmt.Sprintf("  Duration: %s\n", duration))
	sb.WriteString("\n")

	// Transaction metrics
	sb.WriteString(bold + "  Transactions:" + resetStyle + "\n")
	sb.WriteString(fmt.Sprintf("    Total:     %s%-12d%s", colorCyan, snap.TotalTxs, colorReset))
	sb.WriteString(fmt.Sprintf("    Success:   %s%-12d%s", colorGreen, snap.SuccessfulTxs, colorReset))
	sb.WriteString(fmt.Sprintf("    Failed:    %s%-12d%s\n", colorRed, snap.FailedTxs, colorReset))
	sb.WriteString("\n")

	// Rate and latency
	sb.WriteString(bold + "  Performance:" + resetStyle + "\n")
	sb.WriteString(fmt.Sprintf("    Rate:      %s%.2f tx/s%s\n", colorCyan, snap.CurrentTPS, colorReset))
	sb.WriteString(fmt.Sprintf("    Avg TPS:   %s%.2f tx/s%s\n", colorCyan, snap.AvgTPS, colorReset))
	sb.WriteString(fmt.Sprintf("    Peak TPS:  %s%.2f tx/s%s\n", colorGreen, snap.PeakTPS, colorReset))
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("    Latency:   %s%-10s%s (avg)", colorYellow, snap.AvgLatency.Round(time.Millisecond), colorReset))
	sb.WriteString(fmt.Sprintf("  %s%-10s%s (p95)", colorYellow, snap.P95Latency.Round(time.Millisecond), colorReset))
	sb.WriteString(fmt.Sprintf("  %s%-10s%s (p99)\n", colorYellow, snap.P99Latency.Round(time.Millisecond), colorReset))
	sb.WriteString("\n")

	// Error rate
	errorColor := colorGreen
	if snap.ErrorRate > 10 {
		errorColor = colorYellow
	}
	if snap.ErrorRate > 50 {
		errorColor = colorRed
	}
	sb.WriteString(fmt.Sprintf("    Error Rate: %s%.2f%%%s\n", errorColor, snap.ErrorRate, colorReset))

	// Progress bar
	if snap.TargetTxs > 0 {
		progress := float64(snap.TotalTxs) / float64(snap.TargetTxs) * 100
		if progress > 100 {
			progress = 100
		}
		sb.WriteString(fmt.Sprintf("    Progress:   %s\n", d.progressBar(progress, 50)))
	}
}

func (d *Display) renderSystemMetrics(sb *strings.Builder, sm *SystemMetrics) {
	snap := sm.GetSnapshot()

	sb.WriteString(bold + "System Metrics:" + resetStyle + "\n")
	sb.WriteString(d.separator())

	// CPU
	cpuColor := colorGreen
	if snap.CPUPercent > 70 {
		cpuColor = colorYellow
	}
	if snap.CPUPercent > 90 {
		cpuColor = colorRed
	}
	sb.WriteString(fmt.Sprintf("  CPU:     %s%5.1f%%%s  %s\n",
		cpuColor, snap.CPUPercent, colorReset,
		d.progressBar(snap.CPUPercent, 40)))

	// Memory
	var memPercent float64
	if snap.MemoryTotalMB > 0 {
		memPercent = snap.MemoryUsedMB / snap.MemoryTotalMB * 100
	}
	memColor := colorGreen
	if memPercent > 70 {
		memColor = colorYellow
	}
	if memPercent > 90 {
		memColor = colorRed
	}
	sb.WriteString(fmt.Sprintf("  Memory:  %s%5.1f%%%s  %s  (%.1f / %.1f GB)\n",
		memColor, memPercent, colorReset,
		d.progressBar(memPercent, 40),
		snap.MemoryUsedMB/1024, snap.MemoryTotalMB/1024))

	// Disk I/O
	sb.WriteString(fmt.Sprintf("  Disk:    %sRead: %7.2f MB/s%s  %sWrite: %7.2f MB/s%s\n",
		colorCyan, snap.DiskReadMB, colorReset,
		colorCyan, snap.DiskWriteMB, colorReset))

	// Network
	sb.WriteString(fmt.Sprintf("  Network: %sRx: %7.2f MB/s%s     %sTx: %7.2f MB/s%s\n",
		colorCyan, snap.NetworkRxMB, colorReset,
		colorCyan, snap.NetworkTxMB, colorReset))
}

func (d *Display) renderFooter(sb *strings.Builder) {
	sb.WriteString(d.separator())
	sb.WriteString(colorGray)
	sb.WriteString(fmt.Sprintf("  Last update: %s", time.Now().Format("15:04:05")))
	sb.WriteString("  |  Press Ctrl+C to stop")
	sb.WriteString(colorReset)
	sb.WriteString("\n")
}

func (d *Display) separator() string {
	return colorGray + strings.Repeat("-", d.width) + colorReset + "\n"
}

func (d *Display) progressBar(percent float64, width int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	filled := int(percent / 100 * float64(width))
	empty := width - filled

	color := colorGreen
	if percent > 70 {
		color = colorYellow
	}
	if percent > 90 {
		color = colorRed
	}

	bar := color + "[" +
		strings.Repeat("=", filled) +
		strings.Repeat(" ", empty) +
		"]" + colorReset

	return bar
}
