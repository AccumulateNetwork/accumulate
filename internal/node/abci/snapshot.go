// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package abci

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	abci "github.com/tendermint/tendermint/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	_ "gitlab.com/accumulatenetwork/accumulate/internal/database/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
)

// ListSnapshots queries the node for available snapshots.
func ListSnapshots(cfg *config.Config) ([]*snapshot.Header, error) {
	snapDir := config.MakeAbsolute(cfg.RootDir, cfg.Accumulate.Snapshots.Directory)
	entries, err := os.ReadDir(snapDir)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load snapshot: %w", err)
	}

	snapshots := make([]*snapshot.Header, 0, len(entries))
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
			return nil, errors.Format(errors.StatusUnknownError, "load snapshot %s: %w", entry.Name(), err)
		}
		defer f.Close()

		header, _, err := snapshot.Open(f)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "open snapshot %s: %w", entry.Name(), err)
		}

		snapshots = append(snapshots, header)
	}
	return snapshots, nil
}

func (app *Accumulator) ListSnapshots(req abci.RequestListSnapshots) abci.ResponseListSnapshots {
	entries, err := ListSnapshots(app.Config)
	if err != nil {
		app.logger.Error("Failed to list snapshots", "error", err)
		return abci.ResponseListSnapshots{}
	}

	var resp abci.ResponseListSnapshots
	resp.Snapshots = make([]*abci.Snapshot, 0, len(entries))
	for _, header := range entries {
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
	if req.Format != snapshot.Version1 || req.Chunk != 0 {
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
	if req.Snapshot.Format != snapshot.Version1 {
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
	err := app.Executor.RestoreSnapshot(app.DB, rd)
	if err != nil {
		app.logger.Error("Failed to restore snapshot", "error", err)
		return abci.ResponseApplySnapshotChunk{Result: abci.ResponseApplySnapshotChunk_ABORT}
	}

	app.ready = true
	return abci.ResponseApplySnapshotChunk{Result: abci.ResponseApplySnapshotChunk_ACCEPT}
}
