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
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestSmartDevnetConnection demonstrates the smart discovery system
func TestSmartDevnetConnection(t *testing.T) {
	// Skip if explicitly disabled
	if os.Getenv("SKIP_DEVNET_TESTS") == "true" {
		t.Skip("Skipping devnet test (SKIP_DEVNET_TESTS=true)")
	}

	// Use smart discovery to find endpoint
	finder := NewDevnetEndpointFinder()
	endpoint := finder.FindEndpoint(t)

	if endpoint == "" {
		// Try to start devnet if not running
		endpoint = GetOrStartDevnet(t)
	}

	if endpoint == "" {
		t.Fatal("Could not find or start devnet")
	}

	// Save discovery info for other tests
	_ = finder.SaveDiscoveryInfo(endpoint)

	t.Logf("✅ Successfully connected to devnet at: %s", endpoint)

	// Create client
	client := jsonrpc.NewClient(endpoint)
	client.Client.Timeout = 30 * time.Second
	ctx := context.Background()

	// Get network status
	status, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		t.Fatalf("Failed to get network status: %v", err)
	}

	t.Logf("Network: %s", status.Network.NetworkName)
	t.Logf("Partitions: %d", len(status.Network.Partitions))
	for _, p := range status.Network.Partitions {
		t.Logf("  - %s (%s)", p.ID, p.Type)
	}

	// Discover all partitions
	partitions, err := DiscoverPartitions(endpoint)
	if err != nil {
		t.Logf("Warning: Could not discover partitions: %v", err)
	} else {
		t.Logf("Discovered %d partitions: %v", len(partitions), partitions)
	}

	// Monitor endpoint health
	healthChan := MonitorEndpointHealth(endpoint, 5*time.Second)

	// Run a simple test transaction
	t.Log("Running test transaction...")
	runSimpleTransaction(t, client, healthChan)
}

// TestSmartDevnetMultiNode tests connecting to multiple nodes
func TestSmartDevnetMultiNode(t *testing.T) {
	if os.Getenv("SKIP_DEVNET_TESTS") == "true" {
		t.Skip("Skipping devnet test (SKIP_DEVNET_TESTS=true)")
	}

	finder := NewDevnetEndpointFinder()
	primaryEndpoint := finder.FindEndpoint(t)

	if primaryEndpoint == "" {
		t.Fatal("No devnet found")
	}

	// Try to find endpoints for different partitions
	partitions, err := DiscoverPartitions(primaryEndpoint)
	if err != nil {
		t.Fatalf("Failed to discover partitions: %v", err)
	}

	t.Logf("Testing connectivity to %d partitions", len(partitions))

	for _, partition := range partitions {
		endpoint, err := FindHealthyValidator(partition, primaryEndpoint)
		if err != nil {
			t.Logf("⚠️  Could not find validator for %s: %v", partition, err)
			continue
		}

		// Test the endpoint
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		client := jsonrpc.NewClient(endpoint)

		status, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
		cancel()

		if err != nil {
			t.Logf("❌ %s endpoint %s not responding: %v", partition, endpoint, err)
		} else {
			t.Logf("✅ %s endpoint %s is healthy (version: %d)",
				partition, endpoint, status.ExecutorVersion)
		}
	}
}

// runSimpleTransaction runs a simple test transaction
func runSimpleTransaction(t *testing.T, client *jsonrpc.Client, healthChan <-chan bool) {
	ctx := context.Background()

	// Check health before proceeding
	select {
	case healthy := <-healthChan:
		if !healthy {
			t.Log("⚠️  Endpoint health check failed, but continuing...")
		}
	default:
		// No health update yet
	}

	// Generate a test account
	seed := fmt.Sprintf("smart test seed %d", time.Now().Unix())
	hash := sha256.Sum256([]byte(seed))
	key := ed25519.NewKeyFromSeed(hash[:])

	liteURL, err := protocol.LiteTokenAddress(key[32:], "ACME", protocol.SignatureTypeED25519)
	if err != nil {
		t.Fatalf("Failed to create lite address: %v", err)
	}

	t.Logf("Test account: %s", liteURL)

	// Fund via faucet
	t.Log("Requesting funds from faucet...")
	submission, err := client.Faucet(ctx, liteURL, api.FaucetOptions{})
	if err != nil {
		t.Logf("Faucet error (this is expected if faucet is not configured): %v", err)
		return
	}

	if submission.Status != nil && submission.Status.Error != nil {
		t.Logf("Faucet returned error: %v", submission.Status.Error)
		return
	}

	t.Log("Faucet request submitted, waiting for balance...")

	// Wait for balance
	var balance *big.Int
	for i := 0; i < 10; i++ {
		time.Sleep(2 * time.Second)

		record, err := client.Query(ctx, liteURL, &api.DefaultQuery{})
		if err != nil {
			continue
		}

		if accRecord, ok := record.(*api.AccountRecord); ok {
			if tokenAccount, ok := accRecord.Account.(*protocol.LiteTokenAccount); ok {
				balance = &tokenAccount.Balance
				if balance.Cmp(big.NewInt(0)) > 0 {
					break
				}
			}
		}
	}

	if balance == nil || balance.Cmp(big.NewInt(0)) == 0 {
		t.Log("No balance received from faucet (this is okay for basic connectivity test)")
		return
	}

	balanceACME := new(big.Float).Quo(new(big.Float).SetInt(balance), big.NewFloat(1e8))
	t.Logf("✅ Account funded: %s ACME", balanceACME.String())

	// Create another account and send a small amount
	seed2 := fmt.Sprintf("smart test seed 2 %d", time.Now().Unix())
	hash2 := sha256.Sum256([]byte(seed2))
	key2 := ed25519.NewKeyFromSeed(hash2[:])

	liteURL2, _ := protocol.LiteTokenAddress(key2[32:], "ACME", protocol.SignatureTypeED25519)

	t.Logf("Sending 0.001 ACME to %s", liteURL2)

	// Build and send transaction
	env, err := build.Transaction().
		For(liteURL).
		SendTokens(big.NewInt(int64(0.001*1e8)), 0).To(liteURL2).
		SignWith(liteURL).Version(1).Timestamp(uint64(time.Now().UnixNano())).PrivateKey(key).
		Done()

	if err != nil {
		t.Fatalf("Failed to build transaction: %v", err)
	}

	submissions, err := client.Submit(ctx, env, api.SubmitOptions{})
	if err != nil {
		t.Logf("Transaction submission error: %v", err)
		return
	}

	for _, sub := range submissions {
		if sub.Status != nil && sub.Status.Error != nil {
			t.Logf("Transaction error: %v", sub.Status.Error)
		} else {
			t.Log("✅ Transaction submitted successfully")
		}
	}
}

// TestDevnetAutoRecovery tests automatic endpoint recovery
func TestDevnetAutoRecovery(t *testing.T) {
	if os.Getenv("SKIP_DEVNET_TESTS") == "true" {
		t.Skip("Skipping devnet test (SKIP_DEVNET_TESTS=true)")
	}

	finder := NewDevnetEndpointFinder()
	endpoint := finder.FindEndpoint(t)

	if endpoint == "" {
		t.Fatal("No devnet found")
	}

	// Monitor health with faster interval
	healthChan := MonitorEndpointHealth(endpoint, 1*time.Second)

	t.Log("Monitoring endpoint health for 10 seconds...")

	healthyCount := 0
	unhealthyCount := 0

	timeout := time.After(10 * time.Second)
	for {
		select {
		case healthy := <-healthChan:
			if healthy {
				healthyCount++
				t.Logf("✅ Endpoint healthy (check %d)", healthyCount)
			} else {
				unhealthyCount++
				t.Logf("❌ Endpoint unhealthy (check %d)", unhealthyCount)

				// Try to find alternative endpoint
				t.Log("Searching for alternative endpoint...")
				altEndpoint := finder.FindEndpoint(t)
				if altEndpoint != "" && altEndpoint != endpoint {
					t.Logf("Found alternative endpoint: %s", altEndpoint)
					endpoint = altEndpoint
					healthChan = MonitorEndpointHealth(endpoint, 1*time.Second)
				}
			}

		case <-timeout:
			t.Logf("Health monitoring complete: %d healthy, %d unhealthy checks",
				healthyCount, unhealthyCount)

			if healthyCount == 0 {
				t.Error("Endpoint was never healthy during monitoring period")
			} else if unhealthyCount > healthyCount {
				t.Error("Endpoint was unhealthy more often than healthy")
			} else {
				t.Logf("✅ Endpoint stability: %.1f%% uptime",
					float64(healthyCount)/float64(healthyCount+unhealthyCount)*100)
			}
			return
		}
	}
}
