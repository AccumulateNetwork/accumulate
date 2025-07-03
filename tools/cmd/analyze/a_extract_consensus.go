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

        tmed25519 "github.com/cometbft/cometbft/crypto/ed25519"
        types "github.com/cometbft/cometbft/types"
        sv2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
)

// WriteConsensusSection writes a consensus section to the snapshot based on network configuration
func WriteConsensusSection(writer *sv2.Writer, extractState *ExtractState, targetPartition string) error {
        fmt.Printf("Creating consensus section for partition %s...\n", targetPartition)

        // Create a new consensus section
        consensusSection, err := writer.OpenRaw(sv2.SectionTypeConsensus)
        if err != nil {
                return fmt.Errorf("create consensus section: %w", err)
        }
        defer consensusSection.Close()

        // Create a new CometBFT GenesisDoc
        doc := types.GenesisDoc{}

        // Set the chain ID based on the network ID and partition ID
        networkID := "accumulate" // Default network name
        // Get network name from config if available
        if extractState.NetworkConfig != nil {
                // First try the top-level ID field
                if extractState.NetworkConfig.ID != "" {
                        networkID = extractState.NetworkConfig.ID
                        fmt.Printf("Using network ID from config: %s\n", networkID)
                } else if extractState.NetworkConfig.Globals.Network.NetworkName != "" {
                        // Fall back to NetworkName if available
                        networkID = extractState.NetworkConfig.Globals.Network.NetworkName
                        fmt.Printf("Using network name from config: %s\n", networkID)
                }
        }

        doc.ChainID = networkID + "." + targetPartition

        // Set default consensus parameters
        doc.ConsensusParams = types.DefaultConsensusParams()
        // Customize consensus parameters
        doc.ConsensusParams.Block.MaxBytes = 22020096 // Default max block size
        doc.ConsensusParams.Block.MaxGas = -1         // No gas limit

        // First check if validator keys were provided via command line
        if len(extractState.ValidatorKeys) > 0 {
                fmt.Printf("Using %d validator keys provided via command line\n", len(extractState.ValidatorKeys))
                for i, pubKeyStr := range extractState.ValidatorKeys {
                        // Decode the public key
                        pubKeyBytes, err := hex.DecodeString(pubKeyStr)
                        if err != nil {
                                fmt.Printf("Warning: Failed to decode validator public key %s: %v\n", pubKeyStr, err)
                                continue
                        }

                        // Validate the key length for ED25519
                        if len(pubKeyBytes) != 32 {
                                fmt.Printf("Warning: Invalid ED25519 public key length %d (expected 32 bytes): %s\n", len(pubKeyBytes), pubKeyStr)
                                continue
                        }

                        // Create the validator entry
                        key := tmed25519.PubKey(pubKeyBytes)
                        name := fmt.Sprintf("Validator-%d-%s", i+1, targetPartition)

                        // Add the validator to the genesis doc
                        val := types.GenesisValidator{
                                Address: key.Address(),
                                PubKey:  key,
                                Power:   1, // All validators have equal voting power
                                Name:    name,
                        }
                        doc.Validators = append(doc.Validators, val)
                }
        } else if extractState.NetworkConfig != nil && 
                len(extractState.NetworkConfig.Globals.Network.Validators) > 0 {
                // Try to get validator keys from network config
                validators := extractState.NetworkConfig.Globals.Network.Validators
                fmt.Printf("No validator keys provided via command line, using %d validators from network config\n", 
                        len(validators))
                
                for _, validator := range validators {
                        // Check if this validator is active for the target partition
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
                        
                        // Use the validator's public key
                        pubKeyStr := validator.PublicKey
                        if pubKeyStr == "" {
                                fmt.Printf("Warning: Validator %s has no public key\n", validator.Operator)
                                continue
                        }
                        
                        // Decode the public key
                        pubKeyBytes, err := hex.DecodeString(pubKeyStr)
                        if err != nil {
                                fmt.Printf("Warning: Failed to decode validator public key %s: %v\n", pubKeyStr, err)
                                continue
                        }

                        // Validate the key length for ED25519
                        if len(pubKeyBytes) != 32 {
                                fmt.Printf("Warning: Invalid ED25519 public key length %d (expected 32 bytes): %s\n", len(pubKeyBytes), pubKeyStr)
                                continue
                        }

                        // Create the validator entry
                        key := tmed25519.PubKey(pubKeyBytes)
                        name := fmt.Sprintf("Validator-%s-%s", validator.Operator, targetPartition)

                        // Add the validator to the genesis doc
                        val := types.GenesisValidator{
                                Address: key.Address(),
                                PubKey:  key,
                                Power:   1, // All validators have equal voting power
                                Name:    name,
                        }
                        doc.Validators = append(doc.Validators, val)
                }
        } else {
                // No validator keys provided via CLI or network config
                fmt.Printf("No validator keys provided via command line or network config, aborting consensus section creation\n")
                return fmt.Errorf("no validator keys provided for partition %s", targetPartition)
        }

        // If no valid validators were found, abort
        if len(doc.Validators) == 0 {
                return fmt.Errorf("no valid validator keys provided for partition %s", targetPartition)
        }

        // Marshal the genesis doc to JSON
        b, err := json.Marshal(doc)
        if err != nil {
                return fmt.Errorf("marshal consensus doc: %w", err)
        }

        // Write the consensus section
        _, err = consensusSection.Write(b)
        if err != nil {
                return fmt.Errorf("write consensus section: %w", err)
        }

        fmt.Printf("Successfully created consensus section for partition %s with %d validators\n", targetPartition, len(doc.Validators))
        return nil
}
