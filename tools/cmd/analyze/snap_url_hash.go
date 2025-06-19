// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// HashURL generates a SHA-256 hash for an account URL
func (sc *SnapCombine) HashURL(url string) [32]byte {
	return sha256.Sum256([]byte(url))
}

// StoreURLHash stores a URL and its hash in the URL hash mapping
// This allows us to efficiently retrieve URLs by their hash later
func (sc *SnapCombine) StoreURLHash(url string) ([32]byte, error) {
	// Generate the hash for the URL
	urlHash := sc.HashURL(url)
	
	if sc.useMemory {
		// In-memory storage (for testing)
		if sc.urlHashMap == nil {
			sc.urlHashMap = make(map[[32]byte]string)
		}
		
		// Check if this URL is already in the map with a different hash
		for existingHash, existingURL := range sc.urlHashMap {
			if existingURL == url && existingHash != urlHash {
				fmt.Printf("Warning: URL %s has multiple hashes\n", url)
				break
			}
		}
		
		sc.urlHashMap[urlHash] = url
	} else {
		// File-based storage (for production)
		if sc.urlHashFile == nil {
			// Create the URL hash file if it doesn't exist
			urlHashFile, err := os.CreateTemp(sc.dbPath, "url-hashes-")
			if err != nil {
				return urlHash, fmt.Errorf("failed to create URL hash file: %w", err)
			}
			sc.urlHashFile = urlHashFile
			sc.urlHashFilePath = urlHashFile.Name()
		}
		
		// Write the hash and URL to the file
		// Format: hashHex,url
		hashHex := hex.EncodeToString(urlHash[:])
		
		// Escape commas in URL to prevent CSV parsing issues
		safeURL := strings.ReplaceAll(url, ",", "\\,")
		line := fmt.Sprintf("%s,%s\n", hashHex, safeURL)
		
		_, err := sc.urlHashFile.WriteString(line)
		if err != nil {
			return urlHash, fmt.Errorf("failed to write URL hash to file: %w", err)
		}
	}
	
	return urlHash, nil
}

// GetURLFromHash retrieves a URL by its hash
// This is used when reading records from the database during snapshot writing
func (sc *SnapCombine) GetURLFromHash(urlHash [32]byte) (string, error) {
	if sc.useMemory {
		// In-memory lookup (for testing)
		if url, ok := sc.urlHashMap[urlHash]; ok {
			return url, nil
		}
		return "", fmt.Errorf("URL hash not found in memory map")
	} else {
		// File-based lookup (for production)
		if sc.urlHashFile == nil {
			return "", fmt.Errorf("URL hash file not initialized")
		}
		
		// Reset file position to beginning
		_, err := sc.urlHashFile.Seek(0, 0)
		if err != nil {
			return "", fmt.Errorf("failed to reset URL hash file position: %w", err)
		}
		
		// Read the file line by line to find the matching hash
		hashHex := hex.EncodeToString(urlHash[:])
		scanner := bufio.NewScanner(sc.urlHashFile)
		
		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.SplitN(line, ",", 2)
			
			if len(parts) != 2 {
				fmt.Printf("Warning: invalid URL hash mapping line: %s\n", line)
				continue // Invalid line format
			}
			
			if parts[0] == hashHex {
				// Unescape commas in URL
				url := strings.ReplaceAll(parts[1], "\\,", ",")
				return url, nil
			}
		}
		
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("error reading URL hash file: %w", err)
		}
		
		return "", fmt.Errorf("URL hash not found in file")
	}
}

// StoreRecordWithURLHash stores a record in the database using the URL hash as part of the key
// This allows us to efficiently retrieve records by account URL
func (sc *SnapCombine) StoreRecordWithURLHash(recordType, url, chainID string, value []byte) error {
	// Hash the URL
	urlHash, err := sc.StoreURLHash(url)
	if err != nil {
		return fmt.Errorf("failed to store URL hash: %w", err)
	}
	
	// Create a composite key using the record type, URL hash, and chain ID
	keyStr := fmt.Sprintf("%s/%x/%s", recordType, urlHash, chainID)
	keyHash := sha256.Sum256([]byte(keyStr))
	
	// Store the record in the database
	_, err = sc.db.Put(keyHash, value)
	if err != nil {
		return fmt.Errorf("failed to store record with URL hash: %w", err)
	}
	
	return nil
}

// LoadURLHashMap loads all URL hash mappings from the file into memory
// This is useful when writing the snapshot to avoid repeated file scans
func (sc *SnapCombine) LoadURLHashMap() error {
	if sc.useMemory {
		// Already in memory, nothing to do
		return nil
	}
	
	if sc.urlHashFile == nil {
		return fmt.Errorf("URL hash file not initialized")
	}
	
	// Initialize the in-memory map
	sc.urlHashMap = make(map[[32]byte]string)
	
	// Reset file position to beginning
	_, err := sc.urlHashFile.Seek(0, 0)
	if err != nil {
		return fmt.Errorf("failed to reset URL hash file position: %w", err)
	}
	
	// Read the file line by line and populate the map
	scanner := bufio.NewScanner(sc.urlHashFile)
	
	// Track statistics for logging
	lineCount := 0
	skippedLines := 0
	
	for scanner.Scan() {
		line := scanner.Text()
		lineCount++
		parts := strings.SplitN(line, ",", 2)
		
		if len(parts) != 2 {
			fmt.Printf("Warning: invalid URL hash mapping line %d: %s\n", lineCount, line)
			skippedLines++
			continue // Invalid line format
		}
		
		// Convert hex hash to byte array
		hashBytes, err := hex.DecodeString(parts[0])
		if err != nil {
			fmt.Printf("Warning: invalid hash hex at line %d: %s - %v\n", lineCount, parts[0], err)
			skippedLines++
			continue // Invalid hash format
		}
		
		// Check hash length
		if len(hashBytes) != 32 {
			fmt.Printf("Warning: hash length mismatch at line %d: got %d bytes, expected 32\n", lineCount, len(hashBytes))
			skippedLines++
			continue
		}
		
		// Convert to [32]byte and store in map
		var hash [32]byte
		copy(hash[:], hashBytes)
		
		// Unescape commas in URL
		url := strings.ReplaceAll(parts[1], "\\,", ",")
		sc.urlHashMap[hash] = url
	}
	
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading URL hash file: %w", err)
	}
	
	fmt.Printf("Loaded %d URL hash mappings into memory (skipped %d invalid lines)\n", len(sc.urlHashMap), skippedLines)
	return nil
}
