//go:build dagbft

// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// cmdInit returns the init command for initializing a single node.
func cmdInit() *cobra.Command {
	var (
		dataDir      string
		validatorKey string
		p2pPort      int
		apiPort      int
		genesis      string
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a DAG-BFT node",
		Long:  `Initialize a single DAG-BFT node with the specified configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dataDir == "" {
				return fmt.Errorf("--dir is required")
			}
			if genesis == "" {
				return fmt.Errorf("--genesis is required")
			}

			// Resolve paths
			dataDir, err := filepath.Abs(dataDir)
			if err != nil {
				return fmt.Errorf("resolve data dir: %w", err)
			}
			genesis, err = filepath.Abs(genesis)
			if err != nil {
				return fmt.Errorf("resolve genesis: %w", err)
			}

			// Create data directory
			if err := os.MkdirAll(dataDir, 0755); err != nil {
				return fmt.Errorf("create data dir: %w", err)
			}

			// Load genesis
			genesisDoc, err := devnet.LoadGenesis(genesis)
			if err != nil {
				return fmt.Errorf("load genesis: %w", err)
			}

			fmt.Printf("Node initialized at %s\n", dataDir)
			fmt.Printf("Genesis: %s (validators: %d)\n", genesisDoc.ChainID, len(genesisDoc.Validators))

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "dir", "", "Node data directory")
	cmd.Flags().StringVar(&validatorKey, "validator-key", "", "Path to validator private key")
	cmd.Flags().IntVar(&p2pPort, "p2p-port", 9000, "P2P listen port")
	cmd.Flags().IntVar(&apiPort, "api-port", 9001, "API listen port")
	cmd.Flags().StringVar(&genesis, "genesis", "", "Path to genesis file")

	return cmd
}
