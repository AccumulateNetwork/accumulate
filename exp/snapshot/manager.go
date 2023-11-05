// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrBadHash = errors.New("hash does not match")

type Manager struct {
	opts ManagerOptions

	snapshot *SnapshotMetadata
	dir      string
	files    []*os.File
}

type ManagerOptions struct {
}

func NewManager(opts ManagerOptions) *Manager {
	return &Manager{opts: opts}
}

func (m *Manager) Start(snapshot *SnapshotMetadata) error {
	if m.snapshot != nil {
		return errors.New("already started")
	}

	dir, err := os.MkdirTemp("", "accumulate-snapshot-*")
	if err != nil {
		return err
	}

	m.snapshot = snapshot
	m.dir = dir
	m.files = make([]*os.File, len(snapshot.Chunks))
	return nil
}

func (m *Manager) Reset() error {
	var errs []error
	for _, f := range m.files {
		err := f.Close()
		if err != nil {
			errs = append(errs, err)
		}
	}
	m.snapshot = nil
	m.dir = ""
	m.files = nil

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

func (m *Manager) Apply(index int, chunk []byte) error {
	if m.snapshot == nil {
		return errors.New("not started")
	}

	if index < 0 || index >= len(m.snapshot.Chunks) {
		return errors.New("index out of range")
	}
	if m.files[index] != nil {
		return errors.New("already received chunk")
	}

	c := m.snapshot.Chunks[index]
	if sha256.Sum256(chunk) != c.Hash {
		return ErrBadHash
	}

	f, err := os.Create(filepath.Join(m.dir, fmt.Sprintf("chunk-%d", index)))
	if err != nil {
		return err
	}

	_, err = f.Write(chunk)
	if err != nil {
		return err
	}

	m.files[index] = f
	return nil
}

func (m *Manager) Done() bool {
	for _, f := range m.files {
		if f == nil {
			return false
		}
	}
	return true
}

func (m *Manager) WriteTo(wr io.WriteSeeker) error {
	if m.snapshot == nil {
		return errors.New("not started")
	}

	if !m.Done() {
		return errors.New("missing chunks")
	}

	for i, f := range m.files {
		_, err := f.Seek(0, io.SeekStart)
		if err != nil {
			return err
		}

		c := m.snapshot.Chunks[i]
		_, err = wr.Seek(int64(c.Offset), io.SeekStart)
		if err != nil {
			return err
		}

		_, err = io.Copy(wr, f)
		if err != nil {
			return err
		}
	}
	return nil
}
