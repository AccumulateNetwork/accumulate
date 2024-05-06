// Copyright 2024 The Accumulate Authors
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
	"sync"

	"github.com/edsrzf/mmap-go"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

type Database struct {
	path      string
	commitMu  sync.Mutex
	files     []*blockFile
	records   records
	next      uint64
	fileLimit uint64
	nameFn    func(i int) string
}

type records = vmap[[32]byte, recordLocation]
type recordsView = vmapView[[32]byte, recordLocation]

type blockFile struct {
	file *os.File
	mmap mmap.MMap
}

type blockLocation struct {
	file   uint
	offset int64
}

type recordLocation struct {
	file   uint
	block  uint64
	header int64
	offset int64
	length int64
}

type Option func(*Database)

func WithFileLimit(limit uint64) Option {
	return func(d *Database) {
		d.fileLimit = limit
	}
}

func WithNameFunc(fn func(i int) string) Option {
	return func(d *Database) {
		d.nameFn = fn
	}
}

func Open(path string, options ...Option) (_ *Database, err error) {
	// List all the entries
	entries, err := os.ReadDir(path)
	switch {
	case err == nil,
		errors.Is(err, fs.ErrNotExist):
		// Directory exists, or doesn't

	default:
		// Some other error
		return nil, err
	}

	db := new(Database)
	defer func() {
		if err != nil {
			_ = db.Close()
		}
	}()

	db.path = path
	db.fileLimit = 1 << 30
	db.nameFn = func(i int) string { return fmt.Sprintf("%d.blocks", i) }

	for _, o := range options {
		o(db)
	}

	// Open all the files
	db.files = make([]*blockFile, 0, len(entries))
	for _, e := range entries {
		f, err := os.OpenFile(filepath.Join(path, e.Name()), os.O_RDWR, 0)
		if err != nil {
			return nil, err
		}

		m, err := mmap.Map(f, mmap.RDWR, 0)
		if err != nil {
			return nil, err
		}

		db.files = append(db.files, &blockFile{file: f, mmap: m})
	}

	// Build the block index
	blocks := map[uint64]blockLocation{}
	records := db.records.View()
	for fileNo, f := range db.files {
		var offset int64
		var block *uint64
		for {
			e, n, err := readEntryMmap(f.mmap, offset)
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return nil, fmt.Errorf("reading entries from %v: %w", f.file.Name(), err)
			}

			start := offset
			offset += int64(n)

			switch e := e.(type) {
			case *startBlockEntry:
				if block != nil {
					return nil, fmt.Errorf("%v is corrupted", f.file.Name())
				}
				if _, ok := blocks[e.ID]; ok {
					return nil, fmt.Errorf("duplicate block %d", e.ID)
				}
				blocks[e.ID] = blockLocation{file: uint(fileNo), offset: start}
				block = &e.ID

				if db.next < e.ID {
					db.next = e.ID
				}

			case *endBlockEntry:
				if block == nil {
					return nil, fmt.Errorf("%v is corrupted", f.file.Name())
				}
				block = nil

			case *recordEntry:
				if block == nil {
					return nil, fmt.Errorf("%v is corrupted", f.file.Name())
				}

				records.Put(e.Key.Hash(), recordLocation{
					file:   uint(fileNo),
					block:  *block,
					header: start + 2, // The header has a 2 byte length prefix
					offset: offset,
					length: e.Length,
				})

				if e.Length <= 0 {
					continue
				}

				// Skip the record data
				offset += e.Length
			}
		}
	}
	err = records.Commit()
	if err != nil {
		return nil, err
	}

	return db, nil
}

func (db *Database) newFile() (*blockFile, error) {
	// Ensure the directory exists
	err := os.Mkdir(db.path, 0700)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, err
	}

	// Create a new file
	name := db.nameFn(len(db.files))
	if name == "" {
		return nil, fmt.Errorf("invalid block file name: empty")
	} else if filepath.Base(name) != name {
		return nil, fmt.Errorf("invalid block file name: %q contains a slash or is empty", name)
	}
	name = filepath.Join(db.path, name)

	f, err := os.OpenFile(name, os.O_RDWR|os.O_EXCL|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}

	bf := &blockFile{f, nil}
	db.files = append(db.files, bf)
	return bf, nil
}

func (db *Database) Close() error {
	var errs []error
	for _, f := range db.files {
		e1 := f.mmap.Unmap()
		e2 := f.file.Close()
		if e1 != nil {
			errs = append(errs, e1)
		}
		if e2 != nil {
			errs = append(errs, e2)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs[0]
}

// Begin begins a change set.
func (d *Database) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	view := d.records.View()
	get := func(key *record.Key) ([]byte, error) {
		return d.get(view, key)
	}

	forEach := func(fn func(*record.Key, []byte) error) error {
		return d.forEach(view, fn)
	}

	var commit memory.CommitFunc
	if writable {
		commit = func(entries map[[32]byte]memory.Entry) error {
			return d.commit(view, entries)
		}
	}

	discard := view.Discard

	return memory.NewChangeSet(memory.ChangeSetOptions{
		Prefix:  prefix,
		Get:     get,
		ForEach: forEach,
		Commit:  commit,
		Discard: discard,
	})
}

func (d *Database) get(view *recordsView, key *record.Key) ([]byte, error) {
	loc, ok := view.Get(key.Hash())
	if !ok || loc.length < 0 {
		return nil, (*database.NotFoundError)(key)
	}
	if !d.validLoc(loc) {
		return nil, errors.InternalError.WithFormat("record is corrupted")
	}
	return d.read(loc), nil
}

func (d *Database) forEach(view *recordsView, fn func(*record.Key, []byte) error) error {
	return view.ForEach(func(key [32]byte, loc recordLocation) error {
		// Skip deleted entries
		if loc.length < 0 {
			return nil
		}
		if !d.validLoc(loc) {
			return errors.InternalError.WithFormat("record is corrupted")
		}

		header, err := d.readHeader(loc)
		if err != nil {
			return err
		}

		return fn(header.Key, d.read(loc))
	})
}

func (d *Database) validLoc(loc recordLocation) bool {
	// If any of these conditions fail, there's a bug. Record locations are
	// initialized from disk, so any issue there indicates corruption or an
	// initialization bug. Record locations are only ever added by Commit, which
	// writes the records to disk and remaps them before committing the record
	// locations, so any issue there indicates a bug.
	switch {
	case loc.header < 0 || loc.offset < 0 || loc.header > loc.offset:
		// Corrupted offsets
		return false

	case loc.file >= uint(len(d.files)):
		// loc.file is invalid
		return false

	case d.files[loc.file].mmap == nil:
		// File is not memory mapped
		return false

	case loc.offset+loc.length > int64(len(d.files[loc.file].mmap)):
		// Requested range is outside the memory mapped region
		return false
	}
	return true
}

func (d *Database) read(loc recordLocation) []byte {
	b := make([]byte, loc.length)
	copy(b, d.files[loc.file].mmap[loc.offset:])
	return b
}

func (d *Database) readHeader(loc recordLocation) (*recordEntry, error) {
	b := make([]byte, loc.length)
	copy(b, d.files[loc.file].mmap[loc.header:loc.offset])
	e := new(recordEntry)
	err := e.UnmarshalBinary(b)
	return e, err
}

func (d *Database) commit(view *recordsView, entries map[[32]byte]memory.Entry) error {
	defer view.Discard()

	// Commits must be serialized
	d.commitMu.Lock()
	defer d.commitMu.Unlock()

	// Seek to the end of the newest file or create a new file
	fileNo := len(d.files) - 1
	var f *blockFile
	var offset int64
	var err error
	if fileNo < 0 {
		fileNo = 0
		f, err = d.newFile()
	} else {
		f = d.files[fileNo]
		offset, err = f.file.Seek(0, io.SeekEnd)
	}
	if err != nil {
		return err
	}

	var block uint64
	var haveBlock bool
	for kh, e := range entries {
		// Time for a new file?
		if offset >= int64(d.fileLimit) {
			// Close the block
			if haveBlock {
				haveBlock = false
				_, err = writeEntry(f.file, &endBlockEntry{})
				if err != nil {
					return err
				}
			}

			// Remap the file
			if f.mmap != nil {
				err := f.mmap.Unmap()
				f.mmap = nil
				if err != nil {
					return err
				}
			}

			f.mmap, err = mmap.Map(f.file, mmap.RDWR, 0)
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

			n, err := writeEntry(f.file, b)
			if err != nil {
				return err
			}
			offset += int64(n)
		}

		l := int64(len(e.Value))
		if e.Delete {
			l = -1
		}

		// Write the entry
		n, err := writeEntry(f.file, &recordEntry{Key: e.Key, Length: l})
		if err != nil {
			return err
		}
		offset += int64(n)
		view.Put(kh, recordLocation{file: uint(fileNo), block: block, offset: offset, length: l})

		if e.Delete {
			continue
		}

		// Write the data
		n, err = f.file.Write(e.Value)
		if err != nil {
			return err
		}
		offset += int64(n)
	}
	err = view.Commit()
	if err != nil {
		return err
	}

	if !haveBlock {
		return nil
	}

	// Close the block
	_, err = writeEntry(f.file, &endBlockEntry{})
	if err != nil {
		return err
	}

	// Remap the file
	if f.mmap != nil {
		err := f.mmap.Unmap()
		f.mmap = nil
		if err != nil {
			return err
		}
	}

	f.mmap, err = mmap.Map(f.file, mmap.RDWR, 0)
	if err != nil {
		return err
	}

	return nil
}