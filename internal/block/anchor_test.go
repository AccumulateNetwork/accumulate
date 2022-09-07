package block_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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

func TestValidatorSignature(t *testing.T) {
	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	dn := sim.Partition(sim.Partitions[0].Id)
	bvn0 := sim.Partition(sim.Partitions[1].Id)
	bvn1 := sim.Partition(sim.Partitions[2].Id)
	bvn2 := sim.Partition(sim.Partitions[3].Id)

	batch := dn.Begin(true)
	defer batch.Discard()

	globals := dn.Executor.ActiveGlobals()
	globals.Network.Version = 1
	signer := globals.AsSigner(dn.Partition.Id)
	sigset, err := batch.Transaction(make([]byte, 32)).SignaturesForSigner(signer)
	require.NoError(t, err)
	count, err := sigset.Add(0, &protocol.ED25519Signature{PublicKey: bvn0.Executor.Key[32:], Signer: signer.GetUrl(), SignerVersion: signer.GetVersion()})
	require.NoError(t, err)
	require.Equal(t, count, 1)
	count, err = sigset.Add(0, &protocol.ED25519Signature{PublicKey: bvn1.Executor.Key[32:], Signer: signer.GetUrl(), SignerVersion: signer.GetVersion()})
	require.NoError(t, err)
	require.Equal(t, count, 2)

	globals.Network.Version++
	signer = globals.AsSigner(dn.Partition.Id)
	sigset, err = batch.Transaction(make([]byte, 32)).SignaturesForSigner(signer)
	require.NoError(t, err)
	count, err = sigset.Add(0, &protocol.ED25519Signature{PublicKey: bvn1.Executor.Key[32:], Signer: signer.GetUrl(), SignerVersion: signer.GetVersion()})
	require.NoError(t, err)
	require.Equal(t, count, 2)
	count, err = sigset.Add(0, &protocol.ED25519Signature{PublicKey: bvn2.Executor.Key[32:], Signer: signer.GetUrl(), SignerVersion: signer.GetVersion()})
	require.NoError(t, err)
	require.Equal(t, count, 3)
}
