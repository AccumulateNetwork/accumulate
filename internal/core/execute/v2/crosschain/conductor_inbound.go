// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// ProcessInbound processes inbound cross-partition messages through the conductor
func (cc *CrossChainConductor) ProcessInbound(ctx context.Context, messages []messaging.Message) []messaging.Message {
	// Check if paused - if so, drop all crosschain messages
	if cc.IsPaused() {
		cc.logger.Debug("CCC is paused, filtering crosschain messages")
		nonCrosschain := make([]messaging.Message, 0, len(messages))
		for _, msg := range messages {
			if !cc.isCrossPartitionMessage(msg) {
				nonCrosschain = append(nonCrosschain, msg)
			}
		}
		return nonCrosschain
	}

	// Filter and validate inbound messages
	validMessages := make([]messaging.Message, 0, len(messages))

	for _, msg := range messages {
		// Check for recovery requests first
		if recoveryReq, ok := msg.(*messaging.RecoveryRequest); ok {
			// Handle recovery request in background
			go cc.handleRecoveryRequest(ctx, recoveryReq)
			continue // Don't add to valid messages
		}

		// Skip non-crosschain messages
		if !cc.isCrossPartitionMessage(msg) {
			validMessages = append(validMessages, msg)
			continue
		}

		// Validate crosschain messages
		if valid, reason := cc.validateInboundMessage(msg); valid {
			validMessages = append(validMessages, msg)
		} else {
			cc.logger.Info("Rejected inbound crosschain message",
				"type", msg.Type(),
				"hash", logging.AsHex(msg.Hash()),
				"reason", reason)
			// Track rejected messages
			atomic.AddInt64(&cc.syntheticsErrors, 1)
		}
	}

	if len(validMessages) < len(messages) {
		cc.logger.Info("Filtered inbound messages",
			"received", len(messages),
			"valid", len(validMessages),
			"rejected", len(messages)-len(validMessages))
	}

	return validMessages
}

// validateInboundMessage validates sequence order and message integrity
func (cc *CrossChainConductor) validateInboundMessage(msg messaging.Message) (bool, string) {
	ctx := context.Background()

	switch m := msg.(type) {
	case *messaging.SequencedMessage:
		// Use simplified sequence tracker for validation (no buffering)
		valid, reason, requestRecovery := cc.sequenceTracker.ValidateAndTrackSynthetic(m)

		// Request missing messages immediately if gap detected
		if requestRecovery {
			// Extract gap info from reason (format: "out of order, gap detected [X-Y], dropping message Z")
			if strings.Contains(reason, "gap detected") {
				parts := strings.Split(reason, "[")
				if len(parts) > 1 {
					gapRange := strings.Split(strings.Split(parts[1], "]")[0], "-")
					if len(gapRange) == 2 {
						gapStart, _ := strconv.ParseUint(gapRange[0], 10, 64)
						gapEnd, _ := strconv.ParseUint(gapRange[1], 10, 64)
						go func() {
							if err := cc.sequenceTracker.RequestMissingMessages(
								ctx,
								m.Source.String(),
								MessageTypeSynthetic,
								gapStart, gapEnd); err != nil {
								cc.logger.Error("Failed to request missing synthetic transactions",
									"source", m.Source,
									"gap", fmt.Sprintf("[%d-%d]", gapStart, gapEnd),
									"error", err)
							}
						}()
					}
				}
			}
		}

		return valid, reason

	case *messaging.BlockAnchor:
		// Anchors must have valid signature
		if m.Signature == nil {
			return false, "missing anchor signature"
		}

		// Extract anchor sequence - we need to examine the anchor body
		var sequence uint64
		var source *url.URL

		// Check what type of anchor this is - we need to unwrap the message
		switch anchor := m.Anchor.(type) {
		case *messaging.SequencedMessage:
			// This is a sequenced anchor message
			source = anchor.Source
			sequence = anchor.Number
		default:
			// For other anchor types, try to extract partition info
			// For now, accept if we can't determine sequence
			return true, ""
		}

		// Use simplified sequence tracker for anchor validation
		valid, reason, requestRecovery := cc.sequenceTracker.ValidateAndTrackAnchor(m, source, sequence)

		// Request missing anchors immediately if gap detected
		if requestRecovery {
			// Extract gap info from reason (format: "anchor out of order, gap detected [X-Y], dropping anchor Z")
			if strings.Contains(reason, "gap detected") {
				parts := strings.Split(reason, "[")
				if len(parts) > 1 {
					gapRange := strings.Split(strings.Split(parts[1], "]")[0], "-")
					if len(gapRange) == 2 {
						gapStart, _ := strconv.ParseUint(gapRange[0], 10, 64)
						gapEnd, _ := strconv.ParseUint(gapRange[1], 10, 64)
						go func(src *url.URL) {
							if err := cc.sequenceTracker.RequestMissingMessages(
								ctx,
								src.String(),
								MessageTypeAnchor,
								gapStart, gapEnd); err != nil {
								cc.logger.Error("Failed to request missing anchors",
									"source", src,
									"gap", fmt.Sprintf("[%d-%d]", gapStart, gapEnd),
									"error", err)
							}
						}(source)
					}
				}
			}
		}

		return valid, reason

	default:
		// Unknown crosschain message type
		return false, "unknown crosschain message type"
	}
}

// isCrossPartitionMessage determines if a message is a cross-partition anchor or synthetic transaction
func (cc *CrossChainConductor) isCrossPartitionMessage(msg messaging.Message) bool {
	switch msg.Type() {
	case messaging.MessageTypeSynthetic, messaging.MessageTypeBadSynthetic:
		return true
	case messaging.MessageTypeBlockAnchor:
		return true
	default:
		return false
	}
}

// getMessageType determines the message type for blocking purposes
func (cc *CrossChainConductor) getMessageType(messages []messaging.Message) MessageType {
	// Check the first message to determine type - in practice, envelopes should be homogeneous
	if len(messages) == 0 {
		return ConductorMessageTypeOther
	}

	switch messages[0].Type() {
	case messaging.MessageTypeBlockAnchor:
		return MessageTypeAnchor
	case messaging.MessageTypeSynthetic, messaging.MessageTypeBadSynthetic:
		return MessageTypeSynthetic
	default:
		return ConductorMessageTypeOther
	}
}

// handleRecoveryRequest processes incoming recovery requests from other partitions
func (cc *CrossChainConductor) handleRecoveryRequest(ctx context.Context, req *messaging.RecoveryRequest) {
	cc.logger.Info("Received recovery request",
		"from", req.DestinationPartition,
		"type", req.MessageType,
		"last_seq", req.LastKnownSequence)

	// Use the gap recovery handler which resets the send index.
	// The simple index-based recovery works as follows:
	// 1. Reset our SentTxIndex to req.LastKnownSequence
	// 2. Next batch send will include everything from that point forward
	// 3. No special recovery logic needed - normal send path handles it
	err := cc.HandleGapRequest(ctx, req)
	if err != nil {
		cc.logger.Error("Failed to handle gap request",
			"from", req.DestinationPartition,
			"error", err)
	}
}
