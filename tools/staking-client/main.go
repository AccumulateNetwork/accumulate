package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/lightclient"
)

// AccountEntry maintains the order of accounts and their names
type AccountEntry struct {
	Name    string
	Account *lightclient.AccountInfo
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: staking-client <server>")
		fmt.Println("Servers: local, testnet, beta, canary, mainnet, mainnet-ssl")
		os.Exit(1)
	}

	serverName := os.Args[1]

	fmt.Println("=== Accumulate Staking Client ===")
	fmt.Printf("Server: %s\n\n", serverName)

	// Create light client
	client, err := lightclient.NewClient(serverName)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Println("Retrieving staking registry...")

	// Get staking registry and accounts
	stakingAccounts, totalStaked, err := client.GetStakingAccountsWithTotal(ctx)
	if err != nil {
		log.Fatalf("Failed to get staking accounts: %v", err)
	}

	fmt.Printf("\n=== Staking Registry Summary ===\n")
	fmt.Printf("Total Registered Staking Accounts: %d\n", len(stakingAccounts))
	fmt.Printf("Total Staked Tokens: %d ACME\n\n", totalStaked)

	fmt.Println("=== Registered Staking Accounts ===")
	for i, account := range stakingAccounts {
		fmt.Printf("\nAccount %d: %s\n", i+1, account.URL)
		fmt.Printf("  Type: %s\n", account.Type)
		fmt.Printf("  Balance: %d ACME\n", account.Balance)
		fmt.Printf("  Token URL: %s\n", account.TokenURL)
		
		if len(account.Authorities) > 0 {
			fmt.Printf("  Authorities:\n")
			for _, auth := range account.Authorities {
				fmt.Printf("    - %s\n", auth)
			}
		}
	}

	// Collect account names and metadata while maintaining order
	accountEntries := make([]*AccountEntry, 0, len(stakingAccounts))
	// Track duplicates using a slice of names
	duplicateNames := make([]string, 0, len(stakingAccounts))

	for _, account := range stakingAccounts {
		name := account.URL
		// Check if name exists in duplicates
		found := false
		for i, dupName := range duplicateNames {
			if dupName == name {
				// Update existing entry with new account info
				accountEntries[i].Account = account
				found = true
				break
			}
		}
		if !found {
			// Add new entry
			accountEntries = append(accountEntries, &AccountEntry{
				Name:    name,
				Account: account,
			})
			duplicateNames = append(duplicateNames, name)
		}
	}

	// Sort account entries by name
	sort.Slice(accountEntries, func(i, j int) bool {
		return accountEntries[i].Name < accountEntries[j].Name
	})

	// Print sorted names and metadata
	fmt.Printf("\n=== Sorted Staking Accounts (by Name) ===\n")
	fmt.Printf("Total Unique Accounts: %d\n\n", len(accountEntries))

	for i, entry := range accountEntries {
		fmt.Printf("Account %d: %s\n", i+1, entry.Name)
		fmt.Printf("  Type: %s\n", entry.Account.Type)
		fmt.Printf("  Balance: %d ACME\n", entry.Account.Balance)
		fmt.Printf("  Token URL: %s\n", entry.Account.TokenURL)
		if len(entry.Account.Authorities) > 0 {
			fmt.Printf("  Authorities:\n")
			for _, auth := range entry.Account.Authorities {
				fmt.Printf("    - %s\n", auth)
			}
		}
		fmt.Println()
	}

	fmt.Printf("\nSuccessfully retrieved %d staking accounts!\n", len(stakingAccounts))
}
