// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus_test

import (
	"fmt"
	"testing"

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

			sim.StepN(2)
		})
	}
}
