// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/cometbft"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	sv2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
)

// WriteConsensusSection writes a consensus section to the snapshot based on network configuration
func WriteConsensusSection(writer *sv2.Writer, extractState *ExtractState, targetPartition string) error {
	fmt.Printf("Creating consensus section for partition %s...\n", targetPartition)

	// Create a new consensus section
	section, err := writer.OpenRaw(sv2.SectionTypeConsensus)
	if err != nil {
		return fmt.Errorf("create consensus section: %w", err)
	}
	defer section.Close()

	// Create an Accumulate cometbft.GenesisDoc (not standard CometBFT types.GenesisDoc)
	doc := &cometbft.GenesisDoc{
		ChainID:    fmt.Sprintf("cyclops.%s", targetPartition),
		Validators: []*cometbft.Validator{},
		// Note: Params and Block fields can be nil for basic functionality
	}

	// Process validator keys from command line arguments
	if len(extractState.ValidatorKeys) > 0 {
		fmt.Printf("Processing %d validator keys from command line...\n", len(extractState.ValidatorKeys))
		for i, keyHex := range extractState.ValidatorKeys {
			// Decode hex key
			keyBytes, err := hex.DecodeString(keyHex)
			if err != nil {
				fmt.Printf("Warning: Failed to decode validator key %d: %v\n", i, err)
				continue
			}

			if len(keyBytes) != 32 {
				fmt.Printf("Warning: Validator key %d has invalid length %d (expected 32)\n", i, len(keyBytes))
				continue
			}

			// Create Accumulate validator
			validator := &cometbft.Validator{
				Address: keyBytes[:20], // Use first 20 bytes as address
				Type:    protocol.SignatureTypeED25519,
				PubKey:  keyBytes,
				Power:   1,
				Name:    fmt.Sprintf("validator-%d", i),
			}

			doc.Validators = append(doc.Validators, validator)
			fmt.Printf("Added validator %d: %s (power: %d)\n", i, validator.Name, validator.Power)
		}
	}

	// Process validator keys from network configuration
	if extractState.NetworkConfig != nil {
		fmt.Printf("Processing validator keys from network configuration...\n")
		for _, validator := range extractState.NetworkConfig.Globals.Network.Validators {
			// Check if this network matches our target partition
			isActiveForPartition := false
			for _, p := range validator.Partitions {
				if p.ID == targetPartition && p.Active {
					isActiveForPartition = true
					break
				}
			}

			if !isActiveForPartition {
				fmt.Printf("Skipping validator %s: not active for partition %s\n", validator.Operator, targetPartition)
				continue
			}

			if validator.PublicKey == "" {
				fmt.Printf("Node has no validator key, skipping\n")
				continue
			}

			// Try to decode validator key - first as hex, then as base64
			var keyBytes []byte
			var err error
			
			// Try hex decoding first (network JSON typically uses hex format)
			keyBytes, err = hex.DecodeString(validator.PublicKey)
			if err != nil {
				// If hex fails, try base64 decoding (priv_validator_key.json uses base64)
				keyBytes, err = base64.StdEncoding.DecodeString(validator.PublicKey)
				if err != nil {
					fmt.Printf("Warning: Failed to decode validator key for node (tried both hex and base64): %v\n", err)
					continue
				}
				fmt.Printf("Debug: Decoded validator key as base64 (length: %d)\n", len(keyBytes))
			} else {
				fmt.Printf("Debug: Decoded validator key as hex (length: %d)\n", len(keyBytes))
			}

			if len(keyBytes) != 32 {
				fmt.Printf("Warning: Node validator key has invalid length %d (expected 32)\n", len(keyBytes))
				continue
			}

			// Create Accumulate validator
			validator := &cometbft.Validator{
				Address: keyBytes[:20], // Use first 20 bytes as address
				Type:    protocol.SignatureTypeED25519,
				PubKey:  keyBytes,
				Power:   1,
				Name:    fmt.Sprintf("%s-node", validator.Operator),
			}

			doc.Validators = append(doc.Validators, validator)
			fmt.Printf("Added validator from network config: %s (power: %d)\n", validator.Name, validator.Power)
		}
	}

	// Check if we have any validators
	if len(doc.Validators) == 0 {
		fmt.Printf("No validator keys provided via command line or network config, aborting consensus section creation\n")
		return fmt.Errorf("no validators configured for consensus section")
	}

	fmt.Printf("Created Accumulate GenesisDoc with %d validators for chain %s\n", len(doc.Validators), doc.ChainID)

	// Marshal the Accumulate GenesisDoc to JSON
	jsonData, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal accumulate genesis doc: %w", err)
	}

	// Write consensus JSON to file for inspection
	var consensusFileName string
	if targetPartition == "dn" {
		consensusFileName = "consensus_dn.json"
	} else if targetPartition == "bvn-cyclops" {
		consensusFileName = "consensus_bvn.json"
	} else {
		consensusFileName = fmt.Sprintf("consensus_%s.json", targetPartition)
	}
	
	// Pretty print the JSON for better readability
	var prettyJSON []byte
	prettyJSON, err = json.MarshalIndent(doc, "", "  ")
	if err != nil {
		fmt.Printf("Warning: Failed to pretty print JSON for %s: %v\n", consensusFileName, err)
		prettyJSON = jsonData // Fall back to compact JSON
	}
	
	err = os.WriteFile(consensusFileName, prettyJSON, 0644)
	if err != nil {
		fmt.Printf("Warning: Failed to write consensus JSON file %s: %v\n", consensusFileName, err)
	} else {
		fmt.Printf("Wrote consensus data to %s for inspection\n", consensusFileName)
	}

	// Write the consensus section to snapshot
	_, err = section.Write(jsonData)
	if err != nil {
		return fmt.Errorf("write consensus section: %w", err)
	}

	fmt.Printf("Successfully created consensus section for partition %s with %d validators\n", targetPartition, len(doc.Validators))
	return nil
}
