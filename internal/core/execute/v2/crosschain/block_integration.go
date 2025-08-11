// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"

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