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
)

type file struct {
	file *os.File
	mmap mmapRef
}

func openFile(name string, flags int) (_ *file, err error) {
	f := new(file)
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

	err = f.mmap.Map(f.file, int(st.Size()))
	if err != nil {
		return nil, err
	}

	return f, nil
}

func (f *file) Name() string {
	return f.file.Name()
}

func (f *file) ReadAt(b []byte, off int64) (n int, err error) {
	m := f.mmap.Acquire()
	defer doRelease(&err, m)

	if int(off) >= len(m.data) {
		return 0, io.EOF
	}

	n = copy(b, m.data[off:])
	if n < len(b) {
		return n, io.EOF
	}
	return n, nil
}

func (f *file) ReadRange(start, end int64) *fileReader {
	return &fileReader{f, start, end}
}

func (f *file) WriteAt(offset int64, b []byte) (n int, err error) {
	end := offset + int64(len(b))
	for {
		m := f.mmap.Acquire()
		defer doRelease(&err, m)

		if int(end) <= len(m.data) {
			n := copy(m.data[offset:], b)
			return n, nil
		}

		err := f.file.Truncate(end)
		if err != nil {
			return 0, err
		}

		err = f.mmap.Map(f.file, int(end))
		if err != nil {
			return 0, err
		}
	}
}

func (f *file) Close() error {
	return errors.Join(
		f.mmap.Close(),
		f.file.Close())
}
