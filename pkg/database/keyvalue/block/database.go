// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"encoding/binary"
	"errors"
	"sort"
	"sync"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

type Database struct {
	config
	commitMu sync.Mutex
	files    blockFileSet
	index    recordIndex
}

type config struct {
	nextBlock atomic.Uint64
	fileLimit int64
	nameFmt   NameFormat
	filterFn  func(string) bool
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
	db := new(Database)
	defer func() {
		if err != nil {
			_ = db.Close()
		}
	}()

	db.fileLimit = 1 << 30
	db.nameFmt = DefaultNameFormat

	for _, o := range options {
		o(db)
	}

	// Load files
	err = db.openFileSet(path)
	if err != nil {
		return nil, err
	}

	// Determine the next block number
	if len(db.files.files) > 0 {
		it := db.files.files[len(db.files.files)-1].entries(func(typ entryType) bool {
			return typ == entryTypeStartBlock
		})
		it.Range(func(_ int, item entryPos) bool {
			s := item.entry.(*startBlockEntry)
			if db.nextBlock.Load() < s.ID {
				db.nextBlock.Store(s.ID)
			}
			return true
		})
		if it.err != nil {
			return nil, it.err
		}
	}

	// Index blocks
	err = db.index.indexBlocks(db.files.files)
	if err != nil {
		return nil, err
	}

	// Build the block index
	err = db.index.indexRecords(path, db.files.files)
	if err != nil {
		return nil, err
	}

	return db, nil
}

func (db *Database) Close() error {
	return errors.Join(
		db.files.Close(),
		db.index.Close(),
	)
}

// Begin begins a change set.
func (d *Database) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	view := d.index.View(d.files.files)
	get := func(key *record.Key) ([]byte, error) {
		return view.Get(key)
	}

	forEach := func(fn func(*record.Key, []byte) error) error {
		return view.ForEach(fn)
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

func (d *Database) commit(view *recordIndexView, entries map[[32]byte]memory.Entry) error {
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
		if file == nil && len(d.files.files) > 0 {
			file = d.files.files[len(d.files.files)-1]

		} else {
			file, err = d.files.new(&block.blockID)
			if err != nil {
				return err
			}
		}

		n, err := file.writeEntries(len(d.files.files)-1, view, list, block)
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
