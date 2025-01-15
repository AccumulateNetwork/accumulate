// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type recordFileSet struct {
	config *config
	files  []*recordFile
}

type entryAndData struct {
	recordEntry
	Value []byte
}

func openRecordFileSet(cfg *config) (_ *recordFileSet, err error) {
	s := new(recordFileSet)
	s.config = cfg
	defer closeIfError(&err, s)

	// List all the entries
	entries, err := os.ReadDir(cfg.path)
	switch {
	case err == nil,
		errors.Is(err, fs.ErrNotExist):
		// Directory exists, or doesn't

	default:
		// Some other error
		return nil, err
	}

	// Open each file
	for _, e := range entries {
		if cfg.filterFn != nil && !cfg.filterFn(e.Name()) {
			continue
		}

		// Skip files that aren't block files
		_, err := cfg.nameFmt.Parse(e.Name())
		if err != nil {
			continue
		}

		f, err := openRecordFile(filepath.Join(cfg.path, e.Name()))
		if err != nil {
			return nil, err
		}

		s.files = append(s.files, f)
	}
	return s, nil
}

func (s *recordFileSet) Close() error {
	if s == nil {
		return nil
	}
	var errs []error
	for _, f := range s.files {
		errs = append(errs, f.Close())
	}
	return errors.Join(errs...)
}

func (s *recordFileSet) Commit(view *databaseView, entries []*entryAndData) error {
	rw := new(recordWriter)
	rw.encBuf = poolBuffer.Get()
	rw.enc = poolEncoder.Get(rw.encBuf)
	defer poolBuffer.Put(rw.encBuf)
	defer poolEncoder.Put(rw.enc)

	block := new(blockID)
	block.ID = s.config.nextBlock.Add(1)

	write := func(f *recordFile) (int, error) {
		// Check the limit
		if f.used.Load() > s.config.fileLimit {
			return 0, nil
		}

		// Write the start of block marker
		h := f.file.Acquire()
		defer h.Release()
		fw := &recordFileWriter{&f.used, h}
		_, err := rw.Write(fw, &startBlockEntry{blockID: *block}, nil)
		if err != nil {
			return 0, err
		}

		var n int
		for ; n < len(entries) && f.used.Load() < s.config.fileLimit; n++ {
			e := entries[n]

			// Write the entry
			loc, err := rw.Write(fw, &e.recordEntry, e.Value)
			if err != nil {
				return 0, err
			}

			// Update the index
			loc.RecordLen = e.Length
			loc.Block = block.Copy()
			view.records.Put(e.KeyHash, loc)
		}

		// Close the block
		_, err = rw.Write(fw, &endBlockEntry{}, nil)
		if err != nil {
			return 0, err
		}

		// Update the header
		err = fw.Close()
		if err != nil {
			return 0, err
		}

		return n, nil
	}

	var file *recordFile
	for len(entries) > 0 {
		// If it's the first time and there are existing files, get the last
		// file. Otherwise make a new file.
		var err error
		if file == nil && len(s.files) > 0 {
			file = s.files[len(s.files)-1]

		} else {
			file, err = s.new(block)
			if err != nil {
				return err
			}
		}

		n, err := write(file)
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}

		view.blocks.Put(*block, len(s.files)-1)
		entries = entries[n:]
		block.Part++
	}

	return nil
}

func (s *recordFileSet) new(block *blockID) (*recordFile, error) {
	// Ensure the directory exists
	err := os.Mkdir(s.config.path, 0700)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, err
	}

	// Create a new file
	name := s.config.nameFmt.Format(block)
	if name == "" {
		return nil, fmt.Errorf("invalid file name: empty")
	} else if filepath.Base(name) != name {
		return nil, fmt.Errorf("invalid file name: %q contains a slash or is empty", name)
	}

	f, err := newRecordFile(filepath.Join(s.config.path, name))
	if err != nil {
		return nil, err
	}

	s.files = append(s.files, f)
	return f, nil
}
