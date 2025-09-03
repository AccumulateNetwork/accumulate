// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logging

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// DevNetLogger provides enhanced logging for devnet operations
type DevNetLogger struct {
	*slog.Logger
	partition string
	nodeID    int
	isDevNet  bool
}

// DevNetLogEntry represents a structured log entry for devnet operations
type DevNetLogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Component string                 `json:"component"`
	Partition string                 `json:"partition"`
	Node      int                    `json:"node"`
	Message   string                 `json:"message"`
	Metrics   map[string]interface{} `json:"metrics,omitempty"`
	DevNet    bool                   `json:"devnet"`
}

// NewDevNetLogger creates a logger with devnet-specific context
func NewDevNetLogger(base *slog.Logger, partition string, nodeID int, isDevNet bool) *DevNetLogger {
	logger := base.With(
		"module", "devnet",
		"partition", partition,
		"node", nodeID,
		"devnet_active", isDevNet,
	)
	
	return &DevNetLogger{
		Logger:    logger,
		partition: partition,
		nodeID:    nodeID,
		isDevNet:  isDevNet,
	}
}

// ConductorMessage logs CrossChain Conductor message processing
func (d *DevNetLogger) ConductorMessage(ctx context.Context, source, destination *url.URL, sequence uint64, messageType string) {
	// Use structured but grep-friendly format: key=value pairs
	srcStr := "nil"
	if source != nil {
		srcStr = source.String()
	}
	dstStr := "nil"  
	if destination != nil {
		dstStr = destination.String()
	}
	
	d.Logger.InfoContext(ctx, fmt.Sprintf("devnet.conductor msg_processing src=%s dst=%s seq=%d type=%s partition=%s node=%d", 
		srcStr, dstStr, sequence, messageType, d.partition, d.nodeID))
}

// GapDetected logs gap detection in message sequences
func (d *DevNetLogger) GapDetected(ctx context.Context, destination *url.URL, expected, lastKnown, gapSize uint64) {
	d.Logger.WarnContext(ctx, fmt.Sprintf("devnet.recovery gap_detected dst=%s expected=%d last_known=%d gap_size=%d action=reset_and_resend partition=%s", 
		destination, expected, lastKnown, gapSize, d.partition))
}

// GapRecovered logs successful gap recovery
func (d *DevNetLogger) GapRecovered(ctx context.Context, destination *url.URL, recoveredCount uint64, duration time.Duration) {
	d.Logger.InfoContext(ctx, "Gap recovery completed",
		"component", "devnet.recovery",
		"destination", destination,
		"recovered_messages", recoveredCount,
		"recovery_duration_ms", duration.Milliseconds(),
		"status", "success",
	)
}

// PartitionActivity logs partition-specific operations
func (d *DevNetLogger) PartitionActivity(ctx context.Context, activity string, details map[string]interface{}) {
	args := []interface{}{
		"component", "devnet.partition",
		"activity", activity,
	}
	
	// Add details as individual fields
	for key, value := range details {
		args = append(args, key, value)
	}
	
	d.Logger.InfoContext(ctx, fmt.Sprintf("Partition %s: %s", d.partition, activity), args...)
}

// DevNetMetrics logs performance metrics for devnet analysis
func (d *DevNetLogger) DevNetMetrics(ctx context.Context, metrics map[string]interface{}) {
	// Add devnet-specific context to metrics
	enrichedMetrics := map[string]interface{}{
		"component": "devnet.metrics",
		"timestamp": time.Now().Unix(),
		"partition": d.partition,
		"node":      d.nodeID,
	}
	
	// Merge provided metrics
	for key, value := range metrics {
		enrichedMetrics[key] = value
	}
	
	args := make([]interface{}, 0, len(enrichedMetrics)*2)
	for key, value := range enrichedMetrics {
		args = append(args, key, value)
	}
	
	d.Logger.InfoContext(ctx, "DevNet performance metrics", args...)
}

// NetworkTopology logs devnet network configuration
func (d *DevNetLogger) NetworkTopology(bvns, validators, followers int, portRange string) {
	d.Logger.Info("DevNet topology initialized",
		"component", "devnet.orchestrator",
		"network_type", "devnet",
		"bvns", bvns,
		"validators_per_bvn", validators,
		"followers_per_bvn", followers,
		"total_nodes", (validators+followers)*(1+bvns),
		"port_range", portRange,
		"initialization", "complete",
	)
}

// NodeStartup logs individual node startup events
func (d *DevNetLogger) NodeStartup(nodeType string, listenAddr string, status string) {
	d.Logger.Info("DevNet node startup",
		"component", "devnet.orchestrator", 
		"node_type", nodeType,
		"listen_addr", listenAddr,
		"status", status,
		"startup_phase", "initialization",
	)
}

// CrossChainTransmission logs cross-chain message transmission
func (d *DevNetLogger) CrossChainTransmission(ctx context.Context, dest *url.URL, msgCount int, success bool, errorMsg string) {
	level := slog.LevelInfo
	if !success {
		level = slog.LevelWarn
	}
	
	args := []interface{}{
		"component", "devnet.conductor",
		"destination", dest,
		"message_count", msgCount,
		"success", success,
		"transmission_id", fmt.Sprintf("%s-%d-%d", d.partition, d.nodeID, time.Now().Unix()),
	}
	
	if errorMsg != "" {
		args = append(args, "error", errorMsg)
	}
	
	d.Logger.Log(ctx, level, "Cross-chain transmission", args...)
}

// DevNetEvent logs significant devnet events with structured context
func (d *DevNetLogger) DevNetEvent(ctx context.Context, eventType string, description string, metadata map[string]interface{}) {
	args := []interface{}{
		"component", "devnet.events",
		"event_type", eventType,
		"description", description,
		"event_id", fmt.Sprintf("devnet-%d", time.Now().UnixNano()),
	}
	
	// Add metadata
	for key, value := range metadata {
		args = append(args, key, value)
	}
	
	d.Logger.InfoContext(ctx, fmt.Sprintf("DevNet Event: %s", eventType), args...)
}