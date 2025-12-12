// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
)

func mainOld() {
	var (
		workDir    string
		outputFile string
	)

	flag.StringVar(&workDir, "work-dir", "", "Node work directory (e.g., /path/to/dnn)")
	flag.StringVar(&outputFile, "output", "", "Output snapshot file path")
	flag.Parse()

	if workDir == "" || outputFile == "" {
		fmt.Fprintf(os.Stderr, "Usage: snapshot-tool -work-dir <node-dir> -output <snapshot-file>\n")
		fmt.Fprintf(os.Stderr, "Example: snapshot-tool -work-dir /path/to/dnn -output directory-snapshot.snap\n")
		os.Exit(1)
	}

	// Load config from work directory
	fmt.Printf("Loading config from %s...\n", workDir)
	cfg, err := config.Load(workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Create logger
	logger, err := logging.NewConsoleWriter("plain")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	tmLogger := log.NewTMLogger(logger)

	// Open database
	fmt.Printf("Opening database at %s...\n", workDir)
	db, err := database.Open(cfg, tmLogger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Set observer
	db.SetObserver(execute.NewDatabaseObserver())

	// Create output file
	fmt.Printf("Creating snapshot file: %s\n", outputFile)
	file, err := os.Create(outputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create output file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	// Collect snapshot
	partition := cfg.Accumulate.PartitionId
	fmt.Printf("Collecting snapshot for partition %s...\n", partition)
	batch := db.Begin(false)
	defer batch.Discard()

	networkURL := cfg.Accumulate.PartitionUrl()
	err = snapshot.FullCollect(batch, file, networkURL, tmLogger, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to collect snapshot: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Snapshot successfully created: %s\n", outputFile)
}

func main() {
	createSnapshot()
}
