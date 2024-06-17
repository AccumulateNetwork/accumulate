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

	binary2 "gitlab.com/accumulatenetwork/core/schema/pkg/binary"
)

func readEntryAt(rd io.ReaderAt, offset int64, buf *buffer, dec *binary2.Decoder) (entry, int, error) {
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

	l := int(binary.BigEndian.Uint16(lb[:]))
	err = buf.load(rd, l, offset+2)
	n += l
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, n, io.EOF
		}
		return nil, n, err
	}

	dec.Reset(buf)
	e, err := unmarshalEntryBinaryV2(dec)
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

type buffer struct {
	bytes  []byte
	offset int
}

func (b *buffer) load(rd io.ReaderAt, n int, offset int64) error {
	b.bytes = b.bytes[:cap(b.bytes)]
	b.offset = 0

	if n <= len(b.bytes) {
		b.bytes = b.bytes[:n]

	} else if n <= 64 {
		b.bytes = make([]byte, n, 64)

	} else if n < 2*len(b.bytes) {
		b.bytes = append(b.bytes, make([]byte, len(b.bytes))...)
		b.bytes = b.bytes[:n]

	} else {
		b.bytes = append(b.bytes, make([]byte, n-len(b.bytes))...)
		b.bytes = b.bytes[:n]
	}

	_, err := rd.ReadAt(b.bytes, offset)
	return err
}

func (b *buffer) reset() {
	b.bytes = b.bytes[:0]
	b.offset = 0
}

func (b *buffer) Read(c []byte) (int, error) {
	if len(b.bytes) <= b.offset {
		b.reset()
		if len(c) == 0 {
			return 0, nil
		}
		return 0, io.EOF
	}
	n := copy(c, b.bytes[b.offset:])
	b.offset += n
	return n, nil
}

func (b *buffer) ReadByte() (byte, error) {
	if len(b.bytes) <= b.offset {
		b.reset()
		return 0, io.EOF
	}
	c := b.bytes[b.offset]
	b.offset++
	return c, nil
}
