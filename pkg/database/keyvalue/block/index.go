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
	"sync/atomic"

	"github.com/syndtr/goleveldb/leveldb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	ldb2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/leveldb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"golang.org/x/exp/slog"
)

type recordIndex struct {
	db      *leveldb.DB
	blocks  vmap[blockID, int]
	records vmap[[32]byte, *recordLocation]
}

type recordIndexView struct {
	index   *recordIndex
	files   []*blockFile
	blocks  *vmapView[blockID, int]
	records *vmapView[[32]byte, *recordLocation]
}

func (r *recordIndex) indexBlocks(files []*blockFile) error {
	blocks := r.blocks.View()
	for i, f := range files {
		slog.Info("Indexing blocks", "file", filepath.Base(f.file.Name()), "module", "database")

		it := f.entries(func(typ entryType) bool {
			return typ == entryTypeStartBlock
		})
		it.Range(func(_ int, item entryPos) bool {
			s := item.entry.(*startBlockEntry)
			if _, ok := blocks.Get(s.blockID); ok {
				it.err = fmt.Errorf("%v is corrupted: duplicate block %v", f.file.Name(), s.blockID.String())
				return false
			}

			blocks.Put(s.blockID, i)
			return true
		})
		if it.err != nil {
			return it.err
		}
	}

	return blocks.Commit()
}

func (r *recordIndex) openDB(dir string) (exists bool, err error) {
	path := filepath.Join(dir, "index.ldb")
	_, err = os.Stat(path)
	switch {
	case err == nil:
		exists = true
	case !errors.Is(err, fs.ErrNotExist):
		return false, err
	}

	r.db, err = leveldb.OpenFile(path, nil)
	if err != nil {
		return false, err
	}

	rBPT := new(atomic.Pointer[bpt.BPT])
	rBPT.Store(r.openBPT((*kvstore)(r)))
	r.records.fn.get = func(key [32]byte) (*recordLocation, bool) {
		b, err := rBPT.Load().Get(record.KeyFromHash(key))
		if err != nil {
			if !errors.Is(err, errors.NotFound) {
				slog.Error("Failed to look up record location", "error", err)
			}
			return nil, false
		}

		loc := new(recordLocation)
		err = loc.UnmarshalBinary(b)
		if err != nil {
			slog.Error("Failed to decode record location", "error", err)
			return nil, false
		}
		return loc, true
	}

	r.records.fn.forEach = func(fn func([32]byte, *recordLocation) error) error {
		return bpt.ForEach(rBPT.Load(), func(key *record.Key, value []byte) error {
			loc := new(recordLocation)
			err = loc.UnmarshalBinary(value)
			if err != nil {
				return err
			}
			return fn(key.Hash(), loc)
		})
	}

	r.records.fn.commit = func(entries map[[32]byte]*recordLocation) error {
		batch := ldb2.New(r.db).Begin(nil, true)
		wBPT := r.openBPT(keyvalue.RecordStore{Store: batch})

		for kh, loc := range entries {
			b, err := loc.MarshalBinary()
			if err != nil {
				return err
			}
			err = wBPT.Insert(record.KeyFromHash(kh), b)
			if err != nil {
				return err
			}
		}

		err = wBPT.Commit()
		if err != nil {
			return err
		}
		err = batch.Commit()
		if err != nil {
			return err
		}

		rBPT.Store(r.openBPT((*kvstore)(r)))
		return nil
	}

	return exists, nil
}

func (r *recordIndex) indexRecords(dir string, files []*blockFile) error {
	exists, err := r.openDB(dir)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	records := r.records.View()
	for _, f := range files {
		slog.Info("Indexing entries", "file", filepath.Base(f.file.Name()), "module", "database")
		var block *blockID

		it := f.entries(nil)
		it.Range(func(_ int, item entryPos) bool {
			switch e := item.entry.(type) {
			case *startBlockEntry:
				if block != nil {
					it.err = fmt.Errorf("%v is corrupted", f.file.Name())
					return false
				}
				block = &e.blockID

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
	}

	return records.Commit()
}

func (r *recordIndex) openBPT(store database.Store) *bpt.BPT {
	b := bpt.New(nil, nil, store, nil)
	err := b.SetParams(bpt.Parameters{
		Power:           8,
		ArbitraryValues: true,
	})
	if err != nil {
		panic(err)
	}
	return b
}

func (r *recordIndex) View(files []*blockFile) *recordIndexView {
	return &recordIndexView{
		r, files,
		r.blocks.View(),
		r.records.View(),
	}
}

func (r *recordIndex) Close() error {
	if r.db == nil {
		return nil
	}
	return r.db.Close()
}

func (r *recordIndexView) Get(key *record.Key) ([]byte, error) {
	loc, ok := r.records.Get(key.Hash())
	if !ok || loc.RecordLen < 0 {
		return nil, (*database.NotFoundError)(key)
	}

	f, err := r.getFile(loc)
	if err != nil {
		return nil, err
	}

	return f.ReadRecord(loc)
}

func (r *recordIndexView) ForEach(fn func(*record.Key, []byte) error) error {
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

func (r *recordIndexView) getFile(l *recordLocation) (*blockFile, error) {
	i, ok := r.blocks.Get(*l.Block)
	if !ok {
		return nil, errors.InternalError.WithFormat("corrupted: cannot locate block %v", l.Block)
	}
	if i >= len(r.files) || r.files[i] == nil {
		return nil, errors.InternalError.With("corrupted: invalid block index entry")
	}

	f := r.files[i]
	if f.file.Len() < int(l.end()) {
		return nil, errors.InternalError.With("corrupted: record is past the end of the file")
	}
	return f, nil
}

func (r *recordIndexView) Discard() {
	r.blocks.Discard()
	r.records.Discard()
}

func (r *recordIndexView) Commit() error {
	return errors.Join(
		r.blocks.Commit(),
		r.records.Commit(),
	)
}

func (l *recordLocation) end() int64 {
	x := l.Offset + l.HeaderLen
	if l.RecordLen > 0 {
		x += l.RecordLen
	}
	return x
}

type kvstore recordIndex

func (s *kvstore) GetValue(key *record.Key, value database.Value) error {
	kh := key.Hash()
	b, err := s.db.Get(kh[:], nil)
	switch {
	case err == nil:
		return value.LoadBytes(b, false)
	case errors.Is(err, leveldb.ErrNotFound):
		return (*database.NotFoundError)(key)
	default:
		return err
	}
}

func (s *kvstore) PutValue(key *record.Key, value database.Value) error {
	panic("read-only")
}
