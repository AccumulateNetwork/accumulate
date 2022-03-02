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
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/abci"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/genesis"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
)

func BenchmarkHighTps(b *testing.B) {
	store := new(memory.DB)
	_ = store.InitDB("", nil)
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

	kv := new(memory.DB)
	_ = kv.InitDB("", nil)
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
	_, err = exec.InitChain(state, time.Now(), 0)
	require.NoError(b, err)

	liteKey := acctesting.GenerateKey(b.Name(), "Lite")
	liteAddr, err := protocol.LiteTokenAddress(liteKey[32:], protocol.ACME)
	require.NoError(b, err)

	deposit := new(protocol.SyntheticDepositTokens)
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
