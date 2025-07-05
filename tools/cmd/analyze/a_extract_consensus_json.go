package main

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"

	tmed25519 "github.com/cometbft/cometbft/crypto/ed25519"
	tmtypes "github.com/cometbft/cometbft/types"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/cometbft"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	sv2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
)

// WriteJSONConsensusSection writes a consensus section to the snapshot in JSON format
// This matches the format expected by the node's ABCI InitChain function
func WriteJSONConsensusSection(writer *sv2.Writer, extractState *ExtractState, targetPartition string) error {
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
	
	// Create a custom JSON structure that matches CometBFT's expected format
	type CometBFTValidator struct {
		Address string `json:"address"`
		PubKey  struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"pub_key"`
		Power int64  `json:"power"`
		Name  string `json:"name"`
	}
	
	type CometBFTConsensusParams struct {
		Block struct {
			MaxBytes int64 `json:"max_bytes"`
			MaxGas   int64 `json:"max_gas"`
		} `json:"block"`
	}
	
	type CometBFTGenesisDoc struct {
		GenesisTime     string              `json:"genesis_time"`
		ChainID         string              `json:"chain_id"`
		ConsensusParams CometBFTConsensusParams `json:"consensus_params"`
		Validators      []CometBFTValidator     `json:"validators"`
		AppState        json.RawMessage     `json:"app_state"`
	}

	// Create a custom JSON document that matches CometBFT's expected format
	cometDoc := CometBFTGenesisDoc{
		GenesisTime: extractState.SnapshotReader.Header.SystemLedger.Timestamp.Format("2006-01-02T15:04:05.999999999Z"),
		ChainID:     networkID + "." + targetPartition,
	}
	
	// Set consensus parameters
	cometDoc.ConsensusParams.Block.MaxBytes = params.Block.MaxBytes
	cometDoc.ConsensusParams.Block.MaxGas = params.Block.MaxGas
	
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

			// Create a validator in CometBFT's expected format
			val := CometBFTValidator{
				Address: key.Address().String(),
				Name:    name,
				Power:   "1", // All validators have equal voting power
			}
			
			// Set the public key in the expected format
			val.PubKey.Type = "tendermint/PubKeyEd25519"
			val.PubKey.Value = base64.StdEncoding.EncodeToString(pubKeyBytes)
			
			// Add to our custom document
			cometDoc.Validators = append(cometDoc.Validators, val)
			
			// Also add to the original doc for binary fallback
			docVal := &cometbft.Validator{
				Address: key.Address(),
				PubKey:  pubKeyBytes,
				Power:   1,
				Name:    name,
				Type:    protocol.SignatureTypeED25519,
			}
			doc.Validators = append(doc.Validators, docVal)
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

			// Create a validator in CometBFT's expected format
			val := CometBFTValidator{
				Address: key.Address().String(),
				Name:    name,
				Power:   "1", // All validators have equal voting power
			}
			
			// Set the public key in the expected format
			val.PubKey.Type = "tendermint/PubKeyEd25519"
			val.PubKey.Value = base64.StdEncoding.EncodeToString(pubKeyBytes)
			
			// Add to our custom document
			cometDoc.Validators = append(cometDoc.Validators, val)
			
			// Also add to the original doc for binary fallback
			docVal := &cometbft.Validator{
				Address: key.Address(),
				PubKey:  pubKeyBytes,
				Power:   1,
				Name:    name,
				Type:    protocol.SignatureTypeED25519,
			}
			doc.Validators = append(doc.Validators, docVal)
		}
	} else {
		// No validator keys provided via CLI or network config
		fmt.Printf("No validator keys provided via command line or network config, adding a default validator\n")
		
		// Create a default validator with a dummy key
		pubKeyBytes := make([]byte, 32)
		key := tmed25519.PubKey(pubKeyBytes)
		name := fmt.Sprintf("Default-Validator-%s", targetPartition)

		// Create a validator in CometBFT's expected format
		val := CometBFTValidator{
			Address: key.Address().String(),
			Name:    name,
			Power:   "1", // All validators have equal voting power
		}
		
		// Set the public key in the expected format
		val.PubKey.Type = "tendermint/PubKeyEd25519"
		val.PubKey.Value = base64.StdEncoding.EncodeToString(pubKeyBytes)
		
		// Add to our custom document
		cometDoc.Validators = append(cometDoc.Validators, val)
		
		// Also add to the original doc for binary fallback
		docVal := &cometbft.Validator{
			Address: key.Address(),
			PubKey:  pubKeyBytes,
			Power:   1,
			Name:    name,
			Type:    protocol.SignatureTypeED25519,
		}
		doc.Validators = append(doc.Validators, docVal)
	}

	// If no valid validators were found, abort
	if len(cometDoc.Validators) == 0 {
		return fmt.Errorf("no valid validator keys provided for partition %s", targetPartition)
	}

	// Marshal the CometBFT-compatible genesis doc to JSON format
	jsonBytes, err := json.MarshalIndent(cometDoc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal consensus doc to JSON: %w", err)
	}
	
	// Debug: Print information about the JSON data
	fmt.Printf("Consensus JSON data length: %d bytes\n", len(jsonBytes))
	
	// Print a sample of the JSON for debugging
	if len(jsonBytes) < 1000 {
		fmt.Printf("JSON consensus section content: %s\n", string(jsonBytes))
	} else {
		fmt.Printf("JSON consensus section content (first 1000 bytes): %s...\n", string(jsonBytes[:1000]))
	}

	// Write the consensus section
	_, err = sw.Write(jsonBytes)
	if err != nil {
		return fmt.Errorf("write consensus section: %w", err)
	}

	// Close the consensus section explicitly
	err = sw.Close()
	if err != nil {
		return fmt.Errorf("close consensus section: %w", err)
	}

	fmt.Printf("Successfully wrote JSON consensus section for partition %s\n", targetPartition)
	return nil
}
