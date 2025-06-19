// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	snapshot "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// writeRecordToSnapshot writes a single record to the snapshot
func writeRecordToSnapshot(section *snapshot.SectionWriter, keyPath string, value []byte) error {
	// Split the key path into components
	parts := strings.Split(keyPath, "/")
	
	// Convert parts to a slice of interface{}
	keyParts := make([]interface{}, len(parts))
	for i, part := range parts {
		keyParts[i] = part
	}
	
	// Create the key using the Append method
	key := record.NewKey(keyParts...)

	// Create a record entry
	entry := &snapshot.RecordEntry{
		Key:   key,
		Value: value,
	}

	// Write the record to the snapshot
	return section.WriteValue(entry)
}

// WriteSnapshot writes the combined snapshot to a file
func (sc *SnapCombine) WriteSnapshot(path string) error {
	// Check if database is available
	if sc.db == nil {
		return fmt.Errorf("database is not initialized")
	}

	// Check if we have any records to write when in memory mode
	if sc.useMemory && len(sc.RecordKeys) == 0 {
		return fmt.Errorf("no records to write")
	}
	
	// Check if we have a keys file when in file mode
	if !sc.useMemory && sc.keysFile == nil {
		return fmt.Errorf("keys file is not initialized")
	}

	// Step 1: Load URL hash mappings into memory for efficient lookups
	// This is important for reconstructing key paths with original URLs
	if !sc.useMemory {
		// Initialize the URL hash map if it's nil
		if sc.urlHashMap == nil {
			sc.urlHashMap = make(map[[32]byte]string)
		}
		
		// Only attempt to load from file if the URL hash file exists
		if sc.urlHashFile != nil {
			fmt.Println("Loading URL hash mappings into memory...")
			err := sc.LoadURLHashMap()
			if err != nil {
				fmt.Printf("Warning: failed to load URL hash map: %v\n", err)
				fmt.Println("Will continue with available URL mappings")
			}
		} else {
			fmt.Println("Warning: URL hash file not initialized, will use original key paths")
		}
	}

	// Step 2: Create a new snapshot file
	fmt.Printf("Creating output snapshot file: %s\n", path)
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	// Step 3: Create a snapshot writer
	writer, err := snapshot.Create(file)
	if err != nil {
		return fmt.Errorf("failed to create snapshot writer: %w", err)
	}

	// Step 4: Write the header
	err = writer.WriteHeader(&snapshot.Header{
		Version: snapshot.Version2,
	})
	if err != nil {
		return fmt.Errorf("failed to write snapshot header: %w", err)
	}

	// Step 5: Begin a record section
	section, err := writer.OpenRaw(snapshot.SectionTypeRecords)
	if err != nil {
		return fmt.Errorf("failed to begin record section: %w", err)
	}

	// Step 6: Process records from either memory or file
	recordCount := 0
	
	if sc.useMemory {
		// In-memory mode: iterate through tracked record keys
		fmt.Printf("Writing %d records from memory to snapshot...\n", len(sc.RecordKeys))
		
		// Process each record in order
		for _, recordKey := range sc.RecordKeys {
			// Get the value from the database using the hash key
			value, err := sc.db.Get(recordKey.Hash)
			if err != nil {
				fmt.Printf("Warning: failed to get record for key %s: %v\n", recordKey.KeyPath, err)
				continue
			}

			// Write the record to the snapshot
			err = writeRecordToSnapshot(section, recordKey.KeyPath, value)
			if err != nil {
				return fmt.Errorf("failed to write record: %w", err)
			}

			recordCount++
			sc.RecordsWritten++

			// Print progress every 10000 records
			if recordCount % 10000 == 0 {
				fmt.Printf("Wrote %d records to snapshot\n", recordCount)
			}
		}
	} else {
		// File-based mode: read record keys from temporary file
		fmt.Println("Reading record keys from temporary file...")
		
		// Rewind the file to the beginning
		_, err := sc.keysFile.Seek(0, 0)
		if err != nil {
			return fmt.Errorf("failed to rewind keys file: %w", err)
		}
		
		// Create a scanner to read the file line by line
		scanner := bufio.NewScanner(sc.keysFile)
		
		// Process each line in the file
		for scanner.Scan() {
			line := scanner.Text()
			
			// Parse the CSV line with escaped commas
			// Format: hashHex,keyPath,recordType,accountURL,chainID
			// We need to handle escaped commas in the fields
			var parts []string
			var currentPart string
			escaped := false
			
			for i := 0; i < len(line); i++ {
				char := line[i]
				
				if escaped {
					// Add the escaped character (including commas) to the current part
					currentPart += string(char)
					escaped = false
				} else if char == '\\' {
					// Next character is escaped
					escaped = true
				} else if char == ',' {
					// End of part
					parts = append(parts, currentPart)
					currentPart = ""
				} else {
					// Regular character
					currentPart += string(char)
				}
			}
			
			// Add the last part
			parts = append(parts, currentPart)
			
			if len(parts) < 5 {
				fmt.Printf("Warning: invalid record key line: %s\n", line)
				continue
			}
			
			// Extract the hash, key path, and other metadata
			hashHex := parts[0]
			keyPath := strings.ReplaceAll(parts[1], "\\,", ",") // Unescape commas
			recordType := strings.ReplaceAll(parts[2], "\\,", ",")
			accountURL := strings.ReplaceAll(parts[3], "\\,", ",")
			chainID := strings.ReplaceAll(parts[4], "\\,", ",")
			
			// Convert hash from hex to bytes
			hashBytes, err := hex.DecodeString(hashHex)
			if err != nil {
				fmt.Printf("Warning: invalid hash hex: %s - %v\n", hashHex, err)
				continue
			}
			
			// Convert to 32-byte array
			var hashKey [32]byte
			copy(hashKey[:], hashBytes)
			
			// Get the value from the database using the hash key
			value, err := sc.db.Get(hashKey)
			if err != nil {
				fmt.Printf("Warning: failed to get record for key %s: %v\n", keyPath, err)
				continue
			}
			
			// Check if this is an account record and we need to use the URL hash mapping
			if recordType == "Account" && accountURL != "" {
				// Use the URL hash map to get the original URL if available
				// This ensures we use the most accurate URL in the key path
				urlHash := sc.HashURL(accountURL)
				if originalURL, ok := sc.urlHashMap[urlHash]; ok && originalURL != accountURL {
					// Reconstruct the key path with the original URL
					newKeyPath := fmt.Sprintf("%s/%s", recordType, originalURL)
					if chainID != "" {
						newKeyPath += "/" + chainID
					}
					fmt.Printf("Using original URL for key path: %s -> %s\n", keyPath, newKeyPath)
					keyPath = newKeyPath
				}
			}
			
			// Write the record to the snapshot
			err = writeRecordToSnapshot(section, keyPath, value)
			if err != nil {
				return fmt.Errorf("failed to write record: %w", err)
			}
			
			recordCount++
			sc.RecordsWritten++
			
			// Print progress every 10000 records
			if recordCount % 10000 == 0 {
				fmt.Printf("Wrote %d records to snapshot\n", recordCount)
			}
		}
		
		// Check for scanner errors
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("error reading keys file: %w", err)
		}
	}

	// Step 6: End the record section
	err = section.Close()
	if err != nil {
		return fmt.Errorf("failed to end record section: %w", err)
	}

	// Step 7: Finalize the snapshot
	// Ensure all data is written to disk
	err = file.Sync()
	if err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}

	// Get file stats to report size
	stat, err := file.Stat()
	if err == nil {
		fmt.Printf("Snapshot written successfully: %s (size: %d bytes)\n", path, stat.Size())
	} else {
		fmt.Printf("Snapshot written successfully: %s (size unknown)\n", path)
	}

	return nil
}
