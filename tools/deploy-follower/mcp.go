// MCP Tool Registration for deploy-follower
//
// This file provides MCP protocol integration for the deploy-follower tool.
// It can be imported and registered with an MCP server.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterTools registers all deploy-follower tools with an MCP server
func RegisterTools(s *server.MCPServer) {
	// Deploy follower tool
	s.AddTool(mcp.Tool{
		Name:        "deploy_local_follower",
		Description: "Deploy an Accumulate follower node locally from snapshots. This initializes the node directories, restores genesis state from snapshots, creates configuration files, and generates startup scripts. Does NOT use Docker.",
		InputSchema: mcp.ToolInputSchema{
			Type:     "object",
			Required: []string{"work_dir", "dn_snapshot", "bvn_snapshot", "accumulated_binary"},
			Properties: map[string]interface{}{
				"work_dir": map[string]interface{}{
					"type":        "string",
					"description": "Directory to store follower data. Will be created if it doesn't exist. Contains dnn/, bvnn/, logs/, and config files.",
				},
				"dn_snapshot": map[string]interface{}{
					"type":        "string",
					"description": "Path to Directory Network snapshot file (.snap). Used to initialize DN genesis state.",
				},
				"bvn_snapshot": map[string]interface{}{
					"type":        "string",
					"description": "Path to BVN snapshot file (.snap). Used to initialize BVN genesis state.",
				},
				"accumulated_binary": map[string]interface{}{
					"type":        "string",
					"description": "Path to the accumulated binary. Will be copied to work_dir.",
				},
				"monitor_binary": map[string]interface{}{
					"type":        "string",
					"description": "Optional: Path to follower-monitor binary. If provided, creates start-monitor.sh script.",
				},
				"network": map[string]interface{}{
					"type":        "string",
					"description": "Network name (default: mainnet)",
					"default":     "mainnet",
				},
				"bvn": map[string]interface{}{
					"type":        "string",
					"description": "BVN name (default: Cyclops)",
					"default":     "Cyclops",
				},
				"start": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, start the follower after deployment (default: false)",
					"default":     false,
				},
			},
		},
	}, handleDeployFollower)

	// Start follower tool
	s.AddTool(mcp.Tool{
		Name:        "start_local_follower",
		Description: "Start a previously deployed local follower. Runs the start.sh script in the work directory.",
		InputSchema: mcp.ToolInputSchema{
			Type:     "object",
			Required: []string{"work_dir"},
			Properties: map[string]interface{}{
				"work_dir": map[string]interface{}{
					"type":        "string",
					"description": "Path to the follower work directory (created by deploy_local_follower)",
				},
			},
		},
	}, handleStartFollower)

	// Stop follower tool
	s.AddTool(mcp.Tool{
		Name:        "stop_local_follower",
		Description: "Stop a running local follower gracefully. Sends SIGTERM and waits for clean shutdown.",
		InputSchema: mcp.ToolInputSchema{
			Type:     "object",
			Required: []string{"work_dir"},
			Properties: map[string]interface{}{
				"work_dir": map[string]interface{}{
					"type":        "string",
					"description": "Path to the follower work directory",
				},
				"timeout": map[string]interface{}{
					"type":        "integer",
					"description": "Seconds to wait for graceful shutdown before SIGKILL (default: 30)",
					"default":     30,
				},
			},
		},
	}, handleStopFollower)

	// Status tool
	s.AddTool(mcp.Tool{
		Name:        "local_follower_status",
		Description: "Get status of a local follower deployment including sync progress, peer count, and block heights.",
		InputSchema: mcp.ToolInputSchema{
			Type:     "object",
			Required: []string{"work_dir"},
			Properties: map[string]interface{}{
				"work_dir": map[string]interface{}{
					"type":        "string",
					"description": "Path to the follower work directory",
				},
			},
		},
	}, handleFollowerStatus)

	// Start monitor tool
	s.AddTool(mcp.Tool{
		Name:        "start_local_monitor",
		Description: "Start the follower-monitor web UI for real-time sync status visualization.",
		InputSchema: mcp.ToolInputSchema{
			Type:     "object",
			Required: []string{"work_dir"},
			Properties: map[string]interface{}{
				"work_dir": map[string]interface{}{
					"type":        "string",
					"description": "Path to the follower work directory",
				},
				"monitor_binary": map[string]interface{}{
					"type":        "string",
					"description": "Path to follower-monitor binary (optional if already in work_dir or start-monitor.sh exists)",
				},
			},
		},
	}, handleStartMonitor)
}

// Tool Handlers

func handleDeployFollower(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments

	// Extract required parameters
	workDir, _ := args["work_dir"].(string)
	dnSnapshot, _ := args["dn_snapshot"].(string)
	bvnSnapshot, _ := args["bvn_snapshot"].(string)
	accBinary, _ := args["accumulated_binary"].(string)

	if workDir == "" || dnSnapshot == "" || bvnSnapshot == "" || accBinary == "" {
		return errorResult("missing required parameters: work_dir, dn_snapshot, bvn_snapshot, accumulated_binary")
	}

	// Optional parameters
	monitorBinary, _ := args["monitor_binary"].(string)
	network := getStringOr(args, "network", "mainnet")
	bvn := getStringOr(args, "bvn", "Cyclops")
	startAfter, _ := args["start"].(bool)

	// Validate files exist
	for name, path := range map[string]string{
		"dn_snapshot":        dnSnapshot,
		"bvn_snapshot":       bvnSnapshot,
		"accumulated_binary": accBinary,
	} {
		if _, err := os.Stat(path); err != nil {
			return errorResult(fmt.Sprintf("%s not found: %s", name, path))
		}
	}

	result := map[string]interface{}{
		"work_dir": workDir,
		"network":  network,
		"bvn":      bvn,
		"steps":    []string{},
	}
	steps := []string{}

	// Create directories
	dnnDir := filepath.Join(workDir, "dnn")
	bvnnDir := filepath.Join(workDir, "bvnn")
	logsDir := filepath.Join(workDir, "logs")

	for _, dir := range []string{dnnDir, bvnnDir, logsDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return errorResult(fmt.Sprintf("failed to create directory %s: %v", dir, err))
		}
	}
	steps = append(steps, "Created directory structure")

	// Copy accumulated binary
	accDest := filepath.Join(workDir, "accumulated")
	if err := copyFileMCP(accBinary, accDest); err != nil {
		return errorResult(fmt.Sprintf("failed to copy accumulated: %v", err))
	}
	os.Chmod(accDest, 0755)
	steps = append(steps, "Copied accumulated binary")

	// Copy monitor if provided
	if monitorBinary != "" {
		if _, err := os.Stat(monitorBinary); err == nil {
			monDest := filepath.Join(workDir, "follower-monitor")
			if err := copyFileMCP(monitorBinary, monDest); err != nil {
				return errorResult(fmt.Sprintf("failed to copy monitor: %v", err))
			}
			os.Chmod(monDest, 0755)
			steps = append(steps, "Copied follower-monitor binary")
		}
	}

	// Initialize DN
	logFile := filepath.Join(logsDir, "init-dn.log")
	cmd := exec.CommandContext(ctx, accDest, "--work-dir", dnnDir, "restore-genesis", dnSnapshot, "--network", network)
	output, err := cmd.CombinedOutput()
	os.WriteFile(logFile, output, 0644)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to restore DN genesis: %v (see %s)", err, logFile))
	}
	steps = append(steps, "Initialized DN from snapshot")

	// Initialize BVN
	logFile = filepath.Join(logsDir, "init-bvn.log")
	cmd = exec.CommandContext(ctx, accDest, "--work-dir", bvnnDir, "restore-genesis", bvnSnapshot, "--network", network)
	output, err = cmd.CombinedOutput()
	os.WriteFile(logFile, output, 0644)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to restore BVN genesis: %v (see %s)", err, logFile))
	}
	steps = append(steps, "Initialized BVN from snapshot")

	// Create config
	configContent := fmt.Sprintf(`network = "%s"

[[configurations]]
  type = "follower"
  mode = "dual"
  bvn = "%s"
  listen = "/ip4/0.0.0.0/tcp/16591"
  storage-type = "leveldb"
  enable-healing = false
  enable-snapshots = false

  dn-bootstrap-peers = [
    "/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx"
  ]

  bvn-bootstrap-peers = [
    "/dns/bootstrap.accumulate.defidevs.io/tcp/16693/p2p/12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx"
  ]

[logging]
  format = "plain"
  [[logging.rules]]
    level = "info"
`, network, bvn)

	if err := os.WriteFile(filepath.Join(workDir, "accumulate.toml"), []byte(configContent), 0644); err != nil {
		return errorResult(fmt.Sprintf("failed to write config: %v", err))
	}
	steps = append(steps, "Created accumulate.toml")

	// Create scripts
	if err := createStartScript(workDir); err != nil {
		return errorResult(fmt.Sprintf("failed to create start script: %v", err))
	}
	steps = append(steps, "Created start.sh")

	if err := createStopScript(workDir); err != nil {
		return errorResult(fmt.Sprintf("failed to create stop script: %v", err))
	}
	steps = append(steps, "Created stop.sh")

	if monitorBinary != "" {
		if err := createMonitorScript(workDir); err != nil {
			return errorResult(fmt.Sprintf("failed to create monitor script: %v", err))
		}
		steps = append(steps, "Created start-monitor.sh")
		result["monitor_url"] = "http://localhost:8080"
	}

	result["steps"] = steps
	result["status"] = "deployed"
	result["message"] = "Follower deployed successfully"
	result["next_steps"] = []string{
		fmt.Sprintf("cd %s && ./start.sh", workDir),
		"./start-monitor.sh  # if monitor was provided",
	}

	// Start if requested
	if startAfter {
		if err := runStartScript(ctx, workDir); err != nil {
			result["start_error"] = err.Error()
		} else {
			result["started"] = true
		}
	}

	return successResult(result)
}

func handleStartFollower(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	workDir, _ := request.Params.Arguments["work_dir"].(string)
	if workDir == "" {
		return errorResult("missing required parameter: work_dir")
	}

	// Check if already running
	if pid := getPIDFromFile(filepath.Join(workDir, "follower.pid")); pid > 0 && isProcessRunning(pid) {
		return successResult(map[string]interface{}{
			"status":  "already_running",
			"pid":     pid,
			"message": "Follower is already running",
		})
	}

	if err := runStartScript(ctx, workDir); err != nil {
		return errorResult(fmt.Sprintf("failed to start follower: %v", err))
	}

	pid := getPIDFromFile(filepath.Join(workDir, "follower.pid"))
	return successResult(map[string]interface{}{
		"status":  "started",
		"pid":     pid,
		"message": "Follower started successfully",
	})
}

func handleStopFollower(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	workDir, _ := request.Params.Arguments["work_dir"].(string)
	if workDir == "" {
		return errorResult("missing required parameter: work_dir")
	}

	timeout := 30
	if t, ok := request.Params.Arguments["timeout"].(float64); ok {
		timeout = int(t)
	}

	pidFile := filepath.Join(workDir, "follower.pid")
	pid := getPIDFromFile(pidFile)
	if pid <= 0 {
		return successResult(map[string]interface{}{
			"status":  "not_running",
			"message": "No PID file found",
		})
	}

	if !isProcessRunning(pid) {
		os.Remove(pidFile)
		return successResult(map[string]interface{}{
			"status":  "not_running",
			"message": "Process not running (stale PID file removed)",
		})
	}

	// Send SIGTERM
	proc, _ := os.FindProcess(pid)
	proc.Signal(syscall.SIGTERM)

	// Wait for graceful shutdown
	for i := 0; i < timeout*2; i++ {
		if !isProcessRunning(pid) {
			os.Remove(pidFile)
			return successResult(map[string]interface{}{
				"status":  "stopped",
				"message": "Follower stopped gracefully",
			})
		}
		select {
		case <-ctx.Done():
			return errorResult("context cancelled")
		default:
		}
		// Sleep 500ms without using time package
		exec.Command("sleep", "0.5").Run()
	}

	// Force kill
	proc.Signal(syscall.SIGKILL)
	os.Remove(pidFile)
	return successResult(map[string]interface{}{
		"status":  "force_killed",
		"message": "Follower force killed after timeout",
		"warning": "Force kill may cause data corruption",
	})
}

func handleFollowerStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	workDir, _ := request.Params.Arguments["work_dir"].(string)
	if workDir == "" {
		return errorResult("missing required parameter: work_dir")
	}

	result := map[string]interface{}{"work_dir": workDir}

	// Check deployment
	if _, err := os.Stat(filepath.Join(workDir, "accumulate.toml")); err != nil {
		result["deployed"] = false
		result["status"] = "not_deployed"
		return successResult(result)
	}
	result["deployed"] = true

	// Check follower
	pid := getPIDFromFile(filepath.Join(workDir, "follower.pid"))
	if pid > 0 && isProcessRunning(pid) {
		result["running"] = true
		result["pid"] = pid
		result["status"] = "running"

		// Get sync status via RPC
		result["dn"] = getRPCStatusMCP(16592)
		result["bvn"] = getRPCStatusMCP(16692)
	} else {
		result["running"] = false
		result["status"] = "stopped"
	}

	// Check monitor
	monPid := getPIDFromFile(filepath.Join(workDir, "monitor.pid"))
	if monPid > 0 && isProcessRunning(monPid) {
		result["monitor_running"] = true
		result["monitor_pid"] = monPid
		result["monitor_url"] = "http://localhost:8080"
	} else {
		result["monitor_running"] = false
	}

	return successResult(result)
}

func handleStartMonitor(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	workDir, _ := request.Params.Arguments["work_dir"].(string)
	if workDir == "" {
		return errorResult("missing required parameter: work_dir")
	}

	// Check if already running
	if pid := getPIDFromFile(filepath.Join(workDir, "monitor.pid")); pid > 0 && isProcessRunning(pid) {
		return successResult(map[string]interface{}{
			"status":  "already_running",
			"pid":     pid,
			"url":     "http://localhost:8080",
			"message": "Monitor is already running",
		})
	}

	// Try start-monitor.sh first
	monitorScript := filepath.Join(workDir, "start-monitor.sh")
	if _, err := os.Stat(monitorScript); err == nil {
		cmd := exec.CommandContext(ctx, "/bin/bash", monitorScript)
		cmd.Dir = workDir
		if output, err := cmd.CombinedOutput(); err != nil {
			return errorResult(fmt.Sprintf("failed to start monitor: %v\n%s", err, output))
		}

		pid := getPIDFromFile(filepath.Join(workDir, "monitor.pid"))
		return successResult(map[string]interface{}{
			"status":  "started",
			"pid":     pid,
			"url":     "http://localhost:8080",
			"message": "Monitor started",
		})
	}

	// Try monitor_binary parameter
	monitorBinary, _ := request.Params.Arguments["monitor_binary"].(string)
	if monitorBinary == "" {
		monitorBinary = filepath.Join(workDir, "follower-monitor")
	}

	if _, err := os.Stat(monitorBinary); err != nil {
		return errorResult("monitor binary not found and no start-monitor.sh script")
	}

	// Start monitor
	logsDir := filepath.Join(workDir, "logs")
	os.MkdirAll(logsDir, 0755)

	logFile, _ := os.Create(filepath.Join(logsDir, "monitor.log"))
	defer logFile.Close()

	cmd := exec.Command(monitorBinary)
	cmd.Dir = workDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		return errorResult(fmt.Sprintf("failed to start monitor: %v", err))
	}

	os.WriteFile(filepath.Join(workDir, "monitor.pid"), []byte(strconv.Itoa(cmd.Process.Pid)), 0644)

	return successResult(map[string]interface{}{
		"status":  "started",
		"pid":     cmd.Process.Pid,
		"url":     "http://localhost:8080",
		"message": "Monitor started",
	})
}

// Helper functions

func errorResult(msg string) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf(`{"error": "%s"}`, msg),
			},
		},
		IsError: true,
	}, nil
}

func successResult(data map[string]interface{}) (*mcp.CallToolResult, error) {
	jsonData, _ := json.MarshalIndent(data, "", "  ")
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: string(jsonData),
			},
		},
	}, nil
}

func getStringOr(args map[string]interface{}, key, def string) string {
	if v, ok := args[key].(string); ok && v != "" {
		return v
	}
	return def
}

func copyFileMCP(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func getPIDFromFile(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return pid
}

func isProcessRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func runStartScript(ctx context.Context, workDir string) error {
	cmd := exec.CommandContext(ctx, "/bin/bash", "start.sh")
	cmd.Dir = workDir
	return cmd.Run()
}

func createStartScript(workDir string) error {
	script := fmt.Sprintf(`#!/bin/bash
cd "%s"
mkdir -p logs
nohup ./accumulated run-dual dnn bvnn > logs/follower.log 2>&1 &
echo $! > follower.pid
echo "Follower started with PID: $(cat follower.pid)"
`, workDir)
	return os.WriteFile(filepath.Join(workDir, "start.sh"), []byte(script), 0755)
}

func createStopScript(workDir string) error {
	script := fmt.Sprintf(`#!/bin/bash
cd "%s"
for pidfile in follower.pid monitor.pid; do
  if [ -f "$pidfile" ]; then
    pid=$(cat "$pidfile")
    if kill -0 $pid 2>/dev/null; then
      kill -TERM $pid
      for i in {1..30}; do
        kill -0 $pid 2>/dev/null || break
        sleep 1
      done
      kill -9 $pid 2>/dev/null
    fi
    rm -f "$pidfile"
  fi
done
`, workDir)
	return os.WriteFile(filepath.Join(workDir, "stop.sh"), []byte(script), 0755)
}

func createMonitorScript(workDir string) error {
	script := fmt.Sprintf(`#!/bin/bash
cd "%s"
mkdir -p logs
nohup ./follower-monitor > logs/monitor.log 2>&1 &
echo $! > monitor.pid
echo "Monitor started: http://localhost:8080"
`, workDir)
	return os.WriteFile(filepath.Join(workDir, "start-monitor.sh"), []byte(script), 0755)
}

func getRPCStatusMCP(port int) map[string]interface{} {
	cmd := exec.Command("curl", "-s", fmt.Sprintf("http://localhost:%d/status", port))
	output, err := cmd.Output()
	if err != nil {
		return map[string]interface{}{"error": "unable to connect"}
	}

	result := map[string]interface{}{}
	s := string(output)

	// Extract height
	if idx := strings.Index(s, `"latest_block_height"`); idx >= 0 {
		sub := s[idx:]
		start := strings.Index(sub, `":"`)
		if start >= 0 {
			sub = sub[start+2:]
			end := strings.Index(sub, `"`)
			if end >= 0 {
				height, _ := strconv.ParseInt(sub[:end], 10, 64)
				result["height"] = height
			}
		}
	}

	result["catching_up"] = strings.Contains(s, `"catching_up":true`) || strings.Contains(s, `"catching_up": true`)
	return result
}
