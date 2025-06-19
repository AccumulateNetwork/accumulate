// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	snapshot "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
)

// ReadSnapshot reads a snapshot file into the database
func (sc *SnapCombine) ReadSnapshot(path string) error {
	// Initialize record type statistics map if needed
	if sc.RecordTypes == nil {
		sc.RecordTypes = make(map[string]int)
	}

	// Step 1: Open the snapshot file
	fmt.Printf("Opening snapshot file: %s\n", path)
	osFile, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open snapshot file: %w", err)
	}
	defer osFile.Close()

	// Step 2: Verify it's a valid snapshot (check version)
	version, err := snapshot.GetVersion(osFile)
	if err != nil {
		return fmt.Errorf("error determining snapshot version: %w", err)
	}

	// Only support version 2 snapshots
	if version != 2 {
		return fmt.Errorf("unsupported snapshot version: %d (only version 2 is supported)", version)
	}

	fmt.Printf("Snapshot version: %d\n", version)

	// Reset file position
	if _, err := osFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to reset file position: %w", err)
	}

	// Get file stats to determine size
	stat, err := osFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file stats: %w", err)
	}

	// Create a SectionReader
	sectionReader, err := ioutil2.NewSectionReader(osFile, 0, stat.Size())
	if err != nil {
		return fmt.Errorf("failed to create section reader: %w", err)
	}

	// Open the snapshot file
	reader, err := snapshot.Open(sectionReader)
	if err != nil {
		return fmt.Errorf("failed to open snapshot: %w", err)
	}

	// Step 3: Read all records from the snapshot
	fmt.Printf("Found %d sections in the snapshot\n", len(reader.Sections))

	// Process each section
	for i := 0; i < len(reader.Sections); i++ {
		section := reader.Sections[i]

		// Only process record sections
		if section.Type() != snapshot.SectionTypeRecords {
			fmt.Printf("Skipping non-record section %d (type=%d)\n", i, section.Type())
			continue
		}

		// Open the record section
		fmt.Printf("Processing record section %d (size=%d)...\n", i, section.Size())
		records, err := reader.OpenRecords(i)
		if err != nil {
			return fmt.Errorf("failed to open record section %d: %w", i, err)
		}

		// Read each record
		recordCount := 0
		for {
			entry, err := records.Read()
			if err != nil {
				if err == io.EOF {
					break
				}
				return fmt.Errorf("failed to read record: %w", err)
			}

			// Step 4: Process and store each record
			err = sc.processRecord(entry)
			if err != nil {
				fmt.Printf("Warning: failed to process record: %v\n", err)
				// Continue processing other records
			}

			recordCount++
			sc.RecordsRead++

			// Print progress every 10000 records
			if recordCount%10000 == 0 {
				fmt.Printf("Processed %d records in section %d\n", recordCount, i)
			}
		}

		fmt.Printf("Completed section %d: processed %d records\n", i, recordCount)
	}

	// Commit changes to the database
	if sc.db != nil {
		// TODO: Implement database commit if needed
	}

	sc.SnapshotsRead++
	fmt.Printf("Successfully processed snapshot: %s (%d records)\n", path, sc.RecordsRead)
	return nil
}

// processRecord processes a single record from the snapshot and stores it in the database
func (sc *SnapCombine) processRecord(entry *snapshot.RecordEntry) error {
	// Get the record type (first part of the key)
	if entry.Key == nil || entry.Key.Len() == 0 {
		return fmt.Errorf("empty key")
	}

	recordType := fmt.Sprint(entry.Key.Get(0))
	
	// Track record type statistics
	sc.RecordTypes[recordType]++

	// Create a unique key for the database
	// We use the full key path as the key
	keyStr := recordType
	for i := 1; i < entry.Key.Len(); i++ {
		keyStr += "/" + fmt.Sprint(entry.Key.Get(i))
	}

	// Create a hash key from the key string
	hashKey := sha256.Sum256([]byte(keyStr))

	// Extract account URL and chain ID if applicable
	accountURL := ""
	chainID := ""
	if recordType == "Account" && entry.Key.Len() >= 2 {
		accountURL = fmt.Sprint(entry.Key.Get(1))
		if entry.Key.Len() >= 3 {
			chainID = fmt.Sprint(entry.Key.Get(2))
		}
	}

	// If we have an account URL, store it in our URL hash mapping
	if accountURL != "" {
		// Store the URL hash mapping
		_, err := sc.StoreURLHash(accountURL)
		if err != nil {
			fmt.Printf("Warning: failed to store URL hash for %s: %v\n", accountURL, err)
		}
	}

	// Create a RecordKey entry
	recordKey := RecordKey{
		Hash:       hashKey,
		KeyPath:    keyStr,
		RecordType: recordType,
		AccountURL: accountURL,
		ChainID:    chainID,
	}

	// If using memory storage (for tests), add to in-memory structures
	if sc.useMemory {
		// Add to the list of record keys
		recordIndex := len(sc.RecordKeys)
		sc.RecordKeys = append(sc.RecordKeys, recordKey)

		// Update the record type index
		sc.RecordsByType[recordType] = append(sc.RecordsByType[recordType], recordIndex)

		// Update the account records index if applicable
		if accountURL != "" {
			sc.AccountRecords[accountURL] = append(sc.AccountRecords[accountURL], recordIndex)
		}
	} else {
		// Write the record key to the temporary file
		// Format: hash,keyPath,recordType,accountURL,chainID
		// We encode the hash as hex to avoid binary data issues
		hashHex := hex.EncodeToString(hashKey[:])
		
		// Escape commas in fields to prevent CSV parsing issues
		safeKeyStr := strings.ReplaceAll(keyStr, ",", "\\,")
		safeRecordType := strings.ReplaceAll(recordType, ",", "\\,")
		safeAccountURL := strings.ReplaceAll(accountURL, ",", "\\,")
		safeChainID := strings.ReplaceAll(chainID, ",", "\\,")
		
		// Create a CSV-like line with the record key data
		line := fmt.Sprintf("%s,%s,%s,%s,%s\n", 
			hashHex, 
			safeKeyStr, 
			safeRecordType, 
			safeAccountURL, 
			safeChainID)
		
		// Write to the temporary file
		_, err := sc.keysFile.WriteString(line)
		if err != nil {
			return fmt.Errorf("failed to write record key to temporary file: %w", err)
		}
	}

	// Store the record in the database
	// We store all records with their original key and value
	// This ensures we can recreate the snapshot exactly as it was
	if sc.db != nil {
		// Store the record in the database
		// If this record already exists (from another snapshot), it will be overwritten
		_, err := sc.db.Put(hashKey, entry.Value)
		if err != nil {
			return fmt.Errorf("failed to store record in database: %w", err)
		}
	}
	
	// Log account information
	if recordType == "Account" && accountURL != "" {
		fmt.Printf("Found account: %s (Type: %s)\n", accountURL, getAccountType(entry.Value))
	}
	
	return nil
}

// getAccountType extracts the account type from the record value
func getAccountType(value []byte) string {
	// Return Unknown if the value is too small
	if len(value) == 0 {
		return "Unknown"
	}
	
	// Convert to string for pattern matching
	dataStr := string(value)
	
	// Check for known patterns in the raw data
	switch {
	case strings.Contains(dataStr, "TokenAccount"):
		return "TokenAccount"
	case strings.Contains(dataStr, "LiteTokenAccount"):
		return "LiteTokenAccount"
	case strings.Contains(dataStr, "DataAccount"):
		return "DataAccount"
	case strings.Contains(dataStr, "LiteDataAccount"):
		return "LiteDataAccount"
	case strings.Contains(dataStr, "Identity") || strings.Contains(dataStr, "ADI"):
		return "Identity"
	case strings.Contains(dataStr, "KeyBook"):
		return "KeyBook"
	case strings.Contains(dataStr, "KeyPage"):
		return "KeyPage"
	case strings.Contains(dataStr, "SystemLedger"):
		return "SystemLedger"
	case strings.Contains(dataStr, "AnchorLedger"):
		return "AnchorLedger"
	case strings.Contains(dataStr, "SyntheticLedger"):
		return "SyntheticLedger"
	case strings.Contains(dataStr, "LiteIdentity"):
		return "LiteIdentity"
	case strings.Contains(dataStr, "BlockLedger"):
		return "BlockLedger"
	case strings.Contains(dataStr, "TokenIssuer"):
		return "TokenIssuer"
	}
	
	return "Unknown"
}
