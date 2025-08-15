// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// BlockIntegration provides methods for the block executor to send messages
// through the unified transport without requiring major changes to the executor
type BlockIntegration struct {
	conductor *CrossChainConductor
}

// NewBlockIntegration creates a new block integration helper
func NewBlockIntegration(conductor *CrossChainConductor) *BlockIntegration {
	return &BlockIntegration{
		conductor: conductor,
	}
}

// SendAnchor sends an anchor through the unified transport with collection proof support
func (bi *BlockIntegration) SendAnchor(
	ctx context.Context,
	anchor protocol.AnchorBody,
	source *url.URL,
	destination *url.URL,
	sequence uint64,
	sourceChain *database.Chain,
	rootChain *database.Chain,
	blockIndex uint64,
) error {
	if bi.conductor == nil || bi.conductor.unifiedTransport == nil {
		return errors.InternalError.With("unified transport not initialized")
	}
	
	// Convert the anchor to a unified message
	unifiedMsg := ConvertAnchorToUnified(
		anchor,
		source,
		destination,
		sequence,
		sourceChain,
		rootChain,
		blockIndex,
	)
	
	// Send through unified transport
	return bi.conductor.unifiedTransport.Send(ctx, []CrossChainMessage{unifiedMsg})
}

// SendSynthetic sends a synthetic transaction through the unified transport
func (bi *BlockIntegration) SendSynthetic(
	ctx context.Context,
	synthetic *messaging.TransactionMessage,
	source *url.URL,
	destination *url.URL,
	sequence uint64,
	sourceChain *database.Chain,
	rootChain *database.Chain,
	blockIndex uint64,
) error {
	if bi.conductor == nil || bi.conductor.unifiedTransport == nil {
		return errors.InternalError.With("unified transport not initialized")
	}
	
	// Create unified message
	unifiedMsg := &UnifiedMessage{
		Type:        MessageTypeSynthetic,
		Source:      source,
		Destination: destination,
		Sequence:    sequence,
		Payload:     synthetic,
		SourceChain: sourceChain,
		RootChain:   rootChain,
		BlockIndex:  blockIndex,
	}
	
	// Send through unified transport
	return bi.conductor.unifiedTransport.Send(ctx, []CrossChainMessage{unifiedMsg})
}

// SendBatch sends multiple messages (anchors and/or synthetics) as a batch
// This allows the block executor to queue up messages and send them together
// for optimal collection proof usage
func (bi *BlockIntegration) SendBatch(
	ctx context.Context,
	messages []CrossChainMessage,
) error {
	if bi.conductor == nil || bi.conductor.unifiedTransport == nil {
		return errors.InternalError.With("unified transport not initialized")
	}
	
	if len(messages) == 0 {
		return nil
	}
	
	// Send all messages through unified transport
	// The transport will automatically batch by destination and create collection proofs
	return bi.conductor.unifiedTransport.Send(ctx, messages)
}

// QueueAnchor creates a unified message for an anchor without sending it immediately
// This allows the block executor to batch multiple anchors before sending
func (bi *BlockIntegration) QueueAnchor(
	anchor protocol.AnchorBody,
	source *url.URL,
	destination *url.URL,
	sequence uint64,
	sourceChain *database.Chain,
	rootChain *database.Chain,
	blockIndex uint64,
) CrossChainMessage {
	return ConvertAnchorToUnified(
		anchor,
		source,
		destination,
		sequence,
		sourceChain,
		rootChain,
		blockIndex,
	)
}

// QueueSynthetic creates a unified message for a synthetic without sending it immediately
func (bi *BlockIntegration) QueueSynthetic(
	synthetic *messaging.TransactionMessage,
	source *url.URL,
	destination *url.URL,
	sequence uint64,
	sourceChain *database.Chain,
	rootChain *database.Chain,
	blockIndex uint64,
) CrossChainMessage {
	return &UnifiedMessage{
		Type:        MessageTypeSynthetic,
		Source:      source,
		Destination: destination,
		Sequence:    sequence,
		Payload:     synthetic,
		SourceChain: sourceChain,
		RootChain:   rootChain,
		BlockIndex:  blockIndex,
	}
}

// BatchedSender provides a convenient way to accumulate messages and send them as a batch
type BatchedSender struct {
	integration *BlockIntegration
	messages    []CrossChainMessage
}

// NewBatchedSender creates a new batched sender
func (bi *BlockIntegration) NewBatchedSender() *BatchedSender {
	return &BatchedSender{
		integration: bi,
		messages:    make([]CrossChainMessage, 0),
	}
}

// AddAnchor adds an anchor to the batch
func (bs *BatchedSender) AddAnchor(
	anchor protocol.AnchorBody,
	source *url.URL,
	destination *url.URL,
	sequence uint64,
	sourceChain *database.Chain,
	rootChain *database.Chain,
	blockIndex uint64,
) {
	msg := bs.integration.QueueAnchor(
		anchor,
		source,
		destination,
		sequence,
		sourceChain,
		rootChain,
		blockIndex,
	)
	bs.messages = append(bs.messages, msg)
}

// AddSynthetic adds a synthetic transaction to the batch
func (bs *BatchedSender) AddSynthetic(
	synthetic *messaging.TransactionMessage,
	source *url.URL,
	destination *url.URL,
	sequence uint64,
	sourceChain *database.Chain,
	rootChain *database.Chain,
	blockIndex uint64,
) {
	msg := bs.integration.QueueSynthetic(
		synthetic,
		source,
		destination,
		sequence,
		sourceChain,
		rootChain,
		blockIndex,
	)
	bs.messages = append(bs.messages, msg)
}

// Send sends all batched messages
func (bs *BatchedSender) Send(ctx context.Context) error {
	if len(bs.messages) == 0 {
		return nil
	}
	
	err := bs.integration.SendBatch(ctx, bs.messages)
	if err != nil {
		return err
	}
	
	// Clear the batch after successful send
	bs.messages = bs.messages[:0]
	return nil
}

// MessageCount returns the number of messages in the batch
func (bs *BatchedSender) MessageCount() int {
	return len(bs.messages)
}

// Clear clears the batch without sending
func (bs *BatchedSender) Clear() {
	bs.messages = bs.messages[:0]
}

// PrepareBlockMessages prepares messages for block processing
func (bi *BlockIntegration) PrepareBlockMessages(ctx context.Context, messages []messaging.Message) []messaging.Message {
	// Pass messages through the conductor's inbound processing
	return bi.conductor.ProcessInbound(ctx, messages)
}

// CollectBlockProofs collects proofs for messages in a block
func (bi *BlockIntegration) CollectBlockProofs(ctx context.Context, messages []messaging.Message) []interface{} {
	proofs := make([]interface{}, 0)
	
	// Group messages by destination for collection proof optimization
	destGroups := make(map[string][]messaging.Message)
	for _, msg := range messages {
		if seq, ok := msg.(*messaging.SequencedMessage); ok {
			destKey := seq.Source.String()
			destGroups[destKey] = append(destGroups[destKey], msg)
		}
	}
	
	// For each destination group, create collection proofs if beneficial
	for dest, group := range destGroups {
		if len(group) > 1 {
			// Create collection proof for multiple messages to same destination
			bi.conductor.logger.Debug("Creating collection proof", 
				"destination", dest,
				"message_count", len(group))
		}
		// Actual proof creation would happen here via proof service
	}
	
	return proofs
}

// FinalizeBlock finalizes a block with the given height and time
func (bi *BlockIntegration) FinalizeBlock(ctx context.Context, blockHeight uint64, blockTime uint64) error {
	bi.conductor.logger.Debug("Finalizing block", 
		"height", blockHeight,
		"time", blockTime)
	
	// Cleanup old pending transmissions
	bi.conductor.cleanupOldTransmissions()
	
	return nil
}

// HandleBlockBoundary handles the transition between blocks
func (bi *BlockIntegration) HandleBlockBoundary(ctx context.Context, oldHeight uint64, newHeight uint64) error {
	bi.conductor.logger.Debug("Handling block boundary",
		"old_height", oldHeight,
		"new_height", newHeight)
	
	// Could trigger batch proof creation here
	// Could check for missing sequences here
	
	return nil
}

// GroupMessagesBySource groups messages by their source partition
func (bi *BlockIntegration) GroupMessagesBySource(messages []messaging.Message) map[string][]messaging.Message {
	groups := make(map[string][]messaging.Message)
	
	for _, msg := range messages {
		var source string
		switch m := msg.(type) {
		case *messaging.SequencedMessage:
			source = m.Source.String()
		case *messaging.BlockAnchor:
			if seq, ok := m.Anchor.(*messaging.SequencedMessage); ok {
				source = seq.Source.String()
			}
		default:
			source = "unknown"
		}
		
		groups[source] = append(groups[source], msg)
	}
	
	return groups
}

// SortMessagesBySequence sorts sequenced messages by their sequence number
func (bi *BlockIntegration) SortMessagesBySequence(messages []messaging.Message) []messaging.Message {
	// Create a copy to avoid modifying the original
	sorted := make([]messaging.Message, len(messages))
	copy(sorted, messages)
	
	sort.Slice(sorted, func(i, j int) bool {
		seqI, okI := sorted[i].(*messaging.SequencedMessage)
		seqJ, okJ := sorted[j].(*messaging.SequencedMessage)
		
		if !okI || !okJ {
			return false
		}
		
		return seqI.Number < seqJ.Number
	})
	
	return sorted
}

// CollectAnchors extracts anchor messages from a message list
func (bi *BlockIntegration) CollectAnchors(messages []messaging.Message) []*messaging.BlockAnchor {
	anchors := make([]*messaging.BlockAnchor, 0)
	
	for _, msg := range messages {
		if anchor, ok := msg.(*messaging.BlockAnchor); ok {
			anchors = append(anchors, anchor)
		}
	}
	
	return anchors
}

// ValidateAnchor validates an anchor message
func (bi *BlockIntegration) ValidateAnchor(anchor *messaging.BlockAnchor) bool {
	// Basic validation - must have signature
	if anchor.Signature == nil {
		return false
	}
	
	// Must have anchor body
	if anchor.Anchor == nil {
		return false
	}
	
	// Additional validation could be added here
	return true
}

// DetectMissingSequences detects gaps in message sequences
func (bi *BlockIntegration) DetectMissingSequences(messages []messaging.Message, source *url.URL) []uint64 {
	sequences := make([]uint64, 0)
	
	// Collect all sequences from this source
	for _, msg := range messages {
		if seq, ok := msg.(*messaging.SequencedMessage); ok {
			if seq.Source.Equal(source) {
				sequences = append(sequences, seq.Number)
			}
		}
	}
	
	if len(sequences) == 0 {
		return nil
	}
	
	// Sort sequences
	sort.Slice(sequences, func(i, j int) bool {
		return sequences[i] < sequences[j]
	})
	
	// Find gaps
	missing := make([]uint64, 0)
	for i := 1; i < len(sequences); i++ {
		prev := sequences[i-1]
		curr := sequences[i]
		
		// If gap detected
		for seq := prev + 1; seq < curr; seq++ {
			missing = append(missing, seq)
		}
	}
	
	return missing
}

// TriggerRecovery triggers recovery for missing messages
func (bi *BlockIntegration) TriggerRecovery(ctx context.Context, source *url.URL, missingSeqs []uint64) error {
	if len(missingSeqs) == 0 {
		return nil
	}
	
	bi.conductor.logger.Info("Triggering recovery for missing sequences",
		"source", source,
		"missing", missingSeqs)
	
	// Would trigger actual recovery through sequence tracker
	if bi.conductor.sequenceTracker != nil {
		// Request recovery for the range
		minSeq := missingSeqs[0]
		maxSeq := missingSeqs[len(missingSeqs)-1]
		
		return bi.conductor.sequenceTracker.RequestMissingMessages(
			ctx, source.String(), MessageTypeSynthetic, minSeq, maxSeq)
	}
	
	return nil
}