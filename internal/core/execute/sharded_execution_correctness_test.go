// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// setupShardedDB creates an in-memory database and a ShardedExecutor with the
// given shard count. It begins a block so all shards have open batches.
func setupShardedDB(t *testing.T, shardCount int) (*database.Database, *ShardedExecutor) {
	t.Helper()
	db := database.OpenInMemory(nil)
	se, err := NewShardedExecutor(shardCount, db)
	require.NoError(t, err)
	se.BeginBlock()
	return db, se
}

// seedTokenAccount creates a TokenAccount at identity/tokens in the database.
func seedTokenAccount(t *testing.T, db *database.Database, identity string, balance int64) {
	t.Helper()
	u := protocol.AccountUrl(identity, "tokens")
	batch := db.Begin(true)
	defer batch.Discard()
	acct := &protocol.TokenAccount{
		Url:      u,
		TokenUrl: protocol.AcmeUrl(),
	}
	acct.Balance.SetInt64(balance)
	require.NoError(t, batch.Account(u).Main().Put(acct))
	require.NoError(t, batch.Commit())
}

// seedDataAccount creates a DataAccount at identity/data in the database.
func seedDataAccount(t *testing.T, db *database.Database, identity string) {
	t.Helper()
	u := protocol.AccountUrl(identity, "data")
	batch := db.Begin(true)
	defer batch.Discard()
	acct := &protocol.DataAccount{Url: u}
	require.NoError(t, batch.Account(u).Main().Put(acct))
	require.NoError(t, batch.Commit())
}

func tokenURL(identity string) *url.URL {
	return protocol.AccountUrl(identity, "tokens")
}

func dataURL(identity string) *url.URL {
	return protocol.AccountUrl(identity, "data")
}

func readTokenBalance(t *testing.T, db *database.Database, identity string) *big.Int {
	t.Helper()
	u := tokenURL(identity)
	batch := db.Begin(false)
	defer batch.Discard()
	var acct *protocol.TokenAccount
	require.NoError(t, batch.Account(u).Main().GetAs(&acct))
	return &acct.Balance
}

func TestSingleShardSendTokens(t *testing.T) {
	db, se := setupShardedDB(t, 1)
	defer se.Discard()

	seedTokenAccount(t, db, "alice", 1000)

	savingsURL := protocol.AccountUrl("alice", "savings")
	batch := db.Begin(true)
	savingsAcct := &protocol.TokenAccount{Url: savingsURL, TokenUrl: protocol.AcmeUrl()}
	savingsAcct.Balance.SetInt64(0)
	require.NoError(t, batch.Account(savingsURL).Main().Put(savingsAcct))
	require.NoError(t, batch.Commit())
	batch.Discard()

	srcURL := tokenURL("alice")
	require.Equal(t, 0, se.RouteAccount(srcURL))
	require.Equal(t, 0, se.RouteAccount(savingsURL))

	se.Discard()
	se.BeginBlock()

	shard := se.Shard(0)
	var src *protocol.TokenAccount
	require.NoError(t, shard.Account(srcURL).Main().GetAs(&src))
	src.Balance.Sub(&src.Balance, big.NewInt(250))
	require.NoError(t, shard.Account(srcURL).Main().Put(src))

	var dst *protocol.TokenAccount
	require.NoError(t, shard.Account(savingsURL).Main().GetAs(&dst))
	dst.Balance.Add(&dst.Balance, big.NewInt(250))
	require.NoError(t, shard.Account(savingsURL).Main().Put(dst))

	require.NoError(t, se.Commit())

	assert.Equal(t, big.NewInt(750), readTokenBalance(t, db, "alice"))

	rb := db.Begin(false)
	defer rb.Discard()
	var savings *protocol.TokenAccount
	require.NoError(t, rb.Account(savingsURL).Main().GetAs(&savings))
	assert.Equal(t, big.NewInt(250), &savings.Balance)
}

func TestSingleShardWriteData(t *testing.T) {
	db, se := setupShardedDB(t, 4)
	defer se.Discard()

	seedDataAccount(t, db, "mydata")
	u := dataURL("mydata")

	se.Discard()
	se.BeginBlock()

	shardID := se.RouteAccount(u)
	shard := se.Shard(shardID)

	entry := &protocol.AccumulateDataEntry{}
	entry.Data = [][]byte{[]byte("hello"), []byte("world")}
	dataAcct := &protocol.DataAccount{Url: u, Entry: entry}
	require.NoError(t, shard.Account(u).Main().Put(dataAcct))
	require.NoError(t, se.Commit())

	batch := db.Begin(false)
	defer batch.Discard()
	var result *protocol.DataAccount
	require.NoError(t, batch.Account(u).Main().GetAs(&result))
	require.NotNil(t, result.Entry)
}

func TestSingleShardCreateIdentity(t *testing.T) {
	db, se := setupShardedDB(t, 4)
	defer se.Discard()

	acctURL := protocol.AccountUrl("newident", "info")

	se.Discard()
	se.BeginBlock()

	shardID := se.RouteAccount(acctURL)
	shard := se.Shard(shardID)

	acct := &protocol.DataAccount{Url: acctURL}
	require.NoError(t, shard.Account(acctURL).Main().Put(acct))
	require.NoError(t, se.Commit())

	batch := db.Begin(false)
	defer batch.Discard()
	var result *protocol.DataAccount
	require.NoError(t, batch.Account(acctURL).Main().GetAs(&result))
	assert.True(t, acctURL.Equal(result.Url))
}

func TestMultiShardSendTokens(t *testing.T) {
	db, se := setupShardedDB(t, 4)
	defer se.Discard()

	seedTokenAccount(t, db, "alice", 500)
	seedTokenAccount(t, db, "bob", 100)

	aliceURL := tokenURL("alice")
	bobURL := tokenURL("bob")

	aliceShard := se.RouteAccount(aliceURL)
	bobShard := se.RouteAccount(bobURL)

	se.Discard()
	se.BeginBlock()

	aliceExec := se.Shard(aliceShard)
	var aliceAcct *protocol.TokenAccount
	require.NoError(t, aliceExec.Account(aliceURL).Main().GetAs(&aliceAcct))
	aliceAcct.Balance.Sub(&aliceAcct.Balance, big.NewInt(200))
	require.NoError(t, aliceExec.Account(aliceURL).Main().Put(aliceAcct))

	bobExec := se.Shard(bobShard)
	var bobAcct *protocol.TokenAccount
	require.NoError(t, bobExec.Account(bobURL).Main().GetAs(&bobAcct))
	bobAcct.Balance.Add(&bobAcct.Balance, big.NewInt(200))
	require.NoError(t, bobExec.Account(bobURL).Main().Put(bobAcct))

	require.NoError(t, se.Commit())

	assert.Equal(t, big.NewInt(300), readTokenBalance(t, db, "alice"))
	assert.Equal(t, big.NewInt(300), readTokenBalance(t, db, "bob"))
}

func TestMultiShardParallelExecution(t *testing.T) {
	db, se := setupShardedDB(t, 4)
	defer se.Discard()

	for i := 0; i < 20; i++ {
		seedTokenAccount(t, db, fmt.Sprintf("user%d", i), 1000)
	}

	se.Discard()
	se.BeginBlock()

	var mu sync.Mutex
	totalDeducted := int64(0)

	err := se.ForEachShard(func(shard *PerShardExecutor) error {
		for i := 0; i < 20; i++ {
			u := tokenURL(fmt.Sprintf("user%d", i))
			if se.RouteAccount(u) != shard.ID {
				continue
			}
			var acct *protocol.TokenAccount
			if err := shard.Account(u).Main().GetAs(&acct); err != nil {
				return err
			}
			acct.Balance.Sub(&acct.Balance, big.NewInt(10))
			if err := shard.Account(u).Main().Put(acct); err != nil {
				return err
			}
			mu.Lock()
			totalDeducted += 10
			mu.Unlock()
		}
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, se.Commit())

	assert.Equal(t, int64(200), totalDeducted)

	for i := 0; i < 20; i++ {
		bal := readTokenBalance(t, db, fmt.Sprintf("user%d", i))
		assert.Equal(t, big.NewInt(990), bal, "user%d balance", i)
	}
}

func TestSyntheticDispatchRouting(t *testing.T) {
	dispatcher := NewTransactionDispatcher(2) // 4 shards

	aliceURL := tokenURL("alice")
	bobURL := tokenURL("bob")

	txn := &protocol.Transaction{
		Body: &protocol.SendTokens{
			To: []*protocol.TokenRecipient{
				{Url: bobURL, Amount: *big.NewInt(100)},
			},
		},
	}
	txn.Header.Principal = aliceURL

	result := dispatcher.RouteTransaction(txn)

	aliceShard := dispatcher.RouteToShard(aliceURL)
	require.Contains(t, result.Portions, aliceShard)
	require.True(t, result.Portions[aliceShard].IsPrimary)

	require.Len(t, result.Synthetics, 1)
	assert.Equal(t, protocol.TransactionTypeSyntheticDepositTokens, result.Synthetics[0].Type)
	assert.True(t, bobURL.Equal(result.Synthetics[0].Destination))

	bobShard := dispatcher.RouteToShard(bobURL)
	syntheticShard := dispatcher.RouteToShard(result.Synthetics[0].Destination)
	assert.Equal(t, bobShard, syntheticShard)
}

func TestSyntheticAndOriginalExecution(t *testing.T) {
	db, se := setupShardedDB(t, 4)
	defer se.Discard()

	seedTokenAccount(t, db, "alice", 1000)
	seedTokenAccount(t, db, "bob", 0)

	aliceURL := tokenURL("alice")
	bobURL := tokenURL("bob")

	se.Discard()
	se.BeginBlock()

	aliceShard := se.Shard(se.RouteAccount(aliceURL))
	var aliceAcct *protocol.TokenAccount
	require.NoError(t, aliceShard.Account(aliceURL).Main().GetAs(&aliceAcct))
	aliceAcct.Balance.Sub(&aliceAcct.Balance, big.NewInt(500))
	require.NoError(t, aliceShard.Account(aliceURL).Main().Put(aliceAcct))

	bobShard := se.Shard(se.RouteAccount(bobURL))
	var bobAcct *protocol.TokenAccount
	require.NoError(t, bobShard.Account(bobURL).Main().GetAs(&bobAcct))
	bobAcct.Balance.Add(&bobAcct.Balance, big.NewInt(500))
	require.NoError(t, bobShard.Account(bobURL).Main().Put(bobAcct))

	require.NoError(t, se.Commit())

	assert.Equal(t, big.NewInt(500), readTokenBalance(t, db, "alice"))
	assert.Equal(t, big.NewInt(500), readTokenBalance(t, db, "bob"))
}

func TestMultipleTransactionsInBlock(t *testing.T) {
	db, se := setupShardedDB(t, 1)
	defer se.Discard()

	seedTokenAccount(t, db, "alice", 1000)

	savingsURL := protocol.AccountUrl("alice", "savings")
	batch := db.Begin(true)
	savingsAcct := &protocol.TokenAccount{Url: savingsURL, TokenUrl: protocol.AcmeUrl()}
	savingsAcct.Balance.SetInt64(0)
	require.NoError(t, batch.Account(savingsURL).Main().Put(savingsAcct))
	require.NoError(t, batch.Commit())
	batch.Discard()

	aliceURL := tokenURL("alice")

	se.Discard()
	se.BeginBlock()

	shard := se.Shard(se.RouteAccount(aliceURL))

	// Transaction 1: alice -> savings (200)
	var alice *protocol.TokenAccount
	require.NoError(t, shard.Account(aliceURL).Main().GetAs(&alice))
	alice.Balance.Sub(&alice.Balance, big.NewInt(200))
	require.NoError(t, shard.Account(aliceURL).Main().Put(alice))

	var savings *protocol.TokenAccount
	require.NoError(t, shard.Account(savingsURL).Main().GetAs(&savings))
	savings.Balance.Add(&savings.Balance, big.NewInt(200))
	require.NoError(t, shard.Account(savingsURL).Main().Put(savings))

	// Transaction 2: alice -> savings (300) sequenced after txn 1.
	require.NoError(t, shard.Account(aliceURL).Main().GetAs(&alice))
	alice.Balance.Sub(&alice.Balance, big.NewInt(300))
	require.NoError(t, shard.Account(aliceURL).Main().Put(alice))

	require.NoError(t, shard.Account(savingsURL).Main().GetAs(&savings))
	savings.Balance.Add(&savings.Balance, big.NewInt(300))
	require.NoError(t, shard.Account(savingsURL).Main().Put(savings))

	require.NoError(t, se.Commit())

	assert.Equal(t, big.NewInt(500), readTokenBalance(t, db, "alice"))

	rb := db.Begin(false)
	defer rb.Discard()
	require.NoError(t, rb.Account(savingsURL).Main().GetAs(&savings))
	assert.Equal(t, big.NewInt(500), &savings.Balance)
}

func TestOverlappingAccountsSerializePerShard(t *testing.T) {
	db, se := setupShardedDB(t, 4)
	defer se.Discard()

	seedTokenAccount(t, db, "org", 5000)

	fund1URL := protocol.AccountUrl("org", "fund1")
	fund2URL := protocol.AccountUrl("org", "fund2")

	batch := db.Begin(true)
	f1 := &protocol.TokenAccount{Url: fund1URL, TokenUrl: protocol.AcmeUrl()}
	f1.Balance.SetInt64(0)
	require.NoError(t, batch.Account(fund1URL).Main().Put(f1))
	f2 := &protocol.TokenAccount{Url: fund2URL, TokenUrl: protocol.AcmeUrl()}
	f2.Balance.SetInt64(0)
	require.NoError(t, batch.Account(fund2URL).Main().Put(f2))
	require.NoError(t, batch.Commit())
	batch.Discard()

	orgURL := tokenURL("org")

	shardID := se.RouteAccount(orgURL)
	require.Equal(t, shardID, se.RouteAccount(fund1URL))
	require.Equal(t, shardID, se.RouteAccount(fund2URL))

	se.Discard()
	se.BeginBlock()

	shard := se.Shard(shardID)

	// Txn 1: org -> fund1 (1000)
	var org *protocol.TokenAccount
	require.NoError(t, shard.Account(orgURL).Main().GetAs(&org))
	org.Balance.Sub(&org.Balance, big.NewInt(1000))
	require.NoError(t, shard.Account(orgURL).Main().Put(org))

	var fund1 *protocol.TokenAccount
	require.NoError(t, shard.Account(fund1URL).Main().GetAs(&fund1))
	fund1.Balance.Add(&fund1.Balance, big.NewInt(1000))
	require.NoError(t, shard.Account(fund1URL).Main().Put(fund1))

	// Txn 2: org -> fund2 (2000) — must see updated balance from txn 1.
	require.NoError(t, shard.Account(orgURL).Main().GetAs(&org))
	assert.Equal(t, big.NewInt(4000), &org.Balance, "second txn should see post-txn1 balance")
	org.Balance.Sub(&org.Balance, big.NewInt(2000))
	require.NoError(t, shard.Account(orgURL).Main().Put(org))

	var fund2 *protocol.TokenAccount
	require.NoError(t, shard.Account(fund2URL).Main().GetAs(&fund2))
	fund2.Balance.Add(&fund2.Balance, big.NewInt(2000))
	require.NoError(t, shard.Account(fund2URL).Main().Put(fund2))

	require.NoError(t, se.Commit())

	assert.Equal(t, big.NewInt(2000), readTokenBalance(t, db, "org"))

	rb := db.Begin(false)
	defer rb.Discard()
	require.NoError(t, rb.Account(fund1URL).Main().GetAs(&fund1))
	assert.Equal(t, big.NewInt(1000), &fund1.Balance)
	require.NoError(t, rb.Account(fund2URL).Main().GetAs(&fund2))
	assert.Equal(t, big.NewInt(2000), &fund2.Balance)
}

func TestErrorRollbackAllShards(t *testing.T) {
	db, se := setupShardedDB(t, 4)
	defer se.Discard()

	seedTokenAccount(t, db, "alice", 1000)
	seedTokenAccount(t, db, "bob", 500)

	aliceURL := tokenURL("alice")
	bobURL := tokenURL("bob")

	se.Discard()
	se.BeginBlock()

	aliceShard := se.RouteAccount(aliceURL)
	bobShard := se.RouteAccount(bobURL)

	affectedMap := map[int]bool{aliceShard: true, bobShard: true}
	affected := make([]int, 0, len(affectedMap))
	for id := range affectedMap {
		affected = append(affected, id)
	}

	executeFn := func(shard *PerShardExecutor) (interface{}, error) {
		if shard.ID == bobShard {
			return nil, fmt.Errorf("simulated shard failure")
		}
		var alice *protocol.TokenAccount
		if err := shard.Account(aliceURL).Main().GetAs(&alice); err != nil {
			return nil, err
		}
		alice.Balance.Sub(&alice.Balance, big.NewInt(100))
		return nil, shard.Account(aliceURL).Main().Put(alice)
	}

	_, err := se.ExecuteTransactionOnShards(context.Background(), affected, executeFn)
	require.Error(t, err)
	require.Contains(t, err.Error(), "simulated shard failure")

	assert.Equal(t, big.NewInt(1000), readTokenBalance(t, db, "alice"))
	assert.Equal(t, big.NewInt(500), readTokenBalance(t, db, "bob"))
}

func TestDiscardRollsBack(t *testing.T) {
	db, se := setupShardedDB(t, 2)
	defer se.Discard()

	seedTokenAccount(t, db, "alice", 1000)

	se.Discard()
	se.BeginBlock()

	aliceURL := tokenURL("alice")
	shard := se.Shard(se.RouteAccount(aliceURL))

	var alice *protocol.TokenAccount
	require.NoError(t, shard.Account(aliceURL).Main().GetAs(&alice))
	alice.Balance.SetInt64(0)
	require.NoError(t, shard.Account(aliceURL).Main().Put(alice))

	se.Discard()

	assert.Equal(t, big.NewInt(1000), readTokenBalance(t, db, "alice"))
}

func TestBPTRootHashConsistency(t *testing.T) {
	acctURL := protocol.AccountUrl("testacct", "tokens")

	writeAndHash := func() [32]byte {
		db := database.OpenInMemory(nil)

		se, err := NewShardedExecutor(4, db)
		require.NoError(t, err)
		se.BeginBlock()

		shard := se.Shard(se.RouteAccount(acctURL))
		acct := &protocol.TokenAccount{
			Url:      acctURL,
			TokenUrl: protocol.AcmeUrl(),
		}
		acct.Balance.SetInt64(42)
		require.NoError(t, shard.Account(acctURL).Main().Put(acct))
		require.NoError(t, se.Commit())

		batch := db.Begin(false)
		defer batch.Discard()
		var result *protocol.TokenAccount
		require.NoError(t, batch.Account(acctURL).Main().GetAs(&result))
		require.Equal(t, int64(42), result.Balance.Int64())

		hash, err := batch.GetBptRootHash()
		require.NoError(t, err)
		return hash
	}

	hash1 := writeAndHash()
	hash2 := writeAndHash()

	assert.Equal(t, hash1, hash2, "identical operations should produce identical BPT root hashes")
}

func TestDispatchBlockGrouping(t *testing.T) {
	dispatcher := NewTransactionDispatcher(2) // 4 shards

	txns := make([]*protocol.Transaction, 10)
	for i := range txns {
		txns[i] = &protocol.Transaction{
			Body: &protocol.WriteData{},
		}
		txns[i].Header.Principal = protocol.AccountUrl(fmt.Sprintf("acct%d", i), "data")
	}

	shards := dispatcher.DispatchBlock(txns)
	require.Len(t, shards, 4)

	seen := make(map[int]bool)
	for _, indices := range shards {
		for _, idx := range indices {
			require.False(t, seen[idx], "transaction %d dispatched to multiple shards", idx)
			seen[idx] = true
		}
	}
	require.Len(t, seen, 10)

	for shardID, indices := range shards {
		for _, idx := range indices {
			expected := dispatcher.RouteToShard(txns[idx].Header.Principal)
			assert.Equal(t, shardID, expected, "txn %d in wrong shard", idx)
		}
	}
}

func TestTransferCreditsMultiShard(t *testing.T) {
	dispatcher := NewTransactionDispatcher(2) // 4 shards

	srcURL := protocol.AccountUrl("alice", "credits")
	dst1URL := protocol.AccountUrl("bob", "credits")
	dst2URL := protocol.AccountUrl("charlie", "credits")

	txn := &protocol.Transaction{
		Body: &protocol.TransferCredits{
			To: []*protocol.CreditRecipient{
				{Url: dst1URL, Amount: 100},
				{Url: dst2URL, Amount: 200},
			},
		},
	}
	txn.Header.Principal = srcURL

	result := dispatcher.RouteTransaction(txn)

	srcShard := dispatcher.RouteToShard(srcURL)
	require.Contains(t, result.Portions, srcShard)
	assert.True(t, result.Portions[srcShard].IsPrimary)

	dst1Shard := dispatcher.RouteToShard(dst1URL)
	dst2Shard := dispatcher.RouteToShard(dst2URL)
	require.Contains(t, result.Portions, dst1Shard)
	require.Contains(t, result.Portions, dst2Shard)

	assert.Empty(t, result.Synthetics)
}
