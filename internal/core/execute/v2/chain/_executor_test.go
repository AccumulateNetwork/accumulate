// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain_test

import (
	"fmt"
	"math/big"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	tmtypes "github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/memory"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func (e *Executor) ForceCommit() ([]byte, error) {
	return e.commit(new(Block), true)
}

func BenchmarkHighTps(b *testing.B) {
	store := memory.New(nil)
	db := database.New(store, nil)

	network := config.Network{
		Type:             config.BlockValidator,
		LocalPartitionID: b.Name(),
	}

	nodeKey := acctesting.GenerateKey(b.Name(), "Node")
	logger := logging.NewTestLogger(b, "plain", acctesting.DefaultLogLevels, false)
	exec, err := chain.NewNodeExecutor(chain.ExecutorOptions{
		DB:      db,
		Logger:  logger,
		Key:     nodeKey,
		Router:  acctesting.NullRouter{},
		Network: network,
	})
	require.NoError(b, err)
	require.NoError(b, exec.Start())

	kv := memory.New(nil)
	bootstrap, err := genesis.Init(kv, genesis.InitOpts{
		Network:     network,
		GenesisTime: time.Now(),
		Logger:      logger,
		Validators: []tmtypes.GenesisValidator{
			{PubKey: tmed25519.PubKey(nodeKey[32:])},
		},
	})
	require.NoError(b, err)
	err := bootstrap.Bootstrap()
	require.NoError(b, err)

	state, err := kv.MarshalJSON()
	require.NoError(b, err)
	_, err = exec.InitChain(state, time.Now())
	require.NoError(b, err)

	liteKey := acctesting.GenerateKey(b.Name(), "Lite")
	liteAddr, err := protocol.LiteTokenAddress(liteKey[32:], protocol.ACME, protocol.SignatureTypeED25519)
	require.NoError(b, err)

	deposit := new(protocol.SyntheticDepositTokens)
	deposit.Source = network.NodeUrl()
	deposit.Token = protocol.AcmeUrl()
	deposit.Amount = *big.NewInt(1)
	deposit.Cause[0] = 1

	blockBatchField, ok := reflect.TypeOf(chain.Executor{}).FieldByName("blockBatch")
	require.True(b, ok)
	batchStoreField, ok := reflect.TypeOf(database.Batch{}).FieldByName("store")
	require.True(b, ok)

	// HORRIBLE HACK WARNING - Do some pointer hacking to get at the database batch
	blockBatchPtr := (**database.Batch)(unsafe.Pointer(uintptr(unsafe.Pointer(exec)) + blockBatchField.Offset))

	for _, size := range []int{1, 10, 100, 1000} {
		b.ResetTimer()
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			_, err = exec.BeginBlock(BeginBlockRequest{
				IsLeader: true,
				Height:   5,
				Time:     time.Now(),
			})
			require.NoError(b, err)

			// HORRIBLE HACK WARNING - Do some pointer hacking to get at the key-value store batch
			blockBatch := *blockBatchPtr
			batchStorePtr := (*storage.KeyValueTxn)(unsafe.Pointer(uintptr(unsafe.Pointer(blockBatch)) + batchStoreField.Offset))
			batch := (*batchStorePtr).(*memory.Batch)

			for i := 0; i < size; i++ {
				env := acctesting.NewTransaction().
					WithPrincipal(liteAddr).
					WithSigner(network.NodeUrl(), 1).
					WithCurrentTimestamp().
					WithBody(deposit).
					Initiate(protocol.SignatureTypeED25519, nodeKey)

				_, err := exec.DeliverTx(env)
				if err != nil {
					require.NoError(b, err)
				}
			}

			b.ResetTimer()
			b.Run("DeliverTx", func(b *testing.B) {
				env := acctesting.NewTransaction().
					WithPrincipal(liteAddr).
					WithSigner(network.NodeUrl(), 1).
					WithCurrentTimestamp().
					WithBody(deposit).
					Initiate(protocol.SignatureTypeED25519, nodeKey)

				for i := 0; i < b.N; i++ {
					*batchStorePtr = batch.Copy()

					_, err := exec.DeliverTx(env)
					if err != nil {
						require.NoError(b, err)
					}
				}
			})
		})
	}
}

func TestSyntheticTransactionsAreAlwaysRecorded(t *testing.T) {
	t.Skip("TODO Needs a receipt signature")

	exec := setupWithGenesis(t)

	// Start a block
	_, err := exec.BeginBlock(BeginBlockRequest{
		IsLeader: true,
		Height:   2,
		Time:     time.Now(),
	})
	require.NoError(t, err)

	// Create a synthetic transaction where the origin does not exist
	env := acctesting.NewTransaction().
		WithPrincipal(url.MustParse("acc://account-that-does-not-exist")).
		WithSigner(exec.DefaultOperatorPage(), 1).
		WithCurrentTimestamp().
		WithBody(&protocol.SyntheticDepositCredits{
			SyntheticOrigin: protocol.SyntheticOrigin{Cause: [32]byte{1}},
			Amount:          1,
		}).
		InitiateSynthetic(protocol.PartitionUrl(exec.Network.LocalPartitionID)).
		Sign(protocol.SignatureTypeED25519, exec.Key)

	// Check passes
	_, perr := exec.CheckTx(env)
	if perr != nil {
		require.NoError(t, perr)
	}

	// Deliver fails
	_, perr = exec.DeliverTx(env)
	require.NotNil(t, perr)

	// Commit the block
	_, err = exec.ForceCommit()
	require.NoError(t, err)

	// Verify that the synthetic transaction was recorded
	batch := exec.DB.Begin(false)
	defer batch.Discard()
	status, err := batch.Transaction(env.GetTxHash()).GetStatus()
	require.NoError(t, err, "Failed to get the synthetic transaction status")
	require.NotZero(t, status.Code)
}

func TestExecutor_ProcessTransaction(t *testing.T) {
	exec := setupWithGenesis(t)

	// Create the keys and URLs
	alice, bob, charlie := generateKey(), generateKey(), generateKey()
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)
	bobUrl := acctesting.AcmeLiteAddressTmPriv(bob)
	charlieUrl := acctesting.AcmeLiteAddressTmPriv(charlie)

	// Create the transaction
	envelope := acctesting.NewTransaction().
		WithPrincipal(aliceUrl).
		WithSigner(aliceUrl, 1).
		WithCurrentTimestamp().
		WithBody(&protocol.SendTokens{
			To: []*protocol.TokenRecipient{
				{Url: bobUrl, Amount: *big.NewInt(1000)},
				{Url: charlieUrl, Amount: *big.NewInt(2000)},
			},
		}).
		Initiate(protocol.SignatureTypeED25519, alice)

	// Initialize the database
	batch := exec.DB.Begin(true)
	defer batch.Discard()
	txnDb := batch.Transaction(envelope.GetTxHash())
	require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, alice, protocol.AcmeFaucetAmount, 1e9))
	require.NoError(t, txnDb.PutSignatures(&database.SignatureSet{Signatures: envelope.Signatures}))
	require.NoError(t, batch.Commit())

	// Execute the transaction
	batch = exec.DB.Begin(true)
	defer batch.Discard()
	_, _, err := exec.ProcessTransaction(batch, envelope.Transaction)
	require.NoError(t, err)
	require.NoError(t, batch.Commit())

	// Verify the transaction was recorded
	batch = exec.DB.Begin(false)
	defer batch.Discard()
	txnDb = batch.Transaction(envelope.GetTxHash())
	_, err = txnDb.GetState()
	require.NoError(t, err)
	status, err := txnDb.GetStatus()
	require.NoError(t, err)
	require.True(t, status.Delivered)
	require.Zero(t, status.Code)
}

func TestExecutor_DeliverTx(t *testing.T) {
	exec := setupWithGenesis(t)

	// Create the keys and URLs
	alice, bob, charlie := generateKey(), generateKey(), generateKey()
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)
	bobUrl := acctesting.AcmeLiteAddressTmPriv(bob)
	charlieUrl := acctesting.AcmeLiteAddressTmPriv(charlie)

	// Create the transaction
	envelope := acctesting.NewTransaction().
		WithPrincipal(aliceUrl).
		WithSigner(aliceUrl, 1).
		WithCurrentTimestamp().
		WithBody(&protocol.SendTokens{
			To: []*protocol.TokenRecipient{
				{Url: bobUrl, Amount: *big.NewInt(1000)},
				{Url: charlieUrl, Amount: *big.NewInt(2000)},
			},
		}).
		Initiate(protocol.SignatureTypeED25519, alice)

	// Initialize the database
	batch := exec.DB.Begin(true)
	defer batch.Discard()
	txnDb := batch.Transaction(envelope.GetTxHash())
	require.NoError(t, acctesting.CreateLiteTokenAccountWithCredits(batch, alice, protocol.AcmeFaucetAmount, 1e9))
	require.NoError(t, txnDb.PutSignatures(&database.SignatureSet{Signatures: envelope.Signatures}))
	require.NoError(t, batch.Commit())

	// Start a block
	_, err := exec.BeginBlock(BeginBlockRequest{
		IsLeader: true,
		Height:   2,
		Time:     time.Now(),
	})
	require.NoError(t, err)

	// Execute the transaction
	_, perr := exec.DeliverTx(envelope)
	if perr != nil {
		require.NoError(t, perr.Message)
	}

	// Commit the block
	_, err = exec.ForceCommit()
	require.NoError(t, err)

	// Verify the transaction was recorded
	batch = exec.DB.Begin(false)
	defer batch.Discard()
	txnDb = batch.Transaction(envelope.GetTxHash())
	_, err = txnDb.GetState()
	require.NoError(t, err)
	status, err := txnDb.GetStatus()
	require.NoError(t, err)
	require.True(t, status.Delivered)
	require.Zero(t, status.Code)
}
