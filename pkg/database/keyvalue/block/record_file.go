// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	stdbin "encoding/binary"
	"os"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/core/schema/pkg/binary"
)

const recordFileHeaderSize = 1024
const recordFilePageSize = 1 << 16

type recordFile struct {
	file *ioutil.MappedFile
	used atomic.Int64
}

func newRecordFile(name string) (_ *recordFile, err error) {
	f := new(recordFile)
	f.file, err = ioutil.OpenMappedFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	defer closeIfError(&err, f)

	// Write the header
	h := f.file.Acquire()
	defer h.Release()
	err = h.Truncate(recordFileHeaderSize)
	if err != nil {
		return nil, err
	}

	f.used.Store(recordFileHeaderSize)
	return f, (&recordFileWriter{&f.used, h}).Close()
}

func openRecordFile(name string) (_ *recordFile, err error) {
	f := new(recordFile)
	f.file, err = ioutil.OpenMappedFile(name, os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	defer closeIfError(&err, f)

	// Decode the header
	b := make([]byte, recordFileHeaderSize)
	h := f.file.Acquire()
	defer h.Release()
	_, err = h.ReadAt(b, 0)
	if err != nil {
		return nil, err
	}
	header := new(fileHeader)
	err = header.UnmarshalBinary(b)
	if err != nil {
		return nil, err
	}

	// Determine the end of the file
	h.Offset = recordFileHeaderSize
	for h.Len() > 4 {
		n := stdbin.BigEndian.Uint32(h.Raw())
		if n == 0 {
			break
		}
		h.Offset += int64(4 + n)
	}
	f.used.Store(h.Offset)

	return f, nil
}

func (f *recordFile) Close() error {
	return f.file.Close()
}

func (f *recordFile) ReadHeader(l *recordLocation) (*recordEntry, error) {
	h := f.file.Acquire()
	defer h.Release()
	if l.end() >= f.used.Load() {
		return nil, errors.InternalError.With("corrupted: record is past the end of the file")
	}

	rd := h.AcquireRange(l.Offset, l.Offset+l.HeaderLen)
	defer rd.Release()

	dec := poolDecoder.Get(rd, binary.LeaveTrailing())
	defer poolDecoder.Put(dec)

	e := new(recordEntry)
	err := e.UnmarshalBinaryV2(dec)
	return e, err
}

func (f *recordFile) ReadRecord(l *recordLocation) ([]byte, error) {
	if l.RecordLen < 0 {
		panic("record was deleted")
	}
	h := f.file.Acquire()
	defer h.Release()
	if l.end() >= f.used.Load() {
		return nil, errors.InternalError.With("corrupted: record is past the end of the file")
	}
	b := make([]byte, l.RecordLen)
	_, err := h.ReadAt(b, l.Offset+l.HeaderLen)
	return b, err
}

func (l *recordLocation) end() int64 {
	x := l.Offset + l.HeaderLen
	if l.RecordLen > 0 {
		x += l.RecordLen
	}
	return x
}
