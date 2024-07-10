// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/core/schema/pkg/binary"
)

type recordFileWriter struct {
	used *atomic.Int64
	file *ioutil.MappedFileRange
}

func (w *recordFileWriter) Len() int {
	return int(w.used.Load())
}

func (w *recordFileWriter) Write(b []byte) (int, error) {
	offset := w.used.Load()
	size := offset + int64(len(b))
	if r := size % recordFilePageSize; r != 0 {
		size += recordFilePageSize - r
	}

	err := w.file.Truncate(size)
	if err != nil {
		return 0, err
	}

	m, err := w.file.WriteAt(b, offset)
	if err != nil {
		return 0, err
	}

	w.used.Add(int64(m))
	return m, nil
}

func (w *recordFileWriter) Close() error {
	header := &fileHeader{}
	b, err := header.MarshalBinary()
	if err != nil {
		panic(fmt.Errorf("encode header: %w", err))
	}
	b = append(b, binary.EmptyObject)
	if len(b) > recordFileHeaderSize {
		panic("header is too big")
	}

	_, err = w.file.WriteAt(b, 0)
	return err
}
