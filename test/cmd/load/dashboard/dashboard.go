// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dashboard

import (
	"context"
	"io"
	"os"
	"time"
)

// Dashboard coordinates metric collection and display
type Dashboard struct {
	loadMetrics   *LoadMetrics
	systemMetrics *SystemMetrics
	display       *Display
	updateTicker  *time.Ticker
	done          chan struct{}
}

// New creates a new dashboard instance
func New(targetTxs int64) *Dashboard {
	return &Dashboard{
		loadMetrics:   NewLoadMetrics(targetTxs),
		systemMetrics: NewSystemMetrics(),
		display:       NewDisplay(os.Stdout),
		done:          make(chan struct{}),
	}
}

// NewWithWriter creates a dashboard with custom output writer
func NewWithWriter(targetTxs int64, w io.Writer) *Dashboard {
	return &Dashboard{
		loadMetrics:   NewLoadMetrics(targetTxs),
		systemMetrics: NewSystemMetrics(),
		display:       NewDisplay(w),
		done:          make(chan struct{}),
	}
}

// Start begins the dashboard update loop
func (d *Dashboard) Start(ctx context.Context, updateInterval time.Duration) {
	d.updateTicker = time.NewTicker(updateInterval)
	defer d.updateTicker.Stop()

	// Initial clear and render
	d.display.Clear()
	d.render()

	for {
		select {
		case <-ctx.Done():
			d.Stop()
			return
		case <-d.done:
			return
		case <-d.updateTicker.C:
			d.update()
		}
	}
}

// Stop stops the dashboard and restores terminal
func (d *Dashboard) Stop() {
	select {
	case <-d.done:
		return
	default:
		close(d.done)
	}

	if d.updateTicker != nil {
		d.updateTicker.Stop()
	}
	d.display.Show()
}

// LoadMetrics returns the load metrics collector
func (d *Dashboard) LoadMetrics() *LoadMetrics {
	return d.loadMetrics
}

// update collects metrics and refreshes display
func (d *Dashboard) update() {
	// Update system metrics
	if err := d.systemMetrics.Update(); err != nil {
		// Log error but continue
		return
	}

	// Render updated display
	d.render()
}

// render draws the dashboard
func (d *Dashboard) render() {
	d.display.Render(d.loadMetrics, d.systemMetrics)
}

// IsDone returns true if the dashboard has stopped
func (d *Dashboard) IsDone() bool {
	select {
	case <-d.done:
		return true
	default:
		return false
	}
}
