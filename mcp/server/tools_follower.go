package server

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// InitFollower initializes a follower node from database snapshots using Docker
func (s *Server) initFollower(args map[string]interface{}) (map[string]interface{}, error) {
	// Required parameters
	dnDatabase, ok := args["dn_database"].(string)
	if !ok || dnDatabase == "" {
		return nil, fmt.Errorf("missing required parameter: dn_database")
	}

	bvnDatabase, ok := args["bvn_database"].(string)
	if !ok || bvnDatabase == "" {
		return nil, fmt.Errorf("missing required parameter: bvn_database")
	}

	workDir, ok := args["work_dir"].(string)
	if !ok || workDir == "" {
		return nil, fmt.Errorf("missing required parameter: work_dir")
	}

	// Validate paths to prevent directory traversal
	var err error
	dnDatabase, err = ValidateDirectoryPath(dnDatabase, "")
	if err != nil {
		return nil, fmt.Errorf("invalid dn_database path: %w", err)
	}

	bvnDatabase, err = ValidateDirectoryPath(bvnDatabase, "")
	if err != nil {
		return nil, fmt.Errorf("invalid bvn_database path: %w", err)
	}

	workDir, err = ValidatePath(workDir, "")
	if err != nil {
		return nil, fmt.Errorf("invalid work_dir path: %w", err)
	}

	// Optional parameters with validation
	containerName, _ := args["container_name"].(string)
	if containerName == "" {
		containerName = "accumulate-follower"
	}
	if err := ValidateContainerName(containerName); err != nil {
		return nil, fmt.Errorf("invalid container_name: %w", err)
	}

	network, _ := args["network"].(string)
	if network == "" {
		network = "MainNet"
	}
	if err := ValidateNetworkName(network); err != nil {
		return nil, fmt.Errorf("invalid network: %w", err)
	}

	bvnName, _ := args["bvn_name"].(string)
	if bvnName == "" {
		bvnName = "Cyclops"
	}
	if err := ValidateNetworkName(bvnName); err != nil {
		return nil, fmt.Errorf("invalid bvn_name: %w", err)
	}

	// Auto-discover peers option
	autoDiscoverPeers, _ := args["auto_discover_peers"].(bool)

	// Genesis snap files (optional - will use defaults if not provided)
	dnGenesisSnap, _ := args["dn_genesis_snap"].(string)
	bvnGenesisSnap, _ := args["bvn_genesis_snap"].(string)

	// Validate genesis snap paths if provided
	if dnGenesisSnap != "" {
		dnGenesisSnap, err = ValidateFilePath(dnGenesisSnap, "")
		if err != nil {
			return nil, fmt.Errorf("invalid dn_genesis_snap path: %w", err)
		}
	}
	if bvnGenesisSnap != "" {
		bvnGenesisSnap, err = ValidateFilePath(bvnGenesisSnap, "")
		if err != nil {
			return nil, fmt.Errorf("invalid bvn_genesis_snap path: %w", err)
		}
	}

	// Bootstrap peers
	dnBootstrapPeers, _ := args["dn_bootstrap_peers"].([]interface{})
	bvnBootstrapPeers, _ := args["bvn_bootstrap_peers"].([]interface{})

	// Track peer source for result
	var peerSource string

	// Try dynamic peer discovery if auto_discover_peers is true or peers not provided
	if autoDiscoverPeers || (len(dnBootstrapPeers) == 0 && len(bvnBootstrapPeers) == 0) {
		networkName := "mainnet"
		if strings.Contains(strings.ToLower(network), "test") {
			networkName = "testnet"
		}

		client := NewBootstrapClient()

		// Try to get DN peers
		if len(dnBootstrapPeers) == 0 {
			dnPeers, source, err := client.GetBootstrapPeersWithFallback(networkName, "dn")
			if err == nil && len(dnPeers) > 0 {
				for _, p := range dnPeers {
					dnBootstrapPeers = append(dnBootstrapPeers, p)
				}
				peerSource = source
			}
		}

		// Try to get BVN peers
		if len(bvnBootstrapPeers) == 0 {
			bvnPeers, source, err := client.GetBootstrapPeersWithFallback(networkName, "bvn")
			if err == nil && len(bvnPeers) > 0 {
				for _, p := range bvnPeers {
					bvnBootstrapPeers = append(bvnBootstrapPeers, p)
				}
				if peerSource == "" {
					peerSource = source
				}
			}
		}
	}

	// Default bootstrap peers if still not available
	if len(dnBootstrapPeers) == 0 {
		dnBootstrapPeers = getDefaultBootstrapPeers("mainnet", "dn")
		peerSource = "hardcoded defaults"
	}
	if len(bvnBootstrapPeers) == 0 {
		bvnBootstrapPeers = getDefaultBootstrapPeers("mainnet", "bvn")
		if peerSource == "" {
			peerSource = "hardcoded defaults"
		}
	}

	// Create work directory if it doesn't exist
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}

	// Step 1: Copy database snapshots to work directory
	dnnPath := filepath.Join(workDir, "dnn")
	bvnnPath := filepath.Join(workDir, "bvnn")

	if err := copyDatabase(dnDatabase, dnnPath); err != nil {
		return nil, fmt.Errorf("failed to copy DN database: %w", err)
	}

	if err := copyDatabase(bvnDatabase, bvnnPath); err != nil {
		return nil, fmt.Errorf("failed to copy BVN database: %w", err)
	}

	// Step 2: Copy genesis snap files (if provided)
	var dnGenesisPath, bvnGenesisPath string
	if dnGenesisSnap != "" {
		dnGenesisPath = filepath.Join(workDir, "dn-genesis.snap")
		if err := copyFile(dnGenesisSnap, dnGenesisPath); err != nil {
			return nil, fmt.Errorf("failed to copy DN genesis snap: %w", err)
		}
	}
	if bvnGenesisSnap != "" {
		bvnGenesisPath = filepath.Join(workDir, "bvn-genesis.snap")
		if err := copyFile(bvnGenesisSnap, bvnGenesisPath); err != nil {
			return nil, fmt.Errorf("failed to copy BVN genesis snap: %w", err)
		}
	}

	// Step 3: Create accumulate.toml configuration
	configPath := filepath.Join(workDir, "accumulate.toml")
	if err := createFollowerConfig(configPath, network, bvnName, dnBootstrapPeers, bvnBootstrapPeers); err != nil {
		return nil, fmt.Errorf("failed to create config: %w", err)
	}

	result := map[string]interface{}{
		"status":         "initialized",
		"work_dir":       workDir,
		"dn_path":        dnnPath,
		"bvn_path":       bvnnPath,
		"config_path":    configPath,
		"container_name": containerName,
		"network":        network,
		"bvn_name":       bvnName,
		"peer_source":    peerSource,
		"dn_peers_count": len(dnBootstrapPeers),
		"bvn_peers_count": len(bvnBootstrapPeers),
		"next_steps":     "Use accumulate_run_follower to start the follower node in Docker",
	}

	if dnGenesisPath != "" {
		result["dn_genesis_snap"] = dnGenesisPath
	}
	if bvnGenesisPath != "" {
		result["bvn_genesis_snap"] = bvnGenesisPath
	}

	return result, nil
}

// RunFollower starts a follower node in Docker
func (s *Server) runFollower(args map[string]interface{}) (map[string]interface{}, error) {
	workDir, ok := args["work_dir"].(string)
	if !ok || workDir == "" {
		return nil, fmt.Errorf("missing required parameter: work_dir")
	}

	// Validate work directory path
	var err error
	workDir, err = ValidateDirectoryPath(workDir, "")
	if err != nil {
		return nil, fmt.Errorf("invalid work_dir path: %w", err)
	}

	// Optional parameters with validation
	containerName, _ := args["container_name"].(string)
	if containerName == "" {
		containerName = "accumulate-follower"
	}
	if err := ValidateContainerName(containerName); err != nil {
		return nil, fmt.Errorf("invalid container_name: %w", err)
	}

	dockerImage, _ := args["docker_image"].(string)
	if dockerImage == "" {
		// Use configured Docker image (default: v1.4.0, configurable via ACCUMULATE_DOCKER_IMAGE env var)
		dockerImage = s.state.Config.DockerImage
		if dockerImage == "" {
			// Fallback if config not set
			dockerImage = "registry.gitlab.com/accumulatenetwork/accumulate:v1.4.0"
		}
	}
	// Validate docker image name (basic validation - alphanumeric, dots, hyphens, colons, slashes)
	if err := ValidateDockerImage(dockerImage); err != nil {
		return nil, fmt.Errorf("invalid docker_image: %w", err)
	}

	// Validate work directory exists and has required subdirectories
	dnnPath := filepath.Join(workDir, "dnn")
	bvnnPath := filepath.Join(workDir, "bvnn")
	configPath := filepath.Join(workDir, "accumulate.toml")

	if _, err := os.Stat(dnnPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("DN directory not found: %s (run accumulate_init_follower first)", dnnPath)
	}
	if _, err := os.Stat(bvnnPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("BVN directory not found: %s (run accumulate_init_follower first)", bvnnPath)
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s (run accumulate_init_follower first)", configPath)
	}

	// Check if container already exists and remove it
	if err := removeContainerIfExists(containerName); err != nil {
		return nil, fmt.Errorf("failed to remove existing container: %w", err)
	}

	// Build docker run command
	dockerArgs := []string{
		"run",
		"-d",
		"--name", containerName,
		"--restart", "unless-stopped",
		"-v", fmt.Sprintf("%s:/node/dnn", dnnPath),
		"-v", fmt.Sprintf("%s:/node/bvnn", bvnnPath),
		"-v", fmt.Sprintf("%s:/node/accumulate.toml", configPath),
		"-p", "16591-16593:16591-16593", // DN ports
		"-p", "16691-16693:16691-16693", // BVN ports
		dockerImage,
		"run-dual", "/node/dnn", "/node/bvnn",
	}

	// Run Docker container
	cmd := exec.Command("docker", dockerArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to start Docker container: %w\nOutput: %s", err, string(output))
	}

	containerID := strings.TrimSpace(string(output))

	// Wait a bit for container to start
	time.Sleep(2 * time.Second)

	// Check if container is running
	running, err := isContainerRunning(containerName)
	if err != nil {
		return nil, fmt.Errorf("failed to check container status: %w", err)
	}

	if !running {
		// Get container logs for debugging
		logs, _ := getContainerLogs(containerName)
		return nil, fmt.Errorf("container started but is not running. Logs:\n%s", logs)
	}

	return map[string]interface{}{
		"status":         "started",
		"container_id":   containerID,
		"container_name": containerName,
		"work_dir":       workDir,
		"ports": map[string]string{
			"dn_api":  "16591-16593",
			"bvn_api": "16691-16693",
		},
		"message": "Follower started in Docker container",
		"check_status": fmt.Sprintf("docker logs -f %s", containerName),
	}, nil
}

// GetFollowerStatus checks the status of a running follower
func (s *Server) getFollowerStatus(args map[string]interface{}) (map[string]interface{}, error) {
	containerName, _ := args["container_name"].(string)
	if containerName == "" {
		containerName = "accumulate-follower"
	}
	if err := ValidateContainerName(containerName); err != nil {
		return nil, fmt.Errorf("invalid container_name: %w", err)
	}

	// Check if container exists
	exists, err := containerExists(containerName)
	if err != nil {
		return nil, fmt.Errorf("failed to check container existence: %w", err)
	}

	if !exists {
		return map[string]interface{}{
			"status":  "not_found",
			"message": fmt.Sprintf("Container '%s' does not exist", containerName),
		}, nil
	}

	// Check if container is running
	running, err := isContainerRunning(containerName)
	if err != nil {
		return nil, fmt.Errorf("failed to check container status: %w", err)
	}

	// Get container stats
	stats, err := getContainerStats(containerName)
	if err != nil {
		stats = fmt.Sprintf("Failed to get stats: %v", err)
	}

	result := map[string]interface{}{
		"container_name": containerName,
		"running":        running,
		"stats":          stats,
	}

	if running {
		result["status"] = "running"
		result["message"] = "Follower is running"
	} else {
		result["status"] = "stopped"
		result["message"] = "Follower container exists but is not running"

		// Get last few lines of logs
		logs, _ := getContainerLogs(containerName)
		if logs != "" {
			result["recent_logs"] = logs
		}
	}

	return result, nil
}

// StopFollower stops a running follower container
func (s *Server) stopFollower(args map[string]interface{}) (map[string]interface{}, error) {
	containerName, _ := args["container_name"].(string)
	if containerName == "" {
		containerName = "accumulate-follower"
	}
	if err := ValidateContainerName(containerName); err != nil {
		return nil, fmt.Errorf("invalid container_name: %w", err)
	}

	cmd := exec.Command("docker", "stop", containerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to stop container: %w\nOutput: %s", err, string(output))
	}

	return map[string]interface{}{
		"status":         "stopped",
		"container_name": containerName,
		"message":        "Follower container stopped",
	}, nil
}

// RemoveFollower removes a follower container
func (s *Server) removeFollower(args map[string]interface{}) (map[string]interface{}, error) {
	containerName, _ := args["container_name"].(string)
	if containerName == "" {
		containerName = "accumulate-follower"
	}
	if err := ValidateContainerName(containerName); err != nil {
		return nil, fmt.Errorf("invalid container_name: %w", err)
	}

	// Stop container first if running
	s.stopFollower(args)

	cmd := exec.Command("docker", "rm", containerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to remove container: %w\nOutput: %s", err, string(output))
	}

	return map[string]interface{}{
		"status":         "removed",
		"container_name": containerName,
		"message":        "Follower container removed",
	}, nil
}

// Helper functions

func copyDatabase(src, dst string) error {
	// Check if source exists
	srcInfo, err := os.Stat(src)
	if os.IsNotExist(err) {
		return fmt.Errorf("source database not found: %s", src)
	}
	if err != nil {
		return fmt.Errorf("failed to stat source database: %w", err)
	}

	// Verify source is a directory
	if !srcInfo.IsDir() {
		return fmt.Errorf("source must be a directory: %s", src)
	}

	// Check if source contains accumulate.db (basic validation)
	accumDB := filepath.Join(src, "data", "accumulate.db")
	accumInfo, err := os.Stat(accumDB)
	if err != nil {
		return fmt.Errorf("source missing data/accumulate.db/: %s (may be incomplete snapshot)", src)
	}

	// Verify accumulate.db is not empty
	if accumInfo.IsDir() {
		entries, err := os.ReadDir(accumDB)
		if err != nil {
			return fmt.Errorf("accumulate.db directory not readable: %w", err)
		}
		if len(entries) == 0 {
			return fmt.Errorf("accumulate.db directory is empty (corrupted snapshot)")
		}
	}

	// Create destination directory
	if err := os.MkdirAll(dst, 0755); err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}

	// Use cp -r to copy the entire directory
	cmd := exec.Command("cp", "-r", src+"/.", dst)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to copy database: %w\nOutput: %s", err, string(output))
	}

	// Verify destination was created successfully
	dstInfo, err := os.Stat(dst)
	if err != nil {
		return fmt.Errorf("copy completed but destination verification failed: %w", err)
	}
	if !dstInfo.IsDir() {
		return fmt.Errorf("destination is not a directory after copy")
	}

	// Verify critical files were copied
	dstAccumDB := filepath.Join(dst, "data", "accumulate.db")
	if _, err := os.Stat(dstAccumDB); err != nil {
		return fmt.Errorf("copy completed but data/accumulate.db/ missing in destination")
	}

	return nil
}

func createFollowerConfig(path, network, bvnName string, dnPeers, bvnPeers []interface{}) error {
	// Convert peers to strings
	dnPeerStrs := make([]string, len(dnPeers))
	for i, p := range dnPeers {
		dnPeerStrs[i] = fmt.Sprintf("    %q", p)
	}

	bvnPeerStrs := make([]string, len(bvnPeers))
	for i, p := range bvnPeers {
		bvnPeerStrs[i] = fmt.Sprintf("    %q", p)
	}

	config := fmt.Sprintf(`network = %q

[[configurations]]
  type = "follower"
  mode = "dual"
  bvn = %q
  listen = "/ip4/0.0.0.0/tcp/16591"
  storage-type = "badger"
  enable-healing = false
  enable-snapshots = false

  dn-bootstrap-peers = [
%s
  ]

  bvn-bootstrap-peers = [
%s
  ]

[logging]
  format = "plain"
  [[logging.rules]]
    level = "info"
`, network, bvnName, strings.Join(dnPeerStrs, ",\n"), strings.Join(bvnPeerStrs, ",\n"))

	return os.WriteFile(path, []byte(config), 0644)
}

func removeContainerIfExists(name string) error {
	// Check if container exists
	cmd := exec.Command("docker", "ps", "-a", "--filter", fmt.Sprintf("name=^%s$", name), "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return err
	}

	if strings.TrimSpace(string(output)) != "" {
		// Container exists, remove it
		cmd := exec.Command("docker", "rm", "-f", name)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to remove existing container: %w", err)
		}
	}

	return nil
}

func containerExists(name string) (bool, error) {
	cmd := exec.Command("docker", "ps", "-a", "--filter", fmt.Sprintf("name=^%s$", name), "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(string(output)) != "", nil
}

func isContainerRunning(name string) (bool, error) {
	cmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("name=^%s$", name), "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(string(output)) != "", nil
}

func getContainerLogs(name string) (string, error) {
	cmd := exec.Command("docker", "logs", "--tail", "50", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	return string(output), nil
}

func getContainerStats(name string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "stats", "--no-stream", "--format", "CPU: {{.CPUPerc}}, Memory: {{.MemUsage}}", name)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

func copyFile(src, dst string) error {
	// Check if source exists
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return fmt.Errorf("source file not found: %s", src)
	}

	// Create destination directory if needed
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Use cp to copy the file
	cmd := exec.Command("cp", src, dst)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to copy file: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// GetGenesisFiles returns standard genesis snapshot file locations
func (s *Server) getGenesisFiles(args map[string]interface{}) (map[string]interface{}, error) {
	network, _ := args["network"].(string)
	if network == "" {
		network = "mainnet"
	}

	bvn, _ := args["bvn"].(string)
	if bvn == "" {
		bvn = "1"
	}

	// Get user home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	// Standard locations for genesis files
	accumDir := filepath.Join(homeDir, ".accumulate")
	dnGenesis := filepath.Join(accumDir, "dn-genesis.snap")
	bvnGenesis := filepath.Join(accumDir, fmt.Sprintf("bvn%s-genesis.snap", bvn))

	result := map[string]interface{}{
		"network":               network,
		"bvn":                   bvn,
		"dn_genesis_snap":       dnGenesis,
		"bvn_genesis_snap":      bvnGenesis,
		"accumulate_directory":  accumDir,
	}

	// Check if files exist
	_, dnErr := os.Stat(dnGenesis)
	_, bvnErr := os.Stat(bvnGenesis)

	result["dn_genesis_exists"] = (dnErr == nil)
	result["bvn_genesis_exists"] = (bvnErr == nil)

	if dnErr != nil || bvnErr != nil {
		var missing []string
		if dnErr != nil {
			missing = append(missing, "dn-genesis.snap")
		}
		if bvnErr != nil {
			missing = append(missing, fmt.Sprintf("bvn%s-genesis.snap", bvn))
		}
		result["warning"] = fmt.Sprintf("Genesis files not found: %v. These may need to be obtained from network bootstrap.", missing)
		result["note"] = "Genesis files are typically located in ~/.accumulate/ directory"
	}

	return result, nil
}
