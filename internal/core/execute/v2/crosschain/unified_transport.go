// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"sort"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// MessageType identifies the type of crosschain message
type MessageType int

const (
	MessageTypeSynthetic MessageType = iota
	MessageTypeAnchor
	MessageTypeDirectoryAnchor
	MessageTypeBlockSummary
)

// CrossChainMessage represents any message that crosses partition boundaries
type CrossChainMessage interface {
	// Core message properties
	GetDestination() *url.URL
	GetSequence() uint64
	GetType() MessageType
	GetPayload() messaging.Message
	GetSource() *url.URL
	
	// Chain references for proof construction
	GetSourceChain() *database.Chain
	GetRootChain() *database.Chain
}

// UnifiedMessage is the concrete implementation of CrossChainMessage
type UnifiedMessage struct {
	Type        MessageType
	Source      *url.URL
	Destination *url.URL
	Sequence    uint64
	Payload     messaging.Message
	SourceChain *database.Chain
	RootChain   *database.Chain
	
	// Additional metadata
	BlockIndex  uint64
	Metadata    interface{}
}

func (m *UnifiedMessage) GetDestination() *url.URL     { return m.Destination }
func (m *UnifiedMessage) GetSequence() uint64           { return m.Sequence }
func (m *UnifiedMessage) GetType() MessageType          { return m.Type }
func (m *UnifiedMessage) GetPayload() messaging.Message { return m.Payload }
func (m *UnifiedMessage) GetSource() *url.URL           { return m.Source }
func (m *UnifiedMessage) GetSourceChain() *database.Chain { return m.SourceChain }
func (m *UnifiedMessage) GetRootChain() *database.Chain   { return m.RootChain }

// TransportMetrics tracks unified transport operations
type TransportMetrics struct {
	mu sync.RWMutex
	
	// Message counts by type
	SyntheticsSent int64
	AnchorsSent    int64
	
	// Batching metrics
	BatchesCreated      int64
	MessagesPerBatch    []int
	CollectionProofsUsed int64
	IndividualProofsUsed int64
	
	// Performance metrics
	TotalSendTime     time.Duration
	TotalBatchingTime time.Duration
	
	// Error tracking
	SendErrors    int64
	BatchErrors   int64
}

// UnifiedTransport provides a single transport layer for all crosschain messages
type UnifiedTransport struct {
	proofService *ProofService
	logger       logging.OptionalLogger
	metrics      *TransportMetrics
	debugMode    bool
	
	// Configuration
	batchThreshold int // Minimum messages for collection proof
	maxBatchSize   int // Maximum messages per batch
	
	// Dependencies
	batch     *database.Batch
	conductor *CrossChainConductor
}

// NewUnifiedTransport creates a new unified transport service
func NewUnifiedTransport(
	proofService *ProofService,
	conductor *CrossChainConductor,
	logger logging.OptionalLogger,
) *UnifiedTransport {
	return &UnifiedTransport{
		proofService:   proofService,
		conductor:      conductor,
		logger:         logger,
		metrics:        &TransportMetrics{},
		batchThreshold: 2,  // Same as ProofService default
		maxBatchSize:   100, // Reasonable limit for batch size
	}
}

// Send handles sending any type of crosschain message with optimized batching and proofs
func (ut *UnifiedTransport) Send(ctx context.Context, messages []CrossChainMessage) error {
	if len(messages) == 0 {
		return nil
	}
	
	start := time.Now()
	defer func() {
		ut.metrics.mu.Lock()
		ut.metrics.TotalSendTime += time.Since(start)
		ut.metrics.mu.Unlock()
	}()
	
	if ut.debugMode {
		ut.logger.Debug("UnifiedTransport.Send",
			"message_count", len(messages),
			"first_type", messages[0].GetType())
	}
	
	// Update metrics by type
	ut.updateMessageMetrics(messages)
	
	// Group messages by destination for optimal batching
	batches := ut.createBatches(messages)
	
	// Process each batch with appropriate proof strategy
	for dest, batch := range batches {
		if err := ut.processBatch(ctx, dest, batch); err != nil {
			ut.metrics.mu.Lock()
			ut.metrics.SendErrors++
			ut.metrics.mu.Unlock()
			return errors.UnknownError.WithFormat("failed to process batch for %s: %w", dest, err)
		}
	}
	
	return nil
}

// createBatches groups messages by destination and determines proof strategy
func (ut *UnifiedTransport) createBatches(messages []CrossChainMessage) map[string][]CrossChainMessage {
	batchStart := time.Now()
	defer func() {
		ut.metrics.mu.Lock()
		ut.metrics.TotalBatchingTime += time.Since(batchStart)
		ut.metrics.mu.Unlock()
	}()
	
	batches := make(map[string][]CrossChainMessage)
	
	for _, msg := range messages {
		dest := msg.GetDestination().String()
		batches[dest] = append(batches[dest], msg)
	}
	
	// Track batch sizes
	ut.metrics.mu.Lock()
	ut.metrics.BatchesCreated += int64(len(batches))
	for _, batch := range batches {
		ut.metrics.MessagesPerBatch = append(ut.metrics.MessagesPerBatch, len(batch))
	}
	ut.metrics.mu.Unlock()
	
	if ut.debugMode {
		ut.logger.Debug("Created batches",
			"batch_count", len(batches),
			"destinations", ut.getBatchDestinations(batches))
	}
	
	return batches
}

// processBatch handles a batch of messages going to the same destination
func (ut *UnifiedTransport) processBatch(ctx context.Context, destination string, messages []CrossChainMessage) error {
	if len(messages) == 0 {
		return nil
	}
	
	// Determine proof strategy based on batch size
	useCollection := len(messages) >= ut.batchThreshold && len(messages) <= ut.maxBatchSize
	
	if ut.debugMode {
		ut.logger.Debug("Processing batch",
			"destination", destination,
			"message_count", len(messages),
			"use_collection", useCollection)
	}
	
	// Create proof(s) for the batch
	var proof *ProofResponse
	var err error
	
	if useCollection {
		proof, err = ut.createCollectionProof(ctx, messages)
		if err != nil {
			// Fallback to individual proofs
			ut.logger.Info("Collection proof failed, using individual proofs",
				"destination", destination,
				"error", err)
			return ut.createIndividualProofs(ctx, messages)
		}
		
		ut.metrics.mu.Lock()
		ut.metrics.CollectionProofsUsed++
		ut.metrics.mu.Unlock()
	} else {
		err = ut.createIndividualProofs(ctx, messages)
		if err != nil {
			return err
		}
	}
	
	// Send the messages with their proofs
	// This would integrate with the existing message routing system
	return ut.routeMessages(messages, proof)
}

// createCollectionProof creates a single proof for multiple messages
func (ut *UnifiedTransport) createCollectionProof(ctx context.Context, messages []CrossChainMessage) (*ProofResponse, error) {
	if len(messages) == 0 {
		return nil, errors.BadRequest.With("no messages for collection proof")
	}
	
	// Extract sequences and ensure they're sorted
	sequences := make([]uint64, len(messages))
	for i, msg := range messages {
		sequences[i] = msg.GetSequence()
	}
	sort.Slice(sequences, func(i, j int) bool {
		return sequences[i] < sequences[j]
	})
	
	// Use the first message's chain references (they should all be the same for a batch)
	firstMsg := messages[0]
	
	req := ProofRequest{
		Type:        ProofTypeUnified,
		Destination: firstMsg.GetDestination(),
		Sequences:   sequences,
		SourceChain: firstMsg.GetSourceChain(),
		RootChain:   firstMsg.GetRootChain(),
	}
	
	return ut.proofService.CreateProof(ctx, req)
}

// createIndividualProofs creates separate proofs for each message
func (ut *UnifiedTransport) createIndividualProofs(ctx context.Context, messages []CrossChainMessage) error {
	for _, msg := range messages {
		req := ProofRequest{
			Type:        ProofTypeUnified,
			Destination: msg.GetDestination(),
			Sequences:   []uint64{msg.GetSequence()},
			SourceChain: msg.GetSourceChain(),
			RootChain:   msg.GetRootChain(),
		}
		
		_, err := ut.proofService.CreateProof(ctx, req)
		if err != nil {
			return errors.UnknownError.WithFormat("create proof for sequence %d: %w", msg.GetSequence(), err)
		}
		
		ut.metrics.mu.Lock()
		ut.metrics.IndividualProofsUsed++
		ut.metrics.mu.Unlock()
	}
	
	return nil
}

// routeMessages sends messages to their destination with attached proofs
func (ut *UnifiedTransport) routeMessages(messages []CrossChainMessage, proof *ProofResponse) error {
	// This would integrate with the existing message routing infrastructure
	// For now, we'll just log the routing
	if ut.debugMode {
		ut.logger.Debug("Routing messages",
			"count", len(messages),
			"has_proof", proof != nil,
			"is_collection", proof != nil && proof.IsCollection)
	}
	
	// TODO: Integrate with actual message routing system
	// This would involve:
	// 1. Attaching proofs to messages
	// 2. Sending via the network dispatcher
	// 3. Handling acknowledgments
	
	return nil
}

// updateMessageMetrics updates metrics based on message types
func (ut *UnifiedTransport) updateMessageMetrics(messages []CrossChainMessage) {
	ut.metrics.mu.Lock()
	defer ut.metrics.mu.Unlock()
	
	for _, msg := range messages {
		switch msg.GetType() {
		case MessageTypeSynthetic:
			ut.metrics.SyntheticsSent++
		case MessageTypeAnchor, MessageTypeDirectoryAnchor:
			ut.metrics.AnchorsSent++
		}
	}
}

// getBatchDestinations returns a list of batch destinations for logging
func (ut *UnifiedTransport) getBatchDestinations(batches map[string][]CrossChainMessage) []string {
	destinations := make([]string, 0, len(batches))
	for dest := range batches {
		destinations = append(destinations, dest)
	}
	sort.Strings(destinations)
	return destinations
}

// GetMetrics returns a copy of the current metrics
func (ut *UnifiedTransport) GetMetrics() TransportMetrics {
	ut.metrics.mu.RLock()
	defer ut.metrics.mu.RUnlock()
	
	// Return a copy to avoid race conditions
	return TransportMetrics{
		SyntheticsSent:       ut.metrics.SyntheticsSent,
		AnchorsSent:          ut.metrics.AnchorsSent,
		BatchesCreated:       ut.metrics.BatchesCreated,
		MessagesPerBatch:     append([]int(nil), ut.metrics.MessagesPerBatch...),
		CollectionProofsUsed: ut.metrics.CollectionProofsUsed,
		IndividualProofsUsed: ut.metrics.IndividualProofsUsed,
		TotalSendTime:        ut.metrics.TotalSendTime,
		TotalBatchingTime:    ut.metrics.TotalBatchingTime,
		SendErrors:           ut.metrics.SendErrors,
		BatchErrors:          ut.metrics.BatchErrors,
	}
}

// SetDebugMode enables or disables debug logging
func (ut *UnifiedTransport) SetDebugMode(enabled bool) {
	ut.debugMode = enabled
}

// ConvertSyntheticToUnified converts a synthetic transaction to a unified message
func ConvertSyntheticToUnified(
	synth SyntheticTransaction,
	sourceChain *database.Chain,
	rootChain *database.Chain,
	blockIndex uint64,
) *UnifiedMessage {
	return &UnifiedMessage{
		Type:        MessageTypeSynthetic,
		Source:      synth.Source,
		Destination: synth.Destination,
		Sequence:    synth.SequenceNum,
		Payload:     synth.Message,
		SourceChain: sourceChain,
		RootChain:   rootChain,
		BlockIndex:  blockIndex,
	}
}

// ConvertAnchorToUnified converts an anchor to a unified message
func ConvertAnchorToUnified(
	anchor protocol.AnchorBody,
	source *url.URL,
	destination *url.URL,
	sequence uint64,
	sourceChain *database.Chain,
	rootChain *database.Chain,
	blockIndex uint64,
) *UnifiedMessage {
	// Determine specific anchor type
	msgType := MessageTypeAnchor
	if _, ok := anchor.(*protocol.DirectoryAnchor); ok {
		msgType = MessageTypeDirectoryAnchor
	} else if _, ok := anchor.(*protocol.BlockValidatorAnchor); ok {
		msgType = MessageTypeBlockSummary
	}
	
	// Wrap the anchor in a SequencedMessage first, then BlockAnchor
	// The anchor needs to be wrapped properly as a message
	seqMsg := &messaging.SequencedMessage{
		Source:      source,
		Destination: destination,
		Number:      sequence,
		// The Message field would need the actual transaction message
		// For now, we'll leave this as a TODO since we need proper anchor wrapping
	}
	
	// Create the BlockAnchor with the sequenced message
	blockAnchor := &messaging.BlockAnchor{
		Anchor: seqMsg,
		// Signature will be added later in the flow
	}
	
	return &UnifiedMessage{
		Type:        msgType,
		Source:      source,
		Destination: destination,
		Sequence:    sequence,
		Payload:     blockAnchor,
		SourceChain: sourceChain,
		RootChain:   rootChain,
		BlockIndex:  blockIndex,
	}
}