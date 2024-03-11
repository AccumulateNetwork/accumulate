// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"os"

	"github.com/edsrzf/mmap-go"
)

type File struct {
	file        *os.File
	mmap        mmap.MMap
	valueSize   int
	writeOffset int64
	didCreate   bool
}

func OpenFile(filepath string, valueSize int, create bool) (*File, error) {
	var file *os.File
	var err error
	if create {
		file, err = os.Create(filepath)
	} else {
		file, err = os.Open(filepath)
	}
	if err != nil {
		return nil, err
	}
	return &File{file: file, valueSize: valueSize, didCreate: create}, nil
}
