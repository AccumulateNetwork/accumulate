// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"
	"path/filepath"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/core/schema/pkg/binary"
	"golang.org/x/exp/slog"
)

type recordIndex struct {
	blocks  vmap[blockID, int]
	records vmap[[32]byte, *recordLocation]
}

type recordIndexView struct {
	files   []*blockFile
	blocks  *vmapView[blockID, int]
	records *vmapView[[32]byte, *recordLocation]
}

func (r *recordIndex) indexBlocks(files []*blockFile) error {
	blocks := r.blocks.View()
	for i, f := range files {
		slog.Info("Indexing blocks", "file", filepath.Base(f.file.Name()), "module", "database")

		it := f.entries()
		it.Range(func(_ int, item entryPos) bool {
			s, ok := item.entry.(*startBlockEntry)
			if !ok {
				return true
			}

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

func (r *recordIndex) indexRecords(files []*blockFile) error {
	records := r.records.View()
	for _, f := range files {
		slog.Info("Indexing entries", "file", filepath.Base(f.file.Name()), "module", "database")
		var block *blockID

		it := f.entries()
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

func (r *recordIndex) View(files []*blockFile) *recordIndexView {
	return &recordIndexView{
		files:   files,
		blocks:  r.blocks.View(),
		records: r.records.View(),
	}
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

	return loc.readRecord(f)
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

		header, err := loc.readHeader(f)
		if err != nil {
			return err
		}

		record, err := loc.readRecord(f)
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
	if f.Len() < int(l.end()) {
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

func (l *recordLocation) readHeader(f *blockFile) (*recordEntry, error) {
	rd := f.ReadRange(l.Offset, l.Offset+l.HeaderLen)
	dec := binary.NewDecoder(rd)
	e := new(recordEntry)
	err := e.UnmarshalBinaryV2(dec)
	return e, err
}

func (l *recordLocation) readRecord(f *blockFile) ([]byte, error) {
	if l.RecordLen < 0 {
		panic("record was deleted")
	}
	b := make([]byte, l.RecordLen)
	_, err := f.ReadAt(b, l.Offset+l.HeaderLen)
	return b, err
}
