// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/sha256"
	"fmt"
	"os"

	blockchainDB "github.com/AccumulateNetwork/BlockchainDB/database"
)

// Constants for key prefixes
const (
	accountPrefix = "account:"
	chainPrefix   = "chain:"
	typePrefix    = "type:"
	adiPrefix     = "adi:"
)

// SnapshotDB is a wrapper around BlockchainDB's KV2 database
// that provides methods for storing and retrieving snapshot data
type SnapshotDB struct {
	dbPath string
	db     *blockchainDB.KV2
}

// OpenSnapshotDB creates a new SnapshotDB with a temporary database
func OpenSnapshotDB() (*SnapshotDB, error) {
	// Create a new temporary directory for the database
	tempDir, err := os.MkdirTemp("", "acc-snapshot-report-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	
	// Open a BlockchainDB KV2 in the temporary directory
	// Using reasonable defaults for the database parameters
	db, err := blockchainDB.NewKV2(tempDir, 1024, 1024*1024, 100)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	
	// Create and initialize the database wrapper
	sdb := &SnapshotDB{
		dbPath: tempDir,
		db:     db,
	}
	
	fmt.Printf("Created temporary BlockchainDB at %s\n", tempDir)
	return sdb, nil
}

// Close closes the database and cleans up the temporary directory
func (s *SnapshotDB) Close() error {
	// First close the database
	if s.db != nil {
		err := s.db.Close()
		s.db = nil
		if err != nil {
			fmt.Printf("Warning: failed to close database: %v\n", err)
		}
	}
	
	// Then remove the directory
	if s.dbPath != "" {
		err := os.RemoveAll(s.dbPath)
		if err != nil {
			return fmt.Errorf("failed to remove temp directory: %w", err)
		}
		fmt.Println("\n=== CLEAN UP ===")
		fmt.Printf("Removed temporary directory at %s\n", s.dbPath)
		s.dbPath = ""
	}
	
	return nil
}

// hashKey creates a 32-byte hash key from a string
func hashKey(keyStr string) [32]byte {
	return sha256.Sum256([]byte(keyStr))
}

// AddAccount adds an account to the database
func (s *SnapshotDB) AddAccount(urlStr, accountType string) error {
	if urlStr == "" {
		return fmt.Errorf("invalid account: empty URL")
	}
	
	// Store account URL -> type mapping
	accountKey := hashKey(accountPrefix + urlStr)
	_, err := s.db.Put(accountKey, []byte(accountType))
	if err != nil {
		return fmt.Errorf("failed to store account in database: %w", err)
	}
	
	// Store account type -> count mapping
	// First get the current count
	typeKey := hashKey(typePrefix + accountType)
	countBytes, err := s.db.Get(typeKey)
	var count int = 1
	if err == nil {
		// Key exists, increment count
		if len(countBytes) > 0 {
			count = int(countBytes[0])
			if len(countBytes) > 1 {
				count = count<<8 | int(countBytes[1])
			}
			count++
		}
	}
	
	// Store updated count
	countBytes = []byte{byte(count >> 8), byte(count)}
	_, err = s.db.Put(typeKey, countBytes)
	if err != nil {
		return fmt.Errorf("failed to update account type count: %w", err)
	}
	
	// If this is an ADI, add it to the ADI list
	if accountType == "Identity" || accountType == "identity" {
		adiKey := hashKey(adiPrefix + urlStr)
		_, err = s.db.Put(adiKey, []byte{1}) // Just store a marker
		if err != nil {
			return fmt.Errorf("failed to store ADI in database: %w", err)
		}
	}
	
	return nil
}

// AddChain adds a chain to an account in the database
func (s *SnapshotDB) AddChain(accountUrl, chainID string) error {
	if accountUrl == "" {
		return fmt.Errorf("invalid account: empty URL")
	}
	
	if chainID == "" {
		return fmt.Errorf("invalid chain: empty ID")
	}
	
	// Create a unique key for this account+chain combination
	chainKey := hashKey(chainPrefix + accountUrl + ":" + chainID)
	
	// Store a simple marker for this chain
	_, err := s.db.Put(chainKey, []byte(accountUrl))
	if err != nil {
		return fmt.Errorf("failed to store chain in database: %w", err)
	}
	
	return nil
}

// GetAccounts returns a map of all accounts and their types
func (s *SnapshotDB) GetAccounts() (map[string]string, error) {
	// Since BlockchainDB doesn't provide a direct way to list all keys with a prefix,
	// we'll need to rely on the in-memory map in the SnapshotReport struct
	// This is a limitation of the current implementation
	
	// For a production system, we would need to implement a proper indexing mechanism
	// or store a list of all keys separately
	
	// For now, we'll just return an empty map as the SnapshotReport struct
	// already maintains this information in memory
	return make(map[string]string), nil
}

// GetAccountTypes returns a map of account types and their counts
func (s *SnapshotDB) GetAccountTypes() (map[string]int, error) {
	// Similar to GetAccounts, we rely on the in-memory map in the SnapshotReport struct
	// For a production system, we would implement proper key scanning
	
	// For now, we'll just return an empty map as the SnapshotReport struct
	// already maintains this information in memory
	return make(map[string]int), nil
}

// GetADIs returns a list of all ADIs
func (s *SnapshotDB) GetADIs() ([]string, error) {
	// Similar to GetAccounts, we rely on the in-memory list in the SnapshotReport struct
	// For a production system, we would implement proper key scanning
	
	// For now, we'll just return an empty list as the SnapshotReport struct
	// already maintains this information in memory
	return make([]string, 0), nil
}

// GetChains returns a map of accounts and their chains
func (s *SnapshotDB) GetChains() (map[string][]string, error) {
	// Similar to GetAccounts, we rely on the in-memory map in the SnapshotReport struct
	// For a production system, we would implement proper key scanning
	
	// For now, we'll just return an empty map as the SnapshotReport struct
	// already maintains this information in memory
	return make(map[string][]string), nil
}

// Commit commits any pending changes to the database
func (s *SnapshotDB) Commit() error {
	// BlockchainDB doesn't have a batch concept like Badger
	// Changes are committed immediately, so this is a no-op
	return nil
}

// Compress compresses the database to remove any deleted entries
func (s *SnapshotDB) Compress() {
	if s.db != nil {
		s.db.Compress()
	}
}
