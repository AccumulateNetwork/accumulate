// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func mustParseUrl(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func TestTransactionDispatcher_NumShards(t *testing.T) {
	tests := []struct {
		depth    int
		expected int
	}{
		{2, 4},
		{4, 16},
		{6, 64},
	}
	for _, tt := range tests {
		d := NewTransactionDispatcher(tt.depth)
		assert.Equal(t, tt.expected, d.NumShards())
	}
}

func TestTransactionDispatcher_RouteToShard(t *testing.T) {
	d := NewTransactionDispatcher(6) // 64 shards

	// Same account always routes to the same shard.
	acct := mustParseUrl("acc://alice.acme/tokens")
	shard1 := d.RouteToShard(acct)
	shard2 := d.RouteToShard(acct)
	assert.Equal(t, shard1, shard2, "deterministic routing")

	// Shard index is within range.
	assert.True(t, shard1 >= 0 && shard1 < 64)

	// All accounts under the same ADI route to the same shard (ADI hash routing).
	identity := mustParseUrl("acc://alice.acme")
	tokens := mustParseUrl("acc://alice.acme/tokens")
	book := mustParseUrl("acc://alice.acme/book")
	page := mustParseUrl("acc://alice.acme/book/1")
	assert.Equal(t, d.RouteToShard(identity), d.RouteToShard(tokens), "same ADI -> same shard")
	assert.Equal(t, d.RouteToShard(identity), d.RouteToShard(book), "same ADI -> same shard")
	assert.Equal(t, d.RouteToShard(identity), d.RouteToShard(page), "same ADI -> same shard")
}

func TestTransactionDispatcher_SendTokens_SameShard(t *testing.T) {
	d := NewTransactionDispatcher(6)

	// Build a SendTokens where sender and receiver happen to share a shard.
	// We find two accounts on the same shard by brute-force.
	sender, receiver := findAccountsOnSameShard(t, d)

	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: sender},
		Body: &protocol.SendTokens{
			To: []*protocol.TokenRecipient{
				{Url: receiver},
			},
		},
	}

	result := d.RouteTransaction(txn)

	// Single shard handles the principal.
	require.Len(t, result.Portions, 1, "single shard for same-shard send")
	for _, portion := range result.Portions {
		assert.True(t, portion.IsPrimary)
		assert.Contains(t, portion.Accounts, sender)
	}

	// Synthetic deposit to receiver.
	require.Len(t, result.Synthetics, 1)
	assert.Equal(t, protocol.TransactionTypeSyntheticDepositTokens, result.Synthetics[0].Type)
	assert.True(t, result.Synthetics[0].Destination.Equal(receiver))
}

func TestTransactionDispatcher_SendTokens_MultiRecipient(t *testing.T) {
	d := NewTransactionDispatcher(6)
	sender := mustParseUrl("acc://sender.acme/tokens")
	r1 := mustParseUrl("acc://bob.acme/tokens")
	r2 := mustParseUrl("acc://carol.acme/tokens")

	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: sender},
		Body: &protocol.SendTokens{
			To: []*protocol.TokenRecipient{
				{Url: r1},
				{Url: r2},
			},
		},
	}

	result := d.RouteTransaction(txn)

	// Principal shard must exist and be primary.
	senderShard := d.RouteToShard(sender)
	require.Contains(t, result.Portions, senderShard)
	assert.True(t, result.Portions[senderShard].IsPrimary)

	// Two synthetic dispatches.
	assert.Len(t, result.Synthetics, 2)
	assert.Equal(t, protocol.TransactionTypeSyntheticDepositTokens, result.Synthetics[0].Type)
	assert.Equal(t, protocol.TransactionTypeSyntheticDepositTokens, result.Synthetics[1].Type)
}

func TestTransactionDispatcher_CreateIdentity(t *testing.T) {
	d := NewTransactionDispatcher(4) // 16 shards
	parent := mustParseUrl("acc://parent.acme")
	newIdentity := mustParseUrl("acc://child.acme")
	keyBook := mustParseUrl("acc://child.acme/book")

	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: parent},
		Body: &protocol.CreateIdentity{
			Url:        newIdentity,
			KeyBookUrl: keyBook,
		},
	}

	result := d.RouteTransaction(txn)

	// Principal shard is marked primary.
	parentShard := d.RouteToShard(parent)
	require.Contains(t, result.Portions, parentShard)
	assert.True(t, result.Portions[parentShard].IsPrimary)

	// New identity and key book share the same ADI (child.acme), so same shard.
	identityShard := d.RouteToShard(newIdentity)
	keyBookShard := d.RouteToShard(keyBook)
	assert.Equal(t, identityShard, keyBookShard, "same ADI -> same shard")
	require.Contains(t, result.Portions, identityShard)

	// No synthetics for create identity dispatch (synthetics are produced at
	// execution time for chain creation).
	assert.Len(t, result.Synthetics, 0)
}

func TestTransactionDispatcher_AddCredits(t *testing.T) {
	d := NewTransactionDispatcher(4)
	sender := mustParseUrl("acc://alice.acme/tokens")
	recipient := mustParseUrl("acc://alice.acme/book/1")

	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: sender},
		Body: &protocol.AddCredits{
			Recipient: recipient,
		},
	}

	result := d.RouteTransaction(txn)

	// Principal touched.
	senderShard := d.RouteToShard(sender)
	require.Contains(t, result.Portions, senderShard)
	assert.True(t, result.Portions[senderShard].IsPrimary)

	// Synthetic credit deposit.
	require.Len(t, result.Synthetics, 1)
	assert.Equal(t, protocol.TransactionTypeSyntheticDepositCredits, result.Synthetics[0].Type)
	assert.True(t, result.Synthetics[0].Destination.Equal(recipient))
}

func TestTransactionDispatcher_WriteData(t *testing.T) {
	d := NewTransactionDispatcher(4)
	account := mustParseUrl("acc://data.acme/account")

	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: account},
		Body:   &protocol.WriteData{},
	}

	result := d.RouteTransaction(txn)

	// Only the principal shard.
	require.Len(t, result.Portions, 1)
	assert.Len(t, result.Synthetics, 0)
}

func TestTransactionDispatcher_WriteDataTo(t *testing.T) {
	d := NewTransactionDispatcher(4)
	sender := mustParseUrl("acc://alice.acme/data")
	recipient := mustParseUrl("acc://bob.acme/data")

	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: sender},
		Body: &protocol.WriteDataTo{
			Recipient: recipient,
		},
	}

	result := d.RouteTransaction(txn)

	require.Len(t, result.Synthetics, 1)
	assert.Equal(t, protocol.TransactionTypeSyntheticWriteData, result.Synthetics[0].Type)
	assert.True(t, result.Synthetics[0].Destination.Equal(recipient))
}

func TestTransactionDispatcher_AcmeFaucet(t *testing.T) {
	d := NewTransactionDispatcher(4)
	faucet := mustParseUrl("acc://faucet.acme")
	target := mustParseUrl("acc://1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef/ACME")

	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: faucet},
		Body: &protocol.AcmeFaucet{
			Url: target,
		},
	}

	result := d.RouteTransaction(txn)

	require.Len(t, result.Synthetics, 1)
	assert.Equal(t, protocol.TransactionTypeSyntheticDepositTokens, result.Synthetics[0].Type)
}

func TestTransactionDispatcher_CreateTokenAccount(t *testing.T) {
	d := NewTransactionDispatcher(4)
	identity := mustParseUrl("acc://alice.acme")
	newAccount := mustParseUrl("acc://alice.acme/tokens")

	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: identity},
		Body: &protocol.CreateTokenAccount{
			Url:      newAccount,
			TokenUrl: mustParseUrl("acc://ACME"),
		},
	}

	result := d.RouteTransaction(txn)

	// With ADI hash routing, identity and its sub-accounts share the same shard.
	identityShard := d.RouteToShard(identity)
	newShard := d.RouteToShard(newAccount)
	assert.Equal(t, identityShard, newShard, "same ADI -> same shard")

	// Single shard with both accounts.
	require.Len(t, result.Portions, 1)
	require.Contains(t, result.Portions, identityShard)
	assert.True(t, result.Portions[identityShard].IsPrimary)
	assert.Len(t, result.Portions[identityShard].Accounts, 2, "both identity and new account listed")
}

func TestTransactionDispatcher_SyntheticDepositTokens(t *testing.T) {
	d := NewTransactionDispatcher(4)
	dest := mustParseUrl("acc://bob.acme/tokens")

	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: dest},
		Body:   &protocol.SyntheticDepositTokens{},
	}

	result := d.RouteTransaction(txn)

	// Only the principal shard.
	require.Len(t, result.Portions, 1)
	shard := d.RouteToShard(dest)
	require.Contains(t, result.Portions, shard)
	assert.True(t, result.Portions[shard].IsPrimary)
	assert.Len(t, result.Synthetics, 0)
}

func TestTransactionDispatcher_DispatchBlock(t *testing.T) {
	d := NewTransactionDispatcher(4) // 16 shards

	// Create a batch of transactions.
	txns := make([]*protocol.Transaction, 20)
	for i := range txns {
		txns[i] = &protocol.Transaction{
			Header: protocol.TransactionHeader{
				Principal: mustParseUrl(fmt.Sprintf("acc://user%d.acme/tokens", i)),
			},
			Body: &protocol.WriteData{},
		}
	}

	shards := d.DispatchBlock(txns)
	assert.Len(t, shards, 16)

	// Every transaction index should appear exactly once across all shards.
	seen := make(map[int]bool)
	for _, indices := range shards {
		for _, idx := range indices {
			assert.False(t, seen[idx], "transaction %d dispatched to multiple shards", idx)
			seen[idx] = true
		}
	}
	assert.Len(t, seen, 20, "all transactions dispatched")
}

func TestTransactionDispatcher_NilPrincipal(t *testing.T) {
	d := NewTransactionDispatcher(4)

	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: nil},
		Body:   &protocol.WriteData{},
	}

	result := d.RouteTransaction(txn)
	assert.Len(t, result.Portions, 0, "no portions for nil principal")
}

func TestTransactionDispatcher_TransferCredits(t *testing.T) {
	d := NewTransactionDispatcher(4)
	sender := mustParseUrl("acc://alice.acme/credits")
	r1 := mustParseUrl("acc://bob.acme/credits")

	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: sender},
		Body: &protocol.TransferCredits{
			To: []*protocol.CreditRecipient{
				{Url: r1},
			},
		},
	}

	result := d.RouteTransaction(txn)

	// Both sender and recipient shards touched.
	senderShard := d.RouteToShard(sender)
	r1Shard := d.RouteToShard(r1)
	require.Contains(t, result.Portions, senderShard)
	require.Contains(t, result.Portions, r1Shard)

	// Transfer credits are intra-partition, no synthetics.
	assert.Len(t, result.Synthetics, 0)
}

// findAccountsOnSameShard tries account names until it finds two that route to
// the same shard.
func findAccountsOnSameShard(t *testing.T, d *TransactionDispatcher) (*url.URL, *url.URL) {
	t.Helper()
	shardAccounts := make(map[int]*url.URL)
	for i := 0; i < 1000; i++ {
		u := mustParseUrl(fmt.Sprintf("acc://test%d.acme/tokens", i))
		shard := d.RouteToShard(u)
		if existing, ok := shardAccounts[shard]; ok {
			return existing, u
		}
		shardAccounts[shard] = u
	}
	t.Fatal("could not find two accounts on the same shard")
	return nil, nil
}
