// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logging

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

// DevNetMetricsCollector collects and logs metrics for devnet analysis
type DevNetMetricsCollector struct {
	logger *DevNetLogger

	// Message processing metrics
	messagesProcessed int64
	messagesSucceeded int64
	messagesFailed    int64
	totalLatency      int64
	maxLatency        int64

	// Gap recovery metrics
	gapsDetected  int64
	gapsRecovered int64
	
	// Throughput tracking
	lastReportTime    time.Time
	lastMessageCount  int64
}

// NewDevNetMetricsCollector creates a new metrics collector
func NewDevNetMetricsCollector(logger *DevNetLogger) *DevNetMetricsCollector {
	return &DevNetMetricsCollector{
		logger:         logger,
		lastReportTime: time.Now(),
	}
}

// RecordMessageProcessed records a processed message with latency
func (m *DevNetMetricsCollector) RecordMessageProcessed(success bool, latencyMs int64) {
	atomic.AddInt64(&m.messagesProcessed, 1)
	
	if success {
		atomic.AddInt64(&m.messagesSucceeded, 1)
	} else {
		atomic.AddInt64(&m.messagesFailed, 1)
	}
	
	// Update latency metrics
	atomic.AddInt64(&m.totalLatency, latencyMs)
	
	// Update max latency
	for {
		current := atomic.LoadInt64(&m.maxLatency)
		if latencyMs <= current {
			break
		}
		if atomic.CompareAndSwapInt64(&m.maxLatency, current, latencyMs) {
			break
		}
	}
}

// RecordGapDetected records a gap detection event
func (m *DevNetMetricsCollector) RecordGapDetected() {
	atomic.AddInt64(&m.gapsDetected, 1)
}

// RecordGapRecovered records a gap recovery event
func (m *DevNetMetricsCollector) RecordGapRecovered() {
	atomic.AddInt64(&m.gapsRecovered, 1)
}

// ReportMetrics reports current metrics to logs
func (m *DevNetMetricsCollector) ReportMetrics(ctx context.Context) {
	now := time.Now()
	timeSinceLastReport := now.Sub(m.lastReportTime)
	
	processed := atomic.LoadInt64(&m.messagesProcessed)
	succeeded := atomic.LoadInt64(&m.messagesSucceeded)
	failed := atomic.LoadInt64(&m.messagesFailed)
	totalLatency := atomic.LoadInt64(&m.totalLatency)
	maxLatency := atomic.LoadInt64(&m.maxLatency)
	gapsDetected := atomic.LoadInt64(&m.gapsDetected)
	gapsRecovered := atomic.LoadInt64(&m.gapsRecovered)
	
	// Calculate throughput
	messagesSinceLastReport := processed - m.lastMessageCount
	throughputPerSecond := float64(messagesSinceLastReport) / timeSinceLastReport.Seconds()
	
	// Calculate average latency
	avgLatency := float64(0)
	if processed > 0 {
		avgLatency = float64(totalLatency) / float64(processed)
	}
	
	// Calculate success rate
	successRate := float64(0)
	if processed > 0 {
		successRate = float64(succeeded) / float64(processed) * 100
	}
	
	// Log comprehensive metrics
	m.logger.DevNetMetrics(ctx, map[string]interface{}{
		"messages_processed":    processed,
		"messages_succeeded":    succeeded,
		"messages_failed":       failed,
		"success_rate_percent":  successRate,
		"throughput_per_second": throughputPerSecond,
		"avg_latency_ms":        avgLatency,
		"max_latency_ms":        maxLatency,
		"gaps_detected":         gapsDetected,
		"gaps_recovered":        gapsRecovered,
		"gap_recovery_rate":     func() float64 {
			if gapsDetected > 0 {
				return float64(gapsRecovered) / float64(gapsDetected) * 100
			}
			return 0
		}(),
		"reporting_interval_seconds": timeSinceLastReport.Seconds(),
	})
	
	// Update last report tracking
	m.lastReportTime = now
	m.lastMessageCount = processed
}

// GetSnapshot returns a snapshot of current metrics
func (m *DevNetMetricsCollector) GetSnapshot() map[string]interface{} {
	processed := atomic.LoadInt64(&m.messagesProcessed)
	succeeded := atomic.LoadInt64(&m.messagesSucceeded)
	failed := atomic.LoadInt64(&m.messagesFailed)
	totalLatency := atomic.LoadInt64(&m.totalLatency)
	maxLatency := atomic.LoadInt64(&m.maxLatency)
	gapsDetected := atomic.LoadInt64(&m.gapsDetected)
	gapsRecovered := atomic.LoadInt64(&m.gapsRecovered)
	
	avgLatency := float64(0)
	if processed > 0 {
		avgLatency = float64(totalLatency) / float64(processed)
	}
	
	successRate := float64(0)
	if processed > 0 {
		successRate = float64(succeeded) / float64(processed) * 100
	}
	
	return map[string]interface{}{
		"messages_processed":   processed,
		"messages_succeeded":   succeeded,
		"messages_failed":      failed,
		"success_rate_percent": successRate,
		"avg_latency_ms":       avgLatency,
		"max_latency_ms":       maxLatency,
		"gaps_detected":        gapsDetected,
		"gaps_recovered":       gapsRecovered,
	}
}

// StartPeriodicReporting starts automatic periodic metric reporting
func (m *DevNetMetricsCollector) StartPeriodicReporting(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.ReportMetrics(ctx)
			case <-ctx.Done():
				// Final metrics report
				m.logger.Logger.InfoContext(ctx, "DevNet metrics final report")
				m.ReportMetrics(ctx)
				return
			}
		}
	}()
}

// LogPerformanceEvent logs a significant performance event
func (m *DevNetMetricsCollector) LogPerformanceEvent(ctx context.Context, eventType string, details map[string]interface{}) {
	m.logger.Logger.InfoContext(ctx, "DevNet performance event",
		slog.String("component", "devnet.metrics"),
		slog.String("event_type", eventType),
		slog.Any("details", details),
		slog.Int64("timestamp", time.Now().Unix()),
	)
}