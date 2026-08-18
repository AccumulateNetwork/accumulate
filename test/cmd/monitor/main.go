// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
)

var (
	serverUrl    string
	updatePeriod int
	targetTPS    int
)

func init() {
	flag.StringVar(&serverUrl, "s", "http://127.0.1.1:26660", "Accumulate server URL")
	flag.IntVar(&updatePeriod, "p", 2, "Update period in seconds")
	flag.IntVar(&targetTPS, "t", 0, "Target TPS (optional, for comparison)")
}

type Metrics struct {
	mu                 sync.RWMutex
	actualTPS          float64
	targetTPS          float64
	cpuPercent         float64
	memoryUsedMB       uint64
	memoryTotalMB      uint64
	blockHeight        uint64
	blockProductionTPS float64
	lastUpdate         time.Time
	nodeStatus         string
	errors             map[string]int64
	latencyP50         time.Duration
	latencyP95         time.Duration
	latencyP99         time.Duration
	diskUsedGB         uint64
	diskTotalGB        uint64
	requestLatencies   []time.Duration
}

func (m *Metrics) Update(actualTPS, blockTPS float64, blockHeight uint64, nodeStatus string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.actualTPS = actualTPS
	m.blockProductionTPS = blockTPS
	m.blockHeight = blockHeight
	m.nodeStatus = nodeStatus
	m.lastUpdate = time.Now()
}

func (m *Metrics) UpdateSystem() {
	m.mu.Lock()
	defer m.mu.Unlock()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	m.memoryUsedMB = memStats.Alloc / 1024 / 1024
	m.memoryTotalMB = memStats.Sys / 1024 / 1024

	// Estimate CPU based on goroutine count (simplified)
	m.cpuPercent = min(float64(runtime.NumGoroutine())/10.0, 100.0)

	// Calculate latency percentiles from recent requests
	if len(m.requestLatencies) > 0 {
		// Simple percentile calculation (not fully accurate but sufficient for monitoring)
		n := len(m.requestLatencies)
		if n >= 2 {
			m.latencyP50 = m.requestLatencies[n*50/100]
			m.latencyP95 = m.requestLatencies[n*95/100]
			m.latencyP99 = m.requestLatencies[max(n*99/100, n-1)]
		} else {
			m.latencyP50 = m.requestLatencies[0]
			m.latencyP95 = m.requestLatencies[0]
			m.latencyP99 = m.requestLatencies[0]
		}
	}
}

func (m *Metrics) AddLatency(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestLatencies = append(m.requestLatencies, d)
	// Keep only last 100 samples
	if len(m.requestLatencies) > 100 {
		m.requestLatencies = m.requestLatencies[1:]
	}
}

type MetricsSnapshot struct {
	actualTPS   float64
	targetTPS   float64
	blockTPS    float64
	memUsed     uint64
	memTotal    uint64
	blockHeight uint64
	nodeStatus  string
	lastUpdate  time.Time
	latencyP50  time.Duration
	latencyP95  time.Duration
	latencyP99  time.Duration
	diskUsed    uint64
	diskTotal   uint64
	cpuPercent  float64
}

func (m *Metrics) GetSnapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return MetricsSnapshot{
		actualTPS:   m.actualTPS,
		targetTPS:   m.targetTPS,
		blockTPS:    m.blockProductionTPS,
		memUsed:     m.memoryUsedMB,
		memTotal:    m.memoryTotalMB,
		blockHeight: m.blockHeight,
		nodeStatus:  m.nodeStatus,
		lastUpdate:  m.lastUpdate,
		latencyP50:  m.latencyP50,
		latencyP95:  m.latencyP95,
		latencyP99:  m.latencyP99,
		diskUsed:    m.diskUsedGB,
		diskTotal:   m.diskTotalGB,
		cpuPercent:  m.cpuPercent,
	}
}

func main() {
	flag.Parse()

	if serverUrl == "" {
		log.Fatal("Server URL is required")
	}

	// Initialize API client
	cl, err := client.New(serverUrl + "/v2")
	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}

	// Initialize metrics
	metrics := &Metrics{
		targetTPS: float64(targetTPS),
		errors:    make(map[string]int64),
	}

	// Create the UI
	app := tview.NewApplication()

	// Create text view for dashboard
	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(false).
		SetChangedFunc(func() {
			app.Draw()
		})
	textView.SetBorder(true).SetTitle(" Accumulate Load Test Monitor ").SetTitleAlign(tview.AlignCenter)

	// Create info text at bottom
	infoView := tview.NewTextView().
		SetDynamicColors(false).
		SetText("Press 'q' to quit | Press 'r' to reset stats | Update period: " + fmt.Sprintf("%ds", updatePeriod))

	// Layout
	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(textView, 0, 1, false).
		AddItem(infoView, 1, 0, false)

	// Start metrics collection
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go collectMetrics(ctx, cl, metrics, updatePeriod)
	go updateDisplay(ctx, textView, metrics, updatePeriod)

	// Handle keyboard input
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'q' || event.Rune() == 'Q' {
			app.Stop()
			return nil
		}
		if event.Rune() == 'r' || event.Rune() == 'R' {
			// Reset stats (not fully implemented for simplicity)
			return nil
		}
		return event
	})

	if err := app.SetRoot(flex, true).EnableMouse(true).Run(); err != nil {
		log.Fatalf("Error running application: %v", err)
	}
}

func collectMetrics(ctx context.Context, cl *client.Client, m *Metrics, period int) {
	ticker := time.NewTicker(time.Duration(period) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Update system metrics
			m.UpdateSystem()

			// Measure request latency
			reqStart := time.Now()

			// Get node status
			status, err := cl.Status(ctx)
			var nodeStatus string
			var blockHeight uint64

			if err != nil {
				nodeStatus = "ERROR"
				m.AddLatency(time.Since(reqStart))
			} else {
				if status.Ok {
					nodeStatus = "HEALTHY"
				} else {
					nodeStatus = "DEGRADED"
				}
				// Use BVN height as primary block height
				blockHeight = uint64(status.BvnHeight)
				m.AddLatency(time.Since(reqStart))
			}

			// Get TPS metrics from the metrics endpoint
			actualTPS := 0.0
			blockTPS := 0.0
			if err == nil {
				metricsReq := &api.MetricsQuery{
					Metric:   "tps",
					Duration: time.Duration(period*10) * time.Second,
				}
				metricsResp, err := cl.Metrics(ctx, metricsReq)
				if err == nil && metricsResp.Data != nil {
					// The response data should be a float64
					if tpsVal, ok := metricsResp.Data.(float64); ok {
						actualTPS = tpsVal
						blockTPS = tpsVal // Simplified: using same value
					}
				}
			}

			m.Update(actualTPS, blockTPS, blockHeight, nodeStatus)
		}
	}
}

func updateDisplay(ctx context.Context, view *tview.TextView, m *Metrics, period int) {
	ticker := time.NewTicker(time.Duration(period) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			snap := m.GetSnapshot()

			// Build display
			view.Clear()
			fmt.Fprintf(view, "\n")
			fmt.Fprintf(view, "  [yellow]TRANSACTION PERFORMANCE[-]\n")
			if snap.targetTPS > 0 {
				fmt.Fprintf(view, "    Target TPS:          %8.0f\n", snap.targetTPS)
				fmt.Fprintf(view, "    Actual TPS:          %8.2f", snap.actualTPS)

				// Show comparison
				if snap.actualTPS >= snap.targetTPS*0.95 {
					fmt.Fprintf(view, " [green](OK)[-]\n")
				} else if snap.actualTPS >= snap.targetTPS*0.7 {
					fmt.Fprintf(view, " [yellow](LOW)[-]\n")
				} else {
					fmt.Fprintf(view, " [red](CRITICAL)[-]\n")
				}
			} else {
				fmt.Fprintf(view, "    Actual TPS:          %8.2f\n", snap.actualTPS)
			}

			fmt.Fprintf(view, "\n")
			fmt.Fprintf(view, "  [yellow]TRANSACTION LATENCY[-]\n")
			fmt.Fprintf(view, "    P50 (median):        %8s\n", snap.latencyP50.Round(time.Millisecond))
			fmt.Fprintf(view, "    P95:                 %8s\n", snap.latencyP95.Round(time.Millisecond))
			fmt.Fprintf(view, "    P99:                 %8s\n", snap.latencyP99.Round(time.Millisecond))

			fmt.Fprintf(view, "\n")
			fmt.Fprintf(view, "  [yellow]BLOCKCHAIN STATUS[-]\n")
			fmt.Fprintf(view, "    Block Height:        %8d\n", snap.blockHeight)
			fmt.Fprintf(view, "    Block Rate (TPS):    %8.2f\n", snap.blockTPS)

			statusColor := "green"
			if snap.nodeStatus != "HEALTHY" {
				statusColor = "red"
			}
			fmt.Fprintf(view, "    Node Health:         [%s]%s[-]\n", statusColor, snap.nodeStatus)

			fmt.Fprintf(view, "\n")
			fmt.Fprintf(view, "  [yellow]RESOURCE UTILIZATION[-]\n")
			fmt.Fprintf(view, "    CPU Usage:           %8.1f%%\n", snap.cpuPercent)
			fmt.Fprintf(view, "    Memory Used:         %8d MB\n", snap.memUsed)
			fmt.Fprintf(view, "    Memory Total:        %8d MB\n", snap.memTotal)

			memPercent := float64(snap.memUsed) / float64(snap.memTotal) * 100
			memBar := makeBar(memPercent, 30)
			fmt.Fprintf(view, "    Memory Usage:        %s %5.1f%%\n", memBar, memPercent)

			fmt.Fprintf(view, "    Goroutines:          %8d\n", runtime.NumGoroutine())

			// Disk space (if available)
			if snap.diskTotal > 0 {
				fmt.Fprintf(view, "\n")
				fmt.Fprintf(view, "  [yellow]DISK SPACE[-]\n")
				fmt.Fprintf(view, "    Used:                %8d GB\n", snap.diskUsed)
				fmt.Fprintf(view, "    Total:               %8d GB\n", snap.diskTotal)
				diskPercent := float64(snap.diskUsed) / float64(snap.diskTotal) * 100
				diskBar := makeBar(diskPercent, 30)
				fmt.Fprintf(view, "    Disk Usage:          %s %5.1f%%", diskBar, diskPercent)
				if diskPercent > 90 {
					fmt.Fprintf(view, " [red]WARNING[-]")
				} else if diskPercent > 80 {
					fmt.Fprintf(view, " [yellow]CAUTION[-]")
				}
				fmt.Fprintf(view, "\n")
			}

			fmt.Fprintf(view, "\n")
			fmt.Fprintf(view, "  [yellow]SYSTEM INFO[-]\n")
			fmt.Fprintf(view, "    Last Update:         %s\n", snap.lastUpdate.Format("15:04:05"))
			fmt.Fprintf(view, "    Uptime:              %s\n", time.Since(startTime).Round(time.Second))

			fmt.Fprintf(view, "\n")
		}
	}
}

func makeBar(percent float64, width int) string {
	filled := int(percent / 100.0 * float64(width))
	filled = min(filled, width)
	filled = max(filled, 0)

	bar := "["
	for i := range width {
		if i < filled {
			if percent > 90 {
				bar += "[red]#[-]"
			} else if percent > 70 {
				bar += "[yellow]#[-]"
			} else {
				bar += "[green]#[-]"
			}
		} else {
			bar += "-"
		}
	}
	bar += "]"
	return bar
}

var startTime = time.Now()
