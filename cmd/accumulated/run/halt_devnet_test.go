// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !ci
// +build !ci

// These tests start real DevNet instances and test the halt functionality
// end-to-end. They are excluded from CI builds due to Prometheus metrics
// conflicts when running multiple CometBFT instances in the same process.
//
// Run these tests individually:
//   go test -run TestHaltDevNetIntegration -v
//   go test -run TestHaltDevNetCancel -v
//   go test -run TestHaltDevNetAPIResponses -v

package run

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestHaltDevNetIntegration is a full end-to-end integration test that:
// 1. Starts a DevNet with a fast major block schedule (every minute)
// 2. Calls the /admin/halt API
// 3. Waits for the node to shut down after a major block
//
// This test takes up to 90 seconds to run as it waits for real wall clock
// time to reach the next major block boundary.
func TestHaltDevNetIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping DevNet integration test in short mode")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a temporary directory for the devnet
	rootDir := t.TempDir()

	// Configure DevNet with a fast major block schedule (every minute)
	globals := &network.GlobalValues{
		Globals: &protocol.NetworkGlobals{
			MajorBlockSchedule: "* * * * *", // Every minute at second 0
		},
	}

	cfg := &Config{
		Network: "TestDevNet",
		Logging: &Logging{
			Format: "plain",
			Rules: []*LoggingRule{
				{Level: slog.LevelInfo},
				{Level: slog.LevelError, Modules: []string{"badger", "accumulate", "anchoring"}},
				{Level: slog.LevelError, Modules: []string{"p2p", "abci-client", "pubsub", "txindex", "consensus", "mempool", "blocksync", "rpc-server", "evidence", "statesync", "pex", "proxy", "events", "state"}},
			},
		},
		P2P: &P2P{
			Key: &PrivateKeySeed{Seed: record.NewKey("test-halt-devnet")},
		},
		Configurations: []Configuration{
			&DevnetConfiguration{
				Listen:     multiaddr.StringCast("/tcp/36656"), // Use non-standard port to avoid conflicts
				Bvns:       1,
				Validators: 1,
				Globals:    globals,
			},
		},
	}

	cfg.file = rootDir + "/accumulate.toml"

	// Start the instance
	ctx = logging.With(ctx, "test", t.Name())
	inst, err := New(ctx, cfg)
	require.NoError(t, err)
	inst.rootDir = rootDir

	err = inst.Start()
	require.NoError(t, err)

	// Find the HTTP port - it should be at the default offset from the listen port
	// DevNet bootstrap node listens on port + 4 for HTTP
	httpAddr := "http://127.0.0.1:36660"

	// Wait for HTTP server to be ready
	t.Log("Waiting for HTTP server to be ready...")
	var httpReady bool
	for i := 0; i < 30; i++ {
		resp, err := http.Get(httpAddr + "/admin/halt")
		if err == nil {
			resp.Body.Close()
			httpReady = true
			break
		}
		time.Sleep(time.Second)
	}
	require.True(t, httpReady, "HTTP server did not become ready")
	t.Log("HTTP server is ready")

	// Check initial halt status
	resp, err := http.Get(httpAddr + "/admin/halt")
	require.NoError(t, err)
	var status HaltStatus
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&status))
	resp.Body.Close()
	require.False(t, status.Pending, "Halt should not be pending initially")

	// Request halt
	t.Log("Requesting halt...")
	resp, err = http.Post(httpAddr+"/admin/halt", "application/json", nil)
	require.NoError(t, err)
	var haltResp HaltResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&haltResp))
	resp.Body.Close()
	require.Equal(t, "scheduled", haltResp.Status)
	t.Log("Halt scheduled, waiting for major block...")

	// Verify halt is pending
	resp, err = http.Get(httpAddr + "/admin/halt")
	require.NoError(t, err)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&status))
	resp.Body.Close()
	require.True(t, status.Pending, "Halt should be pending after request")
	t.Logf("Halt pending for partitions: %v", status.Partitions)

	// Calculate when the next major block should occur
	now := time.Now()
	nextMinute := now.Truncate(time.Minute).Add(time.Minute)
	waitTime := time.Until(nextMinute) + 10*time.Second // Add buffer for block processing
	t.Logf("Next major block expected around %s (waiting up to %s)", nextMinute.Format(time.RFC3339), waitTime)

	// Wait for the instance to shut down
	select {
	case <-inst.Done():
		t.Log("Instance shut down successfully after major block")
	case <-time.After(waitTime):
		// Check if instance is still running
		select {
		case <-inst.Done():
			t.Log("Instance shut down (detected on timeout check)")
		default:
			t.Fatal("Instance did not shut down after major block")
		}
	}

	// Give the HTTP server a moment to fully stop
	time.Sleep(500 * time.Millisecond)

	// Verify HTTP is no longer responding (node has stopped)
	client := &http.Client{Timeout: time.Second}
	_, err = client.Get(httpAddr + "/admin/halt")
	require.Error(t, err, "HTTP should not be responding after shutdown")
}

// TestHaltDevNetCancel tests that canceling a halt request works correctly.
func TestHaltDevNetCancel(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping DevNet integration test in short mode")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rootDir := t.TempDir()

	// Configure DevNet - use a schedule far in the future so we can test cancel
	globals := &network.GlobalValues{
		Globals: &protocol.NetworkGlobals{
			MajorBlockSchedule: "0 0 1 1 *", // Once a year - effectively never during test
		},
	}

	cfg := &Config{
		Network: "TestDevNet",
		Logging: &Logging{
			Format: "plain",
			Rules: []*LoggingRule{
				{Level: slog.LevelError}, // Quiet logging
			},
		},
		P2P: &P2P{
			Key: &PrivateKeySeed{Seed: record.NewKey("test-halt-cancel")},
		},
		Configurations: []Configuration{
			&DevnetConfiguration{
				Listen:     multiaddr.StringCast("/tcp/37656"),
				Bvns:       1,
				Validators: 1,
				Globals:    globals,
			},
		},
	}

	cfg.file = rootDir + "/accumulate.toml"

	ctx = logging.With(ctx, "test", t.Name())
	inst, err := New(ctx, cfg)
	require.NoError(t, err)
	inst.rootDir = rootDir

	err = inst.Start()
	require.NoError(t, err)
	defer inst.Stop()

	httpAddr := "http://127.0.0.1:37660"

	// Wait for HTTP server
	var httpReady bool
	for i := 0; i < 30; i++ {
		resp, err := http.Get(httpAddr + "/admin/halt")
		if err == nil {
			resp.Body.Close()
			httpReady = true
			break
		}
		time.Sleep(time.Second)
	}
	require.True(t, httpReady)

	// Request halt
	resp, err := http.Post(httpAddr+"/admin/halt", "application/json", nil)
	require.NoError(t, err)
	resp.Body.Close()

	// Verify halt is pending
	resp, err = http.Get(httpAddr + "/admin/halt")
	require.NoError(t, err)
	var status HaltStatus
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&status))
	resp.Body.Close()
	require.True(t, status.Pending)

	// Cancel halt
	req, err := http.NewRequest(http.MethodDelete, httpAddr+"/admin/halt", nil)
	require.NoError(t, err)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	var cancelResp HaltResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&cancelResp))
	resp.Body.Close()
	require.Equal(t, "cancelled", cancelResp.Status)

	// Verify halt is no longer pending
	resp, err = http.Get(httpAddr + "/admin/halt")
	require.NoError(t, err)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&status))
	resp.Body.Close()
	require.False(t, status.Pending, "Halt should not be pending after cancel")

	// Verify instance is still running
	select {
	case <-inst.Done():
		t.Fatal("Instance should still be running after cancel")
	default:
		// Good - instance is still running
	}
}

// TestHaltDevNetAPIResponses tests the API response formats.
func TestHaltDevNetAPIResponses(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping DevNet integration test in short mode")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rootDir := t.TempDir()

	globals := &network.GlobalValues{
		Globals: &protocol.NetworkGlobals{
			MajorBlockSchedule: "0 0 1 1 *", // Never during test
		},
	}

	cfg := &Config{
		Network: "TestDevNet",
		Logging: &Logging{
			Format: "plain",
			Rules:  []*LoggingRule{{Level: slog.LevelError}},
		},
		P2P: &P2P{
			Key: &PrivateKeySeed{Seed: record.NewKey("test-halt-api")},
		},
		Configurations: []Configuration{
			&DevnetConfiguration{
				Listen:     multiaddr.StringCast("/tcp/38656"),
				Bvns:       1,
				Validators: 1,
				Globals:    globals,
			},
		},
	}

	cfg.file = rootDir + "/accumulate.toml"

	ctx = logging.With(ctx, "test", t.Name())
	inst, err := New(ctx, cfg)
	require.NoError(t, err)
	inst.rootDir = rootDir

	err = inst.Start()
	require.NoError(t, err)
	defer inst.Stop()

	httpAddr := "http://127.0.0.1:38660"

	// Wait for HTTP server
	var httpReady bool
	for i := 0; i < 30; i++ {
		if resp, err := http.Get(httpAddr + "/admin/halt"); err == nil {
			resp.Body.Close()
			httpReady = true
			break
		}
		time.Sleep(time.Second)
	}
	require.True(t, httpReady)

	t.Run("GET returns status", func(t *testing.T) {
		resp, err := http.Get(httpAddr + "/admin/halt")
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "application/json", resp.Header.Get("Content-Type"))

		var status HaltStatus
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&status))
		require.False(t, status.Pending)
	})

	t.Run("POST schedules halt", func(t *testing.T) {
		resp, err := http.Post(httpAddr+"/admin/halt", "application/json", nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var haltResp HaltResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&haltResp))
		require.Equal(t, "scheduled", haltResp.Status)
		require.Contains(t, haltResp.Message, "major block")
	})

	t.Run("GET shows pending after POST", func(t *testing.T) {
		resp, err := http.Get(httpAddr + "/admin/halt")
		require.NoError(t, err)
		defer resp.Body.Close()

		var status HaltStatus
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&status))
		require.True(t, status.Pending)
		require.NotEmpty(t, status.Partitions)
		// Should have Directory and BVN1
		require.GreaterOrEqual(t, len(status.Partitions), 2)
	})

	t.Run("DELETE cancels halt", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodDelete, httpAddr+"/admin/halt", nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var cancelResp HaltResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&cancelResp))
		require.Equal(t, "cancelled", cancelResp.Status)
	})

	t.Run("Invalid method returns 405", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPut, httpAddr+"/admin/halt", nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	})
}

func init() {
	// Suppress noisy logging during tests
	fmt.Println("DevNet integration tests loaded")
}
