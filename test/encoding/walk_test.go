// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import (
	"math/big"
	"strings"
	"testing"

	"github.com/cometbft/cometbft/libs/log"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/bsn"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/helpers"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() {
	acctesting.EnableDebugFeatures()
}

func TestWalkAndReplay(t *testing.T) {
	t.Skip("Flakey")

	liteKey := acctesting.GenerateKey("Lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey)

	// Use the simulator to create genesis documents
	net := simulator.SimpleNetwork(t.Name(), 3, 1)
	sim := NewSim(t,
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
		net,
		simulator.SnapshotMap(genesis),
	)

	// Capture blocks
	type Value struct {
		Key   *record.Key
		Value []byte
	}
	type Account struct {
		Url  *url.URL
		Hash [32]byte
	}
	type Block struct {
		Index    uint64
		Hash     [32]byte
		Changes  []*Value
		Accounts map[[32]byte]*Account
	}
	blocks := map[string][]*Block{}
	for _, p := range sim.Partitions() {
		p := p
		events.SubscribeSync(sim.S.EventBus(p.ID), func(e execute.WillCommitBlock) error {
			block := new(Block)
			block.Index = e.Block.Params().Index
			_ = e.Block.ChangeSet().Walk(record.WalkOptions{
				Values:        true,
				Modified:      true,
				IgnoreIndices: true,
			}, func(r record.Record) (bool, error) {
				v, _, err := r.(record.TerminalRecord).GetValue()
				require.NoError(t, err)
				b, err := v.MarshalBinary()
				require.NoError(t, err)
				block.Changes = append(block.Changes, &Value{r.Key(), b})
				return false, nil
			})
			blocks[p.ID] = append(blocks[p.ID], block)
			return nil
		})

		events.SubscribeSync(sim.S.EventBus(p.ID), func(e events.DidCommitBlock) error {
			blocks := blocks[p.ID]
			block := blocks[len(blocks)-1]
			block.Accounts = map[[32]byte]*Account{}
			require.Equal(t, e.Index, block.Index)
			View(t, sim.Database(p.ID), func(batch *database.Batch) {
				var err error
				block.Hash, err = batch.BPT().GetRootHash()
				require.NoError(t, err)
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

	// Restore snapshot into BSN database
	logger := acctesting.NewTestLogger(t)
	store := memory.New(nil)
	bsndb := bsn.NewChangeSet(store, logger.With("module", "database"))
	for _, part := range sim.Partitions() {
		err := snapshot.Restore(bsndb.Partition(part.ID), ioutil2.NewBuffer(genesis[part.ID]), logger.With("module", "snapshot"))
		require.NoError(t, err)
	}
	require.NoError(t, bsndb.Commit())

	// Replay blocks
	for id, blocks := range blocks {
		id = strings.ToLower(id)
		db := &partitionBeginner{logger: logger, store: store, partition: id}
		for _, block := range blocks {
			require.NotNil(t, block.Hash)

			Update(t, db, func(batch *database.Batch) {
				for _, v := range block.Changes {
					w, err := resolveValue[record.ValueWriter](batch, v.Key)
					require.NoError(t, err)
					err = w.LoadBytes(v.Value, true)
					require.NoError(t, err)
				}
			})

			View(t, db, func(batch *database.Batch) {
				hash, err := batch.BPT().GetRootHash()
				require.NoError(t, err)
				if block.Hash == hash {
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

	bsndb = bsn.NewChangeSet(store, logger.With("module", "database"))
	p, err := sim.Router().RouteAccount(lite2)
	require.NoError(t, err)
	require.NotZero(t, GetAccount[*protocol.LiteTokenAccount](t, bsndb.Partition(p), lite2).Balance)
}

type partitionBeginner struct {
	logger    log.Logger
	store     keyvalue.Beginner
	partition string
}

func (p *partitionBeginner) SetObserver(observer database.Observer) {}

func (p *partitionBeginner) Begin(writable bool) *database.Batch {
	s := p.store.Begin(record.NewKey(p.partition+"·"), true)
	b := database.NewBatch(p.partition, s, writable, p.logger)
	b.SetObserver(execute.NewDatabaseObserver())
	return b
}

func (p *partitionBeginner) View(fn func(*database.Batch) error) error {
	b := p.Begin(false)
	defer b.Discard()
	return fn(b)
}

func (p *partitionBeginner) Update(fn func(*database.Batch) error) error {
	b := p.Begin(true)
	defer b.Discard()
	err := fn(b)
	if err != nil {
		return err
	}
	return b.Commit()
}

func zero[T any]() T {
	var z T
	return z
}

// resolveValue resolves the value for the given key.
func resolveValue[T any](r record.Record, key *record.Key) (T, error) {
	var err error
	for key.Len() > 0 {
		r, key, err = r.Resolve(key)
		if err != nil {
			return zero[T](), errors.UnknownError.Wrap(err)
		}
	}

	if s, _, err := r.Resolve(nil); err == nil {
		r = s
	}

	v, ok := r.(T)
	if !ok {
		return zero[T](), errors.InternalError.WithFormat("bad key: %T is not value", r)
	}

	return v, nil
}
