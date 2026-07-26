package server

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
)

// CursorState holds the state for pagination
type CursorState struct {
	Database      string `json:"database"`
	LastAccount   string `json:"last_account"`
	TotalReturned int    `json:"total_returned"`
}

// BPTEntry represents a single entry in the BPT with key, URL, and value
type BPTEntry struct {
	Key   string `json:"key"`   // The BPT record key (as hex hash)
	URL   string `json:"url"`   // The account URL (converted from key)
	Value string `json:"value"` // The BPT value (account state hash)
}

// dbIterateAccounts implements paginated iteration over database accounts with cursor
// FIXED: This now properly iterates ALL BPT entries, not just the first page
func (s *Server) dbIterateAccounts(args map[string]interface{}) (map[string]interface{}, error) {
	// Parse page_size (default: 1000 for efficient bulk extraction, max: 10000)
	pageSize := 1000
	if ps, ok := args["page_size"].(float64); ok {
		pageSize = int(ps)
		if pageSize > 10000 {
			pageSize = 10000
		}
		if pageSize < 1 {
			pageSize = 1
		}
	}

	// Parse cursor if provided
	var cursorState *CursorState
	if cursorStr, ok := args["cursor"].(string); ok && cursorStr != "" {
		cursorState = &CursorState{}
		cursorBytes, err := base64.StdEncoding.DecodeString(cursorStr)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor: %w", err)
		}
		if err := json.Unmarshal(cursorBytes, cursorState); err != nil {
			return nil, fmt.Errorf("invalid cursor format: %w", err)
		}
	}

	// Get database name
	database := ""
	if db, ok := args["database"].(string); ok {
		database = db
	} else if cursorState != nil {
		database = cursorState.Database
	}

	// Resolve database path
	var dbPath string
	if database != "" {
		// Explicit database specified
		if path, ok := knownDatabases[database]; ok {
			dbPath = path
		} else {
			dbPath = database
		}
	} else {
		// No database specified - use first available (DN priority)
		searchOrder := []string{"2025-07-13-dn", "2025-07-13-bvn0", "2025-07-13-bvn1", "2025-07-13-bvn2", "2025-10-22-dn"}
		for _, name := range searchOrder {
			if path, ok := knownDatabases[name]; ok {
				if _, err := os.Stat(path); err == nil {
					dbPath = path
					database = name
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

	// Iterate through BPT accounts
	it := batch.IterateAccounts()

	var entries []BPTEntry
	totalProcessed := 0
	skipping := false

	// If we have a cursor, we need to skip until we find the last account from previous page
	if cursorState != nil && cursorState.LastAccount != "" {
		skipping = true
		totalProcessed = cursorState.TotalReturned
	}

	// Iterate through ALL accounts in the BPT
	for it.Next() {
		account := it.Value()
		if account == nil {
			continue
		}

		accountURL := account.Url().String()

		// If we're skipping, look for the last account from the cursor
		if skipping {
			if accountURL == cursorState.LastAccount {
				// Found it! Stop skipping and start collecting from the NEXT account
				skipping = false
			}
			// Continue to next account (either still looking or just found the marker)
			continue
		}

		// Get the BPT value (account state hash) for this entry
		accountKey := account.Key()
		bptValue, err := batch.BPT().Get(accountKey)
		var valueHex string
		if err == nil && bptValue != nil {
			valueHex = hex.EncodeToString(bptValue)
		} else {
			// If we can't get the value, log but continue
			valueHex = ""
		}

		// Create BPT entry with Key + URL + Value
		keyHash := accountKey.Hash()
		entry := BPTEntry{
			Key:   hex.EncodeToString(keyHash[:]),
			URL:   accountURL,
			Value: valueHex,
		}

		// Collect this entry
		entries = append(entries, entry)

		// Stop if we've reached the page size
		if len(entries) >= pageSize {
			break
		}
	}

	// Check for iteration errors (may occur with corrupted tables)
	iterErr := it.Err()
	warning := ""
	if iterErr != nil {
		if len(entries) > 0 {
			// Partial results - log warning but continue
			warning = fmt.Sprintf("BPT iteration incomplete due to corrupted entries: %v", iterErr)
		} else {
			// No results and error - this is a real failure
			return nil, fmt.Errorf("iteration error: %w", iterErr)
		}
	}

	// Update total processed
	totalProcessed += len(entries)

	// Determine if there are more entries
	// If we got exactly pageSize entries, assume there might be more
	hasMore := len(entries) == pageSize

	// Get last account URL for next cursor
	var lastAccount string
	if len(entries) > 0 {
		lastAccount = entries[len(entries)-1].URL
	}

	// Create next cursor
	var nextCursor string
	if hasMore && lastAccount != "" {
		newCursorState := &CursorState{
			Database:      database,
			LastAccount:   lastAccount,
			TotalReturned: totalProcessed,
		}
		cursorBytes, err := json.Marshal(newCursorState)
		if err != nil {
			return nil, fmt.Errorf("failed to create cursor: %w", err)
		}
		nextCursor = base64.StdEncoding.EncodeToString(cursorBytes)
	}

	// Return result in MCP format with BPT entries (key + url + value)
	result := map[string]interface{}{
		"entries":         entries,
		"next_cursor":     nextCursor,
		"has_more":        hasMore,
		"total_processed": totalProcessed,
	}

	if warning != "" {
		result["warning"] = warning
		result["partial"] = true
	}

	return result, nil
}
