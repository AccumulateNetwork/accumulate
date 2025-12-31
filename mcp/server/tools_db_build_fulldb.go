package server

import (
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"time"

	cometLog "github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// dbBuildFulldb builds a fulldb by extracting all BPT entries from source databases
// and creating a new BPT in the destination database
func (s *Server) dbBuildFulldb(args map[string]interface{}) (map[string]interface{}, error) {
	// Get output database path
	outputPath, ok := args["output"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: output")
	}

	// Get snapshot name (e.g., "2025-07-13")
	snapshot, ok := args["snapshot"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: snapshot")
	}

	// Build list of source databases for this snapshot
	partitions := []string{
		fmt.Sprintf("%s-dn", snapshot),
		fmt.Sprintf("%s-bvn0", snapshot),
		fmt.Sprintf("%s-bvn1", snapshot),
		fmt.Sprintf("%s-bvn2", snapshot),
	}

	log.Printf("Building fulldb from snapshot: %s", snapshot)
	log.Printf("Partitions: %v", partitions)
	log.Printf("Output: %s", outputPath)

	// Remove existing output database if it exists
	if err := os.RemoveAll(outputPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to remove existing database: %w", err)
	}

	// Create logger for database
	logger := cometLog.NewNopLogger()

	// Create output database using OpenBadger
	db, err := database.OpenBadger(outputPath, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create output database: %w", err)
	}
	defer db.Close()

	// Begin a writable batch
	batch := db.Begin(true)
	defer batch.Discard()

	totalAccounts := 0
	accountsPerPartition := make(map[string]int)

	// Process each partition
	for i, partition := range partitions {
		log.Printf("\n=== Extracting partition %d/%d: %s ===", i+1, len(partitions), partition)

		// Get database path
		var dbPath string
		if path, exists := knownDatabases[partition]; exists {
			dbPath = path
		} else {
			log.Printf("  WARNING: Database path not found for %s, skipping", partition)
			continue
		}

		// Check if database exists
		if _, err := os.Stat(dbPath); err != nil {
			log.Printf("  WARNING: Database not found at %s: %v, skipping", dbPath, err)
			continue
		}

		// Extract accounts from this partition
		entries, err := s.extractPartitionToBPT(dbPath, partition)
		if err != nil {
			log.Printf("  ERROR: Failed to extract partition %s: %v", partition, err)
			continue
		}

		// Add entries to destination BPT
		for _, entry := range entries {
			if err := batch.BPT().Insert(entry.Key, entry.Value); err != nil {
				log.Printf("  WARNING: Failed to add account to BPT: %v", err)
				continue
			}
		}

		accountsPerPartition[partition] = len(entries)
		totalAccounts += len(entries)
		log.Printf("✓ Partition %s: %d accounts extracted", partition, len(entries))
	}

	// Commit the batch
	log.Printf("\nCommitting BPT with %d total accounts...", totalAccounts)
	if err := batch.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit batch: %w", err)
	}

	// Get the BPT hash
	readBatch := db.Begin(false)
	defer readBatch.Discard()

	bptHash, err := readBatch.BPT().GetRootHash()
	if err != nil {
		return nil, fmt.Errorf("failed to get BPT hash: %w", err)
	}
	bptHashHex := hex.EncodeToString(bptHash[:])

	log.Printf("\n=== Fulldb Build Complete ===")
	log.Printf("Total accounts: %d", totalAccounts)
	log.Printf("Output: %s", outputPath)
	log.Printf("BPT hash: %s", bptHashHex)

	// Return result
	result := map[string]interface{}{
		"output":       outputPath,
		"snapshot":     snapshot,
		"total":        totalAccounts,
		"partitions":   accountsPerPartition,
		"bpt_hash":     bptHashHex,
	}

	return result, nil
}

// BPTExtractedEntry represents an extracted BPT entry
type BPTExtractedEntry struct {
	Key   *record.Key
	Value []byte
}

// extractPartitionToBPT extracts all accounts from a partition and returns the BPT entries
func (s *Server) extractPartitionToBPT(dbPath string, partition string) ([]BPTExtractedEntry, error) {
	start := time.Now()
	log.Printf("Starting extraction from database: %s", partition)

	// Open source database
	sourceDB, err := s.dbManager.GetDatabase(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Begin read-only batch on source
	sourceBatch := sourceDB.Begin(false)
	defer sourceBatch.Discard()

	// Iterate through all accounts in source BPT
	it := sourceBatch.IterateAccounts()
	var entries []BPTExtractedEntry

	for it.Next() {
		account := it.Value()
		if account == nil {
			continue
		}

		// Get account URL
		accountURL := account.Url()

		// Get the account key
		accountKey := account.Key()

		// Get the BPT value (account state hash) from source
		bptValue, err := sourceBatch.BPT().Get(accountKey)
		if err != nil {
			log.Printf("  WARNING: Failed to get BPT value for %s: %v", accountURL, err)
			continue
		}

		// We need to convert the URL to a record.Key for the destination
		destKey, err := urlToRecordKey(accountURL)
		if err != nil {
			log.Printf("  WARNING: Failed to convert URL to key for %s: %v", accountURL, err)
			continue
		}

		// Collect this entry
		entries = append(entries, BPTExtractedEntry{
			Key:   destKey,
			Value: bptValue,
		})

		if len(entries)%10000 == 0 {
			log.Printf("  Progress: %d accounts extracted", len(entries))
		}
	}

	// Check for iteration errors
	if err := it.Err(); err != nil {
		log.Printf("  WARNING: BPT iteration incomplete: %v", err)
	}

	duration := time.Since(start)
	log.Printf("Extraction complete: %d accounts extracted in %v", len(entries), duration)

	return entries, nil
}

// urlToRecordKey converts a URL to a record.Key
func urlToRecordKey(u *url.URL) (*record.Key, error) {
	// Use the database helper to map URL to key
	// For now, we'll use a simple approach - create a key from the URL string
	key := record.NewKey("Account", u.String())
	return key, nil
}
