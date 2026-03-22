// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gitlab.com/accumulatenetwork/accumulate/test/wallet"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "create":
		createCmd()
	case "info":
		infoCmd()
	case "export-keys":
		exportKeysCmd()
	case "get-account":
		getAccountCmd()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Test Wallet Manager

Usage:
  test-wallet create [options]     Create a new test wallet
  test-wallet info <path>          Display wallet information
  test-wallet export-keys <path>   Export all keys as JSON
  test-wallet get-account <path> <index>  Get account by index

Create Options:
  -path string          Wallet file path (default: ~/.accumulate/test-wallet.json)
  -funder string        Funder account URL (default: acc://test-funder.acme/tokens)
  -lite int            Number of lite accounts (default: 1000)
  -adi-token int       Number of ADI token accounts (default: 1000)
  -adi-data int        Number of ADI data accounts (default: 1000)
  -force               Overwrite existing wallet

Examples:
  # Create default wallet
  test-wallet create

  # Create custom wallet
  test-wallet create -path ./my-wallet.json -lite 100 -adi-token 100

  # View wallet info
  test-wallet info ~/.accumulate/test-wallet.json

  # Export funder key for load generator
  test-wallet export-keys ~/.accumulate/test-wallet.json | jq -r '.funder.privateKey'
`)
}

func createCmd() {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	path := fs.String("path", filepath.Join(os.Getenv("HOME"), ".accumulate", "test-wallet.json"), "Wallet file path")
	funderURL := fs.String("funder", "acc://test-funder.acme/tokens", "Funder account URL")
	numLite := fs.Int("lite", 1000, "Number of lite accounts")
	numADIToken := fs.Int("adi-token", 1000, "Number of ADI token accounts")
	numADIData := fs.Int("adi-data", 1000, "Number of ADI data accounts")
	force := fs.Bool("force", false, "Overwrite existing wallet")

	fs.Parse(os.Args[2:])

	// Check if wallet already exists
	if !*force {
		if _, err := os.Stat(*path); err == nil {
			log.Fatalf("Wallet already exists at %s. Use -force to overwrite.", *path)
		}
	}

	// Create wallet
	w := wallet.New(*path)

	// Generate funder
	log.Printf("Generating funder account: %s", *funderURL)
	if err := w.GenerateFunder(*funderURL); err != nil {
		log.Fatalf("Failed to generate funder: %v", err)
	}

	// Generate test accounts
	total := *numLite + *numADIToken + *numADIData
	log.Printf("Generating %d test accounts (%d lite, %d ADI token, %d ADI data)...",
		total, *numLite, *numADIToken, *numADIData)

	if err := w.GenerateAccounts(*numLite, *numADIToken, *numADIData); err != nil {
		log.Fatalf("Failed to generate accounts: %v", err)
	}

	// Save wallet
	log.Printf("Saving wallet to %s", *path)
	if err := w.Save(); err != nil {
		log.Fatalf("Failed to save wallet: %v", err)
	}

	log.Printf("Wallet created successfully with %d accounts", w.Count())
	fmt.Println()
	fmt.Printf("Wallet path: %s\n", *path)
	fmt.Printf("Funder URL:  %s\n", w.Funder.URL)
	fmt.Printf("Funder key:  %s\n", w.Funder.PrivateKey)
}

func infoCmd() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: test-wallet info <path>\n")
		os.Exit(1)
	}

	path := os.Args[2]

	w, err := wallet.Load(path)
	if err != nil {
		log.Fatalf("Failed to load wallet: %v", err)
	}

	fmt.Printf("Wallet: %s\n\n", path)

	if w.Funder != nil {
		fmt.Printf("Funder Account:\n")
		fmt.Printf("  URL:        %s\n", w.Funder.URL)
		fmt.Printf("  Public Key: %s\n", w.Funder.PublicKey)
		fmt.Printf("  Private Key: %s\n\n", w.Funder.PrivateKey)
	}

	fmt.Printf("Test Accounts:\n")
	fmt.Printf("  Total:      %d\n", w.Count())
	fmt.Printf("  Lite:       %d\n", w.CountByType("lite"))
	fmt.Printf("  ADI Token:  %d\n", w.CountByType("adi-token"))
	fmt.Printf("  ADI Data:   %d\n", w.CountByType("adi-data"))
}

func exportKeysCmd() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: test-wallet export-keys <path>\n")
		os.Exit(1)
	}

	path := os.Args[2]

	w, err := wallet.Load(path)
	if err != nil {
		log.Fatalf("Failed to load wallet: %v", err)
	}

	// Export as JSON
	data, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal wallet: %v", err)
	}

	fmt.Println(string(data))
}

func getAccountCmd() {
	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "Usage: test-wallet get-account <path> <index>\n")
		os.Exit(1)
	}

	path := os.Args[2]
	var index int
	if _, err := fmt.Sscanf(os.Args[3], "%d", &index); err != nil {
		log.Fatalf("Invalid index: %v", err)
	}

	w, err := wallet.Load(path)
	if err != nil {
		log.Fatalf("Failed to load wallet: %v", err)
	}

	acct, err := w.GetAccount(index)
	if err != nil {
		log.Fatalf("Failed to get account: %v", err)
	}

	data, err := json.MarshalIndent(acct, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal account: %v", err)
	}

	fmt.Println(string(data))
}
