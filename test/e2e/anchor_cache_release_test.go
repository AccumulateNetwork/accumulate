// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// Anchors flow every block in both directions, and every anchor copy carries
// the sender's Delivered on the destination's anchor stream to it, so a
// partition's produced anchors are released as the destinations execute them:
// the anchor cache holds the anchors in flight, not the horizon's worth
// (healing spec, "The cache").
func TestAnchorCacheReleasesOnDelivered(t *testing.T) {
	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 1),
		simulator.GenesisWith(GenesisTime, globals),
	)
	const blocks = 120
	sim.StepN(blocks)
	for _, part := range []string{Directory, "BVN0", "BVN1"} {
		held := sim.S.Partition(part).SynthCache().AnchorLen()
		t.Logf("%s holds %d produced anchors after %d blocks", part, held, blocks)
		require.Less(t, held, blocks/4, "%s: the anchor cache should hold only the anchors in flight", part)
	}
}
