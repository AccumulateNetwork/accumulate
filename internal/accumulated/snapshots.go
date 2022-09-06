package accumulated

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tendermint/tendermint/privval"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/blockscheduler"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/events"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
)

func (d *Daemon) collectSnapshot(batch *database.Batch, majorBlock, minorBlock uint64) {
	defer func() {
		if err := recover(); err != nil {
			d.Logger.Error("Panicked while creating snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot", "stack", string(debug.Stack()))
		}
	}()
	defer batch.Discard()

	d.Logger.Info("Creating a snapshot", "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot", "hash", logging.AsHex(batch.BptRoot()).Slice(0, 4))
	d.snapDir = config.MakeAbsolute(d.Config.RootDir, d.Config.Accumulate.Snapshots.Directory)
	err := os.Mkdir(d.snapDir, 0755)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		d.Logger.Error("Failed to create snapshot directory", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
		return
	}

	filename := filepath.Join(d.snapDir, fmt.Sprintf(core.SnapshotMajorFormat, minorBlock))
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

	d.triggerCollectStatesAt = minorBlock + 2

	// TODO maybe the code below should be after collecting the states
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

	entries, err := os.ReadDir(d.snapDir)
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
		err = os.Remove(filepath.Join(d.snapDir, filename))
		if err != nil {
			d.Logger.Error("Failed to prune snapshot", "error", err, "major-block", majorBlock, "minor-block", minorBlock, "module", "snapshot")
		}
	}
}

func (d *Daemon) collectStates(minorBlock uint64) {
	for blk := minorBlock - 2; blk <= minorBlock; blk++ {
		err := d.saveCommit(blk)
		if err != nil {
			return
		}
		err = d.saveBlock(blk)
		if err != nil {
			return
		}
	}
}

func (d *Daemon) saveCommit(minorBlock uint64) error {
	hdrFilename := filepath.Join(d.snapDir, fmt.Sprintf(core.SnapshotHeaderFormat, minorBlock))
	hdrFile, err := os.OpenFile(hdrFilename, os.O_RDWR|os.O_EXCL|os.O_CREATE, 0666)
	if err != nil {
		d.Logger.Error("Failed to create snapshot header", "error", err, "minor-block", minorBlock, "module", "snapshot")
		return nil
	}
	defer func() {
		err = hdrFile.Close()
		if err != nil {
			d.Logger.Error("Failed to close snapshot", "error", err, "minor-block", minorBlock, "module", "snapshot")
			return
		}
	}()
	cmtFilename := filepath.Join(d.snapDir, fmt.Sprintf(core.SnapshotBlockCommitFormat, minorBlock))
	cmtFile, err := os.OpenFile(cmtFilename, os.O_RDWR|os.O_EXCL|os.O_CREATE, 0666)
	if err != nil {
		d.Logger.Error("Failed to create snapshot block commit", "error", err, "minor-block", minorBlock, "module", "snapshot")
		return nil
	}
	defer func() {
		err = cmtFile.Close()
		if err != nil {
			d.Logger.Error("Failed to close block commit", "error", err, "minor-block", minorBlock, "module", "snapshot")
			return
		}
	}()
	height := int64(minorBlock)
	resCmt, err := d.localTm.Commit(context.Background(), &height)
	if err != nil {
		d.Logger.Error("Failed to fetch snapshot block commit", "error", err, "minor-block", minorBlock, "module", "snapshot")
		return nil
	}

	hdrBytes, err := json.Marshal(resCmt.Header)
	if err != nil {
		d.Logger.Error("Failed to marshal snapshot block header", "error", err, "minor-block", minorBlock, "module", "snapshot")
		return nil
	}
	hdrFile.Write(hdrBytes)

	cmtBytes, err := json.Marshal(resCmt.Header)
	if err != nil {
		d.Logger.Error("Failed to marshal snapshot block commit", "error", err, "minor-block", minorBlock, "module", "snapshot")
		return nil
	}
	cmtFile.Write(cmtBytes)
	return nil
}

func (d *Daemon) saveBlock(minorBlock uint64) error {
	blkFilename := filepath.Join(d.snapDir, fmt.Sprintf(core.SnapshotBlockFormat, minorBlock))
	blkFile, err := os.OpenFile(blkFilename, os.O_RDWR|os.O_EXCL|os.O_CREATE, 0666)
	if err != nil {
		d.Logger.Error("Failed to create snapshot block", "error", err, "minor-block", minorBlock, "module", "snapshot")
		return nil
	}
	defer func() {
		err = blkFile.Close()
		if err != nil {
			d.Logger.Error("Failed to close snapshot block", "error", err, "minor-block", minorBlock, "module", "snapshot")
			return
		}
	}()
	height := int64(minorBlock)
	blkRes, err := d.localTm.BlockResults(context.Background(), &height)
	if err != nil {
		d.Logger.Error("Failed to fetch snapshot block", "error", err, "minor-block", minorBlock, "module", "snapshot")
		return nil
	}

	var consensusBytes []byte
	if blkRes.ConsensusParamUpdates != nil {
		consensusBytes, err = blkRes.ConsensusParamUpdates.Marshal()
		if err != nil {
			d.Logger.Error("Failed to marshal snapshot block consensus params", "error", err, "minor-block", minorBlock, "module", "snapshot")
			return nil
		}
	}
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, uint16(len(consensusBytes)))
	blkFile.Write(lenBuf)
	if len(consensusBytes) > 0 {
		blkFile.Write(consensusBytes)
	}

	if blkRes.ValidatorUpdates != nil {
		binary.BigEndian.PutUint16(lenBuf, uint16(len(blkRes.ValidatorUpdates)))
		blkFile.Write(lenBuf)
		for _, validatorUpdate := range blkRes.ValidatorUpdates {
			validatorUpdateBytes, err := validatorUpdate.Marshal()
			if err != nil {
				d.Logger.Error("Failed to marshal snapshot block validator update", "error", err, "minor-block", minorBlock, "module", "snapshot")
				return nil
			}
			binary.BigEndian.PutUint16(lenBuf, uint16(len(validatorUpdateBytes)))
			blkFile.Write(lenBuf)
			blkFile.Write(validatorUpdateBytes)
		}
	} else {
		binary.BigEndian.PutUint16(lenBuf, uint16(0))
		blkFile.Write(lenBuf)
	}
	return nil
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
	router := routing.NewRouter(eventBus, nil)
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
