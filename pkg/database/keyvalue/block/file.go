// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/edsrzf/mmap-go"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/core/schema/pkg/binary"
)

const fileHeaderSize = 1024

type blockFile struct {
	*config
	mu     *sync.RWMutex
	file   *os.File
	data   mmap.MMap
	header *fileHeader
}

type entryAndData struct {
	recordEntry
	Value []byte
}

func newFile(c *config, name string) (*blockFile, error) {
	f := new(blockFile)
	f.config = c
	f.mu = new(sync.RWMutex)
	f.header = new(fileHeader)

	var err error
	f.file, err = os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}

	b, err := f.header.MarshalBinary()
	if err != nil {
		return nil, err
	}
	if len(b) > fileHeaderSize {
		panic("header is too big")
	}

	if len(b) < fileHeaderSize {
		b = append(b, encoding.EmptyObject)
	}
	b = append(b, make([]byte, fileHeaderSize-len(b))...)

	_, err = f.file.Write(b)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func (c *config) openFile(name string) (*blockFile, error) {
	f := new(blockFile)
	f.config = c
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

	err = f.header.UnmarshalBinary(f.data[:fileHeaderSize])
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

func (f *blockFile) writeEntries(fileIndex int, view *recordIndexView, entries []*entryAndData, block *startBlockEntry) (n int, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Check the limit
	var offset int64
	if st, err := f.file.Stat(); err != nil {
		return 0, err
	} else if st.Size() >= f.fileLimit+fileHeaderSize {
		return 0, nil
	} else {
		offset = st.Size()
	}

	// Allocate a buffer
	bufLimit := min(
		len(entries)*1024,
		int(f.fileLimit+fileHeaderSize-offset))
	if bufLimit > 1<<22 {
		bufLimit = 1 << 22
	}
	buf := bytes.NewBuffer(make([]byte, 0, bufLimit))

	// Open the block
	enc := binary.NewEncoder(buf, binary.WithPool(binary.NewPool()))
	err = block.MarshalBinaryV2(enc)
	if err != nil {
		return 0, err
	}
	_ = buf.WriteByte(binary.EmptyObject)

	view.blocks.Put(block.blockID, fileIndex)

	// Write entries
	for n < len(entries) {
		e := entries[n]
		n++

		// Write the header
		before := buf.Len()
		enc.Reset(buf)
		err = e.MarshalBinaryV2(enc)
		if err != nil {
			return 0, err
		}
		_ = buf.WriteByte(binary.EmptyObject)

		// Update the index
		view.records.Put(e.KeyHash, &recordLocation{
			Block:     block.blockID.Copy(),
			Offset:    offset + int64(before),
			HeaderLen: int64(buf.Len() - before),
			RecordLen: e.Length,
		})

		// Write the data
		_, _ = buf.Write(e.Value)

		// Check the file limit
		if offset+int64(buf.Len()) > f.fileLimit+fileHeaderSize {
			break
		}

		// Write the buffer
		if buf.Len() > bufLimit {
			err = f.writeAt(offset, buf.Bytes())
			if err != nil {
				return 0, err
			}
			offset += int64(buf.Len())
			buf.Reset()
		}
	}

	// Close the block
	enc.Reset(buf)
	err = (&endBlockEntry{}).MarshalBinaryV2(enc)
	if err != nil {
		return 0, err
	}
	_ = buf.WriteByte(binary.EmptyObject)

	// Write the buffer
	err = f.writeAt(offset, buf.Bytes())
	if err != nil {
		return 0, err
	}

	return n, nil
}

func (f *blockFile) writeAt(offset int64, b []byte) error {
	err := f.unmap()
	if err != nil {
		return err
	}

	n := offset + int64(len(b))
	err = f.file.Truncate(n)
	if err != nil {
		return err
	}

	f.data, err = mmap.MapRegion(f.file, int(n), mmap.RDWR, 0, 0)
	if err != nil {
		return err
	}

	copy(f.data[offset:], b)
	return nil
}

func (f *blockFile) unmap() error {
	if f.data == nil {
		return nil
	}

	err := f.data.Unmap()
	f.data = nil
	return err
}

func (f *blockFile) entries() *entryIterator {
	return &entryIterator{f, nil}
}

func (f *blockFile) ReadRange(start, end int64) *offsetReader {
	return &offsetReader{f, start, end}
}

type entryIterator struct {
	*blockFile
	err error
}

type entryPos struct {
	entry
	Start int64
	End   int64
}

func (e *entryIterator) Range(yield func(int, entryPos) bool) {
	rd := e.ReadRange(fileHeaderSize, 0)
	dec := new(binary.Decoder)
	for i := 0; rd.Len() > 0; i++ {
		dec.Reset(rd, binary.LeaveTrailing())
		header := rd.offset
		entry, err := unmarshalEntryBinaryV2(dec)
		if err != nil {
			e.err = err
			return
		}
		pos := entryPos{
			entry: entry,
			Start: header,
			End:   rd.offset,
		}
		if !yield(i, pos) {
			return
		}
		if r, ok := entry.(*recordEntry); ok && r.Length > 0 {
			rd.offset += r.Length
		}
	}
}

type offsetReader struct {
	*blockFile
	offset int64
	end    int64
}

func (r *offsetReader) Len() int {
	if r.end > 0 {
		return int(r.end - r.offset)
	}
	return len(r.data) - int(r.offset)
}

func (r *offsetReader) Read(b []byte) (int, error) {
	if n := r.Len(); n == 0 {
		return 0, io.EOF
	} else if len(b) > n {
		b = b[:n]
	}
	n, err := r.ReadAt(b, r.offset)
	r.offset += int64(n)
	return n, err
}

func (r *offsetReader) ReadByte() (byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Len() <= 0 {
		return 0, io.EOF
	}
	b := r.data[r.offset]
	r.offset++
	return b, nil
}

func (r *offsetReader) UnreadByte() error {
	if r.offset <= 0 {
		return io.EOF
	}
	r.offset--
	return nil
}
