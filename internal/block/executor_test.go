package block_test

import (
	"fmt"
<<<<<<< HEAD
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
=======
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block"
>>>>>>> 9bb6159ba9272d413c17600d2edb79876db72043
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

<<<<<<< HEAD
func prepareBenchmarking(t *testing.B) (simu *simulator.Simulator, envp *protocol.Envelope) {
	// Initialize
	sim := simulator.New(t, 3)
	sim.InitChain()

	// Create a lite address
	alice := acctesting.GenerateTmKey(t.Name(), "Alice")
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)

	// Fund the lite account
	faucet := protocol.Faucet.Signer()
	env := acctesting.NewTransaction().
		WithPrincipal(protocol.FaucetUrl).
		WithTimestamp(faucet.Timestamp()).
		WithBody(&protocol.AcmeFaucet{Url: aliceUrl}).
		Faucet()

	return sim, env
}

func HighTps(sim *simulator.Simulator, env *protocol.Envelope, n int) {
	for i := 0; i < n; i++ {
		sim.ExecuteBlock(env)
		//		sim.WaitForGovernor()
		//		sim.WaitForTransaction(env.GetTxHash())
	}
}

func HighTps2(sim *simulator.Simulator, env *protocol.Envelope) {
	sim.ExecuteBlock(env)
}

func BenchmarkHighTps(b *testing.B) {

	sim, env := prepareBenchmarking(b)
=======
func prepareBenchmark(b *testing.B) (newExec *block.Executor, newBlock *block.Block, newEnv *protocol.Envelope) {
	// Initialize the environment
	db, exec := SetupExecSingle(b)
	InitChain(b, db, exec)

	// Start a block
	block := new(block.Block)
	block.IsLeader = true
	block.Index = protocol.GenesisBlock + 1
	block.Time = time.Now()
	block.Batch = db.Begin(true)
	_, err := exec.BeginBlock(block)
	require.NoError(b, err)

	// Setup test data
	liteKey := acctesting.GenerateKey(b.Name(), "Lite")
	liteAddr, err := protocol.LiteTokenAddress(liteKey[32:], protocol.ACME)
	require.NoError(b, err)

	deposit := new(protocol.SyntheticDepositTokens)
	deposit.Source = exec.Network.NodeUrl()
	deposit.Token = protocol.AcmeUrl()
	deposit.Amount = *big.NewInt(1)
	deposit.Cause[0] = 1

	env := acctesting.NewTransaction().
		WithPrincipal(liteAddr).
		WithSigner(exec.Network.ValidatorPage(0), 1).
		WithCurrentTimestamp().
		WithBody(deposit).
		Initiate(protocol.SignatureTypeED25519, exec.Key)

	return exec, block, env
}

func HighTps(b *testing.B, exec *block.Executor, block *block.Block, env *protocol.Envelope) {
	DeliverTx(b, exec, block, env.Copy())
}

func BenchmarkHighTps(b *testing.B) {
	exec, block, env := prepareBenchmark(b)
	defer block.Batch.Discard()

>>>>>>> 9bb6159ba9272d413c17600d2edb79876db72043
	// Execute the benchmark
	for _, size := range []int{1, 10, 100, 1000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			// Preload the block with N transactions
<<<<<<< HEAD
			for i := 0; i < b.N; i++ {
				HighTps(sim, env, size)
			}
		})
	}
}

func BenchmarkHighTps2(b *testing.B) {

	sim, env := prepareBenchmarking(b)
	for i := 0; i < b.N; i++ {
		HighTps2(sim, env)
=======
			for i := 0; i < size; i++ {
				HighTps(b, exec, block, env)
			}

			// Benchmark how long DeliverTx takes
			b.ResetTimer()
			b.Run("DeliverTx", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					HighTps(b, exec, block, env)
				}
			})
		})
>>>>>>> 9bb6159ba9272d413c17600d2edb79876db72043
	}
}
