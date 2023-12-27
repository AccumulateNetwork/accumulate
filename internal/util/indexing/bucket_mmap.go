// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !no_mmap

package indexing

import (
	"errors"
	"io"

	"github.com/edsrzf/mmap-go"
	"golang.org/x/exp/slog"
)

const pageSize = 4 << 10 // 4 kiB

func (b *Bucket) Read(i byte) ([]Entry, error) {
	// Check if the file is empty
	f := b.files[i]
	st, err := f.file.Stat()
	if err != nil || st.Size() == 0 {
		return nil, err
	}

	g, err := f.mapped()
	if err != nil {
		return nil, err
	}

	S := 32 + b.valueSize
	entries := make([]Entry, len(g)/S)
	for i := range entries {
		entries[i].fromBytes(g[i*S:], S, b.valueSize)
	}
	return entries, nil
}

func (f *File) mapped() (mmap.MMap, error) {
	if f.mmap != nil {
		return f.mmap[:f.writeOffset], nil
	}

	// Map the file
	var err error
	f.mmap, err = mmap.Map(f.file, mmap.RDONLY, 0)
	if err != nil {
		return nil, err
	}

	if f.valueSize > 0 && len(f.mmap)%f.valueSize != 0 {
		return nil, errors.New("internal error: invalid file length")
	}

	f.writeOffset = int64(len(f.mmap))
	return f.mmap, nil
}

func (f *File) ReadAt(buf []byte, off int64) (int, error) {
	g, err := f.mapped()
	if err != nil {
		return 0, err
	}
	if off >= int64(len(g)) {
		return 0, io.EOF
	}
	n := copy(buf, g[off:])
	return n, nil
}

func (b *Bucket) Write(hash [32]byte, value []byte) error {
	// Get the buffer to write to
	dst, err := b.files[hash[0]].write(32 + b.valueSize)
	if err != nil {
		return err
	}

	// Write
	Entry{hash, value}.toBytes(dst)
	return nil
}

func (f *File) Write(data []byte) (int, error) {
	dst, err := f.write(len(data))
	if err != nil {
		return 0, err
	}
	return copy(dst, data), nil
}

func (f *File) write(n int) ([]byte, error) {
	if f.valueSize > 0 && n != f.valueSize {
		return nil, errors.New("value does not match expected size")
	}

	// Map or remap the file
	err := f.ensureSpace(n)
	if err != nil {
		return nil, err
	}

	// Get buffer for writing
	dst := f.mmap[f.writeOffset:]
	f.writeOffset += int64(n)
	return dst, nil
}

// ensureSpace ensures there is sufficient space for another record.
func (f *File) ensureSpace(n int) error {
	if len(f.mmap) >= int(f.writeOffset)+n {
		return nil
	}

	// Need more space, remap
	newSize := len(f.mmap) + pageSize
	if f.mmap != nil {
		err := f.mmap.Unmap()
		if err != nil {
			return err
		}
	}

	err := f.file.Truncate(int64(newSize))
	if err != nil {
		return err
	}

	f.mmap, err = mmap.MapRegion(f.file, newSize, mmap.RDWR, 0, 0)
	return err
}

func (f *File) Close() error {
	var err error
	if f.mmap != nil {
		e := f.mmap.Unmap()
		if e != nil {
			slog.Error("Error unmapping bucket", "error", e)
			err = e
		}
	}

	// Truncate extra space
	if f.didCreate {
		e := f.file.Truncate(f.writeOffset)
		if e != nil {
			slog.Error("Error truncating bucket", "error", e)
			err = e
		}
	}

	e := f.file.Close()
	if e != nil {
		slog.Error("Error closing bucket", "error", e)
		err = e
	}
	return err
}
