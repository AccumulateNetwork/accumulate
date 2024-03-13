// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package torrent

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type DownloadJob struct {
	mu     *sync.Mutex
	file   *FileMetadata
	cache  string
	chunks []*os.File
}

func NewDownloadJob(file *FileMetadata) (*DownloadJob, error) {
	dir, err := os.MkdirTemp("", "accumulate-snapshot-*")
	if err != nil {
		return nil, err
	}

	j := new(DownloadJob)
	j.mu = new(sync.Mutex)
	j.file = file
	j.cache = dir
	j.chunks = make([]*os.File, len(file.Chunks))
	return j, nil
}

// Reset aborts the active job, if there is one, and removes all the temporary
// files.
func (j *DownloadJob) Reset() error {
	var errs []error
	for _, f := range j.chunks {
		err := f.Close()
		if err != nil {
			errs = append(errs, err)
		}
	}

	if j.cache != "" {
		err := os.RemoveAll(j.cache)
		if err != nil {
			errs = append(errs, err)
		}
	}

	*j = DownloadJob{}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	}

	var s []string
	for _, e := range errs {
		s = append(s, e.Error())
	}
	return errors.New(strings.Join(s, "; "))
}

// RecordChunk verifies and records the given chunk.
func (j *DownloadJob) RecordChunk(index int, data []byte) error {
	c := j.file.Chunks[index]
	if sha256.Sum256(data) != c.Hash {
		return ErrBadHash
	}

	if index < 0 || index >= len(j.file.Chunks) {
		return errors.New("index out of range")
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	if j.chunks[index] != nil {
		return errors.New("already received chunk")
	}

	f, err := os.Create(filepath.Join(j.cache, fmt.Sprintf("chunk-%d", index)))
	if err != nil {
		return err
	}

	_, err = f.Write(data)
	if err != nil {
		return err
	}

	j.chunks[index] = f
	return nil
}

// Done returns true if every chunk has been received.
func (j *DownloadJob) Done() bool {
	j.mu.Lock()
	defer j.mu.Unlock()

	for _, f := range j.chunks {
		if f == nil {
			return false
		}
	}
	return true
}

// WriteTo writes the chunks to a file.
func (j *DownloadJob) WriteTo(file io.WriteSeeker) error {
	if !j.Done() {
		return errors.New("missing chunks")
	}

	for i, f := range j.chunks {
		_, err := f.Seek(0, io.SeekStart)
		if err != nil {
			return err
		}

		c := j.file.Chunks[i]
		_, err = file.Seek(int64(c.Offset), io.SeekStart)
		if err != nil {
			return err
		}

		_, err = io.Copy(file, f)
		if err != nil {
			return err
		}
	}
	return nil
}
