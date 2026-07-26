// deploy-follower - Deploy an Accumulate follower locally from snapshots
//
// Usage:
//   deploy-follower --work-dir /path/to/data \
//     --dn-snapshot /path/to/dn.snap \
//     --bvn-snapshot /path/to/bvn.snap \
//     --accumulated /path/to/accumulated \
//     [--monitor /path/to/follower-monitor] \
//     [--start]

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"gopkg.in/yaml.v3"
)

// Config represents the YAML configuration file structure
type Config struct {
	Network  string `yaml:"network"`
	BVN      string `yaml:"bvn"`
	Binaries struct {
		Accumulated     string `yaml:"accumulated"`
		FollowerMonitor string `yaml:"follower_monitor"`
		DeployTool      string `yaml:"deploy_tool"`
	} `yaml:"binaries"`
	Snapshots struct {
		Directory string `yaml:"directory"`
		Cyclops   string `yaml:"cyclops"`
		Date      string `yaml:"date"`
	} `yaml:"snapshots"`
	Deployment struct {
		WorkDir string `yaml:"work_dir"`
	} `yaml:"deployment"`
}

var (
	configFile   = flag.String("config", "", "Path to config.yaml file")
	artifactsDir = flag.String("artifacts-dir", "", "Path to artifacts directory (looks for config.yaml)")
	workDir      = flag.String("work-dir", "", "Directory to store follower data")
	dnSnapshot   = flag.String("dn-snapshot", "", "Path to Directory Network snapshot")
	bvnSnapshot  = flag.String("bvn-snapshot", "", "Path to BVN snapshot")
	accumulated  = flag.String("accumulated", "", "Path to accumulated binary")
	monitor      = flag.String("monitor", "", "Path to follower-monitor binary")
	network      = flag.String("network", "", "Network name (default: mainnet)")
	bvn          = flag.String("bvn", "", "BVN name (default: Cyclops)")
	startFlag    = flag.Bool("start", false, "Start the follower after deployment")
	statusFlag   = flag.Bool("status", false, "Show status of existing deployment")
	stopFlag     = flag.Bool("stop", false, "Stop a running follower")
)

func main() {
	flag.Parse()

	// Load config file if specified
	if err := loadConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if *statusFlag {
		showStatus()
		return
	}

	if *stopFlag {
		stopFollower()
		return
	}

	// Validate required flags
	if *workDir == "" || *dnSnapshot == "" || *bvnSnapshot == "" || *accumulated == "" {
		fmt.Println("Usage: deploy-follower [options]")
		fmt.Println()
		fmt.Println("Config file (use instead of individual paths):")
		fmt.Println("  --config        Path to config.yaml file")
		fmt.Println("  --artifacts-dir Path to artifacts directory (looks for config.yaml)")
		fmt.Println()
		fmt.Println("Required (or use config file):")
		fmt.Println("  --work-dir      Directory to store follower data")
		fmt.Println("  --dn-snapshot   Path to Directory Network snapshot")
		fmt.Println("  --bvn-snapshot  Path to BVN snapshot")
		fmt.Println("  --accumulated   Path to accumulated binary")
		fmt.Println()
		fmt.Println("Optional:")
		fmt.Println("  --monitor       Path to follower-monitor binary")
		fmt.Println("  --network       Network name (default: mainnet)")
		fmt.Println("  --bvn           BVN name (default: Cyclops)")
		fmt.Println("  --start         Start the follower after deployment")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  --status        Show status of existing deployment")
		fmt.Println("  --stop          Stop a running follower")
		fmt.Println()
		fmt.Println("Example with config:")
		fmt.Println("  deploy-follower --artifacts-dir /path/to/artifacts --work-dir /path/to/data --start")
		os.Exit(1)
	}

	// Validate files exist
	for name, path := range map[string]string{
		"dn-snapshot":  *dnSnapshot,
		"bvn-snapshot": *bvnSnapshot,
		"accumulated":  *accumulated,
	} {
		if _, err := os.Stat(path); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s not found: %s\n", name, path)
			os.Exit(1)
		}
	}

	if *monitor != "" {
		if _, err := os.Stat(*monitor); err != nil {
			fmt.Fprintf(os.Stderr, "Error: monitor not found: %s\n", *monitor)
			os.Exit(1)
		}
	}

	// Deploy
	if err := deploy(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if *startFlag {
		if err := start(); err != nil {
			fmt.Fprintf(os.Stderr, "Error starting follower: %v\n", err)
			os.Exit(1)
		}
	}
}

// loadConfig loads configuration from a YAML file if specified
func loadConfig() error {
	var configPath string

	// Determine config file path
	if *configFile != "" {
		configPath = *configFile
	} else if *artifactsDir != "" {
		configPath = filepath.Join(*artifactsDir, "config.yaml")
	} else {
		// No config specified, use defaults
		if *network == "" {
			*network = "mainnet"
		}
		if *bvn == "" {
			*bvn = "Cyclops"
		}
		return nil
	}

	// Read and parse config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	// Get the directory containing the config file for relative paths
	configDir := filepath.Dir(configPath)

	// Apply config values as defaults (flags override config)
	if *network == "" {
		if cfg.Network != "" {
			*network = cfg.Network
		} else {
			*network = "mainnet"
		}
	}

	if *bvn == "" {
		if cfg.BVN != "" {
			*bvn = cfg.BVN
		} else {
			*bvn = "Cyclops"
		}
	}

	if *workDir == "" && cfg.Deployment.WorkDir != "" {
		*workDir = cfg.Deployment.WorkDir
	}

	if *dnSnapshot == "" && cfg.Snapshots.Directory != "" {
		*dnSnapshot = resolvePath(configDir, cfg.Snapshots.Directory)
	}

	if *bvnSnapshot == "" && cfg.Snapshots.Cyclops != "" {
		*bvnSnapshot = resolvePath(configDir, cfg.Snapshots.Cyclops)
	}

	if *accumulated == "" && cfg.Binaries.Accumulated != "" {
		*accumulated = resolvePath(configDir, cfg.Binaries.Accumulated)
	}

	if *monitor == "" && cfg.Binaries.FollowerMonitor != "" {
		*monitor = resolvePath(configDir, cfg.Binaries.FollowerMonitor)
	}

	fmt.Printf("Loaded configuration from: %s\n", configPath)
	return nil
}

// resolvePath resolves a path relative to baseDir if it's not absolute
func resolvePath(baseDir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}

func deploy() error {
	fmt.Println("=== Accumulate Follower Deployment ===")
	fmt.Printf("Work directory: %s\n", *workDir)
	fmt.Printf("Network: %s\n", *network)
	fmt.Printf("BVN: %s\n", *bvn)
	fmt.Println()

	// Create directory structure
	dnnDir := filepath.Join(*workDir, "dnn")
	bvnnDir := filepath.Join(*workDir, "bvnn")
	logsDir := filepath.Join(*workDir, "logs")

	for _, dir := range []string{dnnDir, bvnnDir, logsDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}
	fmt.Println("✓ Created directory structure")

	// Copy accumulated binary to work directory
	accDest := filepath.Join(*workDir, "accumulated")
	if err := copyFile(*accumulated, accDest); err != nil {
		return fmt.Errorf("failed to copy accumulated: %w", err)
	}
	os.Chmod(accDest, 0755)
	fmt.Println("✓ Copied accumulated binary")

	// Copy monitor if provided
	if *monitor != "" {
		monDest := filepath.Join(*workDir, "follower-monitor")
		if err := copyFile(*monitor, monDest); err != nil {
			return fmt.Errorf("failed to copy monitor: %w", err)
		}
		os.Chmod(monDest, 0755)
		fmt.Println("✓ Copied follower-monitor binary")
	}

	// Initialize DN from snapshot
	fmt.Println("\nInitializing DN from snapshot...")
	logFile := filepath.Join(logsDir, "init-dn.log")
	cmd := exec.Command(accDest,
		"--work-dir", dnnDir,
		"restore-genesis", *dnSnapshot,
		"--network", *network,
		"--partition", "Directory",
	)
	output, err := cmd.CombinedOutput()
	os.WriteFile(logFile, output, 0644)
	if err != nil {
		fmt.Printf("Output: %s\n", string(output))
		return fmt.Errorf("failed to restore DN genesis: %w (see %s)", err, logFile)
	}
	fmt.Printf("✓ DN initialized (log: %s)\n", logFile)

	// Initialize BVN from snapshot
	fmt.Println("Initializing BVN from snapshot...")
	logFile = filepath.Join(logsDir, "init-bvn.log")
	cmd = exec.Command(accDest,
		"--work-dir", bvnnDir,
		"restore-genesis", *bvnSnapshot,
		"--network", *network,
		"--partition", *bvn,
	)
	output, err = cmd.CombinedOutput()
	os.WriteFile(logFile, output, 0644)
	if err != nil {
		fmt.Printf("Output: %s\n", string(output))
		return fmt.Errorf("failed to restore BVN genesis: %w (see %s)", err, logFile)
	}
	fmt.Printf("✓ BVN initialized (log: %s)\n", logFile)

	// Create configuration
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
`, *network, *bvn)

	configPath := filepath.Join(*workDir, "accumulate.toml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	fmt.Println("✓ Created accumulate.toml")

	// Create start script
	startScript := fmt.Sprintf(`#!/bin/bash
cd "%s"
mkdir -p logs

echo "Starting Accumulate follower..."
nohup ./accumulated run-dual dnn bvnn > logs/follower.log 2>&1 &
echo $! > follower.pid
echo "Follower started with PID: $(cat follower.pid)"

sleep 3
if kill -0 $(cat follower.pid) 2>/dev/null; then
    echo "✓ Follower is running"
    echo ""
    echo "To check DN status:"
    echo "  curl -s http://localhost:16592/status | jq '.result.sync_info'"
    echo ""
    echo "To check BVN status:"
    echo "  curl -s http://localhost:16692/status | jq '.result.sync_info'"
else
    echo "✗ Follower failed to start. Check logs/follower.log"
    exit 1
fi
`, *workDir)

	if err := os.WriteFile(filepath.Join(*workDir, "start.sh"), []byte(startScript), 0755); err != nil {
		return fmt.Errorf("failed to write start script: %w", err)
	}
	fmt.Println("✓ Created start.sh")

	// Create stop script
	stopScript := fmt.Sprintf(`#!/bin/bash
cd "%s"

stop_process() {
    local pidfile=$1
    local name=$2
    if [ -f "$pidfile" ]; then
        pid=$(cat "$pidfile")
        if kill -0 $pid 2>/dev/null; then
            echo "Stopping $name (PID: $pid)..."
            kill -TERM $pid
            for i in {1..30}; do
                if ! kill -0 $pid 2>/dev/null; then
                    echo "✓ $name stopped"
                    rm -f "$pidfile"
                    return 0
                fi
                sleep 1
            done
            echo "Force killing $name..."
            kill -9 $pid 2>/dev/null
            rm -f "$pidfile"
        else
            echo "$name not running (stale PID file)"
            rm -f "$pidfile"
        fi
    else
        echo "No $name PID file found"
    fi
}

stop_process "monitor.pid" "monitor"
stop_process "follower.pid" "follower"
`, *workDir)

	if err := os.WriteFile(filepath.Join(*workDir, "stop.sh"), []byte(stopScript), 0755); err != nil {
		return fmt.Errorf("failed to write stop script: %w", err)
	}
	fmt.Println("✓ Created stop.sh")

	// Create monitor start script if monitor binary provided
	if *monitor != "" {
		monitorScript := fmt.Sprintf(`#!/bin/bash
cd "%s"
mkdir -p logs

echo "Starting follower monitor..."
nohup ./follower-monitor --work-dir "%s" > logs/monitor.log 2>&1 &
echo $! > monitor.pid
echo "Monitor started with PID: $(cat monitor.pid)"
echo "Web UI: http://localhost:9999 (localhost only by default)"
echo ""
echo "Features:"
echo "  - Status tab: Real-time sync status and progress"
echo "  - Logs tab: Live log viewing with filtering"
echo "  - Start/Stop: Control follower via web UI"
`, *workDir, *workDir)

		if err := os.WriteFile(filepath.Join(*workDir, "start-monitor.sh"), []byte(monitorScript), 0755); err != nil {
			return fmt.Errorf("failed to write monitor script: %w", err)
		}
		fmt.Println("✓ Created start-monitor.sh")
	}

	fmt.Println()
	fmt.Println("=== Deployment Complete ===")
	fmt.Printf("Directory: %s\n", *workDir)
	fmt.Println()
	fmt.Println("To start the follower:")
	fmt.Printf("  cd %s && ./start.sh\n", *workDir)
	if *monitor != "" {
		fmt.Println()
		fmt.Println("To start the monitor:")
		fmt.Printf("  ./start-monitor.sh\n")
	}
	fmt.Println()
	fmt.Println("To stop:")
	fmt.Println("  ./stop.sh")

	return nil
}

func start() error {
	fmt.Println()
	fmt.Println("=== Starting Follower ===")

	startScript := filepath.Join(*workDir, "start.sh")
	cmd := exec.Command("/bin/bash", startScript)
	cmd.Dir = *workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func showStatus() {
	if *workDir == "" {
		fmt.Println("Error: --work-dir required for status")
		os.Exit(1)
	}

	fmt.Println("=== Follower Status ===")
	fmt.Printf("Work directory: %s\n", *workDir)
	fmt.Println()

	// Check if deployed
	configPath := filepath.Join(*workDir, "accumulate.toml")
	if _, err := os.Stat(configPath); err != nil {
		fmt.Println("Status: NOT DEPLOYED")
		return
	}

	// Check follower
	fmt.Print("Follower: ")
	if pid := getPID(filepath.Join(*workDir, "follower.pid")); pid > 0 {
		if isRunning(pid) {
			fmt.Printf("RUNNING (PID %d)\n", pid)

			// Get sync status
			fmt.Println()
			fmt.Println("DN Status:")
			showRPCStatus(16592)
			fmt.Println()
			fmt.Println("BVN Status:")
			showRPCStatus(16692)
		} else {
			fmt.Println("STOPPED (stale PID)")
		}
	} else {
		fmt.Println("STOPPED")
	}

	// Check monitor
	fmt.Print("\nMonitor: ")
	if pid := getPID(filepath.Join(*workDir, "monitor.pid")); pid > 0 {
		if isRunning(pid) {
			fmt.Printf("RUNNING (PID %d) - http://localhost:9999\n", pid)
		} else {
			fmt.Println("STOPPED (stale PID)")
		}
	} else {
		fmt.Println("STOPPED")
	}
}

func stopFollower() {
	if *workDir == "" {
		fmt.Println("Error: --work-dir required")
		os.Exit(1)
	}

	stopScript := filepath.Join(*workDir, "stop.sh")
	if _, err := os.Stat(stopScript); err != nil {
		fmt.Println("Error: stop.sh not found - follower not deployed?")
		os.Exit(1)
	}

	cmd := exec.Command("/bin/bash", stopScript)
	cmd.Dir = *workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func getPID(pidFile string) int {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return pid
}

func isRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func showRPCStatus(port int) {
	cmd := exec.Command("curl", "-s", fmt.Sprintf("http://localhost:%d/status", port))
	output, err := cmd.Output()
	if err != nil {
		fmt.Printf("  Unable to connect (port %d)\n", port)
		return
	}

	// Parse output for key info
	s := string(output)
	if height := extractValue(s, "latest_block_height"); height != "" {
		fmt.Printf("  Height: %s\n", height)
	}
	if strings.Contains(s, `"catching_up":true`) || strings.Contains(s, `"catching_up": true`) {
		fmt.Println("  Syncing: Yes (catching up)")
	} else {
		fmt.Println("  Syncing: Complete")
	}
}

func extractValue(s, key string) string {
	idx := strings.Index(s, `"`+key+`"`)
	if idx < 0 {
		return ""
	}
	sub := s[idx+len(key)+3:]
	// Find the value after the colon
	for i, c := range sub {
		if c == '"' {
			end := strings.Index(sub[i+1:], `"`)
			if end >= 0 {
				return sub[i+1 : i+1+end]
			}
		}
	}
	return ""
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
