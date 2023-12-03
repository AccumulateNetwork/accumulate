// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package ioutil

import (
	"io"
	"io/fs"
)

// Buffer is an [io.ReadWriteSeeker] and [io.ReaderAt] backed by a byte array.
type Buffer struct {
	buf []byte
	pos int
}

var _ io.WriteSeeker = (*Buffer)(nil)
var _ SectionReader = (*Buffer)(nil)

func NewBuffer(b []byte) *Buffer {
	return &Buffer{buf: b}
}

func (b *Buffer) Bytes() []byte {
	return b.buf
}

func (b *Buffer) Read(v []byte) (int, error) {
	if b.pos >= len(b.buf) {
		return 0, io.EOF
	}
	n := copy(v, b.buf[b.pos:])
	b.pos += n
	return n, nil
}

func (b *Buffer) ReadAt(v []byte, off int64) (int, error) {
	if off < 0 || off > int64(len(b.buf)) {
		return 0, fs.ErrInvalid
	}
	return copy(v, b.buf[off:]), nil
}

func (b *Buffer) Write(v []byte) (int, error) {
	if len(b.buf) < b.pos+len(v) {
		b.buf = append(b.buf, make([]byte, b.pos+len(v)-len(b.buf))...)
	}
	copy(b.buf[b.pos:], v)
	b.pos += len(v)
	return len(v), nil
}

func (b *Buffer) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		// Ok
	case io.SeekCurrent:
		offset += int64(b.pos)
	case io.SeekEnd:
		offset += int64(len(b.buf))
	default:
		return 0, fs.ErrInvalid
	}

	if offset < 0 || offset > int64(len(b.buf)) {
		return 0, fs.ErrInvalid
	}

	b.pos = int(offset)
	return int64(b.pos), nil
}
