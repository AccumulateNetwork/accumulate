package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var cmdUpdateConsensus = &cobra.Command{
	Use:   "update-consensus --artifacts <artifacts-dir>",
	Short: "Update consensus files with validator public keys from artifacts",
	RunE:  updateConsensusFiles,
}

func init() {
	cmdUpdateConsensus.Flags().String("artifacts", "", "Path to artifacts directory")
	cmdUpdateConsensus.MarkFlagRequired("artifacts")
}

type ConsensusFile struct {
	ChainId    string      `json:"chainId"`
	Validators []Validator `json:"validators"`
}

type Validator struct {
	Address string `json:"address"`
	Type    int    `json:"type"`
	PubKey  string `json:"pubKey"`
	Power   int    `json:"power"`
	Name    string `json:"name"`
}

type ValidatorKey struct {
	Address string `json:"address"`
	PubKey  struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"pub_key"`
}

func updateConsensusFiles(cmd *cobra.Command, args []string) error {
	artifactsDir, _ := cmd.Flags().GetString("artifacts")

	// Read DN validator key
	dnKeyPath := filepath.Join(artifactsDir, "priv_validator_key_defidevs-acme_dn.json")
	dnKey, err := readValidatorKey(dnKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read DN key: %v", err)
	}

	// Read BVN0 validator key
	bvnKeyPath := filepath.Join(artifactsDir, "priv_validator_key_defidevs-acme_bvn0.json")
	bvnKey, err := readValidatorKey(bvnKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read BVN0 key: %v", err)
	}

	// Update DN consensus file
	dnConsensusPath := filepath.Join(artifactsDir, "Directory-consensus.json")
	err = updateConsensusFile(dnConsensusPath, dnKey, "cyclops.Directory")
	if err != nil {
		return fmt.Errorf("failed to update DN consensus: %v", err)
	}

	// Copy to expected name
	dnTargetPath := filepath.Join(artifactsDir, "consensus_dn.json")
	err = copyFile(dnConsensusPath, dnTargetPath)
	if err != nil {
		return fmt.Errorf("failed to copy DN consensus: %v", err)
	}

	// Update BVN consensus file
	bvnConsensusPath := filepath.Join(artifactsDir, "bvn-cyclops-consensus.json")
	err = updateConsensusFile(bvnConsensusPath, bvnKey, "cyclops.bvn-cyclops")
	if err != nil {
		return fmt.Errorf("failed to update BVN consensus: %v", err)
	}

	// Copy to expected name
	bvnTargetPath := filepath.Join(artifactsDir, "consensus_bvn0.json")
	err = copyFile(bvnConsensusPath, bvnTargetPath)
	if err != nil {
		return fmt.Errorf("failed to copy BVN consensus: %v", err)
	}

	fmt.Printf("Updated consensus files:\n")
	fmt.Printf("- %s (DN)\n", dnTargetPath)
	fmt.Printf("- %s (BVN0)\n", bvnTargetPath)

	return nil
}

func readValidatorKey(path string) (*ValidatorKey, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var key ValidatorKey
	err = json.Unmarshal(data, &key)
	if err != nil {
		return nil, err
	}

	return &key, nil
}

func updateConsensusFile(path string, key *ValidatorKey, chainId string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	var consensus ConsensusFile
	err = json.Unmarshal(data, &consensus)
	if err != nil {
		return err
	}

	// Convert base64 public key to hex for consensus format
	pubKeyHex := strings.ToLower(hex.EncodeToString([]byte(key.PubKey.Value)))

	// Update the consensus file
	consensus.ChainId = chainId
	if len(consensus.Validators) > 0 {
		consensus.Validators[0].Address = key.Address
		consensus.Validators[0].PubKey = pubKeyHex
	}

	// Write back to file
	updatedData, err := json.MarshalIndent(consensus, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(path, updatedData, 0644)
}

func copyFile(src, dst string) error {
	data, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(dst, data, 0644)
}
