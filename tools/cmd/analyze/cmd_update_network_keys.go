package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var cmdUpdateNetworkKeys = &cobra.Command{
	Use:   "update-network-keys <network.json> <artifacts-dir>",
	Short: "Update network JSON with validator public keys from artifacts",
	Args:  cobra.ExactArgs(2),
	RunE:  updateNetworkKeys,
}

func init() {
	// No flags needed - using positional arguments
}

// Using existing NetworkConfig struct from a_extract_network.go

func updateNetworkKeys(cmd *cobra.Command, args []string) error {
	// Get arguments
	networkFile := args[0]
	artifactsDir := args[1]
	
	// Parse network configuration using existing function
	netCfg, err := ParseNetworkJson(networkFile)
	if err != nil {
		return fmt.Errorf("parse network config: %w", err)
	}

	// For each validator, update the publicKey from the corresponding key file
	for i, v := range netCfg.Globals.Network.Validators {
		adiName := v.Operator
		adiFlat := sanitizeAdi(adiName)
		keyPath := filepath.Join(artifactsDir, "priv_validator_key_"+adiFlat+"_dn.json")
		keyData, err := ioutil.ReadFile(keyPath)
		if err != nil {
			return fmt.Errorf("read key file for %s: %w", adiName, err)
		}
		var keyFile PrivValidatorKey
		if err := json.Unmarshal(keyData, &keyFile); err != nil {
			return fmt.Errorf("unmarshal key file for %s: %w", adiName, err)
		}
		// Decode the public key from base64 (CometBFT format)
		pubKeyBytes, err := base64.StdEncoding.DecodeString(keyFile.PubKey.Value)
		if err != nil {
			return fmt.Errorf("failed to decode public key for validator %s: %v", v.Operator, err)
		}

		// Set the public key (hex encoded)
		netCfg.Globals.Network.Validators[i].PublicKey = hex.EncodeToString(pubKeyBytes)

		// Compute SHA256 hash of the public key
		hash := sha256.Sum256(pubKeyBytes)

		// Update the validator's public key hash
		netCfg.Globals.Network.Validators[i].PublicKeyHash = hex.EncodeToString(hash[:])
		
		// Add partitions information if not already present
		if len(netCfg.Globals.Network.Validators[i].Partitions) == 0 {
			netCfg.Globals.Network.Validators[i].Partitions = []struct {
				ID     string `json:"id"`
				Active bool   `json:"active"`
			}{
				{ID: "bvn-cyclops", Active: true},
				{ID: "Directory", Active: true},
			}
		}
	}

	// Backup the original file
	bak := networkFile + ".bak"
	if err := os.Rename(networkFile, bak); err != nil {
		return fmt.Errorf("backup network file: %w", err)
	}

	// Write the updated config
	out, err := json.MarshalIndent(netCfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal updated network json: %w", err)
	}
	if err := ioutil.WriteFile(networkFile, out, 0644); err != nil {
		return fmt.Errorf("write updated network file: %w", err)
	}

	fmt.Printf("Updated %s with validator public keys. Backup saved as %s\n", networkFile, bak)
	return nil
}

func sanitizeAdi(adi string) string {
	adi = adi[len("acc://"):]
	adi = strings.ReplaceAll(adi, "/", "-")
	adi = strings.ReplaceAll(adi, ".", "-")
	return adi
}
