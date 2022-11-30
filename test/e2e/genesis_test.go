// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

func TestActivationRoutes(t *testing.T) {
	g := new(core.GlobalValues)
	g.Routing = new(RoutingTable)
	g.Routing.Routes = []Route{
		{Partition: "A", Length: 2, Value: 0},  // 00
		{Partition: "B", Length: 2, Value: 1},  // 01
		{Partition: "C", Length: 2, Value: 2},  // 10
		{Partition: "A", Length: 3, Value: 6},  // 110
		{Partition: "B", Length: 4, Value: 14}, // 1110
		{Partition: "C", Length: 4, Value: 15}, // 1111
	}

	r, err := routing.NewRouteTree(g.Routing)
	require.NoError(t, err)

	total := 0
	count := map[string]int{}
	for i := 0; i < 1e6; i++ {
		s, err := r.RouteNr(rand.Uint64())
		require.NoError(t, err)
		total++
		count[s]++
	}

	// Values are within 1% of expectations
	require.Less(t, math.Abs(float64(count["A"])/float64(total)-0.3750), 0.01)
	require.Less(t, math.Abs(float64(count["B"])/float64(total)-0.3125), 0.01)
	require.Less(t, math.Abs(float64(count["C"])/float64(total)-0.3125), 0.01)
}

func TestGenesisRouting(t *testing.T) {
	g := new(core.GlobalValues)
	g.Routing = new(RoutingTable)
	g.Routing.AddOverride(AccountUrl("staking.acme"), Directory)
	g.Routing.Routes = []Route{{Partition: "BVN0"}}

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, g),
	)

	// Get the routing table
	account := GetAccount[*DataAccount](t, sim.Database(Directory), DnUrl().JoinPath(Routing))
	table := new(RoutingTable)
	require.NoError(t, table.UnmarshalBinary(account.Entry.GetData()[0]))

	// Verify
	overrideEquals(t, table, AccountUrl("staking.acme"), Directory)
	overrideEquals(t, table, AcmeUrl(), Directory)
	overrideEquals(t, table, DnUrl(), Directory)
	overrideEquals(t, table, PartitionUrl("BVN0"), "BVN0")
	require.Len(t, table.Routes, 1)
	require.True(t, table.Routes[0].Equal(&Route{Partition: "BVN0"}))
}

func overrideEquals(t *testing.T, rt *RoutingTable, account *url.URL, partition string) {
	t.Helper()
	for _, o := range rt.Overrides {
		if o.Account.Equal(account) {
			require.Equal(t, partition, o.Partition, "Expected an override routing %v to %s", account, partition)
			return
		}
	}
	require.Failf(t, "Override missing", "Expected an override routing %v to %s", account, partition)
}
