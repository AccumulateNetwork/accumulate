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
	"io"

	"gitlab.com/accumulatenetwork/core/schema/pkg/binary"
)

type recordFileSetIterator struct {
	set  *recordFileSet
	want func(entryType) bool
	err  error
}

type recordFileIterator struct {
	file *recordFile
	want func(entryType) bool
	err  error
}

type recordFilePos struct {
	File      *recordFile
	FileIndex int
	recordPos
}

type recordPos struct {
	Entry entry
	Start int64
	End   int64
}

func (f *recordFileSet) entries(want func(typ entryType) bool) *recordFileSetIterator {
	return &recordFileSetIterator{f, want, nil}
}

func (f *recordFile) entries(want func(typ entryType) bool) *recordFileIterator {
	return &recordFileIterator{f, want, nil}
}

func (it *recordFileSetIterator) Range(yield func(int, recordFilePos) bool) {
	var j int
	for i, file := range it.set.files {
		fit := file.entries(it.want)
		cont := fit.Range(func(_ int, pos recordPos) bool {
			cont := yield(j, recordFilePos{file, i, pos})
			j++
			return cont
		})
		if !cont || fit.err != nil {
			it.err = fit.err
			break
		}
	}
}

func (it *recordFileIterator) Range(yield func(int, recordPos) bool) bool {
	rd := it.file.file.AcquireRange(0, recordFileHeaderSize)
	defer rd.Release()

	dec := poolDecoder.Get(bytes.NewReader(nil))
	defer poolDecoder.Put(dec)

	var typ entryType
	for i := 0; rd.Offset < it.file.used.Load(); i++ {
		rd.Offset = rd.End
		rd.End += 4

		// Read the length
		n := stdbin.BigEndian.Uint32(rd.Raw())
		rd.Offset = rd.End
		start := rd.Offset
		rd.End = rd.Offset + int64(n)

		if it.want != nil {
			// Read the entry type
			dec.Reset(rd, binary.LeaveTrailing())
			err := dec.StartObject()
			if err != nil {
				it.err = err
				return false
			}
			id, err := dec.Field()
			switch {
			case err == nil && id == 1:
				// Ok
			case err == nil /* and id != 1 */ || errors.Is(err, io.EOF):
				it.err = errors.New("field Type is missing")
				return false
			default:
				it.err = err
				return false
			}

			err = typ.UnmarshalBinaryV2(dec)
			if err != nil {
				it.err = err
				return false
			}

			if !it.want(typ) {
				continue
			}
		}

		// Read the entry
		rd.Offset = start
		dec.Reset(rd, binary.LeaveTrailing())
		entry, err := unmarshalEntryBinaryV2(dec)
		if err != nil {
			it.err = err
			return false
		}

		// Yield
		pos := recordPos{
			Entry: entry,
			Start: start,
			End:   rd.Offset,
		}
		if !yield(i, pos) {
			return false
		}
	}
	return true
}
