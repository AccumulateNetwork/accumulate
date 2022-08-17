package block_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestAnchor(t *testing.T) {
	// Initialize
	sim := simulator.New(t, 1)
	sim.InitFromGenesis()

	var statuses []*TransactionStatus
	for i := 0; i < 50 && len(statuses) == 0; i++ {
		ch := make(chan *TransactionStatus)
		go sim.ExecuteBlock(ch)
		for status := range ch {
			statuses = append(statuses, status)
		}
	}

	require.NotEmpty(t, statuses, "Did not get any transactions after 50 blocks")

	var gotAnchor bool
	for _, status := range statuses {
		hash := status.TxID.Hash()
		_ = sim.PartitionFor(status.TxID.Account()).View(func(batch *database.Batch) error {
			state, err := batch.Transaction(hash[:]).Main().Get()
			require.NoError(t, err)
			require.NotNil(t, state.Transaction)

			switch state.Transaction.Body.Type() {
			case TransactionTypeDirectoryAnchor,
				TransactionTypeBlockValidatorAnchor:
				gotAnchor = true
			default:
				t.Fatalf("Unexpected non-anchor %v", state.Transaction.Body.Type())
			}
			return nil
		})
	}
	require.True(t, gotAnchor, "Expected anchors")
}
