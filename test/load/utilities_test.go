// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !testnet
// +build !testnet

package load_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestFindDevnetEndpoint tests the endpoint discovery
func TestFindDevnetEndpoint(t *testing.T) {
	endpoint, err := FindDevnetEndpoint()
	if err != nil {
		t.Skipf("No devnet found: %v", err)
	}

	t.Logf("Found devnet endpoint: %s", endpoint)

	// Verify endpoint works
	client := jsonrpc.NewClient(endpoint)
	ctx := context.Background()

	status, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	require.NoError(t, err)
	require.NotNil(t, status)

	t.Logf("Network status confirmed - Oracle price: %d", status.Oracle.Price)
}

// TestGenerateAccounts tests account generation
func TestGenerateAccounts(t *testing.T) {
	// Generate k accounts
	kAccounts := GenerateKAccounts()
	require.Len(t, kAccounts, 10)

	// Generate a accounts
	aAccounts := GenerateAAccounts()
	require.Len(t, aAccounts, 10)

	// Verify accounts are deterministic
	kAccounts2 := GenerateKAccounts()
	for i := range kAccounts {
		require.Equal(t, kAccounts[i].LiteURL.String(), kAccounts2[i].LiteURL.String())
		require.Equal(t, kAccounts[i].Key, kAccounts2[i].Key)
	}

	// Print account info
	t.Log("K Accounts:")
	for _, info := range GetAccountInfo(kAccounts, "k") {
		t.Log("  " + info)
	}

	t.Log("A Accounts:")
	for _, info := range GetAccountInfo(aAccounts, "a") {
		t.Log("  " + info)
	}
}

// TestFundAccounts tests funding accounts with ACME and credits
func TestFundAccounts(t *testing.T) {
	// Find endpoint
	endpoint, err := FindDevnetEndpoint()
	if err != nil {
		t.Skipf("No devnet found: %v", err)
	}

	// Generate test accounts (just use 2 for faster testing)
	accounts := GenerateTestAccounts("test", 2)

	// Fund accounts
	ctx := context.Background()
	config := FundingConfig{
		TargetBalance: 10,  // Just 10 ACME for testing
		CreditAmount:  100, // 100 credits
		MaxAttempts:   3,
		RetryDelay:    1000000000, // 1 second
	}

	err = FundAndPrepareAccounts(ctx, endpoint, accounts, config)
	require.NoError(t, err)

	// Verify funding
	client := jsonrpc.NewClient(endpoint)
	for i, account := range accounts {
		record, err := client.Query(ctx, account.LiteURL, &api.DefaultQuery{})
		require.NoError(t, err)

		accRecord, ok := record.(*api.AccountRecord)
		require.True(t, ok)

		tokenAccount, ok := accRecord.Account.(*protocol.LiteTokenAccount)
		require.True(t, ok)

		t.Logf("Account %d balance: %d (%.4f ACME)", i+1, tokenAccount.Balance.Int64(), float64(tokenAccount.Balance.Int64())/1e8)
		require.True(t, tokenAccount.Balance.Int64() > 0, "Account should have balance")
	}
}

// TestFullWorkflow tests the complete workflow
func TestFullWorkflow(t *testing.T) {
	// Step 1: Find endpoint
	endpoint, err := FindDevnetEndpoint()
	if err != nil {
		t.Skipf("No devnet found: %v", err)
	}
	t.Logf("Found endpoint: %s", endpoint)

	// Step 2: Generate accounts
	kAccounts := GenerateKAccounts()
	aAccounts := GenerateAAccounts()
	t.Logf("Generated %d k accounts and %d a accounts", len(kAccounts), len(aAccounts))

	// Step 3: Fund k accounts (just fund first 2 for speed)
	ctx := context.Background()
	config := DefaultFundingConfig()
	config.TargetBalance = 20 // Lower for testing

	kAccountsToFund := kAccounts[:2]
	err = FundAndPrepareAccounts(ctx, endpoint, kAccountsToFund, config)
	require.NoError(t, err)
	t.Log("Successfully funded k accounts")

	// Verify at least one account has balance
	client := jsonrpc.NewClient(endpoint)
	record, err := client.Query(ctx, kAccountsToFund[0].LiteURL, &api.DefaultQuery{})
	require.NoError(t, err)

	accRecord, ok := record.(*api.AccountRecord)
	require.True(t, ok)

	tokenAccount, ok := accRecord.Account.(*protocol.LiteTokenAccount)
	require.True(t, ok)

	t.Logf("First k account balance: %.4f ACME", float64(tokenAccount.Balance.Int64())/1e8)
	require.True(t, tokenAccount.Balance.Int64() > 0)
}
