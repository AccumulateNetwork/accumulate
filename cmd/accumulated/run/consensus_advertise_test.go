// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdvertiseConsensusEndpoint_Private(t *testing.T) {
	priv := true
	inst := &Instance{
		config: &Config{P2P: &P2P{Private: &priv}},
		logger: slog.Default(),
	}
	inst.advertiseConsensusEndpoint("bvn1", "203.0.113.5", 26656, 26657)

	adv := inst.consensusAdv.byPartition["bvn1"]
	require.True(t, adv.Private, "a private node must mark its advertisement Private (#4047 §6)")
	require.Equal(t, 26656, adv.Port)
}

func TestAdvertiseConsensusEndpoint_PublicByDefault(t *testing.T) {
	inst := &Instance{
		config: &Config{P2P: &P2P{}}, // no Private flag
		logger: slog.Default(),
	}
	inst.advertiseConsensusEndpoint("bvn1", "203.0.113.5", 26656, 26657)

	require.False(t, inst.consensusAdv.byPartition["bvn1"].Private,
		"a node is public unless [p2p] private is set")
}
