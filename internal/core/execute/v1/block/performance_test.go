// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block_test

import (
	"bufio"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
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
	x := sim.Partition("BVN0")
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

	dataSetLog.SetProcessName("BVN0")

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

type executor struct {
	typ protocol.TransactionType
	fn  func(st *chain.StateManager, tx *chain.Delivery) error
}

var _ chain.SignerValidator = executor{}
var _ chain.PrincipalValidator = executor{}

func (x executor) Type() protocol.TransactionType { return x.typ }

func (executor) SignerIsAuthorized(delegate chain.AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, md chain.SignatureValidationMetadata) (fallback bool, err error) {
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
