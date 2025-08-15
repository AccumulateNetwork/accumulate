// Step 4: Test Metrics functionality
package crosschain

import (
	"testing"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
)

func TestStep4ConductorMetrics(t *testing.T) {
	logger := logging.OptionalLogger{}
	dispatcher := &MockDispatcher{}
	
	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()
	
	// Test GetMetrics method
	sent, errors, retried, transmissionErrors := conductor.GetMetrics()
	require.GreaterOrEqual(t, sent, int64(0))
	require.GreaterOrEqual(t, errors, int64(0))
	require.GreaterOrEqual(t, retried, int64(0))
	require.GreaterOrEqual(t, transmissionErrors, int64(0))
}

func TestStep4PartitionHealth(t *testing.T) {
	logger := logging.OptionalLogger{}
	dispatcher := &MockDispatcher{}
	
	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()
	
	// Test CheckPartitionHealth method
	health := conductor.CheckPartitionHealth()
	require.NotNil(t, health)
	require.IsType(t, make(map[string]interface{}), health)
}

func TestStep4ProofMetrics(t *testing.T) {
	logger := logging.OptionalLogger{}
	dispatcher := &MockDispatcher{}
	
	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()
	
	// Test GetProofMetrics method
	metrics := conductor.GetProofMetrics()
	require.GreaterOrEqual(t, metrics.IndividualProofsCreated, int64(0))
	require.GreaterOrEqual(t, metrics.CollectionProofsCreated, int64(0))
	require.GreaterOrEqual(t, metrics.ValidationAttempts, int64(0))
}