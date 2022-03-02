package chain_test

import (
	"math/big"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/abci"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/genesis"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/badger"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
)

func BenchmarkExecuteSendTokens(b *testing.B) {
	testCases := map[string]struct {
		NewStorage func(log.Logger) storage.KeyValueStore
	}{
		"Memory": {NewStorage: func(log.Logger) storage.KeyValueStore {
			return memory.NewDB()
		}},
		"Badger": {NewStorage: func(logger log.Logger) storage.KeyValueStore {
			db := new(badger.DB)
			require.NoError(b, db.InitDB(filepath.Join(b.TempDir(), "valacc.db"), logger))
			return db
		}},
	}

	for name, tc := range testCases {
		b.Run(name, func(b *testing.B) {
			logger := logging.NewTestLogger(b, "plain", "disabled", false)
			store := tc.NewStorage(logger)
			db := database.New(store, logger)

			network := config.Network{
				Type:          config.BlockValidator,
				LocalSubnetID: "BVN0",
				LocalAddress:  "http://0.0.bvn:12345",
				Subnets: []config.Subnet{
					{
						ID:   protocol.Directory,
						Type: config.Directory,
						Nodes: []config.Node{
							{
								Address: "http://0.dn:12345",
								Type:    config.Validator,
							},
						},
					},
					{
						ID:   "BVN0",
						Type: config.BlockValidator,
						Nodes: []config.Node{
							{
								Address: "http://0.0.bvn:12345",
								Type:    config.Validator,
							},
						},
					},
				},
			}

			_, err := genesis.Init(store, genesis.InitOpts{
				Network: network,
				Logger:  logger,
			})
			require.NoError(b, err)

			exec, err := chain.NewNodeExecutor(chain.ExecutorOptions{
				DB:      db,
				Logger:  logger,
				Key:     acctesting.GenerateKey(b.Name()),
				Network: network,
			})
			require.NoError(b, err)

			require.NoError(b, exec.Start())
			b.Cleanup(func() { require.NoError(b, exec.Stop()) })

			fromKey := acctesting.GenerateKey(b.Name(), "from")
			fromUrl := acctesting.AcmeLiteAddressStdPriv(fromKey)

			toKey0 := acctesting.GenerateKey(b.Name(), "to", 0)
			toUrl0 := acctesting.AcmeLiteAddressStdPriv(toKey0)

			batch := db.Begin(true)
			require.NoError(b, acctesting.CreateLiteTokenAccountWithCredits(batch, tmed25519.PrivKey(fromKey), 1e9, 1e9))
			require.NoError(b, batch.Commit())

			env := acctesting.NewTransaction().
				WithOrigin(fromUrl).
				WithKeyPage(0, 1).
				WithNonce(1).
				WithBody(&protocol.SendTokens{
					To: []*protocol.TokenRecipient{
						{Url: toUrl0, Amount: *big.NewInt(1)},
					},
				}).
				SignLegacyED25519(fromKey)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Creating but never discarding the database batch should be OK with a memory DB
				_, err := exec.BeginBlock(abci.BeginBlockRequest{IsLeader: true, Height: 1})
				if err != nil {
					b.Fatal(err)
				}
				_, perr := exec.DeliverTx(env)
				if perr != nil {
					b.Fatal(perr)
				}
			}
		})
	}
}
