// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !no_mmap

package indexing

import (
	"errors"

	"github.com/edsrzf/mmap-go"
	"golang.org/x/exp/slog"
)

func (b *Bucket) Read(i byte) ([]Entry, error) {
	// Check if the file is empty
	f := b.files[i]
	st, err := f.file.Stat()
	if err != nil || st.Size() == 0 {
		return nil, err
	}

	S := 32 + b.valueSize

	var g mmap.MMap
	if f.mmap == nil {
		// Map the file
		g, err = mmap.Map(f.file, mmap.RDONLY, 0)
		if err != nil {
			return nil, err
		}
		defer func() { _ = g.Unmap() }()

		if len(g)%S != 0 {
			return nil, errors.New("internal error: invalid file length")
		}

	} else {
		// Or use the existing mapping
		g = f.mmap[:f.count*S]
	}

	entries := make([]Entry, len(g)/S)
	for i := range entries {
		entries[i].fromBytes(g[i*S:], S, b.valueSize)
	}
	return entries, nil
}

func (b *Bucket) Write(hash [32]byte, value []byte) error {
	if len(value) != b.valueSize {
		return errors.New("value does not match expected size")
	}

	// Map or remap the file
	S := 32 + b.valueSize
	f := b.files[hash[0]]
	err := f.ensureSpace(S)
	if err != nil {
		return err
	}

	// Write
	dst := f.mmap[f.count*S:]
	Entry{hash, value}.toBytes(dst)
	f.count++
	return nil
}

// ensureSpace ensures there is sufficient space for another record.
func (f *bucketFile) ensureSpace(recordSize int) error {
	if len(f.mmap) >= (f.count+1)*recordSize {
		return nil
	}

	// Need more space, remap
	newSize := len(f.mmap) + pageSize*recordSize
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

func (f *bucketFile) close(valueSize int) error {
	var err error
	if f.mmap != nil {
		e := f.mmap.Unmap()
		if e != nil {
			slog.Error("Error unmapping bucket", "error", e)
			err = e
		}
	}

	// Truncate extra space
	e := f.file.Truncate(int64(f.count * (32 + valueSize)))
	if e != nil {
		slog.Error("Error truncating bucket", "error", e)
		err = e
	}

	e = f.file.Close()
	if e != nil {
		slog.Error("Error closing bucket", "error", e)
		err = e
	}
	return err
}
