// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"bytes"
	"io"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

type Store struct {
	sections []*sectionReader
	readers  []ioutil.SectionReader
	index    int
	count    int
}

var _ keyvalue.Store = (*Store)(nil)

func (r *Reader) AsStore() (*Store, error) {
	s := new(Store)
	s.sections = r.Sections

	var found bool
	for i, section := range s.sections {
		if section.Type() == SectionTypeRecordIndex {
			s.index, found = i, true
			break
		}
	}
	if !found {
		return nil, errors.NotFound.WithFormat("snapshot does not have a record index")
	}

	size := s.sections[s.index].Size()
	if size%indexEntrySize != 0 {
		return nil, errors.InvalidRecord.WithFormat("invalid record index: want size to be a multiple of %d, got %d", indexEntrySize, size)
	}
	s.count = int(size / indexEntrySize)
	s.readers = make([]ioutil.SectionReader, len(s.sections))

	return s, nil
}

func (s *Store) Get(key *record.Key) ([]byte, error) {
	if s.count == 0 {
		return nil, (*database.NotFoundError)(key)
	}
	rd, err := s.open(s.index)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open index: %w", err)
	}
	index := &IndexReader{rd, s.count}

	// This is good enough for a proof of concept but it could be a lot more
	// optimized
	target := key.Hash()
	i := sort.Search(s.count, func(i int) bool {
		if err != nil {
			return false
		}
		var x *RecordIndexEntry
		x, err = index.Read(i)
		return err == nil && bytes.Compare(x.Key[:], target[:]) >= 0
	})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("read index entry: %w", err)
	}

	x, err := index.Read(i)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("read index entry: %w", err)
	}
	if x.Key != target {
		return nil, (*database.NotFoundError)(key)
	}

	// Read the record from the given section and offset
	rd, err = s.open(x.Section)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open index: %w", err)
	}

	entry, err := recordReader{rd}.ReadAt(int64(x.Offset))
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return entry.Value, nil
}

func (s *Store) Put(*record.Key, []byte) error {
	return errors.NotAllowed.With("cannot modify a snapshot store")
}

func (s *Store) Delete(*record.Key) error {
	return errors.NotAllowed.With("cannot modify a snapshot store")
}

// ForEach iterates over each value.
func (s *Store) ForEach(fn func(*record.Key, []byte) error) error {
	for i, ss := range s.sections {
		if ss.Type() != SectionTypeRecords {
			continue
		}

		rd, err := s.open(i)
		if err != nil {
			return err
		}

		rr := recordReader{rd}
	readSection:
		for {
			entry, err := rr.Read()
			switch {
			case err == nil:
				// Ok
			case errors.Is(err, io.EOF):
				break readSection
			default:
				return err
			}

			err = fn(entry.Key, entry.Value)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) open(i int) (ioutil.SectionReader, error) {
	if i >= len(s.sections) {
		return nil, errors.InvalidRecord.WithFormat("invalid section ID %d", i)
	}
	if s.readers[i] != nil {
		return s.readers[i], nil
	}

	rd, err := s.sections[i].Open()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	s.readers[i] = rd
	return rd, nil
}
