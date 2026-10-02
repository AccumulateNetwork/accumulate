// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/dagbft"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestOnlyAKnownNonValidatorIsWatchedFromGenesis: the genesis watch (#4506)
// runs for a node that executes from genesis and is known not to be a
// validator of its partition -- and for no other. A joining node has the
// join; a validator signs the anchors the watch would read; a node whose
// committee is not known (the globals did not arrive) must not be mistaken
// for a follower, since a watched node is demoted on a mismatch.
func TestOnlyAKnownNonValidatorIsWatchedFromGenesis(t *testing.T) {
	val, _, _ := ed25519.GenerateKey(rand.Reader)
	fol, _, _ := ed25519.GenerateKey(rand.Reader)
	g := &core.GlobalValues{Network: &protocol.NetworkDefinition{Partitions: []*protocol.PartitionInfo{{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator}}}}
	g.Network.AddValidator(val, "BVN1", true)

	known := func(key ed25519.PublicKey) *dagbft.Membership {
		m := dagbft.NewMembership("BVN1", key)
		m.SetGlobals(g)
		return m
	}
	require.True(t, watchesFromGenesis(false, known(fol)), "a follower executing from genesis is watched")
	require.False(t, watchesFromGenesis(false, known(val)), "a validator is not")
	require.False(t, watchesFromGenesis(true, known(fol)), "a joining node has the join")
	require.False(t, watchesFromGenesis(false, dagbft.NewMembership("BVN1", fol)), "a node whose committee is not known is not watched")
}
