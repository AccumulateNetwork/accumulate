// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"
	stdbin "encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/core/schema/pkg/binary"
)

const fileHeaderSize = 1024

type blockFile struct {
	*config
	file   *file
	header *fileHeader
	dec    binary.Decoder
}

type entryAndData struct {
	recordEntry
	Value []byte
}

func newBlockFile(c *config, name string) (*blockFile, error) {
	f := new(blockFile)
	f.config = c
	f.header = new(fileHeader)

	var err error
	f.file, err = openFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL)
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

	_, err = f.file.WriteAt(0, b)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func (c *config) openFile(name string) (*blockFile, error) {
	f := new(blockFile)
	f.config = c
	f.header = new(fileHeader)

	var err error
	f.file, err = openFile(name, os.O_RDWR)
	if err != nil {
		return nil, err
	}

	if f.file.Len() < fileHeaderSize {
		return nil, errors.New("file header is missing or corrupted")
	}

	err = f.header.UnmarshalBinary(f.file.data[:fileHeaderSize])
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	return f, nil
}

func (f *blockFile) ReadHeader(l *recordLocation) (*recordEntry, error) {
	f.dec.Reset(f.file.ReadRange(l.Offset, l.Offset+l.HeaderLen))
	e := new(recordEntry)
	err := e.UnmarshalBinaryV2(&f.dec)
	return e, err
}

func (f *blockFile) ReadRecord(l *recordLocation) ([]byte, error) {
	if l.RecordLen < 0 {
		panic("record was deleted")
	}
	b := make([]byte, l.RecordLen)
	_, err := f.file.ReadAt(b, l.Offset+l.HeaderLen)
	return b, err
}

func (f *blockFile) Close() error {
	return f.file.Close()
}

func (f *blockFile) writeEntries(fileIndex int, view *recordIndexView, entries []*entryAndData, block *startBlockEntry) (n int, err error) {
	f.file.mu.Lock()
	defer f.file.mu.Unlock()

	// Check the limit
	var offset int64
	if st, err := f.file.file.Stat(); err != nil {
		return 0, err
	} else if st.Size() >= f.fileLimit+fileHeaderSize {
		return 0, nil
	} else {
		offset = st.Size()
	}

	// Allocate buffers
	bufLimit := min(
		len(entries)*1024,
		int(f.fileLimit+fileHeaderSize-offset))
	if bufLimit > 1<<22 {
		bufLimit = 1 << 22
	}
	w := new(entryWriter)
	w.mainBuf = bytes.NewBuffer(make([]byte, 0, bufLimit))
	w.encBuf = new(bytes.Buffer)
	w.enc = binary.NewEncoder(w.encBuf, binary.WithPool(binary.NewPool()))

	// Open the block
	_, err = w.write(block, nil)
	if err != nil {
		return 0, err
	}

	view.blocks.Put(block.blockID, fileIndex)

	// Write entries
	for n < len(entries) {
		e := entries[n]
		n++

		// Write the entry
		loc, err := w.write(&e.recordEntry, e.Value)
		if err != nil {
			return 0, err
		}

		// Update the index
		loc.Offset += offset
		loc.Block = block.blockID.Copy()
		view.records.Put(e.KeyHash, loc)

		// Check the file limit
		if offset+int64(w.mainBuf.Len()) > f.fileLimit+fileHeaderSize {
			break
		}

		// Write the buffer
		if w.mainBuf.Len() > bufLimit {
			_, err = f.file.WriteAt(offset, w.mainBuf.Bytes())
			if err != nil {
				return 0, err
			}
			offset += int64(w.mainBuf.Len())
			w.mainBuf.Reset()
		}
	}

	// Close the block
	_, err = w.write(&endBlockEntry{}, nil)
	if err != nil {
		return 0, err
	}

	// Write the buffer
	_, err = f.file.WriteAt(offset, w.mainBuf.Bytes())
	if err != nil {
		return 0, err
	}

	return n, nil
}

type entryWriter struct {
	mainBuf *bytes.Buffer
	encBuf  *bytes.Buffer
	lenBuf  [4]byte
	enc     *binary.Encoder
}

func (w *entryWriter) write(e entry, record []byte) (*recordLocation, error) {
	// Encode the entry
	w.encBuf.Reset()
	w.enc.Reset(w.encBuf)
	err := w.enc.Encode(e)
	if err != nil {
		return nil, err
	}
	if len(record) > 0 {
		_ = w.encBuf.WriteByte(binary.EmptyObject)
	}

	// Encode the length
	loc := new(recordLocation)
	loc.HeaderLen = int64(w.encBuf.Len())
	loc.RecordLen = int64(len(record))
	stdbin.BigEndian.PutUint32(w.lenBuf[:], uint32(loc.HeaderLen+loc.RecordLen))

	// Write to the main buffer
	_, _ = w.mainBuf.Write(w.lenBuf[:])
	loc.Offset = int64(w.mainBuf.Len())
	_, _ = io.Copy(w.mainBuf, w.encBuf)
	_, _ = w.mainBuf.Write(record)
	return loc, nil
}

func (f *blockFile) entries(want func(typ entryType) bool) *entryIterator {
	return &entryIterator{f, want, nil}
}

type entryIterator struct {
	*blockFile
	want func(entryType) bool
	err  error
}

type entryPos struct {
	entry
	Start int64
	End   int64
}

func (e *entryIterator) Range(yield func(int, entryPos) bool) {
	rd := e.file.ReadRange(0, fileHeaderSize)
	dec := new(binary.Decoder)
	var typ entryType
	for i := 0; rd.offset < int64(len(e.file.data)); i++ {
		rd.offset = rd.end

		// Read the length
		n := stdbin.BigEndian.Uint32(e.file.data[rd.offset:])
		rd.offset += 4
		start := rd.offset
		rd.end = rd.offset + int64(n)

		if e.want != nil {
			// Read the entry type
			dec.Reset(rd, binary.LeaveTrailing())
			err := dec.StartObject()
			if err != nil {
				e.err = err
				return
			}
			id, err := dec.Field()
			switch {
			case err == nil && id == 1:
				// Ok
			case err == nil /* and id != 1 */ || errors.Is(err, io.EOF):
				e.err = errors.New("field Type is missing")
				return
			default:
				e.err = err
				return
			}

			err = typ.UnmarshalBinaryV2(dec)
			if err != nil {
				e.err = err
				return
			}

			if !e.want(typ) {
				continue
			}
		}

		// Read the entry
		rd.offset = start
		dec.Reset(rd, binary.LeaveTrailing())
		entry, err := unmarshalEntryBinaryV2(dec)
		if err != nil {
			e.err = err
			return
		}

		// Yield
		pos := entryPos{
			entry: entry,
			Start: start,
			End:   rd.offset,
		}
		if !yield(i, pos) {
			return
		}
	}
}
