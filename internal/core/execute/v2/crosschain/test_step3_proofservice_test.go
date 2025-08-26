// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"testing"
)

func TestStep3ProofServiceCreation(t *testing.T) {
	logger := logging.OptionalLogger{}
	dispatcher := &MockDispatcher{}

	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()

	// Test that ProofService is properly initialized
	require.NotNil(t, conductor.proofService)
}

func TestStep3ProofServiceExists(t *testing.T) {
	logger := logging.OptionalLogger{}
	dispatcher := &MockDispatcher{}

	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()

	// Test that ProofService methods are accessible
	proofService := conductor.proofService
	require.NotNil(t, proofService)

	// Test GetBlockIntegration accessor
	blockIntegration := conductor.GetBlockIntegration()
	require.NotNil(t, blockIntegration)
}
