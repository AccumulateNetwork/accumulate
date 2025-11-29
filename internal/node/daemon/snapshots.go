// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"time"

	cmtcfg "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/crypto"
	"github.com/cometbft/cometbft/crypto/ed25519"
	cmtstore "github.com/cometbft/cometbft/proto/tendermint/store"
	sm "github.com/cometbft/cometbft/state"
	"github.com/cometbft/cometbft/store"
	cmttypes "github.com/cometbft/cometbft/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/abci"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	sv2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/cometbft"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var (
	mSnapshotCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "accumulate",
		Subsystem: "snapshot",
		Name:      "collect_count",
		Help:      "The number of collected snapshots",
	})

	mSnapshotSkipped = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "accumulate",
		Subsystem: "snapshot",
		Name:      "collect_skipped",
		Help:      "The number of skipped snapshots",
	})

	mSnapshotFailed = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "accumulate",
		Subsystem: "snapshot",
		Name:      "collect_failed",
		Help:      "The number of failed snapshots",
	})

	mSnapshotDuration = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "accumulate",
		Subsystem: "snapshot",
		Name:      "collect_duration",
		Help:      "The time it takes to collect a snapshot.",
	})

	mSnapshotAccountRecords = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "accumulate",
		Subsystem: "snapshot",
		Name:      "collect_accountRecords",
		Help:      "The number of collected account records",
	})

	mSnapshotMessageRecords = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "accumulate",
		Subsystem: "snapshot",
		Name:      "collect_messageRecords",
		Help:      "The number of collected message records",
	})

	mSnapshotOtherRecords = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "accumulate",
		Subsystem: "snapshot",
		Name:      "collect_otherRecords",
		Help:      "The number of collected records other than accounts and messages",
	})
)

func (d *Daemon) onDidCommitBlock(event events.DidCommitBlock) error {
	if event.Major == 0 || !d.Config.Accumulate.Snapshots.Enable {
		return nil
	}

	// Begin the batch synchronously immediately after commit
	batch := d.db.Begin(false)
	go d.collectSnapshot(batch, event.Time, event.Major, event.Index)
	return nil
}

func (d *Daemon) collectSnapshot(batch *coredb.Batch, blockTime time.Time, majorBlock, minorBlock uint64) {
	if !d.isTimeForSnapshot(blockTime) {
		return
	}

	// Don't collect a snapshot if one is still being collected
	if !d.snapshotLock.TryLock() {
		mSnapshotSkipped.Inc()
		return
	}
	defer d.snapshotLock.Unlock()

	defer func() {
		if err := recover(); err != nil {
			d.Logger.Error("Panicked while creating snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot", "stack", string(debug.Stack()))
		}
	}()
	defer batch.Discard()

	mSnapshotCount.Inc()
	start := time.Now()
	defer func() { mSnapshotDuration.Set(time.Since(start).Seconds()) }()

	d.Logger.Info("Creating a snapshot", "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
	snapDir := config.MakeAbsolute(d.Config.RootDir, d.Config.Accumulate.Snapshots.Directory)
	err := os.Mkdir(snapDir, 0755)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		d.Logger.Error("Failed to create snapshot directory", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
		mSnapshotFailed.Inc()
		return
	}

	filename := filepath.Join(snapDir, fmt.Sprintf(core.SnapshotMajorFormat, minorBlock))
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_EXCL|os.O_CREATE, 0666)
	if err != nil {
		d.Logger.Error("Failed to create snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
		mSnapshotFailed.Inc()
		return
	}
	defer func() {
		err = file.Close()
		if err != nil {
			d.Logger.Error("Failed to close snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
			mSnapshotFailed.Inc()
			return
		}
	}()

	// Timer for updating progress
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()

	var metrics coredb.CollectMetrics
	_, err = batch.Collect(file, d.Config.Accumulate.PartitionUrl().URL, &coredb.CollectOptions{
		Metrics:    &metrics,
		BuildIndex: d.Config.Accumulate.Snapshots.EnableIndexing,
		Predicate: func(r database.Record) (bool, error) {
			switch r.Key().Get(0) {
			case "Account":
				mSnapshotAccountRecords.Inc()
			case "Message", "Transaction":
				mSnapshotMessageRecords.Inc()
			default:
				mSnapshotOtherRecords.Inc()
			}

			select {
			case <-tick.C:
			default:
				return true, nil
			}

			// The sole purpose of this function is to print progress
			switch r.Key().Get(0) {
			case "Account":
				k := r.Key().SliceJ(2)
				h := k.Hash()
				slog.Info("Collecting an account", "module", "snapshot", "majorBlock", majorBlock, "account", k, "hash", h[:4], "totalMessages", metrics.Messages.Count)

			case "Message", "Transaction":
				slog.Info("Collecting a message", "module", "snapshot", "majorBlock", majorBlock, "message", r.Key().Get(1), "count", fmt.Sprintf("%d/%d", metrics.Messages.Collecting, metrics.Messages.Count))
			}

			// Retain everything
			return true, nil
		},
	})
	if err != nil {
		d.Logger.Error("Failed to create snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
		mSnapshotFailed.Inc()
		return
	}

	err = d.eventBus.Publish(events.DidSaveSnapshot{
		MinorIndex: minorBlock,
	})
	if err != nil {
		d.Logger.Error("Failed to publish snapshot notification", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
		return
	}

	retain := d.Config.Accumulate.Snapshots.RetainCount
	if retain == 0 {
		return
	}

	entries, err := os.ReadDir(snapDir)
	if err != nil {
		d.Logger.Error("Failed to prune snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
		return
	}

	snapshots := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !core.SnapshotMajorRegexp.MatchString(entry.Name()) {
			continue
		}

		snapshots = append(snapshots, entry.Name())
	}

	sort.Strings(snapshots)
	if len(snapshots) <= retain {
		return
	}

	for _, filename := range snapshots[:len(snapshots)-retain] {
		err = os.Remove(filepath.Join(snapDir, filename))
		if err != nil {
			d.Logger.Error("Failed to prune snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
		}
	}
}

func (d *Daemon) LoadSnapshot(file ioutil2.SectionReader) error {
	fmt.Println("=== STARTING SNAPSHOT RESTORE ===")
	if d.Logger != nil {
		d.Logger.Info("=== STARTING SNAPSHOT RESTORE ===")
	}

	// First, extract the consensus section from the snapshot
	fmt.Println("Opening snapshot to extract consensus state")
	if d.Logger != nil {
		d.Logger.Info("Opening snapshot to extract consensus state")
	}
	_, err := file.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek to start: %v", err)
	}

	rd, err := sv2.Open(file)
	if err != nil {
		return fmt.Errorf("failed to open snapshot: %v", err)
	}

	fmt.Printf("Snapshot opened successfully - version: %d, sections: %d\n", rd.Header.Version, len(rd.Sections))
	fmt.Printf("Snapshot RootHash: %x\n", rd.Header.RootHash)

	// Look for the consensus section
	var consensusDoc *cometbft.GenesisDoc
	for i, section := range rd.Sections {
		fmt.Printf("Processing snapshot section %d: type=%v, size=%d\n", i, section.Type(), section.Size())

		if section.Type() != sv2.SectionTypeConsensus {
			continue
		}

		fmt.Printf("*** FOUND CONSENSUS SECTION *** index=%d, size=%d\n", i, section.Size())

		consensusDoc = new(cometbft.GenesisDoc)
		r, err := section.Open()
		if err != nil {
			return fmt.Errorf("failed to open consensus section: %v", err)
		}

		// Read the raw bytes for debugging
		rawBytes, err := io.ReadAll(r)
		if err != nil {
			return fmt.Errorf("failed to read consensus section: %v", err)
		}
		fmt.Printf("Read %d bytes from consensus section\n", len(rawBytes))

		// Try to unmarshal
		err = consensusDoc.UnmarshalBinary(rawBytes)
		if err != nil {
			return fmt.Errorf("failed to unmarshal consensus doc: %v", err)
		}

		fmt.Printf("Unmarshaled consensus doc: ChainID=%s, Params=%v, Validators=%v, Block=%v\n",
			consensusDoc.ChainID, consensusDoc.Params, consensusDoc.Validators, consensusDoc.Block)

		// Even if Block is nil, we still have ChainID which is valuable
		if consensusDoc.Block == nil && consensusDoc.ChainID == "" {
			fmt.Println("Consensus doc has no useful data - skipping")
			consensusDoc = nil
			continue
		}

		// If we have a chain ID but no block, we'll create a minimal genesis
		if consensusDoc.Block == nil {
			fmt.Println("WARNING: Consensus doc has ChainID but no Block data")
			fmt.Println("Will create minimal genesis with ChainID only")
		} else {
			fmt.Printf("Consensus doc unmarshaled successfully - chain_id=%s, height=%d, time=%v\n",
				consensusDoc.Block.ChainID, consensusDoc.Block.Height, consensusDoc.Block.Time)
		}
		break
	}

	if consensusDoc == nil {
		fmt.Println("No consensus section found in snapshot - CometBFT will use existing genesis or create new state")
	} else {
		// Write the genesis doc to CometBFT's genesis.json file
		genesisPath := filepath.Join(d.Config.RootDir, "config", "genesis.json")
		fmt.Printf("Writing CometBFT genesis document to: %s\n", genesisPath)

		// Convert validators from snapshot format to CometBFT format
		var validators []cmttypes.GenesisValidator
		for i, v := range consensusDoc.Validators {
			fmt.Printf("Converting validator %d: Type=%v, Power=%d, Name=%s, PubKeyLen=%d\n",
				i, v.Type, v.Power, v.Name, len(v.PubKey))

			// Convert public key based on type
			var pubKey crypto.PubKey
			switch v.Type {
			case protocol.SignatureTypeED25519:
				if len(v.PubKey) != ed25519.PubKeySize {
					fmt.Printf("WARNING: Invalid ED25519 public key length: %d (expected %d)\n", len(v.PubKey), ed25519.PubKeySize)
					continue
				}
				// ed25519.PubKey is a []byte, not an array, so we need to make a copy
				pk := make(ed25519.PubKey, ed25519.PubKeySize)
				copy(pk, v.PubKey)
				pubKey = pk
			default:
				fmt.Printf("WARNING: Unsupported signature type: %v\n", v.Type)
				continue
			}

			validators = append(validators, cmttypes.GenesisValidator{
				Address: v.Address,
				PubKey:  pubKey,
				Power:   v.Power,
				Name:    v.Name,
			})
		}
		fmt.Printf("Converted %d validators from snapshot\n", len(validators))

		// Convert cometbft.GenesisDoc to CometBFT's types.GenesisDoc
		var tmGenesisDoc *cmttypes.GenesisDoc
		if consensusDoc.Block != nil {
			tmGenesisDoc = &cmttypes.GenesisDoc{
				ChainID:         consensusDoc.Block.ChainID,
				GenesisTime:     consensusDoc.Block.Time,
				InitialHeight:   consensusDoc.Block.Height,
				ConsensusParams: cmttypes.DefaultConsensusParams(),
				AppHash:         rd.Header.RootHash[:],
				Validators:      validators,
			}
		} else {
			// No block data, create minimal genesis with just ChainID
			tmGenesisDoc = &cmttypes.GenesisDoc{
				ChainID:         consensusDoc.ChainID,
				GenesisTime:     time.Now().UTC(),
				InitialHeight:   1,
				ConsensusParams: cmttypes.DefaultConsensusParams(),
				AppHash:         rd.Header.RootHash[:],
				Validators:      validators,
			}
			fmt.Printf("Creating minimal genesis with ChainID=%s (no block data in snapshot)\n", consensusDoc.ChainID)
		}

		// Ensure config directory exists
		err = os.MkdirAll(filepath.Dir(genesisPath), 0755)
		if err != nil {
			return fmt.Errorf("failed to create config directory: %v", err)
		}

		// Use CometBFT's SaveAs which uses the proper JSON serialization
		// (cmtjson.MarshalIndent) that encodes int64 values as strings
		err = tmGenesisDoc.SaveAs(genesisPath)
		if err != nil {
			return fmt.Errorf("failed to write genesis.json: %v", err)
		}

		fmt.Printf("Genesis document written successfully - chain_id=%s, height=%d, time=%v\n",
			tmGenesisDoc.ChainID, tmGenesisDoc.InitialHeight, tmGenesisDoc.GenesisTime)

		// Initialize CometBFT's state.db with state derived from genesis
		// This is critical for snapshot restore to work - without this, CometBFT
		// will fail because the state has nil validators after the handshake.
		fmt.Println("Initializing CometBFT state.db from genesis")

		stateDBPath := filepath.Join(d.Config.RootDir, "data", "state.db")
		fmt.Printf("Opening state.db at: %s\n", stateDBPath)

		// Use CometBFT's DB provider to open state.db
		stateDB, err := cmtcfg.DefaultDBProvider(&cmtcfg.DBContext{
			ID:     "state",
			Config: &d.Config.Config,
		})
		if err != nil {
			return fmt.Errorf("failed to open state.db: %v", err)
		}
		defer stateDB.Close()

		// Create state store and make genesis state
		stateStore := sm.NewStore(stateDB, sm.StoreOptions{
			DiscardABCIResponses: false,
		})

		// Validate and complete the genesis doc
		err = tmGenesisDoc.ValidateAndComplete()
		if err != nil {
			return fmt.Errorf("failed to validate genesis doc: %v", err)
		}

		// Create initial state from genesis
		state, err := sm.MakeGenesisState(tmGenesisDoc)
		if err != nil {
			return fmt.Errorf("failed to make genesis state: %v", err)
		}

		// Keep CometBFT state at height 0 (genesis state).
		// CometBFT will skip reconstructLastCommit when LastBlockHeight=0.
		// The ABCI handshake will see the app at height 1 and synchronize.
		// We set the initial AppHash to match the snapshot's root hash.
		state.InitialHeight = 1
		state.AppHash = tmGenesisDoc.AppHash

		// Save state to state.db
		err = stateStore.Save(state)
		if err != nil {
			return fmt.Errorf("failed to save state to state.db: %v", err)
		}

		fmt.Printf("CometBFT state initialized - height=%d, validators=%d, appHash=%X\n",
			state.LastBlockHeight, state.Validators.Size(), state.AppHash)

		// Initialize CometBFT's blockstore.db to match state height
		// This is required because CometBFT's blocksync reactor requires state.LastBlockHeight == blockstore.Height()
		fmt.Println("Initializing CometBFT blockstore.db")

		blockstoreDB, err := cmtcfg.DefaultDBProvider(&cmtcfg.DBContext{
			ID:     "blockstore",
			Config: &d.Config.Config,
		})
		if err != nil {
			return fmt.Errorf("failed to open blockstore.db: %v", err)
		}
		defer blockstoreDB.Close()

		// Note: we don't need a BlockStore instance since we're not saving blocks
		// We only need to save the BlockStoreState directly

		// Save BlockStoreState at height 0 (matching state height 0)
		// No SeenCommit is needed when height is 0
		_ = store.NewBlockStore(blockstoreDB) // Create BlockStore to initialize DB schema
		bss := cmtstore.BlockStoreState{
			Base:   0,
			Height: 0,
		}
		store.SaveBlockStoreState(&bss, blockstoreDB)

		fmt.Printf("CometBFT blockstore initialized - base=%d, height=%d\n", bss.Base, bss.Height)

		// Create priv_validator_state.json (required by CometBFT for non-validators)
		privValStateFile := filepath.Join(d.Config.Config.BaseConfig.DBDir(), "priv_validator_state.json")
		privValState := []byte(`{"height":"0","round":0,"step":0}`)
		if err := os.WriteFile(privValStateFile, privValState, 0600); err != nil {
			return fmt.Errorf("failed to create priv_validator_state.json: %v", err)
		}
		fmt.Println("Created priv_validator_state.json")
	}

	// Reset file pointer for database restore
	fmt.Println("Restoring Accumulate database from snapshot")
	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek to start for database restore: %v", err)
	}

	db, err := coredb.Open(d.Config, d.Logger)
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}

	defer func() {
		_ = db.Close()
	}()

	// Set observer on database so all batches inherit it
	db.SetObserver(execute.NewDatabaseObserver())

	fmt.Println("Starting FullRestore")
	err = snapshot.FullRestore(db, file, d.Logger, d.Config.Accumulate.Describe.PartitionUrl())
	if err != nil {
		return fmt.Errorf("failed to restore database: %v", err)
	}

	fmt.Println("=== SNAPSHOT RESTORE COMPLETE ===")
	return nil
}

func (d *Daemon) isTimeForSnapshot(blockTime time.Time) bool {
	// If the schedule is unset, capture a snapshot on every major block
	if d.snapshotSchedule == nil {
		return true
	}

	// If there are no snapshots, capture a snapshot
	snapDir := config.MakeAbsolute(d.Config.RootDir, d.Config.Accumulate.Snapshots.Directory)
	snapshots, err := abci.ListSnapshots(snapDir)
	if err != nil || len(snapshots) == 0 {
		return true
	}

	// Order by time, descending
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Timestamp().After(snapshots[j].Timestamp())
	})

	// If the block time is after the next schedule time, capture a snapshot
	next := d.snapshotSchedule.Next(snapshots[0].Timestamp().Add(time.Nanosecond))
	return blockTime.Add(time.Nanosecond).After(next)
}
