// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// account_explorer is a command-line tool to explore Accumulate accounts
// and their properties, including sub-accounts and token balances.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/client"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	var (
		network = flag.String("network", "mainnet", "Network to connect to (mainnet, testnet, local)")
		url     = flag.String("url", "acc://ACME", "Account URL to explore")
		depth   = flag.Int("depth", 1, "Depth to explore sub-accounts (0=just account, 1=include directory)")
		jsonOut = flag.Bool("json", false, "Output as JSON")
		verbose = flag.Bool("v", false, "Verbose output")
	)
	flag.Parse()

	// Create client based on network
	var c *client.Client
	var err error
	
	switch *network {
	case "mainnet":
		c, err = client.NewMainnet()
	case "testnet":
		c, err = client.NewTestnet()
	case "local":
		endpoint := os.Getenv("ACCUMULATE_ENDPOINT")
		if endpoint == "" {
			endpoint = "http://localhost:8080/v3"
		}
		c, err = client.NewLocal(endpoint)
	default:
		log.Fatalf("Unknown network: %s", *network)
	}
	
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Explore the account
	explorer := &AccountExplorer{
		client:  c,
		verbose: *verbose,
		jsonOut: *jsonOut,
	}
	
	if err := explorer.Explore(ctx, *url, *depth); err != nil {
		log.Fatalf("Failed to explore account: %v", err)
	}
}

type AccountExplorer struct {
	client  *client.Client
	verbose bool
	jsonOut bool
}

type AccountInfo struct {
	URL         string                 `json:"url"`
	Type        string                 `json:"type"`
	Properties  map[string]interface{} `json:"properties"`
	SubAccounts []AccountInfo          `json:"subAccounts,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

func (e *AccountExplorer) Explore(ctx context.Context, accountURL string, depth int) error {
	info, err := e.exploreAccount(ctx, accountURL, depth)
	if err != nil {
		return err
	}

	if e.jsonOut {
		// Output as JSON
		data, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(data))
	} else {
		// Output as text
		e.printAccountInfo(info, 0)
	}

	return nil
}

func (e *AccountExplorer) exploreAccount(ctx context.Context, accountURL string, depth int) (*AccountInfo, error) {
	if e.verbose {
		log.Printf("Exploring %s (depth=%d)", accountURL, depth)
	}

	info := &AccountInfo{
		URL:        accountURL,
		Properties: make(map[string]interface{}),
	}

	// Get account information
	account, err := e.client.GetAccount(ctx, accountURL)
	if err != nil {
		info.Error = err.Error()
		return info, nil // Return partial info with error
	}

	if account == nil || account.Account == nil {
		info.Error = "account not found"
		return info, nil
	}

	// Get account type
	info.Type = account.Account.Type().String()

	// Extract properties based on account type
	switch acc := account.Account.(type) {
	case *protocol.TokenIssuer:
		info.Properties["symbol"] = acc.Symbol
		info.Properties["precision"] = acc.Precision
		info.Properties["issued"] = acc.Issued.String()
		info.Properties["supplyLimit"] = acc.SupplyLimit.String()
		
	case *protocol.LiteTokenAccount:
		info.Properties["tokenUrl"] = acc.TokenUrl.String()
		info.Properties["balance"] = acc.Balance.String()
		// Note: CreditBalance not available in current protocol
		// info.Properties["creditBalance"] = acc.CreditBalance
		
	case *protocol.TokenAccount:
		info.Properties["tokenUrl"] = acc.TokenUrl.String()
		info.Properties["balance"] = acc.Balance.String()
		
	case *protocol.DataAccount:
		if acc.Entry != nil {
			info.Properties["entryType"] = acc.Entry.Type().String()
			if len(acc.Entry.GetData()) > 0 {
				info.Properties["dataSize"] = len(acc.Entry.GetData()[0])
			}
		}
		
	case *protocol.LiteDataAccount:
		// Note: CreditBalance not available in current protocol
		// info.Properties["creditBalance"] = acc.CreditBalance
		
	case *protocol.KeyBook:
		info.Properties["pageCount"] = acc.PageCount
		
	case *protocol.KeyPage:
		info.Properties["keyCount"] = len(acc.Keys)
		info.Properties["creditBalance"] = acc.CreditBalance
		
	default:
		// For other account types, try to get authorities
		if fullAccount, ok := account.Account.(protocol.FullAccount); ok {
			if auth := fullAccount.GetAuth(); auth != nil {
				authorities := []string{}
				for _, a := range auth.Authorities {
					authorities = append(authorities, a.Url.String())
				}
				info.Properties["authorities"] = authorities
			}
		}
	}

	// Add common properties
	if account.LastBlockTime != nil {
		info.Properties["lastBlockTime"] = account.LastBlockTime.Format(time.RFC3339)
	}

	// Explore sub-accounts if depth > 0
	if depth > 0 {
		// Try to get directory
		dir, err := e.client.GetDirectory(ctx, accountURL, 0, 100)
		if err == nil && dir != nil {
			info.SubAccounts = make([]AccountInfo, 0, len(dir.Records))
			
			for _, record := range dir.Records {
				if record.Account != nil {
					subURL := record.Account.GetUrl().String()
					// Skip self-reference
					if subURL != accountURL {
						subInfo, err := e.exploreAccount(ctx, subURL, depth-1)
						if err != nil {
							subInfo = &AccountInfo{
								URL:   subURL,
								Error: err.Error(),
							}
						}
						info.SubAccounts = append(info.SubAccounts, *subInfo)
					}
				}
			}
		} else if e.verbose {
			log.Printf("Could not get directory for %s: %v", accountURL, err)
		}
	}

	return info, nil
}

func (e *AccountExplorer) printAccountInfo(info *AccountInfo, indent int) {
	prefix := strings.Repeat("  ", indent)
	
	// Print header
	fmt.Printf("%s📁 %s (%s)\n", prefix, info.URL, info.Type)
	
	// Print error if any
	if info.Error != "" {
		fmt.Printf("%s  ❌ Error: %s\n", prefix, info.Error)
		return
	}
	
	// Print properties
	for key, value := range info.Properties {
		fmt.Printf("%s  • %s: %v\n", prefix, key, value)
	}
	
	// Print sub-accounts
	if len(info.SubAccounts) > 0 {
		fmt.Printf("%s  📂 Sub-accounts (%d):\n", prefix, len(info.SubAccounts))
		for _, sub := range info.SubAccounts {
			e.printAccountInfo(&sub, indent+2)
		}
	}
}