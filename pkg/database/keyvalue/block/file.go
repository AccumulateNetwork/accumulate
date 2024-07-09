// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"sync"

	"github.com/edsrzf/mmap-go"
)

type blockFile struct {
	number int
	mu     *sync.RWMutex
	file   *os.File
	data   mmap.MMap
}

func openFile(number int, name string, flag int, perm fs.FileMode) (*blockFile, error) {
	f := new(blockFile)
	f.number = number
	f.mu = new(sync.RWMutex)

	var err error
	f.file, err = os.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}

	if flag&os.O_CREATE == 0 {
		f.data, err = mmap.Map(f.file, mmap.RDWR, 0)
		if err != nil {
			return nil, err
		}
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
