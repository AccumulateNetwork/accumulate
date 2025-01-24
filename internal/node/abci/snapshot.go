// Copyright 2025 The Accumulate Authors
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
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/exp/torrent"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	sv1 "gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	sv2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ListSnapshots returns a list of snapshot metadata objects.
//
// This is one of four ABCI functions we have to implement for
// Tendermint/CometBFT.
func (app *Accumulator) ListSnapshots(ctx context.Context, req *abci.RequestListSnapshots) (*abci.ResponseListSnapshots, error) {
	info, err := ListSnapshots(ctx, config.MakeAbsolute(app.RootDir, app.Snapshots.Directory))
	if err != nil {
		return nil, err
	}

	resp := new(abci.ResponseListSnapshots)
	resp.Snapshots = make([]*abci.Snapshot, 0, len(info))
	for _, info := range info {
		md, hash, err := info.FileMetadata()
		if err != nil {
			return nil, err
		}

		b, err := md.MarshalBinary()
		if err != nil {
			return nil, err
		}

		resp.Snapshots = append(resp.Snapshots, &abci.Snapshot{
			Height:   info.Height(),
			Format:   uint32(info.Version()),
			Chunks:   uint32(len(md.Chunks)),
			Hash:     hash[:],
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
	snapDir := config.MakeAbsolute(app.RootDir, app.Snapshots.Directory)
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

	err = sv1.FullRestore(app.Database, buf, app.logger, config.NetworkUrl{
		URL: protocol.PartitionUrl(app.Partition),
	})
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
	file string
	v1   *sv1.Header
	v2   *sv2.Header

	cache struct {
		fileHash [32]byte
		fileMd   *torrent.FileMetadata
	}
}

func (s *snapshotInfo) FileMetadata() (*torrent.FileMetadata, [32]byte, error) {
	if s.cache.fileMd != nil {
		return s.cache.fileMd, s.cache.fileHash, nil
	}

	f, err := os.Open(s.file)
	if err != nil {
		return nil, [32]byte{}, err
	}
	defer f.Close()

	s.cache.fileMd.Chunks, s.cache.fileHash, err = torrent.ChunksBySize(f, chunkSize)
	if err != nil {
		return nil, [32]byte{}, err
	}

	return s.cache.fileMd, s.cache.fileHash, nil
}

func (s *snapshotInfo) Version() uint64 {
	switch {
	case s.v1 != nil:
		return 1
	case s.v2 != nil:
		return 2
	default:
		panic("inconsistent application state")
	}
}

func (s *snapshotInfo) Height() uint64 {
	switch {
	case s.v1 != nil:
		return s.v1.Height
	case s.v2 != nil:
		return s.v2.SystemLedger.Index
	default:
		panic("inconsistent application state")
	}
}

func (s *snapshotInfo) Timestamp() time.Time {
	switch {
	case s.v1 != nil:
		return s.v1.Timestamp
	case s.v2 != nil:
		return s.v2.SystemLedger.Timestamp
	default:
		panic("inconsistent application state")
	}
}

func (s *snapshotInfo) Open() (*os.File, error) {
	return os.Open(s.file)
}

const chunkSize = 10 << 20

// ListSnapshots finds snapshots in the given directory and reads metadata from
// each.
func ListSnapshots(ctx context.Context, dir string) ([]*snapshotInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load snapshot: %w", err)
	}

	snapshots := make([]*snapshotInfo, 0, len(entries))
	for _, entry := range entries {
		select {
		default:
		case <-ctx.Done():
			return nil, ctx.Err()
		}

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

		// Close if the context is canceled to abort any in-progress reads
		go func() { <-ctx.Done(); f.Close() }()

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

		snapshots = append(snapshots, info)
	}
	return snapshots, nil
}

func snapshotInfoV1(f *os.File) (*snapshotInfo, error) {
	header, _, err := sv1.Open(f)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open snapshot %s: %w", f.Name(), err)
	}

	return &snapshotInfo{file: f.Name(), v1: header}, nil
}

func snapshotInfoV2(f *os.File) (*snapshotInfo, error) {
	s, err := sv2.Open(f)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open snapshot %s: %w", f.Name(), err)
	}

	return &snapshotInfo{file: f.Name(), v2: s.Header}, nil
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

func fileHash(f *os.File) ([32]byte, error) {
	var buf [128 * 1024 * 1024]byte
	hasher := sha256.New()
	var N int
	for {
		n, err := f.Read(buf[:])
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return [32]byte{}, nil
		}

		hasher.Write(buf[:n])
		N += n
		fmt.Printf("Hash progress: %.3f GB\n", float64(N)/(1<<30))
	}

	defer fmt.Println("Finished hashing")
	return [32]byte(hasher.Sum(nil)), nil
}
