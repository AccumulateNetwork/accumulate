// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"bytes"
	"encoding/binary"
	"io"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

type recordIndexEntry struct {
	Key     record.KeyHash
	Section int
	Offset  uint64
	Size    uint64
}

func (w *Writer) Open() (*Collector, error) {
	c := new(Collector)
	c.snapshot = w
	c.number = w.sections

	var err error
	c.wr, err = w.open(SectionTypeRecords)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open records section: %w", err)
	}
	return c, nil
}

// Section - 2 bytes
// Size    - 6 bytes
// Offset  - 8 bytes
// Hash    - 32 bytes
const indexEntrySize = 2 + 6 + 8 + 32

func (w *Writer) WriteIndex() error {
	wr, err := w.open(SectionTypeRecordIndex)
	if err != nil {
		return errors.UnknownError.WithFormat("open record index section: %w", err)
	}

	index := w.index
	w.index = w.index[:0]

	// The index must be sorted
	sort.Slice(index, func(i, j int) bool {
		a, b := index[i], index[j]
		return bytes.Compare(a.Key[:], b.Key[:]) < 0
	})

	for _, x := range index {
		// Combine section and size
		if x.Size&0xFFFF0000_00000000 != 0 {
			return errors.NotAllowed.WithFormat("size is too large")
		}
		v := x.Size | (uint64(x.Section) << (6 * 8))

		var b [indexEntrySize]byte
		binary.BigEndian.PutUint64(b[:8], v)
		binary.BigEndian.PutUint64(b[8:16], x.Offset)
		*(*[32]byte)(b[16:]) = x.Key

		_, err = wr.Write(b[:])
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	err = wr.Close()
	return errors.UnknownError.Wrap(err)
}

type Collector struct {
	snapshot *Writer
	wr       *sectionWriter
	number   int
}

func (c *Collector) Close() error {
	return c.wr.Close()
}

func (c *Collector) Collect(r database.Record, opts database.WalkOptions) error {
	opts.Values = true
	return r.Walk(opts, func(r database.Record) (skip bool, err error) {
		// Load the value
		v, ok := r.(database.Value)
		if !ok {
			return false, errors.Conflict.WithFormat("asked for values but got %T", r)
		}

		u, _, err := v.GetValue()
		if err != nil {
			return false, errors.UnknownError.WithFormat("get record value: %w", err)
		}

		// Marshal the record
		b, err := u.MarshalBinary()
		if err != nil {
			return false, errors.EncodingError.WithFormat("marshal record value: %w", err)
		}

		b, err = (&recordEntry{Key: v.Key(), Value: b}).MarshalBinary()
		if err != nil {
			return false, errors.EncodingError.WithFormat("marshal record value: %w", err)
		}

		// Get the current offset
		offset, err := c.wr.Seek(0, io.SeekCurrent)
		if err != nil {
			return false, errors.InternalError.WithFormat("get offset: %w", err)
		}
		if offset < 0 {
			return false, errors.InternalError.WithFormat("offset is negative")
		}
		if len(b) >= 1<<(6*8) {
			return false, errors.NotAllowed.WithFormat("record is too large")
		}

		// Write the record
		_, err = c.wr.Write(b)
		if err != nil {
			return false, errors.InternalError.WithFormat("write record: %w", err)
		}

		c.snapshot.index = append(c.snapshot.index, recordIndexEntry{
			Key:     v.Key().Hash(),
			Section: c.number,
			Offset:  uint64(offset),
			Size:    uint64(len(b)),
		})
		return false, nil
	})
}
