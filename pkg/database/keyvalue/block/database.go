// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"
	"errors"
	"slices"
	"sync"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/internal/util/pool"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/internal/vmap"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/core/schema/pkg/binary"
)

var poolEncoder = binary.NewEncoderPool()
var poolDecoder = binary.NewDecoderPool()
var poolLocation = pool.New[recordLocation]()
var poolBuffer = binary.NewBufferPool()

type Database struct {
	config
	commitMu       sync.Mutex
	records        *recordFileSet
	indexFiles     *indexFileTree
	blockIndex     *vmap.Map[blockID, int]
	recordLocation *vmap.Map[[32]byte, *recordLocation]
}

type config struct {
	path      string
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

	db.path = path
	db.fileLimit = 1 << 30
	db.nameFmt = DefaultNameFormat

	for _, o := range options {
		o(db)
	}

	// Load record files
	db.records, err = openRecordFileSet(&db.config)
	if err != nil {
		return nil, err
	}

	// Load index files
	db.indexFiles, err = openIndexFileTree(&db.config)
	if err != nil {
		return nil, err
	}

	db.blockIndex = vmap.New[blockID, int]()
	db.recordLocation = vmap.New(
		vmap.WithCommit(db.indexFiles.Commit),
		vmap.WithForEach(db.indexFiles.ForEach))

	// Determine the next block number
	if len(db.records.files) > 0 {
		it := db.records.files[len(db.records.files)-1].entries(func(typ entryType) bool {
			return typ == entryTypeStartBlock
		})
		it.Range(func(_ int, item recordPos) bool {
			s := item.Entry.(*startBlockEntry)
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
	err = db.indexBlocks()
	if err != nil {
		return nil, err
	}

	// Build the block index
	err = db.indexRecords()
	if err != nil {
		return nil, err
	}

	return db, nil
}

func (db *Database) Close() error {
	return errors.Join(
		db.records.Close(),
		db.indexFiles.Close(),
	)
}

// Begin begins a change set.
func (d *Database) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	view := &databaseView{
		recordFiles: d.records,
		indexFiles:  d.indexFiles,
		blocks:      d.blockIndex.View(),
		records:     d.recordLocation.View(),
	}

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

func (d *Database) commit(view *databaseView, entries map[[32]byte]memory.Entry) error {
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

	// Sort to remove non-deterministic weirdness
	slices.SortFunc(list, func(a, b *entryAndData) int {
		return bytes.Compare(a.KeyHash[:], b.KeyHash[:])
	})

	// Write all the entries
	err := d.records.Commit(view, list)
	if err != nil {
		return err
	}

	return view.Commit()
}
