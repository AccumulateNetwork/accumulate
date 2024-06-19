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

	defer func() {
		if err != nil {
			_ = f.Close()
		}
	}()

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

func (f *file) WriteAt(offset int64, b []byte) (int, error) {
	err := f.unmap()
	if err != nil {
		return 0, err
	}

	n := offset + int64(len(b))
	err = f.file.Truncate(n)
	if err != nil {
		return 0, err
	}

	f.data, err = mmap.MapRegion(f.file, int(n), mmap.RDWR, 0, 0)
	if err != nil {
		return 0, err
	}

	m := copy(f.data[offset:], b)
	return m, nil
}

func (f *file) unmap() error {
	if f.data == nil {
		return nil
	}

	err := f.data.Unmap()
	f.data = nil
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
