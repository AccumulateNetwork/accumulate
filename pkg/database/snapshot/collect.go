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

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

type recordKeyAndOffset struct {
	Key     record.KeyHash
	Section int
	Offset  uint64
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
		// Combine section and offset
		if x.Offset&0xFFFF0000_00000000 != 0 {
			return errors.NotAllowed.WithFormat("offset is too large")
		}
		v := x.Offset | (uint64(x.Section) << 6 * 8)

		// Section    - 2 bytes
		// Offset     - 6 bytes
		// Hash       - 32 bytes
		var b [40]byte
		binary.BigEndian.PutUint64(b[:8], v)
		*(*[32]byte)(b[8:]) = x.Key

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
	wr       *ioutil.SegmentWriter[SectionType, *SectionType]
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
		if offset >= 1<<(6*8) {
			return false, errors.NotAllowed.WithFormat("offset is too large")
		}

		// Write the record
		_, err = c.wr.Write(b)
		if err != nil {
			return false, errors.InternalError.WithFormat("write record: %w", err)
		}

		c.snapshot.index = append(c.snapshot.index, recordKeyAndOffset{
			Key:     v.Key().Hash(),
			Section: c.number,
			Offset:  uint64(offset),
		})
		return false, nil
	})
}
