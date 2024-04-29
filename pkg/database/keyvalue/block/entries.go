// Copyright 2024 The Accumulate Authors
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
