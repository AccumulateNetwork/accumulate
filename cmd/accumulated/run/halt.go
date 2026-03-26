// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"log/slog"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
)

// HaltController manages graceful shutdown after major blocks.
// When a halt is requested, the node will shut down after the next
// major block is committed.
type HaltController struct {
	partition     string
	haltRequested atomic.Bool
	shutdown      func()
	logger        *slog.Logger
}

// NewHaltController creates a new halt controller for a partition.
func NewHaltController(partition string, shutdown func(), logger *slog.Logger) *HaltController {
	return &HaltController{
		partition: partition,
		shutdown:  shutdown,
		logger:    logger,
	}
}

// RequestHalt sets the flag to halt after the next major block.
func (hc *HaltController) RequestHalt() {
	hc.haltRequested.Store(true)
	hc.logger.Info("Halt requested after next major block", "partition", hc.partition)
}

// CancelHalt clears the halt request.
func (hc *HaltController) CancelHalt() {
	hc.haltRequested.Store(false)
	hc.logger.Info("Halt request cancelled", "partition", hc.partition)
}

// IsHaltPending returns true if a halt has been requested.
func (hc *HaltController) IsHaltPending() bool {
	return hc.haltRequested.Load()
}

// Partition returns the partition ID this controller is for.
func (hc *HaltController) Partition() string {
	return hc.partition
}

// OnDidCommitBlock handles the DidCommitBlock event.
// If a major block was committed and halt was requested, it triggers shutdown.
func (hc *HaltController) OnDidCommitBlock(e events.DidCommitBlock) error {
	// Only halt on major blocks
	if e.Major == 0 {
		return nil
	}

	// Check if halt was requested
	if !hc.haltRequested.Load() {
		return nil
	}

	hc.logger.Info("Halting after major block as requested",
		"partition", hc.partition,
		"major", e.Major,
		"minor", e.Index,
		"time", e.Time)

	// Trigger graceful shutdown
	// Run in goroutine to not block the event handler
	go hc.shutdown()

	return nil
}

// HaltStatus represents the halt status for API responses.
type HaltStatus struct {
	Pending    bool     `json:"pending"`
	Partitions []string `json:"partitions,omitempty"`
}

// HaltResponse represents a halt API response.
type HaltResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}
