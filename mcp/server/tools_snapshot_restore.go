package server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/privval"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// RestoreFromSnapshots restores follower nodes from .snap files
// This creates node directories with databases and configurations ready for Docker deployment
func (s *Server) restoreFromSnapshots(args map[string]interface{}) (map[string]interface{}, error) {
	// Required parameters
	dnSnapshot, ok := args["dn_snapshot"].(string)
	if !ok || dnSnapshot == "" {
		return nil, fmt.Errorf("missing required parameter: dn_snapshot")
	}

	bvnSnapshot, ok := args["bvn_snapshot"].(string)
	if !ok || bvnSnapshot == "" {
		return nil, fmt.Errorf("missing required parameter: bvn_snapshot")
	}

	workDir, ok := args["work_dir"].(string)
	if !ok || workDir == "" {
		return nil, fmt.Errorf("missing required parameter: work_dir")
	}

	// Verify snapshot files exist
	if _, err := os.Stat(dnSnapshot); err != nil {
		return nil, fmt.Errorf("DN snapshot file not found: %s", dnSnapshot)
	}
	if _, err := os.Stat(bvnSnapshot); err != nil {
		return nil, fmt.Errorf("BVN snapshot file not found: %s", bvnSnapshot)
	}

	// Optional parameters
	network, _ := args["network"].(string)
	if network == "" {
		network = "MainNet"
	}

	bvnName, _ := args["bvn_name"].(string)
	if bvnName == "" {
		bvnName = "Cyclops"
	}

	// Port configuration - support both methods
	var dnListenPort, dnAPIPort, dnP2PPort uint64
	var bvnListenPort, bvnAPIPort, bvnP2PPort uint64

	// Check for explicit ports first (takes precedence)
	if portsMap, ok := args["ports"].(map[string]interface{}); ok {
		dnListenPort, _ = parsePort(portsMap["dn_listen"])
		dnAPIPort, _ = parsePort(portsMap["dn_api"])
		dnP2PPort, _ = parsePort(portsMap["dn_p2p"])
		bvnListenPort, _ = parsePort(portsMap["bvn_listen"])
		bvnAPIPort, _ = parsePort(portsMap["bvn_api"])
		bvnP2PPort, _ = parsePort(portsMap["bvn_p2p"])
	}

	// Fall back to port_offset if explicit ports not provided
	if dnListenPort == 0 {
		portOffset := uint64(0)
		if offset, ok := args["port_offset"].(float64); ok {
			portOffset = uint64(offset)
		}
		basePort := uint64(16591) + portOffset
		dnListenPort = basePort
		dnAPIPort = basePort + 1
		dnP2PPort = basePort + 2
		bvnListenPort = basePort + 100
		bvnAPIPort = basePort + 101
		bvnP2PPort = basePort + 102
	}

	// Create work directory
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}

	// Step 1: Restore Directory Network
	dnnPath := filepath.Join(workDir, "dnn")
	if err := restorePartition(dnSnapshot, dnnPath, network, protocol.Directory,
		dnListenPort, dnAPIPort, dnP2PPort); err != nil {
		return nil, fmt.Errorf("failed to restore DN partition: %w", err)
	}

	// Step 2: Restore Block Validator Network
	bvnnPath := filepath.Join(workDir, "bvnn")
	if err := restorePartition(bvnSnapshot, bvnnPath, network, bvnName,
		bvnListenPort, bvnAPIPort, bvnP2PPort); err != nil {
		return nil, fmt.Errorf("failed to restore BVN partition: %w", err)
	}

	// Step 3: Create dual-node configuration
	configPath := filepath.Join(workDir, "accumulate.toml")
	if err := createDualNodeConfig(configPath, network, bvnName,
		dnListenPort, dnAPIPort, dnP2PPort,
		bvnListenPort, bvnAPIPort, bvnP2PPort); err != nil {
		return nil, fmt.Errorf("failed to create dual-node config: %w", err)
	}

	return map[string]interface{}{
		"status":  "success",
		"message": "Follower node restored successfully from snapshots",
		"details": map[string]interface{}{
			"work_dir":   workDir,
			"dn_dir":     dnnPath,
			"bvn_dir":    bvnnPath,
			"config":     configPath,
			"network":    network,
			"bvn_name":   bvnName,
			"dn_ports":   fmt.Sprintf("%d-%d-%d", dnListenPort, dnAPIPort, dnP2PPort),
			"bvn_ports":  fmt.Sprintf("%d-%d-%d", bvnListenPort, bvnAPIPort, bvnP2PPort),
		},
	}, nil
}

// restorePartition restores a single partition from a snapshot file
func restorePartition(snapshotPath, nodeDir, network, partitionID string,
	listenPort, apiPort, p2pPort uint64) error {

	// Create node directories
	configDir := filepath.Join(nodeDir, "config")
	dataDir := filepath.Join(nodeDir, "data")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data dir: %w", err)
	}

	// Determine partition type
	partitionType := protocol.PartitionTypeDirectory
	if partitionID != protocol.Directory {
		partitionType = protocol.PartitionTypeBlockValidator
	}

	// Create configuration for this partition
	cfg := config.Default(network, partitionType, config.Follower, partitionID)
	cfg.SetRoot(nodeDir)

	// Configure ports
	cfg.P2P.ListenAddress = fmt.Sprintf("tcp://0.0.0.0:%d", p2pPort)
	cfg.RPC.ListenAddress = fmt.Sprintf("tcp://0.0.0.0:%d", listenPort)
	cfg.Accumulate.API.ListenAddress = fmt.Sprintf("http://0.0.0.0:%d", apiPort)

	// Configure bootstrap peers for network connectivity
	if err := configureBootstrapPeers(cfg, network, partitionType); err != nil {
		return fmt.Errorf("failed to configure bootstrap peers: %w", err)
	}

	// Write both accumulate.toml and tendermint.toml
	if err := config.Store(cfg); err != nil {
		return fmt.Errorf("failed to write config files: %w", err)
	}

	// Generate node keys (node_key.json and priv_validator_key.json)
	if err := generateNodeKeys(configDir); err != nil {
		return fmt.Errorf("failed to generate node keys: %w", err)
	}

	// Run restore-snapshot command with --work-dir flag
	cmd := exec.Command("accumulated", "restore-snapshot", snapshotPath, "--work-dir", nodeDir)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("restore-snapshot failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// createDualNodeConfig creates the runtime dual-node configuration
func createDualNodeConfig(configPath, network, bvnName string,
	dnListen, dnAPI, dnP2P, bvnListen, bvnAPI, bvnP2P uint64) error {

	// Create a dual-node configuration file that references both partition directories
	// This config is used when running with "accumulated run-dual"
	config := fmt.Sprintf(`# Dual-node follower configuration for %s
network = %q

# Directory Network Configuration
[[configurations]]
  type = "follower"
  partition = "Directory"
  storage-type = "badger"

  [configurations.p2p]
    listen = "tcp://0.0.0.0:%d"

  [configurations.rpc]
    listen = "tcp://0.0.0.0:%d"

  [configurations.accumulate]
    [configurations.accumulate.api]
      listen = "http://0.0.0.0:%d"

# Block Validator Network Configuration
[[configurations]]
  type = "follower"
  partition = %q
  storage-type = "badger"

  [configurations.p2p]
    listen = "tcp://0.0.0.0:%d"

  [configurations.rpc]
    listen = "tcp://0.0.0.0:%d"

  [configurations.accumulate]
    [configurations.accumulate.api]
      listen = "http://0.0.0.0:%d"

[logging]
  format = "plain"
  [[logging.rules]]
    level = "info"
`, network, network, dnP2P, dnListen, dnAPI, bvnName, bvnP2P, bvnListen, bvnAPI)

	return os.WriteFile(configPath, []byte(config), 0644)
}

// generateNodeKeys generates Tendermint node keys (node_key.json and priv_validator_key.json)
func generateNodeKeys(configDir string) error {
	// Generate node_key.json
	nodeKeyPath := filepath.Join(configDir, "node_key.json")
	if _, err := os.Stat(nodeKeyPath); os.IsNotExist(err) {
		// Use CometBFT's p2p package to load or generate node key
		if _, err := p2p.LoadOrGenNodeKey(nodeKeyPath); err != nil {
			return fmt.Errorf("failed to generate node key: %w", err)
		}
	}

	// Generate priv_validator_key.json
	privValKeyPath := filepath.Join(configDir, "priv_validator_key.json")
	if _, err := os.Stat(privValKeyPath); os.IsNotExist(err) {
		privValKey := privval.GenFilePV(privValKeyPath, filepath.Join(configDir, "priv_validator_state.json"))
		privValKey.Save()
	}

	return nil
}

// configureBootstrapPeers sets up bootstrap peers for network connectivity
func configureBootstrapPeers(cfg *config.Config, network string, partitionType protocol.PartitionType) error {
	// MainNet bootstrap peers (using known seed nodes)
	if strings.ToLower(network) == "mainnet" {
		var peerAddr string
		if partitionType == protocol.PartitionTypeDirectory {
			// DN bootstrap peers
			peerAddr = "/ip4/23.22.212.106/tcp/16591/p2p/12D3KooWGJTh7Cp1Lqc9eCPLVSsNzJ9A8JXJzEQPmRmkCjpZLXSp"
		} else {
			// BVN bootstrap peers
			peerAddr = "/ip4/23.22.212.106/tcp/16691/p2p/12D3KooWHBeB8BJMZD2x4LhMEvKJRMZZTVRJYZo5QzP3aJFpEjKy"
		}

		// Parse multiaddr and add to bootstrap peers
		ma, err := multiaddr.NewMultiaddr(peerAddr)
		if err != nil {
			return fmt.Errorf("failed to parse bootstrap peer address: %w", err)
		}
		cfg.Accumulate.P2P.BootstrapPeers = []multiaddr.Multiaddr{ma}
	}
	// For other networks, bootstrap peers should be provided via config
	return nil
}

// parsePort extracts port number from interface{} (handles both float64 and string)
func parsePort(v interface{}) (uint64, error) {
	switch val := v.(type) {
	case float64:
		return uint64(val), nil
	case string:
		// Could parse string to int if needed
		return 0, fmt.Errorf("string port not yet supported")
	default:
		return 0, fmt.Errorf("invalid port type: %T", v)
	}
}
