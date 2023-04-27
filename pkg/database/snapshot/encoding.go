// Copyright 2023 The Accumulate Authors
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
)

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
		return nil, int64(n), errors.EncodingError.WithFormat("read length: %w", err)
	}
	l := binary.BigEndian.Uint64(v[:])

	// Read the data
	b := make([]byte, l)
	m, err := io.ReadFull(rd, b)
	n += m
	if err != nil {
		return nil, int64(n), errors.EncodingError.WithFormat("read data: %w", err)
	}

	return b, int64(n), nil
}

// readValue unmarshals a length-prefixed record into the value.
func readValue(rd io.Reader, v encoding.BinaryValue) (int64, error) {
	// Read the record
	b, n, err := readRecord(rd)
	if err != nil {
		return n, errors.UnknownError.Wrap(err)
	}

	// Unmarshal the header
	err = v.UnmarshalBinary(b)
	if err != nil {
		return n, errors.EncodingError.WithFormat("unmarshal: %w", err)
	}

	return n, nil
}
