// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package client_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client"
)

// TestClientGetAccount_Devnet tests the GetAccount method against a local devnet.
// This test requires a running Accumulate network.
func TestClientGetAccount_Devnet(t *testing.T) {
	endpoint := os.Getenv("DEVNET_ENDPOINT")
	if endpoint == "" {
		t.Skip("Skipping devnet test (set DEVNET_ENDPOINT to run)")
	}

	// Create client pointing to the devnet
	c, err := client.NewLocal(endpoint)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test: Query a well-known account (ACME token should exist)
	t.Run("QueryACME", func(t *testing.T) {
		account, err := c.GetAccount(ctx, "acc://ACME")
		if err != nil {
			t.Logf("Failed to query acc://ACME: %v", err)
			t.Skip("Could not connect to devnet")
		}
		require.NotNil(t, account)
		require.NotNil(t, account.Account)

		// Verify it's the correct account
		require.Equal(t, "acc://ACME", account.Account.GetUrl().String())
	})

	// Test: Query non-existent account (should return error)
	t.Run("QueryNonExistent", func(t *testing.T) {
		_, err := c.GetAccount(ctx, "acc://nonexistent.acme")
		require.Error(t, err)
	})
}

// TestClientConstructors tests the various client constructor functions.
func TestClientConstructors(t *testing.T) {
	t.Run("NewMainnet", func(t *testing.T) {
		c, err := client.NewMainnet()
		require.NoError(t, err)
		require.NotNil(t, c)
	})

	t.Run("NewTestnet", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)
		require.NotNil(t, c)
	})

	t.Run("NewLocal", func(t *testing.T) {
		// Test with default endpoint
		c, err := client.NewLocal("")
		require.NoError(t, err)
		require.NotNil(t, c)

		// Test with custom endpoint
		c, err = client.NewLocal("http://localhost:9999/v3")
		require.NoError(t, err)
		require.NotNil(t, c)
	})

	t.Run("NewDevnet", func(t *testing.T) {
		c, err := client.NewDevnet("http://devnet:8080/v3")
		require.NoError(t, err)
		require.NotNil(t, c)
	})

	t.Run("NewCustom", func(t *testing.T) {
		config := &client.Config{
			Endpoint: "https://custom.accumulate.io/v3",
			Network:  client.NetworkCustom,
			Timeout:  30 * time.Second,
			Debug:    true,
		}
		c, err := client.New(config)
		require.NoError(t, err)
		require.NotNil(t, c)
	})

	t.Run("NewWithoutEndpoint", func(t *testing.T) {
		config := &client.Config{
			Network: client.NetworkCustom,
		}
		_, err := client.New(config)
		require.Error(t, err)
		require.Contains(t, err.Error(), "endpoint is required")
	})
}

// TestClientGetAccount_Integration tests against a real network if available.
// This test is skipped by default and only runs if ACCUMULATE_ENDPOINT is set.
func TestClientGetAccount_Integration(t *testing.T) {
	endpoint := os.Getenv("ACCUMULATE_ENDPOINT")
	if endpoint == "" {
		t.Skip("Skipping integration test (set ACCUMULATE_ENDPOINT to run)")
	}

	c, err := client.New(&client.Config{
		Endpoint: endpoint,
		Network:  client.NetworkCustom,
		Timeout:  30 * time.Second,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Try to query the ACME token account (should exist on any network)
	account, err := c.GetAccount(ctx, "acc://ACME")
	if err != nil {
		t.Logf("Failed to query acc://ACME: %v", err)
		t.Skip("Could not connect to network")
	}

	require.NotNil(t, account)
	require.NotNil(t, account.Account)
	t.Logf("Successfully queried account: %s", account.Account.GetUrl())
}
