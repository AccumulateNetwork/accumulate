// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/cometbft"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdGenerateKey = &cobra.Command{
	Use:   "generate-key <adi> <output-dir>",
	Short: "Generate a priv_validator_key.json file for an ADI",
	Long:  "Generate a CometBFT private validator key file for the specified ADI in the given directory",
	Args:  cobra.ExactArgs(2),
	RunE:  generateKey,
}

var cmdUpdateKey = &cobra.Command{
	Use:   "update <adi> <network.json> [key-dir]",
	Short: "Update network configuration with validator key",
	Long:  "Read priv_validator_key.json from key-dir (default: current directory) and update the network configuration with the public key",
	Args:  cobra.RangeArgs(2, 3),
	RunE:  updateKey,
}

func init() {
	// No flags needed for these simple commands
}

// generateKey creates a new priv_validator_key.json file for the given ADI
func generateKey(cmd *cobra.Command, args []string) error {
	adi := args[0]
	outputDir := args[1]
	
	// Validate the ADI URL
	adiURL, err := url.Parse(adi)
	if err != nil {
		return fmt.Errorf("invalid ADI URL %q: %w", adi, err)
	}
	
	// Ensure output directory exists
	err = os.MkdirAll(outputDir, 0755)
	if err != nil {
		return fmt.Errorf("create output directory %q: %w", outputDir, err)
	}
	
	// Generate a new Ed25519 key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		return fmt.Errorf("generate key pair: %w", err)
	}
	
	// Derive validator address from public key (first 20 bytes of SHA256 hash)
	hash := sha256.Sum256(pubKey)
	address := hex.EncodeToString(hash[:20])
	
	// Create the private validator key structure
	pvKey := PrivValidatorKey{
		Address: address,
		PubKey: struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		}{
			Type:  "tendermint/PubKeyEd25519",
			Value: base64.StdEncoding.EncodeToString(pubKey),
		},
		PrivKey: struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		}{
			Type:  "tendermint/PrivKeyEd25519",
			Value: base64.StdEncoding.EncodeToString(privKey),
		},
	}
	
	// Marshal to JSON
	data, err := json.MarshalIndent(pvKey, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal private validator key: %w", err)
	}
	
	// Write to file
	filename := filepath.Join(outputDir, "priv_validator_key.json")
	err = os.WriteFile(filename, data, 0600)
	if err != nil {
		return fmt.Errorf("write private validator key file: %w", err)
	}
	
	fmt.Printf("Generated private validator key for ADI: %s\n", adiURL.String())
	fmt.Printf("Address: %s\n", address)
	fmt.Printf("Public Key: %s\n", base64.StdEncoding.EncodeToString(pubKey))
	fmt.Printf("File: %s\n", filename)
	
	return nil
}

// updateKey reads priv_validator_key.json and updates the network configuration
func updateKey(cmd *cobra.Command, args []string) error {
	adi := args[0]
	networkFile := args[1]
	keyDir := "."
	if len(args) > 2 {
		keyDir = args[2]
	}
	
	// Validate the ADI URL
	adiURL, err := url.Parse(adi)
	if err != nil {
		return fmt.Errorf("invalid ADI URL %q: %w", adi, err)
	}
	
	// Read the private validator key
	keyFile := filepath.Join(keyDir, "priv_validator_key.json")
	pvData, err := os.ReadFile(keyFile)
	if err != nil {
		return fmt.Errorf("read priv_validator_key.json: %w", err)
	}
	
	var pvKey PrivValidatorKey
	err = json.Unmarshal(pvData, &pvKey)
	if err != nil {
		return fmt.Errorf("parse private validator key: %w", err)
	}
	
	// Validate key type
	if pvKey.PubKey.Type != "tendermint/PubKeyEd25519" {
		return fmt.Errorf("invalid public key type: expected 'tendermint/PubKeyEd25519', got '%s'", pvKey.PubKey.Type)
	}
	if pvKey.PrivKey.Type != "tendermint/PrivKeyEd25519" {
		return fmt.Errorf("invalid private key type: expected 'tendermint/PrivKeyEd25519', got '%s'", pvKey.PrivKey.Type)
	}
	
	// Validate key values are not empty
	if pvKey.PubKey.Value == "" {
		return fmt.Errorf("public key value is empty")
	}
	if pvKey.PrivKey.Value == "" {
		return fmt.Errorf("private key value is empty")
	}
	
	// Read the network configuration
	networkData, err := os.ReadFile(networkFile)
	if err != nil {
		return fmt.Errorf("read network file: %w", err)
	}
	
	var networkConfig NetworkConfig
	err = json.Unmarshal(networkData, &networkConfig)
	if err != nil {
		return fmt.Errorf("parse network configuration: %w", err)
	}
	
	// Find and update the validator entry
	updated := false
	for i, validator := range networkConfig.Globals.Network.Validators {
		if validator.Operator == adiURL.String() {
			oldKey := validator.PublicKey
			// Convert base64 public key to hex for network config
			pubKeyBytes, err := base64.StdEncoding.DecodeString(pvKey.PubKey.Value)
			if err != nil {
				return fmt.Errorf("decode public key: %w", err)
			}
			
			// Validate Ed25519 public key size (32 bytes)
			if len(pubKeyBytes) != ed25519.PublicKeySize {
				return fmt.Errorf("invalid Ed25519 public key size: expected %d bytes, got %d bytes", ed25519.PublicKeySize, len(pubKeyBytes))
			}
			newKey := hex.EncodeToString(pubKeyBytes)
			networkConfig.Globals.Network.Validators[i].PublicKey = newKey
			updated = true
			fmt.Printf("Updated validator %s:\n", adiURL.String())
			fmt.Printf("  Old key: %s\n", oldKey)
			fmt.Printf("  New key: %s\n", newKey)
			break
		}
	}
	
	if !updated {
		return fmt.Errorf("validator %s not found in network configuration", adiURL.String())
	}
	
	// Write updated network configuration
	updatedData, err := json.MarshalIndent(networkConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal updated network config: %w", err)
	}
	
	err = os.WriteFile(networkFile, updatedData, 0644)
	if err != nil {
		return fmt.Errorf("write updated network config: %w", err)
	}
	
	fmt.Printf("Successfully updated network configuration: %s\n", networkFile)
	return nil
}

// PrivValidatorKey represents the structure of a CometBFT private validator key file
type PrivValidatorKey struct {
	Address string `json:"address"`
	PubKey  struct {
		Type  string `json:"type"`
		Value string `json:"value"` // base64-encoded public key
	} `json:"pub_key"`
	PrivKey struct {
		Type  string `json:"type"`
		Value string `json:"value"` // base64-encoded private key + public key (for Ed25519)
	} `json:"priv_key"`
}

// collectValidatorKeys scans node directories and extracts validator keys
func collectValidatorKeys(nodesDir string) (map[string]*PrivValidatorKey, error) {
	fmt.Printf("Scanning for node directories in: %s\n", nodesDir)
	nodeEntries, err := os.ReadDir(nodesDir)
	if err != nil {
		return nil, fmt.Errorf("read nodes directory: %w", err)
	}
	
	validatorKeys := make(map[string]*PrivValidatorKey) // operator -> private key
	
	for _, entry := range nodeEntries {
		if !entry.IsDir() {
			continue
		}
		
		nodeName := entry.Name()
		nodeDir := filepath.Join(nodesDir, nodeName)
		
		// Look for Node0/config/priv_validator_key.json
		privKeyPath := filepath.Join(nodeDir, "Node0", "config", "priv_validator_key.json")
		if _, err := os.Stat(privKeyPath); os.IsNotExist(err) {
			// Try alternative path structure
			privKeyPath = filepath.Join(nodeDir, "config", "priv_validator_key.json")
			if _, err := os.Stat(privKeyPath); os.IsNotExist(err) {
				fmt.Printf("Warning: No priv_validator_key.json found for %s\n", nodeName)
				continue
			}
		}
		
		fmt.Printf("Found private key file: %s\n", privKeyPath)
		
		// Read the private validator key
		privKeyData, err := os.ReadFile(privKeyPath)
		if err != nil {
			fmt.Printf("Warning: Failed to read %s: %v\n", privKeyPath, err)
			continue
		}
		
		var pvKey PrivValidatorKey
		err = json.Unmarshal(privKeyData, &pvKey)
		if err != nil {
			fmt.Printf("Warning: Failed to parse %s: %v\n", privKeyPath, err)
			continue
		}
		
		// Extract public key base64 string from the parsed structure
		pubKeyB64 := pvKey.PubKey.Value
		log.Printf("Found validator key for %s with public key (base64): %s", nodeDir, pubKeyB64)

		// Decode public key from base64 to bytes
		pubKeyBytes, err := base64.StdEncoding.DecodeString(pubKeyB64)
		if err != nil {
			log.Printf("Error decoding public key base64 for %s: %v", nodeDir, err)
			continue
		}

		// Derive validator address using SHA256 hash of public key (first 20 bytes)
		// This matches CometBFT's address derivation: SHA256(pubkey)[:20]
		hash := sha256.Sum256(pubKeyBytes)
		validatorAddr := hash[:20]
		log.Printf("Derived validator address for %s: %x (from pubkey %x)", nodeDir, validatorAddr, pubKeyBytes)
		
		// Decode validator address from hex string (CometBFT stores address as hex)
		validatorAddrHex := pvKey.Address
		validatorAddrBytes, err := hex.DecodeString(validatorAddrHex)
		if err != nil {
			log.Printf("Error decoding validator address hex for %s: %v", nodeDir, err)
			continue
		}

		// Verify that the address matches the derived address from public key
		if !bytes.Equal(validatorAddr, validatorAddrBytes) {
			log.Printf("WARNING: Address mismatch for %s - derived: %x, stored: %x", nodeDir, validatorAddr, validatorAddrBytes)
			// Use the derived address for consistency
			validatorAddrBytes = validatorAddr
		}
		
		// Map node name to operator
		operator := nodeNameToOperator(nodeName)
		validatorKeys[operator] = &pvKey
		
		fmt.Printf("Found validator key for %s (%s)\n", nodeName, operator)
	}
	
	return validatorKeys, nil
}

// generateConsensusSections creates consensus section JSON files for each partition
func generateConsensusSections(networkConfig *NetworkConfig, validatorKeys map[string]*PrivValidatorKey) error {
	// Group validators by partition
	partitionValidators := make(map[string][]*cometbft.Validator)
	
	for _, validator := range networkConfig.Globals.Network.Validators {
		pvKey, exists := validatorKeys[validator.Operator]
		if !exists {
			fmt.Printf("Warning: No private key found for validator %s\n", validator.Operator)
			continue
		}
		
		// Decode public key from base64 (from the private validator key file)
		pubKeyBytes, err := base64.StdEncoding.DecodeString(pvKey.PubKey.Value)
		if err != nil {
			fmt.Printf("Warning: Invalid base64 public key for validator %s: %v\n", validator.Operator, err)
			continue
		}
		
		// Decode validator address from hex string
		validatorAddrBytes, err := hex.DecodeString(pvKey.Address)
		if err != nil {
			fmt.Printf("Warning: Invalid hex address for validator %s: %v\n", validator.Operator, err)
			continue
		}
		
		// Create cometbft.Validator with exact structure expected by consensus state
		cometValidator := &cometbft.Validator{
			Address: validatorAddrBytes, // Raw address bytes (20 bytes)
			Type:    protocol.SignatureTypeED25519,
			PubKey:  pubKeyBytes, // Raw public key bytes
			Power:   1,           // Voting power as int64
			Name:    validator.Operator,
		}
		
		// Add to each partition this validator participates in
		for _, partition := range validator.Partitions {
			partitionValidators[partition.ID] = append(partitionValidators[partition.ID], cometValidator)
		}
	}
	
	// Generate consensus JSON for each partition
	for partition, validators := range partitionValidators {
		err := writeConsensusJSON(partition, validators)
		if err != nil {
			return fmt.Errorf("failed to write consensus JSON for partition %s: %w", partition, err)
		}
		fmt.Printf("Generated consensus section for partition: %s (%d validators)\n", partition, len(validators))
	}
	
	return nil
}

// writeConsensusJSON writes the consensus section JSON for a partition using exact cometbft.GenesisDoc structure
func writeConsensusJSON(partition string, validators []*cometbft.Validator) error {
	// Create cometbft.GenesisDoc with exact structure expected by consensus state
	doc := &cometbft.GenesisDoc{
		ChainID:    fmt.Sprintf("cyclops.%s", partition),
		Validators: validators,
		// Params and Block can be nil for basic consensus sections
	}
	
	// Marshal to JSON using the same approach as WriteConsensusSection
	jsonData, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal consensus JSON: %w", err)
	}
	
	// Write to file
	filename := fmt.Sprintf("consensus-%s.json", partition)
	err = os.WriteFile(filename, jsonData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write consensus JSON file %s: %w", filename, err)
	}
	
	fmt.Printf("Wrote consensus JSON: %s\n", filename)
	return nil
}

// nodeNameToOperator maps node directory names to operator names in network config
func nodeNameToOperator(nodeName string) string {
	// Simple mapping - can be enhanced based on naming conventions
	// For now, assume operator is derived from node name
	if strings.Contains(nodeName, "bvn") {
		return "acc://defidevs.acme" // Default operator for BVN nodes
	}
	if strings.Contains(nodeName, "dn") || strings.Contains(nodeName, "directory") {
		return "acc://defidevs.acme" // Default operator for Directory nodes
	}
	return "acc://defidevs.acme" // Default fallback
}

// getValidatorPartitions determines which partitions a validator should be active on
func getValidatorPartitions(nodeName string) []string {
	// Simple mapping based on node name
	if strings.Contains(nodeName, "bvn") {
		return []string{"Directory", "bvn-cyclops"} // BVN nodes validate both Directory and their BVN
	}
	if strings.Contains(nodeName, "dn") || strings.Contains(nodeName, "directory") {
		return []string{"Directory"} // Directory nodes only validate Directory
	}
	return []string{"Directory"} // Default to Directory
}
