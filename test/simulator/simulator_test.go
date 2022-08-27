package simulator_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

func TestSimualtor(t *testing.T) {
	sim, err := simulator.New(
		acctesting.NewTestLogger(t),
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.MemoryDatabase,
	)
	require.NoError(t, err)
	require.NoError(t, sim.InitFromGenesis())
}
