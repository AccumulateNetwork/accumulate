// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build integration

package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNativeFollowerIntegration runs a full integration test of the native follower tools.
// This test requires:
// - An accumulated binary in PATH or the project root
// - Restored follower data at the test work directory (or it will be skipped)
//
// Run with: go test -tags=integration -v -run TestNativeFollowerIntegration
func TestNativeFollowerIntegration(t *testing.T) {
	// Check if we have required resources
	workDir := os.Getenv("FOLLOWER_TEST_WORKDIR")
	if workDir == "" {
		workDir = "/home/paul/.accumulate/mainnet-follower"
	}

	// Skip if work directory doesn't exist
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		t.Skipf("Skipping: work directory %s does not exist", workDir)
	}

	// Check for dnn and bvnn subdirectories
	dnnPath := filepath.Join(workDir, "dnn")
	bvnnPath := filepath.Join(workDir, "bvnn")
	if _, err := os.Stat(dnnPath); os.IsNotExist(err) {
		t.Skipf("Skipping: dnn directory %s does not exist", dnnPath)
	}
	if _, err := os.Stat(bvnnPath); os.IsNotExist(err) {
		t.Skipf("Skipping: bvnn directory %s does not exist", bvnnPath)
	}

	// Ensure no follower is running before we start
	cleanupFollower(t)

	// Create server for testing
	s := newTestServer()

	// Run subtests in order
	t.Run("01_StopAnyExisting", func(t *testing.T) {
		testStopFollowerNative(t, s)
	})

	t.Run("02_StatusWhenStopped", func(t *testing.T) {
		testStatusWhenStopped(t, s)
	})

	t.Run("03_StartFollower", func(t *testing.T) {
		testStartFollowerNative(t, s, workDir)
	})

	t.Run("04_StatusWhenRunning", func(t *testing.T) {
		testStatusWhenRunning(t, s)
	})

	t.Run("05_StatusWithAPICheck", func(t *testing.T) {
		testStatusWithAPICheck(t, s)
	})

	t.Run("06_StopFollower", func(t *testing.T) {
		testStopFollowerNative(t, s)
	})

	t.Run("07_VerifyStopped", func(t *testing.T) {
		testVerifyStopped(t, s)
	})

	// Final cleanup
	cleanupFollower(t)
}

func cleanupFollower(t *testing.T) {
	t.Helper()
	// Kill any accumulated processes
	exec.Command("pkill", "-9", "accumulated").Run()
	time.Sleep(time.Second)
}

func testStopFollowerNative(t *testing.T, s *Server) {
	args := map[string]interface{}{}
	result, err := s.stopFollowerNative(args)
	if err != nil {
		t.Fatalf("stopFollowerNative failed: %v", err)
	}

	status, ok := result["status"].(string)
	if !ok {
		t.Fatal("expected status in result")
	}

	// Status should be either "stopped" or "not_running"
	if status != "stopped" && status != "not_running" {
		t.Errorf("unexpected status: %s", status)
	}

	t.Logf("Stop result: status=%s", status)
}

func testStatusWhenStopped(t *testing.T, s *Server) {
	args := map[string]interface{}{}
	result, err := s.statusFollowerNative(args)
	if err != nil {
		t.Fatalf("statusFollowerNative failed: %v", err)
	}

	running, ok := result["running"].(bool)
	if !ok {
		t.Fatal("expected running field in result")
	}

	if running {
		t.Error("expected follower to not be running")
	}

	t.Logf("Status when stopped: running=%v", running)
}

func testStartFollowerNative(t *testing.T, s *Server, workDir string) {
	args := map[string]interface{}{
		"work_dir": workDir,
		"log_file": "/tmp/follower-integration-test.log",
	}

	result, err := s.startFollowerNative(args)
	if err != nil {
		t.Fatalf("startFollowerNative failed: %v", err)
	}

	status, ok := result["status"].(string)
	if !ok {
		t.Fatal("expected status in result")
	}

	if status != "started" {
		t.Errorf("expected status 'started', got '%s'", status)
	}

	pid, _ := result["pid"].(string)
	t.Logf("Start result: status=%s, pid=%s", status, pid)

	// Give it time to start up
	t.Log("Waiting 15 seconds for follower to initialize...")
	time.Sleep(15 * time.Second)
}

func testStatusWhenRunning(t *testing.T, s *Server) {
	args := map[string]interface{}{}
	result, err := s.statusFollowerNative(args)
	if err != nil {
		t.Fatalf("statusFollowerNative failed: %v", err)
	}

	running, ok := result["running"].(bool)
	if !ok {
		t.Fatal("expected running field in result")
	}

	if !running {
		t.Error("expected follower to be running")
	}

	process, _ := result["process"].(string)
	t.Logf("Status when running: running=%v, process=%s", running, process)
}

func testStatusWithAPICheck(t *testing.T, s *Server) {
	// Wait a bit more for APIs to be ready
	t.Log("Waiting 10 more seconds for APIs to be ready...")
	time.Sleep(10 * time.Second)

	args := map[string]interface{}{}
	result, err := s.statusFollowerNative(args)
	if err != nil {
		t.Fatalf("statusFollowerNative failed: %v", err)
	}

	// Check DN status
	dnStatus, ok := result["dn_status"].(map[string]interface{})
	if !ok {
		t.Log("DN status not available as map, checking raw result")
		t.Logf("Full result: %+v", result)
	} else {
		if dnOk, ok := dnStatus["ok"].(bool); ok && dnOk {
			t.Log("DN API is responding")
			if height, ok := dnStatus["dnHeight"].(float64); ok {
				t.Logf("DN height: %.0f", height)
			}
		} else {
			t.Log("DN API not ready yet (this may be normal during startup)")
		}
	}

	// Check BVN status
	bvnStatus, ok := result["bvn_status"].(map[string]interface{})
	if ok {
		if bvnOk, ok := bvnStatus["ok"].(bool); ok && bvnOk {
			t.Log("BVN API is responding")
		} else {
			t.Log("BVN API not ready yet (this may be normal during startup)")
		}
	}
}

func testVerifyStopped(t *testing.T, s *Server) {
	// Give some time for process to fully stop
	time.Sleep(2 * time.Second)

	args := map[string]interface{}{}
	result, err := s.statusFollowerNative(args)
	if err != nil {
		t.Fatalf("statusFollowerNative failed: %v", err)
	}

	running, ok := result["running"].(bool)
	if !ok {
		t.Fatal("expected running field in result")
	}

	if running {
		t.Error("expected follower to be stopped")
	}

	t.Logf("Verified stopped: running=%v", running)
}

// TestNativeFollowerToolDefinitions verifies that all native follower tools are registered
func TestNativeFollowerToolDefinitions(t *testing.T) {
	tools := GetAllTools()

	expectedTools := []string{
		"accumulate_start_follower_native",
		"accumulate_stop_follower_native",
		"accumulate_status_follower_native",
		"accumulate_deploy_follower_native",
	}

	toolMap := make(map[string]bool)
	for _, tool := range tools {
		if name, ok := tool["name"].(string); ok {
			toolMap[name] = true
		}
	}

	for _, expected := range expectedTools {
		if !toolMap[expected] {
			t.Errorf("missing tool definition: %s", expected)
		}
	}
}

// TestNativeFollowerToolRouting verifies that tool routing works
func TestNativeFollowerToolRouting(t *testing.T) {
	s := newTestServer()

	// Test that executeTool routes to the correct handler
	testCases := []struct {
		name        string
		expectError bool
		errorMsg    string
	}{
		{"accumulate_start_follower_native", true, "work_dir is required"},
		{"accumulate_stop_follower_native", false, ""},
		{"accumulate_status_follower_native", false, ""},
		{"accumulate_deploy_follower_native", true, "dn_snapshot is required"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := s.executeTool(tc.name, map[string]interface{}{})

			if tc.expectError {
				if err == nil {
					t.Errorf("expected error containing '%s'", tc.errorMsg)
				} else if !strings.Contains(err.Error(), tc.errorMsg) {
					t.Errorf("expected error containing '%s', got '%s'", tc.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result == nil {
					t.Error("expected non-nil result")
				}
			}
		})
	}
}
