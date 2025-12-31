package server

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	badgerv1 "github.com/dgraph-io/badger"
)

// dbFullScan performs a full BadgerDB scan to extract ALL account URLs from keys and values
// This is similar to the staking complete-account-extractor approach
func (s *Server) dbFullScan(args map[string]interface{}) (map[string]interface{}, error) {
	// Get database name
	database, ok := args["database"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: database")
	}

	// Get optional max_keys parameter (default 2M like staking extractor)
	maxKeys := 2000000
	if mk, ok := args["max_keys"].(float64); ok {
		maxKeys = int(mk)
	}

	// Resolve database path
	var dbPath string
	if path, exists := knownDatabases[database]; exists {
		dbPath = path
	} else {
		dbPath = database
	}

	// Check if database exists
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("database not found at %s: %w", dbPath, err)
	}

	// Open BadgerDB v1 directly for full scan
	// Don't use ReadOnly mode - databases may not have been cleanly closed
	opts := badgerv1.DefaultOptions(dbPath)
	opts.Logger = nil

	db, err := badgerv1.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

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
				if len(val) > 0 && len(val) < 10000000 { // Sanity check for corruption
					valStr := string(val)
					if strings.Contains(valStr, "acc://") {
						valURLs := urlPattern.FindAllString(valStr, -1)
						for _, url := range valURLs {
							// Clean up URL
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

	// Return results
	result := map[string]interface{}{
		"database":       database,
		"keys_scanned":   keysProcessed,
		"accounts_found": len(accounts),
		"accounts":       accounts,
	}

	return result, nil
}

// extractAllURLsFromString extracts all acc:// URLs from a string
// Helper function similar to staking extractor
func extractAllURLsFromString(s string) []string {
	urlPattern := regexp.MustCompile(`acc://[a-zA-Z0-9\-\./_]+`)
	urls := urlPattern.FindAllString(s, -1)

	// Clean up URLs
	var cleaned []string
	for _, url := range urls {
		url = strings.TrimRight(url, ".")
		url = strings.TrimRight(url, "/")
		cleaned = append(cleaned, url)
	}

	return cleaned
}
