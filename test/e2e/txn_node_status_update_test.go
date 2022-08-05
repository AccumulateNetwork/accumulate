package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestNodeStatusUpdate_Up(t *testing.T) {
	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()
	dn := sim.Partition(protocol.Directory)

	// Execute
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(dn.Executor.Describe.AddressBook()).
			WithSigner(dn.Executor.Describe.OperatorsPage(), 1).
			WithTimestampVar(&timestamp).
			WithBody(&NodeStatusUpdate{
				Status:  NodeStatusUp,
				Address: protocol.MustParseInternetAddress("http://foo"),
			}).
			Initiate(SignatureTypeED25519, dn.Executor.Key).
			Build(),
	)...)

	// Verify
	require.Equal(t, "http://foo", dn.Executor.ActiveGlobals_TESTONLY().AddressBook.Entries[0].Address.String())
}
