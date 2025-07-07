package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var cmdUpdateNetworkKeys = &cobra.Command{
	Use:   "update-network-keys --network <network.json> --artifacts <artifacts-dir>",
	Short: "Update network JSON with validator public keys from artifacts",
	RunE:  updateNetworkKeys,
}

var (
	networkFile  string
	artifactsDir string
)

func init() {
	cmdUpdateNetworkKeys.Flags().StringVar(&networkFile, "network", "", "Path to cyclops-network.json")
	cmdUpdateNetworkKeys.Flags().StringVar(&artifactsDir, "artifacts", "./artifacts", "Directory containing validator key files")
	cmdUpdateNetworkKeys.MarkFlagRequired("network")
	cmdUpdateNetworkKeys.MarkFlagRequired("artifacts")
}

// Using existing NetworkConfig struct from a_extract_network.go

func updateNetworkKeys(cmd *cobra.Command, args []string) error {
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
		netCfg.Globals.Network.Validators[i].PublicKey = keyFile.PubKey.Value
		
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
