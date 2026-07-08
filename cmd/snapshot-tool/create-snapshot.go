// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	cmdutil "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func createSnapshot() {
	var (
		dataDir     string
		outputFile  string
		partition   string
		storageType string
		showVersion bool
	)

	flag.StringVar(&dataDir, "data", "", "Database data directory (e.g., .../dnn/data)")
	flag.StringVar(&outputFile, "output", "", "Output snapshot file")
	flag.StringVar(&partition, "partition", "", "Partition ID (Directory or Cyclops)")
	flag.StringVar(&storageType, "storage", "levelDB", "Storage type (badger or levelDB)")
	flag.BoolVar(&showVersion, "version", false, "Print version information and exit")
	flag.Parse()

	if showVersion {
		cmdutil.PrintVersion()
	}

	if dataDir == "" || outputFile == "" || partition == "" {
		fmt.Fprintf(os.Stderr, "Usage: create-snapshot -data <data-dir> -output <file> -partition <id> [-storage <type>]\n")
		fmt.Fprintf(os.Stderr, "Example: create-snapshot -data /path/to/dnn/data -output dir.snap -partition Directory\n")
		os.Exit(1)
	}

	// Create logger
	slogLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	logger := (*logging.Slogger)(slogLogger)

	// Open database
	fmt.Printf("Opening %s database at %s...\n", storageType, dataDir)
	dbPath := filepath.Join(dataDir, "accumulate.db")
	var db *database.Database
	var err error
	switch storageType {
	case "badger":
		db, err = database.OpenBadger(dbPath, logger)
	case "levelDB":
		db, err = database.OpenLevelDB(dbPath, logger)
	default:
		fmt.Fprintf(os.Stderr, "Unknown storage type: %s\n", storageType)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Create partition URL
	partitionURL := config.NetworkUrl{URL: protocol.PartitionUrl(partition)}

	// Create output file
	fmt.Printf("Creating snapshot file: %s\n", outputFile)
	file, err := os.Create(outputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create output file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	// Collect snapshot
	fmt.Printf("Collecting snapshot for partition %s...\n", partition)
	batch := db.Begin(false)
	defer batch.Discard()

	err = snapshot.FullCollect(batch, file, partitionURL, logger, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to collect snapshot: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Snapshot successfully created: %s\n", outputFile)
}
