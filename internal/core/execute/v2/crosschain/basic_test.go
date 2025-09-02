// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestMessageType_Constants(t *testing.T) {
	require.Equal(t, MessageType(0), MessageTypeAnchor)
	require.Equal(t, MessageType(1), MessageTypeSynthetic)
	require.Equal(t, MessageType(2), MessageTypeDirectoryAnchor)
	require.Equal(t, MessageType(3), MessageTypeBlockSummary)
	require.Equal(t, MessageType(4), MessageTypeOther)
}

func TestProofType_Constants(t *testing.T) {
	require.Equal(t, ProofType(0), ProofTypeSynthetic)
	require.Equal(t, ProofType(1), ProofTypeAnchor)
	require.Equal(t, ProofType(2), ProofTypeReceipt)
	require.Equal(t, ProofType(3), ProofTypeUnified)
}

func TestRecoveryType_String(t *testing.T) {
	require.Equal(t, "anchor", RecoveryTypeAnchor.String())
	require.Equal(t, "synthetic", RecoveryTypeSynthetic.String())
	require.Equal(t, "unknown", RecoveryType(999).String())
}

func TestDestinationKey_Structure(t *testing.T) {
	key := DestinationKey{
		Type:        MessageTypeAnchor,
		Destination: "test://destination",
	}
	
	require.Equal(t, MessageTypeAnchor, key.Type)
	require.Equal(t, "test://destination", key.Destination)
}

func TestProofRequest_Structure(t *testing.T) {
	dest := protocol.AccountUrl("test", "destination")
	chainUrl := protocol.AccountUrl("test", "chain")
	
	req := ProofRequest{
		Type:        ProofTypeSynthetic,
		Destination: dest,
		Sequences:   []uint64{1, 2, 3},
		ChainURL:    chainUrl,
		BlockIndex:  42,
		Metadata:    "test-metadata",
	}
	
	require.Equal(t, ProofTypeSynthetic, req.Type)
	require.Equal(t, dest, req.Destination)
	require.Equal(t, []uint64{1, 2, 3}, req.Sequences)
	require.Equal(t, chainUrl, req.ChainURL)
	require.Equal(t, uint64(42), req.BlockIndex)
	require.Equal(t, "test-metadata", req.Metadata)
}

func TestProofResponse_Structure(t *testing.T) {
	proof := &protocol.AnnotatedReceipt{}
	
	resp := ProofResponse{
		Proof:        proof,
		ProofType:    ProofTypeAnchor,
		Sequences:    []uint64{5, 6, 7},
		IsCollection: true,
		ProofSavings: 2,
	}
	
	require.Equal(t, proof, resp.Proof)
	require.Equal(t, ProofTypeAnchor, resp.ProofType)
	require.Equal(t, []uint64{5, 6, 7}, resp.Sequences)
	require.True(t, resp.IsCollection)
	require.Equal(t, 2, resp.ProofSavings)
}

func TestProofBatch_Structure(t *testing.T) {
	dest := protocol.AccountUrl("test", "destination")
	req := ProofRequest{Type: ProofTypeSynthetic}
	
	batch := ProofBatch{
		Destination: dest,
		Requests:    []ProofRequest{req},
	}
	
	require.Equal(t, dest, batch.Destination)
	require.Len(t, batch.Requests, 1)
	require.Equal(t, ProofTypeSynthetic, batch.Requests[0].Type)
}

func TestSyntheticTransaction_Structure(t *testing.T) {
	dest := protocol.AccountUrl("test", "destination")
	chainUrl := protocol.AccountUrl("test", "chain")
	hash := []byte("test-hash")
	
	tx := SyntheticTransaction{
		Destination: dest,
		SequenceNum: 42,
		Sequence:    42, // Alias
		ChainURL:    chainUrl,
		Hash:        hash,
	}
	
	require.Equal(t, dest, tx.Destination)
	require.Equal(t, uint64(42), tx.SequenceNum)
	require.Equal(t, uint64(42), tx.Sequence)
	require.Equal(t, chainUrl, tx.ChainURL)
	require.Equal(t, hash, tx.Hash)
}

func TestRecoveryRequest_Structure(t *testing.T) {
	req := RecoveryRequest{
		Requester:  "test-requester",
		FromNumber: 100,
	}
	
	require.Equal(t, "test-requester", req.Requester)
	require.Equal(t, uint64(100), req.FromNumber)
}

func TestBatchRecoveryRequest_Structure(t *testing.T) {
	chainUrl := protocol.AccountUrl("test", "chain")
	sequences := []uint64{1, 2, 3}
	
	req := BatchRecoveryRequest{
		PartitionID:      "test-partition",
		Type:             RecoveryTypeAnchor,
		MissingSequences: sequences,
		ChainURL:         chainUrl,
	}
	
	require.Equal(t, "test-partition", req.PartitionID)
	require.Equal(t, RecoveryTypeAnchor, req.Type)
	require.Equal(t, sequences, req.MissingSequences)
	require.Equal(t, chainUrl, req.ChainURL)
}

func TestBatchRecoveryResponse_Structure(t *testing.T) {
	resp := BatchRecoveryResponse{
		PartitionID:  "test-partition",
		Type:         RecoveryTypeSynthetic,
		BatchSize:    5,
		ProofSavings: 4,
	}
	
	require.Equal(t, "test-partition", resp.PartitionID)
	require.Equal(t, RecoveryTypeSynthetic, resp.Type)
	require.Equal(t, 5, resp.BatchSize)
	require.Equal(t, 4, resp.ProofSavings)
}

func TestRecoveredTransaction_Structure(t *testing.T) {
	hash := []byte("test-hash")
	data := []byte("test-data")
	
	tx := RecoveredTransaction{
		Hash:        hash,
		SequenceNum: 42,
		Type:        "test-type",
		Data:        data,
	}
	
	require.Equal(t, hash, tx.Hash)
	require.Equal(t, uint64(42), tx.SequenceNum)
	require.Equal(t, "test-type", tx.Type)
	require.Equal(t, data, tx.Data)
}

func TestPendingTransmission_Structure(t *testing.T) {
	dest := protocol.AccountUrl("test", "destination")
	
	pending := PendingTransmission{
		ID:          "test-id",
		Destination: dest,
		AttemptNum:  3,
	}
	
	require.Equal(t, "test-id", pending.ID)
	require.Equal(t, dest, pending.Destination)
	require.Equal(t, 3, pending.AttemptNum)
}

func TestDestinationQueue_Structure(t *testing.T) {
	key := DestinationKey{
		Type:        MessageTypeAnchor,
		Destination: "test://destination",
	}
	
	queue := DestinationQueue{
		Key:          key,
		IsBlocked:    true,
		FailureCount: 5,
		SuccessCount: 10,
	}
	
	require.Equal(t, key, queue.Key)
	require.True(t, queue.IsBlocked)
	require.Equal(t, int64(5), queue.FailureCount)
	require.Equal(t, int64(10), queue.SuccessCount)
}

func TestProofMetrics_Structure(t *testing.T) {
	metrics := ProofMetrics{
		IndividualProofsCreated:   5,
		CollectionProofsCreated:   3,
		TransactionsInCollections: 15,
		ProofsSaved:               12,
		ValidationAttempts:        20,
		ValidationSuccesses:       18,
		ValidationFailures:        2,
		ProofGenErrors:           1,
		ValidationErrors:         1,
	}
	
	require.Equal(t, int64(5), metrics.IndividualProofsCreated)
	require.Equal(t, int64(3), metrics.CollectionProofsCreated)
	require.Equal(t, int64(15), metrics.TransactionsInCollections)
	require.Equal(t, int64(12), metrics.ProofsSaved)
	require.Equal(t, int64(20), metrics.ValidationAttempts)
	require.Equal(t, int64(18), metrics.ValidationSuccesses)
	require.Equal(t, int64(2), metrics.ValidationFailures)
	require.Equal(t, int64(1), metrics.ProofGenErrors)
	require.Equal(t, int64(1), metrics.ValidationErrors)
}

func TestTransportMetrics_Structure(t *testing.T) {
	metrics := TransportMetrics{
		SyntheticsSent:       10,
		AnchorsSent:          5,
		BatchesCreated:       3,
		MessagesPerBatch:     []int{2, 4, 6},
		CollectionProofsUsed: 2,
		IndividualProofsUsed: 1,
		SendErrors:           0,
		BatchErrors:          0,
	}
	
	require.Equal(t, int64(10), metrics.SyntheticsSent)
	require.Equal(t, int64(5), metrics.AnchorsSent)
	require.Equal(t, int64(3), metrics.BatchesCreated)
	require.Equal(t, []int{2, 4, 6}, metrics.MessagesPerBatch)
	require.Equal(t, int64(2), metrics.CollectionProofsUsed)
	require.Equal(t, int64(1), metrics.IndividualProofsUsed)
}

func TestUnifiedMessage_Structure(t *testing.T) {
	dest := protocol.AccountUrl("test", "destination")
	source := protocol.AccountUrl("test", "source")
	
	msg := UnifiedMessage{
		Type:        MessageTypeSynthetic,
		Source:      source,
		Destination: dest,
		Sequence:    42,
		BlockIndex:  100,
		Metadata:    "test-metadata",
	}
	
	require.Equal(t, MessageTypeSynthetic, msg.Type)
	require.Equal(t, source, msg.Source)
	require.Equal(t, dest, msg.Destination)
	require.Equal(t, uint64(42), msg.Sequence)
	require.Equal(t, uint64(100), msg.BlockIndex)
	require.Equal(t, "test-metadata", msg.Metadata)
}

func TestUnifiedMessage_InterfaceMethods(t *testing.T) {
	dest := protocol.AccountUrl("test", "destination")
	source := protocol.AccountUrl("test", "source")
	
	msg := &UnifiedMessage{
		Type:        MessageTypeAnchor,
		Source:      source,
		Destination: dest,
		Sequence:    123,
	}
	
	// Test all interface methods
	require.Equal(t, dest, msg.GetDestination())
	require.Equal(t, uint64(123), msg.GetSequence())
	require.Equal(t, MessageTypeAnchor, msg.GetType())
	require.Equal(t, source, msg.GetSource())
	require.Nil(t, msg.GetPayload())    // Not set in this test
	require.Nil(t, msg.GetSourceChain()) // Not set in this test
	require.Nil(t, msg.GetRootChain())   // Not set in this test
}

func TestDestinationSendState_Basic(t *testing.T) {
	// Test that the struct can be created and accessed
	dest := protocol.AccountUrl("test", "destination")
	
	state := DestinationSendState{
		Destination:    dest,
		SentTxIndex:    100,
		CurrentTxIndex: 105,
	}
	
	require.Equal(t, dest, state.Destination)
	require.Equal(t, uint64(100), state.SentTxIndex)
	require.Equal(t, uint64(105), state.CurrentTxIndex)
}

// Test utility function
func TestMin(t *testing.T) {
	require.Equal(t, 3, min(3, 5))
	require.Equal(t, 3, min(5, 3))
	require.Equal(t, 7, min(7, 7))
}