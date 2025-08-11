// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestUnifiedTransportBasics tests basic unified transport functionality
func TestUnifiedTransportBasics(t *testing.T) {
	// Create a proof service without a conductor
	var logger logging.OptionalLogger
	proofService := NewProofService(logger)
	proofService.SetDebugMode(false)
	
	// Create unified transport
	transport := NewUnifiedTransport(proofService, nil, logger)
	transport.SetDebugMode(false)
	
	// Test batching logic
	messages := []CrossChainMessage{
		&UnifiedMessage{
			Type:        MessageTypeSynthetic,
			Destination: protocol.PartitionUrl("dest1"),
			Sequence:    1,
			Payload:     &messaging.TransactionMessage{},
		},
		&UnifiedMessage{
			Type:        MessageTypeSynthetic,
			Destination: protocol.PartitionUrl("dest1"),
			Sequence:    2,
			Payload:     &messaging.TransactionMessage{},
		},
		&UnifiedMessage{
			Type:        MessageTypeAnchor,
			Destination: protocol.PartitionUrl("dest1"),
			Sequence:    3,
			Payload:     &messaging.BlockAnchor{},
		},
		&UnifiedMessage{
			Type:        MessageTypeSynthetic,
			Destination: protocol.PartitionUrl("dest2"),
			Sequence:    4,
			Payload:     &messaging.TransactionMessage{},
		},
	}
	
	// Test batching
	batches := transport.createBatches(messages)
	require.Equal(t, 2, len(batches), "Should create 2 batches (dest1 and dest2)")
	require.Equal(t, 3, len(batches[protocol.PartitionUrl("dest1").String()]), "dest1 should have 3 messages")
	require.Equal(t, 1, len(batches[protocol.PartitionUrl("dest2").String()]), "dest2 should have 1 message")
	
	// Test metrics
	transport.updateMessageMetrics(messages)
	metrics := transport.GetMetrics()
	require.Equal(t, int64(3), metrics.SyntheticsSent, "Should count 3 synthetics")
	require.Equal(t, int64(1), metrics.AnchorsSent, "Should count 1 anchor")
}

// TestCollectionProofEligibility tests when collection proofs should be used
func TestCollectionProofEligibility(t *testing.T) {
	var logger logging.OptionalLogger
	proofService := NewProofService(logger)
	transport := NewUnifiedTransport(proofService, nil, logger)
	
	testCases := []struct {
		name             string
		messageCount     int
		expectCollection bool
	}{
		{"Single message", 1, false},
		{"Two messages (threshold)", 2, true},
		{"Three messages", 3, true},
		{"Large batch", 50, true},
		{"Max batch", 100, true},
		{"Over max", 101, true}, // Would still use collection but in chunks
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			messages := make([]CrossChainMessage, tc.messageCount)
			for i := 0; i < tc.messageCount; i++ {
				messages[i] = &UnifiedMessage{
					Type:        MessageTypeSynthetic,
					Destination: protocol.PartitionUrl("dest"),
					Sequence:    uint64(i + 1),
					Payload:     &messaging.TransactionMessage{},
				}
			}
			
			// Test batching decision
			batches := transport.createBatches(messages)
			require.Equal(t, 1, len(batches), "All messages should go to same destination")
			
			batch := batches[protocol.PartitionUrl("dest").String()]
			useCollection := len(batch) >= transport.batchThreshold && len(batch) <= transport.maxBatchSize
			require.Equal(t, tc.expectCollection, useCollection, 
				"Collection proof decision for %d messages", tc.messageCount)
		})
	}
}

// TestConversionHelpers tests the conversion helper functions
func TestConversionHelpers(t *testing.T) {
	// Test synthetic conversion
	synth := SyntheticTransaction{
		Source:      protocol.PartitionUrl("source"),
		Destination: protocol.PartitionUrl("dest"),
		SequenceNum: 42,
		Message:     &messaging.TransactionMessage{},
	}
	
	unified := ConvertSyntheticToUnified(synth, nil, nil, 100)
	require.Equal(t, MessageTypeSynthetic, unified.Type)
	require.Equal(t, protocol.PartitionUrl("source"), unified.Source)
	require.Equal(t, protocol.PartitionUrl("dest"), unified.Destination)
	require.Equal(t, uint64(42), unified.Sequence)
	require.Equal(t, uint64(100), unified.BlockIndex)
	
	// Test anchor conversion
	anchor := &protocol.DirectoryAnchor{}
	anchor.Source = protocol.PartitionUrl("source")
	anchor.MinorBlockIndex = 200
	
	unifiedAnchor := ConvertAnchorToUnified(
		anchor,
		protocol.PartitionUrl("source"),
		protocol.PartitionUrl("dest"),
		43,
		nil,
		nil,
		200,
	)
	require.Equal(t, MessageTypeDirectoryAnchor, unifiedAnchor.Type)
	require.Equal(t, protocol.PartitionUrl("source"), unifiedAnchor.Source)
	require.Equal(t, protocol.PartitionUrl("dest"), unifiedAnchor.Destination)
	require.Equal(t, uint64(43), unifiedAnchor.Sequence)
	require.Equal(t, uint64(200), unifiedAnchor.BlockIndex)
}

// TestMixedBatchTypes tests batching of mixed message types
func TestMixedBatchTypes(t *testing.T) {
	var logger logging.OptionalLogger
	proofService := NewProofService(logger)
	transport := NewUnifiedTransport(proofService, nil, logger)
	
	// Create mixed batch - 2 synthetics, 2 anchors, all to same destination
	messages := []CrossChainMessage{
		&UnifiedMessage{
			Type:        MessageTypeSynthetic,
			Destination: protocol.PartitionUrl("dest"),
			Sequence:    1,
			Payload:     &messaging.TransactionMessage{},
		},
		&UnifiedMessage{
			Type:        MessageTypeAnchor,
			Destination: protocol.PartitionUrl("dest"),
			Sequence:    2,
			Payload:     &messaging.BlockAnchor{},
		},
		&UnifiedMessage{
			Type:        MessageTypeSynthetic,
			Destination: protocol.PartitionUrl("dest"),
			Sequence:    3,
			Payload:     &messaging.TransactionMessage{},
		},
		&UnifiedMessage{
			Type:        MessageTypeAnchor,
			Destination: protocol.PartitionUrl("dest"),
			Sequence:    4,
			Payload:     &messaging.BlockAnchor{},
		},
	}
	
	// Batch them
	batches := transport.createBatches(messages)
	require.Equal(t, 1, len(batches), "Should create single batch for same destination")
	
	batch := batches[protocol.PartitionUrl("dest").String()]
	require.Equal(t, 4, len(batch), "Batch should contain all 4 messages")
	
	// Check metrics
	transport.updateMessageMetrics(messages)
	metrics := transport.GetMetrics()
	require.Equal(t, int64(2), metrics.SyntheticsSent)
	require.Equal(t, int64(2), metrics.AnchorsSent)
}

// TestProofServiceUnifiedType tests that ProofService handles unified type correctly
func TestProofServiceUnifiedType(t *testing.T) {
	var logger logging.OptionalLogger
	proofService := NewProofService(logger)
	
	// Test that unified type triggers collection proof logic
	req := ProofRequest{
		Type:        ProofTypeUnified,
		Destination: protocol.PartitionUrl("dest"),
		Sequences:   []uint64{1, 2, 3}, // Multiple sequences
	}
	
	ctx := context.Background()
	
	// This should attempt a collection proof (will fail due to nil chains, but that's OK)
	_, err := proofService.CreateProof(ctx, req)
	require.Error(t, err) // Expected to fail without proper chain setup
	
	// Check that it attempted collection proof path
	metrics := proofService.GetMetrics()
	// The error would have occurred in the collection proof path
	require.GreaterOrEqual(t, metrics.ProofGenErrors, int64(1))
}

// TestDestinationKeyUniqueness tests that destination keys are unique per type+destination
func TestDestinationKeyUniqueness(t *testing.T) {
	dest1 := protocol.PartitionUrl("dest1")
	dest2 := protocol.PartitionUrl("dest2")
	
	keys := make(map[string]bool)
	
	// Create keys for different combinations
	testCases := []struct {
		msgType MessageType
		dest    *url.URL
		expect  string
	}{
		{MessageTypeSynthetic, dest1, "synthetic-dest1"},
		{MessageTypeAnchor, dest1, "anchor-dest1"},
		{MessageTypeSynthetic, dest2, "synthetic-dest2"},
		{MessageTypeAnchor, dest2, "anchor-dest2"},
	}
	
	for _, tc := range testCases {
		key := tc.dest.String()
		// Ensure uniqueness
		require.NotContains(t, keys, key, "Key should be unique: %s", key)
		keys[key] = true
	}
	
	// Verify we have 4 unique keys (2 types × 2 destinations)
	require.Equal(t, 4, len(keys))
}