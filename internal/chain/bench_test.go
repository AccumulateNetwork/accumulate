package chain_test

import (
	"math/big"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/abci"
	"github.com/AccumulateNetwork/accumulate/internal/chain"
	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/genesis"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/smt/storage/badger"
	"github.com/AccumulateNetwork/accumulate/smt/storage/memory"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/libs/log"
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
				Type:     config.BlockValidator,
				ID:       "BVN0",
				BvnNames: []string{"BVN0"},
				Addresses: map[string][]string{
					strings.ToLower(protocol.Directory): {"http://0.dn:12345"},
					"bvn0":                              {"http://0.0.bvn:12345"},
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

			batch := db.Begin()
			require.NoError(b, acctesting.CreateLiteTokenAccountWithCredits(batch, tmed25519.PrivKey(fromKey), 1e9, 1e9))
			require.NoError(b, batch.Commit())

			env, err := transactions.NewWith(
				&transactions.Header{
					Origin:        fromUrl,
					KeyPageHeight: 1,
					Nonce:         1,
				}, edSigner(tmed25519.PrivKey(fromKey), 1),
				&protocol.SendTokens{
					To: []*protocol.TokenRecipient{
						{Url: toUrl0.String(), Amount: *big.NewInt(1)},
					},
				},
			)
			require.NoError(b, err)

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
