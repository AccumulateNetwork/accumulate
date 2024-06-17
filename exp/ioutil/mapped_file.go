// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package ioutil

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"sync"
	"sync/atomic"

	"github.com/edsrzf/mmap-go"
	"gitlab.com/accumulatenetwork/core/schema/pkg/binary"
)

type MappedFile struct {
	mu   *sync.RWMutex
	file *os.File
	data mmap.MMap
	pool *binary.Pool[*MappedFileRange]
}

func OpenMappedFile(name string, flags int, perm fs.FileMode) (_ *MappedFile, err error) {
	f := new(MappedFile)
	f.mu = new(sync.RWMutex)
	f.pool = binary.NewPointerPool[MappedFileRange]()
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

func (f *MappedFile) Name() string {
	return f.file.Name()
}

func (f *MappedFile) Close() error {
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

type MappedFileRange struct {
	f        *MappedFile
	released atomic.Bool
	Offset   int64
	End      int64
}

func (f *MappedFile) Acquire() *MappedFileRange {
	f.mu.RLock()
	h := f.pool.Get()
	h.f = f
	h.released.Store(false)
	h.SetRange(0, int64(len(f.data)))
	return h
}

func (f *MappedFile) AcquireRange(start, end int64) *MappedFileRange {
	f.mu.RLock()
	h := f.pool.Get()
	h.f = f
	h.released.Store(false)
	h.SetRange(start, end)
	return h
}

func (f *MappedFileRange) AcquireRange(start, end int64) *MappedFileRange {
	return f.f.AcquireRange(start, end)
}

func (f *MappedFileRange) Release() {
	// Only release once
	if f.released.Swap(true) {
		return
	}

	f.f.mu.RUnlock()
	f.f.pool.Put(f)
	f.f = nil
}

func (f *MappedFileRange) SetRange(start, end int64) {
	f.Offset = max(start, 0)
	f.End = min(end, int64(len(f.f.data)))
}

func (f *MappedFileRange) Len() int {
	return int(f.End - f.Offset)
}

func (f *MappedFileRange) Raw() []byte {
	return f.f.data[f.Offset:f.End]
}

func (f *MappedFileRange) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		// Ok
	case io.SeekCurrent:
		offset += f.Offset
	case io.SeekEnd:
		offset += f.End
	default:
		return 0, errors.New("invalid whence")
	}
	if offset < 0 || offset > f.End {
		return 0, io.EOF
	}
	f.Offset = offset
	return offset, nil
}

func (f *MappedFileRange) Read(b []byte) (int, error) {
	n, err := f.ReadAt(b, f.Offset)
	f.Offset += int64(n)
	return n, err
}
func (f *MappedFileRange) Write(b []byte) (int, error) {
	n, err := f.WriteAt(b, f.Offset)
	f.Offset += int64(n)
	return n, err
}

func (f *MappedFileRange) ReadAt(b []byte, offset int64) (int, error) {
	if offset > int64(len(f.f.data)) {
		return 0, io.EOF
	}

	n := copy(b, f.f.data[offset:])
	if n < len(b) {
		return n, io.EOF
	}
	return n, nil
}

func (f *MappedFileRange) WriteAt(b []byte, offset int64) (int, error) {
	err := f.Truncate(offset + int64(len(b)))
	if err != nil {
		return 0, err
	}

	m := copy(f.f.data[offset:], b)
	return m, nil
}

func (f *MappedFileRange) ReadByte() (byte, error) {
	if f.Offset >= f.End {
		return 0, io.EOF
	}
	b := f.f.data[f.Offset]
	f.Offset++
	return b, nil
}

func (f *MappedFileRange) UnreadByte() error {
	if f.Offset <= 0 {
		return io.EOF
	}
	f.Offset--
	return nil
}

func (f *MappedFileRange) Truncate(size int64) error {
	for size > int64(len(f.f.data)) {
		err := f.truncateInner(size)
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *MappedFileRange) truncateInner(size int64) error {
	// Upgrade the lock
	demote := f.promote()
	defer demote()

	// Is the mapped region smaller than we want?
	if size <= int64(len(f.f.data)) {
		return nil
	}

	// Is the file smaller than we want?
	st, err := f.f.file.Stat()
	if err != nil {
		return err
	}

	if st.Size() >= size {
		size = st.Size()
	} else {

		// Expand the file
		err = f.f.file.Truncate(size)
		if err != nil {
			return err
		}
	}

	// Remove the old mapping
	if f.f.data != nil {
		err = f.f.data.Unmap()
		if err != nil {
			return err
		}
	}

	// Remap
	f.f.data, err = mmap.MapRegion(f.f.file, int(size), mmap.RDWR, 0, 0)
	return err
}

func (f *MappedFileRange) promote() (demote func()) {
	f.f.mu.RUnlock()
	f.f.mu.Lock()
	return func() {
		f.f.mu.Unlock()
		f.f.mu.RLock()
	}
}
