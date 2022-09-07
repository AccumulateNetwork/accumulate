package accumulated

import (
	"context"
	"errors"
	"fmt"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/privval"
	"github.com/tendermint/tendermint/types"
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
	tms := new(abci.StateSnapshot)
	tms.Height = minorBlock - 2
	for blkh := minorBlock - 2; blkh <= minorBlock; blkh++ {
		height := int64(blkh)

		// Get commit
		resCmt, err := d.localTm.Commit(context.Background(), &height)
		if err != nil {
			d.Logger.Error("Failed to fetch snapshot block commit", "error", err, "minor-block", minorBlock, "module", "snapshot")
			return
		}

		// Get ConsensusParams
		cpRes, err := d.localTm.ConsensusParams(context.Background(), &height)
		if err != nil {
			d.Logger.Error("Failed to fetch snapshot ConsensusParams", "error", err, "minor-block", minorBlock, "module", "snapshot")
			return
		}

		// Get validator set
		page := 1
		pageCnt := 100
		valRes, err := d.localTm.Validators(context.Background(), &height, &page, &pageCnt)
		if err != nil {
			d.Logger.Error("Failed to fetch validators set", "error", err, "minor-block", minorBlock, "module", "snapshot")
			return
		}

		validatorSet := &types.ValidatorSet{
			Validators: valRes.Validators,
			//Proposer:   valRes.Validators[1], // FIXME this happens to be at the time of writing, need to figure how to get this for real
			Proposer: valRes.Proposer,
		}
		lb := types.LightBlock{
			SignedHeader: &types.SignedHeader{
				Header: resCmt.Header,
				Commit: resCmt.Commit,
			},
			ValidatorSet: validatorSet,
		}
		lbProto, err := lb.ToProto()
		if err != nil {
			d.Logger.Error("LightBlock.toProto error", err)
			return
		}
		fmt.Println("AppHash for height", blkh, ":", resCmt.AppHash)
		fmt.Println("Header AppHash for height", blkh, ":", resCmt.Header.AppHash)
		fmt.Println("Header LastBlockID hash for height", blkh, ":", resCmt.LastBlockID.Hash)
		fmt.Println("Header LastCommitHash for height", blkh, ":", resCmt.Header.LastCommitHash)
		fmt.Println("Header DataHash for height", blkh, ":", resCmt.Header.DataHash)
		fmt.Println("Header EvidenceHash for height", blkh, ":", resCmt.Header.EvidenceHash)
		fmt.Println("Header ConsensusHash for height", blkh, ":", resCmt.Header.ConsensusHash)

		tms.Blocks = append(tms.Blocks, *lbProto)

		if blkh == minorBlock-1 {
			cpProto := cpRes.ConsensusParams.ToProto()
			tms.ConsensusParams = &cpProto
		}
	}

	tmsFilename := filepath.Join(d.snapDir, fmt.Sprintf(core.SnapshotTmStateFormat, minorBlock-2))
	tmsFile, err := os.OpenFile(tmsFilename, os.O_RDWR|os.O_EXCL|os.O_CREATE, 0666)
	if err != nil {
		d.Logger.Error("Failed to create state snapshot", "error", err, "minor-block", minorBlock, "module", "snapshot")
		return
	}
	bytes, err := tms.Marshal()
	if err != nil {
		d.Logger.Error("Failed to create marshal snapshot", "error", err, "minor-block", minorBlock, "module", "snapshot")
		return
	}
	tmsFile.Write(bytes)
	defer func() {
		err = tmsFile.Close()
		if err != nil {
			d.Logger.Error("Failed to close state snapshot", "error", err, "minor-block", minorBlock, "module", "snapshot")
			return
		}
	}()

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

	//d.node.RestoreState(lastBlock, currentBlock, nextBlock)
	return nil
}
