// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// DestinationSendState tracks the state of sending messages to a specific destination.
// This implements the simple index-based gap recovery mechanism where:
// - SentTxIndex tracks the last successfully sent sequence
// - CurrentTxIndex tracks the latest available sequence
// - Gap recovery is just resetting SentTxIndex when a gap request arrives
type DestinationSendState struct {
	// Destination is the target partition URL
	Destination *url.URL

	// SentTxIndex is the sequence number of the last successfully sent message.
	// When sending a batch, we send from SentTxIndex+1 to CurrentTxIndex.
	// Only updated on successful send.
	SentTxIndex uint64

	// CurrentTxIndex is the latest sequence number available to send.
	// This increases as new messages are queued for this destination.
	CurrentTxIndex uint64

	// LastSendTime tracks when we last successfully sent to this destination
	LastSendTime time.Time

	// LastSendAttempt tracks when we last attempted to send (successful or not)
	LastSendAttempt time.Time

	// SendInProgress indicates if a send is currently in progress
	SendInProgress bool

	// FailureCount tracks consecutive send failures
	FailureCount int

	// MessageQueue holds messages waiting to be sent
	// Key is sequence number, value is the message
	MessageQueue map[uint64]messaging.Message

	// Metrics
	TotalSent       uint64
	TotalFailed     uint64
	TotalGapResets  uint64
	LargestGapReset uint64

	// mu protects all fields
	mu sync.RWMutex
}

// NewDestinationSendState creates a new destination state tracker
func NewDestinationSendState(dest *url.URL) *DestinationSendState {
	return &DestinationSendState{
		Destination:  dest,
		MessageQueue: make(map[uint64]messaging.Message),
		LastSendTime: time.Now(), // Prevent immediate timeout
	}
}

// GetSendRange returns the range of sequences to send [start, end]
// Returns (0, 0) if nothing to send
func (d *DestinationSendState) GetSendRange() (start, end uint64) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.SentTxIndex >= d.CurrentTxIndex {
		return 0, 0 // Nothing to send
	}

	return d.SentTxIndex + 1, d.CurrentTxIndex
}

// HasPendingMessages returns true if there are messages waiting to be sent
func (d *DestinationSendState) HasPendingMessages() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.CurrentTxIndex > d.SentTxIndex
}

// GetGapSize returns the number of messages waiting to be sent
func (d *DestinationSendState) GetGapSize() uint64 {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.CurrentTxIndex <= d.SentTxIndex {
		return 0
	}
	return d.CurrentTxIndex - d.SentTxIndex
}

// QueueMessage adds a message to the send queue
func (d *DestinationSendState) QueueMessage(seq uint64, msg messaging.Message) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.MessageQueue[seq] = msg
	if seq > d.CurrentTxIndex {
		d.CurrentTxIndex = seq
	}
}

// CollectMessages returns all messages in the range [start, end]
func (d *DestinationSendState) CollectMessages(start, end uint64) []messaging.Message {
	d.mu.RLock()
	defer d.mu.RUnlock()

	messages := make([]messaging.Message, 0, end-start+1)
	for seq := start; seq <= end; seq++ {
		if msg, ok := d.MessageQueue[seq]; ok {
			messages = append(messages, msg)
		}
	}
	return messages
}

// MarkSendSuccess updates state after successful send
func (d *DestinationSendState) MarkSendSuccess(upToSeq uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()

	oldSentIndex := d.SentTxIndex
	d.SentTxIndex = upToSeq
	d.LastSendTime = time.Now()
	d.LastSendAttempt = time.Now()
	d.SendInProgress = false
	d.FailureCount = 0
	d.TotalSent++

	// Clean up sent messages from queue (from old index+1 to new index)
	for seq := oldSentIndex + 1; seq <= upToSeq; seq++ {
		delete(d.MessageQueue, seq)
	}
}

// MarkSendFailure updates state after failed send
func (d *DestinationSendState) MarkSendFailure() {
	d.mu.Lock()
	defer d.mu.Unlock()

	// SentTxIndex is NOT updated on failure - this is key to the design!
	// Next send attempt will include all messages from SentTxIndex+1
	d.LastSendAttempt = time.Now()
	d.SendInProgress = false
	d.FailureCount++
	d.TotalFailed++
}

// ResetForGapRecovery resets the send index for gap recovery.
// This is called when we receive a gap request from the destination.
// The destination tells us the last sequence it successfully received,
// and we reset our send index to that value so the next send will
// include all messages from that point forward.
func (d *DestinationSendState) ResetForGapRecovery(lastKnownSeq uint64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Only reset if going backwards
	if lastKnownSeq >= d.SentTxIndex {
		return false // Nothing to reset
	}

	// Track the size of the gap we're recovering
	gapSize := d.SentTxIndex - lastKnownSeq
	if gapSize > d.LargestGapReset {
		d.LargestGapReset = gapSize
	}

	// Reset the send index - this is the key to gap recovery!
	// Next send will include everything from lastKnownSeq+1
	d.SentTxIndex = lastKnownSeq
	d.TotalGapResets++

	return true
}

// StartSend marks that a send is in progress
func (d *DestinationSendState) StartSend() bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.SendInProgress {
		return false // Already sending
	}

	d.SendInProgress = true
	d.LastSendAttempt = time.Now()
	return true
}

// GetMetrics returns current metrics
func (d *DestinationSendState) GetMetrics() map[string]interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return map[string]interface{}{
		"destination":       d.Destination.String(),
		"sent_tx_index":     d.SentTxIndex,
		"current_tx_index":  d.CurrentTxIndex,
		"gap_size":          d.GetGapSize(),
		"total_sent":        d.TotalSent,
		"total_failed":      d.TotalFailed,
		"total_gap_resets":  d.TotalGapResets,
		"largest_gap_reset": d.LargestGapReset,
		"failure_count":     d.FailureCount,
		"queue_size":        len(d.MessageQueue),
		"last_send_time":    d.LastSendTime,
		"last_send_attempt": d.LastSendAttempt,
		"send_in_progress":  d.SendInProgress,
	}
}
