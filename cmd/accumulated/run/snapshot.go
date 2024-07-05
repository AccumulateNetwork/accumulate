// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/abci"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var (
	snapshotNeedsEvents = ioc.Needs[*events.Bus](func(s *SnapshotService) string { return s.Partition })
)

func (s *SnapshotService) Requires() []ioc.Requirement {
	return s.Storage.Required(s.Partition)
}

func (s *SnapshotService) Provides() []ioc.Provided {
	return nil
}

func (s *SnapshotService) start(inst *Instance) error {
	setDefaultPtr(&s.RetainCount, 4)
	setDefaultPtr(&s.EnableIndexing, false)

	store, err := s.Storage.open(inst, s.Partition)
	if err != nil {
		return err
	}

	bus, err := snapshotNeedsEvents.Get(inst.services, s)
	if err != nil {
		return err
	}

	c := new(snapshotCollector)
	c.mu = new(sync.Mutex)
	c.partition = protocol.PartitionUrl(s.Partition)
	c.directory = inst.path(s.Directory)
	c.logger = inst.logger
	c.schedule = s.Schedule
	c.db = coredb.New(store, (*logging.Slogger)(inst.logger))
	c.index = *s.EnableIndexing
	c.events = bus
	c.retain = *s.RetainCount

	events.SubscribeSync(bus, c.didCommitBlock)
	return nil
}

type snapshotCollector struct {
	mu        *sync.Mutex
	partition *url.URL
	directory string
	logger    *slog.Logger
	schedule  *network.CronSchedule
	db        *coredb.Database
	index     bool
	events    *events.Bus
	retain    uint64
}

func (c *snapshotCollector) didCommitBlock(e events.DidCommitBlock) error {
	if e.Major == 0 {
		return nil
	}

	// Begin the batch synchronously immediately after commit
	batch := c.db.Begin(false)
	go c.collect(batch, e.Time, e.Major, e.Index)
	return nil
}

func (d *snapshotCollector) collect(batch *coredb.Batch, blockTime time.Time, majorBlock, minorBlock uint64) {
	if !d.isTimeForSnapshot(blockTime) {
		return
	}

	// Don't collect a snapshot if one is still being collected
	if !d.mu.TryLock() {
		return
	}
	defer d.mu.Unlock()

	defer func() {
		if err := recover(); err != nil {
			d.logger.Error("Panicked while creating snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot", "stack", string(debug.Stack()))
		}
	}()
	defer batch.Discard()

	d.logger.Info("Creating a snapshot", "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
	err := os.Mkdir(d.directory, 0755)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		d.logger.Error("Failed to create snapshot directory", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
		return
	}

	filename := filepath.Join(d.directory, fmt.Sprintf(core.SnapshotMajorFormat, minorBlock))
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_EXCL|os.O_CREATE, 0666)
	if err != nil {
		d.logger.Error("Failed to create snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
		return
	}
	defer func() {
		err = file.Close()
		if err != nil {
			d.logger.Error("Failed to close snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
			return
		}
	}()

	// Timer for updating progress
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()

	var metrics coredb.CollectMetrics
	err = batch.Collect(file, d.partition, &coredb.CollectOptions{
		Metrics:    &metrics,
		BuildIndex: d.index,
		Predicate: func(r database.Record) (bool, error) {
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
		d.logger.Error("Failed to create snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
		return
	}

	err = d.events.Publish(events.DidSaveSnapshot{
		MinorIndex: minorBlock,
	})
	if err != nil {
		d.logger.Error("Failed to publish snapshot notification", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
		return
	}

	if d.retain == 0 {
		return
	}

	entries, err := os.ReadDir(d.directory)
	if err != nil {
		d.logger.Error("Failed to prune snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
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
	if len(snapshots) <= int(d.retain) {
		return
	}

	for _, filename := range snapshots[:len(snapshots)-int(d.retain)] {
		err = os.Remove(filepath.Join(d.directory, filename))
		if err != nil {
			d.logger.Error("Failed to prune snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
		}
	}
}

func (c *snapshotCollector) isTimeForSnapshot(blockTime time.Time) bool {
	// If the schedule is unset, capture a snapshot on every major block
	if c.schedule == nil {
		return true
	}

	// If there are no snapshots, capture a snapshot
	snapshots, err := abci.ListSnapshots2(c.directory)
	if err != nil || len(snapshots) == 0 {
		return true
	}

	// Order by time, descending
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Timestamp.After(snapshots[j].Timestamp)
	})

	// If the block time is after the next schedule time, capture a snapshot
	next := c.schedule.Next(snapshots[0].Timestamp.Add(time.Nanosecond))
	return blockTime.Add(time.Nanosecond).After(next)
}
