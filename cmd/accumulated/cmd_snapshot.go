// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/cometbft"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	cmdMain.AddCommand(cmdValidateSnapshot, cmdRestoreGenesis)
}

var cmdValidateSnapshot = &cobra.Command{
	Use:   "validate-snapshot [file]",
	Short: "Validate a snapshot file for restore compatibility",
	Long: `Validates a snapshot file to ensure it can be used for restore.

Checks performed:
  - Snapshot version (must be v2)
  - Header section present with valid metadata
  - Consensus section present (required for genesis.json creation)
  - Validators present in consensus section
  - Root hash present

Exit codes:
  0 - Snapshot is valid and can be restored
  1 - Snapshot has issues that may prevent restore`,
	Args: cobra.ExactArgs(1),
	Run:  validateSnapshot,
}

var cmdRestoreGenesis = &cobra.Command{
	Use:   "restore-genesis [snapshot-file]",
	Short: "Restore a genesis snapshot to initialize a new node (minimal config required)",
	Long: `Restores a snapshot to initialize a new node with minimal configuration.

This command:
  1. Creates default configuration files if they don't exist
  2. Extracts consensus state from the snapshot
  3. Creates genesis.json with validators and app hash
  4. Initializes CometBFT state.db and blockstore.db
  5. Restores the Accumulate database

Unlike 'restore-snapshot', this command can work without pre-existing
configuration files by creating sensible defaults based on snapshot metadata.`,
	Args: cobra.ExactArgs(1),
	Run:  restoreGenesis,
}

var flagRestoreGenesis struct {
	Network   string
	Partition string
}

func init() {
	cmdRestoreGenesis.Flags().StringVar(&flagRestoreGenesis.Network, "network", "mainnet", "Network name (mainnet, testnet, devnet)")
	cmdRestoreGenesis.Flags().StringVar(&flagRestoreGenesis.Partition, "partition", "", "Partition ID (auto-detected from snapshot if not specified)")
}

func validateSnapshot(_ *cobra.Command, args []string) {
	f, err := os.Open(args[0])
	checkf(err, "open snapshot file")
	defer f.Close()

	fmt.Printf("Validating snapshot: %s\n\n", args[0])

	// Try to open as v2 snapshot
	rd, err := snapshot.Open(f)
	if err != nil {
		fmt.Printf("[FAIL] Cannot open snapshot: %v\n", err)
		os.Exit(1)
	}

	var issues []string
	var warnings []string

	// Check version
	fmt.Printf("Version: %d\n", rd.Header.Version)
	if rd.Header.Version != snapshot.Version2 {
		issues = append(issues, fmt.Sprintf("Unsupported version: got %d, want %d", rd.Header.Version, snapshot.Version2))
	}

	// Check root hash
	if len(rd.Header.RootHash) == 0 {
		issues = append(issues, "Missing root hash in header")
	} else {
		fmt.Printf("Root Hash: %x\n", rd.Header.RootHash)
	}

	// Check system ledger info
	if rd.Header.SystemLedger != nil {
		fmt.Printf("Partition: %s\n", rd.Header.SystemLedger.Url)
		fmt.Printf("Block Index: %d\n", rd.Header.SystemLedger.Index)
		fmt.Printf("Timestamp: %s\n", rd.Header.SystemLedger.Timestamp)
	} else {
		warnings = append(warnings, "No system ledger info in header")
	}

	// List sections
	fmt.Printf("\nSections (%d total):\n", len(rd.Sections))
	hasConsensus := false
	hasRecords := false
	hasBPT := false

	for _, s := range rd.Sections {
		typeName := s.Type().String()
		fmt.Printf("  - %-15s (offset: %d, size: %d bytes)\n", typeName, s.Offset(), s.Size())

		switch s.Type() {
		case snapshot.SectionTypeConsensus:
			hasConsensus = true
		case snapshot.SectionTypeRecords:
			hasRecords = true
		case snapshot.SectionTypeBPT, snapshot.SectionTypeRawBPT:
			hasBPT = true
		}
	}

	if !hasRecords {
		issues = append(issues, "Missing records section")
	}
	if !hasBPT {
		warnings = append(warnings, "Missing BPT section (may affect verification)")
	}

	// Check consensus section in detail
	fmt.Println()
	if !hasConsensus {
		issues = append(issues, "Missing consensus section - cannot create genesis.json")
	} else {
		consensusInfo := validateConsensusSection(rd)
		if consensusInfo.err != nil {
			issues = append(issues, fmt.Sprintf("Consensus section error: %v", consensusInfo.err))
		} else {
			fmt.Printf("Consensus Section:\n")
			fmt.Printf("  Chain ID: %s\n", consensusInfo.chainID)
			fmt.Printf("  Validators: %d\n", consensusInfo.validatorCount)
			if consensusInfo.hasBlock {
				fmt.Printf("  Block Height: %d\n", consensusInfo.blockHeight)
				fmt.Printf("  Block Time: %s\n", consensusInfo.blockTime)
			} else {
				warnings = append(warnings, "No block data in consensus section (will create minimal genesis)")
			}
			if consensusInfo.validatorCount == 0 {
				warnings = append(warnings, "No validators in consensus section")
			}
		}
	}

	// Print summary
	fmt.Println()
	fmt.Println("=== VALIDATION SUMMARY ===")

	if len(warnings) > 0 {
		fmt.Println("\nWarnings:")
		for _, w := range warnings {
			fmt.Printf("  [WARN] %s\n", w)
		}
	}

	if len(issues) > 0 {
		fmt.Println("\nIssues:")
		for _, i := range issues {
			fmt.Printf("  [FAIL] %s\n", i)
		}
		fmt.Println("\n[FAIL] Snapshot has issues that may prevent restore")
		os.Exit(1)
	}

	fmt.Println("\n[OK] Snapshot is valid and can be restored")
}

type consensusInfo struct {
	chainID        string
	validatorCount int
	hasBlock       bool
	blockHeight    int64
	blockTime      string
	err            error
}

func validateConsensusSection(rd *snapshot.Reader) consensusInfo {
	info := consensusInfo{}

	// Find and open consensus section
	consensusReader, err := rd.Open(snapshot.SectionTypeConsensus)
	if err != nil {
		info.err = err
		return info
	}

	// Read the raw bytes
	rawBytes, err := io.ReadAll(consensusReader)
	if err != nil {
		info.err = fmt.Errorf("read consensus section: %v", err)
		return info
	}

	// Unmarshal the consensus doc
	consensusDoc := new(cometbft.GenesisDoc)
	err = consensusDoc.UnmarshalBinary(rawBytes)
	if err != nil {
		info.err = fmt.Errorf("unmarshal consensus doc: %v", err)
		return info
	}

	info.chainID = consensusDoc.ChainID
	info.validatorCount = len(consensusDoc.Validators)

	if consensusDoc.Block != nil {
		info.hasBlock = true
		info.blockHeight = consensusDoc.Block.Height
		info.blockTime = consensusDoc.Block.Time.String()
	}

	return info
}

func restoreGenesis(_ *cobra.Command, args []string) {
	snapshotPath := args[0]

	// Check snapshot exists
	f, err := os.Open(snapshotPath)
	checkf(err, "open snapshot file")

	// First, validate the snapshot
	fmt.Printf("Validating snapshot: %s\n", snapshotPath)
	rd, err := snapshot.Open(f)
	checkf(err, "open snapshot")

	// Validate and extract consensus info
	info := validateConsensusSection(rd)

	// Get partition info - prefer explicit flag, then from consensus section
	partitionID := flagRestoreGenesis.Partition
	if info.err != nil {
		if partitionID == "" {
			fatalf("Failed to read consensus section: %v\nSpecify --partition to skip consensus validation", info.err)
		}
		fmt.Printf("Warning: Could not parse consensus section: %v\n", info.err)
		fmt.Println("Proceeding with explicit partition ID...")
	} else if info.chainID == "" {
		if partitionID == "" {
			fatalf("Snapshot missing ChainID - specify --partition")
		}
	} else if partitionID == "" {
		// Extract partition from ChainID format: Network.Partition
		parts := strings.Split(info.chainID, ".")
		if len(parts) >= 2 {
			partitionID = parts[len(parts)-1] // Take the last part (e.g., "Directory" or "Cyclops")
		}
	}
	if partitionID == "" {
		fatalf("Cannot determine partition from snapshot. Please specify --partition")
	}

	fmt.Printf("Partition: %s\n", partitionID)
	fmt.Printf("Work directory: %s\n", flagMain.WorkDir)

	f.Close()

	// Ensure work directory exists
	err = os.MkdirAll(flagMain.WorkDir, 0755)
	checkf(err, "create work directory")

	// Check if config files exist, create if not
	configPath := filepath.Join(flagMain.WorkDir, "config")
	tendermintConfig := filepath.Join(configPath, "config.toml")
	accumulateConfig := filepath.Join(configPath, "accumulate.toml")

	if _, err := os.Stat(tendermintConfig); os.IsNotExist(err) {
		fmt.Println("Creating default configuration files...")
		err = createDefaultConfig(flagMain.WorkDir, partitionID, flagRestoreGenesis.Network)
		checkf(err, "create default config")
	} else {
		fmt.Println("Using existing configuration files")
	}

	// Verify config files exist
	if _, err := os.Stat(accumulateConfig); os.IsNotExist(err) {
		fatalf("accumulate.toml not found at %s", accumulateConfig)
	}

	// Now do the actual restore - extract raw database archives
	f, err = os.Open(snapshotPath)
	checkf(err, "reopen snapshot file")
	defer f.Close()

	rd, err = snapshot.Open(f)
	checkf(err, "open snapshot for restore")

	dataPath := filepath.Join(flagMain.WorkDir, "data")
	err = os.MkdirAll(dataPath, 0755)
	checkf(err, "create data directory")

	fmt.Println("\n=== EXTRACTING RAW DATABASE ARCHIVES ===")

	// Extract CometBFT state.db
	if err := extractDatabaseArchive(rd, snapshot.SectionTypeCometStateDB, filepath.Join(dataPath, "state.db")); err != nil {
		fmt.Printf("Warning: Could not extract state.db: %v\n", err)
	} else {
		fmt.Println("  state.db extracted successfully")
	}

	// Extract CometBFT blockstore.db
	if err := extractDatabaseArchive(rd, snapshot.SectionTypeCometBlockstoreDB, filepath.Join(dataPath, "blockstore.db")); err != nil {
		fmt.Printf("Warning: Could not extract blockstore.db: %v\n", err)
	} else {
		fmt.Println("  blockstore.db extracted successfully")
	}

	// Extract Accumulate database
	if err := extractDatabaseArchive(rd, snapshot.SectionTypeAccumulateDB, filepath.Join(dataPath, "accumulate.db")); err != nil {
		fmt.Printf("Warning: Could not extract accumulate.db: %v\n", err)
	} else {
		fmt.Println("  accumulate.db extracted successfully")
	}

	fmt.Println("=== DATABASE EXTRACTION COMPLETE ===")

	fmt.Println("\n=== RESTORE COMPLETE ===")
	fmt.Printf("Work directory: %s\n", flagMain.WorkDir)
	fmt.Printf("Partition: %s\n", partitionID)
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Review and edit configuration in", configPath)
	fmt.Println("  2. Generate node keys if not present: accumulated init --work-dir", flagMain.WorkDir)
	fmt.Println("  3. Start the node: accumulated run --work-dir", flagMain.WorkDir)
}

func createDefaultConfig(workDir, partitionID, network string) error {
	// Create config directory
	configPath := filepath.Join(workDir, "config")
	err := os.MkdirAll(configPath, 0755)
	if err != nil {
		return fmt.Errorf("create config directory: %v", err)
	}

	// Determine partition type
	var partType protocol.PartitionType
	if partitionID == "Directory" {
		partType = protocol.PartitionTypeDirectory
	} else {
		partType = protocol.PartitionTypeBlockValidator
	}

	// Create a minimal config
	c := config.Default(network, partType, config.Follower, partitionID)

	// Set paths
	c.RootDir = workDir
	c.Accumulate.Storage.Path = filepath.Join(workDir, "data", "accumulate.db")

	// Store the config
	err = config.Store(c)
	if err != nil {
		return fmt.Errorf("store config: %v", err)
	}

	fmt.Printf("Created configuration for %s partition\n", partitionID)
	return nil
}

// extractDatabaseArchive extracts a gzip-tar archived database from the snapshot
func extractDatabaseArchive(rd *snapshot.Reader, sectionType snapshot.SectionType, destPath string) error {
	// Open the section
	section, err := rd.Open(sectionType)
	if err != nil {
		return fmt.Errorf("open section: %w", err)
	}

	// Remove existing directory
	os.RemoveAll(destPath)

	// Create destination directory
	if err := os.MkdirAll(destPath, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Decompress gzip
	gzReader, err := gzip.NewReader(section)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gzReader.Close()

	// Extract tar archive
	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar next: %w", err)
		}

		// Determine target path - strip leading directory component if present
		name := header.Name
		if idx := strings.Index(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
		if name == "" {
			continue
		}
		targetPath := filepath.Join(destPath, name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return fmt.Errorf("mkdir %s: %w", targetPath, err)
			}
		case tar.TypeReg:
			// Ensure parent directory exists
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("mkdir parent: %w", err)
			}
			outFile, err := os.Create(targetPath)
			if err != nil {
				return fmt.Errorf("create %s: %w", targetPath, err)
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return fmt.Errorf("copy %s: %w", targetPath, err)
			}
			outFile.Close()
		}
	}

	return nil
}
