// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"encoding/binary"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

const align = 64

type Writer struct {
	Logger      logging.OptionalLogger
	file        io.WriteSeeker
	openSection bool
	prevSection int64
}

type SectionWriter struct {
	typ     SectionType
	offset  int64
	file    *Writer
	section io.WriteSeeker
}

func NewWriter(w io.WriteSeeker) *Writer {
	return &Writer{file: w}
}

func (w *SectionWriter) Write(p []byte) (n int, err error) {
	return w.section.Write(p)
}

func (w *SectionWriter) Seek(offset int64, whence int) (int64, error) {
	return w.section.Seek(offset, whence)
}

// Open opens a section.
func (w *Writer) Open(typ SectionType) (*SectionWriter, error) {
	if w.openSection {
		return nil, errors.BadRequest.WithFormat("previous section has not been closed")
	}

	// Get the current offset
	offset, err := w.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get file offset: %w", err)
	}

	// Update the previous section's header
	if offset > 0 {
		// Seek to the header
		_, err = w.file.Seek(w.prevSection+16, io.SeekStart)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("set file offset: %w", err)
		}

		var headerPart [8]byte
		binary.BigEndian.PutUint64(headerPart[:], uint64(offset))
		_, err = w.file.Write(headerPart[:])
		if err != nil {
			return nil, errors.UnknownError.WithFormat("read section header: %w", err)
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

	// Create a section reader
	section, err := ioutil2.NewSectionWriter(w.file, offset+64, -1)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("create section writer: %w", err)
	}

	w.openSection = true
	return &SectionWriter{typ, offset, w, section}, nil
}

func (w *SectionWriter) Close() error {
	return w.file.closeSection(w)
}

func (w *Writer) closeSection(s *SectionWriter) error {
	// Get current offset
	current, err := w.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return errors.UnknownError.WithFormat("get file offset: %w", err)
	}

	// Seek to the header
	_, err = w.file.Seek(s.offset, io.SeekStart)
	if err != nil {
		return errors.UnknownError.WithFormat("seek to section header: %w", err)
	}

	// Write the section header
	var header [64]byte
	binary.BigEndian.PutUint16(header[0:], uint16(s.typ))               // Type
	binary.BigEndian.PutUint64(header[8:], uint64(current-s.offset-64)) // Size
	_, err = w.file.Write(header[:])
	if err != nil {
		return errors.UnknownError.WithFormat("write section header: %w", err)
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
			return errors.UnknownError.WithFormat("pad end of section: %w", err)
		}
	}

	w.openSection = false
	w.prevSection = s.offset
	return nil
}
