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
	"time"

	tmed25519 "github.com/cometbft/cometbft/crypto/ed25519"
	cometLog "github.com/cometbft/cometbft/libs/log"
	cmtversion "github.com/cometbft/cometbft/proto/tendermint/version"
	cmttypes "github.com/cometbft/cometbft/types"
	"github.com/cometbft/cometbft/version"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/cometbft"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	var (
		dbPath     string
		dnDbPath   string
		outputFile string
		partition  string
		dbType     string
	)

	flag.StringVar(&dbPath, "db", "", "Path to database directory (e.g., /path/to/accumulate.db)")
	flag.StringVar(&dnDbPath, "dn-db", "", "Path to DN database for reading network definition (required for BVN snapshots)")
	flag.StringVar(&outputFile, "output", "", "Output .snap file path")
	flag.StringVar(&partition, "partition", "", "Partition name: Directory, Apollo, Yutu, Cyclops, or custom BVN name")
	flag.StringVar(&dbType, "type", "leveldb", "Database type: badger or leveldb")
	flag.Parse()

	if dbPath == "" || outputFile == "" || partition == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s -db <database-path> -output <snap-file> -partition <partition-name> [-dn-db <dn-database-path>]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nThis tool creates v2 snapshots with consensus section for use as genesis snapshots.\n")
		fmt.Fprintf(os.Stderr, "\nFor BVN snapshots, use -dn-db to specify the DN database path for reading network definition.\n")
		fmt.Fprintf(os.Stderr, "\nExample for DN:\n")
		fmt.Fprintf(os.Stderr, "  %s -db /path/to/dnn/data/accumulate.db -output dn.snap -partition Directory\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nExample for BVN (Cyclops/BVN2):\n")
		fmt.Fprintf(os.Stderr, "  %s -db /path/to/bvnn/data/accumulate.db -dn-db /path/to/dnn/data/accumulate.db -output bvn2.snap -partition Cyclops\n", os.Args[0])
		os.Exit(1)
	}

	// Create logger
	slogLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	logger := (*logging.Slogger)(slogLogger)
	cometLogger := cometLog.NewNopLogger()

	// Open database
	var db *database.Database
	var err error
	fmt.Printf("Opening %s database at %s...\n", dbType, dbPath)
	switch dbType {
	case "leveldb":
		db, err = database.OpenLevelDB(dbPath, logger)
	case "badger":
		db, err = database.OpenBadger(dbPath, cometLogger)
	default:
		fmt.Fprintf(os.Stderr, "Unknown database type: %s (use 'leveldb' or 'badger')\n", dbType)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Set observer
	db.SetObserver(execute.NewDatabaseObserver())

	// Open DN database if specified (for reading network definition)
	var dnDb *database.Database
	if dnDbPath != "" {
		fmt.Printf("Opening DN database at %s for network definition...\n", dnDbPath)
		switch dbType {
		case "leveldb":
			dnDb, err = database.OpenLevelDB(dnDbPath, logger)
		case "badger":
			dnDb, err = database.OpenBadger(dnDbPath, cometLogger)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open DN database: %v\n", err)
			os.Exit(1)
		}
		defer dnDb.Close()
		dnDb.SetObserver(execute.NewDatabaseObserver())
	}

	_ = logger
	_ = cometLogger

	// Create partition URL
	partitionURL := protocol.PartitionUrl(partition)

	// Create output directory if needed
	outputDir := filepath.Dir(outputFile)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	// Create output file
	fmt.Printf("Creating v2 snapshot file: %s\n", outputFile)
	file, err := os.Create(outputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create output file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	// Collect snapshot using v2 format with consensus section
	fmt.Printf("Collecting v2 snapshot for partition %s...\n", partition)
	fmt.Printf("Partition URL: %s\n", partitionURL.String())

	// Read the network definition to get validator info
	// Use DN database if specified, otherwise use the main database
	var netDefBatch *database.Batch
	if dnDb != nil {
		netDefBatch = dnDb.Begin(false)
		defer netDefBatch.Discard()
	} else {
		netDefBatch = db.Begin(false)
		defer netDefBatch.Discard()
	}

	// Get the chain ID for the partition
	chainID := "MainNet." + partition

	// Read the system ledger to get block height and timestamp
	mainBatch := db.Begin(false)
	defer mainBatch.Discard()

	var systemLedger *protocol.SystemLedger
	ledgerUrl := partitionURL.JoinPath(protocol.Ledger)
	err = mainBatch.Account(ledgerUrl).Main().GetAs(&systemLedger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read system ledger: %v\n", err)
		os.Exit(1)
	}

	blockHeight := int64(systemLedger.Index)
	blockTime := systemLedger.Timestamp
	if blockTime.IsZero() {
		blockTime = time.Now().UTC()
	}

	fmt.Printf("System ledger: block height=%d, timestamp=%v\n", blockHeight, blockTime)

	// Create a CometBFT Block with the block height and timestamp
	// This is required for CometBFT to properly initialize at the correct height
	createBlock := func() *cometbft.Block {
		block := &cmttypes.Block{
			Header: cmttypes.Header{
				Version: cmtversion.Consensus{
					Block: version.BlockProtocol,
				},
				ChainID:         chainID,
				Height:          blockHeight,
				Time:            blockTime,
				ProposerAddress: make([]byte, 20), // Required by CometBFT (20-byte placeholder)
			},
			LastCommit: &cmttypes.Commit{
				Height: blockHeight - 1,
			},
		}
		return (*cometbft.Block)(block)
	}

	// Create consensus document for the genesis snapshot
	// This will use the mainnet validators from the network definition
	_, err = db.Collect(file, partitionURL, &database.CollectOptions{
		BuildIndex: false,
		DidWriteHeader: func(w *snapshot.Writer) error {
			fmt.Printf("Writing consensus section...\n")

			// Create default consensus params
			defaults := cmttypes.DefaultConsensusParams()

			// Read the network definition from the DataAccount
			// The network definition is stored as serialized data inside a DataAccount
			var dataAccount *protocol.DataAccount
			err := netDefBatch.Account(protocol.DnUrl().JoinPath(protocol.Network)).Main().GetAs(&dataAccount)
			if err != nil {
				fmt.Printf("Warning: Could not read network data account: %v\n", err)
				// Create a minimal consensus doc without validators
				doc := new(cometbft.GenesisDoc)
				doc.ChainID = chainID
				// Use default consensus params
				doc.Params = (*cometbft.ConsensusParams)(defaults)
				// Add block data for CometBFT initialization
				doc.Block = createBlock()

				b, err := doc.MarshalBinary()
				if err != nil {
					return fmt.Errorf("marshal consensus doc: %w", err)
				}
				sw, err := w.OpenRaw(snapshot.SectionTypeConsensus)
				if err != nil {
					return fmt.Errorf("open consensus section: %w", err)
				}
				_, err = sw.Write(b)
				if err != nil {
					return fmt.Errorf("write consensus section: %w", err)
				}
				return sw.Close()
			}

			// Unmarshal the NetworkDefinition from the data account entry
			netDef := new(protocol.NetworkDefinition)
			if dataAccount.Entry == nil || len(dataAccount.Entry.GetData()) == 0 {
				fmt.Printf("Warning: Network data account has no data entry\n")
				// Create a minimal consensus doc without validators
				doc := new(cometbft.GenesisDoc)
				doc.ChainID = chainID
				doc.Params = (*cometbft.ConsensusParams)(defaults)
				// Add block data for CometBFT initialization
				doc.Block = createBlock()

				b, err := doc.MarshalBinary()
				if err != nil {
					return fmt.Errorf("marshal consensus doc: %w", err)
				}
				sw, err := w.OpenRaw(snapshot.SectionTypeConsensus)
				if err != nil {
					return fmt.Errorf("open consensus section: %w", err)
				}
				_, err = sw.Write(b)
				if err != nil {
					return fmt.Errorf("write consensus section: %w", err)
				}
				return sw.Close()
			}

			err = netDef.UnmarshalBinary(dataAccount.Entry.GetData()[0])
			if err != nil {
				fmt.Printf("Warning: Could not unmarshal network definition: %v\n", err)
				// Create a minimal consensus doc without validators
				doc := new(cometbft.GenesisDoc)
				doc.ChainID = chainID
				doc.Params = (*cometbft.ConsensusParams)(defaults)
				// Add block data for CometBFT initialization
				doc.Block = createBlock()

				b, err := doc.MarshalBinary()
				if err != nil {
					return fmt.Errorf("marshal consensus doc: %w", err)
				}
				sw, err := w.OpenRaw(snapshot.SectionTypeConsensus)
				if err != nil {
					return fmt.Errorf("open consensus section: %w", err)
				}
				_, err = sw.Write(b)
				if err != nil {
					return fmt.Errorf("write consensus section: %w", err)
				}
				return sw.Close()
			}

			fmt.Printf("Successfully read network definition with %d validators\n", len(netDef.Validators))

			// Build the consensus doc with validators
			doc := new(cometbft.GenesisDoc)
			doc.ChainID = chainID
			doc.Params = (*cometbft.ConsensusParams)(defaults)
			// Add block data for CometBFT initialization
			doc.Block = createBlock()

			// Add validators that are active on this partition
			for _, v := range netDef.Validators {
				if !v.IsActiveOn(partition) {
					continue
				}

				var name string
				if v.Operator == nil {
					name = fmt.Sprintf("Validator-%x", v.PublicKeyHash[:4])
				} else {
					name = v.Operator.ShortString()
				}

				key := tmed25519.PubKey(v.PublicKey)
				doc.Validators = append(doc.Validators, &cometbft.Validator{
					Address: key.Address(),
					PubKey:  key,
					Type:    protocol.SignatureTypeED25519,
					Power:   1,
					Name:    name,
				})
			}

			fmt.Printf("Added %d validators to consensus section\n", len(doc.Validators))

			// Write the consensus section
			b, err := doc.MarshalBinary()
			if err != nil {
				return fmt.Errorf("marshal consensus doc: %w", err)
			}
			sw, err := w.OpenRaw(snapshot.SectionTypeConsensus)
			if err != nil {
				return fmt.Errorf("open consensus section: %w", err)
			}
			_, err = sw.Write(b)
			if err != nil {
				return fmt.Errorf("write consensus section: %w", err)
			}
			return sw.Close()
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to collect snapshot: %v\n", err)
		os.Exit(1)
	}

	// Get file info for size
	info, err := file.Stat()
	if err == nil {
		fmt.Printf("\nV2 Snapshot successfully created!\n")
		fmt.Printf("  File: %s\n", outputFile)
		fmt.Printf("  Size: %.2f MB\n", float64(info.Size())/(1024*1024))
	} else {
		fmt.Printf("\nSnapshot created: %s\n", outputFile)
	}
}

