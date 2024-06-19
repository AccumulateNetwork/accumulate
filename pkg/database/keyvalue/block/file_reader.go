// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"io"
)

type fileReader struct {
	*file
	offset int64
	end    int64
}

func (r *fileReader) Len() int {
	return int(r.end - r.offset)
}

func (r *fileReader) Read(b []byte) (int, error) {
	if n := r.Len(); n == 0 {
		return 0, io.EOF
	} else if len(b) > n {
		b = b[:n]
	}
	n, err := r.ReadAt(b, r.offset)
	r.offset += int64(n)
	return n, err
}

func (r *fileReader) ReadByte() (c byte, err error) {
	if r.Len() == 0 {
		return 0, io.EOF
	}

	m := r.mmap.Acquire()
	defer doRelease(&err, m)

	if int(r.offset) >= len(m.data) {
		return 0, io.EOF
	}
	b := m.data[r.offset]
	r.offset++
	return b, nil
}

func (r *fileReader) UnreadByte() error {
	if r.offset <= 0 {
		return io.EOF
	}
	r.offset--
	return nil
}
