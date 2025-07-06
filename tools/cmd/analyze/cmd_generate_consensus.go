// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	cometbft "github.com/cometbft/cometbft/types"
)

var cmdGenerateConsensusSection = &cobra.Command{
	Use:   "generate-consensus-section",
	Short: "Generate CometBFT-compatible consensus section for a specific partition",
	Long: `Generate a standalone consensus section JSON file for a specific partition.
This creates a CometBFT GenesisDoc structure containing validators and configuration
for the specified partition, which can be embedded in partition snapshots.`,
	Args: cobra.NoArgs,
	RunE: generateConsensusSection,
}

var (
	flagNetworkConfig string
	flagPartition     string
	flagOutput        string
)

func init() {
	cmdGenerateConsensusSection.Flags().StringVar(&flagNetworkConfig, "network-config", "", "Path to network configuration JSON file")
	cmdGenerateConsensusSection.Flags().StringVar(&flagPartition, "partition", "", "Partition ID to generate consensus section for")
	cmdGenerateConsensusSection.Flags().StringVar(&flagOutput, "output", "", "Output file path for consensus section JSON")
	
	cmdGenerateConsensusSection.MarkFlagRequired("network-config")
	cmdGenerateConsensusSection.MarkFlagRequired("partition")
	cmdGenerateConsensusSection.MarkFlagRequired("output")
}

func generateConsensusSection(cmd *cobra.Command, args []string) error {
	// Read network configuration
	networkData, err := os.ReadFile(flagNetworkConfig)
	if err != nil {
		return fmt.Errorf("failed to read network config: %w", err)
	}

	// Parse network configuration
	var networkConfig struct {
		Network struct {
			Partitions []struct {
				ID         string `json:"id"`
				Type       string `json:"type"`
				Validators []struct {
					Operator  string `json:"operator"`
					PublicKey string `json:"publicKey"`
				} `json:"validators"`
			} `json:"partitions"`
			Validators []struct {
				Operator  string `json:"operator"`
				PublicKey string `json:"publicKey"`
			} `json:"validators"`
		} `json:"network"`
	}

	if err := json.Unmarshal(networkData, &networkConfig); err != nil {
		return fmt.Errorf("failed to parse network config: %w", err)
	}

	// Find the target partition
	var targetPartition *struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Validators []struct {
			Operator  string `json:"operator"`
			PublicKey string `json:"publicKey"`
		} `json:"validators"`
	}

	for i := range networkConfig.Network.Partitions {
		if networkConfig.Network.Partitions[i].ID == flagPartition {
			targetPartition = &networkConfig.Network.Partitions[i]
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

	// If partition has specific validators, use those
	if len(targetPartition.Validators) > 0 {
		activeValidators = targetPartition.Validators
		fmt.Printf("Using partition-specific validators: %d\n", len(activeValidators))
	} else {
		// Otherwise, use all network validators (for Directory Network)
		activeValidators = networkConfig.Network.Validators
		fmt.Printf("Using all network validators: %d\n", len(activeValidators))
	}

	if len(activeValidators) == 0 {
		return fmt.Errorf("no validators found for partition: %s", flagPartition)
	}

	// Create CometBFT validators
	var cometValidators []cometbft.GenesisValidator
	for _, validator := range activeValidators {
		if validator.PublicKey == "" {
			return fmt.Errorf("validator %s missing public key", validator.Operator)
		}

		// Parse the public key
		pubKeyBytes, err := encoding.ParsePublicKey(validator.PublicKey)
		if err != nil {
			return fmt.Errorf("failed to parse public key for validator %s: %w", validator.Operator, err)
		}

		// Create CometBFT validator
		cometValidator := cometbft.GenesisValidator{
			Address: pubKeyBytes.Address(),
			PubKey:  pubKeyBytes,
			Power:   1, // Equal voting power for all validators
			Name:    validator.Operator,
		}

		cometValidators = append(cometValidators, cometValidator)
		fmt.Printf("Added validator: %s (address: %s)\n", validator.Operator, cometValidator.Address)
	}

	// Create consensus section (CometBFT GenesisDoc)
	consensusSection := &cometbft.GenesisDoc{
		ChainID:         fmt.Sprintf("cyclops.%s", flagPartition),
		GenesisTime:     cometbft.DefaultGenesisDocFromMap(nil).GenesisTime, // Use default genesis time
		ConsensusParams: cometbft.DefaultConsensusParams(),
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
