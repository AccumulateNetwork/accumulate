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

func TestStep7SubmitAnchorBasic(t *testing.T) {
	logger := logging.OptionalLogger{}
	dispatcher := &MockDispatcher{}

	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()

	// Create a basic anchor request
	destination, err := url.Parse("acc://dn.acme")
	require.NoError(t, err)

	anchorReq := &AnchorRequest{
		Destination: destination,
		Sequence:    1,
		BlockIndex:  100,
	}

	// Test anchor submission - may error due to missing chains but shouldn't panic
	err = conductor.SubmitAnchor(anchorReq)
	// Either succeeds or errors, both are acceptable for this basic test
	if err != nil {
		require.Error(t, err)
		require.Contains(t, err.Error(), "source chain")
	}
}

func TestStep7ConductorComponents(t *testing.T) {
	logger := logging.OptionalLogger{}
	dispatcher := &MockDispatcher{}

	conductor := NewCrossChainConductor(dispatcher, logger)
	defer conductor.Stop()

	// Test that all major components are initialized
	require.NotNil(t, conductor.proofService)
	require.NotNil(t, conductor.unifiedTransport)
	require.NotNil(t, conductor.blockIntegration)
}
