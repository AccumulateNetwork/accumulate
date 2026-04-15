// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/exp/torrent"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	sv2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// ChunkSize is the size of snapshot chunks for torrent-style transfer.
const ChunkSize = 10 << 20

// SnapshotInfo holds metadata about a snapshot file on disk.
type SnapshotInfo struct {
	File     string
	FileHash [32]byte
	FileMd   torrent.FileMetadata
	V1       *Header
	V2       *sv2.Header
}

// Version returns the snapshot format version.
func (s *SnapshotInfo) Version() uint64 {
	switch {
	case s.V1 != nil:
		return 1
	case s.V2 != nil:
		return 2
	default:
		panic("inconsistent application state")
	}
}

// Height returns the block height of the snapshot.
func (s *SnapshotInfo) Height() uint64 {
	switch {
	case s.V1 != nil:
		return s.V1.Height
	case s.V2 != nil:
		return s.V2.SystemLedger.Index
	default:
		panic("inconsistent application state")
	}
}

// Timestamp returns the timestamp of the snapshot.
func (s *SnapshotInfo) Timestamp() time.Time {
	switch {
	case s.V1 != nil:
		return s.V1.Timestamp
	case s.V2 != nil:
		return s.V2.SystemLedger.Timestamp
	default:
		panic("inconsistent application state")
	}
}

// Open opens the snapshot file for reading.
func (s *SnapshotInfo) Open() (*os.File, error) {
	return os.Open(s.File)
}

// ListSnapshots finds snapshots in the given directory and reads metadata from
// each.
func ListSnapshots(dir string) ([]*SnapshotInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load snapshot: %w", err)
	}

	snapshots := make([]*SnapshotInfo, 0, len(entries))
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
		var info *SnapshotInfo
		switch ver {
		case Version1:
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
		info.FileHash = *(*[32]byte)(hasher.Sum(nil))

		// Chunk the file
		_, err = f.Seek(0, io.SeekStart)
		if err != nil {
			return nil, err
		}
		info.FileMd.Chunks, err = torrent.ChunksBySize(f, ChunkSize)
		if err != nil {
			return nil, err
		}

		snapshots = append(snapshots, info)
	}
	return snapshots, nil
}

func snapshotInfoV1(f *os.File) (*SnapshotInfo, error) {
	header, _, err := Open(f)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open snapshot %s: %w", f.Name(), err)
	}

	return &SnapshotInfo{File: f.Name(), V1: header}, nil
}

func snapshotInfoV2(f *os.File) (*SnapshotInfo, error) {
	s, err := sv2.Open(f)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open snapshot %s: %w", f.Name(), err)
	}

	return &SnapshotInfo{File: f.Name(), V2: s.Header}, nil
}
