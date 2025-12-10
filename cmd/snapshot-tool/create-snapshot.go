package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/leveldb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func createSnapshot() {
	var (
		dataDir     string
		outputFile  string
		network     string
		partition   string
		storageType string
	)

	flag.StringVar(&dataDir, "data", "", "Database data directory (e.g., .../dnn/data)")
	flag.StringVar(&outputFile, "output", "", "Output snapshot file")
	flag.StringVar(&network, "network", "MainNet", "Network name")
	flag.StringVar(&partition, "partition", "", "Partition ID (Directory or Cyclops)")
	flag.StringVar(&storageType, "storage", "levelDB", "Storage type (badger or levelDB)")
	flag.Parse()

	if dataDir == "" || outputFile == "" || partition == "" {
		fmt.Fprintf(os.Stderr, "Usage: create-snapshot -data <data-dir> -output <file> -partition <id> [-network <name>] [-storage <type>]\n")
		fmt.Fprintf(os.Stderr, "Example: create-snapshot -data /path/to/dnn/data -output dir.snap -partition Directory\n")
		os.Exit(1)
	}

	// Create logger
	logger, err := logging.NewConsoleWriter("plain")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	tmLogger := log.NewTMLogger(logger)

	// Open key-value store directly
	fmt.Printf("Opening %s database at %s...\n", storageType, dataDir)
	var store keyvalue.Beginner
	switch storageType {
	case "badger":
		store, err = badger.New(filepath.Join(dataDir, "accumulate.db"), tmLogger)
	case "levelDB":
		store, err = leveldb.OpenLevelDB(filepath.Join(dataDir, "accumulate.db"), tmLogger, false)
	default:
		fmt.Fprintf(os.Stderr, "Unknown storage type: %s\n", storageType)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open store: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	// Create database wrapper
	db := database.New(store, tmLogger)
	db.SetObserver(execute.NewDatabaseObserver())

	// Create partition URL
	partitionURL := protocol.PartitionUrl(partition)
	networkURL, err := url.Parse(fmt.Sprintf("acc://%s.acme", network))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse network URL: %v\n", err)
		os.Exit(1)
	}

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

	partitionURL.URL = networkURL
	err = snapshot.FullCollect(batch, file, partitionURL, tmLogger, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to collect snapshot: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Snapshot successfully created: %s\n", outputFile)
}
