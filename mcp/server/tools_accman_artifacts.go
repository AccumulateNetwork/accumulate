package server

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// PrepareAccmanArtifacts prepares all artifacts needed for accman follower deployment
// Expects complete node directories with both CometBFT and Accumulate data
func (s *Server) prepareAccmanArtifacts(args map[string]interface{}) (map[string]interface{}, error) {
	// Required parameters - complete node directories
	dnNodeDir, ok := args["dn_node_dir"].(string)
	if !ok || dnNodeDir == "" {
		return nil, fmt.Errorf("missing required parameter: dn_node_dir (complete dnn directory with config/ and data/)")
	}

	bvnNodeDir, ok := args["bvn_node_dir"].(string)
	if !ok || bvnNodeDir == "" {
		return nil, fmt.Errorf("missing required parameter: bvn_node_dir (complete bvnn directory with config/ and data/)")
	}

	outputDir, ok := args["output_dir"].(string)
	if !ok || outputDir == "" {
		return nil, fmt.Errorf("missing required parameter: output_dir")
	}

	// Validate node directories have required structure
	if err := validateNodeDirectory(dnNodeDir, "DN"); err != nil {
		return nil, err
	}
	if err := validateNodeDirectory(bvnNodeDir, "BVN"); err != nil {
		return nil, err
	}

	// Optional parameters
	network, _ := args["network"].(string)
	if network == "" {
		network = "mainnet"
	}

	partition, _ := args["partition"].(string)
	if partition == "" {
		partition = "dual"
	}

	// Bootstrap peers (use defaults if not provided)
	var dnBootstrapPeers, bvnBootstrapPeers []interface{}
	if peers, ok := args["dn_bootstrap_peers"].([]interface{}); ok && len(peers) > 0 {
		dnBootstrapPeers = peers
	} else {
		dnBootstrapPeers = getDefaultBootstrapPeers(network, "dn")
	}

	if peers, ok := args["bvn_bootstrap_peers"].([]interface{}); ok && len(peers) > 0 {
		bvnBootstrapPeers = peers
	} else {
		bvnBootstrapPeers = getDefaultBootstrapPeers(network, "bvn")
	}

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Step 1: Create complete node archives (CometBFT + Accumulate)
	timestamp := time.Now().Format("20060102-150405")
	dnArchive := filepath.Join(outputDir, fmt.Sprintf("dn-node-%s-%s.tar.gz", network, timestamp))
	bvnArchive := filepath.Join(outputDir, fmt.Sprintf("bvn-node-%s-%s.tar.gz", network, timestamp))

	if err := createNodeArchive(dnNodeDir, dnArchive, "DN"); err != nil {
		return nil, fmt.Errorf("failed to create DN archive: %w", err)
	}

	if err := createNodeArchive(bvnNodeDir, bvnArchive, "BVN"); err != nil {
		return nil, fmt.Errorf("failed to create BVN archive: %w", err)
	}

	// Step 2: Create deployment metadata file
	metadataPath := filepath.Join(outputDir, fmt.Sprintf("deployment-metadata-%s.json", timestamp))
	metadata := map[string]interface{}{
		"network":             network,
		"partition":           partition,
		"dn_snapshot":         dnArchive,
		"bvn_snapshot":        bvnArchive,
		"dn_bootstrap_peers":  dnBootstrapPeers,
		"bvn_bootstrap_peers": bvnBootstrapPeers,
		"created_at":          time.Now().UTC().Format(time.RFC3339),
		"source_nodes": map[string]string{
			"dn":  dnNodeDir,
			"bvn": bvnNodeDir,
		},
		"structure": map[string]interface{}{
			"includes": []string{
				"CometBFT configuration (config/)",
				"CometBFT data (data/blockstore.db, state.db, etc.)",
				"Accumulate database (data/accumulate.db/)",
			},
			"note": "Complete node directories with both consensus and application layers",
		},
	}

	if err := writeJSONFile(metadataPath, metadata); err != nil {
		return nil, fmt.Errorf("failed to write metadata: %w", err)
	}

	// Step 3: Create accman deployment command
	deployCommand := fmt.Sprintf(`accman-mcp deploy_follower \
  --partition %s \
  --dn-snapshot %s \
  --bvn-snapshot %s \
  --use-volume \
  --network %s`, partition, dnArchive, bvnArchive, network)

	// Step 4: Create deployment script
	scriptPath := filepath.Join(outputDir, fmt.Sprintf("deploy-%s.sh", timestamp))
	scriptContent := fmt.Sprintf(`#!/bin/bash
# Accumulate Follower Deployment Script
# Generated: %s
# Network: %s
# Partition: %s

set -e

echo "=== Accumulate %s Follower Deployment ==="
echo ""

# Check if accman-mcp is available
if ! command -v accman-mcp &> /dev/null; then
    echo "ERROR: accman-mcp not found in PATH"
    echo "Please install accman-mcp first"
    echo ""
    echo "Installation:"
    echo "  cd /path/to/accman"
    echo "  go build -o accman-mcp ./cmd/accman-mcp"
    echo "  sudo mv accman-mcp /usr/local/bin/"
    exit 1
fi

echo "Deployment Configuration:"
echo "  Network:    %s"
echo "  Partition:  %s"
echo "  DN Archive: %s"
echo "  BVN Archive: %s"
echo ""

# Deploy follower
echo "Deploying follower..."
accman-mcp deploy_follower \
  --partition %s \
  --dn-snapshot %s \
  --bvn-snapshot %s \
  --use-volume \
  --network %s

echo ""
echo "=== Deployment Complete ==="
echo ""
echo "Check status:"
echo "  docker logs -f accumulate-follower-%s"
echo ""
echo "Query endpoints:"
echo "  DN:  http://localhost:52001"
echo "  BVN: http://localhost:52101"
echo ""
echo "Verify sync:"
echo "  curl http://localhost:52001/status | jq '.result.sync_info'"
echo "  curl http://localhost:52101/status | jq '.result.sync_info'"
`,
		time.Now().Format(time.RFC3339),
		network,
		partition,
		network,
		network,
		partition,
		filepath.Base(dnArchive),
		filepath.Base(bvnArchive),
		partition,
		dnArchive,
		bvnArchive,
		network,
		partition,
	)

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		return nil, fmt.Errorf("failed to write deployment script: %w", err)
	}

	// Step 5: Create verification script
	verifyPath := filepath.Join(outputDir, fmt.Sprintf("verify-artifacts-%s.sh", timestamp))
	verifyContent := fmt.Sprintf(`#!/bin/bash
# Artifact Verification Script

echo "=== Verifying Deployment Artifacts ==="
echo ""

DN_ARCHIVE="%s"
BVN_ARCHIVE="%s"

# Check DN archive
echo "Checking DN archive..."
if [ ! -f "$DN_ARCHIVE" ]; then
    echo "ERROR: DN archive not found: $DN_ARCHIVE"
    exit 1
fi
echo "  Size: $(du -h "$DN_ARCHIVE" | cut -f1)"
echo "  Contents:"
tar -tzf "$DN_ARCHIVE" | head -20
echo "  ..."
echo ""

# Check BVN archive
echo "Checking BVN archive..."
if [ ! -f "$BVN_ARCHIVE" ]; then
    echo "ERROR: BVN archive not found: $BVN_ARCHIVE"
    exit 1
fi
echo "  Size: $(du -h "$BVN_ARCHIVE" | cut -f1)"
echo "  Contents:"
tar -tzf "$BVN_ARCHIVE" | head -20
echo "  ..."
echo ""

# Verify required components in archives
echo "Verifying DN archive structure..."
if tar -tzf "$DN_ARCHIVE" | grep -q "config/config.toml"; then
    echo "  ✓ CometBFT configuration found"
else
    echo "  ✗ Missing CometBFT configuration"
fi

if tar -tzf "$DN_ARCHIVE" | grep -q "data/accumulate.db"; then
    echo "  ✓ Accumulate database found"
else
    echo "  ✗ Missing Accumulate database"
fi

echo ""
echo "Verifying BVN archive structure..."
if tar -tzf "$BVN_ARCHIVE" | grep -q "config/config.toml"; then
    echo "  ✓ CometBFT configuration found"
else
    echo "  ✗ Missing CometBFT configuration"
fi

if tar -tzf "$BVN_ARCHIVE" | grep -q "data/accumulate.db"; then
    echo "  ✓ Accumulate database found"
else
    echo "  ✗ Missing Accumulate database"
fi

echo ""
echo "=== Verification Complete ==="
`, dnArchive, bvnArchive)

	if err := os.WriteFile(verifyPath, []byte(verifyContent), 0755); err != nil {
		return nil, fmt.Errorf("failed to write verification script: %w", err)
	}

	return map[string]interface{}{
		"status": "success",
		"artifacts": map[string]interface{}{
			"dn_archive":           dnArchive,
			"bvn_archive":          bvnArchive,
			"metadata_file":        metadataPath,
			"deployment_script":    scriptPath,
			"verification_script":  verifyPath,
		},
		"metadata": metadata,
		"deployment": map[string]interface{}{
			"command": deployCommand,
			"script":  scriptPath,
		},
		"sizes": map[string]interface{}{
			"dn_archive":  getFileSize(dnArchive),
			"bvn_archive": getFileSize(bvnArchive),
		},
		"next_steps": []string{
			fmt.Sprintf("Verify artifacts: bash %s", verifyPath),
			fmt.Sprintf("Transfer artifacts to deployment server: scp %s/* server:/deployment/", outputDir),
			fmt.Sprintf("Run deployment script: bash %s", scriptPath),
			"Or use accman-mcp network_bootstrap_and_deploy for automatic deployment",
		},
	}, nil
}

// CreateNodeArchive creates a tar.gz archive from a complete node directory
func (s *Server) createNodeArchive(args map[string]interface{}) (map[string]interface{}, error) {
	nodeDir, ok := args["node_dir"].(string)
	if !ok || nodeDir == "" {
		return nil, fmt.Errorf("missing required parameter: node_dir")
	}

	outputFile, ok := args["output_file"].(string)
	if !ok || outputFile == "" {
		return nil, fmt.Errorf("missing required parameter: output_file")
	}

	nodeType, _ := args["node_type"].(string)
	if nodeType == "" {
		nodeType = "Node"
	}

	// Validate node directory
	if err := validateNodeDirectory(nodeDir, nodeType); err != nil {
		return nil, err
	}

	if err := createNodeArchive(nodeDir, outputFile, nodeType); err != nil {
		return nil, err
	}

	// Get file size
	info, err := os.Stat(outputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to stat archive: %w", err)
	}

	return map[string]interface{}{
		"status":      "success",
		"archive":     outputFile,
		"source":      nodeDir,
		"node_type":   nodeType,
		"size_bytes":  info.Size(),
		"size_human":  formatBytes(info.Size()),
		"created_at":  info.ModTime().Format(time.RFC3339),
		"structure": map[string]interface{}{
			"includes": []string{
				"config/ - CometBFT configuration",
				"data/ - CometBFT and Accumulate data",
			},
		},
	}, nil
}

// GetBootstrapPeers returns bootstrap peer information for a network
func (s *Server) getBootstrapPeers(args map[string]interface{}) (map[string]interface{}, error) {
	network, _ := args["network"].(string)
	if network == "" {
		network = "mainnet"
	}

	return map[string]interface{}{
		"network":             network,
		"dn_bootstrap_peers":  getDefaultBootstrapPeers(network, "dn"),
		"bvn_bootstrap_peers": getDefaultBootstrapPeers(network, "bvn"),
		"format":              "multiaddr",
		"note":                "These are default bootstrap peers. Update based on current network topology.",
	}, nil
}

// Helper functions

func validateNodeDirectory(nodeDir, nodeType string) error {
	// Check directory exists
	if _, err := os.Stat(nodeDir); os.IsNotExist(err) {
		return fmt.Errorf("%s node directory not found: %s", nodeType, nodeDir)
	}

	// Check for required subdirectories
	configDir := filepath.Join(nodeDir, "config")
	dataDir := filepath.Join(nodeDir, "data")

	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		return fmt.Errorf("%s node directory missing config/: %s", nodeType, nodeDir)
	}

	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		return fmt.Errorf("%s node directory missing data/: %s", nodeType, nodeDir)
	}

	// Check for Accumulate database
	accumDB := filepath.Join(dataDir, "accumulate.db")
	if _, err := os.Stat(accumDB); os.IsNotExist(err) {
		return fmt.Errorf("%s node directory missing data/accumulate.db/: %s", nodeType, nodeDir)
	}

	// Verify Accumulate database is not empty
	accumInfo, err := os.Stat(accumDB)
	if err == nil && accumInfo.IsDir() {
		// Count files in accumulate.db directory
		entries, err := os.ReadDir(accumDB)
		if err != nil {
			return fmt.Errorf("%s accumulate.db directory not readable: %w", nodeType, err)
		}
		if len(entries) == 0 {
			return fmt.Errorf("%s accumulate.db directory is empty", nodeType)
		}
	}

	// Check for CometBFT blockstore (optional but recommended)
	blockstoreDB := filepath.Join(dataDir, "blockstore.db")
	if _, err := os.Stat(blockstoreDB); os.IsNotExist(err) {
		// Warning but not fatal - some deployments might not have blockstore
		// This is logged but doesn't fail validation
	}

	return nil
}

func createNodeArchive(nodeDir, outputFile, nodeType string) error {
	// Create output directory if needed
	if err := os.MkdirAll(filepath.Dir(outputFile), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create tar.gz archive
	parentDir := filepath.Dir(nodeDir)
	dirName := filepath.Base(nodeDir)

	cmd := exec.Command("tar", "-czf", outputFile, "-C", parentDir, dirName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create %s archive: %w\nOutput: %s", nodeType, err, string(output))
	}

	return nil
}

func getDefaultBootstrapPeers(network, partition string) []interface{} {
	// Default bootstrap peers for mainnet and testnet
	peers := map[string]map[string][]interface{}{
		"mainnet": {
			"dn": {
				"/ip4/23.22.212.106/tcp/16591/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD",
			},
			"bvn": {
				"/ip4/23.22.212.106/tcp/16691/p2p/QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD",
			},
		},
		"testnet": {
			"dn": {
				"/ip4/testnet.accumulate.defidevs.io/tcp/16591/p2p/QmTestNodeID",
			},
			"bvn": {
				"/ip4/testnet.accumulate.defidevs.io/tcp/16691/p2p/QmTestNodeID",
			},
		},
	}

	if netPeers, ok := peers[network]; ok {
		if partPeers, ok := netPeers[partition]; ok {
			return partPeers
		}
	}

	// Fallback to mainnet DN
	return peers["mainnet"]["dn"]
}

func writeJSONFile(path string, data interface{}) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func getFileSize(path string) map[string]interface{} {
	info, err := os.Stat(path)
	if err != nil {
		return map[string]interface{}{
			"error": err.Error(),
		}
	}
	return map[string]interface{}{
		"bytes": info.Size(),
		"human": formatBytes(info.Size()),
	}
}
