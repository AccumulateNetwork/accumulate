// Copyright 2022 The Accumulate Authors
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

// NewSegmentedWriter returns a new segmented writer.
func NewSegmentedWriter[V enumGet, U enumSet[V]](w io.WriteSeeker) *SegmentedWriter[V, U] {
	return &SegmentedWriter[V, U]{file: w}
}

// SegmentedWriter writes a segmented file.
type SegmentedWriter[V enumGet, U enumSet[V]] struct {
	file        io.WriteSeeker
	openSegment bool
	prevSegment int64
}

// Open opens a segment.
func (w *SegmentedWriter[V, U]) Open(typ V) (*SegmentWriter[V, U], error) {
	if w.openSegment {
		return nil, errors.BadRequest.WithFormat("previous segment has not been closed")
	}

	// Get the current offset
	offset, err := w.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get file offset: %w", err)
	}

	// Update the previous segment's header
	if offset > 0 {
		// Seek to the header
		_, err = w.file.Seek(w.prevSegment+16, io.SeekStart)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("set file offset: %w", err)
		}

		var headerPart [8]byte
		binary.BigEndian.PutUint64(headerPart[:], uint64(offset))
		_, err = w.file.Write(headerPart[:])
		if err != nil {
			return nil, errors.UnknownError.WithFormat("read segment header: %w", err)
		}

		// Restore the previous location
		_, err = w.file.Seek(offset, io.SeekStart)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("set file offset: %w", err)
		}
	}

	// Save space for the header
	_, err = w.file.Write(make([]byte, 64))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("allocate space for header: %w", err)
	}

	// Create a segment reader
	segment, err := NewSectionWriter(w.file, offset+64, -1)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("create segment writer: %w", err)
	}

	w.openSegment = true
	return &SegmentWriter[V, U]{typ, offset, w, segment}, nil
}

func (w *SegmentedWriter[V, U]) closeSegment(s *SegmentWriter[V, U]) error {
	// align is the boundary alignment of segments
	const align = 64

	// Get current offset
	current, err := w.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return errors.UnknownError.WithFormat("get file offset: %w", err)
	}

	// Seek to the header
	_, err = w.file.Seek(s.offset, io.SeekStart)
	if err != nil {
		return errors.UnknownError.WithFormat("seek to segment header: %w", err)
	}

	// Write the segment header
	var header [64]byte
	binary.BigEndian.PutUint16(header[0:], uint16(s.typ.GetEnumValue())) // Type
	binary.BigEndian.PutUint64(header[8:], uint64(current-s.offset-64))  // Size
	_, err = w.file.Write(header[:])
	if err != nil {
		return errors.UnknownError.WithFormat("write segment header: %w", err)
	}

	// Return to the original offset
	_, err = w.file.Seek(current, io.SeekStart)
	if err != nil {
		return errors.UnknownError.WithFormat("restore file offset: %w", err)
	}

	// Pad
	if current%align > 0 {
		pad := align - current%align
		_, err = w.file.Write(make([]byte, pad))
		if err != nil {
			return errors.UnknownError.WithFormat("pad end of segment: %w", err)
		}
	}

	w.openSegment = false
	w.prevSegment = s.offset
	return nil
}

// A SegmentWriter writes a section of a segmented file.
type SegmentWriter[V enumGet, U enumSet[V]] struct {
	typ     V
	offset  int64
	file    *SegmentedWriter[V, U]
	segment *SectionWriter
}

// Type returns the segment's type.
func (s *SegmentWriter[V, U]) Type() V { return s.typ }

// Write writes bytes.
func (w *SegmentWriter[V, U]) Write(p []byte) (n int, err error) {
	return w.segment.Write(p)
}

// Seek seeks to an offset.
func (w *SegmentWriter[V, U]) Seek(offset int64, whence int) (int64, error) {
	return w.segment.Seek(offset, whence)
}

// Close closes the segment and finalizes its header.
func (w *SegmentWriter[V, U]) Close() error {
	return w.file.closeSegment(w)
}
