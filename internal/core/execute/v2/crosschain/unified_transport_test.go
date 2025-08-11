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
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// unifiedMockDispatcher implements execute.Dispatcher for unified transport testing
type unifiedMockDispatcher struct {
	sentMessages []messaging.Message
}

func (m *unifiedMockDispatcher) Submit(ctx context.Context, dest *url.URL, msg messaging.Message) error {
	m.sentMessages = append(m.sentMessages, msg)
	return nil
}

func (m *unifiedMockDispatcher) Send(ctx context.Context, msg messaging.Message) (<-chan any, func()) {
	ch := make(chan any, 1)
	ch <- msg
	close(ch)
	return ch, func() {}
}

func (m *unifiedMockDispatcher) Close() error {
	return nil
}

// TestUnifiedTransportMixedMessages tests sending both anchors and synthetics together
func TestUnifiedTransportMixedMessages(t *testing.T) {
	// Setup
	logger := (*logging.TestLogger)(nil)
	dispatcher := &unifiedMockDispatcher{}
	conductor := NewCrossChainConductor(dispatcher, logger)
	
	// Create test database
	store := storage.OpenInMemory(nil)
	defer store.Close()
	db := database.OpenInMemory(store, logger.With("test", "db"))
	batch := db.Begin(true)
	defer batch.Discard()
	
	// Create test chains
	sourceChain, err := batch.Account(protocol.PartitionUrl("source").WithTxID([32]byte{1})).MainChain().Get()
	require.NoError(t, err)
	rootChain, err := batch.Account(protocol.PartitionUrl("source")).RootChain().Get()
	require.NoError(t, err)
	
	// Create mixed messages (synthetics and anchors)
	messages := []CrossChainMessage{
		// Synthetic transaction 1
		&UnifiedMessage{
			Type:        MessageTypeSynthetic,
			Source:      protocol.PartitionUrl("source"),
			Destination: protocol.PartitionUrl("destination"),
			Sequence:    1,
			Payload: &messaging.TransactionMessage{
				Transaction: &protocol.Transaction{
					Header: protocol.TransactionHeader{Principal: protocol.AcmeUrl()},
					Body:   &protocol.SendTokens{To: []*protocol.TokenRecipient{{}}},
				},
			},
			SourceChain: sourceChain,
			RootChain:   rootChain,
			BlockIndex:  100,
		},
		// Synthetic transaction 2
		&UnifiedMessage{
			Type:        MessageTypeSynthetic,
			Source:      protocol.PartitionUrl("source"),
			Destination: protocol.PartitionUrl("destination"),
			Sequence:    2,
			Payload: &messaging.TransactionMessage{
				Transaction: &protocol.Transaction{
					Header: protocol.TransactionHeader{Principal: protocol.AcmeUrl()},
					Body:   &protocol.SendTokens{To: []*protocol.TokenRecipient{{}}},
				},
			},
			SourceChain: sourceChain,
			RootChain:   rootChain,
			BlockIndex:  100,
		},
		// Anchor 1
		&UnifiedMessage{
			Type:        MessageTypeAnchor,
			Source:      protocol.PartitionUrl("source"),
			Destination: protocol.PartitionUrl("destination"),
			Sequence:    3,
			Payload: &messaging.BlockAnchor{
				Anchor: &protocol.BlockValidatorAnchor{
					MinorBlockIndex: 100,
				},
			},
			SourceChain: sourceChain,
			RootChain:   rootChain,
			BlockIndex:  100,
		},
		// Anchor 2
		&UnifiedMessage{
			Type:        MessageTypeAnchor,
			Source:      protocol.PartitionUrl("source"),
			Destination: protocol.PartitionUrl("destination"),
			Sequence:    4,
			Payload: &messaging.BlockAnchor{
				Anchor: &protocol.BlockValidatorAnchor{
					MinorBlockIndex: 101,
				},
			},
			SourceChain: sourceChain,
			RootChain:   rootChain,
			BlockIndex:  101,
		},
	}
	
	// Send through unified transport
	err = conductor.SendCrossChainMessages(context.Background(), messages)
	require.NoError(t, err)
	
	// Verify metrics
	transportMetrics := conductor.unifiedTransport.GetMetrics()
	require.Equal(t, int64(2), transportMetrics.SyntheticsSent)
	require.Equal(t, int64(2), transportMetrics.AnchorsSent)
	
	// Since we have 4 messages to the same destination, they should use a collection proof
	require.GreaterOrEqual(t, transportMetrics.CollectionProofsUsed, int64(1))
	
	t.Logf("Transport metrics: synthetics=%d, anchors=%d, collection_proofs=%d",
		transportMetrics.SyntheticsSent, transportMetrics.AnchorsSent, transportMetrics.CollectionProofsUsed)
}

// TestUnifiedTransportBatching tests that messages are properly batched by destination
func TestUnifiedTransportBatching(t *testing.T) {
	// Setup
	logger := (*logging.TestLogger)(nil)
	dispatcher := &unifiedMockDispatcher{}
	conductor := NewCrossChainConductor(dispatcher, logger)
	
	// Create test database
	store := storage.OpenInMemory(nil)
	defer store.Close()
	db := database.OpenInMemory(store, logger.With("test", "db"))
	batch := db.Begin(true)
	defer batch.Discard()
	
	// Create test chains
	sourceChain, err := batch.Account(protocol.PartitionUrl("source").WithTxID([32]byte{1})).MainChain().Get()
	require.NoError(t, err)
	rootChain, err := batch.Account(protocol.PartitionUrl("source")).RootChain().Get()
	require.NoError(t, err)
	
	// Create messages to different destinations
	messages := []CrossChainMessage{
		// To destination1 (3 messages - should use collection proof)
		&UnifiedMessage{
			Type:        MessageTypeSynthetic,
			Destination: protocol.PartitionUrl("destination1"),
			Sequence:    1,
			Payload: &messaging.TransactionMessage{
				Transaction: &protocol.Transaction{
					Header: protocol.TransactionHeader{Principal: protocol.AcmeUrl()},
					Body:   &protocol.SendTokens{},
				},
			},
			SourceChain: sourceChain,
			RootChain:   rootChain,
		},
		&UnifiedMessage{
			Type:        MessageTypeSynthetic,
			Destination: protocol.PartitionUrl("destination1"),
			Sequence:    2,
			Payload: &messaging.TransactionMessage{
				Transaction: &protocol.Transaction{
					Header: protocol.TransactionHeader{Principal: protocol.AcmeUrl()},
					Body:   &protocol.SendTokens{},
				},
			},
			SourceChain: sourceChain,
			RootChain:   rootChain,
		},
		&UnifiedMessage{
			Type:        MessageTypeAnchor,
			Destination: protocol.PartitionUrl("destination1"),
			Sequence:    3,
			Payload: &messaging.BlockAnchor{
				Anchor: &protocol.BlockValidatorAnchor{MinorBlockIndex: 100},
			},
			SourceChain: sourceChain,
			RootChain:   rootChain,
		},
		// To destination2 (1 message - should use individual proof)
		&UnifiedMessage{
			Type:        MessageTypeSynthetic,
			Destination: protocol.PartitionUrl("destination2"),
			Sequence:    4,
			Payload: &messaging.TransactionMessage{
				Transaction: &protocol.Transaction{
					Header: protocol.TransactionHeader{Principal: protocol.AcmeUrl()},
					Body:   &protocol.SendTokens{},
				},
			},
			SourceChain: sourceChain,
			RootChain:   rootChain,
		},
	}
	
	// Send through unified transport
	err = conductor.SendCrossChainMessages(context.Background(), messages)
	require.NoError(t, err)
	
	// Verify batching metrics
	transportMetrics := conductor.unifiedTransport.GetMetrics()
	require.Equal(t, int64(2), transportMetrics.BatchesCreated) // 2 destinations
	require.GreaterOrEqual(t, transportMetrics.CollectionProofsUsed, int64(1)) // destination1 batch
	require.GreaterOrEqual(t, transportMetrics.IndividualProofsUsed, int64(1)) // destination2 single
	
	t.Logf("Batching: %d batches created, %d collection proofs, %d individual proofs",
		transportMetrics.BatchesCreated, transportMetrics.CollectionProofsUsed, transportMetrics.IndividualProofsUsed)
}

// TestUnifiedTransportCollectionProofThreshold tests the collection proof threshold
func TestUnifiedTransportCollectionProofThreshold(t *testing.T) {
	// Setup
	logger := (*logging.TestLogger)(nil)
	dispatcher := &unifiedMockDispatcher{}
	conductor := NewCrossChainConductor(dispatcher, logger)
	
	// Create test database
	store := storage.OpenInMemory(nil)
	defer store.Close()
	db := database.OpenInMemory(store, logger.With("test", "db"))
	batch := db.Begin(true)
	defer batch.Discard()
	
	// Create test chains
	sourceChain, err := batch.Account(protocol.PartitionUrl("source").WithTxID([32]byte{1})).MainChain().Get()
	require.NoError(t, err)
	rootChain, err := batch.Account(protocol.PartitionUrl("source")).RootChain().Get()
	require.NoError(t, err)
	
	testCases := []struct {
		name          string
		messageCount  int
		expectCollection bool
	}{
		{"Single message", 1, false},
		{"Two messages (threshold)", 2, true},
		{"Five messages", 5, true},
		{"Fifty messages", 50, true},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create new conductor for each test
			conductor := NewCrossChainConductor(dispatcher, logger)
			
			// Create messages
			messages := make([]CrossChainMessage, tc.messageCount)
			for i := 0; i < tc.messageCount; i++ {
				messages[i] = &UnifiedMessage{
					Type:        MessageTypeSynthetic,
					Destination: protocol.PartitionUrl("destination"),
					Sequence:    uint64(i + 1),
					Payload: &messaging.TransactionMessage{
						Transaction: &protocol.Transaction{
							Header: protocol.TransactionHeader{Principal: protocol.AcmeUrl()},
							Body:   &protocol.SendTokens{},
						},
					},
					SourceChain: sourceChain,
					RootChain:   rootChain,
				}
			}
			
			// Send messages
			err := conductor.SendCrossChainMessages(context.Background(), messages)
			require.NoError(t, err)
			
			// Check metrics
			metrics := conductor.unifiedTransport.GetMetrics()
			if tc.expectCollection {
				require.GreaterOrEqual(t, metrics.CollectionProofsUsed, int64(1),
					"Expected collection proof for %d messages", tc.messageCount)
			} else {
				require.Equal(t, int64(0), metrics.CollectionProofsUsed,
					"Expected no collection proof for %d messages", tc.messageCount)
				require.Equal(t, int64(tc.messageCount), metrics.IndividualProofsUsed,
					"Expected individual proofs for %d messages", tc.messageCount)
			}
		})
	}
}

// TestUnifiedTransportWithRealProofs tests with actual proof generation
func TestUnifiedTransportWithRealProofs(t *testing.T) {
	// Setup
	logger := (*logging.TestLogger)(nil)
	dispatcher := &unifiedMockDispatcher{}
	conductor := NewCrossChainConductor(dispatcher, logger)
	
	// Create test database
	store := storage.OpenInMemory(nil)
	defer store.Close()
	db := database.OpenInMemory(store, logger.With("test", "db"))
	batch := db.Begin(true)
	defer batch.Discard()
	
	// Create and populate source chain with entries
	sourceAccount := protocol.PartitionUrl("source").WithTxID([32]byte{1})
	sourceChain, err := batch.Account(sourceAccount).MainChain().Get()
	require.NoError(t, err)
	
	// Add entries to the chain for proof generation
	for i := uint64(1); i <= 10; i++ {
		entry := make([]byte, 32)
		entry[0] = byte(i)
		err = sourceChain.AddEntry(entry, false)
		require.NoError(t, err)
	}
	
	rootChain, err := batch.Account(protocol.PartitionUrl("source")).RootChain().Get()
	require.NoError(t, err)
	
	// Create messages that reference actual chain entries
	messages := []CrossChainMessage{
		&UnifiedMessage{
			Type:        MessageTypeSynthetic,
			Destination: protocol.PartitionUrl("destination"),
			Sequence:    1,
			Payload: &messaging.TransactionMessage{
				Transaction: &protocol.Transaction{
					Header: protocol.TransactionHeader{Principal: protocol.AcmeUrl()},
					Body:   &protocol.SendTokens{},
				},
			},
			SourceChain: sourceChain,
			RootChain:   rootChain,
		},
		&UnifiedMessage{
			Type:        MessageTypeSynthetic,
			Destination: protocol.PartitionUrl("destination"),
			Sequence:    2,
			Payload: &messaging.TransactionMessage{
				Transaction: &protocol.Transaction{
					Header: protocol.TransactionHeader{Principal: protocol.AcmeUrl()},
					Body:   &protocol.SendTokens{},
				},
			},
			SourceChain: sourceChain,
			RootChain:   rootChain,
		},
		&UnifiedMessage{
			Type:        MessageTypeAnchor,
			Destination: protocol.PartitionUrl("destination"),
			Sequence:    3,
			Payload: &messaging.BlockAnchor{
				Anchor: &protocol.BlockValidatorAnchor{
					MinorBlockIndex: 100,
				},
			},
			SourceChain: sourceChain,
			RootChain:   rootChain,
		},
	}
	
	// Send through unified transport
	err = conductor.SendCrossChainMessages(context.Background(), messages)
	require.NoError(t, err)
	
	// Verify proof generation
	proofMetrics := conductor.proofService.GetMetrics()
	require.GreaterOrEqual(t, proofMetrics.CollectionProofsCreated, int64(1))
	require.Equal(t, int64(3), proofMetrics.TransactionsInCollections)
	require.Equal(t, int64(2), proofMetrics.ProofsSaved) // 3 messages with 1 proof = 2 proofs saved
	
	t.Logf("Proof metrics: collection_proofs=%d, transactions_in_collections=%d, proofs_saved=%d",
		proofMetrics.CollectionProofsCreated, proofMetrics.TransactionsInCollections, proofMetrics.ProofsSaved)
}