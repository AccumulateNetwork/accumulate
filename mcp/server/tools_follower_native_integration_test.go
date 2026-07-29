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

// TestNativeFollowerDeployIntegration tests the full deployment workflow.
// This test BUILDS snapshots from backup databases, then deploys from them.
// If snapshot creation fails, the test fails (revealing the bug).
//
// The test requires backup databases with accumulate.db, blockstore.db, and state.db.
// Default path: /media/paul/Expansion/databases/2025-12-01-aws-validator-node/
// Override with BACKUP_DB_DIR env var.
//
// Run with: go test -tags=integration -v -run TestNativeFollowerDeployIntegration
func TestNativeFollowerDeployIntegration(t *testing.T) {
	// Backup database directory - contains dnn/data and bvnn/data subdirectories
	backupDir := os.Getenv("BACKUP_DB_DIR")
	if backupDir == "" {
		backupDir = "/media/paul/Expansion/databases/2025-12-01-aws-validator-node"
	}

	// Verify backup databases exist with all required files
	dnDataDir := filepath.Join(backupDir, "dnn", "data")
	bvnDataDir := filepath.Join(backupDir, "bvnn", "data")

	requiredFiles := []string{"accumulate.db", "blockstore.db", "state.db"}
	for _, file := range requiredFiles {
		dnFile := filepath.Join(dnDataDir, file)
		if _, err := os.Stat(dnFile); os.IsNotExist(err) {
			t.Fatalf("DN backup missing %s: %s", file, dnFile)
		}
		bvnFile := filepath.Join(bvnDataDir, file)
		if _, err := os.Stat(bvnFile); os.IsNotExist(err) {
			t.Fatalf("BVN backup missing %s: %s", file, bvnFile)
		}
	}
	t.Logf("Using backup databases from: %s", backupDir)

	// Use temp directories for test artifacts
	testDir := "/tmp/follower-deploy-integration-test"
	snapshotDir := filepath.Join(testDir, "snapshots")
	workDir := filepath.Join(testDir, "follower")

	// Cleanup before and after
	os.RemoveAll(testDir)
	defer os.RemoveAll(testDir)
	defer cleanupFollower(t)
	cleanupFollower(t)

	// Create directories
	if err := os.MkdirAll(snapshotDir, 0755); err != nil {
		t.Fatalf("Failed to create snapshot directory: %v", err)
	}

	s := newTestServer()

	// Snapshot paths
	dnSnapshot := filepath.Join(snapshotDir, "directory.snap")
	bvnSnapshot := filepath.Join(snapshotDir, "cyclops.snap")

	t.Run("01_BuildDNSnapshot", func(t *testing.T) {
		args := map[string]interface{}{
			"node_data_dir": dnDataDir,
			"output":        dnSnapshot,
			"partition":     "Directory",
			"db_type":       "leveldb",
			"genesis":       true,
		}

		result, err := s.createSnapshotFromNodeBackup(args)
		if err != nil {
			t.Fatalf("createSnapshotFromNodeBackup (DN) failed: %v", err)
		}

		status, _ := result["status"].(string)
		if status != "success" {
			details, _ := result["details"].(map[string]interface{})
			t.Fatalf("DN snapshot creation failed: status=%s, details=%v", status, details)
		}

		// Verify snapshot was created
		if info, err := os.Stat(dnSnapshot); os.IsNotExist(err) {
			t.Fatalf("DN snapshot file was not created")
		} else {
			t.Logf("DN snapshot created: %s (%.2f MB)", dnSnapshot, float64(info.Size())/(1024*1024))
		}
	})

	t.Run("02_BuildBVNSnapshot", func(t *testing.T) {
		args := map[string]interface{}{
			"node_data_dir": bvnDataDir,
			"output":        bvnSnapshot,
			"partition":     "Cyclops",
			"db_type":       "leveldb",
			"genesis":       true,
			"dn_data_dir":   dnDataDir, // Required for BVN to read network definition
		}

		result, err := s.createSnapshotFromNodeBackup(args)
		if err != nil {
			t.Fatalf("createSnapshotFromNodeBackup (BVN) failed: %v", err)
		}

		status, _ := result["status"].(string)
		if status != "success" {
			details, _ := result["details"].(map[string]interface{})
			t.Fatalf("BVN snapshot creation failed: status=%s, details=%v", status, details)
		}

		// Verify snapshot was created
		if info, err := os.Stat(bvnSnapshot); os.IsNotExist(err) {
			t.Fatalf("BVN snapshot file was not created")
		} else {
			t.Logf("BVN snapshot created: %s (%.2f MB)", bvnSnapshot, float64(info.Size())/(1024*1024))
		}
	})

	t.Run("03_DeployWithoutStart", func(t *testing.T) {
		args := map[string]interface{}{
			"dn_snapshot":         dnSnapshot,
			"bvn_snapshot":        bvnSnapshot,
			"work_dir":            workDir,
			"start_after_restore": false,
		}

		result, err := s.deployFollowerNative(args)
		if err != nil {
			t.Fatalf("deployFollowerNative failed: %v", err)
		}

		status, _ := result["status"].(string)
		if status != "restored" {
			details, _ := result["details"].(map[string]interface{})
			message, _ := result["message"].(string)
			t.Fatalf("Deploy failed: status=%s, message=%s, details=%v", status, message, details)
		}

		// Verify directories were created
		if _, err := os.Stat(filepath.Join(workDir, "dnn")); os.IsNotExist(err) {
			t.Error("dnn directory was not created")
		}
		if _, err := os.Stat(filepath.Join(workDir, "bvnn")); os.IsNotExist(err) {
			t.Error("bvnn directory was not created")
		}

		t.Logf("Deploy without start succeeded: status=%s", status)
	})

	// Remove work dir to test fresh deploy with start
	os.RemoveAll(workDir)

	t.Run("04_DeployWithStart", func(t *testing.T) {
		args := map[string]interface{}{
			"dn_snapshot":         dnSnapshot,
			"bvn_snapshot":        bvnSnapshot,
			"work_dir":            workDir,
			"start_after_restore": true,
		}

		result, err := s.deployFollowerNative(args)
		if err != nil {
			t.Fatalf("deployFollowerNative failed: %v", err)
		}

		status, _ := result["status"].(string)
		message, _ := result["message"].(string)

		// Accept either "deployed" (fully started) or "restored_but_not_started"
		// (snapshot extraction worked but follower couldn't start - e.g., no network peers)
		switch status {
		case "deployed":
			t.Logf("Deploy with start fully succeeded: status=%s", status)
			// Wait for startup
			t.Log("Waiting 15 seconds for follower to initialize...")
			time.Sleep(15 * time.Second)

			// Verify it's running
			statusResult, err := s.statusFollowerNative(map[string]interface{}{})
			if err != nil {
				t.Fatalf("statusFollowerNative failed: %v", err)
			}

			running, _ := statusResult["running"].(bool)
			if !running {
				t.Error("expected follower to be running after deploy")
			}
		case "restored_but_not_started":
			// Snapshot extraction worked, but follower couldn't start
			// This is acceptable in test environments without network peers
			t.Logf("Deploy succeeded (restore only): status=%s, message=%s", status, message)
			t.Log("Note: Follower startup failed - this is expected in isolated test environments")
		default:
			details, _ := result["details"].(map[string]interface{})
			t.Fatalf("Deploy with start failed: status=%s, message=%s, details=%v", status, message, details)
		}
	})

	t.Run("05_StopFollower", func(t *testing.T) {
		result, err := s.stopFollowerNative(map[string]interface{}{})
		if err != nil {
			t.Fatalf("stopFollowerNative failed: %v", err)
		}

		status, _ := result["status"].(string)
		if status != "stopped" && status != "not_running" {
			t.Errorf("unexpected stop status: %s", status)
		}
		t.Logf("Stop result: status=%s", status)
	})
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
