// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// NodeKey represents the structure of a Tendermint node_key.json file
type NodeKey struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

var cmdGenerateNodeKey = &cobra.Command{
	Use:   "generate-node-key [output-file]",
	Short: "Generate a node_key.json file for Tendermint",
	Long:  "Generate a Tendermint node key file. If no output file is specified, writes to node_key.json in current directory.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  generateNodeKey,
}

func generateNodeKey(cmd *cobra.Command, args []string) error {
	// Determine output file
	outputFile := "node_key.json"
	if len(args) > 0 {
		outputFile = args[0]
	}

	// Generate Ed25519 private key
	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		return fmt.Errorf("failed to generate Ed25519 key: %w", err)
	}

	// Create node key structure
	nodeKey := NodeKey{
		Type:  "tendermint/PrivKeyEd25519",
		Value: base64.StdEncoding.EncodeToString(privateKey),
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(nodeKey, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Ensure output directory exists
	outputDir := filepath.Dir(outputFile)
	if outputDir != "." {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Write to file with secure permissions
	err = os.WriteFile(outputFile, jsonData, 0600)
	if err != nil {
		return fmt.Errorf("failed to write node key file: %w", err)
	}

	fmt.Printf("Successfully generated node key file: %s\n", outputFile)
	fmt.Printf("Private key length: %d bytes\n", len(privateKey))
	fmt.Printf("File permissions: 0600 (read/write owner only)\n")
	
	return nil
}

func init() {
	rootCmd.AddCommand(cmdGenerateNodeKey)
}
