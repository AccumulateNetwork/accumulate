// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type blockFileSet struct {
	config *config
	path   string
	files  []*blockFile
}

func (db *Database) openFileSet(dir string) error {
	db.files.path = dir
	db.files.config = &db.config

	// List all the entries
	entries, err := os.ReadDir(dir)
	switch {
	case err == nil,
		errors.Is(err, fs.ErrNotExist):
		// Directory exists, or doesn't

	default:
		// Some other error
		return err
	}

	// Open each file
	db.files.files = make([]*blockFile, 0, len(entries))
	for _, e := range entries {
		if db.filterFn != nil && !db.filterFn(e.Name()) {
			continue
		}

		// Skip files that aren't block files
		_, err := db.nameFmt.Parse(e.Name())
		if err != nil {
			continue
		}

		f, err := db.openFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return err
		}

		db.files.files = append(db.files.files, f)
	}

	// Determine the first block of each file
	first := make(map[*blockFile]*blockID, len(db.files.files))
	for _, f := range db.files.files {
		it := f.entries(nil)
		it.Range(func(_ int, item entryPos) bool {
			s, ok := item.entry.(*startBlockEntry)
			if ok {
				first[f] = &s.blockID
			} else {
				it.err = fmt.Errorf("%v is corrupted: first record is %T, want %T", f.file.Name(), item.entry, (*startBlockEntry)(nil))
			}
			return false
		})
		if it.err != nil {
			return it.err
		}
	}

	// Sort the files by their first block
	slices.SortFunc(db.files.files, func(a, b *blockFile) int {
		return first[a].Compare(first[b])
	})

	return nil
}

func (b *blockFileSet) new(block *blockID) (*blockFile, error) {
	// Ensure the directory exists
	err := os.Mkdir(b.path, 0700)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, err
	}

	// Create a new file
	name := b.config.nameFmt.Format(block)
	if name == "" {
		return nil, fmt.Errorf("invalid block file name: empty")
	} else if filepath.Base(name) != name {
		return nil, fmt.Errorf("invalid block file name: %q contains a slash or is empty", name)
	}

	f, err := newFile(b.config, filepath.Join(b.path, name))
	if err != nil {
		return nil, err
	}

	b.files = append(b.files, f)
	return f, nil
}

func (b *blockFileSet) Close() error {
	var errs []error
	for _, f := range b.files {
		errs = append(errs, f.Close())
	}
	return errors.Join(errs...)
}
