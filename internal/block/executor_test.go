package block_test

import (
	"fmt"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

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
	// Execute the benchmark
	for _, size := range []int{1, 10, 100, 1000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			// Preload the block with N transactions
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
	}
}
