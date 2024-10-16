// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	stdbin "encoding/binary"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type indexFile struct {
	level int
	file  *ioutil.MappedFile
	count atomic.Int64
}

func newIndexFile(name string, level int) (_ *indexFile, err error) {
	// Ensure the directory exists
	err = os.Mkdir(filepath.Dir(name), 0700)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, err
	}

	f := new(indexFile)
	f.level = level
	f.file, err = ioutil.OpenMappedFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	defer closeIfError(&err, f)

	// Always allocate 1024 entries
	h := f.file.Acquire()
	defer h.Release()
	err = h.Truncate(indexFileSize)
	if err != nil {
		return nil, err
	}

	return f, err
}

func openIndexFile(name string, level int) (_ *indexFile, err error) {
	f := new(indexFile)
	f.level = level
	f.file, err = ioutil.OpenMappedFile(name, os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	defer closeIfError(&err, f)

	h := f.file.Acquire()
	defer h.Release()
	if h.Len() != indexFileSize {
		return nil, fmt.Errorf("invalid size: want %d, got %d", indexFileSize, h.Len())
	}

	// Find the empty region at the end of the file and use that to determine
	// the number of entries
	data := h.Raw()
	f.count.Store(int64(sort.Search(indexFileEntryCount, func(i int) bool {
		offset := int64(i) * indexFileEntrySize
		return [32]byte(data[offset:]) == [32]byte{}
	})))

	return f, err
}

func (f *indexFile) Close() error {
	return f.file.Close()
}

func (f *indexFile) get(hash [32]byte) (*recordLocation, bool) {
	h := f.file.Acquire()
	defer h.Release()
	data := h.Raw()

	count := f.count.Load()
	index, match := searchIndex(data, 0, count, hash)
	if !match {
		return nil, false
	}

	loc := poolLocation.Get()
	offset := index * indexFileEntrySize
	readLocation((*[32]byte)(data[offset+32:]), loc)
	return loc, true
}

func (f *indexFile) forEach(fn func([32]byte, *recordLocation) error) error {
	h := f.file.Acquire()
	defer h.Release()

	var hash [64]byte
	loc := new(recordLocation)
	rd := h.AcquireRange(0, 0)
	defer rd.Release()
	for i, n := 0, f.count.Load(); i < int(n); i++ {
		offset := int64(i) * indexFileEntrySize
		_, err := h.ReadAt(hash[:], offset)
		if err != nil {
			return err
		}

		readLocation((*[32]byte)(hash[32:]), loc)
		err = fn([32]byte(hash[:]), loc)
		if err != nil {
			return err
		}
	}

	return nil
}

func (f *indexFile) commit(entries []*locationAndHash) (indexFileNode, error) {
	h := f.file.Acquire()
	defer h.Release()
	data := h.Raw()

	// Overwrite existing entries and determine where to insert new ones
	indices, entries := f.findAndWrite(data, entries)
	if len(entries) == 0 {
		return f, nil
	}

	// Does the file need to be split?
	if int(f.count.Load())+len(indices) <= len(data)/indexFileEntrySize {
		// No, insert the new entries
		f.insert(data, indices, entries)
		return f, nil
	}

	set, err := newIndexFileSet(f)
	if err != nil {
		return nil, err
	}

	// We have to release before closing the file to avoid a deadlock
	h.Release()

	path := f.file.Name()
	err = errors.Join(
		f.Close(),
		os.Remove(path),
	)
	if err != nil {
		return nil, err
	}

	return set.commit(entries)
}

func (f *indexFile) findAndWrite(data []byte, entries []*locationAndHash) ([]int64, []*locationAndHash) {
	var indices []int64
	var insert []*locationAndHash
	count := f.count.Load()
	for _, e := range entries {
		// Find the entry or the insertion point
		index, matches := searchIndex(data, 0, count, e.Hash)
		if matches {
			// Overwrite the previous value
			offset := index * indexFileEntrySize
			writeLocation((*[32]byte)(data[offset+32:]), e.Location)
			continue
		}

		// Sanity check
		if len(indices) > 0 && index < indices[len(indices)-1] {
			// The entries and/or the index is out of order
			panic("data is not sane")
		}

		// Value must be inserted
		indices = append(indices, index)
		insert = append(insert, e)
	}
	return indices, insert
}

func (f *indexFile) insert(data []byte, indices []int64, entries []*locationAndHash) {
	// Relocate existing entries to make space for the new ones
	var ranges [][2]int64
	count := f.count.Load()
	for _, i := range indices {
		if i >= count {
			continue
		}
		if len(ranges) == 0 || ranges[len(ranges)-1][0] != i {
			ranges = append(ranges, [2]int64{i, 1})
		} else {
			ranges[len(ranges)-1][1]++
		}
	}

	// b := unsafe.Slice((*[64]byte)(unsafe.Pointer(unsafe.SliceData(data))), len(data)/64)
	// _ = b

	if len(ranges) > 0 {
		for i := range ranges[1:] {
			ranges[i+1][1] += ranges[i][1]
		}
	}

	// Aggregate neighbors
	last := count
	for i := len(ranges) - 1; i >= 0; i-- {
		x, n := ranges[i][0], ranges[i][1]
		start, end := x*indexFileEntrySize, last*indexFileEntrySize
		dest := n * indexFileEntrySize
		copy(data[start+dest:end+dest], data[start:end])
		last = x
	}

	// Insert the new entries
	for i, e := range entries {
		offset := (int64(i) + indices[i]) * indexFileEntrySize
		copy(data[offset:], e.Hash[:])
		writeLocation((*[32]byte)(data[offset+32:]), e.Location)
	}

	f.count.Add(int64(len(entries)))
}

func writeLocation(buf *[32]byte, loc *recordLocation) {
	stdbin.BigEndian.PutUint64(buf[:], loc.Block.ID)
	stdbin.BigEndian.PutUint64(buf[8:], uint64(loc.Offset))
	stdbin.BigEndian.PutUint16(buf[16:], uint16(loc.Block.Part))
	stdbin.BigEndian.PutUint16(buf[18:], uint16(loc.HeaderLen))
	stdbin.BigEndian.PutUint32(buf[20:], uint32(loc.RecordLen))
}

func readLocation(buf *[32]byte, loc *recordLocation) {
	if loc.Block == nil {
		loc.Block = new(blockID)
	}
	loc.Block.ID = stdbin.BigEndian.Uint64(buf[:])
	loc.Offset = int64(stdbin.BigEndian.Uint64(buf[8:]))
	loc.Block.Part = uint64(stdbin.BigEndian.Uint16(buf[16:]))
	loc.HeaderLen = int64(int16(stdbin.BigEndian.Uint16(buf[18:])))
	loc.RecordLen = int64(int32(stdbin.BigEndian.Uint32(buf[20:])))
}
