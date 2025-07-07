// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"os"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"encoding/hex"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	. "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

var (
	bootstrap = accumulate.BootstrapServers
	// Build time variables - will be set at compile time
	buildTime = "unknown"
	version   = "dev"
)

var rootCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Analysis utilities for Accumulate",
	Long:  "A collection of utilities for analyzing Accumulate databases, snapshots, and networks",
}

var cmdGenKey = &cobra.Command{
	Use:   "gen-key <adi> <output-dir>",
	Short: "Generate a priv_validator_key.json file for an ADI",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		adi := args[0]
		outputDir := args[1]

		adiURL, err := url.Parse(adi)
		if err != nil {
			return fmt.Errorf("invalid ADI URL: %w", err)
		}

		pubKey, privKey, err := ed25519.GenerateKey(nil)
		if err != nil {
			return fmt.Errorf("generate Ed25519 key: %w", err)
		}

		h := sha256.Sum256(pubKey)
		address := hex.EncodeToString(h[:20])

		pvKey := struct {
			Address string `json:"address"`
			PubKey  struct {
				Type  string `json:"type"`
				Value string `json:"value"`
			} `json:"pub_key"`
			PrivKey struct {
				Type  string `json:"type"`
				Value string `json:"value"`
			} `json:"priv_key"`
		}{
			Address: address,
		}
		pvKey.PubKey.Type = "tendermint/PubKeyEd25519"
		pvKey.PubKey.Value = base64.StdEncoding.EncodeToString(pubKey)
		pvKey.PrivKey.Type = "tendermint/PrivKeyEd25519"
		pvKey.PrivKey.Value = base64.StdEncoding.EncodeToString(privKey)

		data, err := json.MarshalIndent(pvKey, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal private validator key: %w", err)
		}

		if err := os.MkdirAll(outputDir, 0700); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}
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
	},
}

func init() {
	rootCmd.PersistentFlags().Var((*MultiaddrSliceFlag)(&bootstrap), "bootstrap", "Set the bootstrap servers")
	
	// Add commands directly to the root command
	// rootCmd.AddCommand(cmdAnalyzeDB) // Commented out - undefined reference
	rootCmd.AddCommand(cmdAnalyzeSnap)
	rootCmd.AddCommand(cmdAnalyzeSnapVersion)
	// rootCmd.AddCommand(cmdAnalyzePartition) // Commented out - undefined reference
	rootCmd.AddCommand(cmdAnalyzeSnapReport)
	// rootCmd.AddCommand(cmdAnalyzeSnapCombine) // Commented out - undefined reference
	rootCmd.AddCommand(sc_Cmd)
	rootCmd.AddCommand(cmdAnalyzeExtract) // Add the extract command
	rootCmd.AddCommand(InfoCommand()) // Add the info command

	// Add key generation and update commands
	rootCmd.AddCommand(cmdGenerateKey)
	rootCmd.AddCommand(cmdUpdateKey)

	rootCmd.AddCommand(cmdGenerateConsensusSection) // Add the generate-consensus-section command
	rootCmd.AddCommand(cmdGenKey)
	rootCmd.AddCommand(cmdUpdateNetworkKeys)
	rootCmd.AddCommand(cmdUpdateConsensus)
}

func main() {
	// Get executable file info to determine build time
	execPath, err := os.Executable()
	var buildTimeStr string
	var elapsed string
	now := time.Now()
	
	if err == nil {
		if stat, err := os.Stat(execPath); err == nil {
			buildTime := stat.ModTime()
			buildTimeStr = buildTime.Format(time.RFC3339)
			elapsed = fmt.Sprintf(" (%.1f minutes ago)", now.Sub(buildTime).Minutes())
		}
	}
	
	if buildTimeStr == "" {
		buildTimeStr = "unknown"
	}
	
	fmt.Printf("[BUILD INFO] Analyze tool compiled at: %s%s\n", buildTimeStr, elapsed)
	fmt.Printf("[BUILD INFO] Current time: %s\n", now.Format(time.RFC3339))
	fmt.Printf("[BUILD INFO] Executable path: %s\n", execPath)
	fmt.Println("[DIAG] TEST MAIN EXEC")
	
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
