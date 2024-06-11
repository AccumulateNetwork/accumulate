// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
)

func readEntryAt(rd io.ReaderAt, offset int64) (entry, int, error) {
	var lb [2]byte
	n, err := rd.ReadAt(lb[:], offset)
	switch {
	case err == nil:
		// Ok
	case !errors.Is(err, io.EOF):
		return nil, n, err
	case n == 0:
		return nil, n, io.EOF
	default:
		return nil, n, io.ErrUnexpectedEOF
	}

	b := make([]byte, binary.BigEndian.Uint16(lb[:]))
	m, err := rd.ReadAt(b, offset+2)
	n += m
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, n, io.EOF
		}
		return nil, n, err
	}

	e, err := unmarshalEntry(b)
	return e, n, err
}

func writeEntry(wr io.Writer, e entry) (int, error) {
	b, err := e.MarshalBinary()
	if err != nil {
		return 0, err
	}
	if len(b) > math.MaxUint16 {
		panic("entry is larger than 64 KiB")
	}

	var lenBuf [2]byte
	binary.BigEndian.PutUint16(lenBuf[:], uint16(len(b)))
	n, err := wr.Write(lenBuf[:])
	if err != nil {
		return n, err
	}

	m, err := wr.Write(b)
	return n + m, err
}
