// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// # File format
//
// Snapshots use [ioutil]'s segmented file format, with the following section
// types:
//
//   - [SectionTypeHeader] - a [Header] containing metadata about the snapshot.
//     A snapshot must always begin with this section.
//   - [SectionTypeSnapshot] - a nested snapshot. Useful for creating
//     multi-network snapshots. Used by the Block Summary Network
//     (internal/bsn).
//   - [SectionTypeRecords] - multiple [RecordEntry], usually the contents of a
//     key-value database.
//   - [SectionTypeRecordIndex] - an index of all of the records in a snapshot,
//     stored in a fixed-width format and sorted by key hash. Allows random
//     access of records within the snapshot. See [RecordIndexEntry] and
//     [Store].
//   - [SectionTypeBPT] - BPT entries stored as multiple [RecordEntry] where the
//     value field is the BPT entry's hash.
//   - [SectionTypeRawBPT] - Deprecated. BPT entries stored as a 32-byte key
//     hash and 32-byte value hash (fixed-width).
//   - [SectionTypeConsensus] - consensus parameters in an
//     implementation-specific format.
//
// Unless otherwise stated, sections storing structured data store entries as a
// series of length-prefixed values marshalled with standard Accumulate binary
// encoding. For example, [SectionTypeHeader] has a 64-bit length followed by a
// binary-encoded [Header], and [SectionTypeRecords] is a series of
// binary-encoded [RecordEntry], each prefixed with a 64-bit length.
//
// Snapshot types suffixed with V1 are used for v1 snapshots (deprecated).
package snapshot

import (
	"bufio"
	"io"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

const Version2 = 2

type rawWriter = ioutil.SegmentedWriter[SectionType, *SectionType]
type sectionReader = ioutil.Segment[SectionType, *SectionType]

func GetVersion(file ioutil.SectionReader) (uint64, error) {
	r, err := open(file)
	if err != nil {
		return 0, errors.UnknownError.Wrap(err)
	}

	return r.Header.Version, nil
}

func open(file ioutil.SectionReader) (*Reader, error) {
	// Read the sections
	r := new(Reader)
	for sr := ioutil.NewSegmentedReader[SectionType](file); ; {
		section, err := sr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, errors.UnknownError.Wrap(err)
		}
		r.Sections = append(r.Sections, section)
	}

	// Check for the header section
	if len(r.Sections) == 0 {
		return nil, errors.BadRequest.WithFormat("empty snapshot")
	}
	if r.Sections[0].Type() != SectionTypeHeader {
		return nil, errors.BadRequest.WithFormat("bad first section: expected %v, got %v", SectionTypeHeader, r.Sections[0].Type())
	}

	// Open it
	sr, err := r.Sections[0].Open()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open header section: %w", err)
	}

	// Unmarshal the header
	r.Header = new(Header)
	_, err = r.Header.readFrom(sr)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("read header: %w", err)
	}

	return r, nil
}

// Open opens a snapshot file for reading.
func Open(file ioutil.SectionReader) (*Reader, error) {
	r, err := open(file)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Return an error if the version is wrong
	if r.Header.Version != Version2 {
		return nil, errors.EncodingError.WithFormat("wrong version: want %d, got %d", Version2, r.Header.Version)
	}

	return r, nil
}

// Create opens a snapshot file for writing.
func Create(file io.WriteSeeker) (*Writer, error) {
	wr := ioutil.NewSegmentedWriter[SectionType](file)
	return &Writer{wr: wr}, nil
}

// Append opens a snapshot file for appending.
func Append(file io.ReadWriteSeeker) (*Writer, error) {
	wr, existing, err := ioutil.AppendToSegmented[SectionType](file)

	var haveHeader bool
	for _, s := range existing {
		if s.Type() == SectionTypeHeader {
			haveHeader = true
			break
		}
	}

	return &Writer{wr: wr, sections: len(existing), wroteHeader: haveHeader}, err
}

type Reader struct {
	Sections []*sectionReader
	Header   *Header
}

func (r *Reader) open(i int, typ ...SectionType) (ioutil.SectionReader, *sectionReader, error) {
	if i < 0 {
		for _, s := range r.Sections {
			if s.Type().isOneOf(typ...) {
				rd, err := s.Open()
				if err != nil {
					return nil, nil, errors.UnknownError.Wrap(err)
				}
				return rd, s, nil
			}
		}
		var s []string
		for _, typ := range typ {
			s = append(s, typ.String())
		}
		return nil, nil, errors.NotFound.WithFormat("%v section not found", strings.Join(s, "|"))
	}

	if i < 0 || i >= len(r.Sections) {
		return nil, nil, errors.NotFound.WithFormat("section %d not found", i)
	}

	s := r.Sections[i]
	if !s.Type().isOneOf(typ...) {
		return nil, nil, errors.BadRequest.WithFormat("section %d's type is %v not %v", i, s.Type(), typ)
	}

	rd, err := s.Open()
	if err != nil {
		return nil, nil, errors.UnknownError.Wrap(err)
	}
	return rd, s, nil
}

// Open opens the first section of the given type
func (r *Reader) Open(typ ...SectionType) (ioutil.SectionReader, error) {
	for _, s := range r.Sections {
		if s.Type().isOneOf(typ...) {
			return s.Open()
		}
	}
	return nil, errors.NotFound.WithFormat("%v section not found", typ)
}

func (r *Reader) OpenIndex(i int) (*IndexReader, error) {
	rd, _, err := r.open(i, SectionTypeRecordIndex)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	size := r.Sections[i].Size()
	if size%indexEntrySize != 0 {
		return nil, errors.InvalidRecord.WithFormat("invalid record index: want size to be a multiple of %d, got %d", indexEntrySize, size)
	}
	return &IndexReader{rd, int(size / indexEntrySize)}, nil
}

func (r *Reader) OpenRecords(i int) (RecordReader, error) {
	rd, _, err := r.open(i, SectionTypeRecords)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	bufio.NewReader(rd)
	return recordReader{rd}, nil
}

func (r *Reader) OpenBPT(i int) (RecordReader, error) {
	rd, s, err := r.open(i, SectionTypeRawBPT, SectionTypeBPT)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if s.Type() == SectionTypeRawBPT {
		return rawBptReader{rd}, nil
	}
	return recordReader{rd}, nil
}

type IndexReader struct {
	rd    ioutil.SectionReader
	Count int
}

func (i *IndexReader) Read(n int) (*RecordIndexEntry, error) {
	v := new(RecordIndexEntry)
	err := v.readAt(i.rd, n)
	return v, errors.UnknownError.Wrap(err)
}

type RecordReader interface {
	io.Seeker
	Read() (*RecordEntry, error)
	ReadAt(offset int64) (*RecordEntry, error)
}

// recordReader reads length-prefixed record entries.
type recordReader struct {
	ioutil.SectionReader
}

func (r recordReader) Read() (*RecordEntry, error) {
	v := new(RecordEntry)
	_, err := readValue(r.SectionReader, v)
	return v, err
}

func (r recordReader) ReadAt(offset int64) (*RecordEntry, error) {
	v := new(RecordEntry)
	_, err := readValueAt(r.SectionReader, offset, v)
	return v, err
}

// rawBptReader reads (key hash, value) BPT pairs.
type rawBptReader struct {
	ioutil.SectionReader
}

func (r rawBptReader) Read() (*RecordEntry, error) {
	var b [64]byte
	_, err := io.ReadFull(r.SectionReader, b[:])
	if err != nil {
		return nil, err
	}

	return &RecordEntry{
		Key:   record.KeyFromHash(*(*[32]byte)(b[:32])),
		Value: b[32:],
	}, nil
}

func (r rawBptReader) ReadAt(offset int64) (*RecordEntry, error) {
	var b [64]byte
	_, err := readFullAt(r.SectionReader, offset, b[:])
	if err != nil {
		return nil, err
	}

	return &RecordEntry{
		Key:   record.KeyFromHash(*(*[32]byte)(b[:32])),
		Value: b[32:],
	}, nil
}

type Writer struct {
	wr          *rawWriter
	wroteHeader bool
	sections    int
}

func (w *Writer) WriteHeader(header *Header) error {
	// Write the header
	sw, err := w.OpenRaw(SectionTypeHeader)
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

func (w *Writer) OpenRaw(typ SectionType) (*SectionWriter, error) {
	if typ != SectionTypeHeader && !w.wroteHeader {
		return nil, errors.NotReady.WithFormat("header has not been written")
	}
	no := w.sections
	w.sections++
	w.wroteHeader = true
	sw, err := w.wr.Open(typ)
	if err != nil {
		return nil, err
	}
	return &SectionWriter{no, *sw}, nil
}

type SectionWriter struct {
	no int
	ioutil.SegmentWriter[SectionType, *SectionType]
}

func (c *SectionWriter) SectionNumber() int { return c.no }

func (c *SectionWriter) WriteValue(r encoding.BinaryValue) error {
	_, err := writeValue(c, r)
	return err
}
