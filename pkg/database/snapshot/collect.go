// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

func (w *Writer) OpenRecords() (*Collector, error) {
	c := new(Collector)
	c.number = w.sections

	var err error
	c.wr, err = w.OpenRaw(SectionTypeRecords)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open records section: %w", err)
	}
	return c, nil
}

type Collector struct {
	wr     *SectionWriter
	number int
}

type CollectOptions struct {
	Walk       database.WalkOptions
	Predicate  func(database.Record) (bool, error)
	DidCollect func(value database.Value, section, offset uint64) error
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
			if !ok {
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
		err = c.wr.WriteValue(&RecordEntry{Key: v.Key(), Value: b})
		if err != nil {
			return false, errors.InternalError.WithFormat("write record: %w", err)
		}

		if opts.DidCollect != nil {
			err = opts.DidCollect(v, uint64(c.number), uint64(offset))
			if err != nil {
				return false, errors.UnknownError.Wrap(err)
			}
		}

		return false, nil
	})
}
