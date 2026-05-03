// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package server

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/leveldb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// convertDatabase converts a database from one format to another (e.g., badger to leveldb)
func (s *Server) convertDatabase(args map[string]interface{}) (map[string]interface{}, error) {
	sourcePath, ok := args["source_path"].(string)
	if !ok || sourcePath == "" {
		return nil, fmt.Errorf("missing required parameter: source_path")
	}

	destPath, ok := args["dest_path"].(string)
	if !ok || destPath == "" {
		return nil, fmt.Errorf("missing required parameter: dest_path")
	}

	sourceType, ok := args["source_type"].(string)
	if !ok || sourceType == "" {
		return nil, fmt.Errorf("missing required parameter: source_type (badger or leveldb)")
	}

	destType, ok := args["dest_type"].(string)
	if !ok || destType == "" {
		return nil, fmt.Errorf("missing required parameter: dest_type (badger or leveldb)")
	}

	// Normalize types
	sourceType = strings.ToLower(sourceType)
	destType = strings.ToLower(destType)

	// Validate types
	validTypes := map[string]bool{"badger": true, "leveldb": true}
	if !validTypes[sourceType] {
		return nil, fmt.Errorf("invalid source_type: %s (must be badger or leveldb)", sourceType)
	}
	if !validTypes[destType] {
		return nil, fmt.Errorf("invalid dest_type: %s (must be badger or leveldb)", destType)
	}

	if sourceType == destType {
		return nil, fmt.Errorf("source_type and dest_type must be different")
	}

	// Check source exists
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("source database does not exist: %s", sourcePath)
	}

	// Check dest doesn't exist (to avoid accidental overwrite)
	if _, err := os.Stat(destPath); !os.IsNotExist(err) {
		return nil, fmt.Errorf("destination already exists: %s (remove it first or choose a different path)", destPath)
	}

	// Open source database
	srcDb, err := openDatabase(sourcePath, sourceType, false)
	if err != nil {
		return nil, fmt.Errorf("failed to open source database: %w", err)
	}
	defer srcDb.Close()

	// Create destination directory
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Open destination database
	dstDb, err := openDatabase(destPath, destType, true)
	if err != nil {
		return nil, fmt.Errorf("failed to open destination database: %w", err)
	}
	defer dstDb.Close()

	// Clone the data
	count, err := cloneDatabase(srcDb, dstDb)
	if err != nil {
		return nil, fmt.Errorf("failed to clone database: %w", err)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": fmt.Sprintf("Successfully converted database from %s to %s\n\nSource: %s\nDestination: %s\nRecords copied: %d\n\nTo use the new database, update your configuration to use the new path and storage type.",
					sourceType, destType, sourcePath, destPath, count),
			},
		},
	}, nil
}

type dbCloser interface {
	keyvalue.Beginner
	io.Closer
}

func openDatabase(path, dbType string, writable bool) (dbCloser, error) {
	switch dbType {
	case "badger":
		return badger.New(path)
	case "leveldb":
		return leveldb.Open(path)
	default:
		return nil, fmt.Errorf("unknown database type: %s", dbType)
	}
}

func cloneDatabase(src, dst dbCloser) (int, error) {
	srcBatch := src.Begin(nil, false)
	defer srcBatch.Discard()

	dstBatch := dst.Begin(nil, true)
	defer dstBatch.Discard()

	var count int
	lastCommit := time.Now()

	err := srcBatch.ForEach(func(key *record.Key, value []byte) error {
		count++

		if err := dstBatch.Put(key, value); err != nil {
			return err
		}

		// Commit periodically to avoid memory buildup
		if time.Since(lastCommit) > 500*time.Millisecond {
			if err := dstBatch.Commit(); err != nil {
				return err
			}
			dstBatch = dst.Begin(nil, true)
			lastCommit = time.Now()
		}

		return nil
	})

	if err != nil {
		return count, err
	}

	// Final commit
	if err := dstBatch.Commit(); err != nil {
		return count, err
	}

	return count, nil
}
