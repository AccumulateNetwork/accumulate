package ioutil2

import (
	"io"
	"io/fs"
)

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
