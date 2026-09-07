// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/fastsync"
	apiv3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// TestProofServiceMatchesPrivate is the point of AIP-59: what the public proof
// service returns must be what the private ranger returns, byte for byte, and
// a client must be able to walk it by induction. If the public surface returned
// anything of its own, a verifier using it would not be running the induction
// the network runs on itself.
func TestProofServiceMatchesPrivate(t *testing.T) {
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.MajorBlockSchedule = "* * * * *"
	g.Globals.OperatorAcceptThreshold.Set(1, 3)
	g.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.GenesisWith(GenesisTime, g),
	)

	genesis := loadDirectoryGlobals(t, sim)
	sim.StepUntilN(200, MajorBlock(1))
	sim.StepUntilN(200, MajorBlock(2))
	sim.StepN(10)

	ranger, ok := sim.S.Services().Private().(private.MajorHeaderRanger)
	require.True(t, ok)

	// The private path, as fast sync uses it
	viaPrivate, err := ranger.MajorHeaderRange(context.Background(), DnUrl(), 1, 2, private.SequenceOptions{})
	require.NoError(t, err)
	require.Len(t, viaPrivate, 2)

	// The public path: identical records, reached through the service type an
	// external verifier would use
	// The public path: the same records, reached through the service an external
	// verifier would use. It wraps the very ranger fast sync uses.
	svc := &apiimpl.ProofService{Ranger: sim.S.Services().Private().(interface {
		private.MajorHeaderRanger
		private.MinorRootRanger
	})}
	viaPublic, err := svc.MajorHeaderRange(context.Background(),
		apiv3.MajorHeaderRangeOptions{Partition: Directory, Start: 1, End: 2})
	require.NoError(t, err)
	require.Len(t, viaPublic, 2)

	for i := range viaPrivate {
		a, err := viaPrivate[i].MarshalBinary()
		require.NoError(t, err)
		b, err := viaPublic[i].MarshalBinary()
		require.NoError(t, err)
		require.Equalf(t, a, b, "record %d differs between the private and public paths", i)
	}

	// And it is walkable: the whole point is that a verifier derives the set
	spine, err := fastsync.NewSpine(genesis, 1)
	require.NoError(t, err)
	for _, r := range viaPublic {
		require.NoError(t, spine.Advance(r), "advance past major block %d", r.Index)
	}
	require.Equal(t, uint64(3), spine.NextMajor)

	// A partition that does not serve the spine is refused, not answered
	_, err = svc.MajorHeaderRange(context.Background(),
		apiv3.MajorHeaderRangeOptions{Partition: "BVN0", Start: 1, End: 2})
	require.Error(t, err, "a BVN must refuse to serve major headers")

	// A missing partition is refused before it reaches the sequencer
	_, err = svc.MajorHeaderRange(context.Background(),
		apiv3.MajorHeaderRangeOptions{Start: 1, End: 2})
	require.Error(t, err, "a missing partition must be refused")
}
