// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"bytes"
	"encoding/binary"
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

func Open(file ioutil.SectionReader) (*Store, error) {
	rd, header, _, err := open(file)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	s := new(Store)
	s.sections = append(s.sections, header)

	var found bool
	for {
		section, err := rd.Next()
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		s.sections = append(s.sections, section)

		if section.Type() == SectionTypeRecordIndex {
			s.index, found = len(s.sections)-1, true
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
	index, err := s.open(s.index)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open index: %w", err)
	}

	target := key.Hash()
	i := sort.Search(s.count, func(i int) bool {
		if err != nil {
			return false
		}
		var b [indexEntrySize]byte
		err = readEntry(index, i, &b)
		return err == nil && bytes.Compare(b[16:], target[:]) >= 0
	})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("read index entry: %w", err)
	}

	var b [indexEntrySize]byte
	err = readEntry(index, i, &b)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("read index entry: %w", err)
	}
	if bytes.Compare(b[16:], target[:]) != 0 {
		return nil, errors.NotFound.WithFormat("%v not found", key)
	}

	// Decode the record index, open the section, and seek to the record
	x := binary.BigEndian.Uint64(b[:8])
	section := int(x >> (6 * 8))
	size := x & ((1 << (6 * 8)) - 1)
	offset := binary.BigEndian.Uint64(b[8:16])
	rd, err := s.open(section)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open index: %w", err)
	}
	_, err = rd.Seek(int64(offset), io.SeekStart)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("seek to record: %w", err)
	}

	v := make([]byte, size)
	_, err = io.ReadFull(rd, v)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	entry := new(recordEntry)
	err = entry.UnmarshalBinary(v)
	if err != nil {
		return nil, errors.EncodingError.Wrap(err)
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
