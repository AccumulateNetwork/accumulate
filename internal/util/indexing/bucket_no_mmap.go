// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build no_mmap

package indexing

import (
	"errors"
	"io"

	"golang.org/x/exp/slog"
)

// This file exists entirely because of
// https://github.com/rr-debugger/rr/issues/3635

func (b *Bucket) Read(i byte) ([]Entry, error) {
	f := b.files[i]
	st, err := f.file.Stat()
	if err != nil {
		return nil, err
	}

	_, err = f.file.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}

	S := 32 + b.valueSize
	if st.Size()%int64(S) != 0 {
		return nil, errors.New("internal error: invalid file length")
	}

	buf := make([]byte, S)
	entries := make([]Entry, st.Size()/int64(S))
	for i := range entries {
		_, err := io.ReadFull(f.file, buf)
		if err != nil {
			return nil, err
		}
		entries[i].fromBytes(buf, S, b.valueSize)
	}
	return entries, nil
}

func (f *File) ReadAt(buf []byte, off int64) (int, error) {
	return f.file.ReadAt(buf, off)
}

func (b *Bucket) Write(hash [32]byte, value []byte) error {
	if len(value) != b.valueSize {
		return errors.New("value does not match expected size")
	}

	S := 32 + b.valueSize
	buf := make([]byte, S)
	Entry{hash, value}.toBytes(buf)

	n, err := b.files[hash[0]].file.Write(buf)
	_ = n
	return err
}

func (f *File) Close() error {
	err := f.file.Close()
	if err != nil {
		slog.Error("Error closing bucket", "error", err)
	}
	return err
}

func (f *File) Write(data []byte) (int, error) {
	return f.file.Write(data)
}
