// Step 3: Test ProofService functionality
package crosschain

import (
	"testing"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
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