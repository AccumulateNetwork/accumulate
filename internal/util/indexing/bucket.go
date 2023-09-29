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

// Bucket is a collection of 256 files.
type Bucket struct {
	files map[byte]*os.File
}

func OpenBucket(prefix string, create bool) (*Bucket, error) {
	// Ensure the directory exists
	dir := prefix
	if !strings.HasSuffix(dir, "/") {
		dir = filepath.Dir(dir)
	}
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, err
	}

	// Create a file for every byte prefix
	b := new(Bucket)
	b.files = make(map[byte]*os.File, 256)
	for i := 0; i < 256; i++ {
		var file *os.File
		name := fmt.Sprintf("%s%02x", prefix, i)
		if create {
			file, err = os.Create(name)
		} else {
			file, err = os.Open(name)
		}
		if err != nil {
			return nil, err
		}
		b.files[byte(i)] = file
	}

	return b, nil
}

func (b *Bucket) Iterate(window int) *BucketIterator {
	return &BucketIterator{
		bucket: b,
		values: make([][32]byte, window),
	}
}

func (b *Bucket) Write(hash [32]byte) error {
	_, err := b.files[hash[0]].Write(hash[:])
	return err
}

func (b *Bucket) Close() error {
	var err error
	for _, f := range b.files {
		e := f.Close()
		if e != nil {
			err = e
		}
	}
	return err
}

func (b *Bucket) Read(i byte) ([][32]byte, error) {
	f := b.files[i]
	st, err := f.Stat()
	if err != nil || st.Size() == 0 {
		return nil, err
	}

	g, err := mmap.Map(f, mmap.RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer g.Unmap()

	if len(g)%32 != 0 {
		return nil, errors.New("internal error: invalid file length")
	}

	hashes := make([][32]byte, len(g)/32)
	for i := range hashes {
		hashes[i] = *(*[32]byte)(g[i*32:])
	}
	return hashes, nil
}

type BucketIterator struct {
	bucket *Bucket
	values [][32]byte
	file   mmap.MMap
	err    error
	index  int
	read   int
	count  int
}

func (it *BucketIterator) Value() [][32]byte {
	return it.values[:it.count]
}

func (it *BucketIterator) Close() error {
	if it.err != nil {
		return it.err
	}
	if it.file != nil {
		return it.file.Unmap()
	}
	return nil
}

func (it *BucketIterator) Next() bool {
again:
	if it.err != nil || it.index > 0xFF {
		return false
	}

	// Open the next file
	if it.file == nil {
		f := it.bucket.files[byte(it.index)]
		st, err := f.Stat()
		if err != nil {
			it.err = err
			return false
		}
		if st.Size() == 0 {
			it.index++
			goto again
		}

		it.file, it.err = mmap.Map(f, mmap.RDONLY, 0)
		if it.err != nil {
			return false
		}
		if len(it.file)%32 != 0 {
			it.file.Unmap()
			it.err = errors.New("internal error: invalid file length")
			return false
		}
	}

	// Copy the hashes
	total := len(it.file) / 32
	it.count = total - it.read
	if it.count > len(it.values) {
		it.count = len(it.values)
	}
	for i := 0; i < it.count; i++ {
		it.values[i] = *(*[32]byte)(it.file[(it.read+i)*32:])
	}

	it.read += it.count
	if it.read < total {
		return true
	}

	// Done with this file
	it.read = 0
	it.index++
	it.err = it.file.Unmap()
	it.file = nil
	return true
}
