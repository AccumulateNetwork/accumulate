// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() {
	acctesting.EnableDebugFeatures()
	acctesting.SlogDebug("consensus")
}

func TestSimulator(t *testing.T) {
	cases := []struct {
		Bvns, Nodes int
	}{
		{1, 1},
		{1, 2},
		{1, 3},
		{2, 1},
		{2, 2},
		{2, 3},
		{3, 1},
		{3, 2},
		{3, 3},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("%dx%d", c.Bvns, c.Nodes), func(t *testing.T) {
			sim := NewSim(t,
				simulator.SimpleNetwork(t.Name(), c.Bvns, c.Nodes),
				simulator.Genesis(GenesisTime),
			)

			// Step a block and verify that only one block happened
			sim.Step()
			require.Equal(t, 2, int(sim.S.BlockIndex(protocol.Directory)))
			sim.Step()
			require.Equal(t, 3, int(sim.S.BlockIndex(protocol.Directory)))
		})
	}
}
