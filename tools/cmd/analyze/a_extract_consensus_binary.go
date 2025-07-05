// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/hex"
	"fmt"

	tmed25519 "github.com/cometbft/cometbft/crypto/ed25519"
	tmtypes "github.com/cometbft/cometbft/types"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/cometbft"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	sv2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/core/schema/pkg/binary"
)

// WriteBinaryConsensusSection writes a consensus section to the snapshot in binary format
// This matches the format used by the node in genesis.Init
func WriteBinaryConsensusSection(writer *sv2.Writer, extractState *ExtractState, targetPartition string) error {
	// Open the consensus section
	sw, err := writer.OpenRaw(sv2.SectionTypeConsensus)
	if err != nil {
		return fmt.Errorf("open consensus section: %w", err)
	}
	// Note: We'll close this explicitly after writing, not using defer

	// Create a new CometBFT GenesisDoc
	doc := &cometbft.GenesisDoc{}

	// Set the chain ID based on the network ID and partition ID
	networkID := "accumulate" // Default network name
	if extractState.NetworkConfig != nil {
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

	// Create consensus parameters using tmtypes
	params := tmtypes.DefaultConsensusParams()
	params.Block.MaxBytes = 22020096 // Default max block size
	params.Block.MaxGas = -1        // No gas limit
	
	// Convert to cometbft.ConsensusParams
	doc.Params = (*cometbft.ConsensusParams)(params)

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
			val := &cometbft.Validator{
				Address: key.Address(),
				PubKey:  pubKeyBytes,
				Power:   1, // All validators have equal voting power
				Name:    name,
				Type:    protocol.SignatureTypeED25519,
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
			val := &cometbft.Validator{
				Address: key.Address(),
				PubKey:  pubKeyBytes,
				Power:   1, // All validators have equal voting power
				Name:    name,
				Type:    protocol.SignatureTypeED25519,
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

	// Use the same marshaling approach as the node uses when reading
	// Instead of using MarshalBinary, we'll use MarshalBinaryV2 with a binary.Encoder
	// This ensures the format matches exactly what the node expects when unmarshaling
	// with UnmarshalBinaryFrom
	enc := binary.NewEncoder(sw)
	err = doc.MarshalBinaryV2(enc)
	if err != nil {
		return fmt.Errorf("marshal consensus doc to binary: %w", err)
	}

	// Debug: Print information about the binary data
	fmt.Printf("Successfully encoded consensus data using binary.Encoder\n")

	// Close the consensus section explicitly
	err = sw.Close()
	if err != nil {
		return fmt.Errorf("close consensus section: %w", err)
	}

	fmt.Printf("Successfully wrote binary consensus section for partition %s\n", targetPartition)
	return nil
}

// WriteConsensusDirectly writes a consensus section directly to the snapshot
// using the same approach as the node's genesis.Init function
func WriteConsensusDirectly(writer *sv2.Writer, extractState *ExtractState, targetPartition string) error {
	// Create a new CometBFT GenesisDoc
	doc := &cometbft.GenesisDoc{}

	// Set the chain ID based on the network ID and partition ID
	networkID := "accumulate" // Default network name
	if extractState.NetworkConfig != nil {
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

	// Create consensus parameters using tmtypes
	params := tmtypes.DefaultConsensusParams()
	params.Block.MaxBytes = 22020096 // Default max block size
	params.Block.MaxGas = -1        // No gas limit
	
	// Convert to cometbft.ConsensusParams
	doc.Params = (*cometbft.ConsensusParams)(params)

	// Add validators from network config
	if extractState.NetworkConfig != nil && 
		len(extractState.NetworkConfig.Globals.Network.Validators) > 0 {
		// Try to get validator keys from network config
		validators := extractState.NetworkConfig.Globals.Network.Validators
		fmt.Printf("Using %d validators from network config\n", len(validators))
		
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

			// Create the validator entry
			key := tmed25519.PubKey(pubKeyBytes)
			name := fmt.Sprintf("Validator-%s-%s", validator.Operator, targetPartition)

			// Add the validator to the genesis doc
			val := &cometbft.Validator{
				Address: key.Address(),
				PubKey:  pubKeyBytes,
				Power:   1, // All validators have equal voting power
				Name:    name,
				Type:    protocol.SignatureTypeED25519,
			}
			doc.Validators = append(doc.Validators, val)
		}
	}

	// If no valid validators were found, add a default validator
	if len(doc.Validators) == 0 {
		fmt.Printf("No valid validators found, adding a default validator\n")
		// Create a default validator with a dummy key
		pubKeyBytes := make([]byte, 32)
		key := tmed25519.PubKey(pubKeyBytes)
		name := fmt.Sprintf("Default-Validator-%s", targetPartition)

		// Add the validator to the genesis doc
		val := &cometbft.Validator{
			Address: key.Address(),
			PubKey:  pubKeyBytes,
			Power:   1,
			Name:    name,
			Type:    protocol.SignatureTypeED25519,
		}
		doc.Validators = append(doc.Validators, val)
	}

	// Marshal the genesis doc to binary format
	b, err := doc.MarshalBinary()
	if err != nil {
		return fmt.Errorf("marshal consensus doc to binary: %w", err)
	}
	
	// Debug: Print information about the binary data
	fmt.Printf("Consensus binary data length: %d bytes\n", len(b))

	// Write the consensus section directly
	sw, err := writer.OpenRaw(sv2.SectionTypeConsensus)
	if err != nil {
		return fmt.Errorf("open consensus section: %w", err)
	}

	// Write the binary data directly
	_, err = sw.Write(b)
	if err != nil {
		return fmt.Errorf("write consensus section: %w", err)
	}

	// Close the consensus section
	err = sw.Close()
	if err != nil {
		return fmt.Errorf("close consensus section: %w", err)
	}

	fmt.Printf("Successfully wrote binary consensus section for partition %s\n", targetPartition)
	return nil
}
