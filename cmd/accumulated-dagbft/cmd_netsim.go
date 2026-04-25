//go:build dagbft

// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated-dagbft/netsim"
)

// cmdDevnet returns the devnet command group.
func cmdDevnet() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "devnet",
		Short: "Manage DAG-BFT devnet",
		Long:  `Commands for initializing and running a local DAG-BFT devnet.`,
	}

	cmd.AddCommand(
		cmdDevnetInit(),
		cmdDevnetRun(),
		cmdDevnetStatus(),
	)

	return cmd
}

// cmdDevnetInit returns the devnet init command.
func cmdDevnetInit() *cobra.Command {
	var (
		dataDir    string
		numNodes   int
		basePort   int
		network    string
		numWorkers int
		seed       string
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a DAG-BFT devnet",
		Long: `Initialize a multi-node DAG-BFT devnet for testing.

Supported node counts: 3, 4, 5, 7, 10, 13 (BFT quorum requirements)

Example:
  accumulated-dagbft devnet init --nodes 7 --dir /tmp/dagbft-devnet`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dataDir == "" {
				return fmt.Errorf("--dir is required")
			}

			// Resolve path
			dataDir, err := filepath.Abs(dataDir)
			if err != nil {
				return fmt.Errorf("resolve data dir: %w", err)
			}

			// Create configuration
			config := devnet.NewDevnetConfig()
			config.DataDir = dataDir
			config.NumNodes = numNodes
			config.BasePort = basePort
			config.Network = network
			config.NumWorkers = numWorkers

			// Generate or use provided seed
			if seed != "" {
				copy(config.Seed[:], []byte(seed))
			} else {
				if _, err := rand.Read(config.Seed[:]); err != nil {
					return fmt.Errorf("generate seed: %w", err)
				}
			}

			// Apply defaults
			config.ApplyDefaults()

			// Validate
			if err := config.Validate(); err != nil {
				return fmt.Errorf("invalid configuration: %w", err)
			}

			// Create base directory
			if err := os.MkdirAll(dataDir, 0755); err != nil {
				return fmt.Errorf("create data dir: %w", err)
			}

			// Generate node configurations
			if err := config.GenerateNodeConfigs(); err != nil {
				return fmt.Errorf("generate node configs: %w", err)
			}

			// Generate genesis
			genesis, err := devnet.NewGenesisDoc(config)
			if err != nil {
				return fmt.Errorf("create genesis: %w", err)
			}

			// Validate genesis
			if err := genesis.Validate(); err != nil {
				return fmt.Errorf("invalid genesis: %w", err)
			}

			// Initialize each node
			fmt.Printf("Initializing %d-node devnet in %s\n", numNodes, dataDir)
			for i, node := range config.Nodes {
				fmt.Printf("  [%d/%d] Initializing %s...\n", i+1, numNodes, node.ID)
				if err := node.Initialize(genesis); err != nil {
					return fmt.Errorf("initialize %s: %w", node.ID, err)
				}
			}

			// Save devnet configuration
			configPath := devnet.ConfigPath(dataDir)
			if err := config.Save(configPath); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			// Print summary
			fmt.Println()
			fmt.Println("Devnet initialized successfully!")
			fmt.Printf("  Network:    %s\n", config.Network)
			fmt.Printf("  Nodes:      %d\n", config.NumNodes)
			fmt.Printf("  Base port:  %d\n", config.BasePort)
			fmt.Printf("  Data dir:   %s\n", config.DataDir)
			fmt.Println()
			fmt.Println("Validator public keys:")
			for _, node := range config.Nodes {
				fmt.Printf("  %s: %s\n", node.ID, node.ValidatorKeyHex()[:32]+"...")
			}
			fmt.Println()
			fmt.Println("To start the devnet, run:")
			fmt.Printf("  accumulated-dagbft devnet run --dir %s\n", dataDir)

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "dir", "", "Base data directory for devnet (required)")
	cmd.Flags().IntVar(&numNodes, "nodes", 7, "Number of validator nodes (3, 4, 5, 7, 10, or 13)")
	cmd.Flags().IntVar(&basePort, "base-port", 9000, "Starting port number")
	cmd.Flags().StringVar(&network, "network", "dagbft-devnet", "Network ID")
	cmd.Flags().IntVar(&numWorkers, "workers", 4, "Number of workers per node")
	cmd.Flags().StringVar(&seed, "seed", "", "Deterministic seed for key generation (optional)")

	return cmd
}

// cmdDevnetStatus returns the devnet status command.
func cmdDevnetStatus() *cobra.Command {
	var dataDir string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show devnet status",
		Long:  `Display the status of a running DAG-BFT devnet.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dataDir == "" {
				return fmt.Errorf("--dir is required")
			}

			// Load configuration
			configPath := devnet.ConfigPath(dataDir)
			config, err := devnet.Load(configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			fmt.Printf("Devnet: %s\n", config.Network)
			fmt.Printf("Nodes: %d\n", config.NumNodes)
			fmt.Println()
			fmt.Println("Node Status:")
			for _, node := range config.Nodes {
				peerAddr, _ := node.PeerMultiaddr()
				fmt.Printf("  %s:\n", node.ID)
				fmt.Printf("    P2P:  %s\n", peerAddr)
				fmt.Printf("    API:  http://127.0.0.1:%d\n", node.APIPort)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "dir", "", "Devnet data directory")

	return cmd
}
