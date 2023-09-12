// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

type Database struct {
	path      string
	files     []*os.File
	blocks    map[uint64]blockLocation
	records   map[[32]byte]recordLocation
	next      uint64
	fileLimit uint64
}

type blockLocation struct {
	file   uint
	offset int64
}

type recordLocation struct {
	file   uint
	block  uint64
	offset int64
}

func Open(path string) (_ *Database, err error) {
	// List all the entries
	entries, err := os.ReadDir(path)
	switch {
	case err == nil:
		// Directory exists

	case errors.Is(err, fs.ErrNotExist):
		// Create the directory
		err = os.Mkdir(path, 0700)
		if err != nil {
			return nil, err
		}

	default:
		// Some other error
		return nil, err
	}

	db := new(Database)
	db.path = path
	db.fileLimit = 1 << 30
	defer func() {
		if err != nil {
			_ = db.Close()
		}
	}()

	// Open all the files
	db.files = make([]*os.File, 0, len(entries))
	for _, e := range entries {
		f, err := os.OpenFile(filepath.Join(path, e.Name()), os.O_RDWR, 0)
		if err != nil {
			return nil, err
		}
		db.files = append(db.files, f)
	}

	// Build the block index
	db.blocks = map[uint64]blockLocation{}
	db.records = map[[32]byte]recordLocation{}
	for fileNo, f := range db.files {
		var offset int64
		var block *uint64
		for {
			e, n, err := readEntry(f)
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return nil, fmt.Errorf("reading entries from %v: %w", f.Name(), err)
			}

			pos := offset
			offset += int64(n)

			switch e := e.(type) {
			case *startBlockEntry:
				if block != nil {
					return nil, fmt.Errorf("%v is corrupted", f.Name())
				}
				if _, ok := db.blocks[e.ID]; ok {
					return nil, fmt.Errorf("duplicate block %d", e.ID)
				}
				db.blocks[e.ID] = blockLocation{file: uint(fileNo), offset: pos}
				block = &e.ID

				if db.next < e.ID {
					db.next = e.ID
				}

			case *endBlockEntry:
				if block == nil {
					return nil, fmt.Errorf("%v is corrupted", f.Name())
				}
				block = nil

			case *recordEntry:
				if block == nil {
					return nil, fmt.Errorf("%v is corrupted", f.Name())
				}

				db.records[e.Key.Hash()] = recordLocation{file: uint(fileNo), block: *block, offset: pos}
			}
		}
	}

	// Ensure there's at least one file
	if len(db.files) == 0 {
		_, err := db.newFile()
		if err != nil {
			return nil, err
		}
	}

	return db, nil
}

func (db *Database) newFile() (*os.File, error) {
	name := fmt.Sprintf("%v.blocks", time.Now().UTC().Unix())
	f, err := os.OpenFile(filepath.Join(db.path, name), os.O_RDWR|os.O_EXCL|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	db.files = append(db.files, f)
	return f, nil
}

func (db *Database) Close() error {
	var errs []error
	for _, f := range db.files {
		err := f.Close()
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs[0]
}

// Begin begins a change set.
func (d *Database) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	// TODO Transactional reading
	get := func(key *record.Key) ([]byte, error) {
		loc, ok := d.records[key.Hash()]
		if !ok {
			return nil, errors.NotFound.WithFormat("%v not found", key)
		}
		if loc.file >= uint(len(d.files)) {
			return nil, errors.InternalError.WithFormat("record is corrupted")
		}

		f := d.files[loc.file]
		e, _, err := readEntryAt(f, loc.offset)
		if err != nil {
			return nil, err
		}
		r, ok := e.(*recordEntry)
		if !ok {
			return nil, errors.InternalError.With("entry is not a record")
		}
		if r.Deleted {
			return nil, errors.NotFound.WithFormat("%v not found", key)
		}
		return r.Value, nil
	}

	commit := func(entries map[[32]byte]memory.Entry) error {
		// Seek to the end of the newest file
		fileNo := len(d.files) - 1
		f := d.files[fileNo]
		offset, err := f.Seek(0, io.SeekEnd)
		if err != nil {
			return err
		}

		var block uint64
		var haveBlock bool
		for kh, e := range entries {
			// Time for a new file?
			if offset >= int64(d.fileLimit) {
				// Close the block
				haveBlock = false
				_, err = writeEntry(f, &endBlockEntry{})
				if err != nil {
					return err
				}

				// Open a new file
				offset = 0
				fileNo++
				f, err = d.newFile()
				if err != nil {
					return err
				}
			}

			// Time for a new block?
			if !haveBlock {
				// Open the block
				d.next++
				b := new(startBlockEntry)
				b.ID = d.next
				b.Parent = block
				block = b.ID
				haveBlock = true

				n, err := writeEntry(f, b)
				if err != nil {
					return err
				}
				offset += int64(n)
			}

			// Write the entry
			d.records[kh] = recordLocation{file: uint(fileNo), block: block, offset: offset}
			n, err := writeEntry(f, &recordEntry{Key: e.Key, Deleted: e.Delete, Value: e.Value})
			if err != nil {
				return err
			}
			offset += int64(n)
		}

		// Close the block
		if haveBlock {
			_, err = writeEntry(f, &endBlockEntry{})
			if err != nil {
				return err
			}
		}

		return nil
	}

	return memory.NewChangeSet(prefix, get, commit)
}
