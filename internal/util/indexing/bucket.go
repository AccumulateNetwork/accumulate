// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/edsrzf/mmap-go"
)

const pageSize = 1 << 10 // 1024 entries

// Bucket is a collection of 256 files.
type Bucket struct {
	files     [256]*bucketFile
	valueSize int
}

func OpenBucket(prefix string, valueSize int, create bool) (*Bucket, error) {
	// Ensure the directory exists
	dir := prefix
	if !strings.HasSuffix(dir, "/") {
		dir = filepath.Dir(dir)
	}
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, err
	}

	b := new(Bucket)
	b.valueSize = valueSize

	// Create a file for every byte prefix
	for i := 0; i < 256; i++ {
		name := fmt.Sprintf("%s%02x", prefix, i)

		var file *os.File
		if create {
			file, err = os.Create(name)
		} else {
			file, err = os.Open(name)
		}
		if err != nil {
			return nil, err
		}
		b.files[i] = &bucketFile{file: file}
	}

	return b, nil
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
	copy(dst, hash[:])
	copy(dst[32:], value)
	f.count++
	return nil
}

func (b *Bucket) Close() error {
	var err error
	for _, f := range b.files {
		if f.mmap != nil {
			e := f.mmap.Unmap()
			if e != nil {
				err = e
			}
		}

		// Truncate extra space
		e := f.file.Truncate(int64(f.count * (32 + b.valueSize)))
		if e != nil {
			err = e
		}

		e = f.file.Close()
		if e != nil {
			err = e
		}
	}
	return err
}

type Entry struct {
	Hash  [32]byte
	Value []byte
}

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
		entries[i].Hash = *(*[32]byte)(g[i*S:])
		if b.valueSize > 0 {
			entries[i].Value = make([]byte, b.valueSize)
			copy(entries[i].Value, g[i*S+32:])
		}
	}
	return entries, nil
}

type bucketFile struct {
	file  *os.File
	mmap  mmap.MMap
	count int
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
