package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	badgerv1 "github.com/dgraph-io/badger"
)

// Database paths
var databases = map[string]string{
	"2025-07-13-dn":   "/media/paul/Expansion/databases/2025-07-13-dn/accumulate.db",
	"2025-07-13-bvn0": "/media/paul/Expansion/databases/2025-07-13-bvn0/accumulate.db",
	"2025-07-13-bvn1": "/media/paul/Expansion/databases/2025-07-13-bvn1/accumulate.db",
	"2025-07-13-bvn2": "/media/paul/Expansion/databases/2025-07-13-bvn2/accumulate.db",
}

type ScanResult struct {
	Database      string   `json:"database"`
	KeysScanned   int      `json:"keys_scanned"`
	AccountsFound int      `json:"accounts_found"`
	Duration      string   `json:"duration"`
	Accounts      []string `json:"accounts"`
}

func scanDatabase(name string, dbPath string, maxKeys int) (*ScanResult, error) {
	start := time.Now()
	log.Printf("Scanning %s (max %d keys)...", name, maxKeys)

	// Open BadgerDB
	opts := badgerv1.DefaultOptions(dbPath)
	opts.Logger = nil

	db, err := badgerv1.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	log.Printf("  Database opened, starting scan...")

	// Account set to avoid duplicates
	accountSet := make(map[string]struct{})
	var accounts []string
	keysProcessed := 0

	// Regex to extract acc:// URLs
	urlPattern := regexp.MustCompile(`acc://[a-zA-Z0-9\-\./_]+`)

	err = db.View(func(txn *badgerv1.Txn) error {
		it := txn.NewIterator(badgerv1.DefaultIteratorOptions)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			keysProcessed++

			// Progress indicator every 10k keys
			if keysProcessed%10000 == 0 {
				log.Printf("  Progress: %d keys scanned, %d accounts found", keysProcessed, len(accounts))
			}

			// Limit to prevent excessive processing
			if keysProcessed > maxKeys {
				break
			}

			item := it.Item()
			key := item.Key()

			// Extract URLs from the key
			keyURLs := urlPattern.FindAllString(string(key), -1)
			for _, url := range keyURLs {
				// Clean up URL
				url = strings.TrimRight(url, ".")
				url = strings.TrimRight(url, "/")
				if _, exists := accountSet[url]; !exists {
					accountSet[url] = struct{}{}
					accounts = append(accounts, url)
				}
			}

			// Extract URLs from the value (with corruption protection)
			item.Value(func(val []byte) error {
				if len(val) > 0 && len(val) < 10000000 { // Sanity check
					valStr := string(val)
					if strings.Contains(valStr, "acc://") {
						valURLs := urlPattern.FindAllString(valStr, -1)
						for _, url := range valURLs {
							url = strings.TrimRight(url, ".")
							url = strings.TrimRight(url, "/")
							if _, exists := accountSet[url]; !exists {
								accountSet[url] = struct{}{}
								accounts = append(accounts, url)
							}
						}
					}
				}
				return nil
			})
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("database scan error: %w", err)
	}

	duration := time.Since(start)
	log.Printf("  Scan complete: %d accounts found in %d keys (%v)", len(accounts), keysProcessed, duration)

	return &ScanResult{
		Database:      name,
		KeysScanned:   keysProcessed,
		AccountsFound: len(accounts),
		Duration:      duration.String(),
		Accounts:      accounts,
	}, nil
}

func main() {
	maxKeys := 2000000 // Default 2M keys
	if len(os.Args) > 1 {
		fmt.Sscanf(os.Args[1], "%d", &maxKeys)
	}

	log.Printf("Starting full database scan of all 2025-07-13 partitions (max %d keys per database)", maxKeys)
	log.Printf("This will scan ALL keys and values for acc:// URLs, not just BPT-indexed accounts\n")

	allResults := make([]*ScanResult, 0)
	totalAccounts := 0

	// Scan all partitions in order
	partitions := []string{"2025-07-13-dn", "2025-07-13-bvn0", "2025-07-13-bvn1", "2025-07-13-bvn2"}

	for i, partition := range partitions {
		log.Printf("\n=== Partition %d/4: %s ===", i+1, partition)

		dbPath, exists := databases[partition]
		if !exists {
			log.Printf("  ERROR: Database path not found for %s", partition)
			continue
		}

		// Check if database exists
		if _, err := os.Stat(dbPath); err != nil {
			log.Printf("  ERROR: Database not found at %s: %v", dbPath, err)
			continue
		}

		result, err := scanDatabase(partition, dbPath, maxKeys)
		if err != nil {
			log.Printf("  ERROR: %v", err)
			continue
		}

		allResults = append(allResults, result)
		totalAccounts += result.AccountsFound
	}

	// Print summary
	log.Printf("\n\n=== SCAN COMPLETE ===")
	log.Printf("Total partitions scanned: %d", len(allResults))
	log.Printf("Total accounts found: %d\n", totalAccounts)

	log.Printf("Breakdown by partition:")
	for _, result := range allResults {
		log.Printf("  %s: %d accounts (%d keys scanned in %s)",
			result.Database, result.AccountsFound, result.KeysScanned, result.Duration)
	}

	// Save results to file
	outputFile := "/tmp/fullscan-2025-07-13-results.json"
	data, err := json.MarshalIndent(map[string]interface{}{
		"total_accounts": totalAccounts,
		"partitions":     allResults,
	}, "", "  ")
	if err != nil {
		log.Printf("ERROR: Failed to marshal results: %v", err)
		return
	}

	if err := os.WriteFile(outputFile, data, 0644); err != nil {
		log.Printf("ERROR: Failed to write results: %v", err)
		return
	}

	log.Printf("\nResults saved to: %s", outputFile)

	// Also save all unique accounts to a file
	allAccountsSet := make(map[string]struct{})
	for _, result := range allResults {
		for _, account := range result.Accounts {
			allAccountsSet[account] = struct{}{}
		}
	}

	allAccountsList := make([]string, 0, len(allAccountsSet))
	for account := range allAccountsSet {
		allAccountsList = append(allAccountsList, account)
	}

	accountsFile := "/tmp/fullscan-2025-07-13-accounts.json"
	accountsData, _ := json.MarshalIndent(allAccountsList, "", "  ")
	if err := os.WriteFile(accountsFile, accountsData, 0644); err != nil {
		log.Printf("ERROR: Failed to write accounts list: %v", err)
		return
	}

	log.Printf("All unique accounts saved to: %s", accountsFile)
	log.Printf("\n=== COMPARISON WITH BPT ITERATOR ===")
	log.Printf("Previous BPT iterator extraction found:")
	log.Printf("  DN:   170 accounts")
	log.Printf("  BVN0:   1 account")
	log.Printf("  BVN1:  13 accounts")
	log.Printf("  BVN2:  54 accounts")
	log.Printf("  Total: 238 accounts")
	log.Printf("\nFull scan found: %d accounts", totalAccounts)
	if totalAccounts > 238 {
		improvement := float64(totalAccounts) / 238.0
		log.Printf("Improvement: %.1fx more accounts!", improvement)
	}
}
