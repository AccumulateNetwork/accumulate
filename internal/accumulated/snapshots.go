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

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/consts"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
)

func (d *Daemon) onDidCommitBlock(event events.DidCommitBlock) {
	if d.Config.Accumulate.Snapshots.Frequency == 0 {
		return
	}

	// We have not implemented major blocks, so let's pretend like one happens
	// every 12 hours
	const majorBlockInterval = uint64(12 * time.Hour / time.Second)
	if event.Index%majorBlockInterval != 0 {
		return
	}
	majorBlock := event.Index / majorBlockInterval

	if majorBlock%uint64(d.Config.Accumulate.Snapshots.Frequency) != 0 {
		return
	}

	// Begin the batch synchronously immediately after commit
	batch := d.db.Begin(false)
	go d.collectSnapshot(batch, majorBlock, event.Index)
}

func (d *Daemon) collectSnapshot(batch *database.Batch, majorBlock, minorBlock uint64) {
	defer func() {
		if err := recover(); err != nil {
			d.Logger.Error("Panicked while creating snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot", "stack", string(debug.Stack()))
		}
	}()
	defer batch.Discard()

	d.Logger.Info("Creating a snapshot", "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot", "hash", logging.AsHex(batch.BptRoot()))
	snapDir := config.MakeAbsolute(d.Config.RootDir, d.Config.Accumulate.Snapshots.Directory)
	err := os.Mkdir(snapDir, 0755)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		d.Logger.Error("Failed to create snapshot directory", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
		return
	}

	filename := filepath.Join(snapDir, fmt.Sprintf(consts.SnapshotMajorFormat, minorBlock))
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

	err = batch.SaveSnapshot(file, &d.Config.Accumulate.Network)
	if err != nil {
		d.Logger.Error("Failed to create snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
		return
	}

	d.eventBus.Publish(events.DidSaveSnapshot{
		MinorIndex: minorBlock,
	})

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
		if !consts.SnapshotMajorRegexp.MatchString(entry.Name()) {
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
