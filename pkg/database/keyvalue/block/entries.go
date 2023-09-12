// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"encoding/binary"
	"io"
	"math"
)

func readEntry(rd io.Reader) (entry, int, error) {
	var lenBuf [2]byte
	n, err := io.ReadFull(rd, lenBuf[:])
	if err != nil {
		return nil, n, err
	}

	len := binary.BigEndian.Uint16(lenBuf[:])
	b := make([]byte, len)
	m, err := io.ReadFull(rd, b)
	if err != nil {
		return nil, n + m, err
	}

	e, err := unmarshalEntry(b)
	return e, n + m, err
}

func readEntryMmap(mmap []byte, offset int64) (entry, int, error) {
	if int64(len(mmap)) <= offset {
		return nil, 0, io.EOF
	}
	if int64(len(mmap)) < offset+2 {
		return nil, 0, io.ErrUnexpectedEOF
	}

	b := make([]byte, binary.BigEndian.Uint16(mmap[offset:offset+2]))
	n := copy(b, mmap[offset+2:])
	if n < len(b) {
		return nil, n + 2, io.ErrUnexpectedEOF
	}

	e, err := unmarshalEntry(b)
	return e, n + 2, err
}

func readEntryAt(rd io.ReaderAt, offset int64) (entry, int, error) {
	var lenBuf [2]byte
	n, err := rd.ReadAt(lenBuf[:], offset)
	if err != nil {
		return nil, n, err
	}

	len := binary.BigEndian.Uint16(lenBuf[:])
	b := make([]byte, len)
	m, err := rd.ReadAt(b, offset+4)
	if err != nil {
		return nil, n + m, err
	}

	e, err := unmarshalEntry(b)
	return e, n + m, err
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
