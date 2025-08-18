// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"testing"
)

func TestStep11MessageTypeNames(t *testing.T) {
	logger := logging.OptionalLogger{}
	dispatcher := &MockDispatcher{}

	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()

	// Test all message type names
	require.Equal(t, "anchor", conductor.getMessageTypeName(MessageTypeAnchor))
	require.Equal(t, "synthetic", conductor.getMessageTypeName(MessageTypeSynthetic))
	require.Equal(t, "other", conductor.getMessageTypeName(ConductorMessageTypeOther))
	require.Equal(t, "unknown", conductor.getMessageTypeName(MessageType(999)))
}

func TestStep11DestinationKeyCreation(t *testing.T) {
	logger := logging.OptionalLogger{}
	dispatcher := &MockDispatcher{}

	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()

	// Test destination key creation with different message types
	destination, err := url.Parse("acc://test.acme")
	require.NoError(t, err)

	anchorKey := conductor.createDestinationKey(MessageTypeAnchor, destination)
	syntheticKey := conductor.createDestinationKey(MessageTypeSynthetic, destination)

	require.NotEqual(t, anchorKey, syntheticKey)
	require.Equal(t, MessageTypeAnchor, anchorKey.Type)
	require.Equal(t, MessageTypeSynthetic, syntheticKey.Type)
	require.Equal(t, destination.String(), anchorKey.Destination)
	require.Equal(t, destination.String(), syntheticKey.Destination)
}

func TestStep11QueueManagement(t *testing.T) {
	logger := logging.OptionalLogger{}
	dispatcher := &MockDispatcher{}

	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()

	destination, err := url.Parse("acc://test.acme")
	require.NoError(t, err)

	// Create destination keys for different message types
	key1 := conductor.createDestinationKey(MessageTypeAnchor, destination)
	key2 := conductor.createDestinationKey(MessageTypeSynthetic, destination)

	// Get or create queues
	queue1 := conductor.getOrCreateDestinationQueue(key1)
	queue2 := conductor.getOrCreateDestinationQueue(key2)

	require.NotNil(t, queue1)
	require.NotNil(t, queue2)
	require.NotEqual(t, queue1, queue2)
}
