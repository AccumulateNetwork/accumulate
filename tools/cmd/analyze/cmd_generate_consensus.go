// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/cometbft/cometbft/types"
	crypted25519 "github.com/cometbft/cometbft/crypto/ed25519"
	stded25519 "crypto/ed25519"
)

var cmdGenerateConsensusSection = &cobra.Command{
	Use:   "generate-consensus-section <network-config-file> <partition-id> <output-file>",
	Short: "Generate CometBFT-compatible consensus section for a specific partition",
	Long: `Generate a standalone consensus section JSON file for a specific partition.
This creates a CometBFT GenesisDoc structure containing validators and configuration
for the specified partition, which can be embedded in partition snapshots.

Arguments:
  network-config-file  Path to network configuration JSON file
  partition-id         Partition ID to generate consensus section for
  output-file          Output file path for consensus section JSON`,
	Args: cobra.ExactArgs(3),
	RunE: generateConsensusSection,
}

func init() {
	// No flags needed - using positional arguments
}

func generateConsensusSection(cmd *cobra.Command, args []string) error {
	// Parse positional arguments
	flagNetworkConfig := args[0]
	flagPartition := args[1]
	flagOutput := args[2]
	
	// Parse network configuration using existing function
	networkConfig, err := ParseNetworkJson(flagNetworkConfig)
	if err != nil {
		return fmt.Errorf("failed to parse network config: %w", err)
	}

	fmt.Printf("Looking for partition: %q\n", flagPartition)

	// Find the target partition
	var targetPartition *struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}

	for i := range networkConfig.Globals.Network.Partitions {
		if networkConfig.Globals.Network.Partitions[i].ID == flagPartition {
			targetPartition = &networkConfig.Globals.Network.Partitions[i]
			break
		}
	}

	if targetPartition == nil {
		return fmt.Errorf("partition not found: %s", flagPartition)
	}

	fmt.Printf("Generating consensus section for partition: %s\n", flagPartition)
	fmt.Printf("Partition type: %s\n", targetPartition.Type)

	// Determine which validators are active for this partition
	var activeValidators []struct {
		Operator  string `json:"operator"`
		PublicKey string `json:"publicKey"`
	}

	// Find validators that are assigned to this partition
	for _, netValidator := range networkConfig.Globals.Network.Validators {
		// Check if this validator is assigned to the target partition
		for _, partition := range netValidator.Partitions {
			if partition.ID == flagPartition && partition.Active {
				activeValidators = append(activeValidators, struct {
					Operator  string `json:"operator"`
					PublicKey string `json:"publicKey"`
				}{
					Operator:  netValidator.Operator,
					PublicKey: netValidator.PublicKey,
				})
				break
			}
		}
	}
	fmt.Printf("Found %d active validators for partition %s\n", len(activeValidators), flagPartition)

	if len(activeValidators) == 0 {
		return fmt.Errorf("no validators found for partition: %s", flagPartition)
	}

	// Create CometBFT validators
	var cometValidators []types.GenesisValidator
	for _, validator := range activeValidators {
		if validator.PublicKey == "" {
			return fmt.Errorf("validator %s missing public key", validator.Operator)
		}

		// Parse the public key (hex string)
		pubKeyBytes, err := hex.DecodeString(validator.PublicKey)
		if err != nil {
			return fmt.Errorf("failed to decode public key for validator %s: %w", validator.Operator, err)
		}
		if len(pubKeyBytes) != stded25519.PublicKeySize {
			return fmt.Errorf("invalid ed25519 public key length for validator %s", validator.Operator)
		}
		cometPubKey := crypted25519.PubKey(pubKeyBytes)

		// Create CometBFT validator
		cometValidator := types.GenesisValidator{
			Address: cometPubKey.Address(),
			PubKey:  cometPubKey,
			Power:   1, // Equal voting power for all validators
			Name:    validator.Operator,
		}

		cometValidators = append(cometValidators, cometValidator)
		fmt.Printf("Added validator: %s (address: %s)\n", validator.Operator, cometValidator.Address)
	}

	// Create consensus section (CometBFT GenesisDoc)
	consensusSection := &types.GenesisDoc{
		ChainID:         fmt.Sprintf("cyclops.%s", flagPartition),
		GenesisTime:     time.Now().UTC(),
		ConsensusParams: types.DefaultConsensusParams(),
		Validators:      cometValidators,
		AppHash:         nil,
		AppState:        nil,
	}

	fmt.Printf("Created consensus section with chain ID: %s\n", consensusSection.ChainID)
	fmt.Printf("Consensus section contains %d validators\n", len(consensusSection.Validators))

	// Validate the consensus section
	if err := consensusSection.ValidateAndComplete(); err != nil {
		return fmt.Errorf("consensus section validation failed: %w", err)
	}

	// Marshal to JSON
	consensusJSON, err := json.MarshalIndent(consensusSection, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal consensus section: %w", err)
	}

	// Ensure output directory exists
	outputDir := filepath.Dir(flagOutput)
	if outputDir != "." {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Write to output file
	if err := os.WriteFile(flagOutput, consensusJSON, 0644); err != nil {
		return fmt.Errorf("failed to write consensus section: %w", err)
	}

	fmt.Printf("Successfully wrote consensus section to: %s\n", flagOutput)
	return nil
}
