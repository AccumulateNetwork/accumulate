// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"bufio"
	"encoding/binary"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

const Version2 = 2

type rawReader = ioutil.SegmentedReader[SectionType, *SectionType]
type rawWriter = ioutil.SegmentedWriter[SectionType, *SectionType]
type sectionReader = ioutil.Segment[SectionType, *SectionType]
type sectionWriter = ioutil.SegmentWriter[SectionType, *SectionType]

// Open opens a snapshot file for reading.
func Open(file ioutil.SectionReader) (*Reader, error) {
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

type Reader struct {
	Sections []*sectionReader
	Header   *Header
}

func (r *Reader) open(i int, typ SectionType) (ioutil.SectionReader, error) {
	if i < 0 || i >= len(r.Sections) {
		return nil, errors.NotFound.WithFormat("section %d not found", i)
	}
	if r.Sections[i].Type() != typ {
		return nil, errors.BadRequest.WithFormat("section %d's type is %v not %v", i, r.Sections[i].Type(), typ)
	}

	rd, err := r.Sections[i].Open()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return rd, nil
}

func (r *Reader) OpenIndex(i int) (*IndexReader, error) {
	rd, err := r.open(i, SectionTypeRecordIndex)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	size := r.Sections[i].Size()
	if size%indexEntrySize != 0 {
		return nil, errors.InvalidRecord.WithFormat("invalid record index: want size to be a multiple of %d, got %d", indexEntrySize, size)
	}
	return &IndexReader{rd, int(size / indexEntrySize)}, nil
}

func (r *Reader) OpenRecords(i int) (*RecordReader, error) {
	rd, err := r.open(i, SectionTypeRecords)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	bufio.NewReader(rd)
	return &RecordReader{rd}, nil
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

type RecordReader struct {
	rd ioutil.SectionReader
}

func (r *RecordReader) Seek(offset int64, whence int) (int64, error) {
	return r.rd.Seek(offset, whence)
}

func (r *RecordReader) Read() (*RecordEntry, error) {
	// Read the length
	var x [4]byte
	_, err := io.ReadFull(r.rd, x[:])
	if err != nil {
		return nil, err
	}

	// Read the record
	n := binary.BigEndian.Uint32(x[:])
	b := make([]byte, n)
	_, err = io.ReadFull(r.rd, b)
	if err != nil {
		return nil, err
	}

	// Parse the record
	v := new(RecordEntry)
	err = v.UnmarshalBinary(b)
	if err != nil {
		return nil, errors.EncodingError.Wrap(err)
	}

	return v, nil
}

type Writer struct {
	wr          *rawWriter
	wroteHeader bool
	sections    int
	index       []RecordIndexEntry
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

func (w *Writer) OpenRaw(typ SectionType) (*sectionWriter, error) {
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

type RecordIndexEntry struct {
	Key     record.KeyHash
	Section int
	Offset  uint64
}

// Section - 2 bytes
// Offset  - 6 bytes
// Hash    - 32 bytes
const indexEntrySize = 2 + 6 + 32

func (r *RecordIndexEntry) writeTo(wr io.Writer) error {
	// Combine section and offset
	if r.Offset&0xFFFF0000_00000000 != 0 {
		return errors.EncodingError.WithFormat("offset is too large")
	}
	v := r.Offset | (uint64(r.Section) << (6 * 8))

	var b [indexEntrySize]byte
	binary.BigEndian.PutUint64(b[:8], v)
	*(*[32]byte)(b[8:]) = r.Key

	_, err := wr.Write(b[:])
	return errors.UnknownError.Wrap(err)
}

func (r *RecordIndexEntry) readAt(rd io.ReaderAt, n int) error {
	var b [40]byte
	_, err := rd.ReadAt(b[:], int64(n)*indexEntrySize)
	if err != nil {
		return errors.EncodingError.Wrap(err)
	}

	v := binary.BigEndian.Uint64(b[:])
	r.Section = int(v >> (6 * 8))
	r.Offset = v & ((1 << (6 * 8)) - 1)
	r.Key = *(*[32]byte)(b[8:])
	return nil
}
