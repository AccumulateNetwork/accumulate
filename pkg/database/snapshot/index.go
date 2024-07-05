// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"bytes"
	"log/slog"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type Indexer struct {
	wr   *SectionWriter
	last [32]byte
}

func (w *Writer) OpenIndex() (*Indexer, error) {
	wr, err := w.OpenRaw(SectionTypeRecordIndex)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open record index section: %w", err)
	}

	return &Indexer{wr: wr}, nil
}

func (i *Indexer) Close() error {
	return i.wr.Close()
}

func (i *Indexer) Write(e RecordIndexEntry) error {
	// Keys must be in descending order
	c := bytes.Compare(i.last[:], e.Key[:])
	switch {
	case c == 0:
		slog.Warn("Duplicate key in index", "key", e.Key)
		// return errors.BadRequest.WithFormat("duplicate key")
	case c > 0:
		return errors.BadRequest.WithFormat("keys must be sorted in descending order")
	}
	i.last = e.Key
	err := e.writeTo(i.wr)
	return errors.UnknownError.Wrap(err)
}
