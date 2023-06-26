// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"bufio"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

const Version2 = 2

type rawWriter = ioutil.SegmentedWriter[SectionType, *SectionType]
type sectionReader = ioutil.Segment[SectionType, *SectionType]
type sectionWriter = ioutil.SegmentWriter[SectionType, *SectionType]

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
	v := new(RecordEntry)
	_, err := readValue(r.rd, v)
	return v, err
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

func (w *Writer) OpenRaw(typ SectionType) (*sectionWriter, error) {
	if typ != SectionTypeHeader && !w.wroteHeader {
		return nil, errors.NotReady.WithFormat("header has not been written")
	}
	w.sections++
	w.wroteHeader = true
	return w.wr.Open(typ)
}
