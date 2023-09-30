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
	files     map[byte]*os.File
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

func (b *Bucket) Write(hash [32]byte, value []byte) error {
	if len(value) != b.valueSize {
		return errors.New("value does not match expected size")
	}
	value = append(hash[:], value...)
	_, err := b.files[hash[0]].Write(value)
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

type Entry struct {
	Hash  [32]byte
	Value []byte
}

func (b *Bucket) Read(i byte) ([]Entry, error) {
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

	S := 32 + b.valueSize
	if len(g)%S != 0 {
		return nil, errors.New("internal error: invalid file length")
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
