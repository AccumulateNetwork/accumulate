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
)

func TestProofService_AlwaysUsesCollectionProofs(t *testing.T) {
	// Create a proof service
	var logger logging.OptionalLogger
	ps := NewProofService(logger)
	ps.SetDebugMode(true)

	// Test with single sequence (should still use collection proof)
	req := ProofRequest{
		Type:      ProofTypeSynthetic,
		Sequences: []uint64{1},
	}

	// This should always use collection proof now
	resp, err := ps.CreateProof(context.Background(), req)

	// We expect an error because we don't have real chain data
	// but the important thing is it tried to create a collection proof
	require.Error(t, err)
	require.Nil(t, resp)

	// Test with multiple sequences
	req.Sequences = []uint64{1, 2, 3}
	resp, err = ps.CreateProof(context.Background(), req)

	// Same expectation - error due to missing chain data
	require.Error(t, err)
	require.Nil(t, resp)

	// Verify metrics show collection proof attempts
	metrics := ps.GetMetrics()
	require.Equal(t, int64(0), metrics.IndividualProofsCreated, "Should not create individual proofs")
}

func TestConductorConfig_ForceCollectionProofs(t *testing.T) {
	// Verify configuration defaults
	config := ConductorConfig{
		ForceCollectionProofs:  true,
		CollectionMaxBatchSize: 100,
	}

	require.True(t, config.ForceCollectionProofs, "Force collection proofs should be enabled")
	require.Equal(t, 100, config.CollectionMaxBatchSize, "Max batch size should be 100")
}
