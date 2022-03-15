package chain_test

import (
	"fmt"
	"math/big"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/crypto/ed25519"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	tmtypes "github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/abci"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/genesis"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
)

func BenchmarkHighTps(b *testing.B) {
	store := memory.New(nil)
	db := database.New(store, nil)

	network := config.Network{
		Type:          config.BlockValidator,
		LocalSubnetID: b.Name(),
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
	_, err = genesis.Init(kv, genesis.InitOpts{
		Network:     network,
		GenesisTime: time.Now(),
		Logger:      logger,
		Validators: []tmtypes.GenesisValidator{
			{PubKey: tmed25519.PubKey(nodeKey[32:])},
		},
	})
	require.NoError(b, err)

	state, err := kv.MarshalJSON()
	require.NoError(b, err)
	_, err = exec.InitChain(state, time.Now())
	require.NoError(b, err)

	liteKey := acctesting.GenerateKey(b.Name(), "Lite")
	liteAddr, err := protocol.LiteTokenAddress(liteKey[32:], protocol.ACME)
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
			_, err = exec.BeginBlock(abci.BeginBlockRequest{
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
					WithOrigin(liteAddr).
					WithNonceTimestamp().
					WithBody(deposit).
					Sign(func(nonce uint64, hash []byte) (protocol.Signature, error) {
						ed := new(protocol.ED25519Signature)
						err := ed.Sign(nonce, nodeKey, hash)
						return ed, err
					})

				_, err := exec.DeliverTx(env)
				if err != nil {
					require.NoError(b, err)
				}
			}

			b.ResetTimer()
			b.Run("DeliverTx", func(b *testing.B) {
				env := acctesting.NewTransaction().
					WithOrigin(liteAddr).
					WithNonceTimestamp().
					WithBody(deposit).
					Sign(func(nonce uint64, hash []byte) (protocol.Signature, error) {
						ed := new(protocol.ED25519Signature)
						err := ed.Sign(nonce, nodeKey, hash)
						return ed, err
					})

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
	// Setup
	logger := logging.NewTestLogger(t, "plain", acctesting.DefaultLogLevels, false)
	db := database.OpenInMemory(logger)
	key := acctesting.GenerateKey(t.Name())
	network := config.Network{
		Type:          config.BlockValidator,
		LocalSubnetID: t.Name(),
	}
	chain, err := chain.NewNodeExecutor(chain.ExecutorOptions{
		DB:      db,
		Logger:  logger,
		Key:     key,
		Network: network,
		Router:  acctesting.NullRouter{},
	})
	require.NoError(t, err)
	require.NoError(t, chain.Start())

	// Genesis
	temp := memory.New(logger)
	_, err = genesis.Init(temp, genesis.InitOpts{
		Network:     network,
		GenesisTime: time.Now(),
		Logger:      logger,
		Validators: []tmtypes.GenesisValidator{
			{PubKey: ed25519.PubKey(key[32:])},
		},
	})
	require.NoError(t, err)

	state, err := temp.MarshalJSON()
	require.NoError(t, err)

	_, err = chain.InitChain(state, time.Now())
	require.NoError(t, err)

	// Start a block
	_, err = chain.BeginBlock(abci.BeginBlockRequest{
		IsLeader: true,
		Height:   2,
		Time:     time.Now(),
	})
	require.NoError(t, err)

	// Create a synthetic transaction where the origin does not exist
	env := acctesting.NewTransaction().
		WithOrigin(url.MustParse("acc://account-that-does-not-exist")).
		WithNonceTimestamp().
		WithBody(&protocol.SyntheticDepositCredits{
			SyntheticOrigin: protocol.SyntheticOrigin{Cause: [32]byte{1}, Source: acctesting.FakeBvn},
			Amount:          1,
		}).
		Sign(func(nonce uint64, hash []byte) (protocol.Signature, error) {
			ed := new(protocol.ED25519Signature)
			err := ed.Sign(nonce, key, hash)
			return ed, err
		})

	// Check passes
	_, err = chain.CheckTx(env)
	require.Nilf(t, err, "%v", err)

	// Deliver fails
	_, err = chain.DeliverTx(env)
	require.NotNil(t, err)

	// Commit the block
	_, err = chain.ForceCommit()
	require.NoError(t, err)

	// Verify that the synthetic transaction was recorded
	batch := db.Begin(false)
	defer batch.Discard()
	status, err := batch.Transaction(env.GetTxHash()).GetStatus()
	require.NoError(t, err, "Failed to get the synthetic transaction status")
	require.NotZero(t, status.Code)
}
