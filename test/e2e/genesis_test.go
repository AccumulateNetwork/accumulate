package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

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
