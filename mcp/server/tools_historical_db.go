package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// DatabaseInfo contains information about a historical database
type DatabaseInfo struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	Exists       bool   `json:"exists"`
	SizeBytes    int64  `json:"sizeBytes"`
	SizeGB       string `json:"sizeGB"`
	LastModified string `json:"lastModified,omitempty"`
}

// knownDatabases lists the known historical database paths
var knownDatabases = map[string]string{
	// Current databases from /media/paul/Expansion/databases
	"2025-07-13-bvn0": "/media/paul/Expansion/databases/2025-07-13-bvn0/accumulate.db",
	"2025-07-13-bvn1": "/media/paul/Expansion/databases/2025-07-13-bvn1/accumulate.db",
	"2025-07-13-bvn2": "/media/paul/Expansion/databases/2025-07-13-bvn2/accumulate.db",
	"2025-07-13-dn":   "/media/paul/Expansion/databases/2025-07-13-dn/accumulate.db",

	// Historical databases
	"2024-03-31-bvn0-historical": "/media/paul/Expansion/databases/2024-03-31-bvn0-historical/accumulate.db",
	"2024-03-31-dn-historical":   "/media/paul/Expansion/databases/2024-03-31-dn-historical/accumulate.db",

	// Partition databases
	"2025-06-04-bvn0-partition": "/media/paul/Expansion/databases/2025-06-04-bvn0-partition/accumulate.db",
	"2025-06-04-bvn2-partition": "/media/paul/Expansion/databases/2025-06-04-bvn2-partition/accumulate.db",
	"2025-06-05-bvn1-partition": "/media/paul/Expansion/databases/2025-06-05-bvn1-partition/accumulate.db",
	"2025-06-06-dn-partition":   "/media/paul/Expansion/databases/2025-06-06-dn-partition/accumulate.db",

	// October databases
	"2025-10-22-bvn0": "/media/paul/Expansion/databases/2025-10-22-bvn0/accumulate.db",
	"2025-10-22-bvn1": "/media/paul/Expansion/databases/2025-10-22-bvn1/accumulate.db",
	"2025-10-22-bvn2": "/media/paul/Expansion/databases/2025-10-22-bvn2/accumulate.db",
	"2025-10-22-dn":   "/media/paul/Expansion/databases/2025-10-22-dn/accumulate.db",

	// Staking databases
	"2025-09-07-staking-original":    "/media/paul/Expansion/databases/2025-09-07-staking-original/accumulate.db",
	"2025-09-07-staking-post-genesis": "/media/paul/Expansion/databases/2025-09-07-staking-post-genesis/accumulate.db",
	"2025-10-24-staking-current":     "/media/paul/Expansion/databases/2025-10-24-staking-current/accumulate.db",

	// Other databases
	"2025-09-13-bvn0-repaired":       "/media/paul/Expansion/databases/2025-09-13-bvn0-repaired/accumulate.db",
	"2025-09-16-database-analysis":   "/media/paul/Expansion/databases/2025-09-16-database-analysis/accumulate.db",
	"2025-10-01-aws-mainnet-bvn0":    "/media/paul/Expansion/databases/2025-10-01-aws-mainnet-bvn0/accumulate.db",
	"2025-10-11-bvn0-clean":          "/media/paul/Expansion/databases/2025-10-11-bvn0-clean/accumulate.db",
	"2025-10-11-bvn0-clean-current":  "/media/paul/Expansion/databases/2025-10-11-bvn0-clean-current/accumulate.db",
	"2025-10-13-bootstrap-230000":    "/media/paul/Expansion/databases/2025-10-13-bootstrap-230000/accumulate.db",
	"2025-10-13-bootstrap-230546":    "/media/paul/Expansion/databases/2025-10-13-bootstrap-230546/accumulate.db",
}

// getDatabaseSearchOrder returns the priority order of databases to try for a given account URL
func getDatabaseSearchOrder(accountURL string) []string {
	u := strings.ToLower(accountURL)

	// DN accounts (system accounts)
	if strings.Contains(u, "dn.acme") || strings.Contains(u, "/acme") || u == "acc://acme" ||
		strings.Contains(u, "staking.acme") || strings.Contains(u, "oracle") {
		return []string{"2025-07-13-dn", "2025-10-22-dn", "2025-07-13-bvn0", "2025-07-13-bvn1", "2025-07-13-bvn2"}
	}

	// Default: try DN first, then BVNs
	return []string{"2025-07-13-dn", "2025-07-13-bvn0", "2025-07-13-bvn1", "2025-07-13-bvn2", "2025-10-22-dn"}
}

// resolveDatabase resolves a database name/path, with automatic routing if dbName is empty
func resolveDatabase(dbName string, accountURL string) (string, error) {
	// If database explicitly specified, use it
	if dbName != "" {
		if path, ok := knownDatabases[dbName]; ok {
			// Known databases are trusted, validate path exists
			absPath, err := ValidateDirectoryPath(path, "")
			if err != nil {
				return "", fmt.Errorf("known database path invalid: %w", err)
			}
			return absPath, nil
		}
		// User-provided path - validate to prevent directory traversal
		absPath, err := ValidateDirectoryPath(dbName, "")
		if err != nil {
			return "", fmt.Errorf("invalid database path: %w", err)
		}
		return absPath, nil
	}

	// Auto-routing: try databases in priority order
	searchOrder := getDatabaseSearchOrder(accountURL)
	for _, name := range searchOrder {
		if path, ok := knownDatabases[name]; ok {
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
	}

	return "", fmt.Errorf("no available database found for account %s", accountURL)
}

// dbList lists all known historical databases
func (s *Server) dbList(args map[string]interface{}) (map[string]interface{}, error) {
	var dbs []DatabaseInfo

	for name, path := range knownDatabases {
		info := DatabaseInfo{
			Name:   name,
			Path:   path,
			Exists: false,
		}

		if stat, err := os.Stat(path); err == nil {
			info.Exists = true
			info.LastModified = stat.ModTime().Format("2006-01-02 15:04:05")

			// Calculate directory size
			size, _ := getDirSize(path)
			info.SizeBytes = size
			info.SizeGB = fmt.Sprintf("%.2f GB", float64(size)/1e9)
		}

		dbs = append(dbs, info)
	}

	data, err := json.MarshalIndent(dbs, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal databases: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(data),
			},
		},
	}, nil
}

// dbQueryAccount queries an account from a historical database
func (s *Server) dbQueryAccount(args map[string]interface{}) (map[string]interface{}, error) {
	// Get account URL parameter (required)
	accountURL, ok := args["url"].(string)
	if !ok {
		return nil, fmt.Errorf("url parameter is required")
	}

	// Get database parameter (optional - will auto-route if not specified)
	dbName, _ := args["database"].(string)

	// Resolve database path with auto-routing
	dbPath, err := resolveDatabase(dbName, accountURL)
	if err != nil {
		return nil, err
	}

	// Check if database exists
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("database not found at %s: %w", dbPath, err)
	}

	// Parse URL
	u, err := url.Parse(accountURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %s: %w", accountURL, err)
	}

	// Get database connection from pool (reuses existing connection)
	db, err := s.dbManager.GetDatabase(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	// Note: Don't close - connection is managed by dbManager

	// Begin a read-only batch
	batch := db.Begin(false)
	defer batch.Discard()

	// Get the account
	account := batch.Account(u)

	// Read the main account record
	var acc protocol.Account
	err = account.Main().GetAs(&acc)
	if err != nil {
		return nil, fmt.Errorf("failed to get account %s: %w", accountURL, err)
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(acc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal account: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(data),
			},
		},
	}, nil
}

// dbListAccounts lists accounts from a historical database's BPT
func (s *Server) dbListAccounts(args map[string]interface{}) (map[string]interface{}, error) {
	// Get database parameter (optional - defaults to first available database)
	dbName, _ := args["database"].(string)

	// Get optional limit parameter (default 100)
	limit := 100
	if limitVal, ok := args["limit"]; ok {
		switch v := limitVal.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		case string:
			fmt.Sscanf(v, "%d", &limit)
		}
	}

	// Resolve database path (use DN as default for listing)
	var dbPath string
	if dbName != "" {
		// Explicit database specified
		if path, ok := knownDatabases[dbName]; ok {
			dbPath = path
		} else {
			dbPath = dbName
		}
	} else {
		// No database specified - use first available (DN priority)
		searchOrder := []string{"2025-07-13-dn", "2025-07-13-bvn0", "2025-07-13-bvn1", "2025-07-13-bvn2", "2025-10-22-dn"}
		for _, name := range searchOrder {
			if path, ok := knownDatabases[name]; ok {
				if _, err := os.Stat(path); err == nil {
					dbPath = path
					dbName = name
					break
				}
			}
		}
		if dbPath == "" {
			return nil, fmt.Errorf("no available database found")
		}
	}

	// Check if database exists
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("database not found at %s: %w", dbPath, err)
	}

	// Get database connection from pool (reuses existing connection)
	db, err := s.dbManager.GetDatabase(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	// Note: Don't close - connection is managed by dbManager

	// Begin a read-only batch
	batch := db.Begin(false)
	defer batch.Discard()

	var accounts []string
	count := 0

	// Iterate through BPT accounts
	it := batch.IterateAccounts()

	for it.Next() {
		if count >= limit {
			break
		}

		account := it.Value()
		if account != nil {
			accounts = append(accounts, account.Url().String())
			count++
		}
	}

	// Check for iteration errors (may occur with corrupted tables)
	// If we got some results, continue with warning rather than failing completely
	iterErr := it.Err()
	warning := ""
	if iterErr != nil {
		if len(accounts) > 0 {
			// Partial results - log warning but continue
			warning = fmt.Sprintf("BPT iteration incomplete due to corrupted entries: %v", iterErr)
		} else {
			// No results and error - this is a real failure
			return nil, fmt.Errorf("iteration error: %w", iterErr)
		}
	}

	// Format response
	result := map[string]interface{}{
		"database": dbName,
		"count":    len(accounts),
		"limit":    limit,
		"accounts": accounts,
	}

	if warning != "" {
		result["warning"] = warning
		result["partial"] = true
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal accounts: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(data),
			},
		},
	}, nil
}

// dbGetBPTHash gets the BPT root hash from a historical database
func (s *Server) dbGetBPTHash(args map[string]interface{}) (map[string]interface{}, error) {
	// Get database parameter
	dbName, ok := args["database"].(string)
	if !ok {
		return nil, fmt.Errorf("database parameter is required")
	}

	// Check if it's a known database name
	dbPath := dbName
	if path, ok := knownDatabases[dbName]; ok {
		dbPath = path
	}

	// Check if database exists
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("database not found at %s: %w", dbPath, err)
	}

	// Get database connection from pool (reuses existing connection)
	db, err := s.dbManager.GetDatabase(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	// Note: Don't close - connection is managed by dbManager

	// Begin a read-only batch
	batch := db.Begin(false)
	defer batch.Discard()

	// Get BPT root hash
	hash, err := batch.GetBptRootHash()
	if err != nil {
		return nil, fmt.Errorf("failed to get BPT root hash: %w", err)
	}

	// Format response
	result := map[string]interface{}{
		"bptRootHash": fmt.Sprintf("%x", hash),
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal hash: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(data),
			},
		},
	}, nil
}

// dbQueryTransaction queries a transaction from a historical database
func (s *Server) dbQueryTransaction(args map[string]interface{}) (map[string]interface{}, error) {
	// Get database parameter
	dbName, ok := args["database"].(string)
	if !ok {
		return nil, fmt.Errorf("database parameter is required")
	}

	// Get transaction ID parameter
	txid, ok := args["txid"].(string)
	if !ok {
		return nil, fmt.Errorf("txid parameter is required")
	}

	// Check if it's a known database name
	dbPath := dbName
	if path, ok := knownDatabases[dbName]; ok {
		dbPath = path
	}

	// Check if database exists
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("database not found at %s: %w", dbPath, err)
	}

	// Parse transaction ID
	id, err := url.ParseTxID(txid)
	if err != nil {
		return nil, fmt.Errorf("invalid transaction ID %s: %w", txid, err)
	}

	// Get database connection from pool (reuses existing connection)
	db, err := s.dbManager.GetDatabase(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	// Note: Don't close - connection is managed by dbManager

	// Begin a read-only batch
	batch := db.Begin(false)
	defer batch.Discard()

	// Get the transaction using the hash
	// batch.Transaction2() takes [32]byte and returns *Transaction (not *AccountTransaction)
	txn := batch.Transaction2(id.Hash())

	// Read the transaction data
	sigOrTxn, err := txn.Main().Get()
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	if sigOrTxn.Transaction == nil {
		return nil, fmt.Errorf("transaction not found")
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(sigOrTxn.Transaction, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal transaction: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(data),
			},
		},
	}, nil
}

// dbQueryChain queries a chain from a historical database
func (s *Server) dbQueryChain(args map[string]interface{}) (map[string]interface{}, error) {
	// Get account URL parameter (required)
	accountURL, ok := args["url"].(string)
	if !ok {
		return nil, fmt.Errorf("url parameter is required")
	}

	// Get chain name parameter (required)
	chainName, ok := args["chain_name"].(string)
	if !ok {
		return nil, fmt.Errorf("chain_name parameter is required")
	}

	// Get database parameter (optional - will auto-route if not specified)
	dbName, _ := args["database"].(string)

	// Get optional start parameter (default 0)
	start := uint64(0)
	if startVal, ok := args["start"]; ok {
		switch v := startVal.(type) {
		case float64:
			start = uint64(v)
		case int:
			start = uint64(v)
		case uint64:
			start = v
		}
	}

	// Get optional count parameter (default 10)
	count := uint64(10)
	if countVal, ok := args["count"]; ok {
		switch v := countVal.(type) {
		case float64:
			count = uint64(v)
		case int:
			count = uint64(v)
		case uint64:
			count = v
		}
	}

	// Resolve database path with auto-routing
	dbPath, err := resolveDatabase(dbName, accountURL)
	if err != nil {
		return nil, err
	}

	// Check if database exists
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("database not found at %s: %w", dbPath, err)
	}

	// Parse URL
	u, err := url.Parse(accountURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %s: %w", accountURL, err)
	}

	// Get database connection from pool (reuses existing connection)
	db, err := s.dbManager.GetDatabase(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	// Note: Don't close - connection is managed by dbManager

	// Begin a read-only batch
	batch := db.Begin(false)
	defer batch.Discard()

	// Get the account
	account := batch.Account(u)

	// Get the chain by name
	chain, err := account.ChainByName(chainName)
	if err != nil {
		return nil, fmt.Errorf("failed to get chain %s: %w", chainName, err)
	}

	// Get chain head to determine total entries
	head, err := chain.Head().Get()
	if err != nil {
		return nil, fmt.Errorf("failed to get chain head: %w", err)
	}

	// Build response with chain metadata and entries
	response := map[string]interface{}{
		"name":   chain.Name(),
		"type":   chain.Type().String(),
		"height": head.Count,
		"start":  start,
		"count":  count,
	}

	// Retrieve entries
	var entries []map[string]interface{}
	end := start + count
	if end > uint64(head.Count) {
		end = uint64(head.Count)
	}

	for i := start; i < end; i++ {
		entryData, err := chain.Entry(int64(i))
		if err != nil {
			return nil, fmt.Errorf("failed to get entry %d: %w", i, err)
		}

		entries = append(entries, map[string]interface{}{
			"index": i,
			"hash":  fmt.Sprintf("%x", entryData),
		})
	}

	response["entries"] = entries

	// Marshal to JSON
	data, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal chain data: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": string(data),
			},
		},
	}, nil
}

// getDirSize calculates the total size of a directory
func getDirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}
