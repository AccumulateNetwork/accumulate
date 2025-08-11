// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMessageTypeConsolidation(t *testing.T) {
	t.Parallel()

	// Test that legacy constants map correctly to new MessageType values
	require.Equal(t, MessageTypeAnchor, ConductorMessageTypeAnchor)
	require.Equal(t, MessageTypeSynthetic, ConductorMessageTypeSynthetic)
	require.Equal(t, MessageTypeBlockSummary, ConductorMessageTypeOther)
}

func TestMessageTypeConstants(t *testing.T) {
	t.Parallel()

	// Test that MessageType constants are properly defined
	require.Equal(t, MessageType(0), MessageTypeSynthetic)
	require.Equal(t, MessageType(1), MessageTypeAnchor)
	require.Equal(t, MessageType(2), MessageTypeDirectoryAnchor)
	require.Equal(t, MessageType(3), MessageTypeBlockSummary)
}

func TestDestinationKeyWithMessageType(t *testing.T) {
	t.Parallel()

	// Test that DestinationKey works with the unified MessageType
	key1 := DestinationKey{
		Type:        MessageTypeAnchor,
		Destination: "bvn-example",
	}
	
	key2 := DestinationKey{
		Type:        ConductorMessageTypeAnchor, // Legacy constant
		Destination: "bvn-example",
	}
	
	// Both should be equal since they use the same underlying type
	require.Equal(t, key1, key2)
	require.Equal(t, key1.Type, key2.Type)
}