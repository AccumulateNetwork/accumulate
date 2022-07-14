package e2e

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/genesis"
	"gitlab.com/accumulatenetwork/accumulate/internal/testdata"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestFactomAddresses(t *testing.T) {
	// Initialize
	sim := simulator.NewWith(t, simulator.SimulatorOptions{
		BvnCount:        3,
		FactomAddresses: func() (io.Reader, error) { return strings.NewReader(testdata.FactomAddresses), nil },
	})
	sim.InitFromGenesis()

	// Verify
	factomAddresses, err := genesis.LoadFactomAddressesAndBalances(strings.NewReader(testdata.FactomAddresses))
	require.NoError(t, err)

	for _, addr := range factomAddresses {
		account := simulator.GetAccount[*protocol.LiteTokenAccount](sim, addr.Address)
		assert.Equalf(t, int(5*addr.Balance), int(account.Balance.Int64()), "Incorrect balance for %v", addr.Address)
	}
}
