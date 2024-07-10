// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"
	"path/filepath"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"golang.org/x/exp/slog"
)

type databaseView struct {
	recordFiles *recordFileSet
	indexFiles  *indexFileTree
	blocks      *vmapView[blockID, int]
	records     *vmapView[[32]byte, *recordLocation]
}

func (db *Database) indexBlocks() error {
	blocks := db.blockIndex.View()
	it := db.records.entries(func(typ entryType) bool {
		return typ == entryTypeStartBlock
	})
	var prev *recordFile
	it.Range(func(_ int, item recordFilePos) bool {
		if item.File != prev {
			slog.Info("Indexing blocks", "file", filepath.Base(item.File.file.Name()), "module", "database")
			prev = item.File
		}

		s := item.Entry.(*startBlockEntry)
		if _, ok := blocks.Get(s.blockID); ok {
			it.err = fmt.Errorf("%v is corrupted: duplicate block %v", filepath.Base(item.File.file.Name()), s.blockID.String())
			return false
		}

		blocks.Put(s.blockID, item.FileIndex)
		return true
	})
	if it.err != nil {
		return it.err
	}

	return blocks.Commit()
}

func (db *Database) indexRecords() error {
	// Assume the database has already been indexed
	if _, ok := db.indexFiles.root.(emptyIndexFileTree); !ok {
		return nil
	}

	records := db.recordLocation.View()
	it := db.records.entries(nil)

	var prev *recordFile
	var block *blockID
	it.Range(func(_ int, item recordFilePos) bool {
		if item.File != prev {
			slog.Info("Indexing entries", "file", filepath.Base(item.File.file.Name()), "module", "database")
			block = nil
			prev = item.File
		}

		switch e := item.Entry.(type) {
		case *startBlockEntry:
			if block != nil {
				it.err = fmt.Errorf("%v is corrupted", filepath.Base(item.File.file.Name()))
				return false
			}
			block = &e.blockID
			slog.Info("Indexing block", "file", filepath.Base(item.File.file.Name()), "block", block, "module", "database")

		case *endBlockEntry:
			if block == nil {
				it.err = fmt.Errorf("%v is corrupted", filepath.Base(item.File.file.Name()))
				return false
			}
			block = nil

			// Commit at the end of each block to keep the memory usage under
			// control for large databases
			err := records.Commit()
			if err != nil {
				it.err = err
				return false
			}
			records = db.recordLocation.View()

		case *recordEntry:
			if block == nil {
				it.err = fmt.Errorf("%v is corrupted", filepath.Base(item.File.file.Name()))
				return false
			}

			records.Put(e.KeyHash, &recordLocation{
				Block:     block,
				Offset:    item.Start,
				HeaderLen: item.End - item.Start,
				RecordLen: e.Length,
			})
		}
		return true
	})
	if it.err != nil {
		return it.err
	}

	return records.Commit()
}

func (r *databaseView) Get(key *record.Key) ([]byte, error) {
	loc, ok := r.records.Get(key.Hash())
	if !ok {
		loc, ok = r.indexFiles.Get(key.Hash())
		if ok {
			defer poolLocation.Put(loc)
		}
	}
	if !ok || loc.RecordLen < 0 {
		return nil, (*database.NotFoundError)(key)
	}

	f, err := r.getFile(loc)
	if err != nil {
		return nil, err
	}

	return f.ReadRecord(loc)
}

func (r *databaseView) ForEach(fn func(*record.Key, []byte) error) error {
	return r.records.ForEach(func(key [32]byte, loc *recordLocation) error {
		// Skip deleted entries
		if loc.RecordLen < 0 {
			return nil
		}

		f, err := r.getFile(loc)
		if err != nil {
			return err
		}

		header, err := f.ReadHeader(loc)
		if err != nil {
			return err
		}

		record, err := f.ReadRecord(loc)
		if err != nil {
			return err
		}

		return fn(header.Key, record)
	})
}

func (r *databaseView) getFile(l *recordLocation) (*recordFile, error) {
	i, ok := r.blocks.Get(*l.Block)
	if !ok {
		return nil, errors.InternalError.WithFormat("corrupted: cannot locate block %v", l.Block)
	}
	if i >= len(r.recordFiles.files) || r.recordFiles.files[i] == nil {
		return nil, errors.InternalError.With("corrupted: invalid block index entry")
	}
	return r.recordFiles.files[i], nil
}

func (r *databaseView) Discard() {
	r.blocks.Discard()
	r.records.Discard()
}

func (r *databaseView) Commit() error {
	return errors.Join(
		r.blocks.Commit(),
		r.records.Commit(),
	)
}
