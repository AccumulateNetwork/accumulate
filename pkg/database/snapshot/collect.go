// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"bytes"
	"io"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

func (w *Writer) OpenRecords() (*Collector, error) {
	c := new(Collector)
	c.snapshot = w
	c.number = w.sections

	var err error
	c.wr, err = w.OpenRaw(SectionTypeRecords)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open records section: %w", err)
	}
	return c, nil
}

func (w *Writer) WriteIndex() error {
	wr, err := w.OpenRaw(SectionTypeRecordIndex)
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
		err = x.writeTo(wr)
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

type CollectOptions struct {
	Walk      database.WalkOptions
	Predicate func(database.Record) (bool, error)
}

func (c *Collector) Close() error {
	return c.wr.Close()
}

func (c *Collector) Collect(r database.Record, opts CollectOptions) error {
	if opts.Predicate == nil {
		opts.Walk.Values = true
	}

	return r.Walk(opts.Walk, func(r database.Record) (skip bool, err error) {
		if opts.Predicate != nil {
			ok, err := opts.Predicate(r)
			if err != nil {
				return false, errors.UnknownError.Wrap(err)
			}
			if ok {
				return true, nil
			}
		}

		// Load the value
		v, ok := r.(database.Value)
		if !ok {
			return false, nil
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

		// Write the record and its size
		_, err = writeValue(c.wr, &RecordEntry{Key: v.Key(), Value: b})
		if err != nil {
			return false, errors.InternalError.WithFormat("write record: %w", err)
		}

		c.snapshot.index = append(c.snapshot.index, RecordIndexEntry{
			Key:     v.Key().Hash(),
			Section: c.number,
			Offset:  uint64(offset),
		})
		return false, nil
	})
}
