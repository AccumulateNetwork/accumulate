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
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestAJoiningNodeAdvertisesItsServicesOnlyOnceActive — DIFFERENCES E30
// (Paul, 2026-10-05): a joining node's query and sequencer services refuse
// every read, so advertising them makes the node a source its peers ask and
// are refused by -- two followers joining together each waited on the
// other. The services are registered with the network when the node first
// becomes ACTIVE for the partition, once; a node that never joined registers
// at once.
func TestAJoiningNodeAdvertisesItsServicesOnlyOnceActive(t *testing.T) {
	registered := 0
	whenServing(nodestate.Always{}, func() { registered++ })
	require.Equal(t, 1, registered, "a node that never joined did not register at once")

	registered = 0
	m := nodestate.New(protocol.PartitionUrl("BVN1"))
	whenServing(m, func() { registered++ })
	require.Zero(t, registered, "a joining node registered before it was ACTIVE")
	require.True(t, m.PromoteToActive([32]byte{1}, 10))
	require.Equal(t, 1, registered, "the node did not register when it became ACTIVE")
	require.True(t, m.Demote(20))
	require.True(t, m.PromoteToActive([32]byte{2}, 30))
	require.Equal(t, 1, registered, "the node registered again on a later promotion")

	// Already ACTIVE when the service starts: registered at once.
	registered = 0
	whenServing(m, func() { registered++ })
	require.Equal(t, 1, registered)
}

// A validator's services are advertised whatever its state (#4543): its
// peers find the partition by them, and after a whole-network restart no
// node is ACTIVE until it has found its peers.
func TestAValidatorAdvertisesItsServicesAtOnce(t *testing.T) {
	registered := 0
	m := nodestate.New(protocol.PartitionUrl("BVN1"))
	m.AdvertiseAtOnce()
	whenServing(m, func() { registered++ })
	require.Equal(t, 1, registered, "a BOOTING validator was not advertised")
	require.False(t, m.CanServeCurrent(), "advertising changed what the node answers")
	require.True(t, m.PromoteToActive([32]byte{1}, 10))
	require.Equal(t, 1, registered, "advertised twice")
}

// A validator advertises its sequencer at once; a node in no committee once
// it is ACTIVE. Peers find their partition by the sequencer advertisement,
// and after a whole-network restart no node is ACTIVE until it has found
// them: the first whole restart after E30 found no validator on any node
// for as long as it ran (#4543).
func TestAValidatorIsKnownByItsKey(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	other, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	committee := types.NewCommittee([]types.ValidatorInfo{{PublicKey: pub, Stake: 1}}, 1)

	require.True(t, inCommittee(committee, priv))
	otherPriv := make([]byte, ed25519.PrivateKeySize)
	copy(otherPriv[32:], other)
	require.False(t, inCommittee(committee, otherPriv), "a node in no committee")
	require.False(t, inCommittee(nil, priv))
	require.False(t, inCommittee(committee, priv[:16]), "a key that is no key")
}
