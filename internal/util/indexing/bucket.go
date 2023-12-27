// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const BucketCount = 256

// Bucket is a collection of 256 files.
type Bucket struct {
	files     [BucketCount]*File
	valueSize int
}

type Entry struct {
	Hash  [32]byte
	Value []byte
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
	for i := 0; i < BucketCount; i++ {
		name := fmt.Sprintf("%s%02x", prefix, i)
		b.files[i], err = OpenFile(name, valueSize+32, create)
		if err != nil {
			return nil, err
		}
	}

	return b, nil
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

func (e *Entry) fromBytes(b []byte, S, valueSize int) {
	e.Hash = *(*[32]byte)(b[:32])
	if valueSize > 0 {
		e.Value = make([]byte, valueSize)
		copy(e.Value, b[32:])
	}
}

func (e Entry) toBytes(b []byte) {
	copy(b, e.Hash[:])
	copy(b[32:], e.Value)
}
