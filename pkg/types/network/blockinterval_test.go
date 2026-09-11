// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package network_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4267: a network runs at one cadence and the network has to say what it is.
// The value is recorded in globals when the network is deployed, so it can be
// read back from the network rather than inferred from a node's binary.

func TestBlockInterval_IsRecordedWhetherOrNotItIsStated(t *testing.T) {
	// Not stated: a default is fine, an unrecorded one is not
	g := network.NewGlobals(nil)
	require.Equal(t, protocol.DefaultBlockInterval, g.Globals.BlockInterval,
		"a deployed network must carry an interval even when the operator states none")
	require.NotZero(t, g.Globals.BlockInterval)

	// Stated: the operator's value is kept, not overwritten by the default
	stated := &network.GlobalValues{Globals: &protocol.NetworkGlobals{BlockInterval: 5 * time.Second}}
	g = network.NewGlobals(stated)
	require.Equal(t, 5*time.Second, g.Globals.BlockInterval)
}

// The field is appended, so a network deployed before it reads as absent
// rather than as a corrupt record -- and once written it round-trips.
func TestBlockInterval_Encoding(t *testing.T) {
	before := &protocol.NetworkGlobals{}
	before.OperatorAcceptThreshold.Set(2, 3)
	before.ValidatorAcceptThreshold.Set(2, 3)
	before.MajorBlockSchedule = protocol.DefaultMajorBlockSchedule
	data, err := before.MarshalBinary()
	require.NoError(t, err)

	// A reader that knows the field decodes old data with it absent
	var old protocol.NetworkGlobals
	require.NoError(t, old.UnmarshalBinary(data))
	require.Zero(t, old.BlockInterval, "a network deployed before the field declares none")

	// And a stated value survives the round trip
	before.BlockInterval = 1500 * time.Millisecond
	data, err = before.MarshalBinary()
	require.NoError(t, err)
	var got protocol.NetworkGlobals
	require.NoError(t, got.UnmarshalBinary(data))
	require.Equal(t, 1500*time.Millisecond, got.BlockInterval)
	require.True(t, before.Equal(&got))
}

// Zero means "the operator said nothing", and a default is recorded. A
// negative value is a wrong statement rather than an absent one, so it
// survives to be rejected at deployment instead of being quietly replaced.
func TestBlockInterval_NegativeIsNotSilentlyDefaulted(t *testing.T) {
	g := network.NewGlobals(&network.GlobalValues{
		Globals: &protocol.NetworkGlobals{BlockInterval: -5 * time.Second},
	})
	require.Equal(t, -5*time.Second, g.Globals.BlockInterval,
		"a negative interval must reach the deployment check, not be replaced by the default")
}
