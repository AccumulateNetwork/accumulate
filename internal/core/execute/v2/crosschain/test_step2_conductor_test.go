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

func TestStep2ConductorCreation(t *testing.T) {
	logger := logging.OptionalLogger{}
	dispatcher := &MockDispatcher{}

	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()

	// Test that basic fields are initialized
	require.NotNil(t, conductor)
	require.NotNil(t, conductor.proofService)
	require.NotNil(t, conductor.unifiedTransport)
	require.NotNil(t, conductor.blockIntegration)
}
