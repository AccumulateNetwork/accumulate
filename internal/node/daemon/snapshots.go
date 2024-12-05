// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/abci"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
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
	db, err := coredb.Open(d.Config, d.Logger)
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}

	defer func() {
		_ = db.Close()
	}()

	err = snapshot.FullRestore(db, file, d.Logger, d.Config.Accumulate.Describe.PartitionUrl())
	if err != nil {
		return fmt.Errorf("failed to restore database: %v", err)
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
	next := d.snapshotSchedule.Next(snapshots[0].Timestamp.Add(time.Nanosecond))
	return blockTime.Add(time.Nanosecond).After(next)
}
