package accumulated

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"time"

	"github.com/tendermint/tendermint/privval"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/abci"
	"gitlab.com/accumulatenetwork/accumulate/internal/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/blockscheduler"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/events"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
)

func (d *Daemon) onDidCommitBlock(event events.DidCommitBlock) error {
	if event.Major == 0 {
		return nil
	}

	// Begin the batch synchronously immediately after commit
	batch := d.db.Begin(false)
	go d.collectSnapshot(batch, event.Time, event.Major, event.Index)
	return nil
}

func (d *Daemon) collectSnapshot(batch *database.Batch, blockTime time.Time, majorBlock, minorBlock uint64) {
	// Don't collect a snapshot if one is still being collected
	if !d.snapshotLock.TryLock() {
		return
	}
	defer d.snapshotLock.Unlock()

	defer func() {
		if err := recover(); err != nil {
			d.Logger.Error("Panicked while creating snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot", "stack", string(debug.Stack()))
		}
	}()
	defer batch.Discard()

	if !d.isTimeForSnapshot(blockTime) {
		return
	}

	d.Logger.Info("Creating a snapshot", "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot", "hash", logging.AsHex(batch.BptRoot()).Slice(0, 4))
	snapDir := config.MakeAbsolute(d.Config.RootDir, d.Config.Accumulate.Snapshots.Directory)
	err := os.Mkdir(snapDir, 0755)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		d.Logger.Error("Failed to create snapshot directory", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
		return
	}

	filename := filepath.Join(snapDir, fmt.Sprintf(core.SnapshotMajorFormat, minorBlock))
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_EXCL|os.O_CREATE, 0666)
	if err != nil {
		d.Logger.Error("Failed to create snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
		return
	}
	defer func() {
		err = file.Close()
		if err != nil {
			d.Logger.Error("Failed to close snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
			return
		}
	}()

	err = snapshot.FullCollect(batch, file, &d.Config.Accumulate.Describe)
	if err != nil {
		d.Logger.Error("Failed to create snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
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
	db, err := database.Open(d.Config, d.Logger)
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}

	defer func() {
		_ = db.Close()
	}()

	// read private validator
	pv, err := privval.LoadFilePV(
		d.Config.PrivValidator.KeyFile(),
		d.Config.PrivValidator.StateFile(),
	)
	if err != nil {
		return fmt.Errorf("failed to load private validator: %v", err)
	}

	eventBus := events.NewBus(d.Logger.With("module", "events"))
	router := routing.NewRouter(eventBus, nil, d.Logger)
	execOpts := block.ExecutorOptions{
		Logger:     d.Logger,
		Key:        pv.Key.PrivKey.Bytes(),
		Describe:   d.Config.Accumulate.Describe,
		Router:     router,
		EventBus:   eventBus,
		IsFollower: true,
	}

	// On DNs initialize the major block scheduler
	if execOpts.Describe.NetworkType == config.Directory {
		execOpts.MajorBlockScheduler = blockscheduler.Init(execOpts.EventBus)
	}

	exec, err := block.NewNodeExecutor(execOpts, db)
	if err != nil {
		return fmt.Errorf("failed to initialize chain executor: %v", err)
	}

	batch := db.Begin(true)
	defer batch.Discard()
	err = exec.RestoreSnapshot(batch, file)
	if err != nil {
		return fmt.Errorf("failed to restore snapshot: %v", err)
	}

	err = batch.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit changes: %v", err)
	}

	return nil
}

func (d *Daemon) isTimeForSnapshot(blockTime time.Time) bool {
	// If the schedule is unset, capture a snapshot on every major block
	if d.snapshotSchedule == nil {
		return true
	}

	// If there are no snapshots, capture a snapshot
	snapshots, err := abci.ListSnapshots(d.Config)
	if err != nil || len(snapshots) == 0 {
		return true
	}

	// Order by time, descending
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Timestamp.After(snapshots[j].Timestamp)
	})

	// If the block time is after the next schedule time, capture a snapshot
	next := d.snapshotSchedule.Next(snapshots[0].Timestamp)
	return blockTime.Add(time.Nanosecond).After(next)
}
