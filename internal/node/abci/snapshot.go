// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package abci

import (
	"context"
	"os"
	"path/filepath"

	abci "github.com/cometbft/cometbft/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// ListSnapshots queries the node for available snapshots.
func ListSnapshots(cfg *config.Config) ([]*snapshot.Header, error) {
	snapDir := config.MakeAbsolute(cfg.RootDir, cfg.Accumulate.Snapshots.Directory)
	entries, err := os.ReadDir(snapDir)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load snapshot: %w", err)
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
			return nil, errors.UnknownError.WithFormat("load snapshot %s: %w", entry.Name(), err)
		}
		defer f.Close()

		header, _, err := snapshot.Open(f)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("open snapshot %s: %w", entry.Name(), err)
		}

		snapshots = append(snapshots, header)
	}
	return snapshots, nil
}

func (app *Accumulator) ListSnapshots(_ context.Context, req *abci.RequestListSnapshots) (*abci.ResponseListSnapshots, error) {
	// https://gitlab.com/accumulatenetwork/accumulate/-/issues/3356
	return &abci.ResponseListSnapshots{}, nil

	// entries, err := ListSnapshots(app.Config)
	// if err != nil {
	// 	app.logger.Error("Failed to list snapshots", "error", err)
	// 	return abci.ResponseListSnapshots{}
	// }

	// var resp abci.ResponseListSnapshots
	// resp.Snapshots = make([]*abci.Snapshot, 0, len(entries))
	// for _, header := range entries {
	// 	resp.Snapshots = append(resp.Snapshots, &abci.Snapshot{
	// 		Height: header.Height,
	// 		Format: uint32(header.Version),
	// 		Chunks: 1,
	// 		Hash:   header.RootHash[:],
	// 	})
	// }
	// return resp
}

// LoadSnapshotChunk queries the node for the body of a snapshot.
func (app *Accumulator) LoadSnapshotChunk(_ context.Context, req *abci.RequestLoadSnapshotChunk) (*abci.ResponseLoadSnapshotChunk, error) {
	// https://gitlab.com/accumulatenetwork/accumulate/-/issues/3356
	return &abci.ResponseLoadSnapshotChunk{}, nil

	// if req.Format != snapshot.Version1 || req.Chunk != 0 {
	// 	app.logger.Error("Invalid snapshot request", "height", req.Height, "format", req.Format, "chunk", req.Chunk)
	// 	return abci.ResponseLoadSnapshotChunk{}
	// }

	// snapDir := config.MakeAbsolute(app.RootDir, app.Accumulate.Snapshots.Directory)
	// f, err := os.Open(filepath.Join(snapDir, fmt.Sprintf(core.SnapshotMajorFormat, req.Height)))
	// if err != nil {
	// 	app.logger.Error("Failed to load snapshot", "error", err, "height", req.Height, "format", req.Format, "chunk", req.Chunk)
	// 	return abci.ResponseLoadSnapshotChunk{}
	// }
	// defer f.Close()

	// data, err := io.ReadAll(f)
	// if err != nil {
	// 	app.logger.Error("Failed to load snapshot", "error", err, "height", req.Height, "format", req.Format, "chunk", req.Chunk)
	// 	return abci.ResponseLoadSnapshotChunk{}
	// }

	// return abci.ResponseLoadSnapshotChunk{Chunk: data}
}

// OfferSnapshot offers a snapshot to the node.
func (app *Accumulator) OfferSnapshot(_ context.Context, req *abci.RequestOfferSnapshot) (*abci.ResponseOfferSnapshot, error) {
	// https://gitlab.com/accumulatenetwork/accumulate/-/issues/3356
	return &abci.ResponseOfferSnapshot{}, nil

	// if req.Snapshot == nil {
	// 	return abci.ResponseOfferSnapshot{Result: abci.ResponseOfferSnapshot_REJECT}
	// }
	// if req.Snapshot.Format != snapshot.Version1 {
	// 	return abci.ResponseOfferSnapshot{Result: abci.ResponseOfferSnapshot_REJECT_FORMAT}
	// }
	// if req.Snapshot.Chunks != 1 {
	// 	return abci.ResponseOfferSnapshot{Result: abci.ResponseOfferSnapshot_REJECT}
	// }

	// return abci.ResponseOfferSnapshot{Result: abci.ResponseOfferSnapshot_ACCEPT}
}

// ApplySnapshotChunk applies a snapshot to the node.
func (app *Accumulator) ApplySnapshotChunk(_ context.Context, req *abci.RequestApplySnapshotChunk) (*abci.ResponseApplySnapshotChunk, error) {
	// https://gitlab.com/accumulatenetwork/accumulate/-/issues/3356
	return &abci.ResponseApplySnapshotChunk{}, nil

	// if req.Index != 0 {
	// 	return abci.ResponseApplySnapshotChunk{Result: abci.ResponseApplySnapshotChunk_REJECT_SNAPSHOT}
	// }

	// rd := bytes.NewReader(req.Chunk)
	// err := snapshot.FullRestore(app.Database, rd, app.logger, &app.Accumulate.Describe)
	// if err != nil {
	// 	app.logger.Error("Failed to restore snapshot", "error", err)
	// 	return abci.ResponseApplySnapshotChunk{Result: abci.ResponseApplySnapshotChunk_ABORT}
	// }

	// app.ready = true
	// return abci.ResponseApplySnapshotChunk{Result: abci.ResponseApplySnapshotChunk_ACCEPT}
}
