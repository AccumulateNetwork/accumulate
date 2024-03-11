// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"encoding/binary"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func (h *Header) readFrom(rd io.Reader) (int64, error) {
	// Read the header bytes
	b, n, err := readRecord(rd)
	if err != nil {
		return n, errors.UnknownError.Wrap(err)
	}

	// Version check
	vh := new(versionHeader)
	err = vh.UnmarshalBinary(b)
	if err != nil {
		return n, errors.EncodingError.WithFormat("unmarshal: %w", err)
	}
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

// writeValue marshals the value and writes the result prefixed with its length.
func writeValue(wr io.Writer, v encoding.BinaryValue) (int64, error) {
	// Marshal the value
	data, err := v.MarshalBinary()
	if err != nil {
		return 0, errors.EncodingError.WithFormat("marshal: %w", err)
	}

	// Write the length
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(len(data)))
	n, err := wr.Write(b[:])
	if err != nil {
		return int64(n), errors.EncodingError.WithFormat("write length: %w", err)
	}

	// Write the record
	m, err := wr.Write(data)
	if err != nil {
		return int64(n + m), errors.EncodingError.WithFormat("write data: %w", err)
	}

	return int64(n + m), nil
}

// readRecord reads a length-prefixed record
func readRecord(rd io.Reader) ([]byte, int64, error) {
	// Read the length
	var v [8]byte
	n, err := io.ReadFull(rd, v[:])
	if err != nil {
		return nil, int64(n), err
	}
	l := binary.BigEndian.Uint64(v[:])

	// Read the data
	b := make([]byte, l)
	m, err := io.ReadFull(rd, b)
	n += m
	if err != nil {
		return nil, int64(n), err
	}

	return b, int64(n), nil
}

// readRecordAt reads a length-prefixed record
func readRecordAt(rd io.ReaderAt, off int64) ([]byte, int64, error) {
	// Read the length
	var v [8]byte
	n, err := readFullAt(rd, off, v[:])
	if err != nil {
		return nil, int64(n), err
	}
	l := binary.BigEndian.Uint64(v[:])

	// Read the data
	b := make([]byte, l)
	m, err := readFullAt(rd, off+int64(len(v)), b)
	n += m
	if err != nil {
		return nil, int64(n), err
	}

	return b, int64(n), nil
}

func readFullAt(rd io.ReaderAt, off int64, b []byte) (int, error) {
	var m int
	for len(b) > 0 {
		n, err := rd.ReadAt(b, off)
		m += n
		if err != nil {
			return m, err
		}
		if n == 0 {
			return m, errors.InternalError.With("read nothing")
		}
		b = b[n:]
	}
	return m, nil
}

// readValue unmarshals a length-prefixed record into the value.
func readValue(rd io.Reader, v encoding.BinaryValue) (int64, error) {
	// Read the record
	b, n, err := readRecord(rd)
	if err != nil {
		return n, err
	}

	// Unmarshal the header
	err = v.UnmarshalBinary(b)
	if err != nil {
		return n, errors.EncodingError.WithFormat("unmarshal: %w", err)
	}

	return n, nil
}

// readValueAt unmarshals a length-prefixed record into the value.
func readValueAt(rd io.ReaderAt, off int64, v encoding.BinaryValue) (int64, error) {
	// Read the record
	b, n, err := readRecordAt(rd, off)
	if err != nil {
		return n, err
	}

	// Unmarshal the header
	err = v.UnmarshalBinary(b)
	if err != nil {
		return n, errors.EncodingError.WithFormat("unmarshal: %w", err)
	}

	return n, nil
}
