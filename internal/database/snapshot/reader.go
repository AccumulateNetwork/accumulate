// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"encoding/binary"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
)

type Reader struct {
	file   ioutil2.SectionReader
	offset int64
	done   bool
}

type ReaderSection struct {
	file   ioutil2.SectionReader
	typ    SectionType
	offset int64
	size   int64
}

func NewReader(file ioutil2.SectionReader) *Reader {
	return &Reader{file: file}
}

func (s *ReaderSection) Type() SectionType { return s.typ }
func (s *ReaderSection) Offset() int64     { return s.offset }
func (s *ReaderSection) Size() int64       { return s.size }

// Next finds the section.
func (r *Reader) Next() (*ReaderSection, error) {
	if r.done {
		return nil, io.EOF
	}

	_, err := r.file.Seek(r.offset, io.SeekStart)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "seek to next section: %w", err)
	}

	var header [64]byte
	_, err = io.ReadFull(r.file, header[:])
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "read section header: %w", err)
	}

	s := new(ReaderSection)
	s.file = r.file
	s.offset = r.offset + 64
	s.typ = SectionType(binary.BigEndian.Uint16(header[0:]))
	s.size = int64(binary.BigEndian.Uint64(header[8:]))
	next := int64(binary.BigEndian.Uint64(header[16:]))

	r.offset = next
	if next == 0 {
		r.done = true
	}
	return s, nil
}

func (s *ReaderSection) Open() (ioutil2.SectionReader, error) {
	return ioutil2.NewSectionReader(s.file, s.offset, s.offset+s.size)
}
