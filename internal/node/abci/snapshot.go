// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package abci

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"

	abci "github.com/cometbft/cometbft/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/exp/torrent"
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
		b, err := info.md.MarshalBinary()
		if err != nil {
			return nil, err
		}

		resp.Snapshots = append(resp.Snapshots, &abci.Snapshot{
			Height:   info.height,
			Format:   uint32(info.version),
			Chunks:   uint32(len(info.md.Chunks)),
			Hash:     info.hash[:],
			Metadata: b,
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

	_, err = f.Seek(int64(req.Chunk)*chunkSize, io.SeekStart)
	if err != nil {
		return nil, err
	}

	var buf [chunkSize]byte
	_, err = io.ReadFull(f, buf[:])
	if err != nil {
		return nil, err
	}

	return &abci.ResponseLoadSnapshotChunk{Chunk: buf[:]}, nil
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

	ok, err := app.snapshots.Start(req)
	if !ok {
		return &abci.ResponseOfferSnapshot{Result: abci.ResponseOfferSnapshot_REJECT}, nil
	}
	if err != nil {
		return nil, err
	}
	return &abci.ResponseOfferSnapshot{Result: abci.ResponseOfferSnapshot_ACCEPT}, nil
}

// ApplySnapshotChunk applies a snapshot to this node. This is called for each
// chunk of the snapshot. The receiver is responsible for assembling the chunks
// and determining when they have all been received.
//
// This is one of four ABCI functions we have to implement for
// Tendermint/CometBFT.
func (app *Accumulator) ApplySnapshotChunk(_ context.Context, req *abci.RequestApplySnapshotChunk) (*abci.ResponseApplySnapshotChunk, error) {
	err := app.snapshots.Apply(int(req.Index), req.Chunk)
	switch {
	case errors.Is(err, torrent.ErrBadHash):
		return &abci.ResponseApplySnapshotChunk{Result: abci.ResponseApplySnapshotChunk_RETRY}, nil
	case err != nil:
		return nil, err
	case !app.snapshots.Done():
		return &abci.ResponseApplySnapshotChunk{Result: abci.ResponseApplySnapshotChunk_ACCEPT}, nil
	}

	// Assemble the snapshot
	bptHash := app.snapshots.request.AppHash
	buf := new(ioutil.Buffer)
	e1 := app.snapshots.WriteTo(buf)
	e2 := app.snapshots.Reset() // Reset regardless of success
	switch {
	case e1 != nil:
		if errors.Is(err, torrent.ErrBadHash) {
			return &abci.ResponseApplySnapshotChunk{Result: abci.ResponseApplySnapshotChunk_RETRY_SNAPSHOT}, nil
		}
		return nil, e1
	case e2 != nil:
		return nil, e2
	}

	err = snapshot.FullRestore(app.Database, buf, app.logger, app.Accumulate.PartitionUrl())
	if err != nil {
		return nil, err
	}
	batch := app.Database.Begin(false)
	defer batch.Discard()
	root, err := batch.GetBptRootHash()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(root[:], bptHash) {
		// TODO Can we reset the database?
		return &abci.ResponseApplySnapshotChunk{Result: abci.ResponseApplySnapshotChunk_REJECT_SNAPSHOT}, nil
	}

	return &abci.ResponseApplySnapshotChunk{Result: abci.ResponseApplySnapshotChunk_ACCEPT}, nil
}

type snapshotInfo struct {
	md       torrent.FileMetadata
	version  uint64
	height   uint64
	hash     [32]byte
	rootHash [32]byte
}

const chunkSize = 10 << 20

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
		if err != nil {
			return nil, err
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
		info.md.Chunks, err = torrent.ChunksBySize(f, chunkSize)
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

type snapshotManager struct {
	active  *torrent.DownloadJob
	request *abci.RequestOfferSnapshot
}

func (m *snapshotManager) Start(req *abci.RequestOfferSnapshot) (bool, error) {
	if m.active != nil {
		return false, errors.BadRequest.With("already started")
	}

	md := new(torrent.FileMetadata)
	err := md.UnmarshalBinary(req.Snapshot.Metadata)
	if err != nil {
		return false, nil // Reject
	}

	j, err := torrent.NewDownloadJob(md)
	if err != nil {
		return false, err
	}

	m.active = j
	m.request = req
	return true, nil
}

func (m *snapshotManager) Reset() error {
	if m.active == nil {
		return nil
	}

	err := m.active.Reset()
	m.active = nil
	m.request = nil
	return err
}

func (m *snapshotManager) Apply(index int, chunk []byte) error {
	if m.active == nil {
		return errors.BadRequest.With("not started")
	}

	return m.active.RecordChunk(index, chunk)
}

func (m *snapshotManager) Done() bool {
	if m.active == nil {
		return false
	}
	return m.active.Done()
}

func (m *snapshotManager) WriteTo(f io.ReadWriteSeeker) error {
	if m.active == nil {
		return errors.BadRequest.With("not started")
	}

	// Write
	_, err := f.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}
	err = m.active.WriteTo(f)
	if err != nil {
		return err
	}

	// Verify
	_, err = f.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(h, f)
	if err != nil {
		return err
	}
	if !bytes.Equal(h.Sum(nil), m.request.Snapshot.Hash) {
		return torrent.ErrBadHash
	}
	return nil
}
