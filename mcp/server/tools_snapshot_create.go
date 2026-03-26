// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CreateSnapshotFromNodeBackup creates a portable .snap file from a node backup.
// This is the recommended way to create snapshots for follower deployment.
//
// Requirements for CometBFT compatibility:
// - blockstore.db: Required to extract commit signatures at snapshot height
// - state.db: Required for CometBFT state initialization
// - accumulate.db: The Accumulate protocol database
//
// The resulting snapshot includes:
// - Consensus section with validators, commit signatures, LastBlockID, LastResultsHash
// - Raw database archives (state.db, blockstore.db, accumulate.db)
// - BPT (Binary Patricia Tree) for account state verification
//
// For BVN snapshots, the DN database is required to read the network definition
// which contains validator information for the partition.
func (s *Server) createSnapshotFromNodeBackup(args map[string]interface{}) (map[string]interface{}, error) {
	// Required parameters
	nodeDataDir, ok := args["node_data_dir"].(string)
	if !ok || nodeDataDir == "" {
		return nil, fmt.Errorf("missing required parameter: node_data_dir (path to node's data/ directory)")
	}

	outputPath, ok := args["output"].(string)
	if !ok || outputPath == "" {
		return nil, fmt.Errorf("missing required parameter: output (path for output .snap file)")
	}

	partition, ok := args["partition"].(string)
	if !ok || partition == "" {
		return nil, fmt.Errorf("missing required parameter: partition (Directory, Apollo, Yutu, Cyclops, or custom BVN name)")
	}

	// Validate paths
	var err error
	nodeDataDir, err = ValidateDirectoryPath(nodeDataDir, "")
	if err != nil {
		return nil, fmt.Errorf("invalid node_data_dir path: %w", err)
	}

	outputPath, err = ValidatePath(outputPath, "")
	if err != nil {
		return nil, fmt.Errorf("invalid output path: %w", err)
	}

	// Optional parameters
	dnDataDir, _ := args["dn_data_dir"].(string)
	if dnDataDir != "" {
		dnDataDir, err = ValidateDirectoryPath(dnDataDir, "")
		if err != nil {
			return nil, fmt.Errorf("invalid dn_data_dir path: %w", err)
		}
	}

	dbType, _ := args["db_type"].(string)
	if dbType == "" {
		dbType = "badger" // Default to badger for mainnet
	}
	if dbType != "badger" && dbType != "leveldb" {
		return nil, fmt.Errorf("invalid db_type: must be 'badger' or 'leveldb'")
	}

	genesis, _ := args["genesis"].(bool) // Skip message/transaction history for faster collection

	// Construct paths to required databases
	accumulateDB := filepath.Join(nodeDataDir, "accumulate.db")
	blockstoreDB := filepath.Join(nodeDataDir, "blockstore.db")
	stateDB := filepath.Join(nodeDataDir, "state.db")

	// Validate all required files exist
	var missingFiles []string
	if _, err := os.Stat(accumulateDB); os.IsNotExist(err) {
		missingFiles = append(missingFiles, "accumulate.db")
	}
	if _, err := os.Stat(blockstoreDB); os.IsNotExist(err) {
		missingFiles = append(missingFiles, "blockstore.db")
	}
	if _, err := os.Stat(stateDB); os.IsNotExist(err) {
		missingFiles = append(missingFiles, "state.db")
	}

	if len(missingFiles) > 0 {
		return map[string]interface{}{
			"status":  "error",
			"message": "Missing required database files",
			"details": map[string]interface{}{
				"node_data_dir": nodeDataDir,
				"missing_files": missingFiles,
				"required": []string{
					"accumulate.db - Accumulate protocol database",
					"blockstore.db - CometBFT block storage (required for commit signatures)",
					"state.db - CometBFT consensus state",
				},
			},
		}, nil
	}

	// Check if this is a BVN partition (not Directory) - needs DN database
	isDirectory := strings.EqualFold(partition, "Directory") || strings.EqualFold(partition, "DN")
	if !isDirectory && dnDataDir == "" {
		return map[string]interface{}{
			"status":  "error",
			"message": "BVN snapshots require DN database for network definition",
			"details": map[string]interface{}{
				"partition":   partition,
				"explanation": "The DN database contains the NetworkDefinition which lists validators for each partition. Without it, the snapshot cannot include proper validator information.",
				"solution":    "Provide dn_data_dir parameter pointing to the DN node's data/ directory",
			},
		}, nil
	}

	// Find create-snap binary
	createSnapBinary := s.findCreateSnapBinary()
	if createSnapBinary == "" {
		return map[string]interface{}{
			"status":  "error",
			"message": "create-snap binary not found",
			"details": map[string]interface{}{
				"searched_locations": []string{
					filepath.Join(s.getRepoRoot(), "create-snap"),
					filepath.Join(s.getRepoRoot(), "cmd/create-snap/create-snap"),
					"/tmp/create-snap",
				},
				"solution": "Build create-snap: cd cmd/create-snap && go build -o ../../create-snap",
			},
		}, nil
	}

	// Build command arguments
	cmdArgs := []string{
		"-db", accumulateDB,
		"-blockstore", blockstoreDB,
		"-statedb", stateDB,
		"-output", outputPath,
		"-partition", partition,
		"-type", dbType,
	}

	if !isDirectory && dnDataDir != "" {
		dnAccumulateDB := filepath.Join(dnDataDir, "accumulate.db")
		if _, err := os.Stat(dnAccumulateDB); os.IsNotExist(err) {
			return nil, fmt.Errorf("DN accumulate.db not found at %s", dnAccumulateDB)
		}
		cmdArgs = append(cmdArgs, "-dn-db", dnAccumulateDB)
	}

	if genesis {
		cmdArgs = append(cmdArgs, "-genesis")
	}

	// Create output directory if needed
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Run create-snap
	cmd := exec.Command(createSnapBinary, cmdArgs...)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return map[string]interface{}{
			"status":  "error",
			"message": "create-snap failed",
			"details": map[string]interface{}{
				"command": fmt.Sprintf("%s %s", createSnapBinary, strings.Join(cmdArgs, " ")),
				"error":   err.Error(),
				"output":  string(output),
			},
		}, nil
	}

	// Get output file info
	fileInfo, err := os.Stat(outputPath)
	if err != nil {
		return nil, fmt.Errorf("snapshot created but cannot stat output file: %w", err)
	}

	return map[string]interface{}{
		"status":  "success",
		"message": "Snapshot created successfully",
		"details": map[string]interface{}{
			"output_path":    outputPath,
			"size_bytes":     fileInfo.Size(),
			"size_mb":        fmt.Sprintf("%.2f", float64(fileInfo.Size())/(1024*1024)),
			"partition":      partition,
			"genesis_mode":   genesis,
			"db_type":        dbType,
			"includes": []string{
				"Consensus section with validators and commit signatures",
				"LastBlockID for CometBFT state initialization",
				"LastResultsHash for block validation",
				"Raw state.db archive",
				"Raw blockstore.db archive",
				"Raw accumulate.db archive",
				"BPT for account verification",
			},
		},
		"next_steps": []string{
			"Validate snapshot: accumulated validate-snapshot " + outputPath,
			"Restore to node: accumulated --work-dir /path/to/node restore-genesis " + outputPath,
		},
	}, nil
}

// findCreateSnapBinary searches for the create-snap binary in common locations
func (s *Server) findCreateSnapBinary() string {
	searchPaths := []string{
		filepath.Join(s.getRepoRoot(), "create-snap"),
		filepath.Join(s.getRepoRoot(), "cmd/create-snap/create-snap"),
		"/tmp/create-snap",
		"create-snap", // In PATH
	}

	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
		// Also try exec.LookPath for PATH lookup
		if found, err := exec.LookPath(path); err == nil {
			return found
		}
	}

	return ""
}

// getRepoRoot returns the accumulate repository root directory
func (s *Server) getRepoRoot() string {
	// Fallback: try common locations first - these are more reliable than
	// walking up from the executable path (which fails during tests)
	home, _ := os.UserHomeDir()
	candidates := []string{
		"/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate",
		filepath.Join(home, "go/src/gitlab.com/AccumulateNetwork/accumulate"),
	}

	for _, candidate := range candidates {
		// Check if this looks like the main accumulate repo (has cmd/create-snap)
		cmdDir := filepath.Join(candidate, "cmd", "create-snap")
		if _, err := os.Stat(cmdDir); err == nil {
			return candidate
		}
	}

	// Try to find repo root relative to MCP server location
	// The MCP server is at mcp/server/, so repo root is ../../
	execPath, err := os.Executable()
	if err == nil {
		// Go up from executable location
		dir := filepath.Dir(execPath)
		for i := 0; i < 10; i++ {
			// Check if this looks like the main accumulate repo root
			// (has cmd/create-snap directory, not just any go.mod)
			cmdDir := filepath.Join(dir, "cmd", "create-snap")
			if _, err := os.Stat(cmdDir); err == nil {
				return dir
			}
			dir = filepath.Dir(dir)
		}
	}

	return ""
}
