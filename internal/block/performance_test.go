package block_test

import (
	"bufio"
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
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
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
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

	batch := sim.SubnetFor(aliceUrl).Database.Begin(true)
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
		batch := sim.SubnetFor(aliceUrl).Database.Begin(true)
		defer batch.Discard()
		_, err := sim.SubnetFor(aliceUrl).Executor.ProcessSignature(batch, delivery, delivery.Signatures[0])
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
	x := sim.Subnet(sim.Subnets[1].Id)
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

	dataSetLog.SetProcessName(x.Subnet.Id)

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

func BenchmarkSendSynthTxnAfterAnchor(b *testing.B) {
	// Tests AC-1860
	var timestamp uint64

	// Initialize
	sim := simulator.New(b, 3)
	sim.InitFromGenesis()

	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	bob := acctesting.GenerateKey("Bob")
	bobUrl := acctesting.AcmeLiteAddressStdPriv(bob)

	sim.CreateAccount(&protocol.LiteIdentity{Url: aliceUrl.RootIdentity(), CreditBalance: 1e9})
	sim.CreateAccount(&protocol.LiteTokenAccount{Url: aliceUrl, TokenUrl: protocol.AcmeUrl(), Balance: *big.NewInt(1e9)})

	// Capture the first deposit
	var deposit *chain.Delivery
	sim.SubnetFor(bobUrl.RootIdentity()).SubmitHook = func(envelopes []*chain.Delivery) ([]*chain.Delivery, bool) {
		for i, env := range envelopes {
			if env.Transaction.Body.Type() == protocol.TransactionTypeSyntheticDepositTokens {
				fmt.Printf("Dropping %X\n", env.Transaction.GetHash()[:4])
				deposit = env
				return append(envelopes[:i], envelopes[i+1:]...), false
			}
		}
		return envelopes, true
	}

	// Execute
	envs := sim.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(aliceUrl).
			WithTimestampVar(&timestamp).
			WithSigner(aliceUrl, 1).
			WithBody(&protocol.SendTokens{
				To: []*protocol.TokenRecipient{{
					Url:    bobUrl,
					Amount: *big.NewInt(1),
				}},
			}).
			Initiate(protocol.SignatureTypeED25519, alice).
			Build())
	sim.WaitForTransaction(delivered, envs[0].Transaction[0].GetHash(), 50)

	// Wait for the synthetic transaction to be sent and the block to be
	// anchored
	sim.ExecuteBlocks(10)
	require.NotNil(b, deposit, "synthetic transactions have not been sent")

	// Verify the block has been anchored
	var receipt *protocol.ReceiptSignature
	for _, sig := range deposit.Signatures {
		if sig, ok := sig.(*protocol.ReceiptSignature); ok {
			receipt = sig
		}
	}
	require.NotNil(b, receipt)
	req := new(query.RequestByUrl)
	req.Url = protocol.DnUrl().JoinPath(protocol.AnchorPool).WithFragment(fmt.Sprintf("anchor/%x", receipt.Proof.Anchor))
	sim.Query(protocol.DnUrl(), req, true)

	// Submit the synthetic transaction
	sim.SubnetFor(bobUrl).Submit(&protocol.Envelope{
		Transaction: []*protocol.Transaction{deposit.Transaction},
		Signatures:  deposit.Signatures,
	})
	sim.WaitForTransactionFlow(delivered, deposit.Transaction.GetHash())
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
