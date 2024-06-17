// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/edsrzf/mmap-go"
)

const fileHeaderSize = 1024

type blockFile struct {
	mu     *sync.RWMutex
	file   *os.File
	data   mmap.MMap
	header *fileHeader
}

func newFile(ordinal uint64, name string) (*blockFile, error) {
	f := new(blockFile)
	f.mu = new(sync.RWMutex)
	f.header = new(fileHeader)
	f.header.Ordinal = ordinal

	var err error
	f.file, err = os.Create(name)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func openFile(name string) (*blockFile, error) {
	f := new(blockFile)
	f.mu = new(sync.RWMutex)
	f.header = new(fileHeader)

	var err error
	f.file, err = os.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}

	st, err := f.file.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() < fileHeaderSize {
		return nil, errors.New("file header is missing or corrupted")
	}

	f.data, err = mmap.MapRegion(f.file, int(st.Size()), mmap.RDWR, 0, 0)
	if err != nil {
		return nil, err
	}

	err = f.header.UnmarshalBinary(f.data[:1024])
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	return f, nil
}

func (f *blockFile) Len() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.data)
}

func (f *blockFile) ReadAt(b []byte, off int64) (int, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if int(off) >= len(f.data) {
		return 0, io.EOF
	}

	n := copy(b, f.data[off:])
	if n < len(b) {
		return n, io.EOF
	}
	return n, nil
}

func (f *blockFile) Remap() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	var err error
	if f.data != nil {
		err = f.data.Unmap()
		f.data = nil
	}
	if err != nil {
		return err
	}

	f.data, err = mmap.Map(f.file, mmap.RDWR, 0)
	return err
}

func (f *blockFile) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	var errs []error
	if f.data != nil {
		errs = append(errs, f.data.Unmap())
	}
	errs = append(errs, f.file.Close())

	f.data = nil
	f.file = nil
	return errors.Join(errs...)
}
