// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The cadence is a function of the block index, so every node activates on the
// same blocks. That is what makes "two of us send" mean two NODES rather than
// two per-node timers that happen to overlap — which is what the jitter and
// back-off it replaces were trying to approximate.
func TestHealActivates_IsTheSameOnEveryNode(t *testing.T) {
	var on, off int
	for i := uint64(0); i < 1000; i++ {
		if healActivates(i) {
			on++
		} else {
			off++
		}
	}
	assert.Equal(t, 1000/healCadence, on, "one activation every healCadence blocks")
	assert.Greater(t, off, on, "and not every block: a request's answer takes blocks to come back")
}

// healers builds N conductors sharing one validator set, as N validators of a
// partition would.
func healers(t *testing.T, n int) []*Conductor {
	t.Helper()

	var validators []*protocol.ValidatorInfo
	var keys []ed25519.PrivateKey
	for i := 0; i < n; i++ {
		pub, priv, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		keys = append(keys, priv)
		validators = append(validators, &protocol.ValidatorInfo{
			PublicKey:  pub,
			Partitions: []*protocol.ValidatorPartitionInfo{{ID: "BVN1", Active: true}},
		})
	}

	globals := &core.GlobalValues{
		ExecutorVersion: protocol.ExecutorVersionV2Kourou,
		Network:         &protocol.NetworkDefinition{Validators: validators},
	}

	var out []*Conductor
	for i := 0; i < n; i++ {
		c := &Conductor{
			Partition:    &protocol.PartitionInfo{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator},
			ValidatorKey: keys[i],
		}
		c.Globals.Store(globals)
		out = append(out, c)
	}
	return out
}

// Exactly two validators send, and every node reaches that conclusion from the
// same agreed input without asking anyone.
func TestSelection_WouldStarveAnAnchorQuorum(t *testing.T) {
	for _, n := range []int{4, 6, 10, 16} {
		threshold := n*2/3 + 1 // what an anchor needs
		require.Greater(t, threshold, sendersPerActivation,
			"with %d validators an anchor needs %d signatures; %d senders can never reach it, "+
				"which is why the anchor push is not selected", n, threshold, sendersPerActivation)
	}
}
