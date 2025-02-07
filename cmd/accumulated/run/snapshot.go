// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"github.com/cometbft/cometbft/rpc/client"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	sv1 "gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/abci"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	sv2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/cometbft"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var (
	snapshotNeedsEvents     = ioc.Needs[*events.Bus](func(s *SnapshotService) string { return s.Partition })
	snapshotWantsConsensus  = ioc.Wants[client.Client](func(s *SnapshotService) string { return s.Partition })
	snapshotProvidesService = ioc.Provides[api.SnapshotService](func(s *SnapshotService) string { return s.Partition })
)

func (s *SnapshotService) Requires() []ioc.Requirement {
	req := []ioc.Requirement{
		snapshotNeedsEvents.Requirement(s),
		snapshotWantsConsensus.Requirement(s),
	}
	req = append(req, s.Storage.Required(s.Partition)...)
	return req
}

func (s *SnapshotService) Provides() []ioc.Provided {
	return []ioc.Provided{
		snapshotProvidesService.Provided(s),
	}
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

	consensus, err := snapshotWantsConsensus.Get(inst.services, s)
	if err != nil {
		return err
	}

	c := new(snapshotCollector)
	c.context = inst.context
	c.mu = new(sync.Mutex)
	c.partition = protocol.PartitionUrl(s.Partition)
	c.directory = inst.path(s.Directory)
	c.logger = inst.logger
	c.schedule = s.Schedule
	c.db = coredb.New(store, (*logging.Slogger)(inst.logger))
	c.index = *s.EnableIndexing
	c.events = bus
	c.consensus = consensus
	c.retain = *s.RetainCount

	err = snapshotProvidesService.Register(inst.services, s, c)
	if err != nil {
		return err
	}
	registerRpcService(inst, api.ServiceTypeSnapshot.AddressFor(s.Partition), message.SnapshotService{SnapshotService: c})

	events.SubscribeSync(bus, c.didCommitBlock)
	return nil
}

type snapshotCollector struct {
	context   context.Context
	mu        *sync.Mutex
	partition *url.URL
	directory string
	logger    *slog.Logger
	schedule  *network.CronSchedule
	db        *coredb.Database
	index     bool
	events    *events.Bus
	consensus client.Client
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

func (c *snapshotCollector) collect(batch *coredb.Batch, blockTime time.Time, majorBlock, minorBlock uint64) {
	if !c.isTimeForSnapshot(blockTime) {
		return
	}

	// Don't collect a snapshot if one is still being collected
	if !c.mu.TryLock() {
		return
	}
	defer c.mu.Unlock()

	defer func() {
		if err := recover(); err != nil {
			c.logger.Error("Panicked while creating snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot", "stack", string(debug.Stack()))
		}
	}()
	defer batch.Discard()

	// Clear the manual trigger file, if it exists
	if err := os.Remove(filepath.Join(c.directory, ".capture")); err != nil && !errors.Is(err, fs.ErrNotExist) {
		c.logger.Error("Failed to remove manual trigger file", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
	}

	c.logger.Info("Creating a snapshot", "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
	err := os.Mkdir(c.directory, 0755)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		c.logger.Error("Failed to create snapshot directory", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
		return
	}

	filename := filepath.Join(c.directory, fmt.Sprintf(core.SnapshotMajorFormat, minorBlock))
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_EXCL|os.O_CREATE, 0666)
	if err != nil {
		c.logger.Error("Failed to create snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
		return
	}
	defer func() {
		err = file.Close()
		if err != nil {
			c.logger.Error("Failed to close snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
			return
		}
	}()

	// Timer for updating progress
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()

	var metrics coredb.CollectMetrics
	w, err := batch.Collect(file, c.partition, &coredb.CollectOptions{
		Metrics:    &metrics,
		BuildIndex: c.index,
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
		c.logger.Error("Failed to create snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
		return
	}

	// Write consensus info
	err = c.collectConsensusDoc(w, minorBlock)
	if err != nil {
		c.logger.Error("Failed to collect consensus doc", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
	}

	err = c.events.Publish(events.DidSaveSnapshot{
		MinorIndex: minorBlock,
	})
	if err != nil {
		c.logger.Error("Failed to publish snapshot notification", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
		return
	}

	if c.retain == 0 {
		return
	}

	entries, err := os.ReadDir(c.directory)
	if err != nil {
		c.logger.Error("Failed to prune snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
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
	if len(snapshots) <= int(c.retain) {
		return
	}

	for _, filename := range snapshots[:len(snapshots)-int(c.retain)] {
		err = os.Remove(filepath.Join(c.directory, filename))
		if err != nil {
			c.logger.Error("Failed to prune snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
		}
	}
}

func (c *snapshotCollector) isTimeForSnapshot(blockTime time.Time) bool {
	// This is a hack to manually trigger a snapshot
	if st, err := os.Stat(filepath.Join(c.directory, ".capture")); err == nil && !st.IsDir() {
		return true
	}

	// Don't capture a snapshot unless it's manually triggered or scheduled
	if c.schedule == nil {
		return false
	}

	// If there are no snapshots, capture a snapshot
	snapshots, err := abci.ListSnapshots(c.context, c.directory)
	if err != nil || len(snapshots) == 0 {
		return true
	}

	// Order by time, descending
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Timestamp().After(snapshots[j].Timestamp())
	})

	// If the block time is after the next schedule time, capture a snapshot
	next := c.schedule.Next(snapshots[0].Timestamp().Add(time.Nanosecond))
	return blockTime.Add(time.Nanosecond).After(next)
}

func (c *snapshotCollector) collectConsensusDoc(w *sv2.Writer, minorBlock uint64) error {
	if c.consensus == nil {
		return nil
	}

	ctx := context.Background()
	block, err := c.consensus.Block(ctx, Ptr(int64(minorBlock)+1))
	if err != nil {
		return errors.UnknownError.WithFormat("get consensus block %d: %w", minorBlock+1, err)
	}
	if block.Block.LastCommit.Height != int64(minorBlock) {
		return errors.InvalidRecord.WithFormat("last commit height does not match: want %d, got %d", minorBlock, block.Block.LastCommit.Height)
	}
	// TODO: Check block.Block.AppHash?

	doc := new(cometbft.GenesisDoc)
	doc.Block = (*cometbft.Block)(block.Block)

	b, err := doc.MarshalBinary()
	if err != nil {
		return errors.UnknownError.WithFormat("marshal consensus doc: %w", err)
	}

	sw, err := w.OpenRaw(sv2.SectionTypeConsensus)
	if err != nil {
		return errors.UnknownError.WithFormat("open consensus section: %w", err)
	}
	defer sw.Close()

	_, err = sw.Write(b)
	if err != nil {
		return errors.UnknownError.WithFormat("write consensus section: %w", err)
	}

	return nil
}

var _ api.SnapshotService = (*snapshotCollector)(nil)

func (c *snapshotCollector) ListSnapshots(ctx context.Context, opts api.ListSnapshotsOptions) ([]*api.SnapshotInfo, error) {
	snapshots, err := abci.ListSnapshots(ctx, c.directory)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("list snapshots: %w", err)
	}

	info := make([]*api.SnapshotInfo, 0, len(snapshots))
	for _, s := range snapshots {
		err := (func() error {
			f, err := s.Open()
			if err != nil {
				return errors.UnknownError.WithFormat("open snapshot file: %w", err)
			}
			defer f.Close()

			switch s.Version() {
			case sv1.Version1:
				h, _, err := sv1.Open(f)
				if err != nil {
					return errors.UnknownError.WithFormat("open snapshot (v1): %w", err)
				}

				// Version 1 snapshots do not include consensus info
				info = append(info, &api.SnapshotInfo{
					Header: &sv2.Header{
						Version:  h.Version,
						RootHash: h.RootHash,
						SystemLedger: &protocol.SystemLedger{
							Url:             c.partition,
							Index:           h.Height,
							Timestamp:       h.Timestamp,
							ExecutorVersion: h.ExecutorVersion,
						},
					},
				})
				return nil

			case sv2.Version2:
				r, err := sv2.Open(f)
				if err != nil {
					return errors.UnknownError.WithFormat("open snapshot (v2): %w", err)
				}

				var consensus *cometbft.GenesisDoc
				for _, s := range r.Sections {
					if s.Type() != sv2.SectionTypeConsensus {
						continue
					}

					consensus = new(cometbft.GenesisDoc)
					r, err := s.Open()
					if err != nil {
						return errors.UnknownError.WithFormat("open consensus section: %w", err)
					}

					err = consensus.UnmarshalBinaryFrom(r)
					if err != nil {
						return errors.UnknownError.WithFormat("unmarshal consensus doc: %w", err)
					}
				}

				info = append(info, &api.SnapshotInfo{
					Header:        r.Header,
					ConsensusInfo: consensus,
				})
				return nil

			default:
				return errors.UnknownError.WithFormat("unknown snapshot version: %d", s.Version())
			}
		})()
		if err != nil {
			c.logger.ErrorContext(ctx, "Failed to read snapshot", "error", err, "module", "snapshots", "height", s.Height())
		}
	}
	return info, nil
}
