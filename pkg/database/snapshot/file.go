// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"encoding/binary"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

const Version2 = 2

type Reader struct {
	rd     *ioutil.SegmentedReader[SectionType, *SectionType]
	header *Header
}

type Writer struct {
	wr          *ioutil.SegmentedWriter[SectionType, *SectionType]
	wroteHeader bool
}

// Open opens a snapshot file for reading.
func Open(file ioutil.SectionReader) (*Reader, error) {
	// Get the first section - it must be a header
	r := ioutil.NewSegmentedReader[SectionType](file)
	s, err := r.Next()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if s.Type() != SectionTypeHeader {
		return nil, errors.BadRequest.WithFormat("bad first section: expected %v, got %v", SectionTypeHeader, s.Type())
	}

	// Open it
	sr, err := s.Open()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open header section: %w", err)
	}

	// Unmarshal the header
	header := new(Header)
	_, err = header.readFrom(sr)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("read header: %w", err)
	}

	// Return an error if the version is wrong
	if header.Version != Version2 {
		return nil, errors.EncodingError.WithFormat("wrong version: want %d, got %d", Version2, header.Version)
	}

	return &Reader{r, header}, nil
}

// Create opens a snapshot file for writing.
func Create(file io.WriteSeeker, header *Header) (*Writer, error) {
	wr := ioutil.NewSegmentedWriter[SectionType](file)
	return &Writer{wr: wr}, nil
}

func (w *Writer) WriteHeader(header *Header) error {
	// Write the header
	sw, err := w.open(SectionTypeHeader)
	if err != nil {
		return errors.UnknownError.WithFormat("open header section: %w", err)
	}

	header.Version = Version2
	_, err = header.writeTo(sw)
	if err != nil {
		return errors.UnknownError.WithFormat("write header section: %w", err)
	}
	err = sw.Close()
	if err != nil {
		return errors.UnknownError.WithFormat("close header section: %w", err)
	}
	return nil
}

func (w *Writer) open(typ SectionType) (*ioutil.SegmentWriter[SectionType, *SectionType], error) {
	if typ != SectionTypeHeader && !w.wroteHeader {
		return nil, errors.NotReady.WithFormat("header has not been written")
	}
	w.wroteHeader = true
	return w.wr.Open(typ)
}

func (h *Header) readFrom(rd io.Reader) (int64, error) {
	// Get the length of the header
	var v [8]byte
	n, err := io.ReadFull(rd, v[:])
	if err != nil {
		return int64(n), errors.EncodingError.WithFormat("read length: %w", err)
	}
	l := binary.BigEndian.Uint64(v[:])

	// Read the header bytes
	b := make([]byte, l)
	m, err := io.ReadFull(rd, b)
	if err != nil {
		return int64(n + m), errors.EncodingError.WithFormat("read data: %w", err)
	}

	// Version check
	vh := new(versionHeader)
	err = vh.UnmarshalBinary(b)
	if vh.Version != Version2 {
		h.Version = vh.Version
		return int64(n + m), nil
	}

	// Unmarshal the header
	err = h.UnmarshalBinary(b)
	if err != nil {
		return int64(n + m), errors.EncodingError.WithFormat("unmarshal: %w", err)
	}

	return int64(n + m), nil
}

func (h *Header) writeTo(wr io.Writer) (int64, error) {
	// Marshal the header
	b, err := h.MarshalBinary()
	if err != nil {
		return 0, errors.EncodingError.WithFormat("marshal: %w", err)
	}

	// Write the length
	var v [8]byte
	binary.BigEndian.PutUint64(v[:], uint64(len(b)))
	n, err := wr.Write(v[:])
	if err != nil {
		return int64(n), errors.EncodingError.WithFormat("write length: %w", err)
	}

	// Write the header bytes
	m, err := wr.Write(b)
	if err != nil {
		return int64(n + m), errors.EncodingError.WithFormat("write data: %w", err)
	}

	return int64(n + m), nil
}
