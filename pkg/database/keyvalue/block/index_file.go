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
	"sort"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/core/schema/pkg/binary"
)

const indexFileEntrySize = 64
const indexFileAllocCount = 1 << 10

type indexFile struct {
	file  *file
	count atomic.Int64
}

func newIndexFile(name string) (*indexFile, error) {
	var err error
	f := new(indexFile)
	f.file, err = openFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL)
	return f, err
}

func openIndexFile(name string) (_ *indexFile, err error) {
	f := new(indexFile)
	f.file, err = openFile(name, os.O_RDWR)
	if err != nil {
		return nil, err
	}
	defer closeIfError(&err, f)

	if len(f.file.data)%indexFileEntrySize != 0 {
		return nil, fmt.Errorf("size is not a multiple of %d", indexFileEntrySize)
	}

	f.count.Store(int64(len(f.file.data)) / indexFileEntrySize)
	return f, err
}

func (f *indexFile) Close() error {
	st, err := f.file.file.Stat()
	if err != nil {
		return err
	}
	if c := f.count.Load(); st.Size()/indexFileEntrySize > c {
		err = f.file.file.Truncate(c * indexFileEntrySize)
	}
	return f.file.Close()
}

func (f *indexFile) Append(key *record.KeyHash, loc *recordLocation) error {
	for {
		ok, err := f.append(key, loc)
		if ok || err != nil {
			return err
		}
		err = f.grow(1)
		if err != nil {
			return err
		}
	}
}

func (f *indexFile) append(key *record.KeyHash, loc *recordLocation) (bool, error) {
	f.file.mu.RLock()
	defer f.file.mu.RUnlock()

	count := f.count.Load()
	for i := 0; i < int(count); i++ {
		offset := int64(i) * indexFileEntrySize
		if [32]byte(f.file.data[offset:]) == *key {
			return true, f.writeAt(key, loc, offset)
		}
	}

	if int64(len(f.file.data))/indexFileEntrySize <= count {
		return false, nil
	}

	offset := count * indexFileEntrySize
	f.count.Add(1)
	return true, f.writeAt(key, loc, offset)
}

func (f *indexFile) Insert(key *record.KeyHash, loc *recordLocation) error {
	for {
		ok, err := f.insert(key, loc)
		if ok || err != nil {
			return err
		}
		err = f.grow(1)
		if err != nil {
			return err
		}
	}
}

func (f *indexFile) insert(key *record.KeyHash, loc *recordLocation) (bool, error) {
	f.file.mu.RLock()
	defer f.file.mu.RUnlock()

	count := f.count.Load()
	i := f.find(count, *key)
	offset := i * indexFileEntrySize
	if i < count && [32]byte(f.file.data[offset:offset+32]) == *key {
		return true, f.writeAt(key, loc, offset)
	}

	// Do we need extra space?
	if int64(len(f.file.data))/indexFileEntrySize <= count {
		return false, nil
	}

	if i < count {
		end := count * indexFileEntrySize
		copy(f.file.data[offset+64:], f.file.data[offset:end])
	}
	f.count.Add(1)
	return true, f.writeAt(key, loc, offset)
}

func (f *indexFile) grow(extra int64) error {
	count := f.count.Load() + extra
	if count <= int64(len(f.file.data))/indexFileEntrySize {
		return nil
	}

	n := count
	if r := n % indexFileAllocCount; r != 0 {
		n += indexFileAllocCount - r
	}
	n *= indexFileEntrySize

	return f.file.Grow(n)
}

func (f *indexFile) writeAt(key *record.KeyHash, loc *recordLocation, offset int64) error {
	wr := f.file.WriteRange(offset, offset+64)
	return f.write(key, loc, wr)
}

func (f *indexFile) write(key *record.KeyHash, loc *recordLocation, wr io.Writer) error {
	_, err := wr.Write(key[:])
	if err != nil {
		return err
	}

	enc := binary.NewEncoder(wr)
	err = loc.MarshalBinaryV2(enc)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return io.ErrUnexpectedEOF
		}
		return err
	}

	_, err = wr.Write([]byte{binary.EmptyObject})
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func (f *indexFile) FindLinear(key *record.Key) (*recordLocation, error) {
	f.file.mu.RLock()
	defer f.file.mu.RUnlock()

	hash := key.Hash()

	var offset int64
	var found bool
	for i, c := 0, f.count.Load(); i < int(c); i++ {
		offset = int64(i) * indexFileEntrySize
		if [32]byte(f.file.data[offset:]) == hash {
			found = true
			break
		}
	}
	if !found {
		return nil, (*database.NotFoundError)(key)
	}

	return f.readAt(offset)
}

func (f *indexFile) FindBinary(key *record.Key) (*recordLocation, error) {
	hash := key.Hash()

	count := f.count.Load()
	index := f.find(count, hash)
	if index >= count {
		return nil, (*database.NotFoundError)(key)
	}

	offset := int64(index) * indexFileEntrySize
	if [32]byte(f.file.data[offset:offset+32]) != hash {
		return nil, (*database.NotFoundError)(key)
	}

	return f.readAt(offset)
}

func (f *indexFile) find(count int64, hash record.KeyHash) int64 {
	return int64(sort.Search(int(count), func(i int) bool {
		offset := i * indexFileEntrySize
		return bytes.Compare(hash[:], f.file.data[offset:offset+32]) <= 0
	}))
}

func (f *indexFile) readAt(offset int64) (*recordLocation, error) {
	rd := f.file.ReadRange(offset+32, offset+64)
	dec := binary.NewDecoder(rd)
	loc := new(recordLocation)
	err := loc.UnmarshalBinaryV2(dec)
	return loc, err
}
