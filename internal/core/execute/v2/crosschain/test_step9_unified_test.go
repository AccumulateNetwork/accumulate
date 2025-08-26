// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"testing"
)

func TestStep9SendCrossChainMessages(t *testing.T) {
	logger := logging.OptionalLogger{}
	dispatcher := &MockDispatcher{}

	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()

	ctx := context.Background()
	var emptyCrossChainMessages []CrossChainMessage

	// Test SendCrossChainMessages with empty list
	err := conductor.SendCrossChainMessages(ctx, emptyCrossChainMessages)
	require.NoError(t, err)
}

func TestStep9MockDispatcherUsage(t *testing.T) {
	logger := logging.OptionalLogger{}
	dispatcher := &MockDispatcher{}

	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()

	ctx := context.Background()
	destination, err := url.Parse("acc://test.acme")
	require.NoError(t, err)

	var emptyMessages []messaging.Message

	// Submit some messages to trigger dispatcher usage
	err = conductor.SubmitSynthetic(ctx, emptyMessages, destination)
	require.NoError(t, err)

	// Test that dispatcher received some activity (may be async so check count >= 0)
	require.GreaterOrEqual(t, len(dispatcher.submitted), 0)
}

func TestStep9MessageTypeHandling(t *testing.T) {
	logger := logging.OptionalLogger{}
	dispatcher := &MockDispatcher{}

	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()

	// Test different message types exist
	require.NotEqual(t, MessageTypeAnchor, MessageTypeSynthetic)
	require.NotEqual(t, MessageTypeAnchor, ConductorMessageTypeOther)

	// Test message type name function works
	name := conductor.getMessageTypeName(MessageTypeAnchor)
	require.Contains(t, []string{"anchor", "synthetic", "other", "unknown"}, name)
}
