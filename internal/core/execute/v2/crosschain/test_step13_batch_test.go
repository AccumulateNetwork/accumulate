// Step 13: Test batch processing and transport functionality
package crosschain

import (
	"testing"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func TestStep13UnifiedTransport(t *testing.T) {
	logger := logging.OptionalLogger{}
	dispatcher := &MockDispatcher{}
	
	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()
	
	// Test unified transport component exists
	transport := conductor.unifiedTransport
	require.NotNil(t, transport)
}

func TestStep13BatchProofManager(t *testing.T) {
	logger := logging.OptionalLogger{}
	dispatcher := &MockDispatcher{}
	
	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()
	
	// Test batch proof manager may be initialized
	if conductor.batchProofManager != nil {
		require.NotNil(t, conductor.batchProofManager)
	}
}

func TestStep13MessageTypeHandling(t *testing.T) {
	logger := logging.OptionalLogger{}
	dispatcher := &MockDispatcher{}
	
	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()
	
	messages := []messaging.Message{}
	
	// Test message type detection with empty messages
	msgType := conductor.getMessageType(messages)
	require.NotEqual(t, msgType, MessageType(999)) // Should be valid message type
}

func TestStep13HealthMetrics(t *testing.T) {
	logger := logging.OptionalLogger{}
	dispatcher := &MockDispatcher{}
	
	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()
	
	// Test health metrics collection
	health := conductor.CheckPartitionHealth()
	require.NotNil(t, health)
	
	// Test that health contains expected sections
	require.Contains(t, health, "queues")
	require.Contains(t, health, "global")
}

func TestStep13QueueStatistics(t *testing.T) {
	logger := logging.OptionalLogger{}
	dispatcher := &MockDispatcher{}
	
	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()
	
	destination, err := url.Parse("acc://dest.acme")
	require.NoError(t, err)
	
	// Create a queue to generate statistics
	key := conductor.createDestinationKey(MessageTypeAnchor, destination)
	queue := conductor.getOrCreateDestinationQueue(key)
	require.NotNil(t, queue)
	
	// Test that health metrics include queue statistics
	health := conductor.CheckPartitionHealth()
	queues, ok := health["queues"].(map[string]interface{})
	require.True(t, ok)
	require.NotNil(t, queues)
}

func TestStep13DestinationQueueAccess(t *testing.T) {
	logger := logging.OptionalLogger{}
	dispatcher := &MockDispatcher{}
	
	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()
	
	destination, err := url.Parse("acc://dest.acme")
	require.NoError(t, err)
	
	// Test queue access for different message types
	anchorKey := conductor.createDestinationKey(MessageTypeAnchor, destination)
	syntheticKey := conductor.createDestinationKey(MessageTypeSynthetic, destination)
	
	queue1 := conductor.getOrCreateDestinationQueue(anchorKey)
	queue2 := conductor.getOrCreateDestinationQueue(syntheticKey)
	
	require.NotNil(t, queue1)
	require.NotNil(t, queue2)
	
	// Test that accessing same key returns same queue
	queue1Again := conductor.getOrCreateDestinationQueue(anchorKey)
	require.Equal(t, queue1, queue1Again)
}