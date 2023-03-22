// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import (
	"bytes"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/helpers"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestWalkAndReplay(t *testing.T) {
	liteKey := acctesting.GenerateKey("Lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey)

	// Use the simulator to create genesis documents
	net := simulator.SimpleNetwork("Gold/1.0.0", 3, 1)
	sim := NewSim(t,
		simulator.MemoryDatabase,
		net,
		simulator.Genesis(GenesisTime),
	)

	MakeLiteTokenAccount(t, sim.DatabaseFor(lite), liteKey[32:], protocol.AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(lite), lite.RootIdentity(), 1e18)
	CreditTokens(t, sim.DatabaseFor(lite), lite, big.NewInt(1e17))

	sim.StepN(100)

	// Capture the snapshot now as the genesis snapshot
	genesis := map[string][]byte{}
	for _, part := range sim.Partitions() {
		helpers.View(t, sim.Database(part.ID), func(batch *database.Batch) {
			buf := new(ioutil2.Buffer)
			_, err := snapshot.Collect(batch, new(snapshot.Header), buf, snapshot.CollectOptions{
				PreserveAccountHistory: func(*database.Account) (bool, error) { return true, nil },
			})
			require.NoError(t, err)
			genesis[part.ID] = buf.Bytes()
		})
	}

	// Set up the simulator and harness
	sim = NewSim(t,
		simulator.MemoryDatabase,
		net,
		simulator.SnapshotMap(genesis),
	)

	// Capture blocks
	type Value struct {
		Key   record.Key
		Value []byte
	}
	type Account struct {
		Url  *url.URL
		Hash [32]byte
	}
	type Block struct {
		Index    uint64
		Hash     []byte
		Changes  []*Value
		Accounts map[[32]byte]*Account
	}
	blocks := map[string][]*Block{}
	for _, p := range sim.Partitions() {
		sim.S.SetCommitHook(p.ID, func(p *protocol.PartitionInfo, state execute.BlockState) {
			block := new(Block)
			block.Index = state.Params().Index
			_ = state.WalkChanges(func(r record.TerminalRecord) error {
				v, _, err := r.GetValue()
				require.NoError(t, err)
				b, err := v.MarshalBinary()
				require.NoError(t, err)
				block.Changes = append(block.Changes, &Value{r.Key(), b})
				return nil
			})
			blocks[p.ID] = append(blocks[p.ID], block)
		})

		p := p
		events.SubscribeSync(sim.S.EventBus(p.ID), func(e events.DidCommitBlock) error {
			blocks := blocks[p.ID]
			block := blocks[len(blocks)-1]
			block.Accounts = map[[32]byte]*Account{}
			require.Equal(t, e.Index, block.Index)
			View(t, sim.Database(p.ID), func(batch *database.Batch) {
				block.Hash = batch.BptRoot()
				require.NoError(t, batch.ForEachAccount(func(account *database.Account, hash [32]byte) error {
					u := account.Url()
					block.Accounts[u.AccountID32()] = &Account{u, hash}
					return nil
				}))
			})
			return nil
		})
	}

	liteKey2 := acctesting.GenerateKey("Lite", 2)
	lite2 := acctesting.AcmeLiteAddressStdPriv(liteKey2)

	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(lite).
			SendTokens(1, protocol.AcmePrecisionPower).To(lite2).
			SignWith(lite).Version(1).Timestamp(1).PrivateKey(liteKey))
	sim.StepUntil(
		Txn(st.TxID).Completes())

	require.NotZero(t, GetAccount[*protocol.LiteTokenAccount](t, sim.DatabaseFor(lite2), lite2).Balance)

	sim.StepN(100)

	// Replay blocks
	sim = NewSim(t,
		simulator.MemoryDatabase,
		net,
		simulator.SnapshotMap(genesis),
	)

	fmt.Println()
	fmt.Println()

	for id, blocks := range blocks {
		db := sim.Database(id)
		for _, block := range blocks {
			require.NotNil(t, block.Hash)

			Update(t, db, func(batch *database.Batch) {
				for _, v := range block.Changes {
					require.NoError(t, batch.PutRawValue(v.Key, v.Value))
				}
			})

			View(t, db, func(batch *database.Batch) {
				if bytes.Equal(block.Hash, batch.BptRoot()) {
					return
				}
				t.Errorf("Root hash does not match for %s block %d", id, block.Index)

				require.NoError(t, batch.ForEachAccount(func(account *database.Account, hash [32]byte) error {
					u := account.Url()
					a, ok := block.Accounts[u.AccountID32()]
					if !ok {
						t.Errorf("Extra account %v", u)
						return nil
					}

					delete(block.Accounts, u.AccountID32())

					if hash != a.Hash {
						t.Errorf("%v hash does not match", u)
					}
					return nil
				}))

				for _, a := range block.Accounts {
					t.Errorf("Missing account %v", a.Url)
				}

				t.FailNow()
			})

		}
	}

	require.NotZero(t, GetAccount[*protocol.LiteTokenAccount](t, sim.DatabaseFor(lite2), lite2).Balance)
}
