// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	dagconfig "gitlab.com/accumulatenetwork/accumulate/pkg/consensus/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func withInterval(d time.Duration) *network.GlobalValues {
	return &network.GlobalValues{Globals: &protocol.NetworkGlobals{BlockInterval: d}}
}

func stated(d time.Duration) *encoding.Duration {
	v := encoding.Duration(d)
	return &v
}

// #4267: the network declares the cadence, and a node paces from it or does
// not run.
func TestResolveBlockInterval(t *testing.T) {
	t.Run("the network value is what the node paces at", func(t *testing.T) {
		got, err := resolveBlockInterval(nil, withInterval(2*time.Second), "BVN1")
		require.NoError(t, err)
		require.Equal(t, 2*time.Second, got)
	})

	t.Run("agreeing with the network is not a divergence", func(t *testing.T) {
		got, err := resolveBlockInterval(stated(2*time.Second), withInterval(2*time.Second), "BVN1")
		require.NoError(t, err)
		require.Equal(t, 2*time.Second, got)
	})

	t.Run("disagreeing with the network refuses to start", func(t *testing.T) {
		_, err := resolveBlockInterval(stated(time.Second), withInterval(3*time.Second), "BVN1")
		require.Error(t, err)
		// Both values are named, so the operator can see which is which
		require.Contains(t, err.Error(), "1s")
		require.Contains(t, err.Error(), "3s")
	})

	t.Run("the node value is not silently adopted over the network's", func(t *testing.T) {
		got, err := resolveBlockInterval(stated(5*time.Second), withInterval(1*time.Second), "BVN1")
		require.Error(t, err, "a divergence must be refused, not resolved in either direction")
		require.Zero(t, got)
	})

	// A network deployed before block time was recorded declares nothing.
	// Those keep running on local configuration rather than being locked out.
	t.Run("a network that declares nothing falls back to local config", func(t *testing.T) {
		got, err := resolveBlockInterval(stated(4*time.Second), withInterval(0), "BVN1")
		require.NoError(t, err)
		require.Equal(t, 4*time.Second, got)
	})

	t.Run("declaring nothing with nothing configured uses the default", func(t *testing.T) {
		got, err := resolveBlockInterval(nil, withInterval(0), "BVN1")
		require.NoError(t, err)
		require.Equal(t, dagconfig.DefaultBlockInterval, got)
	})

	// Startup hands us an empty GlobalValues when the initial event times out
	t.Run("missing globals do not panic", func(t *testing.T) {
		for _, g := range []*network.GlobalValues{nil, new(network.GlobalValues)} {
			got, err := resolveBlockInterval(nil, g, "BVN1")
			require.NoError(t, err)
			require.Equal(t, dagconfig.DefaultBlockInterval, got)
		}
	})
}

// Rounds pace at half the block interval, because Bullshark commits a leader
// every other round. That relationship is what #4098 was about.
func TestBlockIntervalDrivesRoundPacing(t *testing.T) {
	for _, d := range []time.Duration{time.Second, 2 * time.Second, 3 * time.Second} {
		got, err := resolveBlockInterval(nil, withInterval(d), "BVN1")
		require.NoError(t, err)
		require.Equal(t, d/2, got/2, "a %v block is a %v round", d, d/2)
	}
}

// The recorded default and the pacing default are the same number in two
// packages, because the layering does not let one reference the other. If they
// drift, every node that states no interval is re-paced by the mere act of
// recording one at deployment.
func TestTheRecordedDefaultIsThePacingDefault(t *testing.T) {
	require.Equal(t, dagconfig.DefaultBlockInterval, protocol.DefaultBlockInterval,
		"protocol.DefaultBlockInterval and dagconfig.DefaultBlockInterval must match")
}

// A network must be deployable at a stated cadence: the value reaches the
// genesis globals, which is what every node then paces from. Pinning it per
// node instead would leave no network value to check a node against (#4267).
func TestNetsimRecordsTheStatedInterval(t *testing.T) {
	t.Run("stated", func(t *testing.T) {
		d := &NetSimConfiguration{Bvns: 1, Validators: 1, BlockInterval: Ptr(encoding.Duration(7 * time.Second))}
		g := netsimGlobals(t, d)
		require.Equal(t, 7*time.Second, g.Globals.BlockInterval)
	})

	t.Run("not stated leaves it for NewGlobals to record", func(t *testing.T) {
		d := &NetSimConfiguration{Bvns: 1, Validators: 1}
		g := netsimGlobals(t, d)
		require.Zero(t, g.Globals.BlockInterval,
			"unset here; genesis records the protocol default")
	})
}

// netsimGlobals runs the part of buildGenesis that fills in the globals.
func netsimGlobals(t *testing.T, d *NetSimConfiguration) *network.GlobalValues {
	t.Helper()
	v := setDefaultVal(&d.Globals, new(network.GlobalValues))
	g := setDefaultVal(&v.Globals, new(protocol.NetworkGlobals))
	if d.BlockInterval != nil {
		g.BlockInterval = time.Duration(*d.BlockInterval)
	}
	return v
}
