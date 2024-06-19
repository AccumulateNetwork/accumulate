// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"errors"
	"io"
	"os"
	"sync"

	"github.com/edsrzf/mmap-go"
)

type file struct {
	mu   *sync.RWMutex
	file *os.File
	data mmap.MMap
}

func openFile(name string, flags int) (_ *file, err error) {
	f := new(file)
	f.mu = new(sync.RWMutex)
	defer closeIfError(&err, f)

	f.file, err = os.OpenFile(name, flags, 0600)
	if err != nil {
		return nil, err
	}

	st, err := f.file.Stat()
	if err != nil {
		return nil, err
	}

	if st.Size() == 0 {
		return f, nil
	}

	f.data, err = mmap.MapRegion(f.file, int(st.Size()), mmap.RDWR, 0, 0)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func (f *file) Name() string {
	return f.file.Name()
}

func (f *file) Len() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.data)
}

func (f *file) ReadAt(b []byte, off int64) (int, error) {
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

func (f *file) ReadRange(start, end int64) *fileReader {
	return &fileReader{f, start, end}
}

func (f *file) WriteRange(start, end int64) *fileWriter {
	return &fileWriter{f, start, end}
}

func (f *file) WriteAt(b []byte, offset int64) (int, error) {
	err := f.Grow(offset + int64(len(b)))
	if err != nil {
		return 0, err
	}

	f.mu.RLock()
	defer f.mu.RUnlock()
	m := copy(f.data[offset:], b)
	return m, nil
}

func (f *file) Grow(size int64) error {
	// Fast path
	f.mu.RLock()
	if len(f.data) >= int(size) {
		f.mu.RUnlock()
		return nil
	}

	// Upgrade to exclusive lock
	f.mu.RUnlock()
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.data) >= int(size) {
		return nil
	}

	if f.data != nil {
		err := f.data.Unmap()
		if err != nil {
			return err
		}
	}

	err := f.file.Truncate(size)
	if err != nil {
		return err
	}

	f.data, err = mmap.MapRegion(f.file, int(size), mmap.RDWR, 0, 0)
	return err
}

func (f *file) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	var errs []error
	if f.data != nil {
		errs = append(errs, f.data.Unmap())
	}
	if f.file != nil {
		errs = append(errs, f.file.Close())
	}

	f.data = nil
	f.file = nil
	return errors.Join(errs...)
}
