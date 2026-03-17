// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/config"
)

// cmdDAGBFT is the main command group for DAG-BFT consensus commands.
var cmdDAGBFT = &cobra.Command{
	Use:   "dagbft",
	Short: "DAG-BFT consensus commands",
	Long:  `Commands for managing and operating DAG-BFT consensus nodes.`,
}

// cmdDAGBFTKeygen generates node keys for DAG-BFT.
var cmdDAGBFTKeygen = &cobra.Command{
	Use:   "keygen",
	Short: "Generate node keys",
	Long:  `Generate ed25519 key pair for a DAG-BFT consensus node.`,
	Run: func(cmd *cobra.Command, args []string) {
		out, err := runDAGBFTKeygen(cmd, args)
		printOutput(cmd, out, err)
	},
}

// cmdDAGBFTConfig handles configuration generation and validation.
var cmdDAGBFTConfig = &cobra.Command{
	Use:   "config",
	Short: "Generate or validate DAG-BFT configuration",
	Long:  `Generate an example configuration file or validate an existing one.`,
	Run: func(cmd *cobra.Command, args []string) {
		out, err := runDAGBFTConfig(cmd, args)
		printOutput(cmd, out, err)
	},
}

var flagDAGBFTKeygen struct {
	Output string
	Format string
}

var flagDAGBFTConfig struct {
	Output   string
	Validate string
	JSON     bool
}

func init() {
	cmdMain.AddCommand(cmdDAGBFT)
	cmdDAGBFT.AddCommand(cmdDAGBFTKeygen)
	cmdDAGBFT.AddCommand(cmdDAGBFTConfig)

	// Keygen flags
	cmdDAGBFTKeygen.Flags().StringVarP(&flagDAGBFTKeygen.Output, "output", "o", "", "Output directory for key files (default: current directory)")
	cmdDAGBFTKeygen.Flags().StringVarP(&flagDAGBFTKeygen.Format, "format", "f", "text", "Output format: text, json")

	// Config flags
	cmdDAGBFTConfig.Flags().StringVarP(&flagDAGBFTConfig.Output, "output", "o", "", "Output file for generated config")
	cmdDAGBFTConfig.Flags().StringVar(&flagDAGBFTConfig.Validate, "validate", "", "Path to config file to validate")
	cmdDAGBFTConfig.Flags().BoolVar(&flagDAGBFTConfig.JSON, "json", false, "Output in JSON format")
}

// NodeKeys represents ed25519 key pair for a node.
type NodeKeys struct {
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

func runDAGBFTKeygen(cmd *cobra.Command, _ []string) (string, error) {
	// Generate ed25519 key pair
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", fmt.Errorf("failed to generate key pair: %w", err)
	}

	pubHex := hex.EncodeToString(pub)
	privHex := hex.EncodeToString(priv)

	keys := NodeKeys{
		PublicKey:  pubHex,
		PrivateKey: privHex,
	}

	// Determine output location
	outputDir := flagDAGBFTKeygen.Output
	if outputDir == "" {
		outputDir = "."
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write key files
	pubKeyPath := filepath.Join(outputDir, "node_key.pub")
	privKeyPath := filepath.Join(outputDir, "node_key")

	if err := os.WriteFile(pubKeyPath, []byte(pubHex+"\n"), 0644); err != nil {
		return "", fmt.Errorf("failed to write public key: %w", err)
	}

	if err := os.WriteFile(privKeyPath, []byte(privHex+"\n"), 0600); err != nil {
		return "", fmt.Errorf("failed to write private key: %w", err)
	}

	// Output based on format
	if flagDAGBFTKeygen.Format == "json" {
		data, err := json.MarshalIndent(keys, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal keys: %w", err)
		}
		return string(data), nil
	}

	return fmt.Sprintf("Generated node keys:\n  Public key:  %s\n  Private key: %s\n  Files written to: %s", pubHex, privKeyPath, outputDir), nil
}

func runDAGBFTConfig(cmd *cobra.Command, _ []string) (string, error) {
	// Validate existing config
	if flagDAGBFTConfig.Validate != "" {
		cfg, err := config.LoadFile(flagDAGBFTConfig.Validate)
		if err != nil {
			return "", fmt.Errorf("validation failed: %w", err)
		}

		if flagDAGBFTConfig.JSON {
			data, err := json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				return "", fmt.Errorf("failed to marshal config: %w", err)
			}
			return string(data), nil
		}

		return fmt.Sprintf("Configuration is valid: %s", flagDAGBFTConfig.Validate), nil
	}

	// Generate example config
	exampleConfig := config.ExampleConfig()

	// Output to file or stdout
	if flagDAGBFTConfig.Output != "" {
		if err := os.WriteFile(flagDAGBFTConfig.Output, []byte(exampleConfig), 0644); err != nil {
			return "", fmt.Errorf("failed to write config file: %w", err)
		}
		return fmt.Sprintf("Example configuration written to: %s", flagDAGBFTConfig.Output), nil
	}

	return exampleConfig, nil
}
