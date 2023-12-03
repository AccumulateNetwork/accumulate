// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package ioutil

import (
	"encoding/binary"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// NewSegmentedReader returns a new segmented reader for the file.
func NewSegmentedReader[V enumGet, U enumSet[V]](file SectionReader) *SegmentedReader[V, U] {
	return &SegmentedReader[V, U]{file: file}
}

// SegmentedReader reads a segmented file.
type SegmentedReader[V enumGet, U enumSet[V]] struct {
	file   SectionReader
	offset int64
	done   bool
}

func findNext[V enumGet, U enumSet[V]](rd io.ReadSeeker, at int64) (typ U, size, next int64, _ error) {
	_, err := rd.Seek(at, io.SeekStart)
	if err != nil {
		return nil, 0, 0, errors.UnknownError.WithFormat("seek to next segment: %w", err)
	}

	var header [64]byte
	_, err = io.ReadFull(rd, header[:])
	if err != nil {
		return nil, 0, 0, errors.UnknownError.WithFormat("read segment header: %w", err)
	}

	typ = U(new(V))
	v := binary.BigEndian.Uint16(header[0:])
	if !typ.SetEnumValue(uint64(v)) {
		return nil, 0, 0, errors.UnknownError.WithFormat("%d is not a valid segment type", v)
	}

	size = int64(binary.BigEndian.Uint64(header[8:]))
	next = int64(binary.BigEndian.Uint64(header[16:]))
	return typ, size, next, nil
}

// Next finds the segment.
func (r *SegmentedReader[V, U]) Next() (*Segment[V, U], error) {
	if r.done {
		return nil, io.EOF
	}

	typ, size, next, err := findNext[V, U](r.file, r.offset)
	if err != nil {
		return nil, err
	}

	s := new(Segment[V, U])
	s.file = r.file
	s.offset = r.offset + 64
	s.typ = *typ
	s.size = size

	r.offset = next
	if next == 0 {
		r.done = true
	}
	return s, nil
}

// Segment is a segment of a file.
type Segment[V enumGet, U enumSet[V]] struct {
	typ    V
	offset int64
	file   SectionReader
	size   int64
}

// Type returns the segment's type.
func (s *Segment[V, U]) Type() V { return s.typ }

// Offset returns the segment's offset.
func (s *Segment[V, U]) Offset() int64 { return s.offset }

// Size returns the segment's size.
func (s *Segment[V, U]) Size() int64 { return s.size }

// Open opens the segment for reading.
func (s *Segment[V, U]) Open() (SectionReader, error) {
	return NewSectionReader(s.file, s.offset, s.offset+s.size)
}
