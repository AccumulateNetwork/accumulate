package block_test

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func BenchmarkPerformance(b *testing.B) {

	// Initialize
	sim := simulator.New(b, 3)
	sim.InitFromGenesis()

	alice := acctesting.GenerateKey("Alice")
	bob := acctesting.GenerateKey("Alice")
	charlie := acctesting.GenerateKey("Alice")

	// Create the keys and URLs
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	bobUrl := acctesting.AcmeLiteAddressStdPriv(bob)
	charlieUrl := acctesting.AcmeLiteAddressStdPriv(charlie)

	batch := sim.PartitionFor(aliceUrl).Database.Begin(true)
	require.NoError(b, acctesting.CreateLiteTokenAccountWithCredits(batch, ed25519.PrivKey(alice), protocol.AcmeFaucetAmount, 1e9))
	require.NoError(b, batch.Commit())

	// Create the transaction
	delivery := acctesting.NewTransaction().
		WithPrincipal(aliceUrl).
		WithSigner(aliceUrl, 1).
		WithCurrentTimestamp().
		WithBody(&protocol.SendTokens{
			To: []*protocol.TokenRecipient{
				{Url: bobUrl, Amount: *big.NewInt(1000)},
				{Url: charlieUrl, Amount: *big.NewInt(2000)},
			},
		}).Initiate(protocol.SignatureTypeED25519, alice).BuildDelivery()

	for i := 0; i < b.N; i++ {
		batch := sim.PartitionFor(aliceUrl).Database.Begin(true)
		defer batch.Discard()
		_, err := sim.PartitionFor(aliceUrl).Executor.ProcessSignature(batch, delivery, delivery.Signatures[0])
		require.NoError(b, err)
	}
}

func BenchmarkBlockTimes(b *testing.B) {
	// Initialize the siulator, genesis
	sim := simulator.New(b, 1)
	sim.InitFromGenesis()

	// Create a lite address
	alice := acctesting.GenerateTmKey(b.Name(), "Alice")
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)

	// Start a block
	x := sim.Partition(sim.Partitions[1].Id)
	x.Executor.EnableTimers()
	block := new(block.Block)
	block.IsLeader = true
	block.Index = 3
	block.Time = time.Now()
	block.Batch = x.Database.Begin(true)
	defer block.Batch.Discard()
	require.NoError(b, x.Executor.BeginBlock(block))

	// Pre-populate the block with 500 transactions
	for i := 0; i < 500; i++ {
		env, err := chain.NormalizeEnvelope(acctesting.NewTransaction().
			WithPrincipal(protocol.FaucetUrl).
			WithBody(&protocol.AcmeFaucet{Url: aliceUrl}).
			Faucet())
		require.NoError(b, err)
		_, err = env[0].LoadTransaction(block.Batch)
		require.NoError(b, err)
		_, err = x.Executor.ExecuteEnvelope(block, env[0])
		require.NoError(b, err)
	}
	// Construct a new transaction
	env, err := chain.NormalizeEnvelope(acctesting.NewTransaction().
		WithPrincipal(protocol.FaucetUrl).
		WithBody(&protocol.AcmeFaucet{Url: aliceUrl}).
		Faucet())
	require.NoError(b, err)
	_, err = env[0].LoadTransaction(block.Batch)
	require.NoError(b, err)

	dataSetLog := new(logging.DataSetLog)

	dataSetLog.SetProcessName(x.Partition.Id)

	analysisDir := config.MakeAbsolute(b.TempDir(), "analysis")
	defer os.RemoveAll(analysisDir)
	dataSetLog.SetPath(analysisDir)

	_ = os.MkdirAll(analysisDir, 0700)

	ymd, hm := logging.GetCurrentDateTime()
	dataSetLog.SetFileTag(ymd, hm)

	dataSetLog.Initialize("executor", logging.DefaultOptions())
	ds := dataSetLog.GetDataSet("executor")

	tick := time.Now()
	// Benchmark ExecuteEnvelope
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block2 := *block // Copy the block
		block2.Batch = block.Batch.Begin(true)
		_, err = x.Executor.ExecuteEnvelope(&block2, env[0])
		block2.Batch.Discard()
		require.NoError(b, err)
		if ds != nil {
			ds.Save("height", i, 10, true)
			ds.Save("time_since_start", time.Since(tick).Seconds(), 6, false)
			x.Executor.BlockTimers.Store(ds)
		}
	}
	b.StopTimer()

	dumpLogs(b, dataSetLog)
}

func dumpLogs(b logging.TB, dataSetLog *logging.DataSetLog) {
	b.Helper()
	files, err := dataSetLog.DumpDataSetToDiskFile()
	require.NoError(b, err)

	//dump results
	for _, file := range files {
		f, err := os.Open(file)
		require.NoError(b, err)
		defer f.Close()
		scanner := bufio.NewScanner(f)
		b.Log(file)

		for scanner.Scan() {
			b.Log(scanner.Text())
		}

		b.Log("\n")
		if err := scanner.Err(); err != nil {
			require.NoError(b, err)
		}
	}
}

func BenchmarkBlock(b *testing.B) {
	// Disable debug features for the duration
	acctesting.DisableDebugFeatures()
	defer acctesting.EnableDebugFeatures()

	// bvnCount := []int{1, 2, 4, 8}
	bvnCount := []int{1}
	blockSize := []int{50, 100, 200, 500, 1000}
	// blockSize := []int{50}
	scenarios := map[string][]executor{
		"no-op": {
			{protocol.TransactionTypeAddCredits, func(st *chain.StateManager, tx *chain.Delivery) error {
				return nil
			}},
		},
		"create account": {
			{protocol.TransactionTypeAddCredits, func(st *chain.StateManager, tx *chain.Delivery) error {
				u := &url.URL{Authority: hex.EncodeToString(tx.Transaction.GetHash())}
				return st.Create(&protocol.UnknownAccount{Url: u})
			}},
		},
		"synth txn": {
			{protocol.TransactionTypeAddCredits, func(st *chain.StateManager, tx *chain.Delivery) error {
				u := &url.URL{Authority: hex.EncodeToString(tx.Transaction.GetHash())}
				st.Submit(u, &protocol.SyntheticDepositCredits{})
				return nil
			}},
			{protocol.TransactionTypeSyntheticDepositCredits, func(st *chain.StateManager, tx *chain.Delivery) error {
				return nil
			}},
		},
	}

	alice := protocol.AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	for scname, scenario := range scenarios {
		for _, bvnCount := range bvnCount {
			sim := simulator.New(b, bvnCount)
			sim.InitFromGenesis()
			sim.CreateIdentity(alice, aliceKey[32:])

			for _, x := range sim.Executors {
				for _, exec := range scenario {
					x.Executor.SetExecutor(exec)
				}
			}

			for _, blockSize := range blockSize {
				var timestamp uint64
				envs := make([]*protocol.Envelope, blockSize)
				for j := range envs {
					envs[j] = acctesting.NewTransaction().
						WithPrincipal(alice).
						WithSigner(alice.JoinPath("book", "1"), 1).
						WithTimestampVar(&timestamp).
						WithBody(&protocol.AddCredits{}).
						Initiate(protocol.SignatureTypeED25519, aliceKey).
						Build()
				}

				b.Run(fmt.Sprintf("%d BVNs, %d txns, %s", bvnCount, blockSize, scname), func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						sim.RunAndReset(func() {
							sim.MustSubmitAndExecuteBlock(envs...)
							sim.WaitForTransactions(delivered, envs...)
						})
					}
				})
			}
		}
	}

}

type executor struct {
	typ protocol.TransactionType
	fn  func(st *chain.StateManager, tx *chain.Delivery) error
}

func (x executor) Type() protocol.TransactionType { return x.typ }

func (executor) SignerIsAuthorized(delegate chain.AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, checkAuthz bool) (fallback bool, err error) {
	return false, nil // All signers are authorized
}

func (executor) TransactionIsReady(delegate chain.AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (ready, fallback bool, err error) {
	return true, false, nil // Transaction is always ready
}

func (executor) AllowMissingPrincipal(transaction *protocol.Transaction) bool {
	return true // Principal can be missing
}

func (x executor) Execute(st *chain.StateManager, tx *chain.Delivery) (protocol.TransactionResult, error) {
	return nil, x.fn(st, tx)
}

func (x executor) Validate(st *chain.StateManager, tx *chain.Delivery) (protocol.TransactionResult, error) {
	return nil, x.fn(st, tx)
}
