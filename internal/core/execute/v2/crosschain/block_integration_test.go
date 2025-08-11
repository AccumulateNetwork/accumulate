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
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestBlockIntegrationBatching tests that the block integration layer properly batches messages
func TestBlockIntegrationBatching(t *testing.T) {
	// Setup
	var logger logging.OptionalLogger
	proofService := NewProofService(logger)
	conductor := &CrossChainConductor{
		proofService:     proofService,
		unifiedTransport: NewUnifiedTransport(proofService, nil, logger),
		logger:           logger,
	}
	blockIntegration := NewBlockIntegration(conductor)
	
	// Create a batched sender
	sender := blockIntegration.NewBatchedSender()
	
	// Add mixed messages to the batch
	dest := protocol.PartitionUrl("destination")
	source := protocol.PartitionUrl("source")
	
	// Add 2 synthetic transactions
	synth1 := &messaging.TransactionMessage{
		Transaction: &protocol.Transaction{
			Header: protocol.TransactionHeader{Principal: protocol.AcmeUrl()},
			Body:   &protocol.SendTokens{},
		},
	}
	sender.AddSynthetic(synth1, source, dest, 1, nil, nil, 100)
	
	synth2 := &messaging.TransactionMessage{
		Transaction: &protocol.Transaction{
			Header: protocol.TransactionHeader{Principal: protocol.AcmeUrl()},
			Body:   &protocol.SendTokens{},
		},
	}
	sender.AddSynthetic(synth2, source, dest, 2, nil, nil, 100)
	
	// Add 2 anchors
	anchor1 := &protocol.DirectoryAnchor{}
	anchor1.Source = source
	anchor1.MinorBlockIndex = 100
	sender.AddAnchor(anchor1, source, dest, 3, nil, nil, 100)
	
	anchor2 := &protocol.BlockValidatorAnchor{}
	anchor2.Source = source
	anchor2.MinorBlockIndex = 101
	sender.AddAnchor(anchor2, source, dest, 4, nil, nil, 101)
	
	// Verify batch contains 4 messages
	require.Equal(t, 4, sender.MessageCount())
	
	// Send the batch (will fail due to nil chains, but that's OK for this test)
	ctx := context.Background()
	err := sender.Send(ctx)
	// We expect an error because we don't have real chains set up
	require.Error(t, err)
	
	// But we can check that the transport received the messages
	metrics := conductor.unifiedTransport.GetMetrics()
	require.Equal(t, int64(2), metrics.SyntheticsSent)
	require.Equal(t, int64(2), metrics.AnchorsSent)
	
	// After send attempt, batch should be cleared
	require.Equal(t, 0, sender.MessageCount())
}

// TestBlockIntegrationQueueMethods tests the queue methods that create messages without sending
func TestBlockIntegrationQueueMethods(t *testing.T) {
	// Setup
	var logger logging.OptionalLogger
	conductor := &CrossChainConductor{
		proofService:     NewProofService(logger),
		unifiedTransport: NewUnifiedTransport(nil, nil, logger),
		logger:           logger,
	}
	blockIntegration := NewBlockIntegration(conductor)
	
	// Queue an anchor
	anchor := &protocol.DirectoryAnchor{}
	anchor.Source = protocol.PartitionUrl("source")
	anchor.MinorBlockIndex = 100
	
	queuedAnchor := blockIntegration.QueueAnchor(
		anchor,
		protocol.PartitionUrl("source"),
		protocol.PartitionUrl("dest"),
		1,
		nil,
		nil,
		100,
	)
	
	require.NotNil(t, queuedAnchor)
	require.Equal(t, MessageTypeDirectoryAnchor, queuedAnchor.GetType())
	require.Equal(t, protocol.PartitionUrl("dest"), queuedAnchor.GetDestination())
	require.Equal(t, uint64(1), queuedAnchor.GetSequence())
	
	// Queue a synthetic
	synth := &messaging.TransactionMessage{
		Transaction: &protocol.Transaction{
			Header: protocol.TransactionHeader{Principal: protocol.AcmeUrl()},
			Body:   &protocol.SendTokens{},
		},
	}
	
	queuedSynth := blockIntegration.QueueSynthetic(
		synth,
		protocol.PartitionUrl("source"),
		protocol.PartitionUrl("dest"),
		2,
		nil,
		nil,
		100,
	)
	
	require.NotNil(t, queuedSynth)
	require.Equal(t, MessageTypeSynthetic, queuedSynth.GetType())
	require.Equal(t, protocol.PartitionUrl("dest"), queuedSynth.GetDestination())
	require.Equal(t, uint64(2), queuedSynth.GetSequence())
}

// TestBlockIntegrationDirectSend tests sending individual messages directly
func TestBlockIntegrationDirectSend(t *testing.T) {
	// Setup
	var logger logging.OptionalLogger
	conductor := &CrossChainConductor{
		proofService:     NewProofService(logger),
		unifiedTransport: NewUnifiedTransport(nil, nil, logger),
		logger:           logger,
	}
	blockIntegration := NewBlockIntegration(conductor)
	
	ctx := context.Background()
	
	// Send an anchor directly
	anchor := &protocol.BlockValidatorAnchor{}
	anchor.Source = protocol.PartitionUrl("source")
	anchor.MinorBlockIndex = 100
	
	err := blockIntegration.SendAnchor(
		ctx,
		anchor,
		protocol.PartitionUrl("source"),
		protocol.PartitionUrl("dest"),
		1,
		nil,
		nil,
		100,
	)
	// Error expected due to nil chains
	require.Error(t, err)
	
	// Check metrics to verify it was processed
	metrics := conductor.unifiedTransport.GetMetrics()
	require.Equal(t, int64(1), metrics.AnchorsSent)
	
	// Send a synthetic directly
	synth := &messaging.TransactionMessage{
		Transaction: &protocol.Transaction{
			Header: protocol.TransactionHeader{Principal: protocol.AcmeUrl()},
			Body:   &protocol.SendTokens{},
		},
	}
	
	err = blockIntegration.SendSynthetic(
		ctx,
		synth,
		protocol.PartitionUrl("source"),
		protocol.PartitionUrl("dest"),
		2,
		nil,
		nil,
		100,
	)
	// Error expected due to nil chains
	require.Error(t, err)
	
	// Check metrics
	metrics = conductor.unifiedTransport.GetMetrics()
	require.Equal(t, int64(1), metrics.SyntheticsSent)
}

// TestBatchedSenderClear tests that the batched sender can be cleared without sending
func TestBatchedSenderClear(t *testing.T) {
	// Setup
	var logger logging.OptionalLogger
	conductor := &CrossChainConductor{
		proofService:     NewProofService(logger),
		unifiedTransport: NewUnifiedTransport(nil, nil, logger),
		logger:           logger,
	}
	blockIntegration := NewBlockIntegration(conductor)
	
	sender := blockIntegration.NewBatchedSender()
	
	// Add some messages
	synth := &messaging.TransactionMessage{
		Transaction: &protocol.Transaction{
			Header: protocol.TransactionHeader{Principal: protocol.AcmeUrl()},
			Body:   &protocol.SendTokens{},
		},
	}
	sender.AddSynthetic(synth, protocol.PartitionUrl("source"), protocol.PartitionUrl("dest"), 1, nil, nil, 100)
	
	anchor := &protocol.DirectoryAnchor{}
	anchor.Source = protocol.PartitionUrl("source")
	sender.AddAnchor(anchor, protocol.PartitionUrl("source"), protocol.PartitionUrl("dest"), 2, nil, nil, 100)
	
	require.Equal(t, 2, sender.MessageCount())
	
	// Clear without sending
	sender.Clear()
	require.Equal(t, 0, sender.MessageCount())
	
	// Verify nothing was sent
	metrics := conductor.unifiedTransport.GetMetrics()
	require.Equal(t, int64(0), metrics.SyntheticsSent)
	require.Equal(t, int64(0), metrics.AnchorsSent)
}