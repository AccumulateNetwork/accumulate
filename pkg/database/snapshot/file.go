// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"io"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

const Version2 = 2

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
func Create(file io.WriteSeeker) (*Writer, error) {
	wr := ioutil.NewSegmentedWriter[SectionType](file)
	return &Writer{wr: wr}, nil
}

type Reader struct {
	rd     *ioutil.SegmentedReader[SectionType, *SectionType]
	header *Header
}

type Writer struct {
	wr          *ioutil.SegmentedWriter[SectionType, *SectionType]
	wroteHeader bool
	sections    int
	index       []recordKeyAndOffset
}

func (w *Writer) WriteHeader(header *Header) error {
	// Write the header
	sw, err := w.open(SectionTypeHeader)
	if err != nil {
		return errors.UnknownError.WithFormat("open header section: %w", err)
	}

	header.Version = Version2
	_, err = writeValue(sw, header)
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
	w.sections++
	w.wroteHeader = true
	return w.wr.Open(typ)
}

func (h *Header) readFrom(rd io.Reader) (int64, error) {
	// Read the header bytes
	b, n, err := readRecord(rd)
	if err != nil {
		return n, errors.UnknownError.Wrap(err)
	}

	// Version check
	vh := new(versionHeader)
	err = vh.UnmarshalBinary(b)
	if vh.Version != Version2 {
		h.Version = vh.Version
		return n, nil
	}

	// Unmarshal the header
	err = h.UnmarshalBinary(b)
	if err != nil {
		return n, errors.EncodingError.WithFormat("unmarshal: %w", err)
	}

	return n, nil
}
