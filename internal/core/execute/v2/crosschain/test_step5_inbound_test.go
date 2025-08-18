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
	"testing"
)

func TestStep5ProcessInboundEmpty(t *testing.T) {
	logger := logging.OptionalLogger{}
	dispatcher := &MockDispatcher{}

	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()

	ctx := context.Background()
	var emptyMessages []messaging.Message

	// Test processing empty message list
	result := conductor.ProcessInbound(ctx, emptyMessages)
	require.NotNil(t, result)
	require.Len(t, result, 0)
}

func TestStep5ProcessInboundNil(t *testing.T) {
	logger := logging.OptionalLogger{}
	dispatcher := &MockDispatcher{}

	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()

	ctx := context.Background()

	// Test processing nil message list (should handle gracefully)
	result := conductor.ProcessInbound(ctx, nil)
	require.NotNil(t, result)
	require.Len(t, result, 0)
}
