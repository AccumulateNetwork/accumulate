// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package client_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client"
)

// TestGetNodeInfo tests the GetNodeInfo method
func TestGetNodeInfo(t *testing.T) {
	t.Run("ValidCall", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)

		ctx := context.Background()
		// This will fail with network error but validates the method exists
		_, err = c.GetNodeInfo(ctx)
		// Just verify it doesn't panic
	})

	t.Run("WithTimeout", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		_, err = c.GetNodeInfo(ctx)
		// Will timeout or fail, just verify no panic
	})
}

// TestGetNetworkStatus tests the GetNetworkStatus method
func TestGetNetworkStatus(t *testing.T) {
	t.Run("ValidCall", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)

		ctx := context.Background()
		_, err = c.GetNetworkStatus(ctx)
		// Will fail with network error but validates the method exists
	})

	t.Run("WithCancelledContext", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err = c.GetNetworkStatus(ctx)
		require.Error(t, err)
	})
}

// TestGetConsensusStatus tests the GetConsensusStatus method
func TestGetConsensusStatus(t *testing.T) {
	t.Run("ValidCall", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)

		ctx := context.Background()
		_, err = c.GetConsensusStatus(ctx)
		// May fail with "not available" or network error
	})
}

// TestGetMetrics tests the GetMetrics method
func TestGetMetrics(t *testing.T) {
	t.Run("ValidCall", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)

		ctx := context.Background()
		_, err = c.GetMetrics(ctx, "Directory")
		// Will fail with network error but validates the method exists
	})

	t.Run("DifferentPartitions", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)

		ctx := context.Background()

		partitions := []string{
			"Directory",
			"BVN0",
			"BVN1",
			"BVN2",
			"", // Empty partition
		}

		for _, partition := range partitions {
			_, err = c.GetMetrics(ctx, partition)
			// Just verify no panic
		}
	})
}

// TestFindService tests the FindService method
func TestFindService(t *testing.T) {
	t.Run("NilService", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)

		ctx := context.Background()
		_, err = c.FindService(ctx, nil)
		// May error but shouldn't panic
	})
}

// TestListSnapshots tests the ListSnapshots method
func TestListSnapshots(t *testing.T) {
	t.Run("ValidCall", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)

		ctx := context.Background()
		_, err = c.ListSnapshots(ctx)
		// Will fail with network error but validates the method exists
	})
}

// TestNetworkConfigs tests the various network configurations
func TestNetworkConfigs(t *testing.T) {
	t.Run("MainnetConfig", func(t *testing.T) {
		c, err := client.NewMainnet()
		require.NoError(t, err)
		require.NotNil(t, c)
	})

	t.Run("TestnetConfig", func(t *testing.T) {
		c, err := client.NewTestnet()
		require.NoError(t, err)
		require.NotNil(t, c)
	})

	t.Run("LocalDefaultEndpoint", func(t *testing.T) {
		c, err := client.NewLocal("")
		require.NoError(t, err)
		require.NotNil(t, c)
	})

	t.Run("LocalCustomEndpoint", func(t *testing.T) {
		c, err := client.NewLocal("http://127.0.0.1:9090/v3")
		require.NoError(t, err)
		require.NotNil(t, c)
	})

	t.Run("DevnetConfig", func(t *testing.T) {
		c, err := client.NewDevnet("http://devnet.local:8080/v3")
		require.NoError(t, err)
		require.NotNil(t, c)
	})
}

// TestConfigOptions tests various configuration options
func TestConfigOptions(t *testing.T) {
	t.Run("ShortTimeout", func(t *testing.T) {
		config := &client.Config{
			Endpoint: "http://localhost:8080/v3",
			Network:  client.NetworkLocal,
			Timeout:  100 * time.Millisecond,
		}
		c, err := client.New(config)
		require.NoError(t, err)
		require.NotNil(t, c)
	})

	t.Run("LongTimeout", func(t *testing.T) {
		config := &client.Config{
			Endpoint: "http://localhost:8080/v3",
			Network:  client.NetworkLocal,
			Timeout:  10 * time.Minute,
		}
		c, err := client.New(config)
		require.NoError(t, err)
		require.NotNil(t, c)
	})

	t.Run("DebugEnabled", func(t *testing.T) {
		config := &client.Config{
			Endpoint: "http://localhost:8080/v3",
			Network:  client.NetworkLocal,
			Debug:    true,
		}
		c, err := client.New(config)
		require.NoError(t, err)
		require.NotNil(t, c)
	})

	t.Run("AllNetworkTypes", func(t *testing.T) {
		networks := []client.NetworkType{
			client.NetworkMainnet,
			client.NetworkTestnet,
			client.NetworkLocal,
			client.NetworkDevnet,
			client.NetworkCustom,
		}

		for _, network := range networks {
			config := &client.Config{
				Endpoint: "http://localhost:8080/v3",
				Network:  network,
			}
			c, err := client.New(config)
			require.NoError(t, err)
			require.NotNil(t, c)
		}
	})
}
