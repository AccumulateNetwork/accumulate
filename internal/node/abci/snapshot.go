// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package abci

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"

	abci "github.com/cometbft/cometbft/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	sv1 "gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	sv2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// ListSnapshots queries the node for available snapshots.
func ListSnapshots(cfg *config.Config) ([]*snapshot.Header, error) {
	snapDir := config.MakeAbsolute(cfg.RootDir, cfg.Accumulate.Snapshots.Directory)
	info, err := listSnapshots(snapDir)
	if err != nil {
		return nil, err
	}

	snapshots := make([]*snapshot.Header, 0, len(info))
	for _, info := range info {
		snapshots = append(snapshots, &sv1.Header{
			Version:  info.version,
			Height:   info.height,
			RootHash: info.rootHash,
		})
	}
	return snapshots, nil
}

// ListSnapshots returns a list of snapshot metadata objects.
//
// This is one of four ABCI functions we have to implement for
// Tendermint/CometBFT.
func (app *Accumulator) ListSnapshots(_ context.Context, req *abci.RequestListSnapshots) (*abci.ResponseListSnapshots, error) {
	info, err := listSnapshots(config.MakeAbsolute(app.Config.RootDir, app.Config.Accumulate.Snapshots.Directory))
	if err != nil {
		return nil, err
	}

	resp := new(abci.ResponseListSnapshots)
	resp.Snapshots = make([]*abci.Snapshot, 0, len(info))
	for _, info := range info {
		metadata := make([]byte, 32*len(info.chunks))
		for i, chunk := range info.chunks {
			copy(metadata[i*32:], chunk[:])
		}

		resp.Snapshots = append(resp.Snapshots, &abci.Snapshot{
			Height:   info.height,
			Format:   uint32(info.version),
			Chunks:   uint32(len(info.chunks)),
			Hash:     info.hash[:],
			Metadata: metadata,
		})
	}
	return resp, nil
}

// LoadSnapshotChunk queries the node for the body of a snapshot, to be offered
// to another node.
//
// This is one of four ABCI functions we have to implement for
// Tendermint/CometBFT.
func (app *Accumulator) LoadSnapshotChunk(_ context.Context, req *abci.RequestLoadSnapshotChunk) (*abci.ResponseLoadSnapshotChunk, error) {
	snapDir := config.MakeAbsolute(app.RootDir, app.Accumulate.Snapshots.Directory)
	f, err := os.Open(filepath.Join(snapDir, fmt.Sprintf(core.SnapshotMajorFormat, req.Height)))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	chunk, err := app.getSnapshotChunk(f, req.Chunk)
	if err != nil {
		return nil, err
	}

	return &abci.ResponseLoadSnapshotChunk{Chunk: chunk}, nil
}

// OfferSnapshot offers a snapshot to this node. This initiates the snapshot
// sync process on the receiver.
//
// This is one of four ABCI functions we have to implement for
// Tendermint/CometBFT.
func (app *Accumulator) OfferSnapshot(_ context.Context, req *abci.RequestOfferSnapshot) (*abci.ResponseOfferSnapshot, error) {
	if req.Snapshot == nil {
		return &abci.ResponseOfferSnapshot{Result: abci.ResponseOfferSnapshot_REJECT}, nil
	}
	if req.Snapshot.Format != sv2.Version2 {
		return &abci.ResponseOfferSnapshot{Result: abci.ResponseOfferSnapshot_REJECT_FORMAT}, nil
	}

	r, err := app.startSnapshotSync(req.Snapshot, req.AppHash)
	if err != nil {
		return nil, err
	}
	return &abci.ResponseOfferSnapshot{Result: r}, nil
}

// ApplySnapshotChunk applies a snapshot to this node. This is called for each
// chunk of the snapshot. The receiver is responsible for assembling the chunks
// and determining when they have all been received.
//
// This is one of four ABCI functions we have to implement for
// Tendermint/CometBFT.
func (app *Accumulator) ApplySnapshotChunk(_ context.Context, req *abci.RequestApplySnapshotChunk) (*abci.ResponseApplySnapshotChunk, error) {
	r, err := app.acceptSnapshotChunk(req.Index, req.Chunk)
	if err != nil {
		return nil, err
	}
	return &abci.ResponseApplySnapshotChunk{Result: r}, nil
}

// finishedLoadingSnapshot should be called once the complete snapshot has been
// assembled.
func (app *Accumulator) finishedLoadingSnapshot(assembled ioutil.SectionReader) error {
	return snapshot.FullRestore(app.Database, assembled, app.logger, app.Accumulate.PartitionUrl())
}

type snapshotInfo struct {
	version  uint64
	height   uint64
	hash     [32]byte
	rootHash [32]byte
	chunks   [][32]byte
}

// listSnapshots finds snapshots in the given directory and reads metadata from
// each.
func listSnapshots(dir string) ([]*snapshotInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load snapshot: %w", err)
	}

	snapshots := make([]*snapshotInfo, 0, len(entries))
	for _, entry := range entries {
		// Is it a file?
		if entry.IsDir() {
			continue
		}

		// Does it match the regex?
		if !core.SnapshotMajorRegexp.MatchString(entry.Name()) {
			continue
		}

		// Open it
		filename := filepath.Join(dir, entry.Name())
		f, err := os.Open(filename)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load snapshot %s: %w", entry.Name(), err)
		}
		defer f.Close()

		// Determine the snapshot version and reset the offset
		ver, err := sv2.GetVersion(f)
		if err != nil {
			return nil, err
		}
		_, err = f.Seek(0, io.SeekStart)
		if err != nil {
			return nil, err
		}

		// Read the header
		var info *snapshotInfo
		switch ver {
		case sv1.Version1:
			info, err = snapshotInfoV1(f)
		case sv2.Version2:
			info, err = snapshotInfoV2(f)
		default:
			return nil, errors.InternalError.WithFormat("unsupported snapshot version %d", ver)
		}

		// Hash the file
		_, err = f.Seek(0, io.SeekStart)
		if err != nil {
			return nil, err
		}
		hasher := sha256.New()
		_, err = io.Copy(hasher, f)
		if err != nil {
			return nil, err
		}
		info.hash = *(*[32]byte)(hasher.Sum(nil))

		// Chunk the file
		_, err = f.Seek(0, io.SeekStart)
		if err != nil {
			return nil, err
		}
		err = getSnapshotChunks(f, info)
		if err != nil {
			return nil, err
		}

		snapshots = append(snapshots, info)
	}
	return snapshots, nil
}

func snapshotInfoV1(f *os.File) (*snapshotInfo, error) {
	header, _, err := sv1.Open(f)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open snapshot %s: %w", f.Name(), err)
	}

	return &snapshotInfo{
		version:  header.Version,
		height:   header.Height,
		rootHash: header.RootHash,
	}, nil
}

func snapshotInfoV2(f *os.File) (*snapshotInfo, error) {
	s, err := sv2.Open(f)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open snapshot %s: %w", f.Name(), err)
	}

	return &snapshotInfo{
		version:  s.Header.Version,
		height:   s.Header.SystemLedger.Index,
		rootHash: s.Header.RootHash,
	}, nil
}

// getSnapshotChunks returns the number of chunks that the snapshot will be
// split into for transfer.
func getSnapshotChunks(snapshot *os.File, info *snapshotInfo) error {
	info.chunks = [][32]byte{
		// TODO Set to chunk hashes
	}
	panic("TODO")
}

// getSnapshotChunk returns the given chunk of the snapshot.
func (*Accumulator) getSnapshotChunk(snapshot *os.File, chunk uint32) ([]byte, error) {
	panic("TODO")
}

// startSnapshotSync initializes the snapshot sync state.
func (*Accumulator) startSnapshotSync(snapshot *abci.Snapshot, bptHash []byte) (abci.ResponseOfferSnapshot_Result, error) {
	if len(snapshot.Metadata) != 32*int(snapshot.Chunks) {
		return abci.ResponseOfferSnapshot_REJECT, nil
	}
	chunks := make([][32]byte, snapshot.Chunks)
	for i := range chunks {
		chunks[i] = *(*[32]byte)(snapshot.Metadata[32*i:])
	}

	// Reference:
	//  - Cosmos ABCI: https://github.com/cosmos/cosmos-sdk/blob/167b702732620d35cf3c8e786fe828d4f97e5268/baseapp/abci.go#L276
	//  - Snapshot manager: https://github.com/cosmos/cosmos-sdk/blob/main/store/snapshots/manager.go#L265
	panic("TODO")
}

// acceptSnapshotChunk accepts a chunk of a snapshot. acceptSnapshotChunk must
// call finishedLoadingSnapshot once all the chunks have been received.
func (app *Accumulator) acceptSnapshotChunk(chunk uint32, data []byte) (abci.ResponseApplySnapshotChunk_Result, error) {
	// Reference:
	//  - Cosmos ABCI: https://github.com/cosmos/cosmos-sdk/blob/167b702732620d35cf3c8e786fe828d4f97e5268/baseapp/abci.go#L325
	//  - Snapshot manager: https://github.com/cosmos/cosmos-sdk/blob/main/store/snapshots/manager.go#L401
	// Optional inputs:
	//  - Sender
	// Optional outputs:
	//  - Chunks to refetch
	//  - Senders to reject
	panic("TODO")
}
