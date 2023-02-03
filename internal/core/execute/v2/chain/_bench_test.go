// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain_test

import (
	"math/big"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/badger"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/memory"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func BenchmarkExecuteSendTokens(b *testing.B) {
	testCases := map[string]struct {
		NewStorage func(log.Logger) storage.KeyValueStore
	}{
		"Memory": {NewStorage: func(logger log.Logger) storage.KeyValueStore {
			return memory.New(logger)
		}},
		"Badger": {NewStorage: func(logger log.Logger) storage.KeyValueStore {
			db, err := badger.New(filepath.Join(b.TempDir(), "badger.db"), logger)
			require.NoError(b, err)
			return db
		}},
	}

	for name, tc := range testCases {
		b.Run(name, func(b *testing.B) {
			logger := logging.NewTestLogger(b, "plain", "disabled", false)
			store := tc.NewStorage(logger)
			db := database.New(store, logger)

			network := config.Network{
				Type:             config.BlockValidator,
				LocalPartitionID: "BVN0",
				LocalAddress:     "http://0.0.bvn:12345",
				Partitions: []config.Partition{
					{
						ID:       protocol.Directory,
						Type:     config.Directory,
						BasePort: 12345,
						Nodes: []config.Node{
							{
								Address: "http://0.dn:12345",
								Type:    config.Validator,
							},
						},
					},
					{
						ID:       "BVN0",
						Type:     config.BlockValidator,
						BasePort: 12345,
						Nodes: []config.Node{
							{
								Address: "http://0.0.bvn:12345",
								Type:    config.Validator,
							},
						},
					},
				},
			}

			bootstrap, err := genesis.Init(store, genesis.InitOpts{
				Network: network,
				Logger:  logger,
			})
			require.NoError(b, err)
			err := bootstrap.Bootstrap()
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
				WithPrincipal(fromUrl).
				WithSigner(fromUrl, 1).
				WithTimestamp(1).
				WithBody(&protocol.SendTokens{
					To: []*protocol.TokenRecipient{
						{Url: toUrl0, Amount: *big.NewInt(1)},
					},
				}).
				Initiate(protocol.SignatureTypeED25519, fromKey)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Creating but never discarding the database batch should be OK with a memory DB
				_, err := exec.BeginBlock(BeginBlockRequest{IsLeader: true, Height: 1})
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
