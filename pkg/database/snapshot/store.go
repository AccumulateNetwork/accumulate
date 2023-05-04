// Copyright 2023 The Accumulate Authors
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
		return nil, errors.NotFound.WithFormat("%v not found", key)
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
	if bytes.Compare(x.Key[:], target[:]) != 0 {
		return nil, errors.NotFound.WithFormat("%v not found", key)
	}

	// Read the record from the given section and offset
	rd, err = s.open(x.Section)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open index: %w", err)
	}

	_, err = rd.Seek(int64(x.Offset), io.SeekStart)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("seek to record: %w", err)
	}

	entry, err := (&RecordReader{rd}).Read()
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

func readEntry(rd ioutil.SectionReader, i int, b *[indexEntrySize]byte) error {
	_, err := rd.ReadAt(b[:], int64(i*len(b)))
	return err
}
