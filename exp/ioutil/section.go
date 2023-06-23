// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package ioutil

import (
	"errors"
	"io"
)

type SectionReader interface {
	io.Reader
	io.ReaderAt
	io.Seeker
}

func NewSectionReader(rd SectionReader, start, end int64) (*io.SectionReader, error) {
	var err error
	if start < 0 {
		start, err = rd.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, err
		}
	}
	if end < 0 {
		end, err = rd.Seek(0, io.SeekEnd)
		if err != nil {
			return nil, err
		}
	}
	return io.NewSectionReader(rd, start, end-start), nil
}

type SectionWriter struct {
	wr io.WriteSeeker

	start, offset, end int64
}

func NewSectionWriter(wr io.WriteSeeker, start, end int64) (*SectionWriter, error) {
	offset, err := wr.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	if start < 0 {
		start = offset
	}
	if offset < start || (end >= 0 && offset >= end) {
		offset, err = wr.Seek(start, io.SeekStart)
		if err != nil {
			return nil, err
		}
	}
	return &SectionWriter{wr, start, offset, end}, nil
}

func (s *SectionWriter) Write(p []byte) (n int, err error) {
	if s.end >= 0 && s.offset+int64(len(p)) > s.end {
		return 0, errors.New("attempted to write past the end of the section")
	}
	n, err = s.wr.Write(p)
	if err != nil {
		return 0, err
	}
	s.offset += int64(n)
	return n, nil
}

func (s *SectionWriter) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		offset += s.start
	case io.SeekCurrent:
		offset += s.offset
	case io.SeekEnd:
		end := s.end
		if end < 0 {
			// If the user did not define the end, find it
			var err error
			end, err = s.wr.Seek(0, io.SeekEnd)
			if err != nil {
				return 0, err
			}
		}

		offset += end
	default:
		return 0, errors.New("invalid whence")
	}

	if offset < s.start {
		return 0, errors.New("attempted to seek past the start of the section")
	}
	if s.end >= 0 && offset >= s.end {
		return 0, io.EOF
	}

	offset, err := s.wr.Seek(offset, io.SeekStart)
	if err != nil {
		return 0, err
	}

	s.offset = offset
	return offset - s.start, nil
}
