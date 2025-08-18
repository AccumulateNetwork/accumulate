// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build integration
// +build integration

package client_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client"
)

// TestLiveMainnet tests against the live mainnet.
// Run with: go test -tags=integration -v ./pkg/client -run TestLiveMainnet
func TestLiveMainnet(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create mainnet client
	c, err := client.NewMainnet()
	require.NoError(t, err)

	t.Run("GetAccount_ACME", func(t *testing.T) {
		account, err := c.GetAccount(ctx, "acc://ACME")
		require.NoError(t, err)
		require.NotNil(t, account)
		require.NotNil(t, account.Account)
		require.Equal(t, "acc://ACME", account.Account.GetUrl().String())

		// ACME is a token issuer
		require.Equal(t, "tokenIssuer", account.Account.Type().String())
		t.Logf("Successfully queried ACME token: %+v", account.Account)
	})

	t.Run("GetNodeInfo", func(t *testing.T) {
		info, err := c.GetNodeInfo(ctx)
		require.NoError(t, err)
		require.NotNil(t, info)
		require.NotEmpty(t, info.Network)
		t.Logf("Node info: Network=%s, PeerID=%s", info.Network, info.PeerID)
	})

	t.Run("GetNetworkStatus", func(t *testing.T) {
		status, err := c.GetNetworkStatus(ctx)
		require.NoError(t, err)
		require.NotNil(t, status)
		t.Logf("Network status: %+v", status)
	})

	t.Run("GetDirectory_ACME", func(t *testing.T) {
		// Get directory of ACME (should have sub-accounts)
		dir, err := c.GetDirectory(ctx, "acc://ACME", 0, 10)
		require.NoError(t, err)
		require.NotNil(t, dir)
		t.Logf("ACME directory has %d total entries", dir.Total)
		if len(dir.Records) > 0 {
			t.Logf("First entry: %s", dir.Records[0].Account.GetUrl())
		}
	})
}

// TestLiveTestnet tests against the live testnet.
// Run with: go test -tags=integration -v ./pkg/client -run TestLiveTestnet
func TestLiveTestnet(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create testnet client
	c, err := client.NewTestnet()
	require.NoError(t, err)

	t.Run("GetAccount_ACME", func(t *testing.T) {
		account, err := c.GetAccount(ctx, "acc://ACME")
		// Testnet might not have ACME or might be down
		if err != nil {
			t.Skipf("Testnet unavailable or ACME not found: %v", err)
		}
		require.NotNil(t, account)
		require.NotNil(t, account.Account)
		t.Logf("Successfully queried ACME on testnet: %+v", account.Account)
	})
}
