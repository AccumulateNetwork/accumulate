// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"golang.org/x/exp/slog"
)

type Database struct {
	config
	path     string
	commitMu sync.Mutex
	files    []*blockFile
	records  records
}

type config struct {
	nextFile  atomic.Uint64
	nextBlock atomic.Uint64
	fileLimit int64
	nameFmt   NameFormat
	filterFn  func(string) bool
}

type records = vmap[[32]byte, recordLocation]
type recordsView = vmapView[[32]byte, recordLocation]

type blockLocation struct {
	file   int
	offset int64
}

type recordLocation struct {
	file   int
	block  uint64
	header int64
	offset int64
	length int64
}

type Option func(*Database)

func WithFileLimit(limit int64) Option {
	return func(d *Database) {
		d.fileLimit = limit
	}
}

func WithNameFormat(fmt NameFormat) Option {
	return func(d *Database) {
		d.nameFmt = fmt
	}
}

func FilterFiles(fn func(string) bool) Option {
	return func(d *Database) {
		d.filterFn = fn
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
	db.nameFmt = DefaultNameFormat

	for _, o := range options {
		o(db)
	}

	// Open all the files
	db.files = make([]*blockFile, 0, len(entries))
	for _, e := range entries {
		if db.filterFn != nil && !db.filterFn(e.Name()) {
			continue
		}

		f, err := db.openFile(filepath.Join(path, e.Name()))
		if err != nil {
			return nil, err
		}

		if f.header.Ordinal == 0 {
			return nil, fmt.Errorf("%s does not have an ordinal", f.file.Name())
		}

		if db.nextFile.Load() < f.header.Ordinal {
			db.nextFile.Store(f.header.Ordinal)
		}

		db.files = append(db.files, f)
	}

	// Sort the files by number, since who knows what order the filesystem
	// returns them in
	slices.SortFunc(db.files, func(a, b *blockFile) int {
		return int(a.header.Ordinal) - int(b.header.Ordinal)
	})

	// Ensure the files are a sequence
	var last uint64
	for _, f := range db.files {
		if f.header.Ordinal == last {
			return nil, fmt.Errorf("multiple files with ordinal %d", last)
		}
		last++
		if f.header.Ordinal > last {
			return nil, fmt.Errorf("missing file with ordinal %d", last)
		}
	}

	// Build the block index
	blocks := map[uint64]blockLocation{}
	records := db.records.View()
	for i, f := range db.files {
		slog.Info("Indexing", "ordinal", f.header.Ordinal, "module", "database")
		var block *uint64

		it := f.entries()
		it.Range(func(_ int, item entryPos) bool {
			switch e := item.entry.(type) {
			case *startBlockEntry:
				if block != nil {
					it.err = fmt.Errorf("%v is corrupted", f.file.Name())
					return false
				}
				if _, ok := blocks[e.ID]; ok {
					it.err = fmt.Errorf("duplicate block %d", e.ID)
					return false
				}
				blocks[e.ID] = blockLocation{file: i, offset: item.Start}
				block = &e.ID

				if db.nextBlock.Load() < e.ID {
					db.nextBlock.Store(e.ID)
				}

			case *endBlockEntry:
				if block == nil {
					it.err = fmt.Errorf("%v is corrupted", f.file.Name())
					return false
				}
				block = nil

			case *recordEntry:
				if block == nil {
					it.err = fmt.Errorf("%v is corrupted", f.file.Name())
					return false
				}

				records.Put(e.KeyHash, recordLocation{
					file:   i,
					block:  *block,
					header: item.Start,
					offset: item.End,
					length: e.Length,
				})
			}
			return true
		})

		if it.err != nil {
			return nil, it.err
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
	ordinal := db.nextFile.Add(1)
	name := db.nameFmt.Format(ordinal)
	if name == "" {
		return nil, fmt.Errorf("invalid block file name: empty")
	} else if filepath.Base(name) != name {
		return nil, fmt.Errorf("invalid block file name: %q contains a slash or is empty", name)
	}

	f, err := db.config.newFile(ordinal, filepath.Join(db.path, name))
	if err != nil {
		return nil, err
	}

	db.files = append(db.files, f)
	return f, nil
}

func (db *Database) Close() error {
	var errs []error
	for _, f := range db.files {
		errs = append(errs, f.Close())
	}
	return errors.Join(errs...)
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
	return d.read(loc)
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

		b, err := d.read(loc)
		if err != nil {
			return err
		}

		return fn(header.Key, b)
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

	case loc.file >= len(d.files):
		// loc.file is invalid
		return false

	case loc.offset+loc.length > int64(d.files[loc.file].Len()):
		// File is not memory mapped or requested range is outside the memory
		// mapped region
		return false
	}
	return true
}

func (d *Database) read(loc recordLocation) ([]byte, error) {
	b := make([]byte, loc.length)
	_, err := d.files[loc.file].ReadAt(b, loc.offset)
	if errors.Is(err, io.EOF) {
		return b, io.ErrUnexpectedEOF
	}
	return b, err
}

func (d *Database) readHeader(loc recordLocation) (*recordEntry, error) {
	b := make([]byte, loc.offset-loc.header)
	_, err := d.files[loc.file].ReadAt(b, loc.header)
	if err != nil {
		return nil, err
	}

	e := new(recordEntry)
	err = e.UnmarshalBinary(b)
	return e, err
}

func (d *Database) commit(view *recordsView, entries map[[32]byte]memory.Entry) error {
	defer view.Discard()

	// Commits must be serialized
	d.commitMu.Lock()
	defer d.commitMu.Unlock()

	// Construct an ordered list of entries
	list := make([]*entryAndData, 0, len(entries))
	for keyHash, entry := range entries {
		n := int64(len(entry.Value))
		if entry.Delete {
			n = -1
		}
		list = append(list, &entryAndData{
			recordEntry{
				Key:     entry.Key,
				KeyHash: keyHash,
				Length:  n,
			},
			entry.Value,
		})
	}

	// Sort without allocation (hopefully)
	sort.Slice(list, func(i, j int) bool {
		for b := 0; b < 32; b += 8 {
			x := binary.BigEndian.Uint64(list[i].KeyHash[b : b+8])
			y := binary.BigEndian.Uint64(list[j].KeyHash[b : b+8])
			z := x - y
			if z != 0 {
				return z < 0
			}
		}
		return false
	})

	// Write all the entries
	var file *blockFile
	block := new(startBlockEntry)
	block.ID = d.nextBlock.Add(1)
	for len(list) > 0 {
		// If it's the first time and there are existing files, get the last
		// file. Otherwise make a new file.
		var err error
		if file == nil && len(d.files) > 0 {
			file = d.files[len(d.files)-1]

		} else {
			file, err = d.newFile()
			if err != nil {
				return err
			}
		}

		n, err := file.writeEntries(len(d.files)-1, view, list, block)
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}

		list = list[n:]
		block.Part++
	}

	return view.Commit()
}
