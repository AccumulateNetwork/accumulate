// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import "io"

var _ io.ReaderAt = (*fileReader)(nil)
var _ io.WriterAt = (*fileWriter)(nil)

type fileReader struct {
	*file
	offset int64
	end    int64
}

func (r *fileReader) Len() int {
	if r.end > 0 {
		return int(r.end - r.offset)
	}
	return len(r.data) - int(r.offset)
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

func (r *fileReader) ReadByte() (byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Len() <= 0 || len(r.data) <= int(r.offset) {
		return 0, io.EOF
	}
	b := r.data[r.offset]
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

type fileWriter struct {
	*file
	offset int64
	end    int64
}

func (r *fileWriter) Len() int {
	if r.end > 0 {
		return int(r.end - r.offset)
	}
	return len(r.data) - int(r.offset)
}

func (r *fileWriter) Write(b []byte) (int, error) {
	if n := r.Len(); n == 0 {
		return 0, io.EOF
	} else if len(b) > n {
		b = b[:n]
	}
	n, err := r.WriteAt(b, r.offset)
	r.offset += int64(n)
	return n, err
}
