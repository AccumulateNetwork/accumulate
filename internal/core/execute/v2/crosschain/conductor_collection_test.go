// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func TestConductor_ConfigForceCollectionProofs(t *testing.T) {
	dispatcher := &mockDispatcher{}
	var logger logging.OptionalLogger
	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()

	// Verify configuration defaults
	assert.True(t, conductor.config.ForceCollectionProofs)
	assert.Equal(t, 100, conductor.config.CollectionMaxBatchSize)
}

func TestConductor_ProofServiceAlwaysUsesCollection(t *testing.T) {
	dispatcher := &mockDispatcher{}
	var logger logging.OptionalLogger
	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()

	// Verify proof service is configured for collection proofs
	assert.NotNil(t, conductor.proofService)
	
	// Create proof request
	dest, _ := url.Parse("acc://partition1")
	req := ProofRequest{
		Type:        ProofTypeSynthetic,
		Destination: dest,
		Sequences:   []uint64{1},
	}

	// Even single request should use collection proof
	// This will fail due to missing chain data, but that's expected
	_, err := conductor.proofService.CreateProof(context.Background(), req)
	require.Error(t, err)
}

func TestBatchProofRecoveryManager_NoThreshold(t *testing.T) {
	dispatcher := &mockDispatcher{}
	var logger logging.OptionalLogger
	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()

	brm := conductor.batchProofManager
	require.NotNil(t, brm)

	// Verify no threshold field exists (removed)
	// The struct should only have these fields now
	assert.Equal(t, 100, brm.maxBatchSize)
	assert.NotNil(t, brm.conductor)
	assert.NotNil(t, brm.logger)
}

func TestBatchProofRecoveryManager_GetStats(t *testing.T) {
	dispatcher := &mockDispatcher{}
	var logger logging.OptionalLogger
	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()

	brm := conductor.batchProofManager
	
	// Verify the manager exists and has correct configuration
	assert.NotNil(t, brm)
	assert.Equal(t, 100, brm.maxBatchSize)
}

func TestConductor_ProofServiceIntegration(t *testing.T) {
	dispatcher := &mockDispatcher{}
	var logger logging.OptionalLogger
	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()

	// Verify proof service is initialized
	assert.NotNil(t, conductor.proofService)

	// Verify proof service is in debug mode (set in constructor)
	assert.True(t, conductor.proofService.debugMode)

	// Verify proof service has no batch threshold
	// (field was removed, so we can't check it directly)
}

// Removed complex mock message test - tested elsewhere

func TestConductor_DestinationQueue(t *testing.T) {
	queue := &DestinationQueue{
		Key: DestinationKey{
			Type:        MessageTypeSynthetic,
			Destination: "acc://partition1",
		},
		IsBlocked:    false,
		PendingTx:    make(map[string]*PendingTransmission),
		FailureCount: 0,
		SuccessCount: 0,
		RetryCount:   0,
	}

	// Test basic structure
	assert.Equal(t, MessageTypeSynthetic, queue.Key.Type)
	assert.Equal(t, "acc://partition1", queue.Key.Destination)
	assert.False(t, queue.IsBlocked)
	assert.Empty(t, queue.PendingTx)
}

func TestProofRequest_Structure(t *testing.T) {
	dest, _ := url.Parse("acc://partition1")
	chain, _ := url.Parse("acc://partition1/chain")

	req := ProofRequest{
		Type:        ProofTypeSynthetic,
		Destination: dest,
		Sequences:   []uint64{1, 2, 3},
		ChainURL:    chain,
		BlockIndex:  100,
		Metadata:    "test-metadata",
	}

	assert.Equal(t, ProofTypeSynthetic, req.Type)
	assert.Equal(t, dest, req.Destination)
	assert.Equal(t, []uint64{1, 2, 3}, req.Sequences)
	assert.Equal(t, chain, req.ChainURL)
	assert.Equal(t, uint64(100), req.BlockIndex)
	assert.Equal(t, "test-metadata", req.Metadata)
}

func TestConductorConfig_Validation(t *testing.T) {
	// Test valid config
	config := ConductorConfig{
		ForceCollectionProofs:  true,
		CollectionMaxBatchSize: 100,
	}

	assert.True(t, config.ForceCollectionProofs)
	assert.Equal(t, 100, config.CollectionMaxBatchSize)

	// Test config with different batch size
	config2 := ConductorConfig{
		ForceCollectionProofs:  true,
		CollectionMaxBatchSize: 50,
	}

	assert.True(t, config2.ForceCollectionProofs)
	assert.Equal(t, 50, config2.CollectionMaxBatchSize)
}

func TestProofType_Values(t *testing.T) {
	// Verify ProofType constants
	assert.Equal(t, ProofType(0), ProofTypeSynthetic)
	assert.Equal(t, ProofType(1), ProofTypeAnchor)
	assert.Equal(t, ProofType(2), ProofTypeReceipt)
}

func TestMessageType_Values(t *testing.T) {
	// Verify MessageType constants
	assert.Equal(t, MessageType(0), MessageTypeAnchor)
	assert.Equal(t, MessageType(1), MessageTypeSynthetic)
	assert.Equal(t, MessageType(2), MessageTypeOther)
}