package abci

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	abci "github.com/tendermint/tendermint/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	_ "gitlab.com/accumulatenetwork/accumulate/internal/database/smt/pmt"
)

// ListSnapshots queries the node for available snapshots.
func (app *Accumulator) ListSnapshots(req abci.RequestListSnapshots) abci.ResponseListSnapshots {
	snapDir := config.MakeAbsolute(app.RootDir, app.Accumulate.Snapshots.Directory)
	entries, err := os.ReadDir(snapDir)
	if err != nil {
		app.logger.Error("Failed to load snapshot", "error", err)
		return abci.ResponseListSnapshots{}
	}

	var resp abci.ResponseListSnapshots
	resp.Snapshots = make([]*abci.Snapshot, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !core.SnapshotMajorRegexp.MatchString(entry.Name()) {
			continue
		}

		filename := filepath.Join(snapDir, entry.Name())
		f, err := os.Open(filename)
		if err != nil {
			app.logger.Error("Failed to load snapshot", "error", err, "name", entry.Name())
			continue
		}
		defer f.Close()

		header, _, err := database.ReadSnapshot(f)
		if err != nil {
			app.logger.Error("Failed to read snapshot header", "error", err, "name", entry.Name())
			continue
		}

		resp.Snapshots = append(resp.Snapshots, &abci.Snapshot{
			Height: header.Height,
			Format: uint32(header.Version),
			Chunks: 1,
			Hash:   header.RootHash[:],
		})
	}
	return resp
}

// LoadSnapshotChunk queries the node for the body of a snapshot.
func (app *Accumulator) LoadSnapshotChunk(req abci.RequestLoadSnapshotChunk) abci.ResponseLoadSnapshotChunk {
	if req.Format != core.SnapshotVersion1 || req.Chunk != 0 {
		app.logger.Error("Invalid snapshot request", "height", req.Height, "format", req.Format, "chunk", req.Chunk)
		return abci.ResponseLoadSnapshotChunk{}
	}

	snapDir := config.MakeAbsolute(app.RootDir, app.Accumulate.Snapshots.Directory)
	f, err := os.Open(filepath.Join(snapDir, fmt.Sprintf(core.SnapshotMajorFormat, req.Height)))
	if err != nil {
		app.logger.Error("Failed to load snapshot", "error", err, "height", req.Height, "format", req.Format, "chunk", req.Chunk)
		return abci.ResponseLoadSnapshotChunk{}
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		app.logger.Error("Failed to load snapshot", "error", err, "height", req.Height, "format", req.Format, "chunk", req.Chunk)
		return abci.ResponseLoadSnapshotChunk{}
	}

	return abci.ResponseLoadSnapshotChunk{Chunk: data}
}

// OfferSnapshot offers a snapshot to the node.
func (app *Accumulator) OfferSnapshot(req abci.RequestOfferSnapshot) abci.ResponseOfferSnapshot {
	if req.Snapshot == nil {
		return abci.ResponseOfferSnapshot{Result: abci.ResponseOfferSnapshot_REJECT}
	}
	if req.Snapshot.Format != core.SnapshotVersion1 {
		return abci.ResponseOfferSnapshot{Result: abci.ResponseOfferSnapshot_REJECT_FORMAT}
	}
	if req.Snapshot.Chunks != 1 {
		return abci.ResponseOfferSnapshot{Result: abci.ResponseOfferSnapshot_REJECT}
	}

	return abci.ResponseOfferSnapshot{Result: abci.ResponseOfferSnapshot_ACCEPT}
}

// ApplySnapshotChunk applies a snapshot to the node.
func (app *Accumulator) ApplySnapshotChunk(req abci.RequestApplySnapshotChunk) abci.ResponseApplySnapshotChunk {
	if req.Index != 0 {
		return abci.ResponseApplySnapshotChunk{Result: abci.ResponseApplySnapshotChunk_REJECT_SNAPSHOT}
	}

	rd := bytes.NewReader(req.Chunk)
	batch := app.DB.Begin(true)
	defer batch.Discard()
	err := batch.RestoreSnapshot(rd, &app.Accumulate.Describe)
	if err != nil {
		app.logger.Error("Failed to restore snapshot", "error", err)
		return abci.ResponseApplySnapshotChunk{Result: abci.ResponseApplySnapshotChunk_ABORT}
	}

	err = batch.Commit()
	if err != nil {
		panic(fmt.Errorf("failed to commit snapshot: %w", err))
	}

	return abci.ResponseApplySnapshotChunk{Result: abci.ResponseApplySnapshotChunk_ACCEPT}
}
